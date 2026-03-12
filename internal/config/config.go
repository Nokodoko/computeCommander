package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level ComputeCommander configuration matching spec section 6.1.
type Config struct {
	Version   int              `yaml:"version"`
	System    SystemConfig     `yaml:"system"`
	Project   ProjectConfig    `yaml:"project"`
	Projects  []ProjectEntry   `yaml:"projects"`
	Database  DatabaseConfig   `yaml:"database"`
	Zellij    ZellijConfig     `yaml:"zellij"`
	Agents    AgentsConfig     `yaml:"agents"`
	Worktrees WorktreesConfig  `yaml:"worktrees"`
	Defaults  DefaultsConfig   `yaml:"defaults"`
	Nudge     NudgeConfig      `yaml:"nudge"`
	Watchdog  WatchdogConfig   `yaml:"watchdog"`
	Merge     MergeConfig      `yaml:"merge"`
	Features  FeaturesConfig   `yaml:"features"`
	Logging   LoggingConfig    `yaml:"logging"`
	Runtimes  RuntimesConfig   `yaml:"runtimes"`
	Agentic   AgenticConfig    `yaml:"agentic"`
	Jira      JiraConfig       `yaml:"jira"`
}

// JiraConfig holds multi-instance Jira integration configuration.
type JiraConfig struct {
	Instances      []JiraInstance    `yaml:"instances"`
	RateLimit      JiraRateLimitCfg  `yaml:"rate_limit"`
	PromptTemplate string            `yaml:"prompt_template"`
	DarkFactory    DarkFactoryConfig `yaml:"dark_factory"`
}

// JiraInstance represents a single Jira server connection.
type JiraInstance struct {
	Name           string   `yaml:"name"`
	BaseURL        string   `yaml:"base_url"`
	Auth           JiraAuth `yaml:"auth"`
	DefaultProject string   `yaml:"default_project"`
	SyncInterval   string   `yaml:"sync_interval"`
}

// JiraAuth configures authentication for a Jira instance.
type JiraAuth struct {
	Type     string `yaml:"type"`     // "pat", "oauth2", "basic"
	Token    string `yaml:"token"`    // PAT or OAuth token (supports ${ENV_VAR})
	Username string `yaml:"username"` // For basic auth
	Password string `yaml:"password"` // For basic auth (supports ${ENV_VAR})
}

// JiraRateLimitCfg controls API request throttling.
type JiraRateLimitCfg struct {
	RequestsPerSecond int `yaml:"requests_per_second"`
	Burst             int `yaml:"burst"`
}

// DarkFactoryConfig controls autonomous execution.
type DarkFactoryConfig struct {
	Enabled            bool   `yaml:"enabled"`
	ExecutionMode      string `yaml:"execution_mode"` // "full_auto", "stepped", "scoped"
	UATTimeout         string `yaml:"uat_timeout"`
	MaxConcurrentTasks int    `yaml:"max_concurrent_tasks"`
}

// SystemConfig holds system-wide configuration for ccv2.
type SystemConfig struct {
	Home            string `yaml:"home"`
	DBPath          string `yaml:"db_path"`
	DashboardLayout string `yaml:"dashboard_layout"`
}

// ProjectEntry represents a registered project in the system-wide config.
type ProjectEntry struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
}

// AgenticConfig holds agentic foundation configuration.
type AgenticConfig struct {
	Trace     TraceConfig     `yaml:"trace"`
	Blocks    BlocksConfig    `yaml:"blocks"`
	Isolation IsolationConfig `yaml:"isolation"`
	Gates     GatesConfig     `yaml:"gates"`
	Holdout   HoldoutConfig   `yaml:"holdout"`
	Blueprint BlueprintConfig `yaml:"blueprint"`
}

// TraceConfig configures the traceability engine.
type TraceConfig struct {
	Enabled       bool   `yaml:"enabled"`
	BatchSize     int    `yaml:"batch_size"`      // Max events before flush (default: 100)
	FlushInterval string `yaml:"flush_interval"`  // Max time before flush (default: "5s")
	RetentionDays int    `yaml:"retention_days"`   // Auto-prune after N days (default: 7)
}

