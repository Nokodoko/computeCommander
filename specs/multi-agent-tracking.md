# Multi-Agent Tracking

Universal agent tracking for ComputeCommander. Decouples agent registration from Claude Code hooks, adds CLI-driven registration/heartbeat for any runtime (pi.dev, gemini, codex, goose), and wires agent lifecycle events into the OpenBrain pane, event feed pane, and evals pane.

## Why

cmdr-bridge.sh works but is hardwired to Claude Code's hook system, making every non-Claude agent invisible:

- **Claude-only tracking.** `cmdr-bridge.sh` relies on Claude Code's `SubagentStart`/`SubagentStop`/`PostToolUse` hooks. Pi.dev, Gemini, Codex, and Goose agents never fire these hooks, so they never appear in `cmdr status` or the dashboard Agents pane.
- **Claude-only counting.** `agent-counter.sh` uses `pgrep -x claude` to count live agents. Non-Claude processes are invisible to the counter, making the dwmblocks status bar inaccurate.
- **Silent OpenBrain.** The OpenBrain pane only watches MEMORY.md files for section diffs. Agent lifecycle events (spawn, complete, stall, error) are logged in the `events` table but never surface in OpenBrain. There is no unified "what's happening" view.
- **Event feed is Claude-only.** `cmdr feed --pane` streams events from the `events` table, but those events are only emitted by `cmdr-bridge.sh` (Claude hooks). Non-Claude agents produce zero events.
- **Evals are Claude-only.** `cmdr evals --pane` shows eval pass/fail results, but evals are only emitted by Claude's intent verification hooks (`cmdr-bridge.sh record-eval`). Non-Claude agents cannot emit eval results.
- **No runtime-agnostic registration API.** Non-Claude agents have no way to announce themselves. The bridge is coupled to Claude's JSON hook payload format.

This spec adds 3 new CLI commands (`register`, `deregister`, `heartbeat`), a DB migration, an agent-counter rewrite, multi-agent event emission, eval emission from any runtime, and an OpenBrain agent activity feed. ~11 files modified, ~5 files created.

## Design Principles

1. **Backwards compatible.** cmdr-bridge.sh continues working unchanged for Claude agents. New commands are additive -- nothing is removed or broken.
2. **Shell-first registration.** `cmdr register` / `cmdr deregister` / `cmdr heartbeat` are plain CLI commands callable from any shell script, wrapper, or hook system. No SDK required.
3. **DB as single source of truth.** All agent counts, statuses, and lifecycle events come from the `sessions` and `events` tables. No more `/tmp` counter files for agent counting.
4. **OpenBrain is memory-first, agents-second.** Agent lifecycle events appear below the memory change feed in the OpenBrain pane. Memory changes remain the primary content; agent events are a secondary activity ticker. The activity section shows a maximum of 5 recent events to prevent visual clutter.
5. **One migration per feature.** All schema changes go in `008_multi_agent.sql`. No schema drift across multiple migration files.

## On-Disk Format

```
computeCommander/
  internal/
    platform/db/
      migrations/
        sqlite/
          001_schema.sql           # existing schema (unchanged)
          008_multi_agent.sql      # new: adds runtime index, heartbeat_at column
        postgres/
          008_multi_agent.sql      # new: postgres equivalent
    agents/
      types.go                     # modified: add Runtime field to ListOpts
      spawner.go                   # modified: add runtime WHERE clause to ListSessions
    commands/
      register.go                  # new: cmdr register / deregister / heartbeat
      openbrain.go                 # modified: add agent activity feed
      status.go                    # modified: DB-backed agent counting
      observability.go             # modified: restructure FeedCmd for subcommands, add `feed emit`
      evals.go                     # modified: document `evals emit` as public API, add runtime field to emit
  scripts/
    cmdr-agent-counter.sh          # new: DB-backed universal agent counter
```

### 008_multi_agent.sql (SQLite)

Adds `heartbeat_at` column for non-hook runtimes and an index on `runtime` for filtered queries.

```sql
-- Migration 008: Multi-agent tracking support

-- Add heartbeat_at for non-Claude runtimes that lack hook-based activity updates
ALTER TABLE sessions ADD COLUMN heartbeat_at TEXT;

-- Index for runtime-filtered queries (e.g., "show all pi agents")
CREATE INDEX IF NOT EXISTS idx_sessions_runtime ON sessions(runtime);

-- Index for heartbeat staleness queries
CREATE INDEX IF NOT EXISTS idx_sessions_heartbeat ON sessions(heartbeat_at)
    WHERE state IN ('booting', 'working');
```

### 008_multi_agent.sql (PostgreSQL)

```sql
-- Migration 008: Multi-agent tracking support

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_sessions_runtime ON sessions(runtime);
CREATE INDEX IF NOT EXISTS idx_sessions_heartbeat ON sessions(heartbeat_at)
    WHERE state IN ('booting', 'working');
```

### register.go

New file. Exports `RegisterCmd(app *App) *cobra.Command` with three subcommands:

```
cmdr register      Register an agent session in the DB
cmdr deregister    Mark an agent session as completed
cmdr heartbeat     Update last_activity / heartbeat_at for an agent
```

### cmdr-agent-counter.sh

