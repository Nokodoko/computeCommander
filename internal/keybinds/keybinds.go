// Package keybinds provides leader-key-based keybind configuration loading and matching.
// It reads keybinds.yaml to determine what action follows each key after the leader key
// (Ctrl+Space) is pressed.
package keybinds

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the keybind configuration loaded from keybinds.yaml.
type Config struct {
	Version  int               `yaml:"version"`
	Leader   string            `yaml:"leader"`
	Bindings map[string]string `yaml:"bindings"`
}

// DefaultConfig returns the default keybind configuration matching the spec.
func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		Leader:  "ctrl+space",
		Bindings: map[string]string{
			"?": "help",
			"u": "update",
			"v": "version",
			"s": "shell",
			"c": "clear",
			"e": "export",
			"r": "restart",
			"b": "backup",
			"R": "restore",
			"f": "feedback",
			"h": "support",
			"p": "plugins",
			"t": "theme",
			"n": "notifications",
			"a": "analytics",
			"i": "integrations",
			"m": "automation",
			"A": "accessibility",
			"d": "fp",
			"q": "quit",
			"S": "sessions",
		},
	}
}

// LoadConfig reads and parses a keybinds.yaml file.
// If the file does not exist, it returns the default config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read keybinds config %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse keybinds config %s: %w", path, err)
	}

	if cfg.Version < 1 {
		return nil, fmt.Errorf("keybinds config version must be >= 1, got %d", cfg.Version)
	}

	return cfg, nil
}

// WriteDefault writes the default keybind config to the given path.
func WriteDefault(path string) error {
	cfg := DefaultConfig()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal keybinds config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LookupAction returns the action name for the given key, or empty string if not bound.
func (c *Config) LookupAction(key string) string {
	if c.Bindings == nil {
		return ""
	}
	return c.Bindings[key]
}

// ActionKeys returns all keys that map to the given action.
func (c *Config) ActionKeys(action string) []string {
	var keys []string
	for k, v := range c.Bindings {
		if v == action {
			keys = append(keys, k)
		}
	}
	return keys
}
