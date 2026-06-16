package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initTempRepo creates a throwaway git repo at a tempdir for tests. Returns
// the path. Runs git config with a deterministic identity so the commit
// shas are reproducible-ish (they still differ run-to-run because of
// committer date, but at least the test doesn't fail on "Please tell me
// who you are").
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, args := range [][]string{
		{"init", "-q", "--initial-branch=main"},
		{"config", "user.email", "test@cmdr.example"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeAndCommit(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	for _, args := range [][]string{
		{"add", name},
		{"commit", "-q", "-m", "commit " + name},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE=2026-05-17T14:00:00Z",
			"GIT_COMMITTER_DATE=2026-05-17T14:00:00Z",
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_TERMINAL_PROMPT=0",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestLoadSnapshot_Clean(t *testing.T) {
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")

	snap, err := LoadSnapshot(context.Background(), dir, 5)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if !snap.IsClean() {
		t.Errorf("expected clean snapshot, got %+v", snap)
	}
	if !strings.HasPrefix(snap.Branch, "main") && !strings.HasPrefix(snap.Branch, "master") {
		t.Errorf("expected branch ~ main/master, got %q", snap.Branch)
	}
	if len(snap.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(snap.Commits))
	}
	if len(snap.Commits) > 0 && snap.Commits[0].Subject != "commit README.md" {
		t.Errorf("unexpected commit subject %q", snap.Commits[0].Subject)
	}
}

func TestLoadSnapshot_StagedUnstagedUntracked(t *testing.T) {
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")

	// Modify and stage a file.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello v2\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	cmd := exec.Command("git", "-C", dir, "add", "README.md")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	// Modify staged file again (so it's also unstaged).
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello v3\n"), 0o644); err != nil {
		t.Fatalf("re-write README: %v", err)
	}
	// Add an untracked file.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("u\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	snap, err := LoadSnapshot(context.Background(), dir, 3)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if snap.Staged != 1 {
		t.Errorf("Staged = %d, want 1", snap.Staged)
	}
	if snap.Unstaged != 1 {
		t.Errorf("Unstaged = %d, want 1", snap.Unstaged)
	}
	if snap.Untracked != 1 {
		t.Errorf("Untracked = %d, want 1", snap.Untracked)
	}
	if snap.IsClean() {
		t.Errorf("snapshot must NOT be clean: %+v", snap)
	}
}

func TestLoadSnapshot_NotARepo(t *testing.T) {
	_, err := LoadSnapshot(context.Background(), "/tmp/definitely-not-a-repo-"+t.Name(), 5)
	if err == nil {
		t.Errorf("expected error on non-repo path")
	}
}

func TestLoadSnapshot_EmptyRepoPath(t *testing.T) {
	_, err := LoadSnapshot(context.Background(), "", 5)
	if err == nil {
		t.Errorf("expected error on empty repo path")
	}
}

func TestLoadSnapshot_ContextTimeout(t *testing.T) {
	// Use an already-cancelled context; LoadSnapshot should fail fast.
	dir := initTempRepo(t)
	writeAndCommit(t, dir, "README.md", "hello\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := LoadSnapshot(ctx, dir, 5)
	if err == nil {
		t.Errorf("expected error with cancelled context")
	}
}

func TestParseOneline(t *testing.T) {
	in := "abc1234 first commit\ndef5678 second commit\n\n"
	got := parseOneline(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(got))
	}
	if got[0].ShortSha != "abc1234" || got[0].Subject != "first commit" {
		t.Errorf("commit 0 = %+v", got[0])
	}
	if got[1].ShortSha != "def5678" || got[1].Subject != "second commit" {
		t.Errorf("commit 1 = %+v", got[1])
	}
}

func TestParsePorcelainV2_BranchAB(t *testing.T) {
	in := "# branch.head feat/foo\n# branch.upstream origin/main\n# branch.ab +3 -2\n? newfile.go\n"
	got, err := parsePorcelainV2(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "feat/foo" {
		t.Errorf("Branch = %q", got.Branch)
	}
	if got.Ahead != 3 || got.Behind != 2 {
		t.Errorf("Ahead=%d Behind=%d, want 3,2", got.Ahead, got.Behind)
	}
	if got.Untracked != 1 {
		t.Errorf("Untracked = %d, want 1", got.Untracked)
	}
}

// keep import linter happy
var _ = time.Second