New file replacing `~/.claude/hooks/agent-counter.sh` with a DB-backed counter.

```bash
#!/bin/sh
# cmdr-agent-counter.sh - Universal agent counter (DB-backed)
# Counts all working agents across all runtimes from sessions table.
COUNT=$(cmdr status --json 2>/dev/null | jq '.sessions | map(select(.state == "working")) | length' 2>/dev/null || echo 0)
printf "%s" "$COUNT" > /tmp/claude_subagent_count
pkill -RTMIN+12 dwmblocks 2>/dev/null || true
```

## Data Model

### AgentSession (modified)

```typescript
interface AgentSession {
  // Identity
  id: string;             // "pi-a3f8c1e2" or "claude-b7d9e4f1"
  agent_name: string;     // "unix-coder", "spec-builder"
  capability: Capability; // "builder", "reviewer", etc.
  runtime: string;        // "claude", "pi", "gemini", "codex", "goose"

  // Workspace
  worktree_path?: string;
  branch_name?: string;
  task_id: string;
  zellij_pane?: string;

  // State
  state: SessionState;    // "booting" | "working" | "completed" | "stalled" | "zombie"
  pid?: number;
  parent_agent?: string;
  depth: number;

  // Tracking
  run_id?: string;
  started_at: string;     // ISO 8601
  last_activity: string;  // ISO 8601, updated by hooks OR heartbeat
  heartbeat_at?: string;  // ISO 8601, set ONLY by cmdr heartbeat (null for hook-based runtimes)

  // Monitoring
  escalation_level: number;
  stalled_since?: string;
  transcript_path?: string;
}
```

### ListOpts (modified)

The existing `ListOpts` struct in `internal/agents/types.go` must be extended with a `Runtime` field:

```go
// ListOpts filters agent session listing.
type ListOpts struct {
	RunID      string
	Capability Capability
	State      SessionState
	Parent     string
	ProjectID  string
	Runtime    runtimes.RuntimeID  // NEW: filter by runtime (e.g., "pi", "claude")
}
```

The `ListSessions` method in `internal/agents/spawner.go` must add a corresponding `WHERE` clause:

```go
if opts.Runtime != "" {
    query += fmt.Sprintf(" AND s.runtime = $%d", argIdx)
    args = append(args, string(opts.Runtime))
    argIdx++
}
```

### AgentLifecycleEvent (new -- stored in existing `events` table)

```typescript
interface AgentLifecycleEvent {
  // Stored in events table
  id: number;                // AUTOINCREMENT
  agent_name: string;
  session_id: string;        // FK to sessions.id
  event_type: AgentEventType;
  level: "info" | "warn";
  data: string;              // JSON: { runtime, capability, task_id, ... }
  created_at: string;        // ISO 8601
}

type AgentEventType =
  | "agent.registered"       // cmdr register or bridge SubagentStart
  | "agent.deregistered"     // cmdr deregister or bridge SubagentStop
  | "agent.heartbeat"        // cmdr heartbeat
  | "agent.stalled"          // watchdog detected stall
  | "agent.working"          // state transition to working
  ;
```

### Agent Session Lifecycle

```
registered ──> booting ──> working ──> completed
                  │            │
                  │            ├──> stalled ──> zombie
                  │            │       │
                  │            │       └──> working  (recovery via heartbeat)
                  │            │
                  └────────────┴──> deregistered (force stop)
```

### Heartbeat Staleness

| Runtime Type | Liveness Signal | Stale Threshold |
|-------------|----------------|-----------------|
| Claude | Hook-based (`last_activity` updated by PostToolUse) | 10 min (existing watchdog) |
| Non-Claude (pi, gemini, codex, goose) | Heartbeat-based (`heartbeat_at` updated by `cmdr heartbeat`) | 5 min since last heartbeat |

## CLI

Binary name: `cmdr`

Every command supports `--json` for structured output.

### Agent Registration

```
cmdr register                              Register a new agent session
  --name <agent-name>     (required)       Agent name (e.g., "unix-coder")
  --runtime <runtime-id>  (required)       Runtime: claude, pi, gemini, codex, goose
  --capability <cap>      (required)       Capability: scout, builder, reviewer, lead, etc.
  --task <task-id>        (required)       Task identifier
  --pid <process-id>                       OS process ID for liveness checks
  --parent <agent-name>                    Parent agent (for subagent tracking)
  --worktree <path>                        Worktree path
  --branch <branch-name>                   Git branch

cmdr deregister <session-id>               Mark agent session as completed
  --state <final-state>                    Final state: completed (default) or zombie
  --reason <text>                          Reason for deregistration

cmdr heartbeat <session-id>                Update agent heartbeat timestamp
  --state <state>                          Optionally update state (working, stalled)
  --tokens-in <n>                          Input tokens consumed since last heartbeat
  --tokens-out <n>                         Output tokens consumed since last heartbeat
```

### Event Emission (new subcommand on existing `cmdr feed`)

```
cmdr feed emit                             Emit an event from any agent/runtime
  --agent <agent-name>     (required)      Agent that produced the event
  --type <event-type>      (required)      Event type (tool_start, tool_end, spawn, session_end, error, custom)
  --level <level>                          Level: debug, info (default), warn, error
  --data <text>                            Freeform event data string
  --session <session-id>                   Link to a registered session
  --runtime <runtime-id>                   Runtime that produced the event (for filtering)
```

