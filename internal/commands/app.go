// Package commands provides CLI command handler implementations for ComputeCommander.
// It bridges the Cobra CLI definitions in cmd/cc/ with the internal service packages.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/internal/agentic/block"
	"github.com/noko/computecommander/internal/agentic/blueprint"
	"github.com/noko/computecommander/internal/agentic/gate"
	"github.com/noko/computecommander/internal/agentic/holdout"
	"github.com/noko/computecommander/internal/agentic/isolation"
	"github.com/noko/computecommander/internal/agentic/trace"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/gateway"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/internal/tui"
	"github.com/noko/computecommander/internal/watchdog"
	"github.com/noko/computecommander/internal/wezterm"
	"github.com/noko/computecommander/internal/worktree"
	"github.com/noko/computecommander/internal/zellij"
	"github.com/noko/computecommander/pkg/runtimes"

	// Import runtime packages for side-effect registration.
	_ "github.com/noko/computecommander/pkg/runtimes/claude"
	_ "github.com/noko/computecommander/pkg/runtimes/codex"
	_ "github.com/noko/computecommander/pkg/runtimes/gemini"
	_ "github.com/noko/computecommander/pkg/runtimes/goose"
	_ "github.com/noko/computecommander/pkg/runtimes/pi"
)

// App holds the initialised service dependencies shared by all command handlers.
type App struct {
	Config          *config.Config
	DB              db.DB
	Spawner         *agents.Spawner
	MailStore       mail.MailStore
	MergeQueue      *merge.SQLQueue
	MergeExecutor   *merge.MergeExecutor
	WorktreeManager worktree.WorktreeManager
	PaneManager     zellij.PaneManager
	WindowManager   wezterm.WindowManager
	Watchdog        *watchdog.Watchdog
	Gateway         *gateway.Gateway
	Version         string

	// Agentic foundation engines.
	TraceEngine     *trace.TraceEngine
	BlockEngine     *block.BlockRuleEngine
	BlueprintEngine *blueprint.BlueprintEngine
	GatePipeline    *gate.GatePipeline
	HoldoutEngine   *holdout.HoldoutEngine
	ManifestStore   *isolation.ManifestStore
}

