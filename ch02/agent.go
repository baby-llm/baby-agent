package ch02

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/openai/openai-go/v3"

	"babyagent/ch02/tool"
	"babyagent/shared"
)

type Agent struct {
	model    string
	client   openai.Client
	messages []openai.ChatCompletionMessageParamUnion
	tools    map[tool.AgentTool]tool.Tool
}

func NewAgent(modelConf shared.ModelConfig, systemPrompt string, tools []tool.Tool) *Agent {
	a := Agent{
		model:    modelConf.Model,
		client:   shared.NewLLMClient(modelConf),
		tools:    make(map[tool.AgentTool]tool.Tool),
		messages: make([]openai.ChatCompletionMessageParamUnion, 0),
	}
	for _, t := range tools {
		a.tools[t.ToolName()] = t
	}
	a.messages = append(a.messages, openai.SystemMessage(systemPrompt))
	return &a
}

func (a *Agent) execute(ctx context.Context, toolName string, argumentsInJSON string) (string, error) {
	t, ok := a.tools[tool.AgentTool(toolName)]
	if !ok {
		return "", errors.New("tool not found")
	}
	return t.Execute(ctx, argumentsInJSON)
}

// Run 提供对于单次用户请求 query 的 tool loop，返回本轮结果的输出。Run 会保持当前对话历史，不同主题的对话轮次应该初始化多个 Agent 实例运行。
func (a *Agent) Run(ctx context.Context, query string) (string, error) {
	a.messages = append(a.messages, openai.UserMessage(query))

	var result string
	for {
		params := openai.ChatCompletionNewParams{
			Model:    a.model,
			Messages: a.messages,
			Tools:    make([]openai.ChatCompletionToolUnionParam, 0),
		}

		for _, t := range a.tools {
			params.Tools = append(params.Tools, t.Info())
		}

		log.Printf("calling llm model %s...", a.model)
		resp, err := a.client.Chat.Completions.New(ctx, params)
		if err != nil {
			return "", fmt.Errorf("send completion request: %w", err)
		}
		if len(resp.Choices) == 0 {
			log.Printf("no choices returned, resp: %v", resp)
			return "", nil
		}
		message := resp.Choices[0].Message
		// 拼接 assistant message 到整体消息链中
		a.messages = append(a.messages, message.ToParam())

		// tool loop 结束，可以返回结果
		if len(message.ToolCalls) == 0 {
			result = message.Content
			break
		}

		for _, toolCall := range message.ToolCalls {
			toolResult, err := a.execute(ctx, toolCall.Function.Name, toolCall.Function.Arguments)
			if err != nil {
				toolResult = err.Error()
			}
			log.Printf("tool call %s, arguments %s, error: %v", toolCall.Function.Name, toolCall.Function.Arguments, err)
			// 返回 tool message 到整体消息链中
			a.messages = append(a.messages, openai.ToolMessage(toolResult, toolCall.ID))
		}

	}
	return result, nil
}