### Eval Emission (new subcommand on existing `cmdr evals`)

```
cmdr evals emit                            Emit an eval result from any runtime
  --project <name>         (required)      Project name
  --task <description>                     Agent task / objective being evaluated
  --type <eval-type>                       Eval type: unit_test, integration, lint, build, custom, semantic_check, structural_check, etc.
  --passed <true|false>                    Pass/fail result
  --detail <text>                          Error detail or evidence string
  --id <eval-id>                           Eval ID (auto-generated if omitted; upserts on conflict)
```

Note: `cmdr evals emit` already exists but is only called by Claude's intent hooks. The change here is documenting it as a public API for all runtimes and ensuring the wrapper scripts call it.

### Status (modified)

```
cmdr status                                Fleet overview (all runtimes)
  --runtime <runtime-id>                   Filter by runtime
  --capability <cap>                       Filter by capability
  --state <state>                          Filter by state
  --pane                                   Dashboard pane mode (ANSI)
  --json                                   JSON output
```

### OpenBrain (modified)

```
cmdr openbrain                             Memory + agent activity feed
  --pane                                   Dashboard pane mode (watch + stream ANSI)
  --project <dir>                          Override project directory
  --agents                                 Include agent lifecycle events (default: true in --pane)
  --no-agents                              Suppress agent lifecycle events
  --agent-limit <n>                        Max recent agent events to display (default: 5)
```

## JSON Output Format

Success (register):

```json
{
  "success": true,
  "command": "register",
  "session_id": "pi-a3f8c1e2",
  "agent_name": "unix-coder",
  "runtime": "pi",
  "state": "booting"
}
```

Success (deregister):

```json
{
  "success": true,
  "command": "deregister",
  "session_id": "pi-a3f8c1e2",
  "final_state": "completed"
}
```

Success (heartbeat):

```json
{
  "success": true,
  "command": "heartbeat",
  "session_id": "pi-a3f8c1e2",
  "heartbeat_at": "2026-03-17T14:32:01Z",
  "state": "working"
}
```

Error:

```json
{
  "success": false,
  "command": "register",
  "error": "session pi-a3f8c1e2 not found"
}
```

List (status with --runtime filter):

```json
{
  "success": true,
  "command": "status",
  "sessions": [...],
  "count": 4,
  "by_runtime": { "claude": 2, "pi": 1, "gemini": 1 }
}
```

## Concurrency Model

SQLite Advisory Locking (existing pattern)

```
Lock:        SQLite busy_timeout=5000 (already configured)
Stale after: N/A (SQLite handles internally)
Retry:       Built into busy_timeout
Timeout:     5 seconds
```

Implementation:

1. `cmdr register` uses a single `INSERT INTO sessions` statement -- atomic by SQLite.
2. `cmdr heartbeat` uses `UPDATE sessions SET heartbeat_at = ?, last_activity = ? WHERE id = ?` -- single row update, atomic.
3. `cmdr deregister` uses `UPDATE sessions SET state = ? WHERE id = ?` -- single row update, atomic.
4. Multiple agents calling register/heartbeat/deregister concurrently are serialized by SQLite's WAL mode + busy_timeout.

### Atomic Writes

All operations are single SQL statements. No multi-statement transactions needed for registration commands.

### Conflict Resolution

Last-write-wins for heartbeat updates. If two heartbeats arrive for the same session within the same second, the later one wins. This is acceptable because heartbeat is an activity signal, not a data merge.

## Migration

| Component | Current (Claude-only) | Target (Multi-agent) |
|-----------|----------------------|----------------------|
| Agent registration | cmdr-bridge.sh (Claude hooks only) | cmdr-bridge.sh (Claude) + `cmdr register` (all runtimes) |
| Agent counting | `pgrep -x claude` in agent-counter.sh | `cmdr status --json` DB query in cmdr-agent-counter.sh |
| Liveness detection | `last_activity` via PostToolUse hook | `last_activity` (Claude) + `heartbeat_at` (non-Claude) |
| OpenBrain content | MEMORY.md file diffs only | MEMORY.md diffs + agent lifecycle events from `events` table |
| Status filtering | No runtime filter | `--runtime` flag for per-runtime filtering |

No data migration needed. Existing sessions continue to work. The new `heartbeat_at` column defaults to NULL, which signals "this agent uses hook-based tracking."

## Integration

### Pi.dev Wrapper Script

Pi.dev agents call `cmdr register` at startup and `cmdr deregister` at exit. A heartbeat loop runs in the background.

```bash
# pi-agent-wrapper.sh (example integration)
SESSION_ID=$(cmdr register --name "$AGENT_NAME" --runtime pi \
  --capability builder --task "$TASK_ID" --pid $$ --json | jq -r '.session_id')

# Background heartbeat every 60 seconds
while kill -0 $$ 2>/dev/null; do
  cmdr heartbeat "$SESSION_ID" --state working
  sleep 60
done &
HEARTBEAT_PID=$!

# Emit events as the agent works
cmdr feed emit --agent "$AGENT_NAME" --type tool_start --data "tool=Edit file=main.go" --session "$SESSION_ID" --runtime pi

# Run the actual pi agent
pi-agent run --task "$TASK_ID"
EXIT_CODE=$?

# Emit eval results from the agent's test runs
cmdr evals emit --project myapp --type build --passed true --detail "Build succeeded"
cmdr evals emit --project myapp --type unit_test --passed true --detail "42/42 tests passed"

# Cleanup
kill $HEARTBEAT_PID 2>/dev/null
cmdr deregister "$SESSION_ID" --state completed
```

