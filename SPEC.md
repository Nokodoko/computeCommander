# ComputeCommander Dashboard Enhancement Suite

Five feature additions to the cmdr KDL dashboard layout and supporting infrastructure: Jira task pane, OpenBrain memory pane, self-healing for frozen/stale panes, prompt-line tool suite replacing the fp pane header area, and a `--cmdr` flag for the external `claude-sessions` Python script.

Spec type: **Feature**

## Why

The cmdr dashboard currently provides real-time visibility into agent sessions, mail, merge queue, event logs, and git status. But several pain points remain:

- **No task visibility.** Agents work on tasks, but there is no dashboard pane showing task state. Users must Alt-Tab to Jira/Linear or run ad-hoc CLI commands. A Jira pane positioned between Agents and Lazygit gives immediate task context.
- **Memory is opaque.** Claude's MEMORY.md and per-project memory writes happen silently. The user has no way to see what memories are being stored/updated during a session without manually reading files.
- **Frozen panes require manual intervention.** Pane commands (`cmdr status --pane`, `cmdr feed --pane`, etc.) can hang, crash, or go stale. The user must manually restart them via zellij. A self-healing watchdog should detect and restart frozen panes automatically.
- **No prompt-line tools.** The area above the fp pane is dead space where accidental typing occurs. This should be a prompt-line with a session picker, current session indicator, and quick-action tools.
- **claude-sessions is disconnected from cmdr.** The external Python script (`~/programming/python_projects/claude_sessions.py`) can pick and resume Claude sessions, but cannot spawn a cmdr dashboard in the target project directory. Adding a `--cmdr` flag bridges this gap.

These five features address ~80% of remaining dashboard friction. Each is independently implementable and testable.

## Design Principles

1. **Pane commands are self-contained.** Each new pane runs a `cmdr <subcmd> --pane` command that loops, refreshes, and handles its own lifecycle. No inter-pane IPC beyond the existing per-tab CWD file mechanism.
2. **KDL layout is the single source of truth.** All pane positions, sizes, and commands are defined in the generated KDL layout via `GenerateLayout()` in `internal/zellij/layout.go`. No pane is added dynamically at runtime.
3. **Self-healing is opt-out, not opt-in.** The pane health checker runs by default inside the dashboard's focus-watcher-wrapper or as a sibling borderless pane. It detects frozen panes and restarts them without user interaction.
4. **Wrapper script pattern.** New panes that need session-switch tracking follow the existing wrapper pattern (fp-wrapper.sh, lazygit-wrapper.sh) with inotifywait watching the per-tab CWD file.
5. **Worktree isolation for development.** Each feature task executes in its own git worktree to enable parallel development without merge conflicts.

## On-Disk Format

```
computeCommander/
  internal/
    zellij/
      layout.go                    # KDL layout generation (MODIFIED)
    commands/
      jira.go                      # Jira pane --pane command (NEW)
      openbrain.go                 # OpenBrain memory pane --pane command (NEW)
      prompt_line.go               # Prompt-line tool suite (NEW)
      session_picker.go            # Session picker (MODIFIED - awareness of prompt pane)
      dashboard.go                 # Dashboard command (MODIFIED)
    watchdog/
      pane_healer.go               # Pane self-healing logic (NEW)
      watchdog.go                  # Watchdog (MODIFIED -- integrate pane healing)
      health.go                    # Health checks (MODIFIED -- add pane health check type)
  ~/programming/python_projects/
    claude_sessions.py             # Python session picker (MODIFIED)
```

### jira.go

New Cobra command `cmdr jira --pane` that renders a task list in the dashboard pane. Reads tasks from the `task_groups` and `task_group_members` DB tables (already exist in the schema). Loops with a configurable refresh interval, prints ANSI-styled task list to stdout. In `--pane` mode, runs a ticker-based loop with terminal-height-aware truncation (same pattern as `status.go:printAgentsPane`).

