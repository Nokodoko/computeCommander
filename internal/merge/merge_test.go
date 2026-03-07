package merge

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

// setupTestDB creates an in-memory SQLite database with the merge_queue table.
func setupTestDB(t *testing.T) db.DB {
	t.Helper()
	d, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}

	ctx := context.Background()
	err = d.Exec(ctx, `CREATE TABLE IF NOT EXISTS merge_queue (
		branch_name TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		files_modified TEXT NOT NULL DEFAULT '[]',
		enqueued_at TEXT NOT NULL DEFAULT (datetime('now')),
		status TEXT NOT NULL DEFAULT 'pending',
		resolved_tier TEXT,
		project_id TEXT
	)`)
	if err != nil {
		t.Fatalf("create merge_queue table: %v", err)
	}

	t.Cleanup(func() { d.Close() })
	return d
}

func TestSQLQueue_EnqueueAndPeek(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	entry := &MergeEntry{
		BranchName:    "feature/auth",
		TaskID:        "task-001",
		AgentName:     "builder-1",
		FilesModified: []string{"auth.go", "auth_test.go"},
	}

	if err := q.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := q.Peek()
	if err != nil {
		t.Fatalf("peek: %v", err)
	}

	if got.BranchName != "feature/auth" {
		t.Errorf("peek branch: got %q, want %q", got.BranchName, "feature/auth")
	}
	if got.TaskID != "task-001" {
		t.Errorf("peek task_id: got %q, want %q", got.TaskID, "task-001")
	}
	if got.AgentName != "builder-1" {
		t.Errorf("peek agent_name: got %q, want %q", got.AgentName, "builder-1")
	}
	if got.Status != MergePending {
		t.Errorf("peek status: got %q, want %q", got.Status, MergePending)
	}
	if len(got.FilesModified) != 2 {
		t.Errorf("peek files_modified: got %d files, want 2", len(got.FilesModified))
	}
}

func TestSQLQueue_Dequeue(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	entry := &MergeEntry{
		BranchName:    "feature/api",
		TaskID:        "task-002",
		AgentName:     "builder-2",
		FilesModified: []string{"api.go"},
	}

	if err := q.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	if got.BranchName != "feature/api" {
		t.Errorf("dequeue branch: got %q, want %q", got.BranchName, "feature/api")
	}
	if got.Status != MergeMerging {
		t.Errorf("dequeue status: got %q, want %q", got.Status, MergeMerging)
	}

	// Peek should now return empty since the only entry is merging
	_, err = q.Peek()
	if err != ErrQueueEmpty {
		t.Errorf("peek after dequeue: got err %v, want ErrQueueEmpty", err)
	}
}

func TestSQLQueue_DequeueEmpty(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	_, err := q.Dequeue()
	if err != ErrQueueEmpty {
		t.Errorf("dequeue empty: got err %v, want ErrQueueEmpty", err)
	}
}

func TestSQLQueue_Status(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	entry := &MergeEntry{
		BranchName:    "feature/db",
		TaskID:        "task-003",
		AgentName:     "builder-3",
		FilesModified: []string{"db.go"},
	}

	if err := q.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	got, err := q.Status("feature/db")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if got.BranchName != "feature/db" {
		t.Errorf("status branch: got %q, want %q", got.BranchName, "feature/db")
	}

	// Not found
	_, err = q.Status("nonexistent")
	if err != ErrNotFound {
		t.Errorf("status nonexistent: got err %v, want ErrNotFound", err)
	}
}

func TestSQLQueue_List(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	entries := []*MergeEntry{
		{BranchName: "feature/a", TaskID: "t1", AgentName: "a1", FilesModified: []string{"a.go"}},
		{BranchName: "feature/b", TaskID: "t2", AgentName: "a2", FilesModified: []string{"b.go"}},
		{BranchName: "feature/c", TaskID: "t3", AgentName: "a3", FilesModified: []string{"c.go"}},
	}

	for _, e := range entries {
		if err := q.Enqueue(e); err != nil {
			t.Fatalf("enqueue %s: %v", e.BranchName, err)
		}
	}

	// List all
	all, err := q.List(ListOpts{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("list all: got %d entries, want 3", len(all))
	}

	// List with status filter
	pending := MergePending
	filtered, err := q.List(ListOpts{Status: &pending})
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("list pending: got %d entries, want 3", len(filtered))
	}

	// List with limit
	limited, err := q.List(ListOpts{Limit: 2})
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("list limit: got %d entries, want 2", len(limited))
	}
}

