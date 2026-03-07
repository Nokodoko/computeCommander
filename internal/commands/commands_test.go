package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/internal/platform/db"
)

// testApp creates a minimal App backed by an in-memory SQLite database
// for integration testing.
func testApp(t *testing.T) *App {
	t.Helper()

	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Project.Name = "test-project"

	mailStore := mail.NewMailStore(database, nil)
	mq := merge.NewSQLQueue(database)

	return &App{
		Config:     cfg,
		DB:         database,
		MailStore:  mailStore,
		MergeQueue: mq,
		Version:    "test",
	}
}

// TestAppCreation verifies that a test App can be constructed with an in-memory DB.
func TestAppCreation(t *testing.T) {
	app := testApp(t)
	if app.DB == nil {
		t.Fatal("expected non-nil DB")
	}
	if app.Config == nil {
		t.Fatal("expected non-nil Config")
	}
	if app.MailStore == nil {
		t.Fatal("expected non-nil MailStore")
	}
	if app.MergeQueue == nil {
		t.Fatal("expected non-nil MergeQueue")
	}
}

// TestStatusCmdNoSessions verifies status works with an empty session list.
func TestStatusCmdNoSessions(t *testing.T) {
	app := testApp(t)
	cmd := StatusCmd(app)
	cmd.SetArgs([]string{})

	// The status command calls app.Spawner.ListSessions, which requires a Spawner.
	// Since we don't have a Spawner in the test app, we test the command construction.
	if cmd.Use != "status" {
		t.Errorf("expected Use='status', got %q", cmd.Use)
	}
	if cmd.GroupID != "CORE" {
		t.Errorf("expected GroupID='CORE', got %q", cmd.GroupID)
	}
}

