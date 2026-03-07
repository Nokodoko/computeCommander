# ComputeCommander Evals Pane

Add an "Evals" pane to the bottom row of the cmdr dashboard. The pane lists eval definitions with their latest pass/fail results, supports running all evals and adding new ones via key bindings. Persists eval data in the existing SQLite/PostgreSQL database via a new migration. Integrates into both the Zellij KDL layout and the BubbleTea TUI.

## Why

- **No visibility into eval outcomes.** Agents complete tasks but there is no dashboard surface to see whether the resulting code passes project evals. Users must leave the dashboard to run tests manually.
- **No centralized eval registry.** Eval definitions are scattered across Makefiles, CI configs, and ad-hoc scripts. A single `evals` table with a CLI interface gives agents and humans a unified way to register, run, and inspect evals.
- **Bottom row has room.** The current 4-pane bottom row (Event Log, Mail, Merge Queue, LazyGit) leaves space for a 5th pane at 20% width each without visual degradation.

This spec covers exactly the surface needed: one new DB table, one new TUI component, one new CLI command, and layout adjustments to both KDL and BubbleTea renderers.

## Design Principles

1. **Follow existing patterns exactly.** The new Evals pane mirrors `MergeQueueView` / `MailSummary` in structure: a Go struct with `View()`, `Refresh()`, `SetSize()`, registered in `Dashboard`, wired via `NewDashboard()`, rendered in `View()`.
2. **Dual-mode parity.** Every change applies to both the KDL layout (zellij panes running `cmdr evals --pane`) and the BubbleTea TUI (in-process `EvalsPane` component). Both modes show the same data.
3. **Database-backed persistence.** Eval definitions and results live in an `evals` table created by migration `004_evals.sql`. Both SQLite and PostgreSQL dialects are provided.
4. **Key bindings are contextual.** `r` (run all) and `a` (add eval) only fire when the Evals pane is focused. They are displayed in the pane footer as visual hints.
5. **Non-breaking addition.** No existing files are removed. No existing behavior changes. The bottom row width calculation shifts from `w/4` to `w/5` and pane numbering extends from 1-7 to 1-8.
6. **Color-coordinated eval types.** Each eval type has a distinct color matching the Dracula palette used by the intent eval hook dunst notifications (`~/.claude/hooks/intent/eval_loop.py:PREDICATE_NOTIFICATION_COLORS`). Pass/fail status colors match the intent-eval-posttool.py and intent-build-verify.py hooks exactly.

## Color Scheme

Eval type colors are drawn from the existing hook dunst notification palette to ensure visual consistency between desktop notifications and the dashboard Evals pane.

### Eval Type Colors (Dracula Palette)

| Eval Type | Hex | ANSI | Matches Hook Predicate |
|-----------|-----|------|------------------------|
| `unit_test` | `#8be9fd` | Cyan | `test_execution` |
| `integration` | `#bd93f9` | Purple | `output_pattern_match` |
| `lint` | `#f1fa8c` | Yellow | `diff_validation` |
| `build` | `#50fa7b` | Green | `structural_check` / build-verify pass |
| `custom` | `#66d9ef` | Blue-Cyan | `type_check` |

### Status Colors

| Status | Hex | ANSI | Matches Hook |
|--------|-----|------|--------------|
| PASS | `#50fa7b` | Green | `intent-eval-posttool.py` pass / `intent-build-verify.py` pass |
| FAIL | `#ff5555` | Red | `intent-eval-posttool.py` fail (fg), `intent-build-verify.py` fail |
| NEVER RUN | `#6272a4` | Gray | Dracula comment color (neutral) |

### Background

All hook notifications use `#1a1a2e` (dark navy) as background. The Evals pane uses the existing `theme.go` dark background which is visually equivalent.

### Go Implementation

In `internal/tui/evals_pane.go`, define lipgloss styles:

```go
var evalTypeStyles = map[string]lipgloss.Style{
    "unit_test":    lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")),
    "integration":  lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")),
    "lint":         lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")),
    "build":        lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
    "custom":       lipgloss.NewStyle().Foreground(lipgloss.Color("#66d9ef")),
}

var (
    evalPassStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Bold(true)
    evalFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)
    evalPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))
)
```

