package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"go.opentelemetry.io/otel/attribute"

	"babyagent/ch11/agent/tool"
	"babyagent/ch11/observe"
	"babyagent/shared"
)

const SystemPrompt = `# BabyAgent

You are BabyAgent, a helpful coding assistant.

## Guidelines
- State intent before tool calls, but NEVER predict or claim results before receiving them.
- Before modifying a file, read it first. Do not assume files or directories exist.
- If a tool call fails, analyze the error before retrying with a different approach.
- Ask for clarification when the request is ambiguous.

Reply directly with text for conversations.
`

type Agent struct {
	model        string
	client       openai.Client
	nativeTools  map[tool.AgentTool]tool.Tool
	systemPrompt string
	metrics      *observe.Metrics
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool, metrics *observe.Metrics) *Agent {
	a := &Agent{
		model:        modelConf.Model,
		client:       shared.NewLLMClient(modelConf),
		nativeTools:  make(map[tool.AgentTool]tool.Tool),
		systemPrompt: systemPrompt,
		metrics:      metrics,
	}
	for _, t := range tools {
		a.nativeTools[t.ToolName()] = t
	}
	return a
}

func (a *Agent) Model() string {
	return a.model
}

func (a *Agent) findTool(toolName string) (tool.Tool, bool) {
	t, ok := a.nativeTools[tool.AgentTool(toolName)]
	return t, ok
}

func (a *Agent) buildTools() []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0, len(a.nativeTools))
	for _, t := range a.nativeTools {
		tools = append(tools, t.Info())
	}
	return tools
}

// RunResult 是 Agent 一轮运行的结果
type RunResult struct {
	Response string
	Rounds   []shared.OpenAIMessage
	Usage    openai.CompletionUsage
}