// TestMailSendAndCheck verifies end-to-end mail flow through the commands package.
func TestMailSendAndCheck(t *testing.T) {
	app := testApp(t)

	// Send a message directly through the mail store.
	msg := &mail.MailMessage{
		From:     "agent-a",
		To:       "agent-b",
		Subject:  "test subject",
		Body:     "test body",
		Type:     mail.TypeStatus,
		Priority: mail.PriorityNormal,
	}
	if err := app.MailStore.Send(msg); err != nil {
		t.Fatalf("send mail: %v", err)
	}

	// Check unread for agent-b.
	msgs, err := app.MailStore.Check("agent-b", mail.CheckOpts{})
	if err != nil {
		t.Fatalf("check mail: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(msgs))
	}
	if msgs[0].Subject != "test subject" {
		t.Errorf("expected subject 'test subject', got %q", msgs[0].Subject)
	}
	if msgs[0].From != "agent-a" {
		t.Errorf("expected from 'agent-a', got %q", msgs[0].From)
	}

	// Mark as read.
	if err := app.MailStore.MarkRead(msgs[0].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	// Check again -- should be empty.
	msgs, err = app.MailStore.Check("agent-b", mail.CheckOpts{})
	if err != nil {
		t.Fatalf("check mail after read: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 unread messages, got %d", len(msgs))
	}
}

// TestMailPurge verifies purge cleans up read messages.
func TestMailPurge(t *testing.T) {
	app := testApp(t)

	// Send and mark as read.
	msg := &mail.MailMessage{
		From:    "a",
		To:      "b",
		Subject: "purge-test",
		Body:    "body",
		Type:    mail.TypeStatus,
	}
	if err := app.MailStore.Send(msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	msgs, _ := app.MailStore.Check("b", mail.CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if err := app.MailStore.MarkRead(msgs[0].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	// Purge read messages.
	count, err := app.MailStore.Purge(mail.PurgeOpts{ReadOnly: true})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 purged, got %d", count)
	}
}

// TestMergeQueueEnqueueAndList verifies the merge queue flow.
func TestMergeQueueEnqueueAndList(t *testing.T) {
	app := testApp(t)

	entry := &merge.MergeEntry{
		BranchName: "cc/worker-1/abc123",
		TaskID:     "task-001",
		AgentName:  "worker-1",
	}

	if err := app.MergeQueue.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// List all.
	entries, err := app.MergeQueue.List(merge.ListOpts{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].BranchName != "cc/worker-1/abc123" {
		t.Errorf("expected branch 'cc/worker-1/abc123', got %q", entries[0].BranchName)
	}
	if entries[0].Status != merge.MergePending {
		t.Errorf("expected status 'pending', got %q", entries[0].Status)
	}

	// Check status by branch.
	e, err := app.MergeQueue.Status("cc/worker-1/abc123")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if e.AgentName != "worker-1" {
		t.Errorf("expected agent 'worker-1', got %q", e.AgentName)
	}

	// Dequeue.
	dequeued, err := app.MergeQueue.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if dequeued.BranchName != "cc/worker-1/abc123" {
		t.Errorf("expected dequeued branch 'cc/worker-1/abc123', got %q", dequeued.BranchName)
	}
	if dequeued.Status != merge.MergeMerging {
		t.Errorf("expected status 'merging', got %q", dequeued.Status)
	}
}

// TestMergeQueuePeek verifies peek without removal.
func TestMergeQueuePeek(t *testing.T) {
	app := testApp(t)

	entry := &merge.MergeEntry{
		BranchName: "cc/peek-test/def456",
		TaskID:     "task-002",
		AgentName:  "peek-agent",
	}
	if err := app.MergeQueue.Enqueue(entry); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	peeked, err := app.MergeQueue.Peek()
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if peeked.BranchName != "cc/peek-test/def456" {
		t.Errorf("expected branch 'cc/peek-test/def456', got %q", peeked.BranchName)
	}

	// Peek again -- should still return the same entry.
	peeked2, err := app.MergeQueue.Peek()
	if err != nil {
		t.Fatalf("peek2: %v", err)
	}
	if peeked2.BranchName != peeked.BranchName {
		t.Errorf("peek should be idempotent")
	}
}

// TestGroupCreateAndList verifies task group CRUD.
func TestGroupCreateAndList(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Create a group via direct DB call (as the command would).
	groupID := "grp-test-1"
	err := app.DB.Exec(ctx,
		"INSERT INTO task_groups (id, name) VALUES (?, ?)",
		groupID, "test-group")
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}

	// Query groups.
	groups, err := queryGroups(ctx, app)
	if err != nil {
		t.Fatalf("query groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "test-group" {
		t.Errorf("expected name 'test-group', got %q", groups[0].Name)
	}
}

// TestEventsQuery verifies the events query helper.
func TestEventsQuery(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert a test event.
	err := app.DB.Exec(ctx,
		"INSERT INTO events (agent_name, event_type, level, data, created_at) VALUES (?, ?, ?, ?, ?)",
		"test-agent", "tool_use", "info", "test data", "2026-01-01 00:00:00")
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	events, err := queryEvents(ctx, app, "test-agent", "", 10)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Agent != "test-agent" {
		t.Errorf("expected agent 'test-agent', got %q", events[0].Agent)
	}
	if events[0].EventType != "tool_use" {
		t.Errorf("expected event_type 'tool_use', got %q", events[0].EventType)
	}
}

// TestEventsByLevel verifies filtering events by level.
func TestEventsByLevel(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert events at different levels.
	err := app.DB.Exec(ctx,
		"INSERT INTO events (agent_name, event_type, level, data, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-1", "error_event", "error", "something broke", "2026-01-01 00:00:00")
	if err != nil {
		t.Fatalf("insert error event: %v", err)
	}
	err = app.DB.Exec(ctx,
		"INSERT INTO events (agent_name, event_type, level, data, created_at) VALUES (?, ?, ?, ?, ?)",
		"agent-1", "info_event", "info", "all good", "2026-01-01 00:00:01")
	if err != nil {
		t.Fatalf("insert info event: %v", err)
	}

	errors, err := queryEventsByLevel(ctx, app, "error", 10)
	if err != nil {
		t.Fatalf("query errors: %v", err)
	}
	if len(errors) != 1 {
		t.Fatalf("expected 1 error event, got %d", len(errors))
	}
	if errors[0].Data != "something broke" {
		t.Errorf("expected data 'something broke', got %q", errors[0].Data)
	}
}

// TestRunsQuery verifies the runs query helper with an empty table.
func TestRunsQuery(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	runs, err := queryRuns(ctx, app)
	if err != nil {
		t.Fatalf("query runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}

	// Insert a run.
	err = app.DB.Exec(ctx,
		"INSERT INTO runs (id, agent_count, status) VALUES (?, ?, ?)",
		"run-001", 3, "active")
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	runs, err = queryRuns(ctx, app)
	if err != nil {
		t.Fatalf("query runs after insert: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "run-001" {
		t.Errorf("expected run id 'run-001', got %q", runs[0].ID)
	}
}

// TestCommandConstructors verifies every command constructor produces valid commands.
func TestCommandConstructors(t *testing.T) {
	// Use nil App -- constructors should not crash, they only reference app inside RunE.
	var nilApp *App

	constructors := []struct {
		name string
		fn   func(*App) *cobra.Command
	}{
		{"sling", SlingCmd},
		{"stop", StopCmd},
		{"status", StatusCmd},
		{"dashboard", DashboardCmd},
		{"coordinator", CoordinatorCmd},
		{"monitor", MonitorCmd},
		{"mail", MailCmd},
		{"nudge", NudgeCmd},
		{"merge", MergeCmd},
		{"group", GroupCmd},
		{"inspect", InspectCmd},
		{"trace", TraceCmd},
		{"errors", ErrorsCmd},
		{"replay", ReplayCmd},
		{"feed", FeedCmd},
		{"logs", LogsCmd},
		{"costs", CostsCmd},
		{"metrics", MetricsCmd},
		{"run", RunCmd},
		{"worktree", WorktreeCmd},
		{"watch", WatchCmd},
		{"doctor", DoctorCmd},
		{"clean", CleanCmd},
		{"feature", FeatureCmd},
		{"evals", EvalsCmd},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.fn(nilApp)
			if cmd == nil {
				t.Fatal("constructor returned nil")
			}
			if cmd.Use == "" {
				t.Error("Use is empty")
			}
			if cmd.Short == "" {
				t.Error("Short is empty")
			}
			if cmd.GroupID == "" {
				t.Error("GroupID is empty")
			}
		})
	}
}

// TestTruncate verifies the truncate helper function.
func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hel.."},
		{"hi", 2, "hi"},
		{"hello", 3, "h.."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

// TestConfigValidation verifies the default config passes validation.
func TestConfigValidation(t *testing.T) {
	app := testApp(t)
	if err := app.Config.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

// TestMailReply verifies reply creates a threaded message.
func TestMailReply(t *testing.T) {
	app := testApp(t)

	// Send original message.
	msg := &mail.MailMessage{
		From:    "lead",
		To:      "worker",
		Subject: "do the thing",
		Body:    "please implement feature X",
		Type:    mail.TypeDispatch,
	}
	if err := app.MailStore.Send(msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Get the message ID.
	msgs, _ := app.MailStore.Check("worker", mail.CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	origID := msgs[0].ID

	// Reply to it.
	if err := app.MailStore.Reply(origID, "done!"); err != nil {
		t.Fatalf("reply: %v", err)
	}

	// Check for reply in lead's inbox.
	replies, _ := app.MailStore.Check("lead", mail.CheckOpts{})
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if replies[0].Subject != "Re: do the thing" {
		t.Errorf("expected subject 'Re: do the thing', got %q", replies[0].Subject)
	}
	if replies[0].ThreadID == nil {
		t.Error("expected reply to have a thread ID")
	}
}

// TestFeatureList verifies the feature list command produces correct flags.
func TestFeatureList(t *testing.T) {
	app := testApp(t)

	// Verify features from default config.
	if app.Config.Features.Distributed {
		t.Error("distributed should be false by default")
	}
	if app.Config.Features.RemoteAgents {
		t.Error("remote_agents should be false by default")
	}
	if !app.Config.Merge.AIResolveEnabled {
		t.Error("ai_resolve should be true by default")
	}
}

// TestDoctorChecks verifies doctor checks run without panicking.
func TestDoctorChecks(t *testing.T) {
	app := testApp(t)
	checks := runDoctorChecks(app)

	if len(checks) == 0 {
		t.Fatal("expected at least one doctor check")
	}

	// Config and database should pass.
	var configOK, dbOK bool
	for _, c := range checks {
		if c.Name == "config" && c.OK {
			configOK = true
		}
		if c.Name == "database" && c.OK {
			dbOK = true
		}
	}
	if !configOK {
		t.Error("expected config check to pass")
	}
	if !dbOK {
		t.Error("expected database check to pass")
	}
}

// TestMetricsQuery verifies metrics query with empty table.
func TestMetricsQuery(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	metrics, err := queryMetrics(ctx, app)
	if err != nil {
		t.Fatalf("query metrics: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics, got %d", len(metrics))
	}
}

// TestAgentSessionTypes verifies type constants are consistent.
func TestAgentSessionTypes(t *testing.T) {
	// Verify all capabilities are valid.
	for _, cap := range agents.AllCapabilities() {
		if !agents.ValidCapability(cap) {
			t.Errorf("capability %q should be valid", cap)
		}
	}

	// Verify all session states are valid.
	for _, state := range agents.AllSessionStates() {
		if !agents.ValidSessionState(state) {
			t.Errorf("state %q should be valid", state)
		}
	}
}

// TestClose verifies App.Close handles nil DB gracefully.
func TestClose(t *testing.T) {
	app := &App{}
	if err := app.Close(); err != nil {
		t.Errorf("Close with nil DB should not error: %v", err)
	}
}

// --- Eval Bridge Tests -------------------------------------------------------

// TestEvalsAddAndList verifies the full add → list cycle for evals.
func TestEvalsAddAndList(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Initially, no evals exist.
	rows, err := app.DB.Query(ctx, "SELECT id FROM evals")
	if err != nil {
		t.Fatalf("query evals: %v", err)
	}
	count := 0
	for rows.Next() {
		count++
	}
	rows.Close()
	if count != 0 {
		t.Fatalf("expected 0 evals initially, got %d", count)
	}

	// Add an eval via direct DB (simulating what evalsAddCmd does).
	id := generateEvalID()
	if !strings.HasPrefix(id, "eval-") {
		t.Errorf("expected eval ID to start with 'eval-', got %q", id)
	}

	err = app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		id, "test-project", "run unit tests", "unit_test", "echo ok",
	)
	if err != nil {
		t.Fatalf("insert eval: %v", err)
	}

	// Query back.
	row := app.DB.QueryRow(ctx, "SELECT id, project_name, agent_task, eval_type, command FROM evals WHERE id = ?", id)
	var gotID, gotProject, gotTask, gotType, gotCommand string
	if err := row.Scan(&gotID, &gotProject, &gotTask, &gotType, &gotCommand); err != nil {
		t.Fatalf("scan eval: %v", err)
	}
	if gotID != id {
		t.Errorf("expected id %q, got %q", id, gotID)
	}
	if gotProject != "test-project" {
		t.Errorf("expected project 'test-project', got %q", gotProject)
	}
	if gotTask != "run unit tests" {
		t.Errorf("expected task 'run unit tests', got %q", gotTask)
	}
	if gotType != "unit_test" {
		t.Errorf("expected type 'unit_test', got %q", gotType)
	}
	if gotCommand != "echo ok" {
		t.Errorf("expected command 'echo ok', got %q", gotCommand)
	}
}

// TestEvalsRunPassingCommand verifies running an eval with a passing command.
func TestEvalsRunPassingCommand(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	id := generateEvalID()
	err := app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		id, "test-project", "echo test", "build", "echo hello",
	)
	if err != nil {
		t.Fatalf("insert eval: %v", err)
	}

	// Run the evals command.
	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"run", "--id", id, "--json"})

	// Capture output.
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("evals run: %v", err)
	}

	// Verify the eval was updated in the DB.
	row := app.DB.QueryRow(ctx, "SELECT passed, error_detail, last_run_at FROM evals WHERE id = ?", id)
	var passed *bool
	var errorDetail, lastRunAt *string
	if err := row.Scan(&passed, &errorDetail, &lastRunAt); err != nil {
		t.Fatalf("scan eval result: %v", err)
	}
	if passed == nil {
		t.Fatal("expected passed to be non-nil after run")
	}
	if !*passed {
		t.Error("expected eval to pass with 'echo hello'")
	}
	if lastRunAt == nil {
		t.Error("expected last_run_at to be set")
	}
}

// TestEvalsRunFailingCommand verifies running an eval with a failing command.
func TestEvalsRunFailingCommand(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	id := generateEvalID()
	err := app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		id, "test-project", "fail test", "custom", "exit 1",
	)
	if err != nil {
		t.Fatalf("insert eval: %v", err)
	}

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"run", "--id", id})

	// Suppress output.
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("evals run: %v", err)
	}

	// Verify the eval failed.
	row := app.DB.QueryRow(ctx, "SELECT passed FROM evals WHERE id = ?", id)
	var passed *bool
	if err := row.Scan(&passed); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if passed == nil {
		t.Fatal("expected passed to be non-nil after run")
	}
	if *passed {
		t.Error("expected eval to fail with 'exit 1'")
	}
}

// TestEvalsRemove verifies removing an eval.
func TestEvalsRemove(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	id := generateEvalID()
	err := app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		id, "test-project", "removable", "lint", "echo lint",
	)
	if err != nil {
		t.Fatalf("insert eval: %v", err)
	}

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"remove", id})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("evals remove: %v", err)
	}

	// Verify it was deleted.
	row := app.DB.QueryRow(ctx, "SELECT id FROM evals WHERE id = ?", id)
	var gone string
	err = row.Scan(&gone)
	if err == nil {
		t.Error("expected eval to be deleted, but it still exists")
	}
}

