# internal/commands/ -- CLI Command Handlers

## Purpose
Bridges the Cobra CLI definitions in `cmd/cc/` with the internal service packages. Each file exports a function returning a `*cobra.Command` wired to the shared `App` struct. This is where all CLI business logic lives.

## Technology
- Go 1.25
- `github.com/spf13/cobra` for command definitions
- Depends on: `internal/agents`, `internal/config`, `internal/gateway`, `internal/mail`, `internal/merge`, `internal/platform/db`, `internal/tui`, `internal/watchdog`, `internal/wezterm`, `internal/worktree`, `internal/zellij`, `pkg/runtimes`

## Contents
| File | Description |
|------|-------------|
| `app.go` | `App` struct (shared dependency container), `NewApp()`, `Close()`, `RunDashboard()`, `RunWatchdog()`, `RunGateway()` |
| `sling.go` | `SlingCmd()` -- spawn a worker agent |
| `stop.go` | `StopCmd()` -- terminate an agent |
| `status.go` | `StatusCmd()` -- fleet status overview |
| `dashboard.go` | `DashboardCmd()` -- launch TUI or wezterm+zellij dashboard |
| `coordinator.go` | `CoordinatorCmd()`, `MonitorCmd()` -- orchestrator and monitor lifecycle |
| `mail.go` | `MailCmd()` -- send, check, list, read, reply, purge inter-agent messages |
| `nudge.go` | `NudgeCmd()` -- send nudge to agent's pane |
| `merge.go` | `MergeCmd()` -- enqueue, list, status, run merges |
| `group.go` | `GroupCmd()` -- task group CRUD |
| `inspect.go` | `InspectCmd()` -- deep agent session inspection |
| `observability.go` | `TraceCmd()`, `ErrorsCmd()`, `ReplayCmd()`, `FeedCmd()`, `LogsCmd()`, `CostsCmd()`, `MetricsCmd()`, `RunCmd()` |
| `watch.go` | `WatchCmd()` -- start watchdog daemon |
| `worktree.go` | `WorktreeCmd()` -- worktree list, status, clean, remove |
| `clean.go` | `CleanCmd()` -- resource cleanup (worktrees + mail) |
| `doctor.go` | `DoctorCmd()` -- health checks (config, db, git, zellij, project dir) |
| `feature.go` | `FeatureCmd()` -- runtime feature flag list/toggle |
| `commands_test.go` | Tests for command handlers |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewApp` | `func NewApp(configPath, version string) (*App, error)` | `*App, error` | Loads config, opens DB, runs migrations, wires all services |
| `SlingCmd` | `func SlingCmd(app *App) *cobra.Command` | `*cobra.Command` | Spawn agent with --task, --capability, --runtime flags |
| `StopCmd` | `func StopCmd(app *App) *cobra.Command` | `*cobra.Command` | Stop agent with --force, --reason flags |
| `StatusCmd` | `func StatusCmd(app *App) *cobra.Command` | `*cobra.Command` | List sessions with --capability, --state filters |
| `DashboardCmd` | `func DashboardCmd(app *App) *cobra.Command` | `*cobra.Command` | Dashboard with --tui flag; falls back to in-process TUI |
| `MailCmd` | `func MailCmd(app *App) *cobra.Command` | `*cobra.Command` | Mail subcommands: send/check/list/read/reply/purge |
| `MergeCmd` | `func MergeCmd(app *App) *cobra.Command` | `*cobra.Command` | Merge subcommands: enqueue/list/status/run |

## Data Types

### App (struct)
Central dependency container. Fields: Config, DB, Spawner, MailStore, MergeQueue, MergeExecutor, WorktreeManager, PaneManager, WindowManager, Watchdog, Gateway, Version

### eventRow, metricsRow, runRow, groupRow (internal query types)
Used by observability and group commands for DB result scanning.

## Logging
- User-facing output via `fmt.Printf` / `fmt.Println`
- JSON output via `json.NewEncoder(os.Stdout).Encode()` when `--json` flag is set
- Errors returned as `fmt.Errorf("context: %w", err)`

## CRUD Entry Points
- **Agents**: `cc sling` (create), `cc status` (read), `cc stop` (delete)
- **Mail**: `cc mail send` (create), `cc mail check/list` (read), `cc mail read` (update), `cc mail purge` (delete)
- **Merge**: `cc merge enqueue` (create), `cc merge list/status` (read), `cc merge run` (execute)
- **Groups**: `cc group create` (create), `cc group list/status` (read)
- **Worktrees**: `cc worktree list/status` (read), `cc worktree clean/remove` (delete)

## Style Guide
- Each command file exports one `XxxCmd(app *App) *cobra.Command` function
- Flags declared with `cmd.Flags().String/Bool/Int()` -- errors from `GetString()` are ignored via `_`
- JSON output guarded by `jsonOut, _ := cmd.Root().Flags().GetBool("json")`
- `truncate()` helper used for tabular output alignment
- Import order: stdlib, cobra, internal packages

**Representative snippet (from `sling.go`):**
```go
func SlingCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sling <name>",
		Short:   "Spawn worker agent",
		GroupID: "CORE",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			taskID, _ := cmd.Flags().GetString("task")
			capability, _ := cmd.Flags().GetString("capability")
			rt, _ := cmd.Flags().GetString("runtime")

			if taskID == "" {
				return fmt.Errorf("--task is required")
			}
			// ... spawn logic
		},
	}
	cmd.Flags().String("task", "", "Task ID (required)")
	return cmd
}
```
