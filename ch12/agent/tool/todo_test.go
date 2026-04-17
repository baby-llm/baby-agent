package tool

import (
	"context"
	"encoding/json"
	"testing"

	"babyagent/ch12/agent/plan"
)

func TestTodoWriteTool_ReplaceSnapshot(t *testing.T) {
	m := plan.NewManager(plan.PlanningState{})
	tool := NewTodoWriteTool(m)

	result, err := tool.Execute(context.Background(), `{"items":[{"content":"Inspect login flow","status":"completed"},{"content":"Patch handler","status":"in_progress"}]}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	state := m.State()
	if got, want := len(state.Items), 2; got != want {
		t.Fatalf("len(state.Items) = %d, want %d", got, want)
	}
	if got, want := state.Items[1].Status, plan.PlanStatusInProgress; got != want {
		t.Fatalf("state.Items[1].Status = %q, want %q", got, want)
	}

	var returned plan.PlanningState
	if err := json.Unmarshal([]byte(result), &returned); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got, want := returned.Revision, 1; got != want {
		t.Fatalf("returned.Revision = %d, want %d", got, want)
	}
}

func TestTodoWriteTool_ReturnsValidationError(t *testing.T) {
	m := plan.NewManager(plan.PlanningState{})
	tool := NewTodoWriteTool(m)

	_, err := tool.Execute(context.Background(), `{"items":[{"content":"","status":"pending"}]}`)
	if err == nil {
		t.Fatal("Execute() error = nil, want validation error")
	}
}

func TestTodoWriteTool_AllowsClearPlan(t *testing.T) {
	m := plan.NewManager(plan.PlanningState{})
	tool := NewTodoWriteTool(m)

	if _, err := tool.Execute(context.Background(), `{"items":[{"content":"Inspect login flow","status":"pending"}]}`); err != nil {
		t.Fatalf("seed Execute() error = %v", err)
	}
	if _, err := tool.Execute(context.Background(), `{"items":[]}`); err != nil {
		t.Fatalf("clear Execute() error = %v", err)
	}

	if got := len(m.State().Items); got != 0 {
		t.Fatalf("len(m.State().Items) = %d, want 0", got)
	}
}
