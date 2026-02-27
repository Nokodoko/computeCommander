# cmdr Window Spawning Fix

Bug fix for ComputeCommander's dashboard and agent window spawning. Go/cobra CLI, wezterm/zellij/dwm stack, single-file change with config fix.

Fixes a regression where `cmdr dashboard` runs a bubbletea TUI in the current terminal instead of spawning a new wezterm X11 window running a zellij session with the dashboard layout.

## Why

The dashboard command is broken -- it renders an in-process bubbletea TUI instead of spawning the intended multi-pane zellij dashboard in a new dwm-managed window:

- **Dashboard runs in-process.** `DashboardCmd` calls `app.RunDashboard()` which starts a bubbletea `tea.Program` in the current terminal. The user sees a single-pane TUI instead of the 5-pane zellij layout (agent_picker, mail, merge_queue, event_log, cmdr_feed).
- **No wezterm window spawned.** The existing `SpawnDashboard()` method on the Spawner is never called. The wezterm `WindowManager` and its `SpawnWindow` method sit unused for the dashboard command.
- **Missing config field.** The project's `.computecommander/config.yaml` has no `dashboard_layout` key under `zellij:`, so even if `SpawnDashboard` were called, `s.zellijLayout` would be empty, producing a zellij session with no layout.
- **Layout file exists but is disconnected.** `~/.computecommander/layouts/cmdr-dashboard.kdl` is a valid 5-pane zellij layout, but nothing in the config points to it.

This is a one-line command change plus a config fix. The wezterm spawning infrastructure (`internal/wezterm/window.go`) is already correct and tested.

## Design Principles

1. **Minimal diff.** The wezterm `SpawnWindow`, the `Spawner.SpawnDashboard` method, and the zellij layout file all exist and work. The fix is to wire them together, not to rewrite anything.
2. **Preserve the bubbletea TUI as a fallback.** `RunDashboard` should remain available for headless/SSH scenarios where wezterm is not available. The dashboard command should attempt `SpawnDashboard` first and fall back to `RunDashboard` if `WindowManager` is nil.
3. **Config-driven behavior.** The `dashboard_layout` path must come from config, not be hardcoded. The default config already specifies `~/.computecommander/layouts/cmdr-dashboard.kdl`; the project config just needs the field populated.
4. **Detached spawn.** The `cmdr dashboard` command must exit immediately after spawning. It must not block waiting for the wezterm process. This is already how `SpawnWindow` works (`cmd.Start()` + `go cmd.Wait()`).
5. **Testable.** The fix must not break any existing tests. The mock-based `wezterm.CommandRunner` interface already covers the spawn path.

## On-Disk Format

### Relevant Files

```
computeCommander/
  .computecommander/
    config.yaml                         # Project config (MISSING dashboard_layout)
  internal/
    commands/
      dashboard.go                      # THE BUG: calls RunDashboard, should call SpawnDashboard
      app.go                            # Has both RunDashboard() and SpawnDashboard() via Spawner
    wezterm/
      window.go                         # SpawnWindow implementation (CORRECT, no changes needed)
      window_test.go                    # Tests for SpawnWindow (PASSING)
    agents/
      spawner.go                        # SpawnDashboard() method (CORRECT, no changes needed)
    zellij/
      pane.go                           # CreatePane for floating agent panes (CORRECT)
  ~/.computecommander/
    layouts/
      cmdr-dashboard.kdl                # Zellij layout (EXISTS, correct 5-pane layout)
```

### .computecommander/config.yaml

The project config is missing the `dashboard_layout` field. Currently:

```yaml
zellij:
    layout: default
    terminal: wezterm
    session_prefix: cc
```

Must become:

```yaml
zellij:
    dashboard_layout: "~/.computecommander/layouts/cmdr-dashboard.kdl"
    layout: default
    terminal: wezterm
    session_prefix: cc
```

### ~/.computecommander/layouts/cmdr-dashboard.kdl

The layout file is correct and needs no changes. It defines a 5-pane zellij layout:

```kdl
layout {
    tab name="[CMDR] Dashboard" {
        pane size=1 borderless=true {
            plugin location="zellij:tab-bar"
        }
        pane size="80%" split_direction="vertical" {
            pane size="80%" split_direction="horizontal" {
                pane size="54%" name="agent_picker" {
                    command "sh"
                    args "-c" "$HOME/.computecommander/hooks/agent-picker.sh"
                }
                pane size="45%" split_direction="vertical" {
                    pane size="52%" name="mail" { ... }
                    pane size="47%" name="merge_queue" { ... }
                }
            }
            pane size="19%" name="event_log" { ... }
        }
        pane size="20%" name="cmdr_feed" { ... }
    }
}
```

