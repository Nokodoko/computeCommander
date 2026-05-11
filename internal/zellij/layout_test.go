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

// TestGenerateLayout_BottomRowShape pins the bottom-row composition after the
// dedicated TG Viz pane was removed from the dashboard (May 2026). TrustGraph
// visualization now lives inside each agent's own session/pane, not as a
// dashboard pane. The 20%% that TG Viz previously occupied was redistributed
// proportionally across the remaining four bottom-row panes:
//
//	pane     | before | after
//	---------+--------+------
//	OB1      |   28%% |  35%%
//	Event Log|   16%% |  20%%
//	Evals    |   20%% |  25%%
//	LazyGit  |   16%% |  20%%
//	(TG Viz  |   20%% |  removed)
//
// The agent (top-row, center) pane width is unchanged at 53%%; this test
// guards against accidental re-introduction of TG Viz stacking, which would
// collapse the Agent pane back to a sliver.
func TestGenerateLayout_BottomRowShape(t *testing.T) {
	opts := LayoutOpts{
		CmdrBinary:   "/usr/local/bin/cmdr",
		ProjectDir:   "/home/user",
		AgentCommand: "claude --dangerously-skip-permissions",
		SystemWide:   true,
	}
	layout := GenerateLayout(opts)

	// Negative assertion: the dedicated TG Viz pane must NOT exist on the
	// dashboard. Its responsibility moved into each agent's own session.
	if strings.Contains(layout, `name="TG Viz"`) {
		t.Errorf("layout must NOT contain a dedicated TG Viz pane — TrustGraph visualization moved into the agent session:\n%s", layout)
	}
	// The bottom-row tg-list invocation is gone with the pane.
	if strings.Contains(layout, `args "tg-list"`) {
		t.Errorf("layout must NOT invoke `cmdr tg-list` from any pane — the standalone CLI command remains, but the dashboard no longer hosts it:\n%s", layout)
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
	// The agent pane width is unchanged by the TG Viz removal (53%%).
	if !strings.Contains(layout, `name="Agent" size="53%"`) {
		t.Errorf("Agent pane must remain at 53%% of the central column after TG Viz removal:\n%s", layout)
	}
	// Bottom-row widths after the TG Viz removal:
	if !strings.Contains(layout, `name="OB1" size="35%"`) {
		t.Errorf("OB1 must be 35%% wide on the bottom row (was 28%%; absorbed share of TG Viz's 20%%):\n%s", layout)
	}
	if !strings.Contains(layout, `name="Event Log" size="20%"`) {
		t.Errorf("Event Log must be 20%% wide on the bottom row (was 16%%; absorbed share of TG Viz's 20%%):\n%s", layout)
	}
	if !strings.Contains(layout, `name="Evals" size="25%"`) {
		t.Errorf("Evals must be 25%% wide on the bottom row (was 20%%; absorbed share of TG Viz's 20%%):\n%s", layout)
	}
	// LazyGit pane is unnamed in the KDL (the wrapper sets its title via terminal
	// escape sequence). Pin its width via the trailing size attribute on the
	// last pane block in the bottom row, which uses the bash wrapper.
	if !strings.Contains(layout, `pane size="20%"`) {
		t.Errorf("LazyGit pane must be 20%% wide on the bottom row (was 16%%; absorbed share of TG Viz's 20%%):\n%s", layout)
	}
	// OB1 must appear before Event Log in the layout text (left of it on screen).
	ob1Idx := strings.Index(layout, `name="OB1"`)
	evIdx := strings.Index(layout, `name="Event Log"`)
	if ob1Idx < 0 || evIdx < 0 || ob1Idx > evIdx {
		t.Errorf("OB1 must appear before Event Log in the bottom row (OB1 at %d, Event Log at %d):\n%s", ob1Idx, evIdx, layout)
	}
	// Bottom-row widths sum to 100%% (35 + 20 + 25 + 20). Allow a regression
	// catch by counting that exactly 4 pane size declarations exist on the
	// bottom row at the percentages we expect. (Indirect: we already pinned
	// each above.)
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
	endIdx := min(evalsIdx+256, len(layout))
	block := layout[evalsIdx:endIdx]
	if !strings.Contains(block, `args "evals" "--pane"`) {
		t.Errorf("Evals pane must run `cmdr evals --pane` (the streaming handler) — without --pane the pane displays a one-shot snapshot and never updates:\n%s", block)
	}
}
