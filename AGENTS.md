# Repository Guidelines

## Project Structure & Module Organization

This repository is a chapter-based Go tutorial for building AI agents. Each chapter is a **snapshot** of the agent at that stage; later chapters intentionally copy earlier code so a reader can study one chapter without jumping across packages.

- `ch01`–`ch12`: runnable chapter implementations.
  - ch01–ch04: `.env` configuration; ch05–ch11: `config.json` (front/back models).
  - ch07 additionally needs PostgreSQL + pgvector and an embedding service.
  - ch10/ch11 share the React frontend in `frontend/`.
  - ch12 uses a Mock LLM and needs no API key.
- `shared/`: infrastructure shared by all chapters (config, env loading, LLM client, MCP config, logging).
- `frontend/`: React + Vite + Tailwind + assistant-ui client for ch10/ch11.
- `.github/workflows/test-build.yml`: CI (gofmt, vet, per-chapter test/build, frontend lint/build).
- `CONTRIBUTORS.md`: credit list for merged contributions.

Follow the existing chapter pattern (`prompt.go`, `agent.go`, `main/main.go`, optional `tool/`, `context/`, `memory/`, `storage/`, `tui/`) when adding functionality.

## Build, Test, and Development Commands

Use Go 1.25+. Use Node 24 + pnpm for the frontend.

- `cp .env.example .env`: configuration for ch01–ch04.
- `cp config.example.json config.json`: configuration for ch05–ch11.
- `go run ./ch01/main --stream -q "hello"`: run a chapter entrypoint.
- `go test -v ./ch08/...`: run tests for one chapter.
- `go test -v ./...`: run all tests in the module.
- `go build -v ./ch09/main`: build a chapter binary (CI builds every chapter main).
- `go vet ./...`: run vet (CI).
- `gofmt -l .`: check formatting (CI); `gofmt -w <paths>` to apply.
- `pnpm --dir frontend install && pnpm --dir frontend dev`: run the frontend.
- `pnpm --dir frontend lint && pnpm --dir frontend build`: CI frontend checks.

## Chapter Independence & Fix Propagation

- **No chapter may import another chapter.** Import only the chapter's own packages and `babyagent/shared`; use `go list`/grep to verify.
- A chapter may copy code from the previous chapter. Keep the copy; do not replace it with a cross-chapter import.
- When fixing a file that exists in several chapters, patch **every copy in the same PR** and list the affected chapters. Past examples: TUI stream finalization (ch03/04/05/06/08/09), `stream.Close()` (all streaming chapters), `reasoning_content` pass-back (ch03–ch11).
- When adding a new capability, add a new chapter instead of branching an old one.
- If two copies are meant to stay byte-identical, say so in the file header and keep them in sync. Intentional differences belong in the chapter README's delta table.

## Documentation Rules

- **Principles and design notes may be ahead of the code. Do not delete them.** Mark them with a `> **实现状态**` note and point to the chapter where they are implemented.
- Root `README.md` is the single source of truth for the roadmap and chapter status.
- Every chapter README should state: prerequisites, what changed vs the previous chapter, what is intentionally not implemented, and the next chapter.
- Prefer official, dated references. When adding community references, only cite material published in the last six months unless it is a foundational spec.
- Keep the chapter's "运行" section executable: exact commands, required env/config, and expected port or output.

## Coding Style & Naming Conventions

- Follow standard Go formatting and idioms (`gofmt` is required; tabs for indentation).
- Keep package names short, lowercase, and domain-focused (`context`, `tool`, `memory`).
- Exported identifiers: `PascalCase`; internal helpers: `camelCase`.
- Prefer small, composable files over large mixed-responsibility files.
- When copying a file across chapters, add a one-line header comment when the change is not obvious.

## Testing Guidelines

- Place tests in `*_test.go` next to the code under test.
- Use descriptive test names like `Test<Feature>_<Scenario>`.
- Add tests for new logic in the same chapter/package and run `go test -v ./<chapter>/...`.
- Prefer Mock/fake clients for LLM and vector-store tests; do not require network or PostgreSQL for unit tests.
- Close SQLite handles in `t.Cleanup` before `t.TempDir()` cleanup (Windows otherwise fails to remove the file).

## Commit & Pull Request Guidelines

- Keep commits scoped and imperative (`implement ch08`, `fix tui ...`, `docs: ...`).
- Optional prefixes are useful (`feat:`, `fix:`, `refactor:`, `docs:`).
- PRs should include:
  - what changed and why,
  - affected chapters/packages and every synchronized copy,
  - local verification commands/results,
  - screenshots or terminal snippets for TUI/web-visible changes.
- When using someone else's commit, preserve authorship and add a `Co-authored-by` trailer; update `CONTRIBUTORS.md`.

## Security & Configuration Tips

- Never commit secrets. `.env` and `config.json` are gitignored; use `.env.example` / `config.example.json` as templates.
- The RAG indexer skips `.env`, `config*.json`, private keys, and `.babyagent/`, but this is a best-effort denylist, not a security boundary.
- `load_skill` and other model-controlled paths should validate path containment before production use.