In `--pane` mode (`internal/commands/evals.go`), use ANSI escape codes for the same colors:

```go
var evalTypeANSI = map[string]string{
    "unit_test":    "\033[38;2;139;233;253m",  // #8be9fd cyan
    "integration":  "\033[38;2;189;147;249m",  // #bd93f9 purple
    "lint":         "\033[38;2;241;250;140m",  // #f1fa8c yellow
    "build":        "\033[38;2;80;250;123m",   // #50fa7b green
    "custom":       "\033[38;2;102;217;239m",  // #66d9ef blue-cyan
}

const (
    ansiPass    = "\033[38;2;80;250;123m\033[1m"   // #50fa7b bold
    ansiFail    = "\033[38;2;255;85;85m\033[1m"    // #ff5555 bold
    ansiPending = "\033[38;2;98;114;164m"          // #6272a4
    ansiReset   = "\033[0m"
)
```

## On-Disk Format

```
computeCommander/
  internal/
    tui/
      evals_pane.go                          # New: EvalsPane TUI component
      pane.go                                # Modified: add PaneEvals constant
      dashboard.go                           # Modified: wire EvalsPane into dashboard
      render.go                              # Modified: update help bar 1-8
    commands/
      evals.go                               # New: EvalsCmd CLI handler
    platform/db/migrations/
      sqlite/004_evals.sql                   # New: evals table migration (SQLite)
      postgres/004_evals.sql                 # New: evals table migration (Postgres)
  internal/zellij/
    layout.go                                # Modified: add Evals pane to bottom row
  cmd/cc/
    main.go                                  # Modified: wire EvalsCmd
```

### `internal/platform/db/migrations/sqlite/004_evals.sql`

SQLite migration for the evals table. Uses the project's standard `CREATE TABLE IF NOT EXISTS` pattern with `TEXT` timestamps and `INTEGER` booleans.

```sql
-- ComputeCommander SQLite schema
-- Migration 004: Evals table

CREATE TABLE IF NOT EXISTS evals (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    agent_task TEXT NOT NULL DEFAULT '',
    eval_type TEXT NOT NULL DEFAULT 'custom'
        CHECK (eval_type IN ('unit_test', 'integration', 'lint', 'build', 'custom')),
    command TEXT NOT NULL,
    passed INTEGER,
    error_detail TEXT,
    last_run_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_evals_project ON evals(project_name);
CREATE INDEX IF NOT EXISTS idx_evals_type ON evals(eval_type);
```

### `internal/platform/db/migrations/postgres/004_evals.sql`

```sql
-- ComputeCommander PostgreSQL schema
-- Migration 004: Evals table

CREATE TABLE IF NOT EXISTS evals (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    agent_task TEXT NOT NULL DEFAULT '',
    eval_type TEXT NOT NULL DEFAULT 'custom'
        CHECK (eval_type IN ('unit_test', 'integration', 'lint', 'build', 'custom')),
    command TEXT NOT NULL,
    passed BOOLEAN,
    error_detail TEXT,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evals_project ON evals(project_name);
CREATE INDEX IF NOT EXISTS idx_evals_type ON evals(eval_type);
```

### `internal/tui/evals_pane.go`

New file. Follows the `MergeQueueView` / `MailSummary` pattern: struct with `View() string`, `Refresh() error`, `SetSize(w, h int)`. Queries the `evals` table via the `db.DB` interface. Renders a table with columns: Project, Task, Type, Result. Footer shows `[r] Run All  [a] Add Eval`.

### `internal/commands/evals.go`

New file. Exports `EvalsCmd(app *App) *cobra.Command`. Subcommands:
- `cmdr evals` (default: list all evals)
- `cmdr evals add --project X --task Y --type Z --command "cmd"`
- `cmdr evals run` (run all evals sequentially)
- `cmdr evals remove <id>`
- `cmdr evals --pane` (long-lived pane mode for KDL layout)

Follows the `GitStatusCmd` / `FeedCmd` pattern for `--pane` mode with a ticker loop.

## Data Model

### Eval