// BlocksConfig configures the block rule engine.
type BlocksConfig struct {
	Enabled      bool     `yaml:"enabled"`
	RulesDir     string   `yaml:"rules_dir"`       // Directory containing YAML rule files
	DefaultRules string   `yaml:"default_rules"`    // Path to default.yaml
	CustomRules  string   `yaml:"custom_rules"`     // Path to custom.yaml (optional)
	FailClosed   bool     `yaml:"fail_closed"`      // If true, block on rule engine failure
}

// IsolationConfig configures the isolation engine.
type IsolationConfig struct {
	Enabled         bool              `yaml:"enabled"`
	UseCgroups      bool              `yaml:"use_cgroups"`       // Enable cgroup v2 isolation
	UseNamespaces   bool              `yaml:"use_namespaces"`    // Enable mount namespace isolation
	DefaultResources ResourceDefaults `yaml:"default_resources"` // Default resource limits
}

// ResourceDefaults holds default resource limits for isolation.
type ResourceDefaults struct {
	CPUShares    int `yaml:"cpu_shares"`    // cgroup cpu.shares (default: 512)
	MemoryMB     int `yaml:"memory_mb"`     // cgroup memory.max in MB (default: 2048)
	DiskMB       int `yaml:"disk_mb"`       // Disk quota in MB (default: 1024)
	MaxProcesses int `yaml:"max_processes"` // pids.max (default: 50)
}

// GatesConfig configures the quality gate pipeline.
type GatesConfig struct {
	Enabled    bool       `yaml:"enabled"`
	Timeout    string     `yaml:"timeout"`     // Per-gate timeout (default: "5m")
	RetryLimit int        `yaml:"retry_limit"` // Max gate retries (default: 3)
	Pipeline   []GateDef  `yaml:"pipeline"`    // Ordered list of gates
}

// GateDef defines a single quality gate in the pipeline.
type GateDef struct {
	Name    string `yaml:"name"`    // lint|typecheck|test|security|format
	Command string `yaml:"command"` // Shell command to run
	Enabled bool   `yaml:"enabled"`
}

// HoldoutConfig configures the anti-gaming holdout system.
type HoldoutConfig struct {
	Enabled        bool    `yaml:"enabled"`
	KeyPath        string  `yaml:"key_path"`        // Path to age recipient file
	DriftThreshold float64 `yaml:"drift_threshold"` // Score below which drift alert fires (default: 0.7)
}

// BlueprintConfig configures the blueprint engine.
type BlueprintConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BlueprintDir string `yaml:"blueprint_dir"` // Directory for blueprint YAML files
	DefaultTimeout string `yaml:"default_timeout"` // Default timeout for blueprints (default: "30m")
}

type ProjectConfig struct {
	Name            string         `yaml:"name"`
	Root            string         `yaml:"root"`
	CanonicalBranch string         `yaml:"canonical_branch"`
	QualityGates    []QualityGate  `yaml:"quality_gates"`
}

type QualityGate struct {
	Name        string `yaml:"name"`
	Command     string `yaml:"command"`
	Description string `yaml:"description"`
}

