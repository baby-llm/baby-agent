package plan

type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
)

type PlanItem struct {
	Content string     `json:"content"`
	Status  PlanStatus `json:"status"`
}

type PlanningState struct {
	Items           []PlanItem `json:"items"`
	Revision        int        `json:"revision"`
	LastUpdatedLoop int        `json:"last_updated_loop"`
}
