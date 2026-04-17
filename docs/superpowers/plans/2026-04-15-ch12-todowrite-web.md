# ch12 TodoWrite Web Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ch12` as a new web-based chapter that teaches explicit planning with `todo_write` and `nag reminder`, using a persisted plan snapshot and a visible todo panel.

**Architecture:** Reuse `ch10`'s HTTP/SSE server shape, add a lightweight planning domain under `ch12/agent/plan`, persist the latest plan snapshot on each chat message, and render the current plan in the existing legacy web chat flow. Keep the planning model intentionally minimal: whole-list overwrite, three statuses, branch-aware recovery from message history.

**Tech Stack:** Go, Gin, Gorm + SQLite, OpenAI-compatible tool calling, React + Vite, Server-Sent Events.

---

## Chunk 1: Backend Chapter Skeleton

### Task 1: Create `ch12` baseline from the `ch10` service structure

**Files:**
- Create: `ch12/README.md`
- Create: `ch12/main/main.go`
- Create: `ch12/agent/agent.go`
- Create: `ch12/agent/stream.go`
- Create: `ch12/agent/tool/bash.go`
- Create: `ch12/agent/tool/tool.go`
- Create: `ch12/server/controller.go`
- Create: `ch12/server/db.go`
- Create: `ch12/server/history.go`
- Create: `ch12/server/service.go`
- Create: `ch12/server/service_test.go`
- Create: `ch12/vo/vo.go`
- Create: `ch12/vo/sse.go`
- Modify: `README.md`

- [ ] **Step 1: Copy the `ch10` server layout into a new `ch12` package tree**

Replicate the `ch10` file structure so `ch12` starts from a known-good web chapter shape rather than mixing TUI-era files into a service chapter.

- [ ] **Step 2: Update package names, imports, and entrypoint wiring**

Set the `ch12` main package to initialize:

```go
db, err := server.InitDB("ch12.db")
a := agent.NewAgent(appConf.LLMProviders.FrontModel, agent.SystemPrompt, []tool.Tool{tool.NewBashTool()})
s := server.NewServer(db, a)
router := server.NewRouter(s)
```

- [ ] **Step 3: Add a failing smoke build target**

Run: `go test -v ./ch12/...`

Expected: FAIL because planning files and new chapter wiring do not exist yet.

- [ ] **Step 4: Fill in the minimum copied code until the chapter builds**

Do not add planning behavior yet. Make `ch12` a clean copy baseline that still behaves like `ch10`.

- [ ] **Step 5: Re-run the smoke target**

Run: `go test -v ./ch12/...`

Expected: PASS for the copied baseline tests or no-op test set.

- [ ] **Step 6: Commit**

```bash
git add ch12 README.md
git commit -m "feat: scaffold ch12 web chapter"
```

## Chunk 2: Planning Domain

### Task 2: Implement the planning state model and validation manager

**Files:**
- Create: `ch12/agent/plan/state.go`
- Create: `ch12/agent/plan/manager.go`
- Create: `ch12/agent/plan/manager_test.go`

- [ ] **Step 1: Write failing tests for plan validation and snapshot replacement**

Cover these behaviors in `ch12/agent/plan/manager_test.go`:

```go
func TestManagerReplaceSnapshot_AllowsSingleInProgress(t *testing.T)
func TestManagerReplaceSnapshot_RejectsMultipleInProgress(t *testing.T)
func TestManagerReplaceSnapshot_RejectsBlankContent(t *testing.T)
func TestManagerReplaceSnapshot_AllowsClearPlan(t *testing.T)
func TestManagerReplaceSnapshot_IncrementsRevision(t *testing.T)
```

- [ ] **Step 2: Run the focused plan tests**

Run: `go test -v ./ch12/agent/plan`

Expected: FAIL with missing types or methods like `PlanningState`, `ReplaceSnapshot`, or `PlanStatusInProgress`.

- [ ] **Step 3: Implement the minimal state model**

Add:

```go
type PlanStatus string
type PlanItem struct { Content string; Status PlanStatus }
type PlanningState struct { Items []PlanItem; Revision int; LastUpdatedLoop int }
```

Implement manager rules:

- content non-empty
- max 8 items
- statuses limited to `pending`, `in_progress`, `completed`
- at most one `in_progress`
- empty list allowed

- [ ] **Step 4: Implement whole-list replacement**

Expose one manager method with behavior equivalent to:

```go
func (m *Manager) ReplaceSnapshot(items []PlanItem, loopIndex int) (PlanningState, error)
```

It should normalize input, overwrite the whole list, increment revision, and update `LastUpdatedLoop`.

- [ ] **Step 5: Re-run the focused tests**

