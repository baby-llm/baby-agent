package eval

import (
	"babyagent/ch12/agent"
	"encoding/json"
	"os"
)

// Dataset is version-controlled test input. Keep production secrets and user data out of it.
type Dataset struct {
	Name  string `json:"name"`
	Cases []Case `json:"cases"`
}
type Case struct {
	ID               string             `json:"id"`
	Query            string             `json:"query"`
	ExpectedContains []string           `json:"expected_contains,omitempty"`
	ExpectedTools    []string           `json:"expected_tools,omitempty"`
	MaxLoopDepth     int                `json:"max_loop_depth,omitempty"`
	MockResponses    []agent.Completion `json:"mock_responses,omitempty"`
}

func LoadDataset(path string) (Dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, err
	}
	var d Dataset
	err = json.Unmarshal(raw, &d)
	return d, err
}
