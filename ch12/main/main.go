package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"babyagent/ch12/agent"
	"babyagent/ch12/eval"
)

func main() {
	datasetPath := flag.String("dataset", "ch12/dataset/demo.json", "path to evaluation dataset JSON")
	outputPath := flag.String("output", "", "optional JSON report path")
	markdownPath := flag.String("markdown", "", "optional Markdown report path")
	parallelism := flag.Int("parallel", 1, "number of cases to run concurrently")
	flag.Parse()

	dataset, err := eval.LoadDataset(*datasetPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load dataset:", err)
		os.Exit(1)
	}
	report := eval.Run(context.Background(), dataset, *parallelism, func(ctx context.Context, c eval.Case) (agent.Result, error) {
		client := &agent.MockClient{Responses: append([]agent.Completion(nil), c.MockResponses...)}
		// The fixture tool lets a dataset exercise the scheduling path without touching the host filesystem.
		read := &agent.MockTool{ToolName: "read", Output: "fixture README content"}
		return agent.New(client, []agent.Tool{read}).Run(ctx, c.Query)
	})

	fmt.Print(report.Markdown())
	if *outputPath != "" {
		if err := report.WriteJSON(*outputPath); err != nil {
			fmt.Fprintln(os.Stderr, "write JSON report:", err)
			os.Exit(1)
		}
	}
	if *markdownPath != "" {
		if err := report.WriteMarkdown(*markdownPath); err != nil {
			fmt.Fprintln(os.Stderr, "write Markdown report:", err)
			os.Exit(1)
		}
	}
	if report.Failed > 0 {
		os.Exit(1)
	}
}
