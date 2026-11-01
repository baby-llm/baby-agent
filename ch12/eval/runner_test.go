package eval

import (
	"babyagent/ch12/agent"
	"context"
	"testing"
)

func TestRun_ScoresAssertionsAndBuildsReport(t *testing.T) {
	d := Dataset{Name: "demo", Cases: []Case{{ID: "pass", ExpectedContains: []string{"hello"}, ExpectedTools: []string{"read"}, MaxLoopDepth: 2}, {ID: "fail", ExpectedContains: []string{"world"}}}}
	r := Run(context.Background(), d, 2, func(_ context.Context, c Case) (agent.Result, error) {
		if c.ID == "pass" {
			return agent.Result{Response: "hello", ToolCalls: []string{"read"}, LoopDepth: 2}, nil
		}
		return agent.Result{Response: "nope", LoopDepth: 1}, nil
	})
	if r.Passed != 1 || r.Failed != 1 {
		t.Fatalf("report = %+v", r)
	}
	if r.Cases[1].Passed || len(r.Cases[1].Failures) != 1 {
		t.Fatalf("case = %+v", r.Cases[1])
	}
	if got := r.Markdown(); got == "" {
		t.Fatal("empty markdown")
	}
}