```go
// Representative structure
func JiraCmd(app *App) *cobra.Command {
    cmd := &cobra.Command{
        Use:     "jira",
        Short:   "Task group status for dashboard pane",
        GroupID: "CORE",
        RunE: func(cmd *cobra.Command, args []string) error {
            pane, _ := cmd.Flags().GetBool("pane")
            if pane {
                return runJiraPane(cmd.Context(), app)
            }
            return printJiraSummary(cmd.Context(), app)
        },
    }
    cmd.Flags().Bool("pane", false, "Dashboard pane mode (loop + ANSI)")
    return cmd
}
```

### openbrain.go

New Cobra command `cmdr openbrain --pane` that watches MEMORY.md files for changes using fsnotify. Displays recently written/modified memory entries with timestamps. Monitors `~/.claude/MEMORY.md` (global) and project-local MEMORY.md (derived from per-tab CWD file). In `--pane` mode, streams updates as they happen with ANSI-styled output.

```go
// Watches these paths:
// 1. ~/.claude/MEMORY.md (global Claude memory)
// 2. <project>/.claude/MEMORY.md (project-local, follows CWD file)
// 3. <project>/.claude/projects/*/MEMORY.md (per-project memories)
func runOpenBrainPane(ctx context.Context, app *App) error {
    // fsnotify watcher + polling fallback
    // diff detection: compare file content hashes on change events
    // render changed sections with timestamps
}
```

### pane_healer.go

Self-healing daemon that runs alongside the dashboard. Uses `zellij action list-clients` and pane content capture via `zellij action dump-screen` to detect frozen/stale panes:
- **Frozen**: pane content unchanged for >30s when the pane command should be refreshing (e.g., `cmdr status --pane` refreshes every 3s)
- **Stale**: pane process exited (PID dead) but pane still visible with old content
- Recovery: `zellij action close-pane <id>` followed by recreating the pane with its original command

```go
type PaneHealer struct {
    panes       PaneManager
    interval    time.Duration
    snapshots   map[string]paneSnapshot // paneName -> last known state
    maxRestarts int                      // per-pane restart cap (default: 5)
}

type paneSnapshot struct {
    contentHash string
    lastChange  time.Time
    restarts    int
    command     string
}
```

### prompt_line.go

New Cobra command `cmdr prompt --pane` that renders a single-line prompt bar showing:
- Current session name/ID (from the per-tab CWD file or agent wrapper state)
- Project directory basename (from CWD file)
- Visual indicators: active session count, memory writes pending
- Key legend: `[Ctrl+Space S] Sessions  [Ctrl+Space ?] Help`

Runs in a 1-row borderless pane at the top of the left column (above fp). This is an info-display pane, not an interactive input. Session switching remains via `Ctrl+Space > S` (existing keybind that launches `cmdr sessions` as a floating pane).

## Data Model

### JiraPaneState

```typescript
interface JiraPaneState {
  // Display
  tasks: TaskEntry[];           // current task list from DB
  cursor: number;               // selected row index (for future keyboard nav)
  lastRefresh: string;          // ISO 8601 timestamp of last DB read

  // Config
  refreshInterval: number;      // milliseconds between DB polls (default: 5000)
  maxRows: number;              // terminal height - header/footer
}
```

### TaskEntry

```typescript
interface TaskEntry {
  id: string;                   // task_group ID from DB, e.g. "tg-a1b2c3d4"
  name: string;                 // task group name, e.g. "Implement auth middleware"
  status: string;               // "pending" | "active" | "completed" | "failed"
  memberCount: number;          // number of agents assigned to this task group
  activeMembers: number;        // agents currently in "working" state
  createdAt: string;            // ISO 8601, e.g. "2026-03-11T14:30:00Z"
}
```

### MemoryEntry

```typescript
interface MemoryEntry {
  file: string;                 // path to MEMORY.md file
  section: string;              // heading that changed (e.g., "## Architecture Decisions")
  operation: "added" | "modified" | "deleted";
  timestamp: string;            // ISO 8601 when change was detected
  preview: string;              // first 80 chars of the changed content
}
```

### PaneHealth

