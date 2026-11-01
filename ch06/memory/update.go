package memory

import (
	"context"
	"log"
	"regexp"
	"strings"

	"github.com/openai/openai-go/v3"

	"babyagent/shared"
)

type MemoryUpdater interface {
	Update(ctx context.Context, oldMemory MemoryContent, newMessages []shared.OpenAIMessage) (MemoryContent, error)
}

type LLMMemoryUpdater struct {
	client    openai.Client
	modelConf shared.ModelConfig
}

func NewLLMMemoryUpdater(modelConf shared.ModelConfig) *LLMMemoryUpdater {
	return &LLMMemoryUpdater{
		client:    shared.NewLLMClient(modelConf),
		modelConf: modelConf,
	}
}

func (u *LLMMemoryUpdater) Update(ctx context.Context, oldMemory MemoryContent, newMessages []shared.OpenAIMessage) (MemoryContent, error) {
	if len(newMessages) == 0 {
		return oldMemory, nil
	}

	var b strings.Builder
	for _, msg := range newMessages {

		contentAny := msg.GetContent().AsAny()
		contentStr, ok := contentAny.(*string)
		if !ok {
			continue
		}

		b.WriteString(shared.GetRoleName(msg))
		b.WriteString(": ")
		b.WriteString(*contentStr)
		b.WriteString("\n")
	}

	prompt := updateMemoryPrompt
	prompt = strings.ReplaceAll(prompt, "{current_memory}", oldMemory.String())
	prompt = strings.ReplaceAll(prompt, "{new_messages}", b.String())

	request := openai.ChatCompletionNewParams{
		Model: u.modelConf.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	}

	resp, err := u.client.Chat.Completions.New(ctx, request)
	if err != nil {
		log.Printf("failed to update memory through llm: %v", err)
		return oldMemory, err
	}
	if len(resp.Choices) == 0 {
		log.Printf("no choices returned, resp: %s", resp.RawJSON())
		return oldMemory, nil
	}

	respContent := resp.Choices[0].Message.Content
	return mergeMemory(oldMemory, respContent), nil
}

// mergeMemory 只更新 LLM 实际返回的层级。模型漏掉某个标签或返回了
// 不符合格式的内容时，保留旧值，避免一次解析失败把整层记忆清空。
// 显式返回空标签（如 <global></global>）表示清空该层。
func mergeMemory(oldMemory MemoryContent, content string) MemoryContent {
	merged := oldMemory
	if value, ok := extractXMLTag(content, "global"); ok {
		merged.GlobalMemory = value
	}
	if value, ok := extractXMLTag(content, "workspace"); ok {
		merged.WorkspaceMemory = value
	}
	return merged
}

// extractXMLTag 使用正则表达式从文本中提取 XML 标签的内容。
// 第二个返回值表示标签是否存在，用于区分“没有返回”和“显式清空”。
func extractXMLTag(content, tagName string) (string, bool) {
	// 匹配 <tagName>...</tagName>，支持多行内容
	pattern := regexp.MustCompile(`<` + regexp.QuoteMeta(tagName) + `>([\s\S]*?)</` + regexp.QuoteMeta(tagName) + `>`)
	matches := pattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

const updateMemoryPrompt = `You are a memory management system for an AI coding assistant. Your task is to analyze conversation messages and update two levels of memory.

## Current Memory
{current_memory}

## New Messages to Process
{new_messages}

## Instructions

Analyze the new messages and update the two memory levels accordingly. Each memory level should be formatted in Markdown.

### Global Memory (User-level)
- User preferences, coding style, frequently used tools/libraries
- Long-term patterns observed across conversations
- User's background, expertise level, recurring needs

### Workspace Memory (Project-level)
- Project structure, architecture, key files
- Build commands, test commands, deployment processes
- Project-specific conventions, tech stack
- Issues encountered and their solutions

## Output Format

Return the updated memories using XML tags. Each memory content should be a valid Markdown string:

<global>
<updated global memory in Markdown format>
</global>

<workspace>
<updated workspace memory in Markdown format>
</workspace>

## Guidelines

1. Use Markdown formatting:
   - Use ## for section headings within a memory level
   - Use - for bullet points
   - Use **bold** for emphasis
   - Use backticks for code, commands, and file names

2. Content principles:
   - Be concise but informative
   - Only update memory levels affected by new messages
   - Preserve existing important information
   - Remove outdated or superseded information

3. If a memory level doesn't need updates, return it unchanged

## Example

Input messages:
- User: I prefer using vim for editing and always run tests with verbose flag
- User: Can you help me set up a Go project?
- Assistant: Created go.mod and main.go files. Used module name "example.com/myapp"

Output:
<global>
## User Preferences
- **Editor**: vim
- **Testing**: Always use verbose flag
</global>

<workspace>
## Project Structure
- go.mod - module: example.com/myapp
- main.go - application entry point
</workspace>

Now process the messages and return the updated memory using XML tags with Markdown-formatted content.
`
