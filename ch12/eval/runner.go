package eval

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"babyagent/ch12/agent"
)

type Executor func(context.Context, Case) (agent.Result, error)

type CaseResult struct {
	ID       string        `json:"id"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	Failures []string      `json:"failures,omitempty"`
	Error    string        `json:"error,omitempty"`

	// 以下字段是评测报告真正要看的输出，必须序列化进 JSON。
	Response  string      `json:"response,omitempty"`
	ToolCalls []string    `json:"tool_calls,omitempty"`
	LoopDepth int         `json:"loop_depth"`
	Usage     agent.Usage `json:"usage,omitempty"`
}

type Report struct {
	Dataset    string       `json:"dataset"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt time.Time    `json:"finished_at"`
	Total      int          `json:"total"`
	Passed     int          `json:"passed"`
	Failed     int          `json:"failed"`
	Cases      []CaseResult `json:"cases"`
}

func Run(ctx context.Context, dataset Dataset, parallelism int, execute Executor) Report {
	if parallelism < 1 {
		parallelism = 1
	}
	report := Report{
		Dataset:   dataset.Name,
		StartedAt: time.Now(),
		Total:     len(dataset.Cases),
		Cases:     make([]CaseResult, len(dataset.Cases)),
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range parallelism {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				report.Cases[i] = runCase(ctx, dataset.Cases[i], execute)
			}
		}()
	}
	for i := range dataset.Cases {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	report.FinishedAt = time.Now()
	for _, r := range report.Cases {
		if r.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	return report
}

func runCase(ctx context.Context, c Case, execute Executor) CaseResult {
	start := time.Now()
	result, err := execute(ctx, c)
	r := CaseResult{
		ID:        c.ID,
		Duration:  time.Since(start),
		Response:  result.Response,
		ToolCalls: result.ToolCalls,
		LoopDepth: result.LoopDepth,
		Usage:     result.Usage,
	}
	if err != nil {
		r.Error = err.Error()
		return r
	}
	for _, want := range c.ExpectedContains {
		if !strings.Contains(strings.ToLower(result.Response), strings.ToLower(want)) {
			r.Failures = append(r.Failures, fmt.Sprintf("response does not contain %q", want))
		}
	}
	if len(c.ExpectedTools) > 0 && !sameStrings(c.ExpectedTools, result.ToolCalls) {
		r.Failures = append(r.Failures, fmt.Sprintf("tools = %v, want %v", result.ToolCalls, c.ExpectedTools))
	}
	if c.MaxLoopDepth > 0 && result.LoopDepth > c.MaxLoopDepth {
		r.Failures = append(r.Failures, fmt.Sprintf("loop depth = %d, max %d", result.LoopDepth, c.MaxLoopDepth))
	}
	r.Passed = len(r.Failures) == 0
	return r
}

func sameStrings(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}