```typescript
interface Eval {
  // Identity
  id: string;           // "eval-a1b2c3d4" -- 8-char hex prefixed with "eval-"
  project_name: string; // e.g., "computeCommander"

  // Task context
  agent_task: string;   // e.g., "implement evals pane" -- what agent was doing (optional)
  eval_type: EvalType;  // enum: unit_test | integration | lint | build | custom

  // Execution
  command: string;      // shell command to run, e.g., "go test ./..."
  passed: boolean | null; // null = never run, true = pass, false = fail
  error_detail?: string;  // stderr or error message on failure

  // Timestamps
  last_run_at?: string; // ISO 8601, set when eval is executed
  created_at: string;   // ISO 8601, set on creation
}
```

### ID Generation

- Format: `eval-{8 hex chars}` (e.g., `eval-a1b2c3d4`)
- Generated via `crypto/rand.Read(4)` + `fmt.Sprintf("eval-%x", b)` -- same pattern as `generateTabHash()` in `layout.go`
- Collision probability negligible for expected eval counts (<1000 per project)

### Eval Type Enum

| Value | Label | Use |
|-------|-------|-----|
| `unit_test` | Unit Test | `go test`, `pytest`, `jest` style test suites |
| `integration` | Integration | End-to-end or multi-service tests |
| `lint` | Lint | `golangci-lint run`, `eslint`, `ruff check` |
| `build` | Build | `go build`, `make build`, `tsc` compilation checks |
| `custom` | Custom | Arbitrary shell commands |

### Eval Result Lifecycle

```
created ──> never_run ──> passed
                |             ^
                |             |
                v             |
              failed ─────────+  (re-run succeeds)
```

## CLI

Binary name: `cmdr` (existing).

### Evals

```
cmdr evals                                List all evals with latest results
  --project <name>                         Filter by project name
  --type <type>                            Filter by eval type
  --json                                   JSON output

cmdr evals add                            Register a new eval
  --project <name>    (required)
  --command <cmd>     (required)           Shell command to execute
  --task <desc>                            Agent task description
  --type <type>                            Eval type (default: custom)

cmdr evals run                            Run all evals sequentially
  --project <name>                         Filter by project name
  --id <eval-id>                           Run a single eval by ID

cmdr evals remove <id>                    Remove an eval definition

cmdr evals --pane                         Long-lived pane mode (for zellij dashboard)
  --project <name>                         Filter by project name
```

## JSON Output Format

Success (list):

```json
{
  "success": true,
  "command": "evals",
  "evals": [
    {
      "id": "eval-a1b2c3d4",
      "project_name": "computeCommander",
      "agent_task": "implement evals pane",
      "eval_type": "unit_test",
      "command": "go test ./internal/tui/...",
      "passed": true,
      "error_detail": null,
      "last_run_at": "2026-03-05T14:32:00Z",
      "created_at": "2026-03-05T10:00:00Z"
    }
  ],
  "count": 1
}
```

Success (add):

```json
{ "success": true, "command": "evals add", "id": "eval-a1b2c3d4" }
```

Success (run):

```json
{
  "success": true,
  "command": "evals run",
  "results": [
    { "id": "eval-a1b2c3d4", "passed": true, "duration_ms": 3420 },
    { "id": "eval-f5e6d7c8", "passed": false, "error": "exit status 1" }
  ],
  "passed": 1,
  "failed": 1,
  "total": 2
}
```

Error:

```json
{ "success": false, "command": "evals", "error": "eval not found: eval-deadbeef" }
```

## Concurrency Model

Not applicable. Eval operations are single-user CLI commands or dashboard refresh reads. The existing SQLite WAL mode (`busy_timeout=5000`) handles concurrent reads from the dashboard pane and CLI writes without additional locking.

### Atomic Writes

Eval results are written via single `UPDATE ... SET passed = ?, error_detail = ?, last_run_at = ?` statements. No temp-file + rename pattern needed -- the DB handles atomicity.

### Conflict Resolution

Last-write-wins. If two CLI invocations run the same eval simultaneously, the last `UPDATE` to complete takes precedence. This is acceptable because eval runs are idempotent.

## Migration

Not applicable. No predecessor system exists for the Evals feature. This is a fresh addition to the ComputeCommander schema (migration 004).

## Integration

### Dashboard (KDL Mode)

The Evals pane runs `cmdr evals --pane` in a zellij pane. It refreshes every 3 seconds via a ticker loop (same pattern as `runGitStatusPane`).

