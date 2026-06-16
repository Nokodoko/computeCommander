package commands

import (
	"strings"
	"testing"
)

func TestLazygitSummary_NoColorWidthLines(t *testing.T) {
	// Use a guaranteed non-repo path so the test is deterministic.
	got := runSummaryCmd(t, "lazygit-summary", "--lines", "5", "--width", "50", "--no-color", "--repo", "/tmp/cmdr-test-not-a-repo-"+t.Name())
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
	if lines[0] != "git: not a repo" {
		t.Errorf("non-repo path must produce 'git: not a repo', got %q", lines[0])
	}
}

func TestLazygitSummary_ZeroLines(t *testing.T) {
	got := runSummaryCmd(t, "lazygit-summary", "--lines", "0", "--width", "60", "--no-color", "--repo", "/tmp/anything")
	if got != "" {
		t.Errorf("--lines 0 must produce no output, got %q", got)
	}
}

func TestLazygitSummary_ZeroWidth(t *testing.T) {
	got := runSummaryCmd(t, "lazygit-summary", "--lines", "3", "--width", "0", "--no-color", "--repo", "/tmp/anything")
	lines := splitLines(got)
	if len(lines) != 3 || lines[0] != "git: not a repo" {
		t.Errorf("--width 0 must produce degraded marker padded to 3 lines, got %v", lines)
	}
}

func TestLazygitSummary_LiveRepo(t *testing.T) {
	// Use the cmdr repo itself as a known git repo.
	got := runSummaryCmd(t, "lazygit-summary", "--lines", "6", "--width", "80", "--no-color", "--repo", "/home/n0ko/Programs/ai/computeCommander")
	lines := splitLines(got)
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d:\n%q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "Git") {
		t.Errorf("first line should start with 'Git', got %q", lines[0])
	}
}