// RunStreaming 执行 agent loop，通过 eventCh 流式输出，结束后返回 RunResult。
// 与 ch10 相比，这里额外把 LLM / tool 的耗时与 token 写入 trace 和 Prometheus。
func (a *Agent) RunStreaming(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, query string, eventCh chan<- StreamEvent, trace *observe.AgentTrace) (RunResult, error) {
	// 构建本轮消息：system + 历史 + 当前 user 消息
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+2)
	messages = append(messages, openai.SystemMessage(a.systemPrompt))
	messages = append(messages, history...)
	messages = append(messages, openai.UserMessage(query))

	// roundMessages 记录本轮新增消息（user + assistant + tool，不含 system 和历史）
	roundMessages := []shared.OpenAIMessage{openai.UserMessage(query)}

	var usage openai.CompletionUsage
	var finalResponse string

	for loop := 1; ; loop++ {
		llmStarted := time.Now()
		llmCtx, span := observe.StartLLMSpan(ctx, a.model, loop)

		params := openai.ChatCompletionNewParams{
			Model:         a.model,
			Messages:      messages,
			Tools:         a.buildTools(),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
		}

		stream := a.client.Chat.Completions.NewStreaming(llmCtx, params)
		acc := openai.ChatCompletionAccumulator{}
		var firstToken time.Time

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if (delta.Content != "" || delta.ReasoningContent != "") && firstToken.IsZero() {
					firstToken = time.Now()
				}
				if reasoningContent := reasoningText(delta); reasoningContent != "" {
					if !emitEvent(ctx, eventCh, StreamEvent{Event: EventReasoning, ReasoningContent: reasoningContent}) {
						stream.Close()
						span.End()
						return RunResult{}, ctx.Err()
					}
				}
				if delta.Content != "" {
					if !emitEvent(ctx, eventCh, StreamEvent{Event: EventContent, Content: delta.Content}) {
						stream.Close()
						span.End()
						return RunResult{}, ctx.Err()
					}
				}
			}
		}
		// 显式关闭流以释放底层 HTTP 连接。不能用 defer：stream 在 agent 主循环里每轮重建，
		// defer 会等到函数返回才关闭，反而累积泄漏。Close 不影响后续 stream.Err() 读取。
		stream.Close()
		llmDuration := time.Since(llmStarted)
		firstTokenDuration := time.Duration(0)
		if !firstToken.IsZero() {
			firstTokenDuration = firstToken.Sub(llmStarted)
		}

		if err := stream.Err(); err != nil {
			span.RecordError(err)
			span.End()
			if !emitEvent(ctx, eventCh, StreamEvent{Event: EventError, Content: err.Error()}) {
				return RunResult{}, ctx.Err()
			}
			return RunResult{}, err
		}
		if len(acc.Choices) == 0 {
			span.End()
			break
		}

		usage = acc.Usage
		message := acc.Choices[0].Message
		span.SetAttributes(
			attribute.Int("gen_ai.usage.input_tokens", int(usage.PromptTokens)),
			attribute.Int("gen_ai.usage.output_tokens", int(usage.CompletionTokens)),
		)
		span.End()

		if a.metrics != nil {
			a.metrics.ObserveLLM(a.model, firstTokenDuration, llmDuration-firstTokenDuration, usage.PromptTokens, usage.CompletionTokens)
		}
		trace.LLM(llmDuration, usage.PromptTokens, usage.CompletionTokens, firstTokenDuration, len(message.ToolCalls))

		assistantMsg := message.ToParam()
		messages = append(messages, assistantMsg)
		roundMessages = append(roundMessages, assistantMsg)

		// 没有 tool call，结束 loop
		if len(message.ToolCalls) == 0 {
			finalResponse = message.Content
			break
		}

		// 执行 tool calls
		for _, toolCall := range message.ToolCalls {
			if !emitEvent(ctx, eventCh, StreamEvent{Event: EventToolCall, ToolCall: toolCall.Function.Name, ToolArguments: toolCall.Function.Arguments}) {
				return RunResult{}, ctx.Err()
			}

			toolStarted := time.Now()
			toolCtx, toolSpan := observe.StartToolSpan(ctx, toolCall.Function.Name)
			t, ok := a.findTool(toolCall.Function.Name)
			var toolResult string
			var err error
			if !ok {
				err = fmt.Errorf("tool not found: %s", toolCall.Function.Name)
			} else {
				toolResult, err = t.Execute(toolCtx, toolCall.Function.Arguments)
			}
			duration := time.Since(toolStarted)
			if err != nil {
				// 工具失败是一条 tool result，不是致命的流错误：把错误文本交回
				// 模型，让它决定下一步。UI 会把它渲染在工具卡片里。
				toolResult = err.Error()
				toolSpan.RecordError(err)
			}
			toolSpan.End()

			status := "ok"
			if err != nil {
				status = "error"
			}
			if a.metrics != nil {
				// 工具名来自模型输出，未知名字统一归到 unknown，避免指标基数失控。
				metricName := toolCall.Function.Name
				if !ok {
					metricName = "unknown"
				}
				a.metrics.ToolCalls.WithLabelValues(metricName, status).Inc()
				a.metrics.ToolDuration.WithLabelValues(metricName, status).Observe(duration.Seconds())
			}
			trace.Tool(toolCall.Function.Name, duration, err)

			if !emitEvent(ctx, eventCh, StreamEvent{Event: EventToolResult, ToolCall: toolCall.Function.Name, ToolResult: toolResult}) {
				return RunResult{}, ctx.Err()
			}

			toolMsg := openai.ToolMessage(toolResult, toolCall.ID)
			messages = append(messages, toolMsg)
			roundMessages = append(roundMessages, toolMsg)
		}

		// 检查 context 是否取消
		select {
		case <-ctx.Done():
			return RunResult{Response: finalResponse, Rounds: roundMessages, Usage: usage}, ctx.Err()
		default:
		}
	}

	return RunResult{
		Response: finalResponse,
		Rounds:   roundMessages,
		Usage:    usage,
	}, nil
}

// emitEvent sends an event to the stream, but aborts when the request context
// is canceled (for example, the browser tab was closed).
func emitEvent(ctx context.Context, ch chan<- StreamEvent, e StreamEvent) bool {
	select {
	case ch <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

// reasoningText returns reasoning content from the SDK field added by the
// baby-llm/openai-go fork. Some providers use "reasoning" or "thinking"
// instead of "reasoning_content"; those aliases are display-only fallbacks
// because upstream openai-go does not model them.
func reasoningText(delta openai.ChatCompletionChunkChoiceDelta) string {
	if delta.ReasoningContent != "" {
		return delta.ReasoningContent
	}

	var extra struct {
		Reasoning string `json:"reasoning"`
		Thinking  string `json:"thinking"`
	}
	if err := json.Unmarshal([]byte(delta.RawJSON()), &extra); err != nil {
		return ""
	}
	if extra.Reasoning != "" {
		return extra.Reasoning
	}
	return extra.Thinking
}
