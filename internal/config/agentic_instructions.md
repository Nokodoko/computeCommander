# internal/config/ -- Configuration Management

## Purpose
Defines the full configuration schema for ComputeCommander (spec section 6.1), including loading from YAML with environment variable expansion, local overlay merging, tilde expansion, and validation.

## Technology
- Go 1.25
- `gopkg.in/yaml.v3` for YAML serialization and deserialization
- `github.com/fsnotify/fsnotify` for file system change monitoring (watcher)

## Contents
| File | Description |
|------|-------------|
| `config.go` | Full `Config` struct hierarchy, `DefaultConfig()`, `LoadConfig()` (with env var and tilde expansion, local overlay merge), `Validate()` |
| `config_test.go` | Tests for defaults, validation, env var expansion, and local overlay merging |
| `watcher.go` | `Watcher` struct: fsnotify-based config file watcher with auto-reload, change handlers, thread-safe access |
| `watcher_test.go` | Tests for watcher initialization, file change detection, handler notification, and close lifecycle |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `DefaultConfig` | `func DefaultConfig() *Config` | `*Config` | Returns config with sensible defaults per spec 6.1 |
| `LoadConfig` | `func LoadConfig(path string) (*Config, error)` | `*Config, error` | Loads YAML, expands `${VAR}` env vars, merges `config.local.yaml` overlay, expands `~/` in paths |
| `Validate` | `func (c *Config) Validate() error` | `error` | Validates all sections: version, driver, agents, zellij layout, runtime, logging level/format |
| `expandEnvVars` | `func expandEnvVars(data []byte) []byte` | `[]byte` | Replaces `${VAR}` patterns with environment variable values |
| `expandTilde` | `func expandTilde(path string) string` | `string` | Replaces leading `~/` with user home directory |
| `NewWatcher` | `func NewWatcher(path string) (*Watcher, error)` | `*Watcher, error` | Creates fsnotify watcher, loads initial config, starts watching for changes |
| `Watcher.OnChange` | `func (w *Watcher) OnChange(handler ConfigChangeHandler)` | - | Registers a handler called on successful config reload |
| `Watcher.Config` | `func (w *Watcher) Config() *Config` | `*Config` | Returns current config (thread-safe via RWMutex) |
| `Watcher.Close` | `func (w *Watcher) Close() error` | `error` | Stops the file watcher goroutine |

## Data Types

### Config (struct, top-level)
Fields: Version, Project, Database, Zellij, Agents, Worktrees, Defaults, Nudge, Watchdog, Merge, Features, Logging, Runtimes

### ProjectConfig (struct)
Fields: Name, Root, CanonicalBranch, QualityGates ([]QualityGate)

### QualityGate (struct)
Fields: Name, Command, Description

### DatabaseConfig (struct)
Fields: Driver (`"postgres"` | `"sqlite"`), Postgres (PostgresConfig), SQLite (SQLiteConfig)

### AgentsConfig (struct)
Fields: MaxConcurrent, StaggerDelayMs, MaxDepth, MaxSessionsPerRun, MaxAgentsPerLead, BaseDir

### DefaultsConfig (struct)
Fields: Runtime (string), AgentCommand (string -- default command for agent pane), ModelMappings (map[string]string)

### LoopDetection (struct)
Fields: Enabled (bool), Window (string), Threshold (int)

### NudgeConfig (struct)
Fields: SoftTimeout, HardTimeout, EscalationEnabled, ContextWindow, LoopDetection (LoopDetection)

### RuntimesConfig (struct)
Fields: Claude, Gemini, Codex (RuntimeConfig), Pi (PiConfig), Goose (RuntimeConfig)

### Watcher (struct)
Fields: path (string), current (*Config), mu (sync.RWMutex), handlers ([]ConfigChangeHandler), watcher (*fsnotify.Watcher), done (chan struct{})

### ConfigChangeHandler (func type)
`func(newConfig *Config)` -- callback invoked when config file changes and reloads successfully

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes
- Watcher uses `log.Printf` for reload failures and fsnotify errors

## CRUD Entry Points
- **Create**: `DefaultConfig()` creates a new config with defaults
- **Read**: `LoadConfig(path)` reads and parses config from YAML file
- **Update**: `LoadConfig` merges `config.local.yaml` overlay onto base config
- **Delete**: N/A

## Style Guide
- PascalCase exports, camelCase internals
- Nested struct hierarchy mirrors YAML structure
- YAML tags use `yaml:"snake_case"`
- Validation accumulates all errors into a single joined string
- Import order: stdlib, yaml

**Representative snippet (from `config.go`):**
```go
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	data = expandEnvVars(data)

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Look for config.local.yaml overlay in the same directory
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	localPath := filepath.Join(dir, name+".local"+ext)

	if localData, err := os.ReadFile(localPath); err == nil {
		localData = expandEnvVars(localData)
		if err := yaml.Unmarshal(localData, cfg); err != nil {
			return nil, fmt.Errorf("parse local config %s: %w", localPath, err)
		}
	}

	cfg.Zellij.DashboardLayout = expandTilde(cfg.Zellij.DashboardLayout)
	cfg.Database.SQLite.Path = expandTilde(cfg.Database.SQLite.Path)
	cfg.Worktrees.BaseDir = expandTilde(cfg.Worktrees.BaseDir)

	return cfg, nil
}
```
