# internal/wezterm/ -- WezTerm Window Management

## Purpose
Manages WezTerm terminal windows for the ComputeCommander dashboard and agent panes. Spawns new WezTerm instances with custom `WM_CLASS` (for tiling WM integration like dwm), handles both zellij session attachment and direct command execution (TUI mode), and clears environment variables to prevent nested agent conflicts.

## Technology
- Go 1.25
- `os/exec` for spawning wezterm processes
- No external Go dependencies; relies on `wezterm` CLI being installed

## Contents
| File | Description |
|------|-------------|
| `window.go` | `WindowManager` interface, `Manager` struct, `SpawnWindow()` with WM_CLASS, zellij session attachment, env var clearing |
| `window_test.go` | Tests for window spawning, env var isolation, and WM_CLASS configuration |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewManager` | `func NewManager(terminal string) *Manager` | `*Manager` | Creates window manager with terminal binary name |
| `SpawnWindow` | `func (m *Manager) SpawnWindow(ctx context.Context, opts WindowOpts) error` | `error` | Spawns wezterm with --class flag, optional zellij attach, or direct command |

## Data Types

### WindowManager (interface)
`SpawnWindow(ctx context.Context, opts WindowOpts) error`

### WindowOpts (struct)
Fields: Title, ZellijSession (attach to this session if set), Command (run this directly if ZellijSession is empty), Env (additional env vars), WMClass (X11 window class for tiling WM rules)

### Manager (struct)
Fields: terminal (binary name, typically "wezterm")

## Logging
- No structured logging; errors returned via `fmt.Errorf`

## CRUD Entry Points
- **Create**: `SpawnWindow()` creates a new terminal window
- **Read**: N/A
- **Update**: N/A
- **Delete**: N/A (windows are managed by the window manager)

## Style Guide
- Interface-first: `WindowManager` interface with `Manager` implementation
- Environment isolation: clears `ZELLIJ`, `ZELLIJ_SESSION_NAME`, `TERM_PROGRAM` to prevent nested session conflicts
- WM_CLASS set via `wezterm start --class <class>` for dwm/i3/sway integration
- Command execution detached from parent process

**Representative snippet (from `window.go`):**
```go
func (m *Manager) SpawnWindow(ctx context.Context, opts WindowOpts) error {
	args := []string{"start"}
	if opts.WMClass != "" {
		args = append(args, "--class", opts.WMClass)
	}
	if opts.ZellijSession != "" {
		args = append(args, "--", "zellij", "attach", opts.ZellijSession)
	} else if opts.Command != "" {
		args = append(args, "--", "sh", "-c", opts.Command)
	}
	cmd := exec.CommandContext(ctx, m.terminal, args...)
	// Clear env vars to prevent nested session conflicts
	cmd.Env = clearEnvVars(os.Environ(), "ZELLIJ", "ZELLIJ_SESSION_NAME")
	return cmd.Start()
}
```
