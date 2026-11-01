package ch05

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/openai/openai-go/v3"

	"babyagent/ch05/tool"
	"babyagent/shared"

	ctxengine "babyagent/ch05/context"
)

type Agent struct {
	model         string
	client        openai.Client
	contextEngine *ctxengine.Engine
	nativeTools   map[tool.AgentTool]tool.Tool
	mcpClients    map[string]*McpClient
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool, mcpClients []*McpClient, contextEngine *ctxengine.Engine) *Agent {
	a := Agent{
		model:         modelConf.Model,
		client:        shared.NewLLMClient(modelConf),
		contextEngine: contextEngine,
		nativeTools:   make(map[tool.AgentTool]tool.Tool),
		mcpClients:    make(map[string]*McpClient),
	}

	a.contextEngine.Init(systemPrompt, ctxengine.TokenBudget{ContextWindow: modelConf.ContextWindow})

	for _, t := range tools {
		a.nativeTools[t.ToolName()] = t
	}
	for _, mcpClient := range mcpClients {
		a.mcpClients[mcpClient.Name()] = mcpClient
	}

	return &a
}

func (a *Agent) execute(ctx context.Context, toolName string, argumentsInJSON string) (string, error) {
	// 判断 native tool
	t, ok := a.nativeTools[toolName]
	if ok {
		return t.Execute(ctx, argumentsInJSON)
	}
	// 判断 MCP Tool
	for _, mcpClient := range a.mcpClients {
		for _, t := range mcpClient.GetTools() {
			if t.ToolName() != toolName {
				continue
			}
			return t.Execute(ctx, argumentsInJSON)
		}
	}
	return "", errors.New("tool not found")
}

func (a *Agent) buildTools() []openai.ChatCompletionToolUnionParam {
	tools := make([]openai.ChatCompletionToolUnionParam, 0)
	// 集成 mcp tools
	for _, t := range a.nativeTools {
		tools = append(tools, t.Info())
	}
	// 集成 mcp tools
	for _, mcpClient := range a.mcpClients {
		for _, t := range mcpClient.GetTools() {
			tools = append(tools, t.Info())
		}
	}
	return tools
}

func (a *Agent) ResetSession() {
	a.contextEngine.Reset()
}

// RunStreaming 和 Run 基本逻辑一致，但是使用流式请求，并且通过 channel 实现流式输出
func (a *Agent) RunStreaming(ctx context.Context, query string, viewCh chan MessageVO) error {
	a.contextEngine.SetPolicyEventHook(func(policyName string, running bool, err error) {
		viewCh <- MessageVO{
			Type: MessageTypePolicy,
			Policy: &PolicyVO{
				Name:    policyName,
				Running: running,
				Error:   err,
			},
		}
	})
	defer a.contextEngine.SetPolicyEventHook(nil)

	draft := a.contextEngine.StartTurn(openai.UserMessage(query))
	defer a.contextEngine.AbortTurn(draft)

	// 为本轮次创建新的消息链。草稿消息在 commit 前不会污染上下文。
	messages := a.contextEngine.BuildRequestMessages()
	messages = append(messages, draft.NewMessages...)
	var usage openai.CompletionUsage
	for {
		params := openai.ChatCompletionNewParams{
			Model:    a.model,
			Messages: messages,
			Tools:    a.buildTools(),
		}

		log.Printf("calling llm model %s...", a.model)
		stream := a.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				// reasoning_content 由 baby-llm/openai-go fork 原生解析；
				// reasoning/thinking 是部分厂商的别名字段，仅用于展示。
				if reasoningContent := reasoningText(delta); reasoningContent != "" {
					viewCh <- MessageVO{
						Type:             MessageTypeReasoning,
						ReasoningContent: &reasoningContent,
					}
				}
				if delta.Content != "" {
					content := delta.Content
					viewCh <- MessageVO{
						Type:    MessageTypeContent,
						Content: &content,
					}
				}
			}
		}
		// 显式关闭流以释放底层 HTTP 连接。不能用 defer：stream 在 agent 主循环里每轮重建，
		// defer 会等到函数返回才关闭，反而累积泄漏。Close 不影响后续 stream.Err() 读取。
		stream.Close()
		if err := stream.Err(); err != nil {
			viewCh <- MessageVO{
				Type:    MessageTypeError,
				Content: shared.Ptr(err.Error()),
			}
			return err
		}
		if len(acc.Choices) == 0 {
			log.Printf("no choices returned, resp: %v", acc)
			return nil
		}
		usage = acc.Usage
		message := acc.Choices[0].Message
		// 拼接 assistant message 到整体消息链中
		assistantMsg := message.ToParam()
		messages = append(messages, assistantMsg)
		draft.NewMessages = append(draft.NewMessages, assistantMsg)

		// tool loop 结束，可以返回结果
		if len(message.ToolCalls) == 0 {
			break
		}

		for _, toolCall := range message.ToolCalls {

			viewCh <- MessageVO{
				Type: MessageTypeToolCall,
				ToolCall: &ToolCallVO{
					Name:      toolCall.Function.Name,
					Arguments: toolCall.Function.Arguments,
				},
			}

			toolResult, err := a.execute(ctx, toolCall.Function.Name, toolCall.Function.Arguments)
			if err != nil {
				toolResult = err.Error()

				viewCh <- MessageVO{
					Type:    MessageTypeError,
					Content: &toolResult,
				}

			}
			log.Printf("tool call %s, arguments %s, error: %v", toolCall.Function.Name, toolCall.Function.Arguments, err)
			// 返回 tool message 到整体消息链中
			toolMsg := openai.ToolMessage(toolResult, toolCall.ID)
			messages = append(messages, toolMsg)
			draft.NewMessages = append(draft.NewMessages, toolMsg)
		}

	}

	err := a.contextEngine.CommitTurn(ctx, draft, ctxengine.Usage{PromptTokens: int(usage.TotalTokens)})
	return err
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
