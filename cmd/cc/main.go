package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/noko/computecommander/internal/commands"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/keybinds"
	"github.com/noko/computecommander/internal/platform/db"
	zellijPkg "github.com/noko/computecommander/internal/zellij"
)

var (
	version = "0.2.0"
	commit  = "dev"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// sharedApp is allocated once and shared by all commands that need it.
// Its fields are zero-valued at build time and populated by appPreRun
// before any RunE executes.
var sharedApp = &commands.App{}

// appInitialised tracks whether we have already initialised the App.
var appInitialised bool

// appPreRun loads the system-wide config (with per-project overlay) and populates sharedApp's fields.
func appPreRun(cmd *cobra.Command, args []string) error {
	if appInitialised {
		return nil
	}

	// v2: Try system-wide config loading first, fall back to per-project.
	// Prefer CMDR_PROJECT env var, then check the binary's own directory
	// for .computecommander/ (hooks call the binary by absolute path from
	// arbitrary CWDs like ~/.claude), then git root, then $PWD.
	wd := os.Getenv("CMDR_PROJECT")
	if wd == "" {
		// Check the directory containing the cmdr binary itself.
		if exe, err := os.Executable(); err == nil {
			binDir := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(binDir, ".computecommander", "local.db")); err == nil {
				wd = binDir
			}
		}
	}
	if wd == "" {
		gitRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err == nil {
			wd = strings.TrimSpace(string(gitRoot))
		}
	}
	if wd == "" {
		wd, _ = os.Getwd()
	}
	real, err := commands.NewAppSystemWide(wd, version)
	if err != nil {
		// Fallback: try loading per-project config directly
		configPath := filepath.Join(".computecommander", "config.yaml")
		real, err = commands.NewApp(configPath, version)
		if err != nil {
			return err
		}
	}
	// Copy all fields from the real App into the shared pointer.
	*sharedApp = *real
	appInitialised = true
	return nil
}

