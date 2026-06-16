package commands

import (
	"strings"
	"testing"
)

func TestEvalsSummary_NoColorWidthLines(t *testing.T) {
	got := runSummaryCmd(t, "evals-summary", "--lines", "5", "--width", "50", "--no-color")
	lines := splitLines(got)
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d:\n%q", len(lines), got)
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

func TestEvalsSummary_DegradedMarker_NilDB(t *testing.T) {
	got := runSummaryCmd(t, "evals-summary", "--lines", "4", "--width", "60", "--no-color")
	lines := splitLines(got)
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%q", len(lines), got)
	}
	if lines[0] != "evals: unavailable" {
		t.Errorf("expected 'evals: unavailable' on first line, got %q", lines[0])
	}
}

func TestEvalsSummary_ZeroLines(t *testing.T) {
	got := runSummaryCmd(t, "evals-summary", "--lines", "0", "--width", "60", "--no-color")
	if got != "" {
		t.Errorf("--lines 0 must produce no output, got %q", got)
	}
}

func TestEvalsSummary_ZeroWidth(t *testing.T) {
	got := runSummaryCmd(t, "evals-summary", "--lines", "3", "--width", "0", "--no-color")
	lines := splitLines(got)
	if len(lines) != 3 || lines[0] != "evals: unavailable" {
		t.Errorf("--width 0 must produce degraded marker padded to 3 lines, got %v", lines)
	}
}