### Claude Code Hooks (unchanged)

cmdr-bridge.sh continues to call `INSERT INTO sessions` directly via sqlite3. No changes needed. The bridge already sets `runtime = 'claude'`.

| Bridge Hook | Equivalent CLI (for other runtimes) |
|-------------|-------------------------------------|
| SubagentStart (INSERT) | `cmdr register --name X --runtime pi ...` |
| SubagentStop (UPDATE state) | `cmdr deregister <session-id>` |
| PostToolUse (UPDATE last_activity) | `cmdr heartbeat <session-id>` |

### Gateway API (extended)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `POST /api/v1/agents/register` | POST | Same as `cmdr register` |
| `POST /api/v1/agents/{id}/heartbeat` | POST | Same as `cmdr heartbeat` |
| `DELETE /api/v1/agents/{id}` | DELETE | Same as `cmdr deregister` (existing) |

### Agent-Facing Commands

```bash
# Pi.dev agent startup
cmdr register --name pi-coder --runtime pi --capability builder --task PROJ-42 --pid $$

# Periodic heartbeat (every 60s)
cmdr heartbeat pi-a3f8c1e2 --state working --tokens-in 1500 --tokens-out 300

# Agent exit
cmdr deregister pi-a3f8c1e2 --state completed
```

### OpenBrain Dashboard Integration

The OpenBrain pane in the dashboard renders two sections. The Activity section shows at most 5 recent agent lifecycle events (configurable via `--agent-limit`). The `runOpenBrainPane` function signature must be refactored from `runOpenBrainPane(ctx, projectDir)` to `runOpenBrainPane(ctx, app, projectDir)` so it can access the DB for agent event queries via `*App`.

```
+---------------------------------------------------+
| Memory                           14:32:05 (3 files)|
| modified memory/MEMORY.md  ## Agent Tracking       |
|          Added heartbeat mechanism for pi.dev a..  |
| added    memory/feedback_pi.md  ## Pi Integration  |
|          Pi.dev agents now register via cmdr re..  |
|---------------------------------------------------|
| Activity                                    (4/5)  |
|  registered  pi-coder       pi     builder  14:31 |
|  working     unix-coder     claude builder  14:30 |
|  completed   code-review    claude reviewer 14:28 |
|  stalled     gemini-scout   gemini scout    14:25 |
+---------------------------------------------------+
```

### Event Feed Pane Integration

The event feed pane (`cmdr feed --pane`) already streams events from the `events` table. With multi-agent tracking:

1. `cmdr register` emits an `agent.registered` event with `data: "runtime=pi capability=builder"`
2. `cmdr deregister` emits an `agent.deregistered` event
3. `cmdr heartbeat` emits an `agent.heartbeat` event (throttled: max 1 event per 5 min per agent to avoid noise)
4. `cmdr feed emit` allows any runtime wrapper to emit arbitrary events

The feed pane already renders events with agent color coding via `colorResolver`. No rendering changes needed — just more events flowing in from non-Claude runtimes.

```
+---------------------------------------------------+
| -- Events (12) --                                  |
| 14:32:01 pi-coder     tool_end     tool=Edit      |
| 14:31:45 unix-coder   tool_end     tool=Bash      |
| 14:31:30 pi-coder     agent.reg..  runtime=pi     |
| 14:31:12 gemini-scout  spawn       capability=scout|
| 14:30:58 code-review  session_end  completed      |
+---------------------------------------------------+
```

### Evals Pane Integration

The evals pane (`cmdr evals --pane`) shows eval pass/fail results. With multi-agent tracking:

1. Non-Claude runtimes call `cmdr evals emit` to record eval results
2. Pi.dev wrapper scripts can run evals and emit results: `cmdr evals emit --project myapp --type build --passed true`
3. The evals pane already polls the DB and renders all evals regardless of source — no rendering changes needed

The key change is making `cmdr evals emit` a documented public API (it already exists but is only used internally by `cmdr-bridge.sh record-eval`).

```
+---------------------------------------------------+
| Evals                                              |
|  PASS unit_test    eval-a3f8     Go tests pass    |
|  FAIL integration  eval-b7d9     API timeout      |
|  PASS build        eval-pi-01    Pi agent build ok|
|  ---  semantic     eval-h-c4e2   Awaiting result  |
+---------------------------------------------------+
```

## What It Does NOT Do