func rootCmd() *cobra.Command {
	var tuiFlag bool
	var restoreFlag bool
	var restoreForceFlag bool

	root := &cobra.Command{
		Use:   "cmdr",
		Short: "ComputeCommander - Agentic IDE for AI coding agent swarms",
		Long: `ComputeCommander (cmdr) is an agentic IDE for AI coding agent swarms.

Running cmdr with no subcommand launches a zellij session with the
multi-pane dashboard layout (FP | Agent Session | Agents + bottom bar).
Each panel is a native zellij pane.

Use --tui to force the in-process Bubbletea TUI instead of zellij.
If zellij is not installed or you are already inside a zellij session,
the Bubbletea TUI is used automatically as a fallback.

Each agent works in an isolated git worktree, communicates through
a structured messaging system, and merges work back with intelligent
conflict resolution.`,
		Version:       fmt.Sprintf("%s (commit: %s)", version, commit),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Only initialize app for the root command's RunE (opening interface)
			// or for commands that explicitly set their own PersistentPreRunE via addAppCmd.
			// This allows init, config, and other non-app commands to run without a project.
			if cmd.Name() == "cmdr" && cmd.RunE != nil {
				return appPreRun(cmd, args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Restore sessions from saved state if requested.
			if restoreFlag {
				if err := sharedApp.RestoreSessionState(restoreForceFlag); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				} else {
					sm := commands.GetSessionManager(sharedApp)
					sessions := sm.ListSessions(false)
					fmt.Fprintf(os.Stderr, "Restored %d session(s) from saved state.\n", len(sessions))
				}
			}

			// Start autosave goroutine (saves every 30s while sessions exist).
			stopAutosave := commands.GetSessionManager(sharedApp).StartAutosave(
				sharedApp.SessionStatePath(), 30*time.Second,
			)
			defer stopAutosave()

			// Check for --tui flag or CC_DASHBOARD_TUI env var.
			if !tuiFlag {
				tuiFlag = os.Getenv("CC_DASHBOARD_TUI") == "1"
			}

			// When --tui is NOT set, try to launch a real zellij session.
			// Note: we do NOT check IsInsideZellij() here because the user's
			// terminal IS zellij. LaunchSession clears ZELLIJ env vars to
			// avoid nesting errors and creates a fresh session.
			if !tuiFlag && zellijPkg.ZellijAvailable() {
				layoutPath := sharedApp.Config.Zellij.DashboardLayout
				if layoutPath == "" {
					layoutPath = zellijPkg.DefaultLayoutPath()
				}

				// Always regenerate the layout file to pick up code changes.
				wd, _ := os.Getwd()
				cmdrBin, _ := os.Executable()
				if cmdrBin == "" {
					cmdrBin = "cmdr"
				}

				// Determine agent command for the center pane.
				agentCmd := sharedApp.Config.Defaults.AgentCommand
				if flagAgent, _ := cmd.Flags().GetString("agent"); flagAgent != "" {
					agentCmd = flagAgent
				}

				if writeErr := zellijPkg.WriteLayout(layoutPath, zellijPkg.LayoutOpts{
					CmdrBinary:    cmdrBin,
					SessionPrefix: sharedApp.Config.Zellij.SessionPrefix,
					ProjectDir:    wd,
					AgentCommand:  agentCmd,
					UseWrapper:    true,
					Version:       version,
				}); writeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not write layout file: %v\nFalling back to TUI.\n", writeErr)
					return sharedApp.RunDashboard(cmd.Context())
				}

				sessionName := sharedApp.Config.Zellij.SessionPrefix + "-dashboard"

				// Write the lock file with the current PID so that
				// dashboardStartTime() can use its mtime to filter out
				// completed agents from previous sessions.
				lockPath := filepath.Join(".computecommander", "cmdr.lock")
				if lockErr := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); lockErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not write lock file: %v\n", lockErr)
				}

				return zellijPkg.LaunchSession(zellijPkg.SessionOpts{
					SessionName: sessionName,
					LayoutPath:  layoutPath,
				})
			}

			// Fallback: in-process Bubbletea TUI.
			return sharedApp.RunDashboard(cmd.Context())
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if appInitialised {
				// Save session state before closing.
				if err := sharedApp.SaveSessionState(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not save session state: %v\n", err)
				}
				_ = sharedApp.Close()
			}
		},
	}

	root.Flags().BoolVar(&tuiFlag, "tui", false, "Force in-process Bubbletea TUI (skip zellij session)")
	root.Flags().BoolVar(&restoreFlag, "restore", false, "Restore sessions from last saved state")
	root.Flags().BoolVar(&restoreForceFlag, "restore-force", false, "Restore even if state file is >24h old")
	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
	root.PersistentFlags().Bool("json", false, "JSON output")
	root.PersistentFlags().Bool("timing", false, "Show execution timing")
	root.PersistentFlags().StringP("agent", "a", "", "Agent command for the main pane (overrides config)")
	root.PersistentFlags().String("sub-agent", "", "Nested agent focus (Claude Code limitation workaround)")

	root.AddGroup(
		&cobra.Group{ID: "LIFECYCLE", Title: "Lifecycle:"},
		&cobra.Group{ID: "CORE", Title: "Core Commands:"},
		&cobra.Group{ID: "INFO", Title: "Information:"},
		&cobra.Group{ID: "DATA", Title: "Data:"},
		&cobra.Group{ID: "COORDINATION", Title: "Coordination:"},
		&cobra.Group{ID: "MESSAGING", Title: "Messaging:"},
		&cobra.Group{ID: "MERGE", Title: "Merge:"},
		&cobra.Group{ID: "GROUPS", Title: "Groups:"},
		&cobra.Group{ID: "OBSERVABILITY", Title: "Observability:"},
		&cobra.Group{ID: "SETTINGS", Title: "Settings:"},
		&cobra.Group{ID: "INFRASTRUCTURE", Title: "Infrastructure:"},
	)

	// Commands that do NOT require an initialised project.
	root.AddCommand(initCmd())
	root.AddCommand(configCmd())

	// v2: Project management and migration commands.
	addAppCmd(root, commands.ProjectCmd(sharedApp))
	addAppCmd(root, commands.MigrateCmd(sharedApp))

	// Commands that require an initialised project.
	// Each is built with the shared App pointer whose fields are populated
	// by PersistentPreRunE before any RunE fires.

	// Lifecycle commands.
	addAppCmd(root, commands.ShutdownCmd(sharedApp))
	addAppCmd(root, commands.ResetCmd(sharedApp))
	addAppCmd(root, commands.RestartCmd(sharedApp))

	// Agent registration commands (multi-runtime).
	addAppCmd(root, commands.RegisterCmd(sharedApp))
	addAppCmd(root, commands.DeregisterCmd(sharedApp))
	addAppCmd(root, commands.HeartbeatCmd(sharedApp))

	// Core commands.
	addAppCmd(root, commands.SlingCmd(sharedApp))
	addAppCmd(root, commands.StopCmd(sharedApp))
	addAppCmd(root, commands.StatusCmd(sharedApp))
	addAppCmd(root, commands.GitStatusCmd(sharedApp))
	addAppCmd(root, commands.DashboardCmd(sharedApp))
	addAppCmd(root, commands.ShellCmd(sharedApp))
	addAppCmd(root, commands.FeedbackCmd(sharedApp))
	addAppCmd(root, commands.SupportCmd(sharedApp))

	// Information commands.
	addAppCmd(root, commands.HelpCmd(sharedApp))
	addAppCmd(root, commands.DocsCmd(sharedApp))
	addAppCmd(root, commands.VersionCmd(sharedApp))
	addAppCmd(root, commands.UpdateCmd(sharedApp))

	// Data commands.
	addAppCmd(root, commands.ExportCmd(sharedApp))
	addAppCmd(root, commands.BackupCmd(sharedApp))
	addAppCmd(root, commands.RestoreCmd(sharedApp))

	// Coordination.
	addAppCmd(root, commands.CoordinatorCmd(sharedApp))
	addAppCmd(root, commands.MonitorCmd(sharedApp))

	addAppCmd(root, commands.MailCmd(sharedApp))
	addAppCmd(root, commands.NudgeCmd(sharedApp))

	addAppCmd(root, commands.MergeCmd(sharedApp))

	addAppCmd(root, commands.GroupCmd(sharedApp))

	addAppCmd(root, commands.InspectCmd(sharedApp))
	addAppCmd(root, commands.TraceCmd(sharedApp))
	addAppCmd(root, commands.ErrorsCmd(sharedApp))
	addAppCmd(root, commands.ReplayCmd(sharedApp))
	addAppCmd(root, commands.FeedCmd(sharedApp))
	addAppCmd(root, commands.LogsCmd(sharedApp))
	addAppCmd(root, commands.CostsCmd(sharedApp))
	addAppCmd(root, commands.MetricsCmd(sharedApp))
	addAppCmd(root, commands.RunCmd(sharedApp))

	// Observability: evals.
	addAppCmd(root, commands.EvalsCmd(sharedApp))

	// Observability: clear logs (distinct from clean).
	addAppCmd(root, commands.ClearCmd(sharedApp))

	// Settings commands.
	addAppCmd(root, commands.ThemeCmd(sharedApp))
	addAppCmd(root, commands.NotificationsCmd(sharedApp))
	addAppCmd(root, commands.AnalyticsCmd(sharedApp))
	addAppCmd(root, commands.IntegrationsCmd(sharedApp))
	addAppCmd(root, commands.AutomationCmd(sharedApp))

	// Navigation commands.
	addAppCmd(root, commands.FpCmd(sharedApp))
	addAppCmd(root, commands.SessionCmd(sharedApp))
	addAppCmd(root, commands.SessionsCmd(sharedApp))

	// Dashboard pane commands.
	addAppCmd(root, commands.JiraCmd(sharedApp))
	addAppCmd(root, commands.JiraBoardCmd(sharedApp))
	addAppCmd(root, commands.OpenBrainCmd(sharedApp))
	addAppCmd(root, commands.PromptLineCmd(sharedApp))

	// Infrastructure commands.
	addAppCmd(root, commands.WorktreeCmd(sharedApp))
	addAppCmd(root, commands.WatchCmd(sharedApp))
	addAppCmd(root, commands.DoctorCmd(sharedApp))
	addAppCmd(root, commands.CleanCmd(sharedApp))
	addAppCmd(root, commands.SweepCmd(sharedApp))
	addAppCmd(root, commands.FeatureCmd(sharedApp))

	// Agentic foundation commands (block, blueprint, gate, holdout, isolation).
	for _, cmd := range commands.AgenticCmd(sharedApp) {
		addAppCmd(root, cmd)
	}

	root.SetUsageTemplate(usageTemplate)

	return root
}

