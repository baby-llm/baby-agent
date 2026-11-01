package context

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/openai/openai-go/v3"

	"babyagent/ch06/memory"
	"babyagent/shared"
)

type messageWrap struct {
	Message shared.OpenAIMessage
	Tokens  int
}

type Engine struct {
	systemPromptTemplate string
	messages             []messageWrap
	policies             []Policy
	onPolicyEvent        func(policyName string, running bool, err error)
	onMemoryEvent        func(running bool, err error)
	contextTokens        int
	contextWindow        int

	memory memory.Memory
}

type TokenBudget struct {
	ContextWindow int
}

type Usage struct {
	PromptTokens int
}

type TurnDraft struct {
	NewMessages []shared.OpenAIMessage
}

func NewContextEngine(memory memory.Memory, policies []Policy) *Engine {
	return &Engine{
		policies:      policies,
		messages:      make([]messageWrap, 0),
		contextWindow: 200000,
		memory:        memory,
	}
}

func (c *Engine) Init(systemPrompt string, budget TokenBudget) {
	c.systemPromptTemplate = systemPrompt
	if budget.ContextWindow > 0 {
		c.contextWindow = budget.ContextWindow
	}
}

func (c *Engine) BuildRequestMessages() []shared.OpenAIMessage {
	result := make([]shared.OpenAIMessage, 0, len(c.messages)+1)
	if c.systemPromptTemplate != "" {
		result = append(result, openai.SystemMessage(c.BuildSystemPrompt()))
	}
	for i := range c.messages {
		result = append(result, c.messages[i].Message)
	}
	return result
}

func (c *Engine) StartTurn(userMsg shared.OpenAIMessage) TurnDraft {
	return TurnDraft{
		NewMessages: []shared.OpenAIMessage{userMsg},
	}
}

func (c *Engine) CommitTurn(ctx context.Context, draft TurnDraft, usage Usage) error {
	// 根据情况压缩上下文
	for i := range draft.NewMessages {
		msg := draft.NewMessages[i]
		c.messages = append(c.messages, messageWrap{Message: msg, Tokens: CountTokens(msg)})
	}
	c.recountTokens()
	if err := c.applyPolicies(ctx); err != nil {
		return err
	}
	// 更新记忆
	if c.onMemoryEvent != nil {
		c.onMemoryEvent(true, nil)
	}
	err := c.memory.Update(ctx, draft.NewMessages)
	if c.onMemoryEvent != nil {
		c.onMemoryEvent(false, err)
	}
	if err != nil {
		return err
	}
	return nil
}

func (c *Engine) AbortTurn(_ TurnDraft) {
	// no-op: draft is only in-memory and never committed unless CommitTurn is called.
}

func (c *Engine) GetContextUsage() float64 {
	if c.contextWindow <= 0 {
		return 0
	}
	return float64(c.contextTokens) / float64(c.contextWindow)
}

func (c *Engine) recountTokens() {
	totalTokens := 0
	// The system prompt is sent on every request, so it belongs in the usage
	// estimate; otherwise policies fire later than the real window allows.
	if c.systemPromptTemplate != "" {
		totalTokens += CountTokens(openai.SystemMessage(c.BuildSystemPrompt()))
	}
	for i := range c.messages {
		totalTokens += c.messages[i].Tokens
	}
	c.contextTokens = totalTokens
}

func (c *Engine) applyPolicies(ctx context.Context) error {
	for _, policy := range c.policies {
		if !policy.ShouldApply(ctx, c) {
			continue
		}
		if c.onPolicyEvent != nil {
			c.onPolicyEvent(policy.Name(), true, nil)
		}
		result, err := policy.Apply(ctx, c)
		if c.onPolicyEvent != nil {
			c.onPolicyEvent(policy.Name(), false, err)
		}
		if err != nil {
			return fmt.Errorf("apply policy %s: %w", policy.Name(), err)
		}
		c.messages = result.Messages
		// Policies compute the new count from the current one; use their result
		// instead of silently discarding it and recounting.
		c.contextTokens = result.ContextTokens
	}
	return nil
}

func (c *Engine) SetPolicyEventHook(hook func(policyName string, running bool, err error)) {
	c.onPolicyEvent = hook
}

func (c *Engine) SetMemoryEventHook(hook func(running bool, err error)) {
	c.onMemoryEvent = hook
}

func (c *Engine) BuildSystemPrompt() string {
	replaceMap := make(map[string]string)
	replaceMap["{runtime}"] = runtime.GOOS
	replaceMap["{workspace_path}"] = shared.GetWorkspaceDir()

	if c.memory != nil {
		replaceMap["{memory}"] = c.memory.String()
	} else {
		replaceMap["{memory}"] = ""
	}

	prompt := c.systemPromptTemplate
	for k, v := range replaceMap {
		prompt = strings.ReplaceAll(prompt, k, v)
	}
	return prompt
}

// Reset 清空所有消息（保留 system prompt）
func (c *Engine) Reset() {
	c.messages = make([]messageWrap, 0)
	c.contextTokens = 0
}