- **Runtime process management.** cmdr does not start/stop pi.dev or gemini processes. It only tracks their lifecycle. The wrapper script is responsible for process management.
- **Cross-runtime communication.** The mail system already handles inter-agent messaging. This spec does not add runtime-aware message routing.
- **Automatic runtime detection.** Agents must explicitly declare their runtime via `--runtime`. cmdr does not auto-detect what runtime a process belongs to.
- **Hook installation for non-Claude runtimes.** This spec does not create hook systems for pi.dev, gemini, etc. Those runtimes use `cmdr register` / `cmdr heartbeat` instead.
- **cmdr-bridge.sh refactoring.** The existing bridge is left as-is. Future work may consolidate it to use `cmdr register` internally, but that is out of scope.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing project runtime |
| CLI framework | Cobra | Existing project convention |
| Database | SQLite (modernc.org/sqlite) | Existing storage layer; dual PostgreSQL support via migration |
| Agent counter | Shell + `cmdr status --json` + jq | Replaces pgrep-based counting with DB-backed query |
| Testing | Go `testing` package | Existing test infrastructure |
| JSON output | `encoding/json` | Existing project convention for `--json` flag |

## Project Infrastructure

### Directory Structure (new/modified files only)

```
computeCommander/
  internal/
    platform/db/
      migrations/
        sqlite/
          008_multi_agent.sql              # new migration
        postgres/
          008_multi_agent.sql              # new migration (postgres dialect)
    agents/
      types.go                             # modified: add Runtime to ListOpts
      spawner.go                           # modified: add runtime WHERE clause to ListSessions
    commands/
      register.go                          # new: register/deregister/heartbeat commands
      openbrain.go                         # modified: agent activity feed
      status.go                            # modified: --runtime filter, DB-backed counting
      observability.go                     # modified: restructure FeedCmd, add feed emit
    gateway/
      gateway.go                           # modified: register + heartbeat endpoints
  scripts/
    cmdr-agent-counter.sh                  # new: DB-backed universal counter
```

### Version Management

No version bump. This is a feature addition within the current development cycle.

### CHANGELOG.md

Entry under `## [Unreleased]`:
```
### Added
- `cmdr register` / `cmdr deregister` / `cmdr heartbeat` CLI commands for runtime-agnostic agent tracking
- OpenBrain pane now shows agent lifecycle events alongside memory changes
- `cmdr status --runtime` filter flag
- `cmdr feed emit` subcommand for multi-runtime event emission
- DB migration 008: heartbeat_at column and runtime index on sessions table
- DB-backed universal agent counter (replaces pgrep-based counting)
```

### Scripts

