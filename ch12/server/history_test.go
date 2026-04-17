package server

import (
	"encoding/json"
	"testing"

	"babyagent/ch12/agent/plan"
)

func TestFindLatestPlanState_FollowsAncestorChain(t *testing.T) {
	rootPlan := mustPlanJSON(t, plan.PlanningState{
		Items: []plan.PlanItem{
			{Content: "Inspect login flow", Status: plan.PlanStatusCompleted},
		},
		Revision:        1,
		LastUpdatedLoop: 1,
	})

	all := []ChatMessage{
		{MessageID: "root", ParentMessageID: "", PlanState: rootPlan},
		{MessageID: "child", ParentMessageID: "root"},
		{MessageID: "leaf", ParentMessageID: "child"},
	}

	got := findLatestPlanState(all, "leaf")
	if got.Revision != 1 {
		t.Fatalf("got.Revision = %d, want 1", got.Revision)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(got.Items) = %d, want 1", len(got.Items))
	}
}

func TestFindLatestPlanState_IgnoresSiblingBranchSnapshots(t *testing.T) {
	rootPlan := mustPlanJSON(t, plan.PlanningState{
		Items: []plan.PlanItem{
			{Content: "Inspect login flow", Status: plan.PlanStatusPending},
		},
		Revision:        1,
		LastUpdatedLoop: 0,
	})
	siblingPlan := mustPlanJSON(t, plan.PlanningState{
		Items: []plan.PlanItem{
			{Content: "Patch sibling branch", Status: plan.PlanStatusInProgress},
		},
		Revision:        2,
		LastUpdatedLoop: 3,
	})

	all := []ChatMessage{
		{MessageID: "root", ParentMessageID: "", PlanState: rootPlan},
		{MessageID: "left", ParentMessageID: "root"},
		{MessageID: "left-leaf", ParentMessageID: "left"},
		{MessageID: "right", ParentMessageID: "root", PlanState: siblingPlan},
	}

	got := findLatestPlanState(all, "left-leaf")
	if len(got.Items) != 1 {
		t.Fatalf("len(got.Items) = %d, want 1", len(got.Items))
	}
	if got.Items[0].Content != "Inspect login flow" {
		t.Fatalf("got.Items[0].Content = %q, want %q", got.Items[0].Content, "Inspect login flow")
	}
}

func TestFindLatestPlanState_ReturnsEmptyWhenNoSnapshot(t *testing.T) {
	all := []ChatMessage{
		{MessageID: "root", ParentMessageID: ""},
		{MessageID: "leaf", ParentMessageID: "root"},
	}

	got := findLatestPlanState(all, "leaf")
	if got.Revision != 0 {
		t.Fatalf("got.Revision = %d, want 0", got.Revision)
	}
	if len(got.Items) != 0 {
		t.Fatalf("len(got.Items) = %d, want 0", len(got.Items))
	}
}

func mustPlanJSON(t *testing.T, state plan.PlanningState) string {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(data)
}
