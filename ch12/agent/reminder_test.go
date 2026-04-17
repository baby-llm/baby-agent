package agent

import (
	"testing"

	"babyagent/ch12/agent/plan"
)

func TestReminderState_ShouldRemindWhenToolLoopStartsWithoutPlan(t *testing.T) {
	r := &reminderState{}

	got := r.Next(plan.PlanningState{}, 1, 1)
	if got == "" {
		t.Fatal("Next() = empty string, want missing-plan reminder")
	}
}

func TestReminderState_ShouldRemindWhenPlanIsStale(t *testing.T) {
	r := &reminderState{}

	got := r.Next(plan.PlanningState{
		Items: []plan.PlanItem{
			{Content: "Inspect login flow", Status: plan.PlanStatusCompleted},
			{Content: "Patch handler", Status: plan.PlanStatusInProgress},
		},
		Revision:        1,
		LastUpdatedLoop: 0,
	}, 2, 2)
	if got == "" {
		t.Fatal("Next() = empty string, want stale-plan reminder")
	}
}

func TestReminderState_DoesNotRepeatReminderInSameTurn(t *testing.T) {
	r := &reminderState{}

	first := r.Next(plan.PlanningState{}, 1, 1)
	second := r.Next(plan.PlanningState{}, 2, 2)
	if first == "" {
		t.Fatal("first Next() = empty string, want reminder")
	}
	if second != "" {
		t.Fatalf("second Next() = %q, want empty string", second)
	}
}