// addAppCmd attaches appPreRun and adds the command to root.
func addAppCmd(root *cobra.Command, cmd *cobra.Command) {
	cmd.PersistentPreRunE = appPreRun
	root.AddCommand(cmd)
}

// --- init command (no App required) ---

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize ComputeCommander in a project or system-wide",
		GroupID: "CORE",
		RunE:    runInit,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip interactive prompts")
	cmd.Flags().String("name", "", "Project name (default: directory name)")
	cmd.Flags().String("db", "", "Database backend: postgres|sqlite (default: auto-detect)")
	cmd.Flags().Bool("system", false, "Initialize system-wide ~/.computecommander/")
	cmd.Flags().Bool("force", false, "Overwrite existing system config")
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	systemFlag, _ := cmd.Flags().GetBool("system")
	forceFlag, _ := cmd.Flags().GetBool("force")

	if systemFlag {
		return runInitSystem(cmd, forceFlag)
	}

	dir := ".computecommander"

	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists; project already initialized", dir)
	}

	dirs := []string{
		dir,
		filepath.Join(dir, "agents"),
		filepath.Join(dir, "hooks"),
		filepath.Join(dir, "specs"),
		filepath.Join(dir, "worktrees"),
		filepath.Join(dir, "logs"),
		filepath.Join(dir, "layouts"),
		filepath.Join(dir, "themes"),
		filepath.Join(dir, "backups"),
		filepath.Join(dir, "plugins"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	projectName, _ := cmd.Flags().GetString("name")
	if projectName == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		projectName = filepath.Base(wd)
	}

	dbDriver, _ := cmd.Flags().GetString("db")
	if dbDriver == "" {
		dbDriver = "sqlite"
	}

	cfg := config.DefaultConfig()
	cfg.Project.Name = projectName
	cfg.Database.Driver = dbDriver

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	rulesContent := `version: 1

global:
  blocked_commands:
    - git push --force
    - git reset --hard
    - rm -rf /
    - rm -rf ~
    - sudo rm
  blocked_paths:
    - .git/
    - .computecommander/
    - /etc/
    - /usr/
`
	rulesPath := filepath.Join(dir, "hooks", "rules.yaml")
	if err := os.WriteFile(rulesPath, []byte(rulesContent), 0o644); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}

	// Generate keybinds.yaml with default leader key mappings.
	keybindsPath := filepath.Join(dir, "keybinds.yaml")
	if err := keybinds.WriteDefault(keybindsPath); err != nil {
		return fmt.Errorf("write keybinds: %w", err)
	}

	// Generate default theme file.
	defaultThemeContent := `name: default
colors:
  primary: "#7C3AED"
  secondary: "#10B981"
  error: "#EF4444"
  warning: "#F59E0B"
  info: "#3B82F6"
  muted: "#6B7280"
  background: "#1F2937"
  foreground: "#F9FAFB"
font:
  size: 14
  family: "monospace"
contrast: "normal"
`
	defaultThemePath := filepath.Join(dir, "themes", "default.yaml")
	if err := os.WriteFile(defaultThemePath, []byte(defaultThemeContent), 0o644); err != nil {
		return fmt.Errorf("write default theme: %w", err)
	}

	// Generate KDL dashboard layout.
	wd, _ := os.Getwd()
	layoutPath := filepath.Join(dir, "layouts", "cmdr-dashboard.kdl")
	if err := zellijPkg.WriteLayout(layoutPath, zellijPkg.LayoutOpts{
		CmdrBinary: "cmdr",
		ProjectDir: wd,
	}); err != nil {
		return fmt.Errorf("write layout: %w", err)
	}

	// Create and migrate the local database.
	var dbPath string
	if dbDriver == "sqlite" {
		dbPath = cfg.Database.SQLite.Path
		if dbPath == "" {
			dbPath = filepath.Join(dir, "local.db")
		}
		database, err := db.NewSQLite(dbPath)
		if err != nil {
			return fmt.Errorf("create database: %w", err)
		}
		if err := db.Migrate(database, "sqlite"); err != nil {
			database.Close()
			return fmt.Errorf("apply database migrations: %w", err)
		}
		database.Close()
	}

	quiet, _ := cmd.Flags().GetBool("quiet")
	if !quiet {
		fmt.Printf("Initialized ComputeCommander in %s/\n", dir)
		fmt.Printf("  Project: %s\n", projectName)
		fmt.Printf("  Database: %s\n", dbDriver)
		fmt.Println("\nCreated:")
		for _, d := range dirs {
			fmt.Printf("  %s/\n", d)
		}
		fmt.Printf("  %s\n", configPath)
		fmt.Printf("  %s\n", rulesPath)
		fmt.Printf("  %s\n", keybindsPath)
		fmt.Printf("  %s\n", defaultThemePath)
		fmt.Printf("  %s\n", layoutPath)
		if dbPath != "" {
			fmt.Printf("  %s (schema applied)\n", dbPath)
		}
	}

	// Launch the dashboard in-process so the user lands directly in the UI.
	// We exec into ourselves with "dashboard" so the newly written config
	// is picked up fresh by appPreRun.
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	return syscall.Exec(self, []string{self, "dashboard"}, os.Environ())
}

