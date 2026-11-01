package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func (r Report) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Eval report: %s\n\n", r.Dataset)
	fmt.Fprintf(&b, "- Total: %d\n- Passed: %d\n- Failed: %d\n\n", r.Total, r.Passed, r.Failed)
	b.WriteString("| Case | Result | Duration | Details |\n|---|---|---:|---|\n")
	for _, c := range r.Cases {
		state := "PASS"
		details := ""
		if !c.Passed {
			state = "FAIL"
			details = strings.Join(c.Failures, "; ")
			if c.Error != "" {
				details = c.Error
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.ID, state, c.Duration.Round(1e6), strings.ReplaceAll(details, "|", "\\|"))
	}
	return b.String()
}
func (r Report) WriteMarkdown(path string) error {
	return os.WriteFile(path, []byte(r.Markdown()), 0644)
}
