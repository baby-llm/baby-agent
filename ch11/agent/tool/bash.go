package tool

import (
	"context"
	"encoding/json"
	"github.com/openai/openai-go/v3"
	openaiShared "github.com/openai/openai-go/v3/shared"
	"os/exec"
	"runtime"
)

type BashTool struct{}

func NewBashTool() *BashTool            { return &BashTool{} }
func (t *BashTool) ToolName() AgentTool { return AgentToolBash }
func (t *BashTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(openaiShared.FunctionDefinitionParam{Name: string(AgentToolBash), Description: openai.String("execute bash command"), Parameters: openai.FunctionParameters{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "the bash command to execute"}}, "required": []string{"command"}}})
}
func (t *BashTool) Execute(ctx context.Context, input string) (string, error) {
	var p struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(input), &p); err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", p.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", p.Command)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}
