package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"babyagent/ch07/db"
	"babyagent/ch07/index"
	"babyagent/ch07/rag"
	"babyagent/ch07/tool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		indexPath = flag.String("index", "", "directory to index (empty = skip indexing)")
		query     = flag.String("q", "", "query to search (empty = skip search)")

		dsn = flag.String("dsn", env("CH07_PG_DSN", ""), "PostgreSQL DSN (or CH07_PG_DSN)")

		embedBase   = flag.String("embed-base-url", env("CH07_EMBED_BASE_URL", ""), "OpenAI-compatible embedding base URL")
		embedKey    = flag.String("embed-api-key", env("CH07_EMBED_API_KEY", ""), "embedding API key")
		embedModel  = flag.String("embed-model", env("CH07_EMBED_MODEL", rag.DefaultEmbeddingModel), "embedding model")
		dimensions  = flag.Int("dimensions", envInt("CH07_EMBED_DIMENSIONS", rag.DefaultDimensions), "embedding dimensions")
		rerankBase  = flag.String("rerank-base-url", env("CH07_RERANK_BASE_URL", ""), "optional rerank base URL")
		rerankKey   = flag.String("rerank-api-key", env("CH07_RERANK_API_KEY", ""), "rerank API key")
		rerankModel = flag.String("rerank-model", env("CH07_RERANK_MODEL", "rerank"), "rerank model")
		topK        = flag.Int("top-k", 5, "number of search results")
		chunkerType = flag.String("chunker", "line", "chunker: line or paragraph")
		maxLines    = flag.Int("max-lines", 100, "line chunker: max lines per chunk")
		maxChars    = flag.Int("max-chars", 2000, "line chunker: max characters per chunk")
		concurrency = flag.Int("concurrency", 4, "number of files indexed in parallel")
	)

	flag.Parse()

	if *indexPath == "" && *query == "" {
		flag.Usage()
		return fmt.Errorf("nothing to do: pass -index <dir> and/or -q <query>")
	}
	if *dsn == "" {
		return fmt.Errorf("missing -dsn (or CH07_PG_DSN), e.g. host=localhost port=5432 user=postgres password=postgres dbname=babyagent sslmode=disable")
	}
	if *embedBase == "" {
		return fmt.Errorf("missing -embed-base-url (or CH07_EMBED_BASE_URL)")
	}

	ctx := context.Background()
	store, err := db.NewPGVectorStore(db.Config{DSN: *dsn, Dimension: *dimensions})
	if err != nil {
		return err
	}
	defer store.Close()

	embedder := rag.NewHTTPEmbeddingService(rag.HTTPEmbeddingConfig{
		APIKey:     *embedKey,
		BaseURL:    *embedBase,
		Model:      *embedModel,
		Dimensions: *dimensions,
	})

	if *indexPath != "" {
		idx := index.NewIndexer(index.IndexerConfig{
			RootPath:    *indexPath,
			ChunkerType: rag.ChunkerType(*chunkerType),
			MaxLines:    *maxLines,
			MaxChars:    *maxChars,
		}, store, embedder)

		result, err := idx.IndexConcurrent(ctx, *concurrency)
		if err != nil {
			return err
		}
		fmt.Printf(
			"indexed %s in %s: files=%d success=%d skipped=%d reindexed=%d failed=%d chunks=%d\n",
			*indexPath, result.Duration, result.TotalFiles, result.SuccessFiles,
			result.SkippedFiles, result.ReindexedFiles, result.FailedFiles, result.TotalChunks,
		)
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  index error: %v\n", err)
		}
	}

	if *query != "" {
		var reranker rag.RerankService
		if *rerankBase != "" {
			reranker = rag.NewHTTPRerankService(rag.HTTPRerankConfig{
				APIKey:  *rerankKey,
				BaseURL: *rerankBase,
				Model:   *rerankModel,
			})
		}

		searchTool := tool.NewSemanticSearchTool(embedder, store, reranker)
		args, err := json.Marshal(map[string]any{"query": *query, "top_k": *topK})
		if err != nil {
			return err
		}
		out, err := searchTool.Execute(ctx, string(args))
		if err != nil {
			return err
		}
		fmt.Println(out)
	}

	return nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
