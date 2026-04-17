package tool

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"babyagent/ch12/agent/plan"
)

type TodoWriteTool struct {
	manager *plan.Manager
}

func NewTodoWriteTool(manager *plan.Manager) *TodoWriteTool {
	return &TodoWriteTool{manager: manager}
}

type TodoWriteParam struct {
	Items []TodoWriteItem `json:"items"`
}

type TodoWriteItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

func (t *TodoWriteTool) ToolName() AgentTool {
	return AgentToolTodoWrite
}

func (t *TodoWriteTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolTodoWrite),
		Description: openai.String("replace the current todo list for this conversation turn"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"description": "short description of the todo item",
							},
							"status": map[string]any{
								"type":        "string",
								"description": "pending, in_progress, or completed",
								"enum":        []string{"pending", "in_progress", "completed"},
							},
						},
						"required": []string{"content", "status"},
					},
				},
			},
			"required": []string{"items"},
		},
	})
}

type todoLoopIndexContextKey struct{}

func WithTodoLoopIndex(ctx context.Context, loopIndex int) context.Context {
	return context.WithValue(ctx, todoLoopIndexContextKey{}, loopIndex)
}

func todoLoopIndexFromContext(ctx context.Context) int {
	value := ctx.Value(todoLoopIndexContextKey{})
	loopIndex, ok := value.(int)
	if !ok {
		return 0
	}
	return loopIndex
}

func (t *TodoWriteTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	var param TodoWriteParam
	if err := json.Unmarshal([]byte(argumentsInJSON), &param); err != nil {
		return "", err
	}

	items := make([]plan.PlanItem, 0, len(param.Items))
	for _, item := range param.Items {
		items = append(items, plan.PlanItem{
			Content: item.Content,
			Status:  plan.PlanStatus(item.Status),
		})
	}

	state, err := t.manager.ReplaceSnapshot(items, todoLoopIndexFromContext(ctx))
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
