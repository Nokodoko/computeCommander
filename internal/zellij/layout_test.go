package zellij

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateLayout_NoKeybinds(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "/usr/local/bin/cmdr",
		ProjectDir:   "/home/user",
		AgentCommand: "claude --dangerously-skip-permissions",
		SystemWide:   true,
	}
	layout := GenerateLayout(opts)
	if strings.Contains(layout, "keybinds") {
		t.Errorf("layout must not contain 'keybinds' block:\n%s", layout)
	}
	if !strings.Contains(layout, `tab name="[CMDR] Dashboard"`) {
		t.Errorf("layout missing tab declaration")
	}
}

func TestGenerateLayout_NoKeybinds_NonSystemWide(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "cmdr",
		ProjectDir:   "/tmp/proj",
		AgentCommand: "claude",
		SystemWide:   false,
	}
	layout := GenerateLayout(opts)
	if strings.Contains(layout, "keybinds") {
		t.Errorf("layout must not contain 'keybinds' block:\n%s", layout)
	}
}

func TestGenerateLayout_IncludesFocusWatcher(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "cmdr",
		ProjectDir:   "/tmp/proj",
		AgentCommand: "claude",
		TabHash:      "abc12345",
	}
	layout := GenerateLayout(opts)
	if strings.Contains(layout, "focus-tracker.wasm") {
		t.Errorf("layout must NOT contain the deprecated focus-tracker WASM plugin:\n%s", layout)
	}
	if !strings.Contains(layout, "focus-watcher.sh") {
		t.Errorf("layout must contain focus-watcher.sh pane:\n%s", layout)
	}
	if !strings.Contains(layout, `"abc12345"`) {
		t.Errorf("layout must pass tab_hash to focus-watcher:\n%s", layout)
	}
}

func TestWriteLayout_NoKeybinds(t *testing.T) {
	dir := t.TempDir()
	layoutPath := filepath.Join(dir, "cmdr-dashboard.kdl")
	// Use SystemWide=false so wrapper scripts go to the temp dir,
	// not ~/.computecommander/scripts/ which would overwrite production scripts.
	opts := LayoutOpts{
		CmdrBinary:   "cmdr",
		ProjectDir:   dir,
		AgentCommand: "claude",
		SystemWide:   false,
	}
	if err := WriteLayout(layoutPath, opts); err != nil {
		t.Fatalf("WriteLayout: %v", err)
	}
	data, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if strings.Contains(string(data), "keybinds") {
		t.Errorf("written layout must not contain 'keybinds':\n%s", string(data))
	}
	// Verify focus-watcher shell script pane is included (WASM plugin replaced).
	if !strings.Contains(string(data), "focus-watcher.sh") {
		t.Errorf("written layout must contain focus-watcher.sh pane")
	}
	if strings.Contains(string(data), "focus-tracker.wasm") {
		t.Errorf("written layout must NOT contain deprecated focus-tracker WASM plugin")
	}
	// Verify focus-watcher script was generated.
	fwScript := filepath.Join(dir, ".computecommander", "scripts", "focus-watcher.sh")
	if _, err := os.Stat(fwScript); os.IsNotExist(err) {
		t.Errorf("focus-watcher.sh not generated at %s", fwScript)
	}
	// Verify wrapper scripts were generated in the project dir, not home.
	fpWrapper := filepath.Join(dir, ".computecommander", "scripts", "fp-wrapper.sh")
	if _, err := os.Stat(fpWrapper); os.IsNotExist(err) {
		t.Errorf("fp-wrapper.sh not generated at %s", fpWrapper)
	}
	lgWrapper := filepath.Join(dir, ".computecommander", "scripts", "lazygit-wrapper.sh")
	if _, err := os.Stat(lgWrapper); os.IsNotExist(err) {
		t.Errorf("lazygit-wrapper.sh not generated at %s", lgWrapper)
	}
}