// NewAppSystemWide constructs an App using the system-wide config loading path.
// It loads ~/.computecommander/config.yaml, then overlays per-project config from projectPath.
func NewAppSystemWide(projectPath, version string) (*App, error) {
	cfg, err := config.LoadSystemConfig(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load system config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return newAppFromConfig(cfg, version)
}

// NewApp constructs an App by reading the project config and initialising all services.
// configPath is the path to the YAML config file (typically .computecommander/config.yaml).
func NewApp(configPath, version string) (*App, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return newAppFromConfig(cfg, version)
}

// newAppFromConfig constructs an App from a fully loaded and validated Config.
func newAppFromConfig(cfg *config.Config, version string) (*App, error) {
	dbCfg := db.DatabaseConfig{
		Driver: cfg.Database.Driver,
		Postgres: db.PostgresConfig{
			Host:     cfg.Database.Postgres.Host,
			Port:     cfg.Database.Postgres.Port,
			Database: cfg.Database.Postgres.Database,
			User:     cfg.Database.Postgres.User,
			Password: cfg.Database.Postgres.Password,
			SSLMode:  cfg.Database.Postgres.SSLMode,
			PoolSize: cfg.Database.Postgres.PoolSize,
		},
	}
	dbCfg.SQLite.Path = cfg.Database.SQLite.Path

	database, err := db.NewDB(dbCfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Migrate(database, cfg.Database.Driver); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	panes := zellij.NewManager(cfg.Zellij.SessionPrefix)
	wt := worktree.NewManager(cfg.Worktrees.BaseDir)

	// Create WindowManager if terminal is wezterm (spawns new dwm clients).
	var windows wezterm.WindowManager
	if cfg.Zellij.Terminal == "wezterm" {
		windows = wezterm.NewManager(cfg.Zellij.SessionPrefix)
	}

	// Resolve the cmdr binary path so the dashboard TUI can self-spawn.
	cmdrBin, err := os.Executable()
	if err != nil || cmdrBin == "" {
		cmdrBin = "cmdr"
	}

	spawner := agents.NewSpawner(agents.SpawnerOpts{
		DB:              database,
		PaneManager:     panes,
		WindowManager:   windows,
		WorktreeManager: wt,
		GetRuntime:      runtimes.GetRuntime,
		WorktreeBaseDir: cfg.Worktrees.BaseDir,
		MaxDepth:        cfg.Agents.MaxDepth,
		MaxConcurrent:   cfg.Agents.MaxConcurrent,
		ZellijLayout:    cfg.Zellij.DashboardLayout,
		SessionPrefix:   cfg.Zellij.SessionPrefix,
		CmdrBinary:      cmdrBin,
	})

	mailStore := mail.NewMailStore(database, nil)

	mq := merge.NewSQLQueue(database)
	executor := merge.NewMergeExecutorWithQueue(mq, ".", nil, cfg.Merge.AIResolveEnabled, cfg.Merge.ReimagineEnabled)

	watchdogOpts := watchdog.WatchdogOpts{
		DB:          database,
		MailStore:   mailStore,
		PaneManager: panes,
		WatchdogCfg: cfg.Watchdog,
		NudgeCfg:    cfg.Nudge,
	}

	// Wire pane healer into the watchdog if enabled in config.
	if cfg.Watchdog.PaneHealer.Enabled {
		watchdogOpts.PaneHealerOpts = &watchdog.PaneHealerOpts{
			PaneManager:     panes,
			CheckInterval:   time.Duration(cfg.Watchdog.PaneHealer.CheckIntervalMs) * time.Millisecond,
			FrozenThreshold: time.Duration(cfg.Watchdog.PaneHealer.FrozenThresholdMs) * time.Millisecond,
			MaxRestarts:     cfg.Watchdog.PaneHealer.MaxRestarts,
		}
	}

	wd := watchdog.NewWatchdog(watchdogOpts)

	// Create OpenBrain proxy if enabled in config.
	var obProxy *gateway.OpenBrainProxy
	if cfg.OpenBrain.Enabled {
		obProxy = gateway.NewOpenBrainProxy(cfg.OpenBrain)
	}

	gw := gateway.NewGateway(gateway.GatewayOpts{
		DB:        database,
		Spawner:   spawner,
		Mail:      mailStore,
		Queue:     mq,
		Version:   version,
		StartAt:   time.Now(),
		OpenBrain: obProxy,
	})

	// Agentic foundation engines.
	traceEngine := trace.NewTraceEngine(database, 100, 5*time.Second)
	blockEngine := block.NewBlockRuleEngine(database)
	blueprintEngine := blueprint.NewBlueprintEngine(database)
	holdoutEngine := holdout.NewHoldoutEngine(database, 0.7)
	manifestStore := isolation.NewManifestStore(database)

	return &App{
		Config:          cfg,
		DB:              database,
		Spawner:         spawner,
		MailStore:       mailStore,
		MergeQueue:      mq,
		MergeExecutor:   executor,
		WorktreeManager: wt,
		PaneManager:     panes,
		WindowManager:   windows,
		Watchdog:        wd,
		Gateway:         gw,
		Version:         version,
		TraceEngine:     traceEngine,
		BlockEngine:     blockEngine,
		BlueprintEngine: blueprintEngine,
		HoldoutEngine:   holdoutEngine,
		ManifestStore:   manifestStore,
	}, nil
}

// Close releases resources held by the App.
func (a *App) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}

// SessionStatePath returns the path to the session state file,
// resolved from the config directory.
func (a *App) SessionStatePath() string {
	if a.Config != nil && a.Config.System.DBPath != "" {
		// System-wide: use ~/.computecommander/
		dir := filepath.Dir(a.Config.System.DBPath)
		// Expand ~ if present.
		if len(dir) > 0 && dir[0] == '~' {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, dir[1:])
			}
		}
		return filepath.Join(dir, "session-state.json")
	}
	// Per-project fallback.
	return filepath.Join(".computecommander", "session-state.json")
}

