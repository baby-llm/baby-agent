package ch04

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"babyagent/ch04/tool"
	"babyagent/shared"
)

type Agent struct {
	systemPrompt string
	model        string
	client       openai.Client
	messages     []openai.ChatCompletionMessageParamUnion
	nativeTools  map[tool.AgentTool]tool.Tool // agent 框架中原生实现的 tools
	mcpClients   map[string]*McpClient        // 集成 mcp 工具
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool, mcpClients []*McpClient) *Agent {
	a := Agent{
		systemPrompt: systemPrompt,
		model:        modelConf.Model,
		client:       openai.NewClient(option.WithBaseURL(modelConf.BaseURL), option.WithAPIKey(modelConf.ApiKey)),
		nativeTools:  make(map[tool.AgentTool]tool.Tool),
		mcpClients:   make(map[string]*McpClient),
		messages:     make([]openai.ChatCompletionMessageParamUnion, 0),
	}
	for _, t := range tools {
		a.nativeTools[t.ToolName()] = t
	}
	for _, mcpClient := range mcpClients {
		a.mcpClients[mcpClient.Name()] = mcpClient
	}
	a.messages = append(a.messages, openai.SystemMessage(systemPrompt))
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
	a.messages = make([]openai.ChatCompletionMessageParamUnion, 0)
	a.messages = append(a.messages, openai.SystemMessage(a.systemPrompt))
}

// RunStreaming 和 Run 基本逻辑一致，但是使用流式请求，并且通过 channel 实现流式输出
func (a *Agent) RunStreaming(ctx context.Context, query string, viewCh chan MessageVO) error {
	// 为本轮次创建新的消息链。这样如果流式过程中失败或者终止了，不会污染历史上下文。
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(a.messages))
	messages = append(messages, a.messages...)
	messages = append(messages, openai.UserMessage(query))

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
				// 不同厂商会把推理内容放在 reasoning_content、reasoning 或 thinking 字段里。
				delta, err := parseDeltaWithReasoning(deltaRaw.RawJSON())
				if err != nil {
					log.Printf("parse delta failed, raw=%s, err=%v", deltaRaw.RawJSON(), err)
					continue
				}
				if reasoningContent := delta.ReasoningText(); reasoningContent != "" {
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
	a.messages = messages
	return nil
}

type deltaWithReasoning struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
	Thinking         string `json:"thinking"`
}

func parseDeltaWithReasoning(rawJSON string) (deltaWithReasoning, error) {
	delta := deltaWithReasoning{}
	err := json.Unmarshal([]byte(rawJSON), &delta)
	return delta, err
}

func (d deltaWithReasoning) ReasoningText() string {
	switch {
	case d.ReasoningContent != "":
		return d.ReasoningContent
	case d.Reasoning != "":
		return d.Reasoning
	default:
		return d.Thinking
	}
}
