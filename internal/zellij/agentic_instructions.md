# internal/zellij/ -- Zellij Terminal Multiplexer Integration

## Purpose
Manages zellij terminal panes for agent processes. Provides pane creation, listing, content capture, key sending, and closing operations via the zellij CLI. Each agent runs in its own zellij pane within a shared session.

## Technology
- Go 1.25
- `os/exec` for zellij CLI commands
- No external Go dependencies; relies on `zellij` binary being installed

## Contents
| File | Description |
|------|-------------|
| `pane.go` | `PaneManager` interface, `Manager` struct: CreatePane, ListPanes, SendKeys, CapturePaneContent, ClosePane, SpawnPane, AttachFloating |
| `pane_test.go` | Tests for pane creation, listing, key sending, content capture, and closing |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewManager` | `func NewManager(sessionPrefix string) *Manager` | `*Manager` | Creates pane manager with zellij session prefix |
| `CreatePane` | `func (m *Manager) CreatePane(ctx context.Context, opts PaneOpts) (string, error)` | `string, error` | Creates a new zellij pane, returns pane ID |
| `SpawnPane` | `func (m *Manager) SpawnPane(ctx context.Context, session, command string) (string, error)` | `string, error` | Creates pane with a specific command in a named session |
| `ListPanes` | `func (m *Manager) ListPanes(ctx context.Context, session string) ([]PaneInfo, error)` | `[]PaneInfo, error` | Lists all panes in a zellij session |
| `SendKeys` | `func (m *Manager) SendKeys(ctx context.Context, session, paneID, keys string) error` | `error` | Sends keystrokes to a pane (used for nudging) |
| `CapturePaneContent` | `func (m *Manager) CapturePaneContent(ctx context.Context, session, paneID string) (string, error)` | `string, error` | Captures current visible content of a pane |
| `ClosePane` | `func (m *Manager) ClosePane(ctx context.Context, session, paneID string) error` | `error` | Closes a pane (kills the process inside) |
| `AttachFloating` | `func (m *Manager) AttachFloating(ctx context.Context, session, command string) error` | `error` | Attaches a floating pane in a session |

## Data Types

### PaneManager (interface)
Methods: CreatePane, ListPanes, SendKeys, CapturePaneContent, ClosePane, SpawnPane, AttachFloating

### PaneOpts (struct)
Fields: Session, Name, Command, WorkDir, Env

### PaneInfo (struct)
Fields: ID, Name, IsActive, Command

### Manager (struct)
Fields: sessionPrefix

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `CreatePane()`, `SpawnPane()` create new panes
- **Read**: `ListPanes()` lists panes, `CapturePaneContent()` reads pane content
- **Update**: `SendKeys()` sends input to a pane
- **Delete**: `ClosePane()` closes/kills a pane

## Style Guide
- Interface-first: `PaneManager` interface with `Manager` implementation
- All zellij operations via `zellij` CLI with `--session` flag
- Session names prefixed with configurable prefix (default "cc")
- Error wrapping: `fmt.Errorf("zellij operation: %w", err)`
- Pane content capture used by watchdog for readiness detection and inspector

**Representative snippet (from `pane.go`):**
```go
func (m *Manager) SendKeys(ctx context.Context, session, paneID, keys string) error {
	args := []string{
		"--session", session,
		"action", "write-chars", "--pane-id", paneID,
		keys,
	}
	cmd := exec.CommandContext(ctx, "zellij", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zellij send-keys to pane %s: %w: %s", paneID, err, string(out))
	}
	return nil
}
```