```json
{
  "scripts": {
    "build": "go build -o cmdr ./cmd/cc",
    "test": "go test ./...",
    "install": "go install ./cmd/cc && cp $(which cmdr) ~/.local/bin/cmdr"
  }
}
```

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| CLI commands (register.go) | 1 | ~250 |
| DB migrations (008 x2) | 2 | ~20 |
| OpenBrain modifications | 1 | ~120 (new lines) |
| Status modifications | 1 | ~40 (new lines) |
| Agent types/spawner modifications | 2 | ~25 (new lines) |
| Gateway modifications | 1 | ~80 (new lines) |
| Event feed emit (observability.go) | 1 | ~80 (new lines, includes FeedCmd restructuring) |
| Evals emit extension (evals.go) | 1 | ~20 (new lines) |
| Register event emission | 0 | ~30 (in register.go) |
| Agent counter script | 1 | ~15 |
| Tests | 2 | ~250 |
| **Total** | **14** | **~930** |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Create DB migration 008_multi_agent.sql for SQLite and PostgreSQL | internal/platform/db/migrations/sqlite/007_jira_parent_key.sql, internal/platform/db/migrations/postgres/007_jira_parent_key.sql | internal/platform/db/migrations/sqlite/008_multi_agent.sql, internal/platform/db/migrations/postgres/008_multi_agent.sql | -- | `sqlite3 :memory: '.read internal/platform/db/migrations/sqlite/001_schema.sql' '.read internal/platform/db/migrations/sqlite/002_system_wide.sql' '.read internal/platform/db/migrations/sqlite/003_agentic_foundation.sql' '.read internal/platform/db/migrations/sqlite/004_evals.sql' '.read internal/platform/db/migrations/sqlite/005_jira_cache.sql' '.read internal/platform/db/migrations/sqlite/006_jira_prompt_log.sql' '.read internal/platform/db/migrations/sqlite/007_jira_parent_key.sql' '.read internal/platform/db/migrations/sqlite/008_multi_agent.sql' '.schema sessions' \| grep -q heartbeat_at` |
| T2 | unix-coder | Implement register.go with register, deregister, heartbeat subcommands | internal/commands/app.go, internal/agents/types.go, internal/platform/db/db.go | internal/commands/register.go | T1 | `go build ./cmd/cc && ./cmdr register --help && ./cmdr deregister --help && ./cmdr heartbeat --help` |
| T3 | unix-coder | Wire register/deregister/heartbeat commands into Cobra command tree | cmd/cc/main.go, internal/commands/register.go | cmd/cc/main.go | T2 | `go build ./cmd/cc && ./cmdr register --help 2>&1 \| grep -q 'runtime'` |
| T4 | unix-coder | Add --runtime filter to status command and DB-backed counting. Add `Runtime` field to `ListOpts` in types.go, add runtime WHERE clause to `ListSessions` in spawner.go, wire `--runtime` flag in status.go | internal/commands/status.go, internal/agents/spawner.go, internal/agents/types.go | internal/commands/status.go, internal/agents/types.go, internal/agents/spawner.go | T1 | `go build ./cmd/cc && ./cmdr status --help 2>&1 \| grep -q 'runtime'` |
| T5 | unix-coder | Add agent lifecycle events to OpenBrain pane. Refactor `runOpenBrainPane(ctx, projectDir)` to `runOpenBrainPane(ctx, app, projectDir)` to accept `*App` for DB access. Query up to 5 recent agent lifecycle events from events table and render an Activity section below the memory feed. Note: functional testing of live agent events depends on T10 (event emission); T8 unit tests should use pre-seeded DB rows | internal/commands/openbrain.go, internal/platform/db/db.go | internal/commands/openbrain.go | T1 | `go build ./cmd/cc && ./cmdr openbrain --help 2>&1 \| grep -qE 'agents\|no-agents'` |
| T6 | unix-coder | Add register and heartbeat gateway endpoints | internal/gateway/gateway.go | internal/gateway/gateway.go | T2 | `go build ./cmd/cc && grep -q 'handleRegisterAgent\|handleHeartbeat' internal/gateway/gateway.go` |
| T7 | unix-coder | Create DB-backed cmdr-agent-counter.sh | -- | scripts/cmdr-agent-counter.sh | T4 | `test -x scripts/cmdr-agent-counter.sh && head -1 scripts/cmdr-agent-counter.sh \| grep -q '#!/bin/sh'` |
| T8 | unix-coder | Write tests for register, deregister, heartbeat, and OpenBrain agent feed. Use pre-seeded DB rows for OpenBrain tests (agent events depend on T10 at runtime, but tests insert rows directly) | internal/commands/register.go, internal/commands/openbrain.go | internal/commands/register_test.go, internal/commands/openbrain_test.go | T2, T5 | `go test ./internal/commands/ -run 'TestRegister\|TestDeregister\|TestHeartbeat\|TestOpenBrainAgent' -v` |
| T9 | unix-coder | Restructure FeedCmd to support subcommands. Currently FeedCmd is a leaf command with RunE; refactor it into a parent command (with default RunE for backwards-compatible `cmdr feed` and `cmdr feed --pane` behavior) and add `feed emit` as a child subcommand for multi-runtime event emission | internal/commands/observability.go | internal/commands/observability.go | T1 | `go build ./cmd/cc && ./cmdr feed emit --help 2>&1 \| grep -q 'agent'` |
| T10 | unix-coder | Update register.go to emit events on register/deregister/heartbeat into events table | internal/commands/register.go, internal/commands/observability.go | internal/commands/register.go | T2, T9 | `grep -q 'agent.registered\|agent.deregistered\|agent.heartbeat' internal/commands/register.go` |
| T11 | unix-coder | Document and extend evals emit as public API -- add --runtime flag, update help text | internal/commands/evals.go | internal/commands/evals.go | T1 | `go build ./cmd/cc && ./cmdr evals emit --help 2>&1 \| grep -q 'project'` |
| T12 | code-review | Review all new code for correctness, DRY, and backwards compatibility | All T1-T11 write targets | -- | T11 | `go vet ./... && go build ./cmd/cc` |

## 16. Dependency Graph

