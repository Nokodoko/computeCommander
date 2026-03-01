# internal/merge/ -- FIFO Merge Queue with 4-Tier Conflict Resolution

## Purpose
Implements a FIFO merge queue and 4-tier conflict resolution system for integrating agent worktree branches into the canonical branch. Tiers: (1) clean merge, (2) auto-resolve (`-X theirs`), (3) AI resolve (stub), (4) reimagine (stub).

## Technology
- Go 1.25
- `os/exec` for git command execution
- `encoding/json` for files_modified serialization
- Depends on: `internal/platform/db`

## Contents
| File | Description |
|------|-------------|
| `types.go` | `MergeStatus` enum, `ResolutionTier` enum, `MergeEntry` struct, `MergeResult` struct, `MergeOpts`, `ListOpts` |
| `queue.go` | `MergeQueue` interface, `SQLQueue` implementation: Enqueue, Dequeue (with tx), Peek, Status, List, UpdateStatus |
| `executor.go` | `MergeExecutor` with 4-tier resolution: tier1CleanMerge, tier2AutoResolve, tier3AIResolve (stub), tier4Reimagine (stub) |
| `merge_test.go` | Tests for queue operations, executor tier progression, and conflict detection |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewSQLQueue` | `func NewSQLQueue(d db.DB) *SQLQueue` | `*SQLQueue` | Creates SQL-backed merge queue |
| `Enqueue` | `func (q *SQLQueue) Enqueue(entry *MergeEntry) error` | `error` | Validates and inserts pending entry |
| `Dequeue` | `func (q *SQLQueue) Dequeue() (*MergeEntry, error)` | `*MergeEntry, error` | Atomically dequeues oldest pending entry (BEGIN/UPDATE/COMMIT) |
| `Peek` | `func (q *SQLQueue) Peek() (*MergeEntry, error)` | `*MergeEntry, error` | Returns oldest pending entry without removing |
| `Status` | `func (q *SQLQueue) Status(branch string) (*MergeEntry, error)` | `*MergeEntry, error` | Looks up entry by branch name |
| `List` | `func (q *SQLQueue) List(opts ListOpts) ([]*MergeEntry, error)` | `[]*MergeEntry, error` | Returns entries filtered by status, ordered by enqueued_at |
| `UpdateStatus` | `func (q *SQLQueue) UpdateStatus(branch string, status MergeStatus, tier *ResolutionTier) error` | `error` | Updates status and resolution tier for a branch |
| `NewMergeExecutorWithQueue` | `func NewMergeExecutorWithQueue(queue, root, runner, ai, reimagine) *MergeExecutor` | `*MergeExecutor` | Creates executor with explicit queue and command runner |
| `Execute` | `func (m *MergeExecutor) Execute(entry *MergeEntry, targetBranch string) (*MergeResult, error)` | `*MergeResult, error` | Runs 4-tier resolution cascade, returns result |

## Data Types

### MergeStatus (string enum)
`pending` | `merging` | `merged` | `conflict` | `failed`

### ResolutionTier (string enum)
`clean-merge` | `auto-resolve` | `ai-resolve` | `reimagine`

### MergeEntry (struct)
Fields: BranchName, TaskID, AgentName, FilesModified ([]string), EnqueuedAt, Status, ResolvedTier

### MergeResult (struct)
Fields: Success, Tier, ConflictFiles, Error

### CommandRunner (interface)
`RunInDir(dir, name string, args ...string) ([]byte, error)` -- abstracts command execution for testing

## Logging
- Tier 3/4 stubs log via `log.Printf("[merge] tierN: ...")`
- Errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `Enqueue()` adds entry to merge queue
- **Read**: `Peek()`, `Status()`, `List()` query the queue
- **Update**: `Dequeue()` atomically changes status to "merging", `UpdateStatus()` sets final status
- **Delete**: N/A (entries transition to terminal states)

## Style Guide
- Interface-first: `MergeQueue` interface with `SQLQueue` implementation
- `CommandRunner` interface abstracts `os/exec` for testability
- Transaction-safe dequeue with BEGIN/COMMIT
- JSON-serialized `files_modified` field stored as TEXT in SQL
- Timestamps stored as RFC3339, with fallback parsing

**Representative snippet (from `executor.go`):**
```go
func (m *MergeExecutor) Execute(entry *MergeEntry, targetBranch string) (*MergeResult, error) {
	// Tier 1: Clean Merge
	result := m.tier1CleanMerge(entry.BranchName, targetBranch)
	if result.Success {
		m.updateEntryStatus(entry, MergeMerged, TierCleanMerge)
		return result, nil
	}
	m.abortMerge()

	// Tier 2: Auto-Resolve
	result = m.tier2AutoResolve(entry.BranchName, targetBranch)
	if result.Success {
		m.updateEntryStatus(entry, MergeMerged, TierAutoResolve)
		return result, nil
	}
	m.abortMerge()

	// Tier 3 & 4: stubs...
}
```
