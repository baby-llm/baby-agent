package context

import (
	"context"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"

	"babyagent/shared"
)

func assistantToolCall(id string) shared.OpenAIMessage {
	return shared.OpenAIMessage{OfAssistant: &openai.ChatCompletionAssistantMessageParam{
		ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: id,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      "bash",
					Arguments: `{"command":"ls"}`,
				},
			},
		}},
	}}
}

func TestCountTokens_IncludesToolCalls(t *testing.T) {
	plain := CountTokens(openai.AssistantMessage(""))
	withTool := CountTokens(assistantToolCall("call-1"))

	if withTool <= plain {
		t.Fatalf("tool-call message tokens = %d, plain = %d; tool arguments were not counted", withTool, plain)
	}
}

func TestSummaryPolicyKeepsToolGroupIntact(t *testing.T) {
	summarizer := &fakeSummarizer{
		limit: 1000,
		fn: func(string, []shared.OpenAIMessage) (string, error) {
			return "summary", nil
		},
	}
	policy := NewSummaryPolicy(summarizer, 2, 10, 0.8)
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage("u1"),
		assistantToolCall("call-1"),
		openai.ToolMessage("r1", "call-1"),
		openai.UserMessage("u2"),
		assistantToolCall("call-2"),
		openai.ToolMessage("r2", "call-2"),
		openai.AssistantMessage("a3"),
	})

	result, err := policy.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("len(Messages) = %d, want 4", len(result.Messages))
	}
	if result.Messages[1].Message.OfAssistant == nil || len(result.Messages[1].Message.OfAssistant.ToolCalls) == 0 {
		t.Fatalf("kept tail should start with the assistant tool-call message")
	}
	if result.Messages[2].Message.OfTool == nil {
		t.Fatalf("the tool result should stay with its assistant message")
	}
}

func TestTruncatePolicyPreservesLeadingSummary(t *testing.T) {
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage(summaryMessagePrefix + "old summary"),
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
		openai.UserMessage("u2"),
		openai.AssistantMessage("a2"),
		openai.UserMessage("u3"),
		openai.AssistantMessage("a3"),
		openai.UserMessage("u4"),
		openai.AssistantMessage("a4"),
	})

	result, err := NewTruncatePolicy(2, 0.8).Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != 5 {
		t.Fatalf("len(Messages) = %d, want 5", len(result.Messages))
	}
	if first := contentString(t, result.Messages[0].Message); !strings.HasPrefix(first, summaryMessagePrefix) {
		t.Fatalf("leading summary was dropped: %q", first)
	}
	if got := contentString(t, result.Messages[1].Message); got != "u3" {
		t.Fatalf("first kept message = %q, want u3", got)
	}
}

func TestEngineCountsSystemPrompt(t *testing.T) {
	engine := NewContextEngine(nil)
	engine.Init("you are a helpful assistant", TokenBudget{ContextWindow: 100000})

	if err := engine.CommitTurn(context.Background(), engine.StartTurn(openai.UserMessage("hi"))); err != nil {
		t.Fatalf("CommitTurn() error = %v", err)
	}

	userTokens := CountTokens(openai.UserMessage("hi"))
	if engine.contextTokens <= userTokens {
		t.Fatalf("contextTokens = %d, should include the system prompt (user message: %d)", engine.contextTokens, userTokens)
	}
}