| Dashboard Mode | Implementation |
|----------------|----------------|
| KDL (zellij) | `cmdr evals --pane` in bottom row, 5th pane at 20% width |
| TUI (bubbletea) | `EvalsPane` component in `Dashboard.View()` bottom row |

### Agent-Facing Commands

```bash
# Register a new eval after implementing a feature
cmdr evals add --project computeCommander --task "implement evals pane" --type unit_test --command "go test ./internal/tui/..."

# Run all evals to verify nothing is broken
cmdr evals run --json

# Check eval status from within an agent session
cmdr evals --project computeCommander --json
```

### Hooks Integration

Not applicable for initial implementation. Future work could add a `PostToolUse` hook that auto-runs evals after code changes.

## What It Does NOT Do

Explicitly out of scope:

- **Parallel eval execution.** Evals run sequentially via `os/exec.Command`. Parallel execution adds complexity (output interleaving, resource contention) with minimal benefit for the expected eval count (<20 per project).
- **Eval scheduling or CI integration.** Evals are triggered manually via `cmdr evals run` or the `r` key binding. No cron, no webhook triggers. CI pipelines handle scheduled testing.
- **Test framework integration.** The evals system runs shell commands and checks exit codes. It does not parse JUnit XML, Go test JSON, or any test framework output. Pass/fail is determined solely by exit code 0 vs non-zero.
- **Eval editing.** To modify an eval, remove it and re-add. No `cmdr evals edit` command.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing project runtime |
| Language | Go | Existing project language |
| Dependencies | None new | Uses existing `os/exec`, `database/sql`, `charmbracelet/bubbletea`, `charmbracelet/lipgloss` |
| Storage | SQLite / PostgreSQL | Existing dual-DB support via `internal/platform/db` |
| Testing | `go test` | Existing test framework |
| Formatting | `gofmt` | Existing formatter |

## Project Infrastructure

### Directory Structure (files touched)

```
computeCommander/
  cmd/cc/
    main.go                                  # Wire EvalsCmd into root command
  internal/
    commands/
      evals.go                               # New: EvalsCmd with add/run/remove/--pane
    tui/
      evals_pane.go                          # New: EvalsPane TUI component
      pane.go                                # Add PaneEvals to PaneID enum
      dashboard.go                           # Wire EvalsPane, update layout math
      render.go                              # Update help bar text (1-8)
    platform/db/migrations/
      sqlite/004_evals.sql                   # New: evals table
      postgres/004_evals.sql                 # New: evals table
    zellij/
      layout.go                              # Add Evals pane to bottom row KDL
```

### Version Management

No version bump required. This is an additive feature within the existing `0.1.0` version.

### CI Workflow