// SaveSessionState persists the current session state to disk.
func (a *App) SaveSessionState() error {
	sm := GetSessionManager(a)
	return sm.SaveState(a.SessionStatePath())
}

// RestoreSessionState loads saved session state from disk and populates
// the session manager. If force is false and the state is older than 24h,
// an error is returned.
func (a *App) RestoreSessionState(force bool) error {
	path := a.SessionStatePath()

	state, err := tui.LoadState(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no saved session state found at %s", path)
		}
		return fmt.Errorf("load session state: %w", err)
	}

	// Staleness check.
	if !force && state.IsStale(24*time.Hour) {
		return fmt.Errorf("session state is older than 24 hours (saved at %s); use --restore-force to override",
			state.SavedAt.Format(time.RFC3339))
	}

	// Warn if the saving process is still alive.
	if state.IsPIDAlive() {
		fmt.Fprintf(os.Stderr, "Warning: process %d that saved this state may still be running\n", state.PID)
	}

	sm := GetSessionManager(a)
	if err := sm.RestoreState(state); err != nil {
		return fmt.Errorf("restore session state: %w", err)
	}

	// Remove the state file after successful restoration.
	_ = os.Remove(path)

	return nil
}

// buildColorResolver returns the agent color resolver for TUI components.
func (a *App) buildColorResolver() tui.AgentColorResolver {
	if a.Spawner == nil {
		return nil
	}
	return a.Spawner.BuildColorResolver(context.Background())
}

// NewDashboard creates a TUI Dashboard wired to the App's services.
func (a *App) NewDashboard() *tui.Dashboard {
	return tui.NewDashboard(tui.DashboardOpts{
		Lister:             a.Spawner,
		Mail:               a.MailStore,
		Queue:              a.MergeQueue,
		Config:             a.Config,
		DB:                 a.DB,
		AgentColorResolver: a.buildColorResolver(),
	})
}

// NewDashboardWithCmd creates a TUI Dashboard with a CLI-overridden agent command.
func (a *App) NewDashboardWithCmd(agentCmd string) *tui.Dashboard {
	return tui.NewDashboard(tui.DashboardOpts{
		Lister:             a.Spawner,
		Mail:               a.MailStore,
		Queue:              a.MergeQueue,
		Config:             a.Config,
		DB:                 a.DB,
		AgentCmd:           agentCmd,
		AgentColorResolver: a.buildColorResolver(),
	})
}

// RunDashboard launches the TUI dashboard and blocks until exit.
func (a *App) RunDashboard(ctx context.Context) error {
	dash := a.NewDashboard()
	return dash.Run(ctx)
}

// RunDashboardWithCmd launches the TUI dashboard with an optional agent command override.
func (a *App) RunDashboardWithCmd(ctx context.Context, agentCmd string) error {
	dash := a.NewDashboardWithCmd(agentCmd)
	return dash.Run(ctx)
}

// RunDashboardWithProject launches the TUI dashboard with project context.
func (a *App) RunDashboardWithProject(ctx context.Context, agentCmd, projectID string) error {
	// Resolve project name from DB if a project ID is provided.
	projectName := ""
	if projectID != "" && a.DB != nil {
		var name string
		row := a.DB.QueryRow(ctx, "SELECT name FROM projects WHERE id = ?", projectID)
		if err := row.Scan(&name); err == nil {
			projectName = name
		}
	}

	dash := tui.NewDashboard(tui.DashboardOpts{
		Lister:             a.Spawner,
		Mail:               a.MailStore,
		Queue:              a.MergeQueue,
		Config:             a.Config,
		DB:                 a.DB,
		AgentCmd:           agentCmd,
		ProjectName:        projectName,
		ProjectID:          projectID,
		AgentColorResolver: a.buildColorResolver(),
	})
	return dash.Run(ctx)
}

// RunWatchdog starts the watchdog daemon and blocks until the context is cancelled.
func (a *App) RunWatchdog(ctx context.Context) error {
	return a.Watchdog.Run(ctx)
}

// RunGateway starts the HTTP gateway on the given address.
func (a *App) RunGateway(ctx context.Context, addr string) error {
	return a.Gateway.Start(ctx, addr)
}