func TestSQLQueue_UpdateStatus(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	entry := &MergeEntry{
		BranchName:    "feature/update",
		TaskID:        "task-004",
		AgentName:     "builder-4",
		FilesModified: []string{"update.go"},
	}

	if err := q.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	tier := TierCleanMerge
	if err := q.UpdateStatus("feature/update", MergeMerged, &tier); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := q.Status("feature/update")
	if err != nil {
		t.Fatalf("status after update: %v", err)
	}
	if got.Status != MergeMerged {
		t.Errorf("status: got %q, want %q", got.Status, MergeMerged)
	}
	if got.ResolvedTier == nil || *got.ResolvedTier != TierCleanMerge {
		t.Errorf("resolved_tier: got %v, want %q", got.ResolvedTier, TierCleanMerge)
	}
}

func TestSQLQueue_FIFOOrder(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	// Enqueue in order
	for i := 0; i < 5; i++ {
		entry := &MergeEntry{
			BranchName:    fmt.Sprintf("feature/fifo-%d", i),
			TaskID:        fmt.Sprintf("task-%d", i),
			AgentName:     fmt.Sprintf("agent-%d", i),
			FilesModified: []string{fmt.Sprintf("file%d.go", i)},
		}
		if err := q.Enqueue(entry); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Dequeue should return in order
	for i := 0; i < 5; i++ {
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		want := fmt.Sprintf("feature/fifo-%d", i)
		if got.BranchName != want {
			t.Errorf("dequeue %d: got branch %q, want %q", i, got.BranchName, want)
		}
	}
}

func TestSQLQueue_EnqueueValidation(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)

	tests := []struct {
		name  string
		entry *MergeEntry
		want  string
	}{
		{
			name:  "missing branch",
			entry: &MergeEntry{TaskID: "t1", AgentName: "a1"},
			want:  "branch name is required",
		},
		{
			name:  "missing task ID",
			entry: &MergeEntry{BranchName: "b1", AgentName: "a1"},
			want:  "task ID is required",
		},
		{
			name:  "missing agent name",
			entry: &MergeEntry{BranchName: "b1", TaskID: "t1"},
			want:  "agent name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := q.Enqueue(tt.entry)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}

// mockRunner records git commands for testing the executor.
type mockRunner struct {
	calls   []string
	results map[string]mockResult
}

type mockResult struct {
	output []byte
	err    error
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		results: make(map[string]mockResult),
	}
}

func (m *mockRunner) RunInDir(dir, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	m.calls = append(m.calls, key)

	if r, ok := m.results[key]; ok {
		return r.output, r.err
	}

	return nil, nil
}

func (m *mockRunner) setResult(cmd string, output []byte, err error) {
	m.results[cmd] = mockResult{output: output, err: err}
}

func TestMergeExecutor_Tier1Success(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)
	runner := newMockRunner()

	executor := NewMergeExecutorWithQueue(q, "/repo", runner, false, false)

	entry := &MergeEntry{
		BranchName: "feature/clean",
		TaskID:     "task-100",
		AgentName:  "builder-10",
		Status:     MergeMerging,
	}

	result, err := executor.Execute(entry, "main")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %v", result.Error)
	}
	if result.Tier != TierCleanMerge {
		t.Errorf("tier: got %q, want %q", result.Tier, TierCleanMerge)
	}

	// Verify git commands were called
	if len(runner.calls) < 2 {
		t.Fatalf("expected at least 2 git calls, got %d: %v", len(runner.calls), runner.calls)
	}
	if runner.calls[0] != "git checkout main" {
		t.Errorf("first call: got %q, want %q", runner.calls[0], "git checkout main")
	}
	if runner.calls[1] != "git merge --no-edit feature/clean" {
		t.Errorf("second call: got %q, want %q", runner.calls[1], "git merge --no-edit feature/clean")
	}
}