type DatabaseConfig struct {
	Driver   string         `yaml:"driver"`
	Postgres PostgresConfig `yaml:"postgres"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
	PoolSize int    `yaml:"pool_size"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type ZellijConfig struct {
	DashboardLayout string `yaml:"dashboard_layout"`
	Layout        string `yaml:"layout"`
	Terminal      string `yaml:"terminal"`
	SessionPrefix string `yaml:"session_prefix"`
}

type AgentsConfig struct {
	MaxConcurrent     int    `yaml:"max_concurrent"`
	StaggerDelayMs    int    `yaml:"stagger_delay_ms"`
	MaxDepth          int    `yaml:"max_depth"`
	MaxSessionsPerRun int    `yaml:"max_sessions_per_run"`
	MaxAgentsPerLead  int    `yaml:"max_agents_per_lead"`
	BaseDir           string `yaml:"base_dir"`
	DefaultCommand    string `yaml:"default_command"`
}

type WorktreesConfig struct {
	BaseDir string `yaml:"base_dir"`
}

type DefaultsConfig struct {
	Runtime       string            `yaml:"runtime"`
	AgentCommand  string            `yaml:"agent_command"`
	ModelMappings map[string]string `yaml:"model_mappings"`
}

type NudgeConfig struct {
	SoftTimeout       string            `yaml:"soft_timeout"`
	HardTimeout       string            `yaml:"hard_timeout"`
	EscalationEnabled bool              `yaml:"escalation_enabled"`
	ContextWindow     int               `yaml:"context_window"`
	LoopDetection     LoopDetection     `yaml:"loop_detection"`
}

type LoopDetection struct {
	Enabled   bool   `yaml:"enabled"`
	Window    string `yaml:"window"`
	Threshold int    `yaml:"threshold"`
}

type WatchdogConfig struct {
	Tier0Enabled      bool             `yaml:"tier0_enabled"`
	Tier0IntervalMs   int              `yaml:"tier0_interval_ms"`
	Tier1Enabled      bool             `yaml:"tier1_enabled"`
	Tier2Enabled      bool             `yaml:"tier2_enabled"`
	StaleThresholdMs  int              `yaml:"stale_threshold_ms"`
	ZombieThresholdMs int              `yaml:"zombie_threshold_ms"`
	NudgeIntervalMs   int              `yaml:"nudge_interval_ms"`
	PaneHealer        PaneHealerConfig `yaml:"pane_healer"`
}

// PaneHealerConfig controls self-healing for frozen/stale dashboard panes.
type PaneHealerConfig struct {
	Enabled           bool `yaml:"enabled"`
	CheckIntervalMs   int  `yaml:"check_interval_ms"`
	FrozenThresholdMs int  `yaml:"frozen_threshold_ms"`
	MaxRestarts       int  `yaml:"max_restarts"`
}

type MergeConfig struct {
	AIResolveEnabled bool `yaml:"ai_resolve_enabled"`
	ReimagineEnabled bool `yaml:"reimagine_enabled"`
	AutoMerge        bool `yaml:"auto_merge"`
}

type FeaturesConfig struct {
	Distributed  bool `yaml:"distributed"`
	RemoteAgents bool `yaml:"remote_agents"`
}

type LoggingConfig struct {
	Verbose       bool   `yaml:"verbose"`
	RedactSecrets bool   `yaml:"redact_secrets"`
	Format        string `yaml:"format"`
	Level         string `yaml:"level"`
}

type RuntimesConfig struct {
	Claude RuntimeConfig `yaml:"claude"`
	Gemini RuntimeConfig `yaml:"gemini"`
	Codex  RuntimeConfig `yaml:"codex"`
	Pi     PiConfig      `yaml:"pi"`
	Goose  RuntimeConfig `yaml:"goose"`
}

type RuntimeConfig struct {
	DefaultModel string            `yaml:"default_model"`
	Models       map[string]string `yaml:"models"`
}

type PiConfig struct {
	Provider string            `yaml:"provider"`
	ModelMap map[string]string `yaml:"model_map"`
}

// DefaultConfig returns a Config with sensible defaults matching spec section 6.1.
func DefaultConfig() *Config {
	return &Config{
		Version: 2,
		System: SystemConfig{
			Home:            "~",
			DBPath:          "~/.computecommander/cc.db",
			DashboardLayout: "~/.computecommander/layouts/cmdr-dashboard.kdl",
		},
		Project: ProjectConfig{
			CanonicalBranch: "main",
		},
		Projects: []ProjectEntry{},
		Database: DatabaseConfig{
			Driver: "sqlite",
			Postgres: PostgresConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "computecommander",
				User:     "cc",
				SSLMode:  "disable",
				PoolSize: 10,
			},
			SQLite: SQLiteConfig{
				Path: "~/.computecommander/cc.db",
			},
		},
		Zellij: ZellijConfig{
			Layout:          "default",
			DashboardLayout: "~/.computecommander/layouts/cmdr-dashboard.kdl",
			Terminal:        "wezterm",
			SessionPrefix:   "cc",
		},
		Agents: AgentsConfig{
			MaxConcurrent:     10,
			StaggerDelayMs:    2000,
			MaxDepth:          2,
			MaxSessionsPerRun: 50,
			MaxAgentsPerLead:  5,
			BaseDir:           "agents",
			DefaultCommand:    "claude --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit",
		},
		Worktrees: WorktreesConfig{
			BaseDir: ".computecommander/worktrees",
		},
		Defaults: DefaultsConfig{
			Runtime:      "claude",
			AgentCommand: "claude --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit",
			ModelMappings: map[string]string{
				"scout":    "gemini",
				"builder":  "claude",
				"reviewer": "claude",
				"lead":     "claude",
				"merger":   "claude",
				"monitor":  "claude",
			},
		},
		Nudge: NudgeConfig{
			SoftTimeout:       "10m",
			HardTimeout:       "30m",
			EscalationEnabled: true,
			ContextWindow:     50,
			LoopDetection: LoopDetection{
				Enabled:   true,
				Window:    "5m",
				Threshold: 3,
			},
		},
		Watchdog: WatchdogConfig{
			Tier0Enabled:      true,
			Tier0IntervalMs:   30000,
			Tier1Enabled:      true,
			Tier2Enabled:      false,
			StaleThresholdMs:  300000,
			ZombieThresholdMs: 1800000,
			NudgeIntervalMs:   60000,
			PaneHealer: PaneHealerConfig{
				Enabled:           true,
				CheckIntervalMs:   10000,
				FrozenThresholdMs: 30000,
				MaxRestarts:       5,
			},
		},
		Merge: MergeConfig{
			AIResolveEnabled: true,
			ReimagineEnabled: false,
			AutoMerge:        true,
		},
		Features: FeaturesConfig{
			Distributed:  false,
			RemoteAgents: false,
		},
		Logging: LoggingConfig{
			Verbose:       false,
			RedactSecrets: true,
			Format:        "human",
			Level:         "info",
		},
		Agentic: AgenticConfig{
			Trace: TraceConfig{
				Enabled:       true,
				BatchSize:     100,
				FlushInterval: "5s",
				RetentionDays: 7,
			},
			Blocks: BlocksConfig{
				Enabled:      true,
				RulesDir:     ".computecommander/blocks",
				DefaultRules: ".computecommander/blocks/default.yaml",
				CustomRules:  ".computecommander/blocks/custom.yaml",
				FailClosed:   true,
			},
			Isolation: IsolationConfig{
				Enabled:       false,
				UseCgroups:    false,
				UseNamespaces: false,
				DefaultResources: ResourceDefaults{
					CPUShares:    512,
					MemoryMB:     2048,
					DiskMB:       1024,
					MaxProcesses: 50,
				},
			},
			Gates: GatesConfig{
				Enabled:    true,
				Timeout:    "5m",
				RetryLimit: 3,
				Pipeline: []GateDef{
					{Name: "format", Command: "gofmt -l .", Enabled: true},
					{Name: "lint", Command: "golangci-lint run", Enabled: true},
					{Name: "typecheck", Command: "go vet ./...", Enabled: true},
					{Name: "test", Command: "go test ./...", Enabled: true},
					{Name: "security", Command: "gosec ./...", Enabled: false},
				},
			},
			Holdout: HoldoutConfig{
				Enabled:        false,
				KeyPath:        ".computecommander/holdouts/holdout-key.pub",
				DriftThreshold: 0.7,
			},
			Blueprint: BlueprintConfig{
				Enabled:        true,
				BlueprintDir:   ".computecommander/blueprints",
				DefaultTimeout: "30m",
			},
		},
		Jira: JiraConfig{
			Instances: []JiraInstance{},
			RateLimit: JiraRateLimitCfg{
				RequestsPerSecond: 10,
				Burst:             20,
			},
			PromptTemplate: ".computecommander/templates/jira-prompt.tmpl",
			DarkFactory: DarkFactoryConfig{
				Enabled:            false,
				ExecutionMode:      "stepped",
				UATTimeout:         "15m",
				MaxConcurrentTasks: 3,
			},
		},
		Runtimes: RuntimesConfig{
			Claude: RuntimeConfig{
				DefaultModel: "claude-sonnet-4",
				Models: map[string]string{
					"fast":     "claude-haiku-3",
					"default":  "claude-sonnet-4",
					"powerful": "claude-opus-4",
				},
			},
			Gemini: RuntimeConfig{
				DefaultModel: "gemini-2.5-pro",
				Models: map[string]string{
					"fast":    "gemini-2.0-flash",
					"default": "gemini-2.5-pro",
				},
			},
			Codex: RuntimeConfig{
				DefaultModel: "o3",
			},
			Pi: PiConfig{
				Provider: "anthropic",
				ModelMap: map[string]string{
					"opus":   "anthropic/claude-opus-4",
					"sonnet": "anthropic/claude-sonnet-4",
				},
			},
			Goose: RuntimeConfig{
				DefaultModel: "claude-sonnet-4",
			},
		},
	}
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// expandEnvVars replaces ${VAR} patterns in a YAML byte slice with environment variable values.
func expandEnvVars(data []byte) []byte {
	return envVarRe.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := string(match[2 : len(match)-1])
		if val, ok := os.LookupEnv(varName); ok {
			return []byte(val)
		}
		return match
	})
}