```typescript
interface PaneHealth {
  paneName: string;             // "Agents", "Event Log", "Mail", "Evals", "Merge Queue"
  paneId: string;               // zellij terminal pane ID (from list-clients output)
  command: string;              // original command + args for restart
  lastContentHash: string;      // SHA-256 of last captured pane content
  lastChangeAt: string;         // ISO 8601 when content last changed
  status: "healthy" | "frozen" | "stale" | "restarting";
  restartCount: number;         // times this pane has been auto-restarted this session
}
```

### Pane Health Lifecycle

```
healthy ──> frozen ──> restarting ──> healthy
   |                       ^
   +──> stale ─────────────+
```

Transitions:
- `healthy -> frozen`: content hash unchanged for >30s on a pane with expected refresh
- `healthy -> stale`: pane process PID no longer alive
- `frozen -> restarting`: healer closes and recreates the pane
- `stale -> restarting`: healer closes and recreates the pane
- `restarting -> healthy`: new pane process started and producing output

## CLI

Binary name: `cmdr`

### Jira Pane

```
cmdr jira                              List task groups with status summary
  --pane                               Dashboard pane mode (loop + ANSI styling)
  --project <id>                       Filter by project ID
  --json                               JSON output

cmdr jira show <id>                    Show task group detail with member agents
  --json                               JSON output
```

### OpenBrain Pane

```
cmdr openbrain                         Show recent memory file changes
  --pane                               Dashboard pane mode (watch + stream ANSI)
  --project <dir>                      Override project directory for memory watch
  --json                               JSON output
```

### Prompt Line

```
cmdr prompt                            Show current session info (one-shot)
  --pane                               Dashboard pane mode (live-updating single line)
```

### Self-Healing (no user-facing CLI)

Pane healing is triggered by the focus-watcher-wrapper or a dedicated healer borderless pane. No standalone CLI command. Configuration via existing `config.yaml`:

```yaml
watchdog:
  pane_healer:
    enabled: true
    check_interval_ms: 10000     # check every 10s
    frozen_threshold_ms: 30000   # 30s without content change = frozen
    max_restarts: 5              # per-pane restart cap per dashboard session
```

## JSON Output Format

Success (jira):

```json
{ "success": true, "command": "jira", "tasks": [{"id": "tg-a1b2c3d4", "name": "Implement auth middleware", "status": "active", "memberCount": 3, "activeMembers": 2, "createdAt": "2026-03-11T14:30:00Z"}], "count": 1 }
```

Success (openbrain):

```json
{ "success": true, "command": "openbrain", "entries": [{"file": "/home/n0ko/.claude/MEMORY.md", "section": "## Architecture Decisions", "operation": "modified", "timestamp": "2026-03-11T14:30:00Z", "preview": "Added PTY embedding decision note for zellij approach"}], "count": 1 }
```

Error:

```json
{ "success": false, "command": "jira", "error": "database connection failed: no such table: task_groups" }
```

## Concurrency Model

Not applicable. Each pane runs as an independent OS process within zellij. No shared mutable state between panes beyond the per-tab CWD file, which uses the existing atomic write pattern (write content, no rename needed since single-writer). The DB is accessed read-only by pane commands; the existing SQLite WAL mode handles concurrent readers.

## Migration

Not applicable. No predecessor system; these are additive features on top of the existing cmdr dashboard. No data migration required.

## Integration

### KDL Layout Integration

The `GenerateLayout()` function in `internal/zellij/layout.go` is the integration point for all pane layout changes. The updated layout structure:

```
+----------+------------------------------------------+-----------+
| prompt   |                                          |           |
| (1 row)  |                                          |  Agents   |
+----------+     Agent Session (borderless)           |  (15%)    |
|          |          67% width                        |           |
|  fp      |          (focused)                        +-----------+
|  (10%)   |                                          |  Jira     |
|          |                                          |  (8%)     |
+----------+------------------------------------------+-----------+
| Event Log (17%) | Mail (17%) | Evals (17%) | Merge Q (17%) |OB(15%)|LG(17%)|
+----------+------+------------+--------------+-------+-----------+------+
```