## Data Model

### Current vs Target Call Chain

The relevant Go types are already correct. This section documents the call chain to show where the break is.

#### Current (Broken)

```
DashboardCmd.RunE
  --> app.RunDashboard(ctx)
        --> tui.NewDashboard(opts)
        --> dash.Run(ctx)       // bubbletea tea.Program in current terminal
                                // BLOCKS until user quits
```

#### Target (Fixed)

```
DashboardCmd.RunE
  --> app.Spawner.SpawnDashboard(ctx)
        --> s.windows.SpawnWindow(ctx, opts)
              --> exec.Command("wezterm", "start", "--class", "cc-cc-dashboard",
                    "--", "sh", "-c", "zellij --session cc-dashboard --layout ...")
              --> cmd.Start()   // detached, non-blocking
              --> go cmd.Wait() // reap zombie
  --> return nil                // command exits immediately

  (fallback if WindowManager is nil)
  --> app.RunDashboard(ctx)     // bubbletea TUI as before
```

### Key Interfaces (No Changes Needed)

```typescript
// internal/wezterm/window.go
interface WindowManager {
  SpawnWindow(ctx: Context, opts: SpawnWindowOpts): error;
  FocusWindow(sessionName: string): error;
  ListWindows(): (Window[], error);
}

interface SpawnWindowOpts {
  ZellijSession: string;  // "cc-dashboard"
  WorkDir: string;        // "" for dashboard
  Layout: string;         // "~/.computecommander/layouts/cmdr-dashboard.kdl"
}
```

```typescript
// internal/agents/spawner.go - SpawnDashboard (already exists, line 86-97)
// Constructs session name as "{prefix}-dashboard" -> "cc-dashboard"
// Passes s.zellijLayout (from config) as Layout
// Calls s.windows.SpawnWindow()
```

### Agent Session State Lifecycle (Unchanged)

```
booting ──> working ──> completed
              │
              v
           stalled ──> zombie
```

### Window Spawn Lifecycle (New Documentation)

```
cmdr dashboard
  │
  ├── WindowManager != nil
  │     │
  │     ├── SpawnDashboard()
  │     │     │
  │     │     └── SpawnWindow(ctx, {session: "cc-dashboard", layout: "...kdl"})
  │     │           │
  │     │           ├── wezterm start --class cc-cc-dashboard -- sh -c "zellij ..."
  │     │           │     │
  │     │           │     ├── New X11 window (dwm client)
  │     │           │     └── zellij --session cc-dashboard --layout cmdr-dashboard.kdl
  │     │           │           ├── agent_picker pane
  │     │           │           ├── mail pane
  │     │           │           ├── merge_queue pane
  │     │           │           ├── event_log pane
  │     │           │           └── cmdr_feed pane
  │     │           │
  │     │           └── return nil (non-blocking)
  │     │
  │     └── cmdr exits
  │
  └── WindowManager == nil (fallback)
        │
        └── RunDashboard(ctx)  // bubbletea TUI, blocks
```

## CLI

Binary name: `cc` (ComputeCommander).

The only affected command:

### Dashboard Command

```
cc dashboard                           Launch the interactive dashboard
  (alias: cc dash)

  Behavior:
    1. If zellij.terminal == "wezterm" in config:
       Spawn new wezterm window with zellij layout.
       cc exits immediately.
    2. If zellij.terminal is not "wezterm" (or WindowManager is nil):
       Fall back to in-process bubbletea TUI.
       cc blocks until user quits.
```

### Agent Spawning (Unchanged, For Reference)

```
cc sling <agent-name>                  Spawn an agent
  --task <id>        (required)
  --capability <cap>
  --runtime <rt>

  Agent panes spawn as floating zellij panes WITHIN the dashboard session.
  This uses zellij.PaneManager.CreatePane with Floating=true.
  This behavior is CORRECT and not part of this fix.
```

## JSON Output Format

Not directly affected by this fix. For reference, the dashboard command produces no JSON output (it spawns a window and exits). If `--json` is passed:

Success (wezterm spawn):

