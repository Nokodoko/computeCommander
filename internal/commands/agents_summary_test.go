package commands

import (
	"strings"
	"testing"
)

// splitLines splits captured stdout into N lines where N is exactly
// the count of newline terminators. `fmt.Fprintln` always appends "\n",
// so a 5-line output is the bytes "line0\nline1\n...line4\n" — strings.Split
// on "\n" yields 6 entries with the last empty. We drop that trailing
// empty so the result is the actual line count.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	// Drop one trailing empty caused by the final "\n".
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// runSummaryCmd invokes a *cobra.Command in-process and returns the
// captured stdout. Mirrors the captureStdout helper in tg_summary_test.go
// but builds the command directly so we don't go through the App init
// path (which needs a real config + DB).
func runSummaryCmd(t *testing.T, cmdName string, args ...string) string {
	t.Helper()
	app := &App{} // Spawner nil → renderer degrades to "agents: unavailable"
	var cmd = AgentsSummaryCmd(app)
	switch cmdName {
	case "evals-summary":
		cmd = EvalsSummaryCmd(app)
	case "lazygit-summary":
		cmd = LazygitSummaryCmd(app)
	}
	cmd.SetArgs(args)
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			// Per phase3 contract, RunE returns nil on every failure path.
			t.Errorf("%s Execute returned err (must always be nil): %v", cmdName, err)
		}
	})
	return out
}

func TestAgentsSummary_NoColorWidthLines(t *testing.T) {
	got := runSummaryCmd(t, "agents-summary", "--lines", "5", "--width", "50", "--no-color")
	lines := splitLines(got)
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d:\n%q\n%v", len(lines), got, lines)
	}
	for i, ln := range lines {
		if len(ln) > 50 {
			t.Errorf("line %d width %d > 50: %q", i, len(ln), ln)
		}
	}
	if strings.Contains(got, "\033") {
		t.Errorf("--no-color must not emit ANSI:\n%s", got)
	}
}

func TestAgentsSummary_DegradedMarker(t *testing.T) {
	got := runSummaryCmd(t, "agents-summary", "--lines", "4", "--width", "60", "--no-color")
	lines := splitLines(got)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%q", len(lines), got)
	}
	if lines[0] != "agents: unavailable" {
		t.Errorf("expected degraded marker on first line, got %q", lines[0])
	}
}

func TestAgentsSummary_ZeroLines(t *testing.T) {
	got := runSummaryCmd(t, "agents-summary", "--lines", "0", "--width", "60", "--no-color")
	if got != "" {
		t.Errorf("--lines 0 must produce no output, got %q", got)
	}
}

func TestAgentsSummary_ZeroWidth(t *testing.T) {
	got := runSummaryCmd(t, "agents-summary", "--lines", "3", "--width", "0", "--no-color")
	lines := splitLines(got)
	if len(lines) != 3 || lines[0] != "agents: unavailable" {
		t.Errorf("--width 0 must produce degraded marker padded to 3 lines, got %v", lines)
	}
}