Top row: 67% height. Right column splits: Agents (upper, ~15%) | Jira (lower, ~8%).
Bottom row: 33% height. Six panes: Event Log, Mail, Evals, Merge Queue, OpenBrain, LazyGit.
Prompt line: 1-row borderless pane above fp in the left column.

### Updated Pane Map

| Pane | Position | Size | Command | Change |
|------|----------|------|---------|--------|
| Prompt | top-left row 1 | 1 row, borderless | `cmdr prompt --pane` | NEW |
| fp | top-left below prompt | 10% of top row width | `fp-wrapper.sh` | Unchanged |
| Agent Session | top-center | 67% width | `cmdr-agent-wrapper.sh` | Unchanged |
| Agents | top-right upper | 15% of right column | `cmdr status --pane` | Shrunk from 23% |
| Jira | top-right lower | 8% of right column | `cmdr jira --pane` | NEW |
| Event Log | bottom | 17% | `cmdr feed --pane` | Adjusted |
| Mail | bottom | 17% | `cmdr mail list --pane` | Unchanged |
| Evals | bottom | 17% | `cmdr evals --pane` | Unchanged |
| Merge Queue | bottom | 17% | `cmdr merge list --pane` | Unchanged |
| OpenBrain | bottom | 15% | `cmdr openbrain --pane` | NEW |
| LazyGit | bottom | 17% | `lazygit-wrapper.sh` | Adjusted |

### claude-sessions Integration

```bash
# Existing behavior (unchanged)
python3 ~/programming/python_projects/claude_sessions.py
# -> picks session via gum filter, execs into `claude --resume <id>`

# Existing --sidecar flag (unchanged)
python3 ~/programming/python_projects/claude_sessions.py --sidecar
# -> picks session, execs into sidecar in project dir

# New --cmdr flag
python3 ~/programming/python_projects/claude_sessions.py --cmdr
# -> picks session via gum filter, then:
#    1. os.chdir(project_path)
#    2. os.execvp("cmdr", ["cmdr", "dashboard"])
#    (spawns full cmdr dashboard in the selected project directory)
```

Implementation in `claude_sessions.py`:

```python
parser.add_argument(
    "--cmdr",
    action="store_true",
    help="Launch cmdr dashboard in the selected session's project directory",
)

# In main(), after session selection:
if args.cmdr:
    launch_cmdr(project_path)
elif args.sidecar:
    launch_sidecar(project_path)
else:
    resume_session(project_path, session_id)

def launch_cmdr(cwd: str) -> None:
    """Change to project directory and exec into cmdr dashboard."""
    os.chdir(cwd)
    os.execvp("cmdr", ["cmdr", "dashboard"])
```

### Agent-Facing Commands

```bash
# Dashboard pane commands (run by zellij, not by users)
cmdr jira --pane                    # task list in Jira pane
cmdr openbrain --pane               # memory watcher in OpenBrain pane
cmdr prompt --pane                  # session info bar in prompt pane

# Existing pane commands (unchanged)
cmdr status --pane                  # agent table in Agents pane
cmdr feed --pane                    # event log in Event Log pane
cmdr mail list --pane               # mail list in Mail pane
cmdr evals --pane                   # evals in Evals pane
cmdr merge list --pane              # merge queue in Merge Queue pane
```

### Hooks Integration

No new hooks required. The pane healer runs as part of the dashboard process lifecycle (inside the focus-watcher-wrapper restart loop or as a dedicated borderless pane).

## What It Does NOT Do

Explicitly out of scope:

- **External Jira/Linear API integration.** The Jira pane reads from the local `task_groups` DB table only, not from external issue trackers. External integrations are handled by `pkg/integrations/` stubs.
- **Memory write interception.** The OpenBrain pane watches files for changes after the fact. It does not intercept, filter, or modify Claude's memory-writing behavior.
- **Cross-tab pane healing.** Self-healing only operates on panes within the current dashboard tab instance (identified by TAB_HASH). Other tabs manage their own health.
- **Interactive prompt REPL.** The prompt-line pane displays information and key hints. All interactive actions (session switch, help) use existing floating pane mechanisms (Ctrl+Space keybind).
- **claude-sessions Python rewrite.** Only a `--cmdr` flag is added. The script is not migrated to Go or restructured.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing project runtime |
| Language | Go (T1-T6, T8), Python 3 (T7) | Match existing codebases |
| CLI Framework | Cobra | Existing CLI framework, all commands follow `XxxCmd(app *App) *cobra.Command` pattern |
| Terminal Output | lipgloss + raw ANSI | Pane-mode rendering uses direct ANSI output, same as `status.go:printAgentsPane` |
| File Watching | fsnotify | Already a project dependency via `internal/config/watcher.go` |
| DB Access | modernc.org/sqlite | Existing SQLite driver for `task_groups` queries |
| Testing | `go test` | Existing test infrastructure |
| Pane Management | zellij CLI (`zellij action`) | All pane operations go through zellij's CLI |

## Project Infrastructure

### Directory Structure

```
computeCommander/
  cmd/cc/
    main.go                           # Cobra command tree (MODIFIED: register new commands)
  internal/
    commands/
      jira.go                         # Jira task pane command
      jira_test.go                    # Tests for Jira pane
      openbrain.go                    # OpenBrain memory pane command
      openbrain_test.go               # Tests for OpenBrain pane
      prompt_line.go                  # Prompt-line tool suite
      prompt_line_test.go             # Tests for prompt-line
      dashboard.go                    # Dashboard command (MODIFIED)
      session_picker.go               # Session picker (existing, read-only reference)
    watchdog/
      pane_healer.go                  # Pane self-healing logic
      pane_healer_test.go             # Tests for pane healer
      watchdog.go                     # Watchdog (MODIFIED)
      health.go                       # Health checks (MODIFIED)
    zellij/
      layout.go                       # KDL layout generation (MODIFIED)
  ~/programming/python_projects/
    claude_sessions.py                # Python session picker (MODIFIED)
```

### Version Management

Existing: version embedded at build time via `go build -ldflags`. No version bump required for these features.

### CHANGELOG.md

Not applicable. The project does not currently maintain a CHANGELOG.md. Changes are tracked via git commits.

### CI Workflow

Existing CI runs `go test ./...` and `go vet ./...`. New test files are automatically discovered.

### Scripts