```json
{ "success": true, "command": "dashboard", "session": "cc-dashboard", "terminal": "wezterm" }
```

Fallback (bubbletea TUI starts, no JSON emitted -- interactive mode).

Error:

```json
{ "success": false, "command": "dashboard", "error": "WindowManager not configured; set zellij.terminal=wezterm in config" }
```

## Concurrency Model

Not applicable to this fix. The wezterm spawn is a fire-and-forget `exec.Command.Start()`. There is no shared state, no locking, no contention. The spawned wezterm process is fully independent of the `cc` process.

### Process Detachment

The existing implementation in `wezterm.execRunner.Start()` (line 72-80 of `window.go`) is correct:

```go
func (e *execRunner) Start(ctx context.Context, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    if err := cmd.Start(); err != nil {
        return fmt.Errorf(...)
    }
    go cmd.Wait() // Detach - reap child without blocking
    return nil
}
```

No changes needed. The `go cmd.Wait()` prevents zombie processes while the 100ms sleep in `SpawnWindow` gives the X11 window time to appear.

## Migration

Not applicable. This is a bug fix, not a data migration. No data formats change.

### Config Migration

The only "migration" is adding the missing `dashboard_layout` field to the project config:

| Field | Current Value | Target Value |
|-------|--------------|--------------|
| `zellij.dashboard_layout` | (absent/empty) | `~/.computecommander/layouts/cmdr-dashboard.kdl` |

This is a one-line addition to `.computecommander/config.yaml`.

## Integration

### Dashboard Command Integration

The dashboard integrates three systems:

| System | Role | Status |
|--------|------|--------|
| `wezterm` | Terminal emulator, creates X11 window | CORRECT (SpawnWindow works) |
| `zellij` | Terminal multiplexer, manages panes from layout | CORRECT (layout file exists) |
| `dwm` | Window manager, manages X11 clients by WM_CLASS | CORRECT (--class flag set) |
| `cobra` command | Entry point, calls SpawnDashboard | **BROKEN** (calls RunDashboard instead) |
| `config.yaml` | Provides dashboard_layout path | **BROKEN** (field missing) |

### Verification Commands

```bash
# After fix: spawn dashboard and verify X11 window
cc dashboard
wmctrl -l -x | grep cc-          # Should show: cc-cc-dashboard.wezterm-gui

# Check wezterm process was spawned
ps aux | grep wezterm             # Should show wezterm with --class cc-cc-dashboard

# Check zellij session exists
zellij list-sessions              # Should show: cc-dashboard
```

### Agent-Facing Commands (Unchanged)

```bash
# Agent picker (inside dashboard zellij session) spawns floating panes
# This uses zellij.PaneManager.CreatePane with Floating=true
# This is CORRECT and not affected by this fix
zellij action new-pane --floating --name "builder-1" -- sh -c "claude --resume builder-1"
```

## What It Does NOT Do

Explicitly out of scope for this fix:

- **No refactoring of SpawnWindow.** The wezterm window spawning code is correct and tested. Do not modify `internal/wezterm/window.go`.
- **No refactoring of SpawnDashboard.** The spawner's `SpawnDashboard` method is correct. Do not modify `internal/agents/spawner.go`.
- **No changes to agent pane spawning.** Agent panes correctly spawn as floating zellij panes within the dashboard session via `zellij.PaneManager.CreatePane`. Do not touch `internal/zellij/pane.go`.
- **No changes to the bubbletea TUI.** `internal/tui/dashboard.go` remains as-is. It serves as the fallback when wezterm is not available.
- **No new dependencies.** This fix only changes call paths in existing code.
- **No layout file modifications.** `cmdr-dashboard.kdl` is correct.
- **No new tests required for the wezterm path.** The mock-based tests in `window_test.go` already cover `SpawnWindow`. A test for the command-level fallback behavior is recommended but optional.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Language | Go 1.22+ | Existing project language |
| CLI framework | cobra | Existing, all commands use it |
| Terminal emulator | wezterm | Existing config default, creates X11 windows |
| Multiplexer | zellij | Existing, manages panes via KDL layouts |
| Window manager | dwm | Existing, manages X11 clients by WM_CLASS |
| Testing | `go test` with mock CommandRunner | Existing pattern in `window_test.go` |
| Config | YAML via `gopkg.in/yaml.v3` | Existing config system |

## Project Infrastructure

### Files Modified by This Fix