func TestMergeExecutor_Tier1FailTier2Success(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)
	runner := newMockRunner()

	// Tier 1 merge fails
	runner.setResult("git merge --no-edit feature/conflict", nil, fmt.Errorf("merge conflict"))

	executor := NewMergeExecutorWithQueue(q, "/repo", runner, false, false)

	entry := &MergeEntry{
		BranchName: "feature/conflict",
		TaskID:     "task-101",
		AgentName:  "builder-11",
		Status:     MergeMerging,
	}

	result, err := executor.Execute(entry, "main")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success via tier 2, got failure: %v", result.Error)
	}
	if result.Tier != TierAutoResolve {
		t.Errorf("tier: got %q, want %q", result.Tier, TierAutoResolve)
	}
}

func TestMergeExecutor_AllTiersFail(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)
	runner := newMockRunner()

	// Both tiers fail
	runner.setResult("git merge --no-edit feature/hard", nil, fmt.Errorf("merge conflict"))
	runner.setResult("git merge --no-edit -X theirs feature/hard", nil, fmt.Errorf("still conflict"))

	executor := NewMergeExecutorWithQueue(q, "/repo", runner, false, false)

	entry := &MergeEntry{
		BranchName: "feature/hard",
		TaskID:     "task-102",
		AgentName:  "builder-12",
		Status:     MergeMerging,
	}

	result, err := executor.Execute(entry, "main")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.Success {
		t.Error("expected failure, got success")
	}
	if result.Error == nil {
		t.Error("expected error, got nil")
	}
}

func TestMergeExecutor_NilEntry(t *testing.T) {
	executor := NewMergeExecutorWithQueue(nil, "/repo", newMockRunner(), false, false)

	_, err := executor.Execute(nil, "main")
	if err == nil {
		t.Fatal("expected error for nil entry")
	}
	if !strings.Contains(err.Error(), "entry is nil") {
		t.Errorf("error %q should contain 'entry is nil'", err.Error())
	}
}

func TestMergeExecutor_EmptyTarget(t *testing.T) {
	executor := NewMergeExecutorWithQueue(nil, "/repo", newMockRunner(), false, false)

	entry := &MergeEntry{BranchName: "feature/x", TaskID: "t", AgentName: "a"}
	_, err := executor.Execute(entry, "")
	if err == nil {
		t.Fatal("expected error for empty target")
	}
	if !strings.Contains(err.Error(), "target branch is required") {
		t.Errorf("error %q should contain 'target branch is required'", err.Error())
	}
}

func TestMergeExecutor_WithAIAndReimagine(t *testing.T) {
	d := setupTestDB(t)
	q := NewSQLQueue(d)
	runner := newMockRunner()

	// All real tiers fail
	runner.setResult("git merge --no-edit feature/complex", nil, fmt.Errorf("conflict"))
	runner.setResult("git merge --no-edit -X theirs feature/complex", nil, fmt.Errorf("conflict"))

	executor := NewMergeExecutorWithQueue(q, "/repo", runner, true, true)

	entry := &MergeEntry{
		BranchName: "feature/complex",
		TaskID:     "task-103",
		AgentName:  "builder-13",
		Status:     MergeMerging,
	}

	result, err := executor.Execute(entry, "main")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Should fail since AI and reimagine are stubs
	if result.Success {
		t.Error("expected failure with stub AI/reimagine, got success")
	}
}

func TestMergeStatus_Values(t *testing.T) {
	statuses := []MergeStatus{MergePending, MergeMerging, MergeMerged, MergeConflict, MergeFailed}
	expected := []string{"pending", "merging", "merged", "conflict", "failed"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("status %d: got %q, want %q", i, s, expected[i])
		}
	}
}

func TestResolutionTier_Values(t *testing.T) {
	tiers := []ResolutionTier{TierCleanMerge, TierAutoResolve, TierAIResolve, TierReimagine}
	expected := []string{"clean-merge", "auto-resolve", "ai-resolve", "reimagine"}

	for i, tier := range tiers {
		if string(tier) != expected[i] {
			t.Errorf("tier %d: got %q, want %q", i, tier, expected[i])
		}
	}
}
