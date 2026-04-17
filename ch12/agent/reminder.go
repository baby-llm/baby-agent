package agent

import (
	"strings"

	"babyagent/ch12/agent/plan"
)

const (
	noTodoReminderText = "Reminder: You are in a multi-step tool workflow. Call todo_write now with a short todo list. Keep it concise and have at most one item in_progress."
	staleTodoReminder  = "Reminder: Your todo list looks stale. Update it with todo_write so completed work is marked done and the current active step is in_progress."
)

type reminderState struct {
	noTodoReminderSent bool
	staleReminderSent  bool
}

func (r *reminderState) Next(state plan.PlanningState, loopIndex int, toolCallsThisTurn int) string {
	if toolCallsThisTurn == 0 {
		return ""
	}

	if len(state.Items) == 0 {
		if r.noTodoReminderSent {
			return ""
		}
		r.noTodoReminderSent = true
		return noTodoReminderText
	}

	if planComplete(state) {
		return ""
	}

	if r.staleReminderSent {
		return ""
	}

	if loopIndex-state.LastUpdatedLoop < 2 {
		return ""
	}

	r.staleReminderSent = true
	return staleTodoReminder
}

func planComplete(state plan.PlanningState) bool {
	if len(state.Items) == 0 {
		return false
	}
	for _, item := range state.Items {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		if item.Status != plan.PlanStatusCompleted {
			return false
		}
	}
	return true
}
