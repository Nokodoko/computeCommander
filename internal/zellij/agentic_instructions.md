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
| `layout.go` | `GenerateLayout()`, `WriteLayout()`, `WriteAgentWrapper()`, `LaunchSession()`, `DefaultLayoutPath()`, `IsInsideZellij()`, `ZellijAvailable()` -- KDL layout generation and session management |
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
| `GenerateLayout` | `func GenerateLayout(opts LayoutOpts) string` | `string` | Generates KDL layout string with keybinds (Ctrl+Space leader, S for sessions picker) and cmdr dashboard (fp + agent + agents + bottom bar) |
| `WriteLayout` | `func WriteLayout(path string, opts LayoutOpts) error` | `error` | Writes KDL layout file, auto-generates agent wrapper script |
| `WriteAgentWrapper` | `func WriteAgentWrapper(dir, agentCmd string) (string, error)` | `string, error` | Generates bash wrapper script for agent pane with session-switch support |
| `buildAgentPane` | `func buildAgentPane(agentCmd, wrapperPath string) string` | `string` | Builds KDL pane block for center agent pane; prefers wrapper script, falls back to direct command or plain shell |
| `LaunchSession` | `func LaunchSession(opts SessionOpts) error` | `error` | Always creates a new zellij session with `--new-session-with-layout`; kills and deletes stale session first, strips ZELLIJ env vars, connects stdin/stdout/stderr |
| `DefaultLayoutPath` | `func DefaultLayoutPath() string` | `string` | Returns `.computecommander/layouts/cmdr-dashboard.kdl` |
| `IsInsideZellij` | `func IsInsideZellij() bool` | `bool` | Checks ZELLIJ/ZELLIJ_SESSION_NAME env vars |
| `ZellijAvailable` | `func ZellijAvailable() bool` | `bool` | Checks if zellij binary is on PATH |
| `WriteAgentWrapper` | `func WriteAgentWrapper(dir, agentCmd string) (string, error)` | `string, error` | Generates bash wrapper script at `.computecommander/scripts/cmdr-agent-wrapper.sh` for agent pane with session-switch support |

## Data Types

### PaneManager (interface)
Methods: CreatePane, ListPanes, SendKeys, CapturePaneContent, ClosePane, SpawnPane, AttachFloating

### PaneOpts (struct)
Fields: Session, Name, Command, WorkDir, Env

### PaneInfo (struct)
Fields: ID, Name, IsActive, Command

### Manager (struct)
Fields: sessionPrefix

### LayoutOpts (struct)
Fields: CmdrBinary (string), SessionPrefix (string), ProjectDir (string), AgentCommand (string), AgentWrapperPath (string)

### SessionOpts (struct)
Fields: SessionName (string), LayoutPath (string), WorkDir (string)

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `CreatePane()`, `SpawnPane()` create new panes; `WriteLayout()` generates layout files; `WriteAgentWrapper()` generates scripts
- **Read**: `ListPanes()` lists panes, `CapturePaneContent()` reads pane content, `IsInsideZellij()` / `ZellijAvailable()` detect environment
- **Update**: `SendKeys()` sends input to a pane
- **Delete**: `ClosePane()` closes/kills a pane
- **Launch**: `LaunchSession()` always creates a new zellij session via `--new-session-with-layout` (kills and deletes stale sessions first, strips ZELLIJ env vars to avoid nesting)

## Style Guide
- Interface-first: `PaneManager` interface with `Manager` implementation
- All zellij operations via `zellij` CLI with `--session` flag
- Session names prefixed with configurable prefix (default "cc")
- Error wrapping: `fmt.Errorf("zellij operation: %w", err)`
- Pane content capture used by watchdog for readiness detection and inspector

**Representative snippet (from `layout.go`):**
```go
func LaunchSession(opts SessionOpts) error {
	zellijBin, err := exec.LookPath("zellij")
	if err != nil {
		return fmt.Errorf("zellij not found in PATH: %w", err)
	}

	if opts.SessionName == "" {
		opts.SessionName = "cc-dashboard"
	}

	// Strip ZELLIJ env vars so we can start a new session even when
	// already running inside zellij (wezterm's default).
	env := filteredEnv()

	// Clean up any stale session with this name.
	kill := exec.Command(zellijBin, "kill-session", opts.SessionName)
	kill.Env = env
	_ = kill.Run()
	del := exec.Command(zellijBin, "delete-session", opts.SessionName)
	del.Env = env
	_ = del.Run()

	cmd := exec.Command(zellijBin, "--session", opts.SessionName,
		"--new-session-with-layout", opts.LayoutPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}
```
