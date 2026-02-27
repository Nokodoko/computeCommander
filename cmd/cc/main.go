package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/noko/computecommander/internal/commands"
	"github.com/noko/computecommander/internal/config"
)

var (
	version = "0.1.0"
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

// appPreRun loads the project config and populates sharedApp's fields.
func appPreRun(cmd *cobra.Command, args []string) error {
	if appInitialised {
		return nil
	}
	configPath := filepath.Join(".computecommander", "config.yaml")
	real, err := commands.NewApp(configPath, version)
	if err != nil {
		return err
	}
	// Copy all fields from the real App into the shared pointer.
	*sharedApp = *real
	appInitialised = true
	return nil
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cc",
		Short: "ComputeCommander - Multi-agent orchestration for AI coding agents",
		Long: `ComputeCommander (cc) orchestrates AI coding agent swarms.

Each agent works in an isolated git worktree, communicates through
a structured messaging system, and merges work back with intelligent
conflict resolution.`,
		Version:       fmt.Sprintf("%s (commit: %s)", version, commit),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if appInitialised {
				_ = sharedApp.Close()
			}
		},
	}

	root.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")
	root.PersistentFlags().Bool("json", false, "JSON output")
	root.PersistentFlags().Bool("timing", false, "Show execution timing")

	root.AddGroup(
		&cobra.Group{ID: "CORE", Title: "Core Commands:"},
		&cobra.Group{ID: "COORDINATION", Title: "Coordination:"},
		&cobra.Group{ID: "MESSAGING", Title: "Messaging:"},
		&cobra.Group{ID: "MERGE", Title: "Merge:"},
		&cobra.Group{ID: "GROUPS", Title: "Groups:"},
		&cobra.Group{ID: "OBSERVABILITY", Title: "Observability:"},
		&cobra.Group{ID: "INFRASTRUCTURE", Title: "Infrastructure:"},
	)

	// Commands that do NOT require an initialised project.
	root.AddCommand(initCmd())
	root.AddCommand(configCmd())

	// Commands that require an initialised project.
	// Each is built with the shared App pointer whose fields are populated
	// by PersistentPreRunE before any RunE fires.
	addAppCmd(root, commands.SlingCmd(sharedApp))
	addAppCmd(root, commands.StopCmd(sharedApp))
	addAppCmd(root, commands.StatusCmd(sharedApp))
	addAppCmd(root, commands.DashboardCmd(sharedApp))

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

	addAppCmd(root, commands.WorktreeCmd(sharedApp))
	addAppCmd(root, commands.WatchCmd(sharedApp))
	addAppCmd(root, commands.DoctorCmd(sharedApp))
	addAppCmd(root, commands.CleanCmd(sharedApp))
	addAppCmd(root, commands.FeatureCmd(sharedApp))

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
		Short:   "Initialize ComputeCommander in a project",
		GroupID: "CORE",
		RunE:    runInit,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip interactive prompts")
	cmd.Flags().String("name", "", "Project name (default: directory name)")
	cmd.Flags().String("db", "", "Database backend: postgres|sqlite (default: auto-detect)")
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
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
	path := filepath.Join(".computecommander", "config.yaml")
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
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
