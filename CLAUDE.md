# CLAUDE.md

Guidance for Claude Code / AI agents working in this repository. See `AGENTS.md` for contribution conventions and `README.md` for the public roadmap.

## Project Overview

BabyAgent is a Go tutorial that builds a Claude Code-like agent from first principles. It is a **single Go module** (`module babyagent`) with 12 chapters. Each chapter is a runnable snapshot; later chapters intentionally copy earlier code.

- ch01: raw HTTP + OpenAI SDK, SSE streaming
- ch02: tool calling + minimal agent loop
- ch03: reasoning fields + Bubble Tea TUI
- ch04: MCP client and namespaced tools
- ch05: context engineering (truncate/offload/summarize)
- ch06: two-level memory (global/workspace)
- ch07: Agentic RAG (pgvector, embedding, rerank, CLI)
- ch08: Docker sandbox + human-in-the-loop confirmation
- ch09: skills (progressive loading + `load_skill`)
- ch10: HTTP/SSE service + SQLite conversation tree
- ch11: observability (OpenTelemetry + Prometheus + AgentTrace)
- ch12: offline evaluation with a Mock LLM

The official OpenAI SDK is replaced by the `baby-llm/openai-go` fork (see `go.mod`), which adds native `reasoning_content` support for thinking providers such as DeepSeek. Do not reintroduce manual `SetExtraFields` hacks for that field.

## Common Commands

```bash
# Configuration
cp .env.example .env                  # ch01-ch04
cp config.example.json config.json    # ch05-ch11

# Run
go run ./ch01/main --stream -q "hello"
go run ./ch03/main                    # TUI
go run ./ch07/main -index . -q "..."  # RAG CLI; needs PG + embedding service
go run ./ch10/main                    # HTTP/SSE on :8080
go run ./ch11/main                    # observability service on :8080
go run ./ch12/main                    # offline eval, no API key

# Verify
go test ./...
go vet ./...
gofmt -l .
pnpm --dir frontend lint && pnpm --dir frontend build
```

## Configuration

- ch01–ch04: `.env` with `OPENAI_API_KEY`; `OPENAI_BASE_URL` / `OPENAI_MODEL` fall back to defaults when empty.
- ch05–ch11: `config.json` with `llm_providers.front_model` and `llm_providers.back_model` (`base_url`, `api_key`, `model`, `context_window`).
- ch04+: optional `mcp-server.json`; default filesystem server needs `npx`.
- ch07: `CH07_PG_DSN`, `CH07_EMBED_BASE_URL`, `CH07_EMBED_API_KEY`, optional rerank vars; see `ch07/README.md`.

`.env` and `config.json` are gitignored. Never commit secrets.

## Architecture

### Agent loop

`RunStreaming` builds `[system] + history + user`, calls the Chat Completions streaming API, accumulates chunks with `ChatCompletionAccumulator`, emits reasoning/content/tool events, executes tools, appends assistant/tool messages, and repeats until no tool calls remain.

Key invariants:
- Call `stream.Close()` after each streaming round (the stream is recreated inside the loop; `defer` would leak).
- Reasoning is parsed from the SDK delta; `message.ToParam()` carries `reasoning_content` back on tool turns via the fork.
- SSE/stream sends must be cancellation-aware (`select` on `ctx.Done()`); a browser disconnect must not block the agent goroutine.

### Chapter-specific layers

- ch05 `context`: `Engine` + `Policy` interface (truncate/offload/summarize), token counting, policy events.
- ch06 `memory`: `MultiLevelMemory` (global + workspace), LLM-driven XML update, filesystem persistence.
- ch07 `rag`/`db`/`index`/`tool`: chunking, HTTP embedding/rerank, pgvector store, incremental indexing, semantic search.
- ch08 `tool/docker_bash.go` + confirmation state machine: allow/reject/always-allow; ESC cancels the run.
- ch09 `skill` + `tool/load_skill.go`: scan `.babyagent/skills/`, inject metadata, load full Markdown on demand.
- ch10 `agent.StreamEvent` + `server`: transport-agnostic events, SSE, SQLite `Conversation`/`ChatMessage`, tree history.
- ch11 `observe`: OTel tracer/spans, Prometheus metrics, `AgentTrace` event log; `server` adds `/metrics` and `/api/trace/:id`.
- ch12 `agent`/`eval`: vendor-neutral `Client`/`Tool` interfaces, `MockClient`, dataset runner, JSON/Markdown report.

### Chapter independence

- No chapter imports another chapter. Use `babyagent/shared` or the chapter's own copy.
- When a fix applies to copied files, patch every copy in the same change (e.g. TUI finalization, `stream.Close`, reasoning pass-back).
- Add a new chapter for a new capability instead of branching an existing one.

## Documentation

- Principles/design may be ahead of code. Keep them and add `> **实现状态**` notes; do not delete unimplemented sections.
- Root README owns status/roadmap. Chapter READMEs own run instructions and the delta vs the previous chapter.
- Community references should be official and dated; prefer the last six months.

## Testing

- Put tests next to the code under test; use `Test<Feature>_<Scenario>`.
- Use fake/mock LLM and vector-store clients; unit tests must not need network or PostgreSQL.
- Close SQLite handles in `t.Cleanup` before `t.TempDir()` cleanup.
- ch12 is the reference for deterministic agent-loop tests.

## Frontend

`frontend/` is the assistant-ui client for ch10/ch11. `src/api.ts` uses native `fetch` + `ReadableStream` for SSE; do not switch back to a reconnecting EventSource, which would duplicate POSTs and messages. See `frontend/README.md`.
