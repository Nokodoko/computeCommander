package keybinds

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
	if cfg.Leader != "ctrl+space" {
		t.Errorf("expected leader ctrl+space, got %q", cfg.Leader)
	}
	if len(cfg.Bindings) == 0 {
		t.Error("expected non-empty bindings")
	}
	// Check a few known bindings.
	if cfg.Bindings["?"] != "help" {
		t.Errorf("expected ? -> help, got %q", cfg.Bindings["?"])
	}
	if cfg.Bindings["q"] != "quit" {
		t.Errorf("expected q -> quit, got %q", cfg.Bindings["q"])
	}
	if cfg.Bindings["d"] != "fp" {
		t.Errorf("expected d -> fp, got %q", cfg.Bindings["d"])
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/keybinds.yaml")
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected default config version 1, got %d", cfg.Version)
	}
}

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybinds.yaml")
	content := `version: 1
leader: "ctrl+space"
bindings:
  "?": help
  "x": custom
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Bindings["x"] != "custom" {
		t.Errorf("expected x -> custom, got %q", cfg.Bindings["x"])
	}
}

func TestLoadConfigInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybinds.yaml")
	content := `version: 0
leader: "ctrl+space"
bindings: {}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for version 0")
	}
}

func TestLookupAction(t *testing.T) {
	cfg := DefaultConfig()
	if action := cfg.LookupAction("?"); action != "help" {
		t.Errorf("expected help, got %q", action)
	}
	if action := cfg.LookupAction("Z"); action != "" {
		t.Errorf("expected empty for unbound key, got %q", action)
	}
}

func TestActionKeys(t *testing.T) {
	cfg := DefaultConfig()
	keys := cfg.ActionKeys("help")
	if len(keys) != 1 || keys[0] != "?" {
		t.Errorf("expected [?] for help, got %v", keys)
	}
}

func TestWriteDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybinds.yaml")
	if err := WriteDefault(path); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
}

func TestRegistryExecute(t *testing.T) {
	reg := NewRegistry()
	called := false
	reg.Register("test", "test action", func() error {
		called = true
		return nil
	})

	if !reg.HasAction("test") {
		t.Error("expected HasAction to return true")
	}
	if err := reg.Execute("test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}

	if err := reg.Execute("unknown"); err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestRegistryActions(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a", "action a", func() error { return nil })
	reg.Register("b", "action b", func() error { return nil })

	actions := reg.Actions()
	if len(actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(actions))
	}
}