```
Phase 1 (single): [T1]
  T1: DB migration 008_multi_agent.sql

Phase 2 (parallel, after Phase 1): [T2, T4, T5, T9, T11]
  T2: register.go CLI commands
  T4: status --runtime filter + DB counting + types.go/spawner.go modifications
  T5: OpenBrain agent activity feed (refactor runOpenBrainPane signature)
  T9: feed emit subcommand (restructure FeedCmd for subcommands)
  T11: evals emit public API extension

Phase 3 (after T2, T9): [T3, T6, T10]
  T3: Wire commands into Cobra tree
  T6: Gateway endpoints
  T10: register/deregister/heartbeat emit events into events table

Phase 4 (parallel, after Phase 2+3): [T7, T8]
  T7: DB-backed agent counter script
  T8: Tests (unit tests use pre-seeded DB; full integration depends on T10)

Final: [T12] -- code review
```

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/platform/db/migrations/sqlite/008_multi_agent.sql` | ~10 | No |
| `internal/platform/db/migrations/postgres/008_multi_agent.sql` | ~10 | No |
| `internal/commands/register.go` | ~250 | No |
| `internal/commands/register_test.go` | ~120 | No |
| `scripts/cmdr-agent-counter.sh` | ~15 | Yes |

Files modified:

- `cmd/cc/main.go` -- add RegisterCmd, DeregisterCmd, HeartbeatCmd to command tree
- `internal/agents/types.go` -- add `Runtime runtimes.RuntimeID` field to `ListOpts`
- `internal/agents/spawner.go` -- add runtime WHERE clause to `ListSessions` query
- `internal/commands/status.go` -- add `--runtime` flag, DB-backed agent count output
- `internal/commands/openbrain.go` -- refactor `runOpenBrainPane` to accept `*App`, add agent lifecycle event feed section
- `internal/commands/openbrain_test.go` -- add tests for agent feed rendering
- `internal/commands/observability.go` -- restructure `FeedCmd` from leaf command to parent+subcommand, add `feed emit` subcommand
- `internal/commands/evals.go` -- extend `evals emit` as public API, add `--runtime` flag
- `internal/gateway/gateway.go` -- add register + heartbeat route handlers

Files deleted: None

## 18. Verification Plan

**Per-task checks:**

- T1: `sqlite3 :memory: '.read internal/platform/db/migrations/sqlite/001_schema.sql' '.read internal/platform/db/migrations/sqlite/002_system_wide.sql' '.read internal/platform/db/migrations/sqlite/003_agentic_foundation.sql' '.read internal/platform/db/migrations/sqlite/004_evals.sql' '.read internal/platform/db/migrations/sqlite/005_jira_cache.sql' '.read internal/platform/db/migrations/sqlite/006_jira_prompt_log.sql' '.read internal/platform/db/migrations/sqlite/007_jira_parent_key.sql' '.read internal/platform/db/migrations/sqlite/008_multi_agent.sql' '.schema sessions' | grep -q heartbeat_at`
- T2: `go build ./cmd/cc && ./cmdr register --help`
- T3: `go build ./cmd/cc && ./cmdr register --help 2>&1 | grep -q 'runtime'`
- T4: `go build ./cmd/cc && ./cmdr status --help 2>&1 | grep -q 'runtime'`
- T5: `go build ./cmd/cc && ./cmdr openbrain --help 2>&1 | grep -qE 'agents|no-agents'`
- T6: `grep -q 'handleRegisterAgent' internal/gateway/gateway.go`
- T7: `test -x scripts/cmdr-agent-counter.sh`
- T8: `go test ./internal/commands/ -run 'TestRegister|TestDeregister|TestHeartbeat|TestOpenBrainAgent' -v`
- T9: `go build ./cmd/cc && ./cmdr feed emit --help 2>&1 | grep -q 'agent'`
- T10: `grep -q 'agent.registered' internal/commands/register.go`
- T11: `go build ./cmd/cc && ./cmdr evals emit --help 2>&1 | grep -q 'project'`
- T12: `go vet ./... && go build ./cmd/cc`

**Integration check:**

```bash
# End-to-end: register a fake pi agent, heartbeat, check status, deregister
go build -o cmdr ./cmd/cc
./cmdr init 2>/dev/null || true
SESSION_ID=$(./cmdr register --name test-pi --runtime pi --capability builder --task TEST-001 --json | jq -r '.session_id')
./cmdr heartbeat "$SESSION_ID" --state working --json | jq -e '.success == true'
./cmdr status --runtime pi --json | jq -e '.count >= 1'
./cmdr deregister "$SESSION_ID" --json | jq -e '.success == true'
./cmdr status --runtime pi --json | jq -e '[.sessions[] | select(.state == "working")] | length == 0'
```

**Rollback:** `git checkout -- internal/ cmd/ scripts/ && rm -f internal/platform/db/migrations/sqlite/008_multi_agent.sql internal/platform/db/migrations/postgres/008_multi_agent.sql internal/commands/register.go internal/commands/register_test.go scripts/cmdr-agent-counter.sh`

### Functional Smoke Tests

#### TUI Smoke Tests

**OpenBrain pane launch (with agent events):**

```bash
timeout 3s cmdr openbrain --pane 2>&1 | head -20
test $? -eq 124 -o $? -eq 0
```

**OpenBrain pane renders Activity section:**

```bash
# Register a test agent, then verify openbrain shows it
SESSION_ID=$(cmdr register --name smoke-test --runtime pi --capability builder --task SMOKE-1 --json | jq -r '.session_id')
timeout 3s cmdr openbrain --pane 2>&1 | grep -q 'Activity\|registered'
cmdr deregister "$SESSION_ID" 2>/dev/null
```

#### Binary Install Verification

```bash
go build -o cmdr ./cmd/cc
./cmdr register --help | grep -q 'runtime'
./cmdr heartbeat --help | grep -q 'session-id'
./cmdr deregister --help | grep -q 'session-id'
```

## 19. Success Criteria (Machine-Verifiable)

- [ ] `sqlite3 :memory: '.read internal/platform/db/migrations/sqlite/001_schema.sql' '.read internal/platform/db/migrations/sqlite/002_system_wide.sql' '.read internal/platform/db/migrations/sqlite/003_agentic_foundation.sql' '.read internal/platform/db/migrations/sqlite/004_evals.sql' '.read internal/platform/db/migrations/sqlite/005_jira_cache.sql' '.read internal/platform/db/migrations/sqlite/006_jira_prompt_log.sql' '.read internal/platform/db/migrations/sqlite/007_jira_parent_key.sql' '.read internal/platform/db/migrations/sqlite/008_multi_agent.sql' '.schema sessions' | grep -q heartbeat_at` -- migration adds heartbeat_at column
- [ ] `go build -o cmdr ./cmd/cc` exits 0 -- project compiles
- [ ] `go test ./...` exits 0 -- all tests pass
- [ ] `go vet ./...` exits 0 -- no vet errors
- [ ] `go build -o cmdr ./cmd/cc && ./cmdr init 2>/dev/null; SID=$(./cmdr register --name test --runtime pi --capability builder --task T1 --json | jq -r '.session_id') && ./cmdr heartbeat "$SID" --json | jq -e '.success == true' && ./cmdr deregister "$SID" --json | jq -e '.success == true'` -- register/heartbeat/deregister pipeline works end-to-end
- [ ] `./cmdr status --runtime pi --json | jq -e '.sessions'` -- runtime filter works
- [ ] `./cmdr openbrain --help 2>&1 | grep -q 'agents'` -- openbrain has agents flag
- [ ] `grep -q 'handleRegisterAgent' internal/gateway/gateway.go` -- gateway has register endpoint
- [ ] `./cmdr feed emit --help 2>&1 | grep -q 'agent'` -- feed emit subcommand exists
- [ ] `./cmdr evals emit --help 2>&1 | grep -q 'project'` -- evals emit is documented as public API
- [ ] `grep -q 'agent.registered' internal/commands/register.go` -- register emits events
- [ ] `test -f internal/commands/register.go` -- register.go exists
- [ ] `test -f internal/commands/register_test.go` -- tests exist
- [ ] `test -x scripts/cmdr-agent-counter.sh` -- counter script is executable
- [ ] `grep -q 'Runtime' internal/agents/types.go` -- ListOpts has Runtime field
- [ ] `grep -q 's.runtime' internal/agents/spawner.go` -- ListSessions filters by runtime

### Functional Smoke Criteria

- [ ] `timeout 3s cmdr openbrain --pane; test $? -eq 124 -o $? -eq 0` -- OpenBrain pane launches without crash
- [ ] `go build -o cmdr ./cmd/cc && ./cmdr register --help | grep -q 'runtime'` -- register command has runtime flag
- [ ] `grep -q 'heartbeat' internal/gateway/gateway.go` -- gateway routes include heartbeat

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| DB migration, CLI commands, gateway endpoints, counter script, event/eval emission, types/spawner modifications | `unix-coder` | Implementation work: new files, SQL, Go code |
| Code quality review | `code-review` | Verify backwards compatibility, DRY, correctness |

## Execution Order

```
Phase 1: Schema
  +-- T1: DB migration 008 (agent: unix-coder)