```json
{
  "scripts": {
    "build": "make build",
    "test": "go test ./...",
    "vet": "go vet ./..."
  }
}
```

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| Jira pane (command + tests) | 2 | ~250 |
| OpenBrain pane (command + tests) | 2 | ~300 |
| Self-healing (pane_healer + tests) | 2 | ~350 |
| Prompt-line (command + tests) | 2 | ~200 |
| KDL layout update (layout.go delta) | 1 | ~80 |
| Command registration (main.go delta) | 1 | ~15 |
| Dashboard integration (dashboard.go delta) | 1 | ~20 |
| Watchdog integration (watchdog.go + health.go delta) | 2 | ~50 |
| claude-sessions Python update | 1 | ~30 |
| **Total** | **14** | **~1295** |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Implement Jira task pane (`cmdr jira --pane`) | internal/commands/status.go, internal/commands/app.go, internal/commands/observability.go | internal/commands/jira.go, internal/commands/jira_test.go | -- | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/commands/ -run TestJira -count=1 -v` |
| T2 | unix-coder | Implement OpenBrain memory pane (`cmdr openbrain --pane`) | internal/commands/status.go, internal/commands/app.go, internal/config/watcher.go | internal/commands/openbrain.go, internal/commands/openbrain_test.go | -- | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/commands/ -run TestOpenBrain -count=1 -v` |
| T3 | unix-coder | Implement pane self-healing daemon | internal/watchdog/watchdog.go, internal/watchdog/health.go, internal/zellij/pane.go | internal/watchdog/pane_healer.go, internal/watchdog/pane_healer_test.go | -- | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/watchdog/ -run TestPaneHeal -count=1 -v` |
| T4 | unix-coder | Implement prompt-line tool suite (`cmdr prompt --pane`) | internal/commands/session_picker.go, internal/commands/status.go | internal/commands/prompt_line.go, internal/commands/prompt_line_test.go | -- | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/commands/ -run TestPromptLine -count=1 -v` |
| T5 | unix-coder | Update KDL layout: add Jira, OpenBrain, prompt-line panes to GenerateLayout() | internal/zellij/layout.go | internal/zellij/layout.go | T1, T2, T3, T4 | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/zellij/ -count=1 -v` |
| T6 | unix-coder | Register jira, openbrain, prompt commands in Cobra command tree | cmd/cc/main.go, internal/commands/app.go | cmd/cc/main.go | T1, T2, T4 | `cd /home/n0ko/Programs/ai/computeCommander && go build -o cmdr ./cmd/cc/ && ./cmdr jira --help && ./cmdr openbrain --help && ./cmdr prompt --help` |
| T7 | unix-coder | Add `--cmdr` flag to claude_sessions.py | /home/n0ko/programming/python_projects/claude_sessions.py | /home/n0ko/programming/python_projects/claude_sessions.py | -- | `python3 /home/n0ko/programming/python_projects/claude_sessions.py --help 2>&1 \| grep -q cmdr` |
| T8 | unix-coder | Integrate pane healer into watchdog Run loop and dashboard lifecycle | internal/watchdog/watchdog.go, internal/watchdog/health.go, internal/commands/dashboard.go | internal/watchdog/watchdog.go, internal/watchdog/health.go, internal/commands/dashboard.go | T3, T5 | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./internal/watchdog/ -count=1 -v && go test ./internal/commands/ -count=1 -v` |
| T9 | code-review | Review all changes for consistency, DRY, Go style, test coverage | All written files from T1-T8 | -- | T5, T6, T7, T8 | `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go vet ./... && go test ./...` |

> **WORKTREE DIRECTIVE:** Each of T1-T4 and T7 MUST execute in an independent git worktree branched from `agent-color-events`. Before beginning work, each worker runs:
> ```bash
> git worktree add .claude/worktrees/<feature-name> -b feat/<feature-name> agent-color-events
> ```
> T5, T6, T8 execute on the main worktree after merging T1-T4 branches. T9 reviews the merged result.

## 16. Dependency Graph

