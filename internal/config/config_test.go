package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("expected driver sqlite, got %s", cfg.Database.Driver)
	}
	if cfg.Defaults.Runtime != "claude" {
		t.Errorf("expected runtime claude, got %s", cfg.Defaults.Runtime)
	}
	if cfg.Agents.MaxConcurrent != 10 {
		t.Errorf("expected max_concurrent 10, got %d", cfg.Agents.MaxConcurrent)
	}
}

func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should validate: %v", err)
	}
}

func TestValidateInvalidDriver(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.Driver = "mysql"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid driver")
	}
}

func TestValidateInvalidRuntime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.Runtime = "chatgpt"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid runtime")
	}
}

func TestValidateInvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logging.Level = "trace"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid log level")
	}
}

func TestValidateInvalidLayout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Zellij.Layout = "grid"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for invalid layout")
	}
}

func TestValidateMaxConcurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.MaxConcurrent = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for max_concurrent < 1")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	content := `version: 1
project:
  name: test-project
  canonical_branch: develop
database:
  driver: sqlite
  sqlite:
    path: test.db
zellij:
  layout: default
  terminal: wezterm
  session_prefix: cc
agents:
  max_concurrent: 5
  stagger_delay_ms: 1000
  max_depth: 2
  max_sessions_per_run: 50
  max_agents_per_lead: 5
  base_dir: agents
defaults:
  runtime: claude
logging:
  level: info
  format: human
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Project.Name != "test-project" {
		t.Errorf("expected project name 'test-project', got %q", cfg.Project.Name)
	}
	if cfg.Project.CanonicalBranch != "develop" {
		t.Errorf("expected canonical_branch 'develop', got %q", cfg.Project.CanonicalBranch)
	}
	if cfg.Agents.MaxConcurrent != 5 {
		t.Errorf("expected max_concurrent 5, got %d", cfg.Agents.MaxConcurrent)
	}
}

func TestLoadConfigEnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	t.Setenv("CC_TEST_PASSWORD", "secret123")
	t.Setenv("CC_TEST_HOST", "db.example.com")

	content := `version: 1
project:
  name: env-test
database:
  driver: postgres
  postgres:
    host: ${CC_TEST_HOST}
    port: 5432
    database: computecommander
    user: cc
    password: ${CC_TEST_PASSWORD}
    sslmode: disable
    pool_size: 10
  sqlite:
    path: local.db
zellij:
  layout: default
  terminal: wezterm
  session_prefix: cc
agents:
  max_concurrent: 10
  stagger_delay_ms: 2000
  max_depth: 2
  max_sessions_per_run: 50
  max_agents_per_lead: 5
  base_dir: agents
defaults:
  runtime: claude
logging:
  level: info
  format: human
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Database.Postgres.Password != "secret123" {
		t.Errorf("expected password 'secret123', got %q", cfg.Database.Postgres.Password)
	}
	if cfg.Database.Postgres.Host != "db.example.com" {
		t.Errorf("expected host 'db.example.com', got %q", cfg.Database.Postgres.Host)
	}
}

func TestLoadConfigLocalOverlay(t *testing.T) {
	dir := t.TempDir()

	mainConfig := `version: 1
project:
  name: overlay-test
database:
  driver: sqlite
  sqlite:
    path: local.db
zellij:
  layout: default
  terminal: wezterm
  session_prefix: cc
agents:
  max_concurrent: 10
  stagger_delay_ms: 2000
  max_depth: 2
  max_sessions_per_run: 50
  max_agents_per_lead: 5
  base_dir: agents
defaults:
  runtime: claude
logging:
  verbose: false
  level: info
  format: human
`
	localConfig := `logging:
  verbose: true
  level: debug
defaults:
  runtime: gemini
`
	configPath := filepath.Join(dir, "config.yaml")
	localPath := filepath.Join(dir, "config.local.yaml")

	if err := os.WriteFile(configPath, []byte(mainConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte(localConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.Logging.Verbose {
		t.Error("expected logging.verbose to be true from local overlay")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level 'debug' from local overlay, got %q", cfg.Logging.Level)
	}
	if cfg.Defaults.Runtime != "gemini" {
		t.Errorf("expected defaults.runtime 'gemini' from local overlay, got %q", cfg.Defaults.Runtime)
	}
	// Verify main config values are preserved
	if cfg.Project.Name != "overlay-test" {
		t.Errorf("expected project name 'overlay-test' from main config, got %q", cfg.Project.Name)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestExpandEnvVarsUnsetVar(t *testing.T) {
	os.Unsetenv("CC_UNSET_VAR_12345")
	input := []byte("password: ${CC_UNSET_VAR_12345}")
	result := expandEnvVars(input)
	expected := "password: ${CC_UNSET_VAR_12345}"
	if string(result) != expected {
		t.Errorf("expected unset var to remain, got %q", string(result))
	}
}
