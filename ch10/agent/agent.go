package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"

	"babyagent/ch10/agent/tool"
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
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool) *Agent {
	a := &Agent{
		model:        modelConf.Model,
		client:       shared.NewLLMClient(modelConf),
		nativeTools:  make(map[tool.AgentTool]tool.Tool),
		systemPrompt: systemPrompt,
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

// executeTool 执行单个 tool call，返回 tool result 和错误。
// tool 不存在时返回错误；Execute 失败时返回错误，result 为错误信息。
func (a *Agent) executeTool(ctx context.Context, toolCall openai.ChatCompletionMessageToolCallUnion) (string, error) {
	t, ok := a.findTool(toolCall.Function.Name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolCall.Function.Name)
	}
	return t.Execute(ctx, toolCall.Function.Arguments)
}

// RunResult 是 Agent 一轮运行的结果
type RunResult struct {
	Response string
	Rounds   []shared.OpenAIMessage
	Usage    openai.CompletionUsage
}

// RunStreaming 执行 agent loop，通过 eventCh 流式输出，结束后返回 RunResult
// history 是本会话之前所有 ChatMessage.Rounds 反序列化后的消息列表
func (a *Agent) RunStreaming(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, query string, eventCh chan<- StreamEvent) (RunResult, error) {
	// 构建本轮消息：system + 历史 + 当前 user 消息
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+2)
	messages = append(messages, openai.SystemMessage(a.systemPrompt))
	messages = append(messages, history...)
	messages = append(messages, openai.UserMessage(query))

	// roundMessages 记录本轮新增消息（user + assistant + tool，不含 system 和历史）
	roundMessages := []shared.OpenAIMessage{openai.UserMessage(query)}

	var usage openai.CompletionUsage
	var finalResponse string

	for {
		params := openai.ChatCompletionNewParams{
			Model:         a.model,
			Messages:      messages,
			Tools:         a.buildTools(),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
		}

		stream := a.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if reasoningContent := reasoningText(delta); reasoningContent != "" {
					if !emitEvent(ctx, eventCh, StreamEvent{Event: EventReasoning, ReasoningContent: reasoningContent}) {
						return RunResult{}, ctx.Err()
					}
				}
				if delta.Content != "" {
					if !emitEvent(ctx, eventCh, StreamEvent{Event: EventContent, Content: delta.Content}) {
						return RunResult{}, ctx.Err()
					}
				}
			}
		}
		// 显式关闭流以释放底层 HTTP 连接。不能用 defer：stream 在 agent 主循环里每轮重建，
		// defer 会等到函数返回才关闭，反而累积泄漏。Close 不影响后续 stream.Err() 读取。
		stream.Close()
		if err := stream.Err(); err != nil {
			if !emitEvent(ctx, eventCh, StreamEvent{Event: EventError, Content: err.Error()}) {
				return RunResult{}, ctx.Err()
			}
			return RunResult{}, err
		}
		if len(acc.Choices) == 0 {
			break
		}

		usage = acc.Usage
		message := acc.Choices[0].Message
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

			toolResult, err := a.executeTool(ctx, toolCall)
			if err != nil {
				// A failed tool is part of the conversation, not a fatal stream
				// error: return the error text to the model and let it decide
				// what to do next. The UI renders it in the tool result card.
				toolResult = err.Error()
			}
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
			return RunResult{Response: finalResponse}, ctx.Err()
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
// is canceled (for example, the browser tab was closed). Every send in
// RunStreaming goes through this helper so a disconnect cannot leave the agent
// goroutine blocked on a full channel.
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