```
computeCommander/
  internal/
    commands/
      dashboard.go                     # MODIFY: call SpawnDashboard instead of RunDashboard
  .computecommander/
    config.yaml                        # MODIFY: add dashboard_layout field
```

### Files NOT Modified (Verified Correct)

```
computeCommander/
  internal/
    wezterm/
      window.go                        # SpawnWindow is correct
      window_test.go                   # Tests pass
    agents/
      spawner.go                       # SpawnDashboard is correct
      spawner_test.go                  # Tests pass
    zellij/
      pane.go                          # Agent pane spawning is correct
    tui/
      dashboard.go                     # Bubbletea TUI (kept as fallback)
    config/
      config.go                        # DefaultConfig already has dashboard_layout default
  ~/.computecommander/
    layouts/
      cmdr-dashboard.kdl               # Layout file is correct
```

### Test Commands

```bash
# Run all tests to verify no regressions
cd /home/n0ko/Programs/ai/computeCommander && go test ./...

# Run specific package tests
go test ./internal/wezterm/...
go test ./internal/agents/...
go test ./internal/commands/...

# Manual verification after fix
cc dashboard
wmctrl -l -x | grep cc-
```

## Estimated Size

| Area | Files | LOC Changed |
|------|-------|-------------|
| Command fix (`dashboard.go`) | 1 | ~15 (replace RunE body with SpawnDashboard + fallback) |
| Config fix (`config.yaml`) | 1 | ~1 (add `dashboard_layout` line) |
| Optional: dashboard command test | 1 | ~30 (test SpawnDashboard is called when WindowManager exists) |
| **Total** | **2-3** | **~16-46** |

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Modify `internal/commands/dashboard.go` to call `SpawnDashboard` with fallback | `unix-coder` | Single-file Go change, straightforward |
| Add `dashboard_layout` to `.computecommander/config.yaml` | `unix-coder` | Config file edit, same worker |
| Run `go test ./...` to verify no regressions | `unix-coder` | Same worker, immediate feedback |
| Optional: Add test for dashboard command fallback logic | `unix-coder` | Test is in same package, same worker |
| Code review of the change | `code-review` | Verify correctness and no unintended side effects |

## Execution Order

```
Phase 1: Implementation
  ├── Modify dashboard.go (agent: unix-coder)
  └── Add dashboard_layout to config.yaml (agent: unix-coder)  [parallel, same worker]

Phase 2: Verification [blocked by Phase 1]
  └── Run go test ./... (agent: unix-coder)

Phase 3: Review [blocked by Phase 2]
  └── Code review (agent: code-review)
```

Recommended directive: `/pai` -- This is a plan-then-implement pipeline. The plan is this spec; implementation is two small edits.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| `dashboard_layout` path does not expand `~` | zellij fails to find layout file | Use `os.UserHomeDir()` to expand `~` before passing to SpawnWindow, or use absolute path in config |
| wezterm not installed | `exec.Command("wezterm", ...)` returns `exec not found` | Fallback to `RunDashboard` (bubbletea TUI) -- this is the designed fallback path |
| `WindowManager` is nil because `zellij.terminal != "wezterm"` | `SpawnDashboard` returns error | Fallback to `RunDashboard` -- dashboard command should check `app.Spawner` or `app.WindowManager` before calling |
| zellij session already exists | zellij reattaches to existing session (correct behavior) | No recovery needed -- zellij handles this gracefully |
| Layout file missing at path | zellij starts with default layout (no panes) | Log warning, surface to user. The layout file should be deployed during `cc init` or documented as a prerequisite |
| Tests break due to nil `Spawner` in test `App` | `go test ./internal/commands/...` fails | The dashboard command must handle nil `Spawner` gracefully (fall back to `RunDashboard`) |

## Success Criteria

Measurable outcomes that define "done" for this fix:

- [ ] `cc dashboard` spawns a new wezterm X11 window (verified via `wmctrl -l -x | grep cc-`)
- [ ] The spawned window runs zellij with the `cmdr-dashboard.kdl` layout (5 panes visible)
- [ ] The `cc dashboard` process exits immediately after spawning (does not block)
- [ ] `dwm` manages the new window as a separate client (tiled or floating per dwm rules)
- [ ] `.computecommander/config.yaml` contains `dashboard_layout` pointing to the KDL file
- [ ] `go test ./...` passes with zero failures
- [ ] If `WindowManager` is nil (no wezterm configured), `cc dashboard` falls back to the bubbletea TUI
- [ ] No changes to `internal/wezterm/window.go`, `internal/agents/spawner.go`, or `internal/zellij/pane.go`

