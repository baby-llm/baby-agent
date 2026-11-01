package agent

import (
	"babyagent/ch11/agent/tool"
	"babyagent/ch11/observe"
	"babyagent/shared"
	"context"
	"encoding/json"
	"fmt"
	"github.com/openai/openai-go/v3"
	"go.opentelemetry.io/otel/attribute"
	"time"
)

const SystemPrompt = `# BabyAgent

You are BabyAgent, a helpful coding assistant.

## Guidelines
- State intent before tool calls, but NEVER predict or claim results before receiving them.
- Before modifying a file, read it first. Do not assume files or directories exist.
- If a tool call fails, analyze the error before retrying with a different approach.
- Ask for clarification when the request is ambiguous.

Reply directly with text for conversations.`

type Agent struct {
	model        string
	client       openai.Client
	nativeTools  map[tool.AgentTool]tool.Tool
	systemPrompt string
	metrics      *observe.Metrics
}

func NewAgent(conf shared.ModelConfig, prompt string, tools []tool.Tool, metrics *observe.Metrics) *Agent {
	a := &Agent{model: conf.Model, client: shared.NewLLMClient(conf), nativeTools: map[tool.AgentTool]tool.Tool{}, systemPrompt: prompt, metrics: metrics}
	for _, t := range tools {
		a.nativeTools[t.ToolName()] = t
	}
	return a
}
func (a *Agent) Model() string { return a.model }
func (a *Agent) buildTools() []openai.ChatCompletionToolUnionParam {
	r := make([]openai.ChatCompletionToolUnionParam, 0, len(a.nativeTools))
	for _, t := range a.nativeTools {
		r = append(r, t.Info())
	}
	return r
}

type RunResult struct {
	Response string
	Rounds   []shared.OpenAIMessage
	Usage    openai.CompletionUsage
}

func (a *Agent) RunStreaming(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, query string, events chan<- StreamEvent, trace *observe.AgentTrace) (RunResult, error) {
	messages := append([]openai.ChatCompletionMessageParamUnion{openai.SystemMessage(a.systemPrompt)}, history...)
	messages = append(messages, openai.UserMessage(query))
	rounds := []shared.OpenAIMessage{openai.UserMessage(query)}
	var usage openai.CompletionUsage
	var response string
	for loop := 1; ; loop++ {
		llmStarted := time.Now()
		llmCtx, span := observe.StartLLMSpan(ctx, a.model, loop)
		stream := a.client.Chat.Completions.NewStreaming(llmCtx, openai.ChatCompletionNewParams{Model: a.model, Messages: messages, Tools: a.buildTools(), StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}})
		acc := openai.ChatCompletionAccumulator{}
		var firstToken time.Time
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			if (delta.Content != "" || delta.ReasoningContent != "") && firstToken.IsZero() {
				firstToken = time.Now()
			}
			if reasoningContent := reasoningText(delta); reasoningContent != "" {
				if !emitEvent(ctx, events, StreamEvent{Event: EventReasoning, ReasoningContent: reasoningContent}) {
					span.End()
					return RunResult{}, ctx.Err()
				}
			}
			if delta.Content != "" {
				if !emitEvent(ctx, events, StreamEvent{Event: EventContent, Content: delta.Content}) {
					span.End()
					return RunResult{}, ctx.Err()
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
			if !emitEvent(ctx, events, StreamEvent{Event: EventError, Content: err.Error()}) {
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
		span.SetAttributes(attribute.Int("gen_ai.usage.input_tokens", int(usage.PromptTokens)), attribute.Int("gen_ai.usage.output_tokens", int(usage.CompletionTokens)))
		span.End()
		if a.metrics != nil {
			a.metrics.ObserveLLM(a.model, firstTokenDuration, llmDuration-firstTokenDuration, usage.PromptTokens, usage.CompletionTokens)
		}
		trace.LLM(llmDuration, usage.PromptTokens, usage.CompletionTokens, firstTokenDuration, len(message.ToolCalls))
		// The baby-llm/openai-go fork carries reasoning_content back through
		// ToParam(), which thinking providers require after a tool-call turn.
		assistant := message.ToParam()
		messages = append(messages, assistant)
		rounds = append(rounds, assistant)
		if len(message.ToolCalls) == 0 {
			response = message.Content
			break
		}
		for _, call := range message.ToolCalls {
			if !emitEvent(ctx, events, StreamEvent{Event: EventToolCall, ToolCall: call.Function.Name, ToolArguments: call.Function.Arguments}) {
				return RunResult{}, ctx.Err()
			}
			toolStarted := time.Now()
			toolCtx, toolSpan := observe.StartToolSpan(ctx, call.Function.Name)
			t, ok := a.nativeTools[call.Function.Name]
			var result string
			var err error
			if !ok {
				err = fmt.Errorf("tool not found: %s", call.Function.Name)
			} else {
				result, err = t.Execute(toolCtx, call.Function.Arguments)
			}
			duration := time.Since(toolStarted)
			if err != nil {
				// A failed tool is a tool result, not a fatal stream error.
				result = err.Error()
				toolSpan.RecordError(err)
			}
			toolSpan.End()
			status := "ok"
			if err != nil {
				status = "error"
			}
			if a.metrics != nil {
				a.metrics.ToolCalls.WithLabelValues(call.Function.Name, status).Inc()
				a.metrics.ToolDuration.WithLabelValues(call.Function.Name, status).Observe(duration.Seconds())
			}
			trace.Tool(call.Function.Name, duration, err)
			if !emitEvent(ctx, events, StreamEvent{Event: EventToolResult, ToolCall: call.Function.Name, ToolResult: result}) {
				return RunResult{}, ctx.Err()
			}
			m := openai.ToolMessage(result, call.ID)
			messages = append(messages, m)
			rounds = append(rounds, m)
		}
		select {
		case <-ctx.Done():
			return RunResult{Response: response, Rounds: rounds, Usage: usage}, ctx.Err()
		default:
		}
	}
	return RunResult{Response: response, Rounds: rounds, Usage: usage}, nil
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

// reasoningText reads reasoning_content from the SDK field added by the
// baby-llm/openai-go fork. Providers that use "reasoning" or "thinking"
// instead are handled as a display-only fallback.
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
