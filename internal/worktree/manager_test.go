package worktree

import (
	"fmt"
	"strings"
	"testing"
)

// mockRunner records commands and returns preset output.
type mockRunner struct {
	calls  []string
	output []byte
	err    error
}

func (m *mockRunner) Run(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return m.output, m.err
}

func (m *mockRunner) RunInDir(dir, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, fmt.Sprintf("[%s] %s %s", dir, name, strings.Join(args, " ")))
	return m.output, m.err
}

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/worktrees")
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.baseDir != "/tmp/worktrees" {
		t.Errorf("baseDir = %q, want %q", m.baseDir, "/tmp/worktrees")
	}
}

func TestWorktreeState(t *testing.T) {
	tt := []struct {
		state WorktreeState
		want  string
	}{
		{WorktreeActive, "active"},
		{WorktreeCompleted, "completed"},
		{WorktreeMerged, "merged"},
		{WorktreeOrphaned, "orphaned"},
	}

	for _, tc := range tt {
		if string(tc.state) != tc.want {
			t.Errorf("WorktreeState = %q, want %q", tc.state, tc.want)
		}
	}
}

func TestCreateWorktree(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("/tmp/wt", runner)

	wt, err := m.Create(CreateOpts{
		Branch: "feature-abc",
		Agent:  "builder-1",
		TaskID: "task-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wt.Branch != "feature-abc" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature-abc")
	}
	if wt.Agent != "builder-1" {
		t.Errorf("Agent = %q, want %q", wt.Agent, "builder-1")
	}
	if wt.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", wt.TaskID, "task-123")
	}
	if wt.State != WorktreeActive {
		t.Errorf("State = %q, want %q", wt.State, WorktreeActive)
	}
	if wt.Path != "/tmp/wt/feature-abc" {
		t.Errorf("Path = %q, want %q", wt.Path, "/tmp/wt/feature-abc")
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
	wantCmd := "git worktree add -b feature-abc /tmp/wt/feature-abc"
	if runner.calls[0] != wantCmd {
		t.Errorf("command = %q\n   want = %q", runner.calls[0], wantCmd)
	}
}

func TestCreateWorktreeCustomBaseDir(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("/default", runner)

	tmpDir := t.TempDir()
	customBase := tmpDir + "/custom/base"

	wt, err := m.Create(CreateOpts{
		Branch:  "fix-bug",
		Agent:   "worker",
		BaseDir: customBase,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := customBase + "/fix-bug"
	if wt.Path != want {
		t.Errorf("Path = %q, want %q", wt.Path, want)
	}
}

func TestCreateWorktreeNoBranch(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("/tmp/wt", runner)

	_, err := m.Create(CreateOpts{Agent: "worker"})
	if err == nil {
		t.Fatal("expected error for empty branch, got nil")
	}
}

func TestCreateWorktreeGitError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("branch already exists")}
	m := NewManagerWithRunner("/tmp/wt", runner)

	_, err := m.Create(CreateOpts{Branch: "existing"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListWorktrees(t *testing.T) {
	porcelainOutput := `worktree /home/user/project
branch refs/heads/main

worktree /home/user/project/.worktrees/feature-a
branch refs/heads/feature-a

worktree /home/user/project/.worktrees/detached
detached

`
	runner := &mockRunner{output: []byte(porcelainOutput)}
	m := NewManagerWithRunner("/tmp/wt", runner)

	worktrees, err := m.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(worktrees))
	}

	if worktrees[0].Path != "/home/user/project" {
		t.Errorf("worktrees[0].Path = %q, want %q", worktrees[0].Path, "/home/user/project")
	}
	if worktrees[0].Branch != "main" {
		t.Errorf("worktrees[0].Branch = %q, want %q", worktrees[0].Branch, "main")
	}

	if worktrees[1].Branch != "feature-a" {
		t.Errorf("worktrees[1].Branch = %q, want %q", worktrees[1].Branch, "feature-a")
	}

	if worktrees[2].Branch != "(detached)" {
		t.Errorf("worktrees[2].Branch = %q, want %q", worktrees[2].Branch, "(detached)")
	}
}

func TestListWorktreesError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("not a git repo")}
	m := NewManagerWithRunner("/tmp/wt", runner)

	_, err := m.List()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveWorktree(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("/tmp/wt", runner)

	err := m.Remove("/tmp/wt/feature-done", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCmd := "git worktree remove /tmp/wt/feature-done"
	if len(runner.calls) != 1 || runner.calls[0] != wantCmd {
		t.Errorf("command = %q, want %q", runner.calls[0], wantCmd)
	}
}

func TestRemoveWorktreeForce(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("/tmp/wt", runner)

	err := m.Remove("/tmp/wt/dirty", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCmd := "git worktree remove /tmp/wt/dirty --force"
	if len(runner.calls) != 1 || runner.calls[0] != wantCmd {
		t.Errorf("command = %q, want %q", runner.calls[0], wantCmd)
	}
}

func TestRemoveWorktreeError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("worktree not found")}
	m := NewManagerWithRunner("/tmp/wt", runner)

	err := m.Remove("/tmp/wt/missing", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCleanWorktrees(t *testing.T) {
	porcelainOutput := `worktree /home/user/project
branch refs/heads/main

worktree /home/user/project/.wt/completed-branch
branch refs/heads/completed-branch

`
	runner := &mockRunner{output: []byte(porcelainOutput)}
	m := NewManagerWithRunner("/tmp/wt", runner)

	// All worktrees default to "active" state from parsing,
	// so cleaning completed/orphaned should remove 0.
	count, err := m.Clean(CleanOpts{
		States: []WorktreeState{WorktreeCompleted, WorktreeOrphaned},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("removed = %d, want 0 (all active)", count)
	}
}

func TestCleanWorktreesDryRun(t *testing.T) {
	runner := &mockRunner{output: []byte("worktree /tmp/x\nbranch refs/heads/main\n\n")}
	m := NewManagerWithRunner("/tmp/wt", runner)

	count, err := m.Clean(CleanOpts{
		States: []WorktreeState{WorktreeActive},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("removed = %d, want 1", count)
	}
}

func TestCleanWorktreesError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("git error")}
	m := NewManagerWithRunner("/tmp/wt", runner)

	_, err := m.Clean(CleanOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseWorktreeList(t *testing.T) {
	tt := []struct {
		name   string
		input  string
		wantN  int
		branch string
	}{
		{"empty", "", 0, ""},
		{"bare", "worktree /repo\nbare\n\n", 1, "(bare)"},
		{"single with branch", "worktree /repo\nbranch refs/heads/main\n\n", 1, "main"},
		{"no trailing newline", "worktree /repo\nbranch refs/heads/dev", 1, "dev"},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			wts := parseWorktreeList(tc.input)
			if len(wts) != tc.wantN {
				t.Errorf("parseWorktreeList returned %d worktrees, want %d", len(wts), tc.wantN)
			}
			if tc.wantN > 0 && wts[0].Branch != tc.branch {
				t.Errorf("Branch = %q, want %q", wts[0].Branch, tc.branch)
			}
		})
	}
}

// Verify Manager satisfies WorktreeManager interface at compile time.
var _ WorktreeManager = (*Manager)(nil)