Run: `go test -v ./ch12/agent/plan`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ch12/agent/plan
git commit -m "feat: add ch12 planning state manager"
```

### Task 3: Add the `todo_write` tool on top of the planning manager

**Files:**
- Create: `ch12/agent/tool/todo.go`
- Create: `ch12/agent/tool/todo_test.go`
- Modify: `ch12/agent/tool/tool.go`

- [ ] **Step 1: Write failing tool tests**

Add tests that prove:

```go
func TestTodoWriteTool_ReplaceSnapshot(t *testing.T)
func TestTodoWriteTool_ReturnsValidationError(t *testing.T)
func TestTodoWriteTool_AllowsClearPlan(t *testing.T)
```

The tests should verify both manager mutation and returned tool result content.

- [ ] **Step 2: Run the focused tool tests**

Run: `go test -v ./ch12/agent/tool`

Expected: FAIL because `todo_write` is not registered.

- [ ] **Step 3: Implement the tool schema and execution**

Add a tool with:

```go
Name: "todo_write"
Parameters:
{
  "items": [{ "content": "...", "status": "pending|in_progress|completed" }]
}
```

`Execute` should parse JSON, call the manager, and return the normalized snapshot as JSON text.

- [ ] **Step 4: Register the tool name in the tool registry**

Update the tool constants and interfaces so `todo_write` is a first-class native tool.

- [ ] **Step 5: Re-run the tool package tests**

Run: `go test -v ./ch12/agent/tool`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ch12/agent/tool
git commit -m "feat: add todo_write tool for ch12"
```

## Chunk 3: Agent Loop Integration

### Task 4: Integrate planning state and todo snapshot events into the agent loop

**Files:**
- Modify: `ch12/agent/agent.go`
- Create: `ch12/agent/agent_test.go`
- Modify: `ch12/agent/stream.go`
- Modify: `ch12/vo/sse.go`
- Modify: `ch12/vo/vo.go`

- [ ] **Step 1: Write failing agent tests for todo snapshot emission**

Add tests that prove:

```go
func TestRunStreaming_EmitsTodoSnapshotAfterTodoWrite(t *testing.T)
func TestRunStreaming_UsesExistingPlanState(t *testing.T)
```

Stub the tool or LLM boundary as narrowly as possible so the tests target event emission and state mutation, not external API behavior.

- [ ] **Step 2: Write failing tests for nag reminder decisions**

Add focused tests like:

```go
func TestReminderState_ShouldRemindWhenToolLoopStartsWithoutPlan(t *testing.T)
func TestReminderState_ShouldRemindWhenPlanIsStale(t *testing.T)
func TestReminderState_DoesNotRepeatReminderInSameTurn(t *testing.T)
```

- [ ] **Step 3: Run the focused agent tests**

Run: `go test -v ./ch12/agent/...`

Expected: FAIL with missing reminder state or todo event support.

- [ ] **Step 4: Extend the run state to carry planning state**

Pass a current `PlanningState` into `RunStreaming`, maintain per-turn counters such as:

```go
loopIndex
toolCallsThisTurn
noTodoReminderSent
staleTodoReminderSent
```

- [ ] **Step 5: Emit a full snapshot event after successful `todo_write`**

Add a new stream event:

```go
const EventTodoSnapshot = "todo_snapshot"
```

and include:

```go
PlanState *plan.PlanningState
```

in the stream payload or equivalent VO.

- [ ] **Step 6: Inject loop-local reminder text instead of durable messages**

Before each LLM call, conditionally append a temporary reminder message to the request-only message slice when:

- tool loop has started with no plan
- plan is stale relative to execution progress

Do not persist reminder messages into stored rounds.

- [ ] **Step 7: Re-run the focused agent tests**

