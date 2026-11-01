package agent

import (
	"context"
	"testing"
)

func TestRun_ExecutesToolAndFeedsResultToNextCompletion(t *testing.T) {
	client := &MockClient{Responses: []Completion{{ToolCalls: []ToolCall{{ID: "call-1", Name: "weather", Arguments: `{"city":"Shanghai"}`}}, Usage: Usage{PromptTokens: 10, CompletionTokens: 3}}, {Content: "Shanghai is sunny.", Usage: Usage{PromptTokens: 15, CompletionTokens: 5}}}}
	tool := &MockTool{ToolName: "weather", Output: "sunny, 28C"}
	result, err := New(client, []Tool{tool}).Run(context.Background(), "What is the weather?")
	if err != nil {
		t.Fatal(err)
	}
	if result.Response != "Shanghai is sunny." || result.LoopDepth != 2 || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(client.Requests) != 2 || len(client.Requests[1]) != 4 {
		t.Fatalf("requests = %#v", client.Requests)
	}
	if got := client.Requests[1][3].Content; got != "sunny, 28C" {
		t.Fatalf("tool result = %q", got)
	}
	if result.Usage.PromptTokens != 25 || result.Usage.CompletionTokens != 8 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}
func TestRun_FailsWhenLoopLimitExceeded(t *testing.T) {
	client := &MockClient{Responses: []Completion{{ToolCalls: []ToolCall{{ID: "1", Name: "x"}}}, {ToolCalls: []ToolCall{{ID: "2", Name: "x"}}}}}
	_, err := New(client, []Tool{&MockTool{ToolName: "x"}}).WithMaxLoops(2).Run(context.Background(), "loop")
	if err == nil {
		t.Fatal("expected loop-limit error")
	}
}
