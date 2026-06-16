package git

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agentui"
)

func fixedNow() time.Time {
	return time.Date(2026, 5, 17, 14, 32, 7, 0, time.UTC)
}

func TestRender_CleanRepo(t *testing.T) {
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")

	opts := RenderOpts{Lines: 8, Width: 80, NoColor: true, RepoPath: dir, Now: fixedNow()}
	got := Render(context.Background(), opts)
	if len(got) != 8 {
		t.Fatalf("expected 8 lines, got %d", len(got))
	}
	for i, ln := range got {
		if v := agentui.VisibleLen(ln); v > opts.Width {
			t.Errorf("line %d width %d > %d: %q", i, v, opts.Width, ln)
		}
	}
	body := strings.Join(got, "\n")
	if !strings.Contains(got[0], "Git") {
		t.Errorf("header should contain 'Git', got %q", got[0])
	}
	if !strings.Contains(body, "clean") {
		t.Errorf("body should report 'clean':\n%s", body)
	}
	if !strings.Contains(body, "commit README.md") {
		t.Errorf("body should include the test commit:\n%s", body)
	}
	if got[len(got)-1] != "updated 14:32:07" {
		t.Errorf("trailer = %q, want 'updated 14:32:07'", got[len(got)-1])
	}
}

func TestRender_NotARepo(t *testing.T) {
	opts := RenderOpts{Lines: 5, Width: 60, NoColor: true, RepoPath: "/tmp/not-a-repo-" + t.Name(), Now: fixedNow()}
	got := Render(context.Background(), opts)
	if len(got) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(got))
	}
	if got[0] != "git: not a repo" {
		t.Errorf("non-repo must produce 'git: not a repo', got %q", got[0])
	}
}

func TestRender_EmptyRepoPath(t *testing.T) {
	opts := RenderOpts{Lines: 4, Width: 60, NoColor: true, RepoPath: "", Now: fixedNow()}
	got := Render(context.Background(), opts)
	if len(got) != 4 || got[0] != "git: not a repo" {
		t.Errorf("empty repo path must degrade, got %v", got)
	}
}

func TestRender_ZeroLines(t *testing.T) {
	got := Render(context.Background(), RenderOpts{Lines: 0, Width: 60, RepoPath: "/tmp/anything"})
	if got != nil {
		t.Errorf("Lines=0 must return nil, got %v", got)
	}
}

func TestRender_ZeroWidth(t *testing.T) {
	got := Render(context.Background(), RenderOpts{Lines: 3, Width: 0, RepoPath: "/tmp/anything", Now: fixedNow()})
	if len(got) != 3 || got[0] != "git: not a repo" {
		t.Errorf("Width=0 must degrade padded, got %v", got)
	}
}

func TestRender_NarrowWidth(t *testing.T) {
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")
	opts := RenderOpts{Lines: 6, Width: 40, NoColor: true, RepoPath: dir, Now: fixedNow()}
	got := Render(context.Background(), opts)
	if len(got) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(got))
	}
	for i, ln := range got {
		if v := agentui.VisibleLen(ln); v > 40 {
			t.Errorf("line %d width %d > 40: %q", i, v, ln)
		}
	}
}

func TestRender_Deterministic(t *testing.T) {
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")
	opts := RenderOpts{Lines: 8, Width: 80, NoColor: true, RepoPath: dir, Now: fixedNow()}
	a := Render(context.Background(), opts)
	b := Render(context.Background(), opts)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("Render must be deterministic with injected Now:\n%v\n---\n%v", a, b)
	}
}