No CI changes. Existing `go test ./...` will pick up any new test files automatically.

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| TUI component (`evals_pane.go`) | 1 | ~180 |
| CLI command (`evals.go`) | 1 | ~250 |
| DB migrations (sqlite + postgres) | 2 | ~30 |
| Layout modification (`layout.go`) | 0 (modified) | ~20 delta |
| Dashboard wiring (`dashboard.go`, `pane.go`, `render.go`) | 0 (modified) | ~60 delta |
| Main wiring (`main.go`) | 0 (modified) | ~3 delta |
| **Total** | **4 new + 5 modified** | **~543** |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | `unix-coder` | Create SQLite and PostgreSQL migration files for `evals` table | `internal/platform/db/migrations/sqlite/001_schema.sql`, `internal/platform/db/migrations/postgres/001_schema.sql`, `internal/platform/db/migrate.go` | `internal/platform/db/migrations/sqlite/004_evals.sql`, `internal/platform/db/migrations/postgres/004_evals.sql` | -- | `test -f internal/platform/db/migrations/sqlite/004_evals.sql && test -f internal/platform/db/migrations/postgres/004_evals.sql` |
| T2 | `unix-coder` | Create `EvalsCmd` CLI command handler with add/run/remove/--pane subcommands | `internal/commands/gitstatus.go`, `internal/commands/observability.go`, `internal/commands/app.go`, `internal/platform/db/db.go` | `internal/commands/evals.go` | T1 | `go build ./internal/commands/` |
| T3 | `unix-coder` | Create `EvalsPane` TUI component following the `MergeQueueView` pattern | `internal/tui/merge_view.go`, `internal/tui/events_pane.go`, `internal/tui/theme.go`, `internal/platform/db/db.go` | `internal/tui/evals_pane.go` | T1 | `go build ./internal/tui/` |
| T4 | `unix-coder` | Add `PaneEvals` to PaneID enum, update `AllPanes()`, `paneOrder`, key bindings, help bar text | `internal/tui/pane.go`, `internal/tui/render.go`, `internal/tui/dashboard.go` | `internal/tui/pane.go`, `internal/tui/render.go` | -- | `go build ./internal/tui/` |
| T5 | `unix-coder` | Wire `EvalsPane` into `Dashboard` struct, `NewDashboard()`, `Refresh()`, `View()`, `calculateLayout()`, `updatePaneSizes()`, `handleFocusedPaneKey()` | `internal/tui/dashboard.go`, `internal/tui/evals_pane.go` | `internal/tui/dashboard.go` | T3, T4 | `go build ./internal/tui/` |
| T6 | `unix-coder` | Add Evals pane to KDL layout bottom row (5 panes at 20% each) | `internal/zellij/layout.go` | `internal/zellij/layout.go` | T2 | `go build ./internal/zellij/` |
| T7 | `unix-coder` | Wire `EvalsCmd` into root command in `main.go` | `cmd/cc/main.go`, `internal/commands/evals.go` | `cmd/cc/main.go` | T2 | `go build ./cmd/cc/` |
| T8 | `unix-coder` | Integration verification: build the full binary and run existing tests | all modified files | -- | T1, T2, T3, T4, T5, T6, T7 | `go build ./... && go test ./internal/tui/... ./internal/commands/... ./internal/zellij/...` |

## 16. Dependency Graph

```
Phase 1 (parallel): [T1, T4]
  T1: Create migration SQL files (no Go code dependencies)
  T4: Add PaneEvals to PaneID enum and update pane infrastructure

Phase 2 (parallel, after Phase 1): [T2, T3]
  T2: Create EvalsCmd CLI handler (needs migration schema from T1)
  T3: Create EvalsPane TUI component (needs migration schema from T1)

Phase 3 (parallel, after Phase 2): [T5, T6, T7]
  T5: Wire EvalsPane into Dashboard (needs T3 EvalsPane + T4 PaneID)
  T6: Add Evals pane to KDL layout (needs T2 EvalsCmd for --pane command)
  T7: Wire EvalsCmd into main.go (needs T2 EvalsCmd)

Final: [T8] -- integration build and test
```

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/platform/db/migrations/sqlite/004_evals.sql` | ~16 | No |
| `internal/platform/db/migrations/postgres/004_evals.sql` | ~16 | No |
| `internal/commands/evals.go` | ~250 | No |
| `internal/tui/evals_pane.go` | ~180 | No |

Files modified:

- `internal/tui/pane.go` -- Add `PaneEvals` constant, update `AllPanes()` and `paneOrder`
- `internal/tui/render.go` -- Update `renderHelpBar()` text from "1-7" to "1-8"
- `internal/tui/dashboard.go` -- Add `evals *EvalsPane` field, wire in `NewDashboard()`, `Refresh()`, `View()`, `calculateLayout()`, `updatePaneSizes()`, `handleKey()`, `handleFocusedPaneKey()`
- `internal/zellij/layout.go` -- Add 5th pane to bottom row in `GenerateLayout()`, change all bottom pane sizes from `25%%` to `20%%`
- `cmd/cc/main.go` -- Add `addAppCmd(root, commands.EvalsCmd(sharedApp))` line

Files deleted: None

## 18. Verification Plan

**Per-task checks:** (derived from Task Manifest Verify Command column)
- T1: `test -f internal/platform/db/migrations/sqlite/004_evals.sql && test -f internal/platform/db/migrations/postgres/004_evals.sql`
- T2: `go build ./internal/commands/`
- T3: `go build ./internal/tui/`
- T4: `go build ./internal/tui/`
- T5: `go build ./internal/tui/`
- T6: `go build ./internal/zellij/`
- T7: `go build ./cmd/cc/`
- T8: `go build ./... && go test ./internal/tui/... ./internal/commands/... ./internal/zellij/...`

**Integration check:** `go build -o /tmp/cmdr-test ./cmd/cc/ && /tmp/cmdr-test evals --help && rm /tmp/cmdr-test`

**Rollback:** `git stash` or `git checkout -- .` to revert all changes (no migrations have been applied to a live DB at build time).

## 19. Success Criteria (Machine-Verifiable)

- [ ] `go build ./...` exits 0
- [ ] `go test ./internal/tui/...` exits 0
- [ ] `go test ./internal/commands/...` exits 0
- [ ] `go test ./internal/zellij/...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] File `internal/platform/db/migrations/sqlite/004_evals.sql` exists and contains `CREATE TABLE IF NOT EXISTS evals`
- [ ] File `internal/platform/db/migrations/postgres/004_evals.sql` exists and contains `CREATE TABLE IF NOT EXISTS evals`
- [ ] File `internal/commands/evals.go` exists and contains `func EvalsCmd`
- [ ] File `internal/tui/evals_pane.go` exists and contains `type EvalsPane struct`
- [ ] `grep -q 'PaneEvals' internal/tui/pane.go` exits 0
- [ ] `grep -q '1-8' internal/tui/render.go` exits 0
- [ ] `grep -q 'evals' internal/tui/dashboard.go` exits 0
- [ ] `grep -q 'Evals' internal/zellij/layout.go` exits 0
- [ ] `grep -q 'EvalsCmd' cmd/cc/main.go` exits 0

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| All implementation tasks (T1-T7) | `unix-coder` | Standard Go implementation work: new files, SQL migrations, CLI wiring |
| Integration verification (T8) | `unix-coder` | Build and test verification is implementation-adjacent |
| Post-implementation review | `code-review` | Verify patterns match existing codebase conventions |

