# cmd/cc/ -- CLI Entry Point

## Purpose
The `main` package and CLI entry point for ComputeCommander (`cmdr`). Defines the root Cobra command tree with 11 command groups, the `init` and `config` subcommands (which do not require an initialised project), and wires all other commands through the shared `App` struct. The root command launches the zellij dashboard by default or falls back to the in-process BubbleTea TUI.

## Technology
- Go 1.25
- `github.com/spf13/cobra` for CLI framework
- `gopkg.in/yaml.v3` for config serialization
- `internal/commands.App` as shared dependency container
- `internal/config` for config loading
- `internal/keybinds` for keybind generation on init
- `internal/platform/db` for database creation and migration on init
- `internal/zellij` for KDL layout generation and session launching

## Contents
| File | Description |
|------|-------------|
| `main.go` | Entry point: `rootCmd()`, `initCmd()`, `configCmd()`, `appPreRun`, version/commit injection via ldflags, zellij dashboard launch logic, agent wrapper generation |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `main` | `func main()` | - | Entry point; executes root command |
| `rootCmd` | `func rootCmd() *cobra.Command` | `*cobra.Command` | Builds the full CLI command tree with 11 groups (LIFECYCLE, CORE, INFO, DATA, COORDINATION, MESSAGING, MERGE, GROUPS, OBSERVABILITY, SETTINGS, INFRASTRUCTURE) |
| `appPreRun` | `func appPreRun(cmd *cobra.Command, args []string) error` | `error` | Lazy-initialises `sharedApp` before RunE |
| `addAppCmd` | `func addAppCmd(root *cobra.Command, cmd *cobra.Command)` | - | Attaches appPreRun and registers subcommand |
| `initCmd` | `func initCmd() *cobra.Command` | `*cobra.Command` | Project initialisation (creates `.computecommander/`) |
| `runInit` | `func runInit(cmd *cobra.Command, args []string) error` | `error` | Creates dirs (incl. themes, backups, plugins), config, rules, keybinds, default theme, KDL layout, and SQLite schema |
| `configCmd` | `func configCmd() *cobra.Command` | `*cobra.Command` | Config subcommand group (show/validate/get/set/edit) |
| `loadProjectConfig` | `func loadProjectConfig() (*config.Config, error)` | `*config.Config, error` | Loads config from `.computecommander/config.yaml` |
| `lookupKey` | `func lookupKey(m map[string]any, key string) any` | `any` | Dot-notation key lookup in nested maps |
| `setKey` | `func setKey(m map[string]any, key string, value string)` | - | Dot-notation key set in nested maps |

## Data Types

| Type | Kind | Description |
|------|------|-------------|
| `sharedApp` | `*commands.App` (package var) | Singleton App instance, zero-valued at build, populated by `appPreRun` |
| `appInitialised` | `bool` (package var) | Guards against double-init |
| `version` / `commit` | `string` (package vars) | Injected via `-ldflags` at build time |
| `usageTemplate` | `string` (package var) | Custom Cobra usage template with grouped command display |

## Logging
- Errors printed to `os.Stderr` via `fmt.Fprintf`
- `SilenceUsage` and `SilenceErrors` are enabled on root command

## CRUD Entry Points
- **Create**: `cmdr init [--name X] [--db sqlite|postgres]` -- creates `.computecommander/` directory tree, config, rules, keybinds, theme, layout, and database
- **Read**: `cmdr config show`, `cmdr config get <key>` -- reads and displays config
- **Update**: `cmdr config set <key> <value>`, `cmdr config edit` -- modifies config
- **Delete**: N/A (no delete command exists)

## Command Groups (registered in rootCmd)
- **LIFECYCLE**: shutdown, reset, restart
- **CORE**: sling, stop, status, dashboard, shell, feedback, support, fp, session, sessions
- **INFO**: help, docs, version, update
- **DATA**: export, backup, restore
- **COORDINATION**: coordinator, monitor
- **MESSAGING**: mail, nudge
- **MERGE**: merge
- **GROUPS**: group
- **OBSERVABILITY**: inspect, trace, errors, replay, feed, logs, costs, metrics, run, clear
- **SETTINGS**: theme, notifications, analytics, integrations, automation
- **INFRASTRUCTURE**: worktree, watch, doctor, clean, feature

## Style Guide
- PascalCase for exported functions, camelCase for unexported
- Cobra commands built as functions returning `*cobra.Command`
- Flag retrieval uses `cmd.Flags().GetString()` pattern (errors ignored)
- App-dependent commands attached via `addAppCmd` helper
- Standard Go import grouping: stdlib, external, internal
- Root command `RunE` handles zellij detection, layout regeneration, agent wrapper generation, lock file writing, and TUI fallback
- Custom `usageTemplate` for grouped command display in help output

**Representative snippet (from `main.go`):**
```go
func addAppCmd(root *cobra.Command, cmd *cobra.Command) {
	cmd.PersistentPreRunE = appPreRun
	root.AddCommand(cmd)
}

func appPreRun(cmd *cobra.Command, args []string) error {
	if appInitialised {
		return nil
	}
	configPath := filepath.Join(".computecommander", "config.yaml")
	real, err := commands.NewApp(configPath, version)
	if err != nil {
		return err
	}
	*sharedApp = *real
	appInitialised = true
	return nil
}
```
