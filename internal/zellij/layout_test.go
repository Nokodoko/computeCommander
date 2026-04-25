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
	// Layout must include either the Rust binary, the bash fallback, or the restart wrapper.
	hasBash := strings.Contains(layout, "focus-watcher.sh")
	hasRust := strings.Contains(layout, "focus-watcher") && strings.Contains(layout, "--tab-hash")
	hasWrapper := strings.Contains(layout, "focus-watcher-wrapper.sh")
	if !hasBash && !hasRust && !hasWrapper {
		t.Errorf("layout must contain a focus-watcher pane (Rust binary or bash fallback):\n%s", layout)
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
	// Verify focus-watcher pane is included (Rust binary, bash fallback, or restart wrapper).
	hasBash := strings.Contains(string(data), "focus-watcher.sh")
	hasRust := strings.Contains(string(data), "focus-watcher") && strings.Contains(string(data), "--tab-hash")
	hasWrapper := strings.Contains(string(data), "focus-watcher-wrapper.sh")
	if !hasBash && !hasRust && !hasWrapper {
		t.Errorf("written layout must contain a focus-watcher pane (Rust binary or bash fallback)")
	}
	if strings.Contains(string(data), "focus-tracker.wasm") {
		t.Errorf("written layout must NOT contain deprecated focus-tracker WASM plugin")
	}
	// Verify focus-watcher is available: either the bash script was generated
	// (when no Rust binary is found), or the wrapper script was generated
	// (wrapping whichever watcher binary/script was resolved).
	fwScript := filepath.Join(dir, ".computecommander", "scripts", "focus-watcher.sh")
	fwWrapperPath := filepath.Join(dir, ".computecommander", "scripts", "focus-watcher-wrapper.sh")
	hasBashOnDisk := false
	if _, err := os.Stat(fwScript); err == nil {
		hasBashOnDisk = true
	}
	hasWrapperOnDisk := false
	if _, err := os.Stat(fwWrapperPath); err == nil {
		hasWrapperOnDisk = true
	}
	hasWatcherInLayout := strings.Contains(string(data), "focus-watcher")
	if !hasBashOnDisk && !hasWrapperOnDisk && !hasWatcherInLayout {
		t.Errorf("no focus-watcher variant found: neither bash script at %s, wrapper at %s, nor reference in layout", fwScript, fwWrapperPath)
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

func TestGenerateLayout_ContainsTGPane(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "/usr/local/bin/cmdr",
		ProjectDir:   "/home/user",
		AgentCommand: "claude --dangerously-skip-permissions",
		SystemWide:   true,
	}
	layout := GenerateLayout(opts)
	if !strings.Contains(layout, `name="TG Viz"`) {
		t.Errorf("layout must contain a TG Viz (TrustGraph visualization) pane")
	}
	// The TG Viz pane is passive — it runs `tail -f /dev/null` so zellij
	// does not spawn a default shell, leaving the pane empty/transparent for
	// the trustgraph-viewer Electron overlay to render through.
	if !strings.Contains(layout, `command "tail"`) || !strings.Contains(layout, `"-f" "/dev/null"`) {
		t.Errorf("layout TG Viz pane must run `tail -f /dev/null` to stay passive")
	}
	// The agent session pane must be named "Agent" so external tools can
	// distinguish it from the TG Viz overlay target.
	if !strings.Contains(layout, `name="Agent"`) {
		t.Errorf("layout must contain a named Agent pane for the central agent session")
	}
	// The agent pane must occupy the full top section of the central column
	// (67% height). If TG Viz is stacked underneath it again, this width drops
	// back to 14% and the agent shrinks to a sliver.
	if !strings.Contains(layout, `name="Agent" size="67%"`) {
		t.Errorf("Agent pane must be 67%% of the central column (TG Viz must NOT be stacked above/below it):\n%s", layout)
	}
	// TG Viz must live on the bottom row (next to LazyGit). The bottom-row
	// vertical split contains it; we assert by checking it's wider than the
	// original 20%% — a regression to the central-column layout would size
	// it 86%%, which would also fail the bottom-row siblings check above.
	if !strings.Contains(layout, `name="TG Viz" size="38%"`) {
		t.Errorf("TG Viz must be on the bottom row at 38%% width, not stacked in the central column:\n%s", layout)
	}
}