## Execution Order

```
Phase 1: Foundation
  +-- T1: Create migration SQL files (agent: unix-coder)
  +-- T4: Add PaneEvals to PaneID enum (agent: unix-coder)     [parallel]

Phase 2: Core Components [blocked by Phase 1]
  +-- T2: Create EvalsCmd CLI handler (agent: unix-coder)
  +-- T3: Create EvalsPane TUI component (agent: unix-coder)   [parallel]

Phase 3: Integration Wiring [blocked by Phase 2]
  +-- T5: Wire EvalsPane into Dashboard (agent: unix-coder)
  +-- T6: Add Evals pane to KDL layout (agent: unix-coder)     [parallel]
  +-- T7: Wire EvalsCmd into main.go (agent: unix-coder)        [parallel]

Phase 4: Verification [blocked by Phase 3]
  +-- T8: Full build + test (agent: unix-coder)
```

Recommended directive: `/pai` -- straightforward plan-then-implement pipeline with clear phase dependencies. No need for full swarm orchestration on a single-feature addition.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Migration numbering conflict (another 004 migration added concurrently) | `go build` fails due to embed conflict or migration tracker rejects duplicate | Renumber to 005 and update references |
| PaneID iota ordering breaks existing key bindings | Tests in `dashboard_test.go` (`TestPaneCycle`, `TestPaneMetaByID`) fail | Ensure `PaneEvals` is appended AFTER `PaneGitStatus` in the iota block, not inserted in the middle |
| KDL layout `Sprintf` argument count mismatch | `GenerateLayout()` produces garbled output or panics at runtime | Count `%s` and `%%` placeholders carefully; add the new pane's format args at the correct position in the `Sprintf` call |
| `evals` table not created on existing installations | `cmdr evals` returns "no such table: evals" error | Migration tracking table (`_migrations`) ensures 004 runs on next `NewApp()` startup; document that users must restart cmdr |
| Bottom row panes too narrow at small terminal widths | Pane content truncated or overlapping | `calculateLayout()` already has minimum width guards (`if bottomPaneW < 10`); 20% of 80-col terminal = 16 chars, which is usable |

## Implementation Details

### T3: EvalsPane Component (detailed specification)

The `EvalsPane` struct in `internal/tui/evals_pane.go` must implement:

