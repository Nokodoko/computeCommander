# internal/agents/ -- Agent Lifecycle Management

## Purpose
Core package for agent lifecycle: spawning agents in isolated worktrees, enforcing tool/command guards per capability, generating runtime overlays from templates, and tracking session state.

## Technology
- Go 1.25
- Embeds Go text/template for overlay generation (`templates/overlay.tmpl`)
- Depends on: `internal/platform/db`, `internal/wezterm`, `internal/worktree`, `internal/zellij`, `pkg/runtimes`

## Contents
| File | Description |
|------|-------------|
| `types.go` | Core domain types: `Capability`, `SessionState`, `AgentSession`, `SpawnRequest`, `SpawnResult`, `StopOpts`, `ListOpts` |
| `spawner.go` | `Spawner` struct: spawn, stop, list agents; creates worktrees, deploys config, creates panes, registers in DB |
| `guards.go` | `GuardRules`: tool and command enforcement per capability; `IsAllowed()` checks global and per-capability rules |
| `overlay.go` | `BuildOverlay()`: renders capability-specific instruction overlays from embedded templates |
| `types_test.go` | Tests for capability and session state validation |
| `spawner_test.go` | Tests for spawner validation and lifecycle |
| `guards_test.go` | Tests for guard rule enforcement |
| `overlay_test.go` | Tests for overlay generation |
| `templates/` | Subdirectory containing `overlay.tmpl` |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewSpawner` | `func NewSpawner(opts SpawnerOpts) *Spawner` | `*Spawner` | Creates spawner with DB, pane/window/worktree managers |
| `Spawn` | `func (s *Spawner) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResult, error)` | `*SpawnResult, error` | Full agent spawn: validate, worktree, overlay, pane, DB insert |
| `Stop` | `func (s *Spawner) Stop(ctx context.Context, agentName string, opts StopOpts) error` | `error` | Terminate agent: close pane, update state |
| `ListSessions` | `func (s *Spawner) ListSessions(ctx context.Context, opts ListOpts) ([]*AgentSession, error)` | `[]*AgentSession, error` | Query sessions with token usage via LEFT JOIN |
| `SpawnDashboard` | `func (s *Spawner) SpawnDashboard(ctx context.Context) error` | `error` | Spawn dashboard in new wezterm window |
| `BuildOverlay` | `func BuildOverlay(cap Capability, taskSpec string) (*runtimes.OverlayContent, error)` | `*OverlayContent, error` | Render overlay template for capability |
| `DefaultGuardRules` | `func DefaultGuardRules() *GuardRules` | `*GuardRules` | Returns spec 3.7 guard rules |
| `IsAllowed` | `func (g *GuardRules) IsAllowed(cap Capability, tool string, args string) (bool, string)` | `bool, string` | Check if tool+args are permitted for capability |
| `AllCapabilities` | `func AllCapabilities() []Capability` | `[]Capability` | Returns all valid capability values |
| `ValidCapability` | `func ValidCapability(c Capability) bool` | `bool` | Validates a capability string |

## Data Types

### Capability (string enum)
`scout` | `builder` | `reviewer` | `lead` | `merger` | `coordinator` | `supervisor` | `monitor`

### SessionState (string enum)
`booting` | `working` | `completed` | `stalled` | `zombie`

### AgentSession (struct)
Fields: ID, AgentName, Capability, WorktreePath, BranchName, TaskID, ZellijPane, ZellijSession, State, PID, ParentAgent, Depth, RunID, StartedAt, LastActivity, EscalationLevel, StalledSince, TranscriptPath, Runtime, InputTokens, OutputTokens

### SpawnRequest (struct)
Fields: TaskID, Capability, Name, Runtime, Parent, Depth, FileScope, SpecPath, SkipScout, SkipReview, MaxAgents

### GuardRules (struct)
Fields: Global (GlobalRules), ByCapability (map[Capability]CapabilityRules)

## Logging
- No structured logging; errors are returned via `fmt.Errorf` with contextual prefixes like `"spawn validate:"`, `"spawn create worktree:"`

## CRUD Entry Points
- **Create**: `Spawner.Spawn()` creates a new agent session
- **Read**: `Spawner.ListSessions()` queries sessions with filters
- **Update**: Internal `updateSessionState()` changes session state
- **Delete**: `Spawner.Stop()` transitions session to completed/zombie

## Style Guide
- PascalCase exports, camelCase internals
- Struct-level methods on `*Spawner` and `*GuardRules`
- Error wrapping: `fmt.Errorf("context: %w", err)`
- SQL uses `$1` positional params (compatible with both postgres and sqlite)
- Imports grouped: stdlib, external, internal

**Representative snippet (from `spawner.go`):**
```go
func (s *Spawner) validateSpawnRequest(req SpawnRequest) error {
	if req.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !ValidCapability(req.Capability) {
		return fmt.Errorf("invalid capability: %q", req.Capability)
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Runtime == "" {
		return fmt.Errorf("runtime is required")
	}
	if req.Depth > s.maxDepth {
		return fmt.Errorf("depth %d exceeds max depth %d", req.Depth, s.maxDepth)
	}
	return nil
}
```