```
Phase 1 (parallel): [T1, T2, T3, T4, T7]
  T1: Jira pane command (worktree: wt-jira)
  T2: OpenBrain memory pane command (worktree: wt-openbrain)
  T3: Pane self-healing daemon (worktree: wt-healer)
  T4: Prompt-line tool suite (worktree: wt-prompt)
  T7: claude-sessions --cmdr flag (worktree: wt-sessions)

Phase 2 (after Phase 1): [T5, T6]
  T5: Update KDL layout with all new panes
  T6: Register new commands in Cobra tree

Phase 3 (after Phase 2): [T8]
  T8: Integrate pane healer into watchdog + dashboard

Final: [T9] -- code review
```

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/commands/jira.go` | ~150 | No |
| `internal/commands/jira_test.go` | ~100 | No |
| `internal/commands/openbrain.go` | ~180 | No |
| `internal/commands/openbrain_test.go` | ~120 | No |
| `internal/watchdog/pane_healer.go` | ~250 | No |
| `internal/watchdog/pane_healer_test.go` | ~100 | No |
| `internal/commands/prompt_line.go` | ~130 | No |
| `internal/commands/prompt_line_test.go` | ~70 | No |

Files modified:

- `internal/zellij/layout.go` -- add Jira, OpenBrain, prompt-line panes to `GenerateLayout()` template
- `cmd/cc/main.go` -- register `jira`, `openbrain`, `prompt` Cobra commands
- `internal/watchdog/watchdog.go` -- add PaneHealer to WatchdogOpts, integrate into Run loop
- `internal/watchdog/health.go` -- add `pane_frozen` and `pane_stale` issue types
- `internal/commands/dashboard.go` -- wire pane healer config into layout opts
- `/home/n0ko/programming/python_projects/claude_sessions.py` -- add `--cmdr` argument and `launch_cmdr()` function

Files deleted: None

## 18. Verification Plan

**Per-task checks:** (derived from Task Manifest Verify Command column)
- T1: `go build ./... && go test ./internal/commands/ -run TestJira -count=1 -v`
- T2: `go build ./... && go test ./internal/commands/ -run TestOpenBrain -count=1 -v`
- T3: `go build ./... && go test ./internal/watchdog/ -run TestPaneHeal -count=1 -v`
- T4: `go build ./... && go test ./internal/commands/ -run TestPromptLine -count=1 -v`
- T5: `go build ./... && go test ./internal/zellij/ -count=1 -v`
- T6: `go build -o cmdr ./cmd/cc/ && ./cmdr jira --help && ./cmdr openbrain --help && ./cmdr prompt --help`
- T7: `python3 /home/n0ko/programming/python_projects/claude_sessions.py --help 2>&1 | grep -q cmdr`
- T8: `go build ./... && go test ./internal/watchdog/ -count=1 -v && go test ./internal/commands/ -count=1 -v`

**Integration check:** `cd /home/n0ko/Programs/ai/computeCommander && go build ./... && go test ./... && go vet ./...`

**Rollback:** Each Phase 1 task runs in an independent git worktree. On task failure: `git worktree remove .claude/worktrees/<name>` discards the work. On integration failure (Phase 2+): `git reset --hard agent-color-events` reverts all merged changes.

## 19. Success Criteria (Machine-Verifiable)

- [ ] `cd /home/n0ko/Programs/ai/computeCommander && go build ./...` exits 0
- [ ] `cd /home/n0ko/Programs/ai/computeCommander && go test ./...` exits 0
- [ ] `cd /home/n0ko/Programs/ai/computeCommander && go vet ./...` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/jira.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/jira_test.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/openbrain.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/openbrain_test.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/watchdog/pane_healer.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/watchdog/pane_healer_test.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/prompt_line.go` exits 0
- [ ] `test -f /home/n0ko/Programs/ai/computeCommander/internal/commands/prompt_line_test.go` exits 0
- [ ] `cd /home/n0ko/Programs/ai/computeCommander && go build -o cmdr ./cmd/cc/ && ./cmdr jira --help` exits 0
- [ ] `cd /home/n0ko/Programs/ai/computeCommander && ./cmdr openbrain --help` exits 0
- [ ] `cd /home/n0ko/Programs/ai/computeCommander && ./cmdr prompt --help` exits 0
- [ ] `python3 /home/n0ko/programming/python_projects/claude_sessions.py --help 2>&1 | grep -q cmdr` exits 0
- [ ] `grep -q 'jira\|Jira' /home/n0ko/Programs/ai/computeCommander/internal/zellij/layout.go` exits 0
- [ ] `grep -q 'openbrain\|OpenBrain' /home/n0ko/Programs/ai/computeCommander/internal/zellij/layout.go` exits 0
- [ ] `grep -q 'prompt' /home/n0ko/Programs/ai/computeCommander/internal/zellij/layout.go` exits 0

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Pane commands (T1, T2, T4) | `unix-coder` | Standard Go CLI implementation following existing `XxxCmd(app *App) *cobra.Command` pattern |
| Self-healing daemon (T3) | `unix-coder` | Extends existing watchdog package with new health check type using established patterns |
| KDL layout integration (T5) | `unix-coder` | Modifies `GenerateLayout()` Sprintf template -- requires understanding of KDL split_direction semantics |
| Command registration (T6) | `unix-coder` | Single-file edit to wire commands into Cobra tree |
| Python script update (T7) | `unix-coder` | Small Python change, independent of Go codebase |
| Watchdog integration (T8) | `unix-coder` | Requires understanding of the watchdog Run loop and health check architecture |
| Final review (T9) | `code-review` | Cross-feature consistency check, DRY analysis, Go style compliance |

