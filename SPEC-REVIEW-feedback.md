# Spec Review Feedback (for spec-builder)

**Iteration:** 2 of 3
**Date:** 2026-03-12

## No Critical Fixes Required

All critical issues from iteration 1 are resolved. The remaining items are warnings that should be addressed but do not block execution.

## Warnings to Address

### 1. [Section 3, 12, 15] Add `agentic_instructions.md` for new packages

The project places an `agentic_instructions.md` in every package directory (verified in `internal/commands/`, `internal/agents/`, `internal/zellij/`, `pkg/integrations/github/`, `pkg/integrations/linear/`). Add to the spec:

- Add `pkg/integrations/jira/agentic_instructions.md` to T1's write scope in the Task Manifest
- Add `internal/darkfactory/agentic_instructions.md` to T6's write scope in the Task Manifest
- Add both files to the Target State table (Section 17)
- Add both files to the Directory Structure in Section 12

### 2. [Section 4] Document `SyncedAt` time format parsing

The SQL schema stores `synced_at` as `TEXT DEFAULT (datetime('now'))` which produces `YYYY-MM-DD HH:MM:SS` (no timezone, no T separator). The Go structs use `time.Time`. Add a note in Section 4 near the struct definitions:

```
Note: SQLite stores synced_at as "YYYY-MM-DD HH:MM:SS" text. The sync engine must
use time.Parse("2006-01-02 15:04:05", raw) when scanning from SQLite, and
time.Parse(time.RFC3339, raw) when scanning from PostgreSQL.
```

### 3. [Section 5] Add `--mode` flag to `cmdr jira factory`

The factory command definition in Section 5 is missing `--mode`. Add:

```
cmdr jira factory                          Start dark factory mode
  --project <key>     (required)           Project scope for automation
  --epic <key>                             Narrow to specific epic
  --mode <mode>       (default: from config) Override execution mode
  --max-concurrent <n>                     Override max concurrent tasks
  --dry-run                                Show execution plan without running
```

### 4. [Section 7] Track `time.AfterFunc` timer in RateLimiter

Add a `retryTimer *time.Timer` field to the `RateLimiter` struct and cancel it before creating a new one:

```go
type RateLimiter struct {
    limiter    *rate.Limiter
    mu         sync.Mutex
    baseRate   rate.Limit
    reduced    bool
    retryTimer *time.Timer  // ADD THIS
}

func (r *RateLimiter) AdaptFromHeaders(remaining int, retryAfter time.Duration) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if retryAfter > 0 {
        if r.retryTimer != nil {
            r.retryTimer.Stop()  // ADD THIS
        }
        r.limiter.SetLimit(0)
        r.retryTimer = time.AfterFunc(retryAfter, func() {  // CHANGED
            r.mu.Lock()
            r.limiter.SetLimit(r.baseRate)
            r.mu.Unlock()
        })
        return
    }
    // ... rest unchanged
}
```

Note: the callback also needs to acquire `r.mu` before modifying the limiter, since it runs in a separate goroutine.

### 5. [Section 5, 9] Add `check-completion` to CLI commands

Section 9 references `cmdr jira check-completion --session $SESSION_ID` in the SubagentStop hook, but this command is not listed in Section 5. Either:

- Add it to Section 5 as an internal command:
  ```
  cmdr jira check-completion               Check if agent completed a Jira task (internal, called by hooks)
    --session <id>                          Session ID to check
  ```
- Or note in Section 9 that this is implemented as a hidden subcommand (not shown in help output)

### 6. [Section 15] Expand T2 and T6 read scopes

**T2:** Add `internal/platform/db/migrations/sqlite/005_jira_cache.sql` to T2's read scope. The sync engine needs the exact table/column names to write correct SQL.

**T6:** Add `internal/agents/types.go` to T6's read scope. The executor needs `SpawnRequest`, `SpawnResult`, `AgentSession`, and `SessionState` types.

### 7. [Section 8] Clarify `task_groups` fallback ownership

Add a sentence to Section 8 clarifying:

"When no Jira instances are configured, `cmdr jira` delegates to the existing `cmdr group list` logic rather than duplicating the `task_groups` read path. This keeps a single code path for local task groups."

Or alternatively: "The rewritten `jira.go` retains the existing `task_groups` query functions as a fallback code path, gated by `len(cfg.Jira.Instances) == 0`."

### 8. [Section 19] Add dark factory help check to success criteria

Add:
```
- [ ] `/home/n0ko/Programs/ai/computeCommander/cmdr jira factory --help` contains "project"
```

## Items That Are Fine (No Action Needed)

- **T9 verify command** is now concrete and passes validation.
- **Worktree base branch** correctly uses `main`.
- **Section numbering** is consistent (1-19).
- **Data models** are proper Go structs with correct tags.
- **Pane healer** correctly uses `SendKeys` matching the existing `PaneManager` interface.
- **Existing file acknowledgment** (Section 17 note about prior implementation pass) is present.
- **Test coverage guidance** per test file is thorough.
