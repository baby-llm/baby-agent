# ch12 TodoWrite Web Design

## Goal

Add a new `ch12` chapter that teaches session-scoped planning for an agent:

- expose an explicit todo list through a `todo_write` tool
- add a lightweight nag reminder when the agent drifts or forgets to update the plan
- render the current plan in the existing web UI instead of the TUI

This chapter should explain "visible execution planning", not a full task system.

## Scope

`ch12` is built on top of the ideas from `ch03`, but its runtime and UI should reuse the web architecture from `ch10`.

In scope:

- session-scoped todo state
- minimal plan state machine: `pending / in_progress / completed`
- `todo_write` tool with whole-list overwrite semantics
- nag reminders injected into the current loop
- SSE event for plan snapshots
- web UI panel for the latest todo snapshot
- branch-aware plan recovery through existing message history

Out of scope:

- background tasks
- persistent task scheduler
- multi-agent coordination
- advanced status types like `blocked` or `cancelled`
- sorting rules beyond keeping the list as-written
- standalone `plan` database tables

## Chapter Positioning

Suggested narrative:

- `ch03`: make the agent's reasoning visible
- `ch12`: make the agent's execution plan visible

This keeps `ch12` focused on "what the agent intends to do next and what it already finished".

## High-Level Architecture

`ch12` should have three layers:

1. Agent planning layer
   - maintains current planning state during a run
   - exposes `todo_write`
   - decides when to inject nag reminders
2. Service persistence layer
   - stores the latest plan snapshot alongside each chat message
   - rebuilds plan state from the nearest ancestor message with a saved snapshot
3. Web presentation layer
   - listens for `todo_snapshot` SSE events
   - renders the current todo list in a right-side plan panel

The design should prefer full snapshots over incremental patches across all boundaries.

## Backend Design

### Directory Layout

Suggested `ch12` layout:

```text
ch12/
  README.md
  main/main.go
  agent/
    agent.go
    stream.go
    plan/
      manager.go
      state.go
    tool/
      bash.go
      todo.go
      tool.go
  server/
    controller.go
    db.go
    history.go
    service.go
  vo/
    sse.go
    vo.go
```

This mirrors the service-style structure already used in `ch10`, while isolating planning logic under `agent/plan`.

### Plan Data Structures

Minimal planning model:

```go
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
)

type PlanItem struct {
	Content string     `json:"content"`
	Status  PlanStatus `json:"status"`
}

type PlanningState struct {
	Items           []PlanItem `json:"items"`
	Revision        int        `json:"revision"`
	LastUpdatedLoop int        `json:"last_updated_loop"`
}
```

Rules:

- `content` must be non-empty
- maximum 8 items
- at most one `in_progress`
- unknown status is invalid
- empty list is allowed and means "clear plan"

### `todo_write` Tool

Tool name:

```text
todo_write
```

Request shape:

```go
type TodoWriteParam struct {
	Items []TodoWriteItem `json:"items"`
}

type TodoWriteItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}
```

Behavior:

- the input fully replaces the current plan
- validation happens in the plan manager
- successful writes increment `Revision`
- successful writes update `LastUpdatedLoop`
- the tool returns a short success string plus the normalized plan snapshot

Why whole-list overwrite:

- easier for the model to generate
- easier to validate
- easier to stream to the frontend
- easier to recover from history

### Nag Reminder Logic

Nag reminders should be temporary, loop-local instructions injected before the next LLM call. They should not be stored as durable conversation messages.

Trigger 1: missing plan

- current user turn has already entered tool loop
- at least one tool call has happened
- no plan exists yet
- reminder has not already been sent in this turn

Reminder goal:

- ask the model to call `todo_write`
- keep the list short
- keep at most one item `in_progress`

Trigger 2: stale plan

- a plan exists
- multiple execution steps have happened since the last plan update
- reminder has not already been sent in this turn

Reminder goal:

- update completed items
- move the next active item to `in_progress`
- keep the plan aligned with actual progress

Do not remind:

