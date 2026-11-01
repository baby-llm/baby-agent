package agent

import (
	"context"
	"fmt"
)

const SystemPrompt = "You are BabyAgent. Use tools when needed and answer directly when done."

type Agent struct {
	client       Client
	tools        map[string]Tool
	maxLoops     int
	systemPrompt string
}

func New(client Client, tools []Tool) *Agent {
	a := &Agent{client: client, tools: make(map[string]Tool), maxLoops: 8, systemPrompt: SystemPrompt}
	for _, tool := range tools {
		a.tools[tool.Name()] = tool
	}
	return a
}
func (a *Agent) WithMaxLoops(n int) *Agent { a.maxLoops = n; return a }

func (a *Agent) Run(ctx context.Context, query string) (Result, error) {
	messages := []Message{{Role: "system", Content: a.systemPrompt}, {Role: "user", Content: query}}
	result := Result{Messages: append([]Message(nil), messages...)}
	for loop := 1; loop <= a.maxLoops; loop++ {
		completion, err := a.client.Complete(ctx, messages)
		if err != nil {
			return result, err
		}
		result.LoopDepth = loop
		result.Usage.PromptTokens += completion.Usage.PromptTokens
		result.Usage.CompletionTokens += completion.Usage.CompletionTokens
		assistant := Message{Role: "assistant", Content: completion.Content, ToolCalls: completion.ToolCalls}
		messages = append(messages, assistant)
		result.Messages = append(result.Messages, assistant)
		if len(completion.ToolCalls) == 0 {
			result.Response = completion.Content
			return result, nil
		}
		for _, call := range completion.ToolCalls {
			tool, ok := a.tools[call.Name]
			if !ok {
				return result, fmt.Errorf("tool not found: %s", call.Name)
			}
			output, err := tool.Execute(ctx, call.Arguments)
			if err != nil {
				return result, fmt.Errorf("execute tool %s: %w", call.Name, err)
			}
			result.ToolCalls = append(result.ToolCalls, call.Name)
			toolMessage := Message{Role: "tool", Content: output, ToolCallID: call.ID}
			messages = append(messages, toolMessage)
			result.Messages = append(result.Messages, toolMessage)
		}
	}
	return result, fmt.Errorf("agent exceeded max loop depth %d", a.maxLoops)
}
