package ch05

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"babyagent/ch05/tool"
	"babyagent/shared"

	ctxengine "babyagent/ch05/context"
)

type Agent struct {
	model         string
	client        openai.Client
	contextEngine *ctxengine.ContextEngine
	nativeTools   map[tool.AgentTool]tool.Tool
	mcpClients    map[string]*McpClient
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool, mcpClients []*McpClient, contextEngine *ctxengine.ContextEngine) *Agent {
	a := Agent{
		model:         modelConf.Model,
		client:        openai.NewClient(option.WithBaseURL(modelConf.BaseURL), option.WithAPIKey(modelConf.ApiKey)),
		contextEngine: contextEngine,
		nativeTools:   make(map[tool.AgentTool]tool.Tool),
		mcpClients:    make(map[string]*McpClient),
	}

	// 设置 system prompt（ContextEngine 会自动处理占位符）
	a.contextEngine.SetSystemPrompt(systemPrompt)
	a.contextEngine.SetContextWindow(modelConf.ContextWindow)

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
	// 设置策略事件 channel，用于转发策略执行状态到 TUI
	// 必须在 AddMessages 之前设置，因为 AddMessages 会触发策略执行
	strategyCh := make(chan ctxengine.StrategyEvent, 10)
	defer close(strategyCh)
	a.contextEngine.SetStrategyEventChan(strategyCh)

	// 启动 goroutine 转发策略事件到 viewCh
	go func() {
		for event := range strategyCh {
			switch event.Type {
			case ctxengine.StrategyEventStart:
				viewCh <- MessageVO{
					Type: MessageTypeStrategy,
					Strategy: &StrategyVO{
						Name:    event.Name,
						Running: true,
					},
				}
			case ctxengine.StrategyEventComplete:
				viewCh <- MessageVO{
					Type: MessageTypeStrategy,
					Strategy: &StrategyVO{
						Name:    event.Name,
						Running: false,
					},
				}
			}
		}
	}()

	// 为本轮次创建新的消息链。这样如果流式过程中失败或者终止了，不会污染历史上下文。
	messages := a.contextEngine.GetAllMessages()
	messages = append(messages, openai.UserMessage(query))

	// 记录开始时的消息数量（不含 system prompt），用于后续提交新消息
	baseMessageCount := len(messages)

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
				deltaRaw := chunk.Choices[0].Delta
				// 推理模型会返回 reasoning_content（有些模型使用 reasoning 字段）
				delta := deltaWithReasoning{}
				_ = json.Unmarshal([]byte(deltaRaw.RawJSON()), &delta)
				if reasoningContent := delta.ReasoningContent; reasoningContent != "" {
					viewCh <- MessageVO{
						Type:             MessageTypeReasoning,
						ReasoningContent: &reasoningContent,
					}
				}
				if delta.Content != "" {
					viewCh <- MessageVO{
						Type:    MessageTypeContent,
						Content: &chunk.Choices[0].Delta.Content,
					}
				}
			}
		}
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
		message := acc.Choices[0].Message
		// 拼接 assistant message 到整体消息链中
		messages = append(messages, message.ToParam())

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
			messages = append(messages, openai.ToolMessage(toolResult, toolCall.ID))
		}

	}

	// 轮次正常结束，agent 保存当前最新的消息链状态
	// 批量添加本轮新增的消息（从 baseMessageCount 开始）
	newMessages := messages[baseMessageCount:]
	a.contextEngine.AddMessages(ctx, newMessages)
	return nil
}

type deltaWithReasoning struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}
