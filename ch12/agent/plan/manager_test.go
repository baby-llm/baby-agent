package plan

import "testing"

func TestManagerReplaceSnapshot_AllowsSingleInProgress(t *testing.T) {
	m := NewManager(PlanningState{})

	state, err := m.ReplaceSnapshot([]PlanItem{
		{Content: "Inspect login flow", Status: PlanStatusCompleted},
		{Content: "Patch handler", Status: PlanStatusInProgress},
		{Content: "Run tests", Status: PlanStatusPending},
	}, 3)
	if err != nil {
		t.Fatalf("ReplaceSnapshot() error = %v", err)
	}

	if got, want := len(state.Items), 3; got != want {
		t.Fatalf("len(state.Items) = %d, want %d", got, want)
	}
	if got, want := state.Items[1].Status, PlanStatusInProgress; got != want {
		t.Fatalf("state.Items[1].Status = %q, want %q", got, want)
	}
	if got, want := state.Revision, 1; got != want {
		t.Fatalf("state.Revision = %d, want %d", got, want)
	}
	if got, want := state.LastUpdatedLoop, 3; got != want {
		t.Fatalf("state.LastUpdatedLoop = %d, want %d", got, want)
	}
}

func TestManagerReplaceSnapshot_RejectsMultipleInProgress(t *testing.T) {
	m := NewManager(PlanningState{})

	_, err := m.ReplaceSnapshot([]PlanItem{
		{Content: "Inspect login flow", Status: PlanStatusInProgress},
		{Content: "Patch handler", Status: PlanStatusInProgress},
	}, 1)
	if err == nil {
		t.Fatal("ReplaceSnapshot() error = nil, want validation error")
	}
}

func TestManagerReplaceSnapshot_RejectsBlankContent(t *testing.T) {
	m := NewManager(PlanningState{})

	_, err := m.ReplaceSnapshot([]PlanItem{
		{Content: "", Status: PlanStatusPending},
	}, 1)
	if err == nil {
		t.Fatal("ReplaceSnapshot() error = nil, want validation error")
	}
}

func TestManagerReplaceSnapshot_AllowsClearPlan(t *testing.T) {
	m := NewManager(PlanningState{
		Items: []PlanItem{
			{Content: "Inspect login flow", Status: PlanStatusCompleted},
		},
		Revision:        2,
		LastUpdatedLoop: 4,
	})

	state, err := m.ReplaceSnapshot(nil, 5)
	if err != nil {
		t.Fatalf("ReplaceSnapshot() error = %v", err)
	}
	if got := len(state.Items); got != 0 {
		t.Fatalf("len(state.Items) = %d, want 0", got)
	}
	if got, want := state.Revision, 3; got != want {
		t.Fatalf("state.Revision = %d, want %d", got, want)
	}
	if got, want := state.LastUpdatedLoop, 5; got != want {
		t.Fatalf("state.LastUpdatedLoop = %d, want %d", got, want)
	}
}

func TestManagerReplaceSnapshot_IncrementsRevision(t *testing.T) {
	m := NewManager(PlanningState{})

	first, err := m.ReplaceSnapshot([]PlanItem{
		{Content: "Inspect login flow", Status: PlanStatusPending},
	}, 1)
	if err != nil {
		t.Fatalf("first ReplaceSnapshot() error = %v", err)
	}
	second, err := m.ReplaceSnapshot([]PlanItem{
		{Content: "Inspect login flow", Status: PlanStatusCompleted},
	}, 2)
	if err != nil {
		t.Fatalf("second ReplaceSnapshot() error = %v", err)
	}
	if got, want := first.Revision, 1; got != want {
		t.Fatalf("first.Revision = %d, want %d", got, want)
	}
	if got, want := second.Revision, 2; got != want {
		t.Fatalf("second.Revision = %d, want %d", got, want)
	}
}