// runInitSystem initializes the system-wide ~/.computecommander/ directory.
func runInitSystem(cmd *cobra.Command, force bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dir := filepath.Join(home, ".computecommander")

	if _, err := os.Stat(dir); err == nil && !force {
		return fmt.Errorf("%s already exists; use --force to overwrite", dir)
	}

	dirs := []string{
		dir,
		filepath.Join(dir, "layouts"),
		filepath.Join(dir, "scripts"),
		filepath.Join(dir, "backups"),
		filepath.Join(dir, "themes"),
		filepath.Join(dir, "migrations", "completed"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	cfg := config.DefaultConfig()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Create the system-wide database.
	dbPath := filepath.Join(dir, "cc.db")
	database, err := db.NewSQLite(dbPath)
	if err != nil {
		return fmt.Errorf("create system database: %w", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		database.Close()
		return fmt.Errorf("apply database migrations: %w", err)
	}
	database.Close()

	// Generate default KDL layout.
	wd, _ := os.Getwd()
	layoutPath := filepath.Join(dir, "layouts", "cmdr-dashboard.kdl")
	if err := zellijPkg.WriteLayout(layoutPath, zellijPkg.LayoutOpts{
		CmdrBinary: "cmdr",
		ProjectDir: wd,
	}); err != nil {
		return fmt.Errorf("write layout: %w", err)
	}

	quiet, _ := cmd.Flags().GetBool("quiet")
	if !quiet {
		fmt.Printf("Initialized system-wide ComputeCommander in %s/\n", dir)
		fmt.Printf("  Database: %s\n", dbPath)
		fmt.Printf("  Config: %s\n", configPath)
		fmt.Printf("  Layout: %s\n", layoutPath)
		fmt.Println("\nCreated:")
		for _, d := range dirs {
			fmt.Printf("  %s/\n", d)
		}
	}

	return nil
}

// --- config command (no App required) ---

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Configuration management",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(configShowCmd())
	cmd.AddCommand(configValidateCmd())
	cmd.AddCommand(configGetCmd())
	cmd.AddCommand(configSetCmd())
	cmd.AddCommand(configEditCmd())

	return cmd
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadProjectConfig()
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			fmt.Print(string(data))
			return nil
		},
	}
}

func configValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadProjectConfig()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			fmt.Println("Configuration is valid.")
			return nil
		},
	}
}

func configGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get specific configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadProjectConfig()
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("unmarshal config: %w", err)
			}
			val := lookupKey(m, args[0])
			if val == nil {
				return fmt.Errorf("key %q not found", args[0])
			}
			out, err := yaml.Marshal(val)
			if err != nil {
				fmt.Println(val)
			} else {
				fmt.Print(string(out))
			}
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := filepath.Join(".computecommander", "config.yaml")
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}
			setKey(m, args[0], args[1])
			out, err := yaml.Marshal(m)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			if err := os.WriteFile(configPath, out, 0o644); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
			fmt.Printf("Set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func configEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			configPath := filepath.Join(".computecommander", "config.yaml")
			c := exec.Command(editor, configPath)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

func loadProjectConfig() (*config.Config, error) {
	// v2: Try system-wide config first, fall back to per-project
	wd, _ := os.Getwd()
	cfg, err := config.LoadSystemConfig(wd)
	if err != nil {
		// Fallback to per-project config
		path := filepath.Join(".computecommander", "config.yaml")
		cfg, err = config.LoadConfig(path)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}
	return cfg, nil
}

func lookupKey(m map[string]any, key string) any {
	parts := strings.Split(key, ".")
	var current any = m
	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = cm[part]
		if !ok {
			return nil
		}
	}
	return current
}

func setKey(m map[string]any, key string, value string) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

var usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} <command> [options]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
