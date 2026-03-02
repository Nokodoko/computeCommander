// Package commands provides CLI command handler implementations for ComputeCommander.
// It bridges the Cobra CLI definitions in cmd/cc/ with the internal service packages.
package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/noko/computecommander/internal/agents"
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

	wd := watchdog.NewWatchdog(watchdog.WatchdogOpts{
		DB:          database,
		MailStore:   mailStore,
		PaneManager: panes,
		WatchdogCfg: cfg.Watchdog,
		NudgeCfg:    cfg.Nudge,
	})

	gw := gateway.NewGateway(gateway.GatewayOpts{
		DB:      database,
		Spawner: spawner,
		Mail:    mailStore,
		Queue:   mq,
		Version: version,
		StartAt: time.Now(),
	})

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
	}, nil
}

// Close releases resources held by the App.
func (a *App) Close() error {
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}

// NewDashboard creates a TUI Dashboard wired to the App's services.
func (a *App) NewDashboard() *tui.Dashboard {
	return tui.NewDashboard(tui.DashboardOpts{
		Lister: a.Spawner,
		Mail:   a.MailStore,
		Queue:  a.MergeQueue,
		Config: a.Config,
	})
}

// NewDashboardWithCmd creates a TUI Dashboard with a CLI-overridden agent command.
func (a *App) NewDashboardWithCmd(agentCmd string) *tui.Dashboard {
	return tui.NewDashboard(tui.DashboardOpts{
		Lister:   a.Spawner,
		Mail:     a.MailStore,
		Queue:    a.MergeQueue,
		Config:   a.Config,
		AgentCmd: agentCmd,
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

// RunWatchdog starts the watchdog daemon and blocks until the context is cancelled.
func (a *App) RunWatchdog(ctx context.Context) error {
	return a.Watchdog.Run(ctx)
}

// RunGateway starts the HTTP gateway on the given address.
func (a *App) RunGateway(ctx context.Context, addr string) error {
	return a.Gateway.Start(ctx, addr)
}
