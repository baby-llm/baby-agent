package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"babyagent/ch01"
	"babyagent/shared"
)

func main() {
	_ = godotenv.Load()

	useRaw := flag.Bool("raw", false, "use raw http implementation")
	useStream := flag.Bool("stream", false, "use streaming response")
	query := flag.String("q", "hello", "prompt text")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	modelConf := shared.NewModelConfig()

	var err error
	switch {
	case *useRaw && *useStream:
		err = ch01.StreamingRequestRawHTTP(ctx, modelConf, *query)
	case *useRaw:
		err = ch01.NonStreamingRequestRawHTTP(ctx, modelConf, *query)
	case *useStream:
		err = ch01.StreamingRequestSDK(ctx, modelConf, *query)
	default:
		err = ch01.NonStreamingRequestSDK(ctx, modelConf, *query)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
}