// expandTilde replaces a leading "~/" in a path with the user's home directory.
// Go's os/exec and most libraries do not expand tilde, so paths like
// "~/.computecommander/layouts/foo.kdl" must be resolved before use.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// LoadConfig loads a YAML config from path, overlays config.local.yaml if present,
// and expands environment variables using ${VAR} syntax.
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

	// Auto-upgrade v1 configs to v2
	if cfg.Version < 2 {
		cfg.UpgradeV1ToV2()
	}

	// Expand tilde in path-valued fields. Go does not expand "~/" automatically,
	// so paths like "~/.computecommander/layouts/foo.kdl" would be passed as
	// literal strings to exec.Command, causing file-not-found failures.
	cfg.Zellij.DashboardLayout = expandTilde(cfg.Zellij.DashboardLayout)
	cfg.Database.SQLite.Path = expandTilde(cfg.Database.SQLite.Path)
	cfg.Worktrees.BaseDir = expandTilde(cfg.Worktrees.BaseDir)
	cfg.System.DBPath = expandTilde(cfg.System.DBPath)
	cfg.System.DashboardLayout = expandTilde(cfg.System.DashboardLayout)
	cfg.System.Home = expandTilde(cfg.System.Home)

	return cfg, nil
}

// LoadSystemConfig loads the system-wide config from ~/.computecommander/config.yaml,
// then overlays the per-project config if projectPath is non-empty.
func LoadSystemConfig(projectPath string) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	systemConfigPath := filepath.Join(home, ".computecommander", "config.yaml")

	// Start with defaults
	cfg := DefaultConfig()

	// Load system-wide config if it exists
	if data, err := os.ReadFile(systemConfigPath); err == nil {
		data = expandEnvVars(data)
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse system config %s: %w", systemConfigPath, err)
		}

		// Load system-wide local overlay
		dir := filepath.Dir(systemConfigPath)
		localPath := filepath.Join(dir, "config.local.yaml")
		if localData, err := os.ReadFile(localPath); err == nil {
			localData = expandEnvVars(localData)
			if err := yaml.Unmarshal(localData, cfg); err != nil {
				return nil, fmt.Errorf("parse system local config %s: %w", localPath, err)
			}
		}
	}

	// Overlay per-project config if provided
	if projectPath != "" {
		projectConfigPath := filepath.Join(projectPath, ".computecommander", "config.yaml")
		if data, err := os.ReadFile(projectConfigPath); err == nil {
			data = expandEnvVars(data)
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse project config %s: %w", projectConfigPath, err)
			}

			// Project-local overlay
			localPath := filepath.Join(projectPath, ".computecommander", "config.local.yaml")
			if localData, err := os.ReadFile(localPath); err == nil {
				localData = expandEnvVars(localData)
				if err := yaml.Unmarshal(localData, cfg); err != nil {
					return nil, fmt.Errorf("parse project local config %s: %w", localPath, err)
				}
			}
		}
	}

	// Auto-upgrade v1 configs to v2
	if cfg.Version < 2 {
		cfg.UpgradeV1ToV2()
	}

	// Expand tilde in all path-valued fields
	cfg.Zellij.DashboardLayout = expandTilde(cfg.Zellij.DashboardLayout)
	cfg.Database.SQLite.Path = expandTilde(cfg.Database.SQLite.Path)
	cfg.Worktrees.BaseDir = expandTilde(cfg.Worktrees.BaseDir)
	cfg.System.DBPath = expandTilde(cfg.System.DBPath)
	cfg.System.DashboardLayout = expandTilde(cfg.System.DashboardLayout)
	cfg.System.Home = expandTilde(cfg.System.Home)

	// Resolve relative DB path against projectPath so that commands invoked
	// from arbitrary CWDs (e.g. hooks running from ~/.claude) still find
	// the project's database rather than a stray one under CWD.
	if projectPath != "" && cfg.Database.SQLite.Path != "" &&
		!filepath.IsAbs(cfg.Database.SQLite.Path) {
		cfg.Database.SQLite.Path = filepath.Join(projectPath, cfg.Database.SQLite.Path)
	}

	return cfg, nil
}

