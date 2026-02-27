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
	Project   ProjectConfig    `yaml:"project"`
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
}

type WorktreesConfig struct {
	BaseDir string `yaml:"base_dir"`
}

type DefaultsConfig struct {
	Runtime       string            `yaml:"runtime"`
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
	Tier0Enabled      bool `yaml:"tier0_enabled"`
	Tier0IntervalMs   int  `yaml:"tier0_interval_ms"`
	Tier1Enabled      bool `yaml:"tier1_enabled"`
	Tier2Enabled      bool `yaml:"tier2_enabled"`
	StaleThresholdMs  int  `yaml:"stale_threshold_ms"`
	ZombieThresholdMs int  `yaml:"zombie_threshold_ms"`
	NudgeIntervalMs   int  `yaml:"nudge_interval_ms"`
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
		Version: 1,
		Project: ProjectConfig{
			CanonicalBranch: "main",
		},
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
				Path: ".computecommander/local.db",
			},
		},
		Zellij: ZellijConfig{
			Layout:        "default",
			DashboardLayout: "~/.computecommander/layouts/cmdr-dashboard.kdl",
			Terminal:      "wezterm",
			SessionPrefix: "cc",
		},
		Agents: AgentsConfig{
			MaxConcurrent:     10,
			StaggerDelayMs:    2000,
			MaxDepth:          2,
			MaxSessionsPerRun: 50,
			MaxAgentsPerLead:  5,
			BaseDir:           "agents",
		},
		Worktrees: WorktreesConfig{
			BaseDir: ".computecommander/worktrees",
		},
		Defaults: DefaultsConfig{
			Runtime: "claude",
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

	// Expand tilde in path-valued fields. Go does not expand "~/" automatically,
	// so paths like "~/.computecommander/layouts/foo.kdl" would be passed as
	// literal strings to exec.Command, causing file-not-found failures.
	cfg.Zellij.DashboardLayout = expandTilde(cfg.Zellij.DashboardLayout)
	cfg.Database.SQLite.Path = expandTilde(cfg.Database.SQLite.Path)
	cfg.Worktrees.BaseDir = expandTilde(cfg.Worktrees.BaseDir)

	return cfg, nil
}

// Validate checks the config for required fields and valid values.
func (c *Config) Validate() error {
	var errs []string

	if c.Version < 1 {
		errs = append(errs, "version must be >= 1")
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