```go
type EvalsPane struct {
    db      db.DB          // database handle for querying evals
    evals   []EvalRow      // cached eval rows from last refresh
    theme   *Theme
    width   int
    height  int
    cursor  int            // selected row for future interaction
    projectFilter string   // optional project name filter
}

type EvalRow struct {
    ID          string
    ProjectName string
    AgentTask   string
    EvalType    string
    Passed      *bool    // nil = never run
    ErrorDetail string
    LastRunAt   string
    CreatedAt   string
}
```

Methods required:
- `NewEvalsPane(database db.DB, theme *Theme) *EvalsPane`
- `Refresh() error` -- queries `SELECT id, project_name, agent_task, eval_type, passed, error_detail, last_run_at, created_at FROM evals ORDER BY created_at DESC`
- `View() string` -- renders a table with columns: Project (16), Task (20), Type (12), Result (8); includes footer with `[r] Run All  [a] Add`
- `SetSize(w, h int)`
- `ScrollUp()`, `ScrollDown()` -- cursor navigation
- `RunAll() error` -- iterates all evals, executes `command` via `os/exec.Command("sh", "-c", command)`, updates `passed`, `error_detail`, `last_run_at` in DB
- `EvalCount() int`, `PassedCount() int`, `FailedCount() int` -- for status bar integration

### T4: PaneID Enum Changes (detailed specification)

In `internal/tui/pane.go`:

```go
const (
    PaneFilePicker PaneID = iota
    PaneAgentSession
    PaneAgents
    PaneEvents
    PaneMail
    PaneMergeQueue
    PaneGitStatus
    PaneEvals        // <-- NEW: appended at end to preserve existing iota values
)
```

Update `AllPanes()`:
```go
{ID: PaneEvals, Title: "Evals", FocusKey: "8"},
```

Update `paneOrder`:
```go
var paneOrder = []PaneID{
    PaneFilePicker, PaneAgentSession, PaneAgents,
    PaneEvents, PaneMail, PaneMergeQueue, PaneGitStatus,
    PaneEvals,  // <-- NEW
}
```

### T5: Dashboard Wiring (detailed specification)

In `internal/tui/dashboard.go`:

1. Add field: `evals *EvalsPane` to `Dashboard` struct
2. In `NewDashboard()`: `evals: NewEvalsPane(opts.DB, theme),`
3. In `Refresh()`: add `if err := d.evals.Refresh(); err != nil { errs = append(errs, err.Error()) }`
4. In `calculateLayout()`: change `bottomPaneW := w / 4` to `bottomPaneW := w / 5`
5. In `updatePaneSizes()`: add `d.evals.SetSize(bottomPaneW-2, bottomH-3)`
6. In `handleKey()`: add `case "8": d.focusedPane = PaneEvals`
7. In `handleFocusedPaneKey()`: add case for `PaneEvals` handling `r` (run all), `a` (add), `j`/`k` (scroll)
8. In `View()`: add the Evals pane rendering between GitStatus and the `bottomRow` join:

```go
// Evals pane gets remaining width (last pane in row).
evalsContent := d.evals.View()
evalsMeta := paneMetaByID(PaneEvals)
evalsPane := RenderPane(evalsContent, evalsMeta, d.focusedPane == PaneEvals, lastPaneW, bottomH, d.theme)
```

Adjust the bottom row `JoinHorizontal` to include the 5th pane. The last pane gets remaining width: `lastPaneW := d.width - (bottomPaneW * 4)`.

### T6: KDL Layout Changes (detailed specification)

In `internal/zellij/layout.go`, within `GenerateLayout()`:

1. Change all bottom row pane sizes from `25%%` to `20%%`
2. Add a 5th pane after LazyGit:

```kdl
pane name="Evals" size="20%%" {
    command "%s"
    args "evals" "--pane"%s
}
```

3. Update the `Sprintf` call to include the additional `cmdrBin` and `projectFlag` arguments for the new pane

### T7: main.go Wiring (detailed specification)

Add after the Infrastructure commands block in `cmd/cc/main.go`:

```go
// Observability: evals.
addAppCmd(root, commands.EvalsCmd(sharedApp))
```

Place it in the OBSERVABILITY group. Given that evals are about quality verification, OBSERVABILITY is the natural fit alongside `trace`, `errors`, `metrics`, and `costs`.
