package context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"

	"babyagent/shared"
)

type fakeSummarizer struct {
	limit int
	fn    func(running string, messages []shared.OpenAIMessage) (string, error)
	calls []summaryCall
}

type summaryCall struct {
	running string
	count   int
}

func (f *fakeSummarizer) GetSummaryInputTokenLimit() int {
	return f.limit
}

func (f *fakeSummarizer) Summarize(_ context.Context, runningSummary string, messages []shared.OpenAIMessage) (string, error) {
	f.calls = append(f.calls, summaryCall{running: runningSummary, count: len(messages)})
	return f.fn(runningSummary, messages)
}

type fakeStorage struct {
	store map[string]string
	fail  bool
}

func (f *fakeStorage) Load(_ context.Context, key string) (string, error) {
	return f.store[key], nil
}

func (f *fakeStorage) Store(_ context.Context, key string, value string) error {
	if f.fail {
		return errors.New("store failed")
	}
	f.store[key] = value
	return nil
}

func buildEngine(msgs []shared.OpenAIMessage) *Engine {
	wrapped := make([]messageWrap, 0, len(msgs))
	total := 0
	for i := range msgs {
		tokens := CountTokens(msgs[i])
		wrapped = append(wrapped, messageWrap{Message: msgs[i], Tokens: tokens})
		total += tokens
	}
	return &Engine{
		messages:      wrapped,
		contextTokens: total,
		contextWindow: 100,
	}
}

func sumTokens(messages []messageWrap) int {
	total := 0
	for i := range messages {
		total += messages[i].Tokens
	}
	return total
}

func contentString(t *testing.T, msg shared.OpenAIMessage) string {
	t.Helper()
	v := msg.GetContent().AsAny()
	s, ok := v.(*string)
	if !ok {
		t.Fatalf("message content is not string: %T", v)
	}
	return *s
}

func TestTruncatePolicyApply_NoChangeWhenWithinKeepRecent(t *testing.T) {
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
	})
	p := NewTruncatePolicy(2, 0.8)

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != len(engine.messages) {
		t.Fatalf("unexpected message length: got %d want %d", len(result.Messages), len(engine.messages))
	}
	if result.ContextTokens != engine.contextTokens {
		t.Fatalf("unexpected context tokens: got %d want %d", result.ContextTokens, engine.contextTokens)
	}
}

func TestTruncatePolicyApply_TruncateBeforeLatestUserBoundary(t *testing.T) {
	engine := buildEngine([]shared.OpenAIMessage{
		openai.AssistantMessage("a0"),
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
		openai.UserMessage("u2"),
		openai.AssistantMessage("a2"),
	})
	p := NewTruncatePolicy(2, 0.8)

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("unexpected message length: got %d want 4", len(result.Messages))
	}
	if result.Messages[0].Message.OfUser == nil {
		t.Fatalf("first message after truncate should be user")
	}
	if got := contentString(t, result.Messages[0].Message); got != "u1" {
		t.Fatalf("unexpected first user content: got %q want %q", got, "u1")
	}
}

func TestTruncatePolicyShouldApply(t *testing.T) {
	engine := &Engine{contextTokens: 81, contextWindow: 100}
	p := NewTruncatePolicy(2, 0.8)
	if !p.ShouldApply(context.Background(), engine) {
		t.Fatalf("ShouldApply() = false, want true")
	}
}

func TestSummaryPolicyApply_GeneratesBatchedSummary(t *testing.T) {
	s := &fakeSummarizer{
		limit: 1000,
		fn: func(running string, messages []shared.OpenAIMessage) (string, error) {
			return running + fmt.Sprintf("[%d]", len(messages)), nil
		},
	}
	p := NewSummaryPolicy(s, 2, 2, 0.8)
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
		openai.UserMessage("u2"),
		openai.AssistantMessage("a2"),
		openai.UserMessage("u3"),
	})

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(s.calls) != 2 {
		t.Fatalf("Summarize() calls = %d, want 2", len(s.calls))
	}
	if s.calls[0].count != 2 || s.calls[1].count != 1 {
		t.Fatalf("unexpected batch sizes: %+v", s.calls)
	}
	if s.calls[1].running != "[2]" {
		t.Fatalf("unexpected running summary: got %q want %q", s.calls[1].running, "[2]")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("unexpected message length: got %d want 3", len(result.Messages))
	}
	if result.Messages[0].Message.OfUser == nil {
		t.Fatalf("summary message should be user role")
	}
	if got := contentString(t, result.Messages[0].Message); !strings.HasPrefix(got, summaryMessagePrefix) || !strings.Contains(got, "[2][1]") {
		t.Fatalf("unexpected summary content: got %q", got)
	}
}

func TestSummaryPolicyApply_EmptySummaryFallsBackToOriginal(t *testing.T) {
	s := &fakeSummarizer{
		limit: 1000,
		fn: func(_ string, _ []shared.OpenAIMessage) (string, error) {
			return "", nil
		},
	}
	p := NewSummaryPolicy(s, 1, 10, 0.8)
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
	})

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != len(engine.messages) {
		t.Fatalf("unexpected message length: got %d want %d", len(result.Messages), len(engine.messages))
	}
	if result.ContextTokens != engine.contextTokens {
		t.Fatalf("unexpected context tokens: got %d want %d", result.ContextTokens, engine.contextTokens)
	}
}