// IsSystemWide returns true if this is a v2 system-wide config.
func (c *Config) IsSystemWide() bool {
	return c.Version >= 2 && c.System.DBPath != ""
}

// SystemDBPath returns the expanded path to the system-wide database.
func (c *Config) SystemDBPath() string {
	return expandTilde(c.System.DBPath)
}

// SystemHome returns the expanded system home directory.
func (c *Config) SystemHome() string {
	if c.System.Home == "" || c.System.Home == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "."
		}
		return home
	}
	return expandTilde(c.System.Home)
}

// UpgradeV1ToV2 applies v2 defaults to a v1 config.
func (c *Config) UpgradeV1ToV2() {
	if c.Version >= 2 {
		return
	}
	c.Version = 2
	if c.System.Home == "" {
		c.System.Home = "~"
	}
	if c.System.DBPath == "" {
		c.System.DBPath = "~/.computecommander/cc.db"
	}
	if c.System.DashboardLayout == "" {
		c.System.DashboardLayout = "~/.computecommander/layouts/cmdr-dashboard.kdl"
	}
	// Preserve per-project SQLite path. The cmdr-bridge hook writes agent
	// sessions to the project-local .computecommander/local.db, so remapping
	// to the system-wide cc.db would cause cmdr status to read from a different
	// database than the one agents are registered in.
	// Migrate zellij dashboard layout
	if c.Zellij.DashboardLayout == ".computecommander/layouts/cmdr-dashboard.kdl" {
		c.Zellij.DashboardLayout = "~/.computecommander/layouts/cmdr-dashboard.kdl"
	}
	if c.Projects == nil {
		c.Projects = []ProjectEntry{}
	}
}