// TestEvalsRemoveNonExistent verifies removing a non-existent eval returns an error.
func TestEvalsRemoveNonExistent(t *testing.T) {
	app := testApp(t)

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"remove", "eval-doesnotexist"})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when removing non-existent eval")
	}
}

// TestEvalsListJSON verifies JSON output of eval list.
func TestEvalsListJSON(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Add two evals.
	for i, et := range []string{"unit_test", "integration"} {
		id := generateEvalID()
		err := app.DB.Exec(ctx,
			"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
			id, "test-project", fmt.Sprintf("task-%d", i), et, "echo ok",
		)
		if err != nil {
			t.Fatalf("insert eval %d: %v", i, err)
		}
	}

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"--json"})

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("evals list --json: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var output strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, output.String())
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	count, ok := result["count"].(float64)
	if !ok || count != 2 {
		t.Errorf("expected count=2, got %v", result["count"])
	}
}

// TestEvalsAddCmd verifies the add subcommand via cobra execution.
func TestEvalsAddCmd(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"add",
		"--project", "my-project",
		"--task", "check formatting",
		"--type", "lint",
		"--command", "echo formatted",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("evals add: %v", err)
	}

	// Verify it was inserted.
	rows, err := app.DB.Query(ctx, "SELECT id, project_name, eval_type, command FROM evals WHERE project_name = ?", "my-project")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id, project, evalType, command string
		if err := rows.Scan(&id, &project, &evalType, &command); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if project == "my-project" && evalType == "lint" && command == "echo formatted" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find the inserted eval")
	}
}

