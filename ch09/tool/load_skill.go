package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"babyagent/ch09/skill"
)

// LoadSkillTool loads full skill content into the conversation
type LoadSkillTool struct{}

// NewLoadSkillTool creates a new load_skill tool
func NewLoadSkillTool() *LoadSkillTool {
	return &LoadSkillTool{}
}

type LoadSkillParam struct {
	Name string `json:"name"`
}

func (t *LoadSkillTool) ToolName() AgentTool {
	return AgentToolLoadSkill
}

func (t *LoadSkillTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolLoadSkill),
		Description: openai.String("Load the full content and instructions for a specific skill. Use this when you need detailed guidance for a task that matches a skill's purpose."),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The skill ID to load (e.g., 'code-review', 'debug')",
				},
			},
			"required": []string{"name"},
		},
	})
}

func (t *LoadSkillTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	p := LoadSkillParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		return "", err
	}

	if p.Name == "" {
		return "", fmt.Errorf("skill name is required")
	}

	loadedSkill, err := skill.LoadSkill(p.Name)
	if err != nil {
		return "", fmt.Errorf("failed to load skill '%s': %w", p.Name, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Skill: %s\n\n", loadedSkill.Name))
	sb.WriteString("## Main Instruction\n\n")
	sb.WriteString(loadedSkill.MainInstruction)
	sb.WriteString("\n\n## Utility Scripts\n")
	if len(loadedSkill.Scripts) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		for _, filePath := range loadedSkill.Scripts {
			sb.WriteString(fmt.Sprintf("- %s\n", filePath))
		}
	}

	sb.WriteString("\n## References\n")
	if len(loadedSkill.References) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		for _, filePath := range loadedSkill.References {
			sb.WriteString(fmt.Sprintf("- %s\n", filePath))
		}
	}

	sb.WriteString("\nYou can read the script/reference files above when you need their full content.")
	return sb.String(), nil
}
