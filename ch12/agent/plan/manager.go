package plan

import (
	"fmt"
	"strings"
	"sync"
)

const maxPlanItems = 8

type Manager struct {
	mu    sync.RWMutex
	state PlanningState
}

func NewManager(initial PlanningState) *Manager {
	return &Manager{state: cloneState(initial)}
}

func (m *Manager) State() PlanningState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneState(m.state)
}

func (m *Manager) ReplaceSnapshot(items []PlanItem, loopIndex int) (PlanningState, error) {
	normalized, err := normalizeItems(items)
	if err != nil {
		return PlanningState{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = PlanningState{
		Items:           normalized,
		Revision:        m.state.Revision + 1,
		LastUpdatedLoop: loopIndex,
	}
	return cloneState(m.state), nil
}

func normalizeItems(items []PlanItem) ([]PlanItem, error) {
	if len(items) > maxPlanItems {
		return nil, fmt.Errorf("todo list must contain at most %d items", maxPlanItems)
	}

	normalized := make([]PlanItem, 0, len(items))
	inProgress := 0
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			return nil, fmt.Errorf("todo item content cannot be empty")
		}

		status := PlanStatus(strings.TrimSpace(string(item.Status)))
		switch status {
		case PlanStatusPending, PlanStatusInProgress, PlanStatusCompleted:
		default:
			return nil, fmt.Errorf("invalid todo status: %s", item.Status)
		}

		if status == PlanStatusInProgress {
			inProgress++
			if inProgress > 1 {
				return nil, fmt.Errorf("todo list can have at most one in_progress item")
			}
		}

		normalized = append(normalized, PlanItem{
			Content: content,
			Status:  status,
		})
	}

	return normalized, nil
}

func cloneState(state PlanningState) PlanningState {
	items := make([]PlanItem, len(state.Items))
	copy(items, state.Items)
	return PlanningState{
		Items:           items,
		Revision:        state.Revision,
		LastUpdatedLoop: state.LastUpdatedLoop,
	}
}