Run: `go test -v ./ch12/agent/...`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add ch12/agent ch12/vo
git commit -m "feat: integrate planning state into ch12 agent loop"
```

## Chunk 4: Server Persistence and Recovery

### Task 5: Persist plan snapshots on chat messages and recover them by branch

**Files:**
- Modify: `ch12/server/db.go`
- Modify: `ch12/server/history.go`
- Modify: `ch12/server/service.go`
- Create: `ch12/server/history_test.go`
- Modify: `ch12/server/service_test.go`
- Modify: `ch12/vo/vo.go`

- [ ] **Step 1: Write failing history tests for plan recovery**

Add tests like:

```go
func TestFindLatestPlanState_FollowsAncestorChain(t *testing.T)
func TestFindLatestPlanState_IgnoresSiblingBranchSnapshots(t *testing.T)
func TestFindLatestPlanState_ReturnsEmptyWhenNoSnapshot(t *testing.T)
```

- [ ] **Step 2: Write failing service tests for persisted plan state**

Extend `service_test.go` to prove:

```go
func TestCreateMessage_SavesLatestPlanSnapshot(t *testing.T)
func TestListMessages_ReturnsPlanState(t *testing.T)
```

Use seeded `ChatMessage` rows where needed.

- [ ] **Step 3: Run the focused server tests**

Run: `go test -v ./ch12/server`

Expected: FAIL because `ChatMessage` has no `PlanState` and history recovery does not exist.

- [ ] **Step 4: Add `PlanState` storage to the DB model**

Extend `ChatMessage` with:

```go
PlanState string
```

and let AutoMigrate create the new column.

- [ ] **Step 5: Add branch-aware plan recovery helpers**

Implement a helper with behavior equivalent to:

```go
func findLatestPlanState(allMsgs []ChatMessage, parentMessageID string) *plan.PlanningState
```

It should walk the same ancestor path logic as history reconstruction and return the nearest saved snapshot.

- [ ] **Step 6: Save the latest snapshot at the end of each run**

When `CreateMessage` finishes:

- serialize current `PlanningState`
- save it into `ChatMessage.PlanState`
- expose the parsed value on `ChatMessageVO`

- [ ] **Step 7: Re-run the focused server tests**

Run: `go test -v ./ch12/server`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add ch12/server ch12/vo
git commit -m "feat: persist and recover ch12 plan snapshots"
```

## Chunk 5: Web UI

### Task 6: Expose todo snapshots to the legacy web chat flow

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/api.ts`
- Modify: `frontend/src/components/ChatPanel.tsx`
- Create: `frontend/src/components/TodoPanel.tsx`
- Modify: `frontend/src/index.css`

- [ ] **Step 1: Write the smallest frontend proof target**

Run: `pnpm --dir frontend build`

Expected: PASS before edits so later failures are attributable to the todo panel changes.

- [ ] **Step 2: Extend the frontend API types**

Add:

```ts
export interface PlanItem { content: string; status: 'pending' | 'in_progress' | 'completed' }
export interface PlanningState { items: PlanItem[]; revision: number; last_updated_loop: number }
```

Then extend:

- `SSEMessageVO` with `event: 'todo_snapshot'`
- `ChatMessageVO` with optional `plan_state`

- [ ] **Step 3: Add a dedicated `TodoPanel` component**

Render:

- empty state when `plan_state` is missing or empty
- active item highlight for `in_progress`
- simple grouped or color-coded rows for completed vs pending

Keep it read-only.

- [ ] **Step 4: Integrate the panel into the legacy chat layout**

Change `frontend/src/App.tsx` so `ch12` uses the legacy app path instead of the assistant-ui path. Keep the reason explicit in code comments: the chapter needs direct access to planning snapshots without adding assistant-ui-specific runtime plumbing.

- [ ] **Step 5: Teach `ChatPanel` to track the latest snapshot**

Initialize from fetched message history, then on SSE:

- replace local plan state when `event === 'todo_snapshot'`
- keep the latest snapshot visible during streaming
- preserve the panel after stream completion

- [ ] **Step 6: Re-run the frontend build**

Run: `pnpm --dir frontend build`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/App.tsx frontend/src/api.ts frontend/src/components/ChatPanel.tsx frontend/src/components/TodoPanel.tsx frontend/src/index.css
git commit -m "feat: add todo panel to ch12 web UI"
```

## Chunk 6: Chapter Documentation and End-to-End Verification

### Task 7: Document the chapter and verify the end-to-end flow

**Files:**
- Modify: `ch12/README.md`
- Modify: `README.md`

- [ ] **Step 1: Write the chapter README around the implemented behavior**

Explain:

- why agents drift on multi-step tasks
- what `todo_write` does
- what `nag reminder` does
- why snapshots are persisted on `ChatMessage`
- why the web UI uses the legacy chat flow for this chapter

- [ ] **Step 2: Update the project root README**

Add `ch12` to:

- chapter list
- learning path
- quick start commands

- [ ] **Step 3: Run the targeted backend verification**

Run: `go test -v ./ch12/...`

Expected: PASS.

- [ ] **Step 4: Run the frontend verification**

Run: `pnpm --dir frontend build`

Expected: PASS.

- [ ] **Step 5: Run a manual end-to-end smoke test**

Start backend:

```bash
go run ./ch12/main
```

Start frontend:

```bash
pnpm --dir frontend dev
```

Manual checks:

- create a conversation
- send a multi-step coding request
- confirm `todo_snapshot` appears in the panel
- confirm the panel updates after later `todo_write`
- refresh the page and confirm the latest plan is restored
- branch from an older message and confirm the panel follows that branch

- [ ] **Step 6: Commit**

```bash
git add ch12/README.md README.md
git commit -m "docs: document ch12 explicit planning chapter"
```
