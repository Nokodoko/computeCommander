# internal/worktree/ -- Git Worktree Management

## Purpose
Manages git worktrees for agent isolation. Each agent gets its own worktree with a unique branch, allowing parallel development without lock contention. Provides create, list, status, remove, and clean operations backed by `git worktree` CLI commands and database tracking.

## Technology
- Go 1.25
- `os/exec` for git CLI commands
- Depends on: `internal/platform/db`

## Contents
| File | Description |
|------|-------------|
| `manager.go` | `WorktreeManager` interface, `Manager` struct: Create, List, Status, Remove, Clean operations using git worktree commands and DB |
| `manager_test.go` | Tests for worktree creation, listing, status, removal, and cleanup logic |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewManager` | `func NewManager(opts ManagerOpts) *Manager` | `*Manager` | Creates worktree manager with DB, base dir, project root |
| `Create` | `func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*WorktreeInfo, error)` | `*WorktreeInfo, error` | Creates git worktree with new branch, registers in DB |
| `List` | `func (m *Manager) List(ctx context.Context) ([]*WorktreeInfo, error)` | `[]*WorktreeInfo, error` | Lists all tracked worktrees from DB |
| `Status` | `func (m *Manager) Status(ctx context.Context, path string) (*WorktreeInfo, error)` | `*WorktreeInfo, error` | Returns status for a specific worktree |
| `Remove` | `func (m *Manager) Remove(ctx context.Context, path string) error` | `error` | Removes worktree via `git worktree remove` and updates DB state to "removed" |
| `Clean` | `func (m *Manager) Clean(ctx context.Context) (int, error)` | `int, error` | Removes stale worktrees, runs `git worktree prune`, returns count cleaned |

## Data Types

### WorktreeManager (interface)
Methods: Create, List, Status, Remove, Clean

### CreateOpts (struct)
Fields: AgentName, TaskID, BranchName (auto-generated if empty), BaseBranch (defaults to "main")

### WorktreeInfo (struct)
Fields: Path, Branch, AgentName, TaskID, CreatedAt, State ("active", "stale", "removed")

### ManagerOpts (struct)
Fields: DB, BaseDir (worktree root directory), ProjectRoot (git repo root)

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `Create()` adds worktree via git and registers in DB
- **Read**: `List()` queries all worktrees, `Status()` queries one by path
- **Update**: Internal state transitions (active -> stale -> removed)
- **Delete**: `Remove()` deletes worktree from filesystem and marks as removed in DB, `Clean()` batch removes stale worktrees

## Style Guide
- Interface-first: `WorktreeManager` interface with `Manager` implementation
- Branch naming convention: `cc/{agent_name}/{task_id}`
- Git commands executed via `os/exec.CommandContext` with project root as working directory
- DB state tracking ensures worktrees are cleaned up even if git commands fail
- Stale detection based on session state (completed/zombie sessions with active worktrees)

**Representative snippet (from `manager.go`):**
```go
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*WorktreeInfo, error) {
	if opts.BranchName == "" {
		opts.BranchName = fmt.Sprintf("cc/%s/%s", opts.AgentName, opts.TaskID)
	}

	worktreePath := filepath.Join(m.baseDir, opts.AgentName)

	_, err := m.runGit(ctx, "worktree", "add", "-b", opts.BranchName, worktreePath, opts.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	info := &WorktreeInfo{
		Path:      worktreePath,
		Branch:    opts.BranchName,
		AgentName: opts.AgentName,
		TaskID:    opts.TaskID,
		State:     "active",
	}
	// Register in DB...
	return info, nil
}
```