// Validate checks the config for required fields and valid values.
func (c *Config) Validate() error {
	var errs []string

	if c.Version < 1 {
		errs = append(errs, "version must be >= 1")
	}

	if c.Version > 2 {
		errs = append(errs, fmt.Sprintf("version %d is not supported (max: 2)", c.Version))
	}

	if c.Database.Driver != "postgres" && c.Database.Driver != "sqlite" {
		errs = append(errs, fmt.Sprintf("database.driver must be 'postgres' or 'sqlite', got %q", c.Database.Driver))
	}

	if c.Database.Driver == "postgres" {
		if c.Database.Postgres.Host == "" {
			errs = append(errs, "database.postgres.host is required when driver is postgres")
		}
		if c.Database.Postgres.Port < 1 || c.Database.Postgres.Port > 65535 {
			errs = append(errs, "database.postgres.port must be between 1 and 65535")
		}
	}

	if c.Database.Driver == "sqlite" {
		if c.Database.SQLite.Path == "" {
			errs = append(errs, "database.sqlite.path is required when driver is sqlite")
		}
	}

	if c.Agents.MaxConcurrent < 1 {
		errs = append(errs, "agents.max_concurrent must be >= 1")
	}

	if c.Agents.MaxDepth < 0 {
		errs = append(errs, "agents.max_depth must be >= 0")
	}

	validLayouts := map[string]bool{"default": true, "vertical": true, "horizontal": true}
	if !validLayouts[c.Zellij.Layout] {
		errs = append(errs, fmt.Sprintf("zellij.layout must be one of: default, vertical, horizontal; got %q", c.Zellij.Layout))
	}

	validRuntimes := map[string]bool{"claude": true, "gemini": true, "codex": true, "pi": true, "goose": true}
	if !validRuntimes[c.Defaults.Runtime] {
		errs = append(errs, fmt.Sprintf("defaults.runtime must be one of: claude, gemini, codex, pi, goose; got %q", c.Defaults.Runtime))
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		errs = append(errs, fmt.Sprintf("logging.level must be one of: debug, info, warn, error; got %q", c.Logging.Level))
	}

	validFormats := map[string]bool{"human": true, "json": true}
	if !validFormats[c.Logging.Format] {
		errs = append(errs, fmt.Sprintf("logging.format must be one of: human, json; got %q", c.Logging.Format))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}
