package context

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/openai/openai-go/v3"
)

type messageWrap struct {
	Message openai.ChatCompletionMessageParamUnion
	Tokens  int
}

type Engine struct {
	systemPromptTemplate string
	messages             []messageWrap
	policies             []Policy
	onPolicyEvent        func(policyName string, running bool, err error)
	contextTokens        int
	contextWindow        int
}

type TokenBudget struct {
	ContextWindow int
}

type Usage struct {
	PromptTokens int
}

type TurnDraft struct {
	NewMessages []openai.ChatCompletionMessageParamUnion
}

func NewContextEngine(policies []Policy) *Engine {
	return &Engine{
		policies:      policies,
		messages:      make([]messageWrap, 0),
		contextWindow: 200000,
	}
}

func (c *Engine) Init(systemPrompt string, budget TokenBudget) {
	c.systemPromptTemplate = systemPrompt
	if budget.ContextWindow > 0 {
		c.contextWindow = budget.ContextWindow
	}
}

func (c *Engine) BuildRequestMessages() []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(c.messages)+1)
	if c.systemPromptTemplate != "" {
		result = append(result, openai.SystemMessage(c.BuildSystemPrompt()))
	}
	for i := range c.messages {
		result = append(result, c.messages[i].Message)
	}
	return result
}

func (c *Engine) StartTurn(userMsg openai.ChatCompletionMessageParamUnion) TurnDraft {
	return TurnDraft{
		NewMessages: []openai.ChatCompletionMessageParamUnion{userMsg},
	}
}

func (c *Engine) CommitTurn(ctx context.Context, draft TurnDraft, usage Usage) error {
	for i := range draft.NewMessages {
		msg := draft.NewMessages[i]
		c.messages = append(c.messages, messageWrap{Message: msg, Tokens: CountTokens(msg)})
	}

	c.recountTokens()

	if err := c.applyPolicies(ctx); err != nil {
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
		c.recountTokens()
	}
	return nil
}

func (c *Engine) SetPolicyEventHook(hook func(policyName string, running bool, err error)) {
	c.onPolicyEvent = hook
}

func (c *Engine) BuildSystemPrompt() string {
	replaceMap := make(map[string]string)
	replaceMap["{runtime}"] = runtime.GOOS
	cwd, _ := os.Getwd()
	replaceMap["{workspace_path}"] = cwd

	// todo 集成 memory

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