## Execution Order

```
Phase 1: Feature Implementation (parallel, each in own git worktree)
  +-- T1: Jira pane command (agent: unix-coder)           [worktree: wt-jira]
  +-- T2: OpenBrain pane command (agent: unix-coder)       [worktree: wt-openbrain]
  +-- T3: Pane self-healing daemon (agent: unix-coder)     [worktree: wt-healer]
  +-- T4: Prompt-line tool suite (agent: unix-coder)       [worktree: wt-prompt]
  +-- T7: claude-sessions --cmdr flag (agent: unix-coder)  [worktree: wt-sessions]

Phase 2: Integration [blocked by Phase 1]
  +-- T5: Update KDL layout (agent: unix-coder)
  +-- T6: Register commands in Cobra tree (agent: unix-coder)  [parallel with T5]

Phase 3: Wiring [blocked by Phase 2]
  +-- T8: Integrate healer into watchdog + dashboard (agent: unix-coder)

Phase 4: Review [blocked by Phase 3]
  +-- T9: Code review across all features (agent: code-review)
```

Recommended directive: `/loop` for Phase 1 (5 independent tasks fan out in parallel worktrees), then `/multi` for Phases 2-4 (sequential pipeline with merge between phases).

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Jira pane crashes on empty DB (no `task_groups` table) | `go test -run TestJiraEmptyDB` | Handle zero-row and missing-table cases gracefully; render "No tasks" |
| OpenBrain fsnotify exhausts inotify watches | `sysctl fs.inotify.max_user_watches` < watched file count | Fall back to polling (2s interval) when `fsnotify.Add()` returns error |
| Pane healer enters restart loop | `restartCount > maxRestarts` within session | Exponential backoff: 3s, 6s, 12s... up to 60s; after maxRestarts, mark pane as "abandoned" and stop trying |
| KDL layout size percentages exceed 100% | `go test ./internal/zellij/` with layout validation test | Sum-check in test: sibling pane sizes must not exceed 100% |
| claude-sessions --cmdr fails when cmdr not installed | `which cmdr` exits non-zero | Print error: "cmdr not found in PATH. Install with: cd ~/Programs/ai/computeCommander && make install" |
| Worktree merge conflicts between features | `git merge` exits non-zero | Features are designed to touch different files; T1-T4 create new files only. Conflicts in layout.go and main.go resolved in T5/T6 |
| Prompt-line pane steals keyboard focus from agent pane | Manual testing: focus stays in agent pane after tab load | Prompt pane uses `borderless=true` without `focus=true`; only agent pane gets `focus=true` |
| OpenBrain shows stale data after session switch | CWD file changes but fsnotify watches old paths | Re-read CWD file on each poll cycle; restart fsnotify watches when project directory changes |

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 1 | Should the Jira pane show tasks from external Jira/Linear APIs or only from the local `task_groups` DB table? | Determines whether HTTP client and API credentials are needed | Local DB only -- external integrations are a separate feature |
| 2 | Should the OpenBrain pane watch all MEMORY.md files across all projects, or only current project + global? | Affects fsnotify watch count and UI noise | Current project (from CWD file) + global `~/.claude/MEMORY.md` only |
| 3 | What exact KDL percentage split for the right column with Agents + Jira? | Visual balance of the dashboard | Agents 65% of right column, Jira 35% of right column (right column itself is 23% of top row) |
| 4 | Should pane self-healing be a separate borderless pane or integrated into focus-watcher-wrapper? | Resource usage and failure isolation | Dedicated borderless pane running `cmdr heal --pane` for isolation -- if the healer crashes, it does not take down the focus-watcher |
| 5 | For the prompt-line, should it show interactive key hints or just session info? | UX complexity | Info-only display: project name, session name, agent count. Key hints for Ctrl+Space shortcuts. No interactive input. |