- for single-turn plain answers with no tool usage
- right after a fresh todo update
- when the plan is already complete
- once the loop is obviously ending

Per-turn limits:

- at most one "missing plan" reminder
- at most one "stale plan" reminder

Suggested per-turn counters:

- `loopIndex`
- `toolCallsThisTurn`
- `noTodoReminderSent`
- `staleTodoReminderSent`

## Persistence Design

Web UI requires plan recovery across requests, so the latest plan snapshot cannot live only in memory.

### Message Storage

Reuse the existing `ChatMessage` model pattern from `ch10` and add one optional field:

```go
PlanState string
```

This field stores serialized `PlanningState` JSON for the latest plan snapshot at the end of that message run.

Design choice:

- do not create a separate `plans` table in `ch12`
- keep the plan attached to the message branch where it was produced

This makes plan recovery naturally branch-aware.

### History Recovery

When handling a new request:

1. walk the message ancestor path using existing branch logic
2. find the nearest ancestor message containing non-empty `PlanState`
3. unmarshal it into the current `PlanningState`
4. pass that planning state into the next agent run

This gives each conversation branch its own visible plan history without introducing a full task database.

## SSE and API Design

### New SSE Event

Add a new SSE event type:

```text
todo_snapshot
```

Payload fields:

- `message_id`
- `event`
- `plan_state`

Where `plan_state` is the full current snapshot:

```json
{
  "items": [
    { "content": "Inspect login flow", "status": "completed" },
    { "content": "Patch handler logic", "status": "in_progress" },
    { "content": "Run regression tests", "status": "pending" }
  ],
  "revision": 2,
  "last_updated_loop": 3
}
```

Emission rule:

- emit immediately after every successful `todo_write`

This gives the frontend a simple "latest snapshot wins" model.

### Message API

Two backend output changes are needed:

1. SSE stream includes `todo_snapshot`
2. message list response includes the saved `plan_state` for each message, or at minimum for the latest message in the branch

Preferred approach:

- add optional `plan_state` to `ChatMessageVO`
- frontend derives the latest plan from the newest loaded message

## Frontend Design

### Layout

Use the existing web app and add a plan panel to the right side of the chat area.

Recommended structure:

```text
Sidebar | Chat Thread | Todo Panel
```

The todo panel should show:

- chapter title or label, e.g. "Current Plan"
- items grouped visually by status
- one active `in_progress` item highlighted
- empty state when no plan exists yet

### State Model

Frontend keeps only one current snapshot:

- initialize from loaded message history
- replace it whenever a new `todo_snapshot` SSE event arrives
- clear or switch it when changing conversations or branches

No client-side patching is needed.

### Visual Direction

Keep the UI simple and instructional:

- compact card panel
- no drag/drop
- no inline editing by user
- no status badges beyond minimal color coding

The panel is a teaching aid for the agent's plan, not a user-managed task board.

## README Positioning

`ch12/README.md` should explain:

1. why agents drift on multi-step tasks
2. why visible planning helps
3. why snapshots are easier than patches for a teaching project
4. why web UI needs persisted plan recovery

Suggested chapter title:

```text
第十二章：显式规划（TodoWrite 与 Nag Reminder）
```

## Testing Strategy

Minimum test targets:

- plan manager validation rules
- `todo_write` whole-list replacement behavior
- nag reminder trigger logic
- plan recovery from ancestor message history
- SSE serialization for `todo_snapshot`

Frontend verification can start with manual checks:

- send a multi-step request and confirm the panel appears
- verify updates after each `todo_write`
- refresh the page and confirm the latest plan is restored
- branch to a different parent message and confirm the plan follows that branch

## Deferred to a Later Chapter

Keep these for a later "full planning" follow-up:

- richer status machine
- status transition guards
- stable sorting rules
- progress summaries
- multiple simultaneous active items
- editable user-side task board
- background execution and scheduling

## Recommendation

Implement `ch12` as the first visible-planning chapter, then treat a later chapter as the advanced version instead of creating a literal `ch03-2` directory.
