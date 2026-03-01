# internal/commands/ -- CLI Command Handlers

## Purpose
Bridges the Cobra CLI definitions in `cmd/cc/` with the internal service packages. Each file exports a function returning a `*cobra.Command` wired to the shared `App` struct. This is where all CLI business logic lives.

## Technology
- Go 1.25
- `github.com/spf13/cobra` for command definitions
- `gopkg.in/yaml.v3` for config manipulation in settings commands
- `github.com/pkg/browser` for opening URLs in default browser
- Depends on: `internal/agents`, `internal/backup`, `internal/config`, `internal/export`, `internal/gateway`, `internal/keybinds`, `internal/mail`, `internal/merge`, `internal/platform/db`, `internal/tui`, `internal/watchdog`, `internal/wezterm`, `internal/worktree`, `internal/zellij`, `pkg/runtimes`

## Contents
| File | Description |
|------|-------------|
| `app.go` | `App` struct (shared dependency container), `NewApp()`, `Close()`, `RunDashboard()`, `RunWatchdog()`, `RunGateway()` |
| `sling.go` | `SlingCmd()` -- spawn a worker agent |
| `stop.go` | `StopCmd()` -- terminate an agent |
| `status.go` | `StatusCmd()` -- fleet status overview with DB/UI detection, styled `--pane` mode for zellij dashboard |
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
| `clear.go` | `ClearCmd()` -- clear DB event logs (distinct from clean) |
| `doctor.go` | `DoctorCmd()` -- health checks (config, db, git, zellij, project dir) |
| `feature.go` | `FeatureCmd()` -- runtime feature flag list/toggle |
| `lifecycle.go` | `ShutdownCmd()`, `ResetCmd()`, `RestartCmd()`, `confirmAction()` -- lifecycle management commands |
| `info.go` | `HelpCmd()`, `DocsCmd()`, `VersionCmd()`, `UpdateCmd()` -- information and help commands |
| `data.go` | `ExportCmd()`, `BackupCmd()`, `RestoreCmd()` -- data management commands |
| `session.go` | `SessionCmd()`, `SessionListCmd()`, `SessionSwitchCmd()`, `SessionStopCmd()`, `FpCmd()` -- directory session management |
| `session_picker.go` | `SessionsCmd()` -- Claude session picker via gum filter for session resumption |
| `settings.go` | `ThemeCmd()`, `NotificationsCmd()`, `AnalyticsCmd()`, `IntegrationsCmd()`, `AutomationCmd()` -- settings and configuration commands |
| `utility.go` | `ShellCmd()`, `FeedbackCmd()`, `SupportCmd()` -- utility commands for shell, feedback, support |
| `commands_test.go` | Tests for command handlers |
| `integration_test.go` | Integration tests: build verification, file structure checks, session manager, go vet |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewApp` | `func NewApp(configPath, version string) (*App, error)` | `*App, error` | Loads config, opens DB, runs migrations, wires all services |
| `SlingCmd` | `func SlingCmd(app *App) *cobra.Command` | `*cobra.Command` | Spawn agent with --task, --capability, --runtime flags |
| `StopCmd` | `func StopCmd(app *App) *cobra.Command` | `*cobra.Command` | Stop agent with --force, --reason flags |
| `StatusCmd` | `func StatusCmd(app *App) *cobra.Command` | `*cobra.Command` | List sessions with --capability, --state, --pane filters; detects DB/UI status; terminal-height-aware pane output |
| `DashboardCmd` | `func DashboardCmd(app *App) *cobra.Command` | `*cobra.Command` | Dashboard with --tui flag; falls back to in-process TUI |
| `MailCmd` | `func MailCmd(app *App) *cobra.Command` | `*cobra.Command` | Mail subcommands: send/check/list/read/reply/purge |
| `MergeCmd` | `func MergeCmd(app *App) *cobra.Command` | `*cobra.Command` | Merge subcommands: enqueue/list/status/run |
| `ShutdownCmd` | `func ShutdownCmd(app *App) *cobra.Command` | `*cobra.Command` | Stop DB + close UI (confirmation-gated) |
| `ResetCmd` | `func ResetCmd(app *App) *cobra.Command` | `*cobra.Command` | Reset DB to empty + close UI (confirmation-gated) |
| `RestartCmd` | `func RestartCmd(app *App) *cobra.Command` | `*cobra.Command` | Restart DB + UI (confirmation-gated) |
| `HelpCmd` | `func HelpCmd(app *App) *cobra.Command` | `*cobra.Command` | Show help in floating pane or stdout |
| `DocsCmd` | `func DocsCmd(app *App) *cobra.Command` | `*cobra.Command` | Open documentation in default browser |
| `VersionCmd` | `func VersionCmd(app *App) *cobra.Command` | `*cobra.Command` | Show version + release notes link |
| `UpdateCmd` | `func UpdateCmd(app *App) *cobra.Command` | `*cobra.Command` | Check for updates |
| `ExportCmd` | `func ExportCmd(app *App) *cobra.Command` | `*cobra.Command` | Export DB data as JSON/CSV with --format, --output, --tables flags |
| `BackupCmd` | `func BackupCmd(app *App) *cobra.Command` | `*cobra.Command` | Backup DB file (confirmation-gated) |
| `RestoreCmd` | `func RestoreCmd(app *App) *cobra.Command` | `*cobra.Command` | Restore DB from backup (confirmation-gated) |
| `ClearCmd` | `func ClearCmd(app *App) *cobra.Command` | `*cobra.Command` | Clear DB event logs (confirmation-gated) |
| `SessionCmd` | `func SessionCmd(app *App) *cobra.Command` | `*cobra.Command` | Session management: list/switch/stop subcommands |
| `SessionsCmd` | `func SessionsCmd(app *App) *cobra.Command` | `*cobra.Command` | Claude session picker via gum filter; writes switch file for agent wrapper |
| `listClaudeSessions` | `func listClaudeSessions() ([]claudeSession, error)` | `[]claudeSession, error` | Discovers Claude sessions from `~/.claude/projects/` via sessions-index.json and .jsonl files |
| `parseSessionJSONL` | `func parseSessionJSONL(path string) *claudeSession` | `*claudeSession` | Parses a Claude .jsonl transcript to extract session metadata |
| `gumFilter` | `func gumFilter(items []string, header string) (string, error)` | `string, error` | Runs `gum filter` for interactive fuzzy selection; handles Ctrl+C gracefully |
| `FpCmd` | `func FpCmd(app *App) *cobra.Command` | `*cobra.Command` | Open/toggle file picker pane |
| `ShellCmd` | `func ShellCmd(app *App) *cobra.Command` | `*cobra.Command` | Open shell in cmdr interface pane |
| `ThemeCmd` | `func ThemeCmd(app *App) *cobra.Command` | `*cobra.Command` | Theme management: list/set/edit subcommands |
| `AnalyticsCmd` | `func AnalyticsCmd(app *App) *cobra.Command` | `*cobra.Command` | Usage analytics dashboard |
| `confirmAction` | `func confirmAction(prompt string) bool` | `bool` | Interactive y/N confirmation prompt |
| `truncate` | `func truncate(s string, maxLen int) string` | `string` | Truncate string with ".." suffix for tabular alignment |
| `printAgentsPane` | `func printAgentsPane(sessions []*agents.AgentSession) error` | `error` | Styled ANSI output for zellij Agents pane; filters stale completed agents, detects terminal height |
| `detectUIStatus` | `func detectUIStatus(app *App) (bool, string, int64)` | `bool, string, int64` | Detect zellij UI session via lock file + PID check |
| `dashboardStartTime` | `func dashboardStartTime() time.Time` | `time.Time` | Returns mtime of cmdr.lock file for session boundary detection |
| `filterPaneSessions` | `func filterPaneSessions(sessions []*agents.AgentSession, dashStart time.Time) []*agents.AgentSession` | `[]*agents.AgentSession` | Filters out completed agents from previous dashboard sessions |
| `stateStyle` | `func stateStyle(state agents.SessionState) (string, string)` | `string, string` | Returns Unicode icon and ANSI color for agent state |
| `formatAgentDuration` | `func formatAgentDuration(s *agents.AgentSession) string` | `string` | Returns human-readable duration (e.g., "3m42s") since session start |
| `terminalHeight` | `func terminalHeight() int` | `int` | Returns terminal row count via TIOCGWINSZ ioctl, 0 if unavailable |
| `isProcessAlive` | `func isProcessAlive(pid int) bool` | `bool` | Checks PID liveness via `kill -0` |

## Data Types

### App (struct)
Central dependency container. Fields: Config, DB, Spawner, MailStore, MergeQueue, MergeExecutor, WorktreeManager, PaneManager, WindowManager, Watchdog, Gateway, Version

### eventRow, metricsRow, runRow, groupRow (internal query types)
Used by observability and group commands for DB result scanning.

### claudeSession (struct, in session_picker.go)
Fields: SessionID, ProjectPath, SessionName, Modified (float64). Represents a Claude Code session for the session picker.

## Logging
- User-facing output via `fmt.Printf` / `fmt.Println`
- JSON output via `json.NewEncoder(os.Stdout).Encode()` when `--json` flag is set
- Errors returned as `fmt.Errorf("context: %w", err)`
- Styled ANSI output in `--pane` mode for zellij dashboard integration with Unicode state icons and ANSI color constants
- Terminal-height-aware rendering via TIOCGWINSZ ioctl for pane output truncation

## CRUD Entry Points
- **Agents**: `cc sling` (create), `cc status` (read), `cc stop` (delete)
- **Mail**: `cc mail send` (create), `cc mail check/list` (read), `cc mail read` (update), `cc mail purge` (delete)
- **Merge**: `cc merge enqueue` (create), `cc merge list/status` (read), `cc merge run` (execute)
- **Groups**: `cc group create` (create), `cc group list/status` (read)
- **Worktrees**: `cc worktree list/status` (read), `cc worktree clean/remove` (delete)
- **Lifecycle**: `cc shutdown` (stop), `cc reset` (delete all data), `cc restart` (restart)
- **Data**: `cc export` (read/export), `cc backup` (create), `cc restore` (update)
- **Sessions**: `cc session list` (read), `cc session switch` (update), `cc session stop` (delete), `cc fp` (read/navigate)
- **Settings**: `cc theme list/set/edit` (read/update), `cc notifications show/set` (read/update)
- **Logs**: `cc clear` (delete event logs)

## Style Guide
- Each command file exports one or more `XxxCmd(app *App) *cobra.Command` functions
- Flags declared with `cmd.Flags().String/Bool/Int()` -- errors from `GetString()` are ignored via `_`
- JSON output guarded by `jsonOut, _ := cmd.Root().Flags().GetBool("json")`
- Confirmation-gated destructive operations use `confirmAction()` with `--force` bypass
- `truncate()` helper used for tabular output alignment
- Import order: stdlib, cobra, internal packages

**Representative snippet (from `lifecycle.go`):**
```go
func ShutdownCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shutdown",
		Aliases: []string{},
		Short:   "Stop DB + close UI (confirmation-gated)",
		Long:    "Shut down the ComputeCommander database and close the zellij UI session.\nRequires confirmation unless --force is specified.",
		GroupID: "LIFECYCLE",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("Are you sure you want to stop cmdr?") {
					fmt.Println("Cancelled.")
					return nil
				}
			}
			// ... shutdown logic
		},
	}
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")
	return cmd
}
```