Phase 2: Core Commands [blocked by Phase 1]
  +-- T2: register.go (agent: unix-coder)
  +-- T4: status --runtime + types.go + spawner.go (agent: unix-coder) [parallel]
  +-- T5: OpenBrain agent feed (agent: unix-coder)                     [parallel]
  +-- T9: feed emit subcommand / FeedCmd restructure (agent: unix-coder) [parallel]
  +-- T11: evals emit public API (agent: unix-coder)                   [parallel]

Phase 3: Wiring [blocked by T2, T9]
  +-- T3: Cobra tree wiring (agent: unix-coder)
  +-- T6: Gateway endpoints (agent: unix-coder)         [parallel]
  +-- T10: register emits events (agent: unix-coder)    [parallel]

Phase 4: Polish [blocked by Phase 2+3]
  +-- T7: Agent counter script (agent: unix-coder)
  +-- T8: Tests (agent: unix-coder)                     [parallel]

Phase 5: Review [blocked by Phase 4]
  +-- T12: Code review (agent: code-review)
```

Recommended directive: `/pai` -- plan-then-implement pipeline. Feature is medium complexity with clear sequential phases.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Migration 008 fails on existing DB with data | `cmdr init` returns error | `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` syntax handles idempotence |
| cmdr-bridge.sh breaks after schema change | Agent pane shows no Claude agents | Schema change is additive (new column, new index) -- no existing columns changed |
| Heartbeat storms from many agents | High SQLite WAL file growth | busy_timeout=5000 serializes writes; heartbeat interval is 60s per agent |
| OpenBrain pane becomes noisy with agent events | Visual clutter in small terminal | `--no-agents` flag suppresses agent events; default shows max 5 recent events; configurable via `--agent-limit` |
| Agent counter script fails (jq missing) | dwmblocks shows stale count | Script falls back to `echo 0` on any error; jq is a common dependency |
| Non-Claude agent wrapper forgets to deregister | Zombie sessions accumulate | Existing watchdog staleness reaper (10 min) + heartbeat_at staleness (5 min) will mark them stalled/zombie |
| FeedCmd restructuring breaks existing `cmdr feed` / `cmdr feed --pane` | `cmdr feed --pane` no longer streams events | FeedCmd must preserve default RunE behavior for backwards compatibility; the restructuring moves existing RunE to the parent command's RunE (or PersistentPreRunE) so bare `cmdr feed` and `cmdr feed --pane` continue working exactly as before |

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 1 | Should `cmdr register` generate session IDs or accept them from the caller? | Affects wrapper script design | Generate server-side: `{runtime}-{8 hex chars}` (e.g., `pi-a3f8c1e2`). Caller receives ID in JSON response. |
| 2 | Should the heartbeat interval be configurable or hardcoded at 60s? | Affects agent wrapper templates | Hardcode 60s recommendation in docs; the interval is the wrapper's responsibility, not cmdr's. |
| 3 | Should `cmdr-agent-counter.sh` replace `agent-counter.sh` in Claude hooks, or coexist? | Affects hook configuration | Coexist initially. Update Claude hooks to use the new script in a follow-up PR. |
