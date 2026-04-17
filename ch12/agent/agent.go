package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"

	"babyagent/ch12/agent/plan"
	"babyagent/ch12/agent/tool"
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
	t, ok := a.nativeTools[toolName]
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
	Plan     plan.PlanningState
}

// RunStreaming 执行 agent loop，通过 eventCh 流式输出，结束后返回 RunResult
// history 是本会话之前所有 ChatMessage.Rounds 反序列化后的消息列表
func (a *Agent) RunStreaming(ctx context.Context, history []openai.ChatCompletionMessageParamUnion, initialPlan plan.PlanningState, query string, eventCh chan<- StreamEvent) (RunResult, error) {
	conversation := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+1)
	conversation = append(conversation, history...)
	conversation = append(conversation, openai.UserMessage(query))

	roundMessages := []shared.OpenAIMessage{openai.UserMessage(query)}

	var usage openai.CompletionUsage
	var finalResponse string
	planManager := plan.NewManager(initialPlan)
	reminders := &reminderState{}
	toolCallsThisTurn := 0
	loopIndex := 0

	for {
		requestMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(conversation)+2)
		requestMessages = append(requestMessages, openai.SystemMessage(a.systemPrompt))
		if reminder := reminders.Next(planManager.State(), loopIndex, toolCallsThisTurn); reminder != "" {
			requestMessages = append(requestMessages, openai.SystemMessage(reminder))
		}
		requestMessages = append(requestMessages, conversation...)

		runTools := a.buildRunTools(planManager)
		params := openai.ChatCompletionNewParams{
			Model:         a.model,
			Messages:      requestMessages,
			Tools:         buildToolParams(runTools),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
		}

		stream := a.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				deltaRaw := chunk.Choices[0].Delta
				delta := deltaWithReasoning{}
				_ = json.Unmarshal([]byte(deltaRaw.RawJSON()), &delta)

				if delta.ReasoningContent != "" {
					eventCh <- StreamEvent{Event: EventReasoning, ReasoningContent: delta.ReasoningContent}
				}
				if delta.Content != "" {
					eventCh <- StreamEvent{Event: EventContent, Content: delta.Content}
				}
			}
		}
		if err := stream.Err(); err != nil {
			eventCh <- StreamEvent{Event: EventError, Content: err.Error()}
			return RunResult{}, err
		}
		if len(acc.Choices) == 0 {
			break
		}

		usage = acc.Usage
		message := acc.Choices[0].Message
		assistantMsg := message.ToParam()
		conversation = append(conversation, assistantMsg)
		roundMessages = append(roundMessages, assistantMsg)

		// 没有 tool call，结束 loop
		if len(message.ToolCalls) == 0 {
			finalResponse = message.Content
			break
		}

		// 执行 tool calls
		for _, toolCall := range message.ToolCalls {
			toolCallsThisTurn++
			eventCh <- StreamEvent{Event: EventToolCall, ToolCall: toolCall.Function.Name, ToolArguments: toolCall.Function.Arguments}

			toolCtx := tool.WithTodoLoopIndex(ctx, loopIndex)
			toolResult, err := executeTool(toolCtx, runTools, toolCall)
			if err != nil {
				toolResult = err.Error()
				eventCh <- StreamEvent{Event: EventError, Content: toolResult}
			}
			if err == nil && toolCall.Function.Name == string(tool.AgentToolTodoWrite) {
				currentPlan := planManager.State()
				eventCh <- StreamEvent{Event: EventTodoSnap, PlanState: &currentPlan}
			}
			eventCh <- StreamEvent{Event: EventToolResult, ToolCall: toolCall.Function.Name, ToolResult: toolResult}

			toolMsg := openai.ToolMessage(toolResult, toolCall.ID)
			conversation = append(conversation, toolMsg)
			roundMessages = append(roundMessages, toolMsg)
		}
		loopIndex++

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
		Plan:     planManager.State(),
	}, nil
}

type deltaWithReasoning struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

func (a *Agent) buildRunTools(planManager *plan.Manager) map[tool.AgentTool]tool.Tool {
	runTools := make(map[tool.AgentTool]tool.Tool, len(a.nativeTools)+1)
	for name, nativeTool := range a.nativeTools {
		runTools[name] = nativeTool
	}
	runTools[tool.AgentToolTodoWrite] = tool.NewTodoWriteTool(planManager)
	return runTools
}

func buildToolParams(tools map[tool.AgentTool]tool.Tool) []openai.ChatCompletionToolUnionParam {
	params := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, current := range tools {
		params = append(params, current.Info())
	}
	return params
}

func executeTool(ctx context.Context, tools map[tool.AgentTool]tool.Tool, toolCall openai.ChatCompletionMessageToolCallUnion) (string, error) {
	current, ok := tools[toolCall.Function.Name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolCall.Function.Name)
	}
	return current.Execute(ctx, toolCall.Function.Arguments)
}