## Outcome

- `cc dashboard` spawns a new wezterm X11 window visible in `wmctrl -l -x` output as `cc-cc-dashboard`
- The spawned wezterm window contains a zellij session with the 5-pane `cmdr-dashboard.kdl` layout
- `cc dashboard` exits immediately after spawn (non-blocking)
- `.computecommander/config.yaml` contains `dashboard_layout` field pointing to the KDL layout file
- `go test ./...` passes with zero failures and zero new test breakages
- Fallback to bubbletea TUI works when `WindowManager` is nil (no wezterm configured)
- No modifications to `internal/wezterm/window.go`, `internal/agents/spawner.go`, or `internal/zellij/pane.go`
- No new runtime dependencies introduced
- No hardcoded paths — layout path comes from config

## Implementation Details

### Change 1: `internal/commands/dashboard.go`

**Current code** (file: `/home/n0ko/Programs/ai/computeCommander/internal/commands/dashboard.go`):

```go
func DashboardCmd(app *App) *cobra.Command {
    return &cobra.Command{
        Use:     "dashboard",
        Short:   "Live TUI dashboard",
        Long:    "Launch the interactive terminal dashboard for monitoring agents, mail, and merge queue.",
        GroupID: "CORE",
        Aliases: []string{"dash"},
        RunE: func(cmd *cobra.Command, args []string) error {
            return app.RunDashboard(cmd.Context())
        },
    }
}
```

**Target code:**

```go
func DashboardCmd(app *App) *cobra.Command {
    return &cobra.Command{
        Use:     "dashboard",
        Short:   "Launch the cmdr dashboard in a new window",
        Long:    "Spawn a new wezterm window running the zellij dashboard layout with agent picker, mail, merge queue, events, and feed panes.",
        GroupID: "CORE",
        Aliases: []string{"dash"},
        RunE: func(cmd *cobra.Command, args []string) error {
            // Prefer spawning a new wezterm window with the zellij dashboard layout.
            // Falls back to the in-process bubbletea TUI if WindowManager is not configured.
            if app.Spawner != nil {
                err := app.Spawner.SpawnDashboard(cmd.Context())
                if err == nil {
                    return nil
                }
                // If SpawnDashboard fails because WindowManager is nil,
                // fall through to the bubbletea TUI fallback.
                // For other errors, return them.
                if app.WindowManager != nil {
                    return err
                }
            }
            return app.RunDashboard(cmd.Context())
        },
    }
}
```

The key logic: try `SpawnDashboard` first. If it fails because there is no `WindowManager` (the error from `spawner.go` line 88), fall back to the bubbletea TUI. If it fails for any other reason (wezterm not found, spawn error), surface the error.

### Change 2: `.computecommander/config.yaml`

Add `dashboard_layout` under the `zellij:` section:

```yaml
zellij:
    dashboard_layout: "~/.computecommander/layouts/cmdr-dashboard.kdl"
    layout: default
    terminal: wezterm
    session_prefix: cc
```

This matches the default value in `config.DefaultConfig()` (line 171 of `config.go`):
```go
DashboardLayout: "~/.computecommander/layouts/cmdr-dashboard.kdl",
```

### Tilde Expansion Concern

The `SpawnWindow` method passes the layout path directly to the shell via `sh -c "zellij --session ... --layout ..."`. The shell will expand `~` in the layout path. However, if the path is passed as a Go string to `exec.Command`, tilde expansion does NOT happen. Looking at the current code in `window.go` (line 129):

```go
zellijCmd := strings.Join(zellijArgs, " ")
// ...
args = append(args, "--")
args = append(args, "sh", "-c", zellijCmd)
```

The command is passed through `sh -c`, so `~` WILL be expanded by the shell. This is correct and no additional handling is needed.

### WM_CLASS Naming

The `SpawnWindow` method constructs `wmClass` as `{classPrefix}-{ZellijSession}`. With `classPrefix="cc"` and `ZellijSession="cc-dashboard"`, the WM_CLASS will be `cc-cc-dashboard`. This is slightly redundant but correct -- dwm will see it as `cc-cc-dashboard.wezterm-gui` in `wmctrl -l -x` output.
