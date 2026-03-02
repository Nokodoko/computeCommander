package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestWatcherLoadsInitialConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Project.Name = "test-project"
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(cfgPath)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	got := w.Config()
	if got.Project.Name != "test-project" {
		t.Errorf("expected project name 'test-project', got %q", got.Project.Name)
	}
}

func TestWatcherReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Project.Name = "initial"
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(cfgPath)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	// Register change handler.
	changed := make(chan string, 1)
	w.OnChange(func(newCfg *Config) {
		changed <- newCfg.Project.Name
	})

	// Modify the config file.
	cfg.Project.Name = "updated"
	data, err = yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the change notification.
	select {
	case name := <-changed:
		if name != "updated" {
			t.Errorf("expected 'updated', got %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for config change notification")
	}
}

func TestWatcherInvalidReload(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.Project.Name = "original"
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(cfgPath)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Close()

	// Write invalid YAML.
	if err := os.WriteFile(cfgPath, []byte("invalid: [yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait briefly for the watcher to process.
	time.Sleep(200 * time.Millisecond)

	// The original config should be preserved.
	got := w.Config()
	if got.Project.Name != "original" {
		t.Errorf("expected config to be preserved as 'original', got %q", got.Project.Name)
	}
}