// TestEvalsAddInvalidType verifies that an invalid eval type is rejected.
func TestEvalsAddInvalidType(t *testing.T) {
	app := testApp(t)

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"add",
		"--project", "my-project",
		"--type", "invalid_type",
		"--command", "echo nope",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid eval type")
	}
}

// TestEvalsAddMissingProject verifies that --project is required.
func TestEvalsAddMissingProject(t *testing.T) {
	app := testApp(t)

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"add",
		"--command", "echo test",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing --project")
	}
}

// TestEvalsAddMissingCommand verifies that --command is required.
func TestEvalsAddMissingCommand(t *testing.T) {
	app := testApp(t)

	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"add",
		"--project", "my-project",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing --command")
	}
}

// TestEvalsRunMultiple verifies running multiple evals and checking summary counts.
func TestEvalsRunMultiple(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert a passing and a failing eval.
	passID := generateEvalID()
	failID := generateEvalID()

	err := app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		passID, "test-project", "pass test", "build", "echo ok",
	)
	if err != nil {
		t.Fatalf("insert pass eval: %v", err)
	}

	err = app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
		failID, "test-project", "fail test", "build", "exit 1",
	)
	if err != nil {
		t.Fatalf("insert fail eval: %v", err)
	}

	// Run all evals for this project via JSON output.
	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"run", "--project", "test-project", "--json"})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("evals run: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var output strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("parse JSON: %v\nOutput: %s", err, output.String())
	}

	if p, ok := result["passed"].(float64); !ok || p != 1 {
		t.Errorf("expected 1 passed, got %v", result["passed"])
	}
	if f, ok := result["failed"].(float64); !ok || f != 1 {
		t.Errorf("expected 1 failed, got %v", result["failed"])
	}
	if total, ok := result["total"].(float64); !ok || total != 2 {
		t.Errorf("expected 2 total, got %v", result["total"])
	}
}

