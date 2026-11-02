# BabyAgent Frontend

React + Vite + Tailwind client for the ch10 (HTTP/SSE) and ch11 (observability) backends. It uses `assistant-ui` for the chat thread, composer, and thread list.

## Prerequisites

- Node.js 24+
- pnpm
- A running ch10 or ch11 backend on `http://localhost:8080`

```bash
pnpm install
pnpm dev      # http://localhost:5173, proxies /api to :8080
pnpm build
pnpm lint
```

ch10 and ch11 both listen on `:8080` by default; run one at a time or change the port in the chapter's `main.go`.

## API contract

The client calls these endpoints through the Vite proxy (`/api` → `http://localhost:8080`):

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/conversation` | create a conversation |
| `GET` | `/api/conversation?user_id=` | list conversations |
| `PATCH` | `/api/conversation/:id` | rename a conversation |
| `DELETE` | `/api/conversation/:id` | delete a conversation |
| `POST` | `/api/conversation/:id/message` | send a message; SSE stream |
| `GET` | `/api/conversation/:id/message` | load message history |

The SSE stream emits JSON `data:` frames with `event` set to `content`, `reasoning`, `tool_call`, `tool_result`, or `error`. ch11 additionally stores an `AgentTrace` and exposes `GET /api/trace/:trace_id`; the current UI does not render the trace panel yet.

`streamThreadRun` in `src/api.ts` uses native `fetch` + `ReadableStream` (not `EventSource`) because a chat turn is one POST: automatic reconnection would create duplicate messages and duplicate agent runs.

## Structure

- `src/api.ts` — HTTP + SSE client, conversation/message types.
- `src/App.tsx` — assistant-ui runtime wiring.
- `src/components/assistant-ui/` — thread, message, composer, reasoning panel, tool-call card, model adapter.

## Known limitations

- `USER_ID` is hardcoded to `user_001`; the backend does not authenticate or check conversation ownership.
- The frontend only works through the Vite dev proxy. For production, add CORS or serve `frontend/dist` from the Go server.
- Trace visualization (ch11) is not implemented in the UI.
