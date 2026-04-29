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
	// The TG Viz pane runs `cmdr tg-list` — a refreshing text list of
	// TrustGraph nodes and edges. The prior Electron overlay was retired
	// for resource reasons; the pane no longer needs to stay passive.
	if !strings.Contains(layout, `args "tg-list"`) {
		t.Errorf("layout TG Viz pane must run `cmdr tg-list`:\n%s", layout)
	}
	// Sanity: the old passive placeholder must be gone.
	if strings.Contains(layout, `"-f" "/dev/null"`) {
		t.Errorf("layout must no longer contain the legacy `tail -f /dev/null` placeholder:\n%s", layout)
	}
	// The agent session pane must be named "Agent" so external tools can
	// distinguish it from other panes.
	if !strings.Contains(layout, `name="Agent"`) {
		t.Errorf("layout must contain a named Agent pane for the central agent session")
	}
	// The agent pane must occupy the central column of the top row.
	// Width is 64% (reduced from 67% to widen the left column for calcurse).
	// If TG Viz is stacked underneath it again, this width collapses and the
	// agent shrinks to a sliver.
	if !strings.Contains(layout, `name="Agent" size="64%"`) {
		t.Errorf("Agent pane must be 64%% of the central column (TG Viz must NOT be stacked above/below it):\n%s", layout)
	}
	// TG Viz lives on the bottom row at 20%% (reduced from 38%% when the
	// Electron overlay was replaced with the lighter-weight tg-list text view).
	if !strings.Contains(layout, `name="TG Viz" size="20%"`) {
		t.Errorf("TG Viz must be on the bottom row at 20%% width:\n%s", layout)
	}
	// Bottom row reordered (UX request): OB1 widened and moved to the left,
	// Event Log narrowed and moved next to it. Evals trimmed, TG Viz/LazyGit unchanged.
	if !strings.Contains(layout, `name="OB1" size="28%"`) {
		t.Errorf("OB1 must be 28%% wide and on the left of the bottom row:\n%s", layout)
	}
	if !strings.Contains(layout, `name="Event Log" size="16%"`) {
		t.Errorf("Event Log must be 16%% wide:\n%s", layout)
	}
	if !strings.Contains(layout, `name="Evals" size="20%"`) {
		t.Errorf("Evals must be 20%% wide:\n%s", layout)
	}
	// OB1 must appear before Event Log in the layout text (left of it on screen).
	ob1Idx := strings.Index(layout, `name="OB1"`)
	evIdx := strings.Index(layout, `name="Event Log"`)
	if ob1Idx < 0 || evIdx < 0 || ob1Idx > evIdx {
		t.Errorf("OB1 must appear before Event Log in the bottom row (OB1 at %d, Event Log at %d):\n%s", ob1Idx, evIdx, layout)
	}
}

// TestGenerateLayout_EvalsPaneIsLive guards the wiring contract for the
// bottom-row Evals pane: it MUST point at `cmdr evals --pane`, the streaming
// long-lived handler that watches the SQLite DB via fsnotify and re-renders
// on every write. A regression to a one-shot command (e.g., `cmdr evals`
// without `--pane`) would freeze the pane on the first row of output and
// hide all subsequent eval activity, which is the user-reported bug this
// test exists to prevent.
func TestGenerateLayout_EvalsPaneIsLive(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "cmdr",
		ProjectDir:   "/tmp/proj",
		AgentCommand: "claude",
	}
	layout := GenerateLayout(opts)

	// Evals pane must be present and named.
	if !strings.Contains(layout, `name="Evals"`) {
		t.Fatalf("layout must contain a named Evals pane:\n%s", layout)
	}

	// The pane must invoke the live streaming handler.
	// Locate the Evals pane block and verify its args.
	evalsIdx := strings.Index(layout, `name="Evals"`)
	if evalsIdx < 0 {
		t.Fatalf("Evals pane not found")
	}
	// Look at a window after the pane declaration that should contain the
	// command + args lines for that pane.
	endIdx := evalsIdx + 256
	if endIdx > len(layout) {
		endIdx = len(layout)
	}
	block := layout[evalsIdx:endIdx]
	if !strings.Contains(block, `args "evals" "--pane"`) {
		t.Errorf("Evals pane must run `cmdr evals --pane` (the streaming handler) — without --pane the pane displays a one-shot snapshot and never updates:\n%s", block)
	}
}