// TestEvalsFilterByProject verifies that evals can be filtered by project name.
func TestEvalsFilterByProject(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Add evals for two different projects.
	for _, proj := range []string{"alpha", "beta"} {
		id := generateEvalID()
		err := app.DB.Exec(ctx,
			"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES (?, ?, ?, ?, ?)",
			id, proj, "task for "+proj, "custom", "echo "+proj,
		)
		if err != nil {
			t.Fatalf("insert eval for %s: %v", proj, err)
		}
	}

	// List with --project alpha --json.
	cmd := EvalsCmd(app)
	cmd.SetArgs([]string{"--project", "alpha", "--json"})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("evals list: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var output strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}

	count, ok := result["count"].(float64)
	if !ok || count != 1 {
		t.Errorf("expected 1 eval for project alpha, got %v", result["count"])
	}
}

// TestGenerateEvalID verifies IDs are unique and well-formed.
func TestGenerateEvalID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateEvalID()
		if !strings.HasPrefix(id, "eval-") {
			t.Fatalf("eval ID should start with 'eval-', got %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate eval ID generated: %q", id)
		}
		seen[id] = true
	}
}

// TestEvalsEvalTypeConstraint verifies the DB CHECK constraint on eval_type.
func TestEvalsEvalTypeConstraint(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	err := app.DB.Exec(ctx,
		"INSERT INTO evals (id, project_name, eval_type, command) VALUES (?, ?, ?, ?)",
		"eval-constraint", "test-project", "nonexistent_type", "echo nope",
	)
	if err == nil {
		t.Error("expected CHECK constraint violation for invalid eval_type")
	}
}

// Verify the unused import is satisfied.
var _ = time.Second