func TestSummaryPolicyShouldApply(t *testing.T) {
	engine := &Engine{contextTokens: 90, contextWindow: 100}
	s := &fakeSummarizer{limit: 1000, fn: func(r string, _ []shared.OpenAIMessage) (string, error) { return r, nil }}
	p := NewSummaryPolicy(s, 1, 10, 0.8)
	if !p.ShouldApply(context.Background(), engine) {
		t.Fatalf("ShouldApply() = false, want true")
	}
}

func TestOffloadPolicyApply_OffloadsLongToolMessagesOnly(t *testing.T) {
	st := &fakeStorage{store: map[string]string{}}
	p := NewOffloadPolicy(st, 0.8, 1, 10)

	long1 := "1234567890ABCDEFGHIJ"
	long2 := "abcdefghijklmnopqrst"
	engine := buildEngine([]shared.OpenAIMessage{
		openai.ToolMessage(long1, "call-1"),
		openai.UserMessage("u1"),
		openai.ToolMessage("short", "call-2"),
		openai.ToolMessage(long2, "call-3"),
		openai.AssistantMessage("a1"),
	})

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(st.store) != 2 {
		t.Fatalf("stored entries = %d, want 2", len(st.store))
	}

	firstOffloaded := contentString(t, result.Messages[0].Message)
	if !strings.HasPrefix(firstOffloaded, "1234567890") || !strings.Contains(firstOffloaded, "load_storage(key=") {
		t.Fatalf("unexpected offloaded content: %q", firstOffloaded)
	}

	thirdUnchanged := contentString(t, result.Messages[2].Message)
	if thirdUnchanged != "short" {
		t.Fatalf("short tool message should remain unchanged, got %q", thirdUnchanged)
	}

	if got, want := result.ContextTokens, sumTokens(result.Messages); got != want {
		t.Fatalf("unexpected context tokens: got %d want %d", got, want)
	}
}

func TestOffloadPolicyApply_StoreFailureKeepsOriginalContent(t *testing.T) {
	st := &fakeStorage{store: map[string]string{}, fail: true}
	p := NewOffloadPolicy(st, 0.8, 0, 5)

	original := "1234567890"
	engine := buildEngine([]shared.OpenAIMessage{
		openai.ToolMessage(original, "call-1"),
	})

	result, err := p.Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got := contentString(t, result.Messages[0].Message)
	if got != original {
		t.Fatalf("content changed on store failure: got %q want %q", got, original)
	}
}

func TestOffloadPolicyShouldApply(t *testing.T) {
	engine := &Engine{contextTokens: 85, contextWindow: 100}
	p := NewOffloadPolicy(&fakeStorage{store: map[string]string{}}, 0.8, 1, 10)
	if !p.ShouldApply(context.Background(), engine) {
		t.Fatalf("ShouldApply() = false, want true")
	}
}

func TestSummaryPolicyApply_KeepZeroDoesNotPanic(t *testing.T) {
	s := &fakeSummarizer{
		limit: 1000,
		fn: func(_ string, _ []shared.OpenAIMessage) (string, error) {
			return "summary", nil
		},
	}
	engine := buildEngine([]shared.OpenAIMessage{
		openai.UserMessage("u1"),
		openai.AssistantMessage("a1"),
	})

	result, err := NewSummaryPolicy(s, 0, 10, 0.8).Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(result.Messages))
	}
}

func TestOffloadPolicyApply_DoesNotSplitRunes(t *testing.T) {
	st := &fakeStorage{store: map[string]string{}}
	content := strings.Repeat("你", 20)
	engine := buildEngine([]shared.OpenAIMessage{
		openai.ToolMessage(content, "call-1"),
	})

	result, err := NewOffloadPolicy(st, 0.8, 0, 5).Apply(context.Background(), engine)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got := contentString(t, result.Messages[0].Message)
	if !strings.HasPrefix(got, strings.Repeat("你", 5)) {
		t.Fatalf("preview = %q, want 5 whole runes", got)
	}
}

type failingPolicy struct{}

func (failingPolicy) Name() string { return "fail" }
func (failingPolicy) ShouldApply(context.Context, *Engine) bool {
	return true
}
func (failingPolicy) Apply(context.Context, *Engine) (PolicyResult, error) {
	return PolicyResult{}, errors.New("boom")
}

func TestEngineCommitTurnRollsBackOnPolicyFailure(t *testing.T) {
	engine := NewContextEngine([]Policy{failingPolicy{}})
	engine.Init("system", TokenBudget{ContextWindow: 1000})
	before := len(engine.messages)

	err := engine.CommitTurn(context.Background(), engine.StartTurn(openai.UserMessage("hi")))
	if err == nil {
		t.Fatal("CommitTurn() error = nil, want policy failure")
	}
	if len(engine.messages) != before {
		t.Fatalf("len(messages) = %d, want rollback to %d", len(engine.messages), before)
	}
}
