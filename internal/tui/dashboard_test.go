package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/pkg/runtimes"
)

// --- Mocks ------------------------------------------------------------------

type mockSessionLister struct {
	sessions []*agents.AgentSession
	err      error
}

func (m *mockSessionLister) ListSessions(_ context.Context, _ agents.ListOpts) ([]*agents.AgentSession, error) {
	return m.sessions, m.err
}

type mockMailStore struct {
	messages []*mail.MailMessage
	err      error
}

func (m *mockMailStore) Send(_ *mail.MailMessage) error                   { return nil }
func (m *mockMailStore) Check(_ string, _ mail.CheckOpts) ([]*mail.MailMessage, error) {
	return m.messages, m.err
}
func (m *mockMailStore) List(_ mail.ListOpts) ([]*mail.MailMessage, error) {
	return m.messages, m.err
}
func (m *mockMailStore) MarkRead(_ string) error   { return nil }
func (m *mockMailStore) Reply(_ string, _ string) error { return nil }
func (m *mockMailStore) Purge(_ mail.PurgeOpts) (int, error) { return 0, nil }

type mockMergeQueue struct {
	entries []*merge.MergeEntry
	err     error
}

func (m *mockMergeQueue) Enqueue(_ *merge.MergeEntry) error          { return nil }
func (m *mockMergeQueue) Dequeue() (*merge.MergeEntry, error)        { return nil, nil }
func (m *mockMergeQueue) Peek() (*merge.MergeEntry, error)           { return nil, nil }
func (m *mockMergeQueue) Status(_ string) (*merge.MergeEntry, error) { return nil, nil }
func (m *mockMergeQueue) List(_ merge.ListOpts) ([]*merge.MergeEntry, error) {
	return m.entries, m.err
}

// --- Tests ------------------------------------------------------------------

func TestNewDashboard(t *testing.T) {
	d := NewDashboard(DashboardOpts{
		Lister:   &mockSessionLister{},
		Mail:     &mockMailStore{},
		Queue:    &mockMergeQueue{},
		Interval: time.Second,
	})
	if d == nil {
		t.Fatal("NewDashboard returned nil")
	}
	if d.interval != time.Second {
		t.Errorf("expected interval 1s, got %s", d.interval)
	}
	if d.focusedPane != PaneAgentSession {
		t.Errorf("expected initial focusedPane PaneAgentSession, got %d", d.focusedPane)
	}
}

func TestNewDashboardDefaultInterval(t *testing.T) {
	d := NewDashboard(DashboardOpts{
		Lister: &mockSessionLister{},
		Mail:   &mockMailStore{},
		Queue:  &mockMergeQueue{},
	})
	if d.interval != 2*time.Second {
		t.Errorf("expected default interval 2s, got %s", d.interval)
	}
}

func TestNewDashboardAgentCmdPrecedence(t *testing.T) {
	// CLI flag overrides config.
	d := NewDashboard(DashboardOpts{
		Lister:   &mockSessionLister{},
		Mail:     &mockMailStore{},
		Queue:    &mockMergeQueue{},
		AgentCmd: "custom-agent --flag",
	})
	if d.agentCmd != "custom-agent --flag" {
		t.Errorf("expected CLI override, got %q", d.agentCmd)
	}

	// Default when nothing set.
	d2 := NewDashboard(DashboardOpts{
		Lister: &mockSessionLister{},
		Mail:   &mockMailStore{},
		Queue:  &mockMergeQueue{},
	})
	if d2.agentCmd != DefaultAgentCommand {
		t.Errorf("expected default agent command, got %q", d2.agentCmd)
	}
}

func TestDashboardRefresh(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "builder-1", Capability: agents.CapBuilder, State: agents.StateWorking, Runtime: runtimes.RuntimeClaude},
		{AgentName: "scout-1", Capability: agents.CapScout, State: agents.StateBooting, Runtime: runtimes.RuntimeGemini},
	}
	msgs := []*mail.MailMessage{
		{From: "scout-1", To: "coord", Subject: "done", Read: false, Type: mail.TypeStatus},
	}
	entries := []*merge.MergeEntry{
		{BranchName: "cc/builder-1/abc", AgentName: "builder-1", Status: merge.MergePending},
	}

	d := NewDashboard(DashboardOpts{
		Lister:   &mockSessionLister{sessions: sessions},
		Mail:     &mockMailStore{messages: msgs},
		Queue:    &mockMergeQueue{entries: entries},
		Interval: time.Second,
	})

	if err := d.Refresh(); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if len(d.agents.Sessions()) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(d.agents.Sessions()))
	}
	if d.mail.UnreadCount() != 1 {
		t.Errorf("expected 1 unread, got %d", d.mail.UnreadCount())
	}
	if d.queue.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", d.queue.PendingCount())
	}
}

func TestDashboardView(t *testing.T) {
	d := NewDashboard(DashboardOpts{
		Lister:   &mockSessionLister{},
		Mail:     &mockMailStore{},
		Queue:    &mockMergeQueue{},
		Interval: time.Second,
	})
	d.width = 120
	d.height = 40

	view := d.View()
	if !strings.Contains(view, "File Picker") {
		t.Error("view should contain File Picker pane title")
	}
	if !strings.Contains(view, "Agent Session") {
		t.Error("view should contain Agent Session pane title")
	}
	if !strings.Contains(view, "Agents") {
		t.Error("view should contain Agents pane title")
	}
	if !strings.Contains(view, "Tab: cycle") {
		t.Error("view should contain updated help bar")
	}
	if !strings.Contains(view, "Ctrl+K: palette") {
		t.Error("view should contain palette hint")
	}
}

func TestPaneNavigation(t *testing.T) {
	// Test Tab cycling.
	current := PaneFilePicker
	current = nextPane(current)
	if current != PaneAgentSession {
		t.Errorf("expected PaneAgentSession after Tab from FilePicker, got %d", current)
	}
	current = nextPane(current)
	if current != PaneAgents {
		t.Errorf("expected PaneAgents after Tab from AgentSession, got %d", current)
	}

	// Test Shift+Tab.
	current = prevPane(current)
	if current != PaneAgentSession {
		t.Errorf("expected PaneAgentSession after Shift+Tab from Agents, got %d", current)
	}

	// Test wrap-around.
	current = PaneGitStatus
	current = nextPane(current)
	if current != PaneFilePicker {
		t.Errorf("expected wrap to FilePicker, got %d", current)
	}

	current = PaneFilePicker
	current = prevPane(current)
	if current != PaneGitStatus {
		t.Errorf("expected wrap to GitStatus, got %d", current)
	}
}

func TestPaneMetaByID(t *testing.T) {
	meta := paneMetaByID(PaneAgentSession)
	if meta.Title != "Agent Session" {
		t.Errorf("expected 'Agent Session', got %q", meta.Title)
	}
	if meta.FocusKey != "2" {
		t.Errorf("expected focus key '2', got %q", meta.FocusKey)
	}
}

func TestRenderPane(t *testing.T) {
	theme := DefaultTheme()
	meta := PaneMeta{ID: PaneEvents, Title: "Events", FocusKey: "4"}

	// Focused.
	focused := RenderPane("hello\nworld", meta, true, 30, 10, theme)
	if !strings.Contains(focused, "Events") {
		t.Error("focused pane should contain title")
	}

	// Unfocused.
	unfocused := RenderPane("hello\nworld", meta, false, 30, 10, theme)
	if !strings.Contains(unfocused, "Events") {
		t.Error("unfocused pane should contain title")
	}
}

func TestAgentTableCursorNavigation(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "a1", State: agents.StateWorking, Runtime: runtimes.RuntimeClaude},
		{AgentName: "a2", State: agents.StateStalled, Runtime: runtimes.RuntimeClaude},
		{AgentName: "a3", State: agents.StateZombie, Runtime: runtimes.RuntimeClaude},
	}
	theme := DefaultTheme()
	tbl := NewAgentTable(&mockSessionLister{sessions: sessions}, theme)
	if err := tbl.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if tbl.Cursor() != 0 {
		t.Errorf("initial cursor should be 0, got %d", tbl.Cursor())
	}

	tbl.CursorDown()
	if tbl.Cursor() != 1 {
		t.Errorf("cursor after down should be 1, got %d", tbl.Cursor())
	}

	tbl.CursorDown()
	tbl.CursorDown() // Should not go past end.
	if tbl.Cursor() != 2 {
		t.Errorf("cursor should clamp at 2, got %d", tbl.Cursor())
	}

	tbl.CursorUp()
	if tbl.Cursor() != 1 {
		t.Errorf("cursor after up should be 1, got %d", tbl.Cursor())
	}

	tbl.CursorUp()
	tbl.CursorUp() // Should not go below 0.
	if tbl.Cursor() != 0 {
		t.Errorf("cursor should clamp at 0, got %d", tbl.Cursor())
	}
}

func TestAgentTableSelected(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "a1", Runtime: runtimes.RuntimeClaude},
		{AgentName: "a2", Runtime: runtimes.RuntimeGemini},
	}
	theme := DefaultTheme()
	tbl := NewAgentTable(&mockSessionLister{sessions: sessions}, theme)
	_ = tbl.Refresh(context.Background())

	sel := tbl.Selected()
	if sel == nil || sel.AgentName != "a1" {
		t.Error("selected should be first agent")
	}

	tbl.CursorDown()
	sel = tbl.Selected()
	if sel == nil || sel.AgentName != "a2" {
		t.Error("selected should be second agent after cursor down")
	}
}

func TestAgentTableView(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "builder-1", Capability: agents.CapBuilder, State: agents.StateWorking, TaskID: "task-1", Runtime: runtimes.RuntimeClaude, StartedAt: time.Now().Add(-3 * time.Minute)},
	}
	theme := DefaultTheme()
	tbl := NewAgentTable(&mockSessionLister{sessions: sessions}, theme)
	_ = tbl.Refresh(context.Background())

	view := tbl.View()
	if !strings.Contains(view, "Agents (1 active)") {
		t.Error("view should show agent count")
	}
	if !strings.Contains(view, "builder") {
		t.Logf("view output:\n%s", view)
		t.Error("view should contain agent name")
	}
	if !strings.Contains(view, "Duration") {
		t.Logf("view output:\n%s", view)
		t.Error("view should contain Duration column header")
	}
	if !strings.Contains(view, "3m") {
		t.Logf("view output:\n%s", view)
		t.Error("view should contain duration value like '3m'")
	}
}

func TestAgentTableCompactView(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "builder-1", State: agents.StateWorking, Runtime: runtimes.RuntimeClaude, InputTokens: 1000, OutputTokens: 500, StartedAt: time.Now().Add(-45 * time.Second)},
	}
	theme := DefaultTheme()
	tbl := NewAgentTable(&mockSessionLister{sessions: sessions}, theme)
	_ = tbl.Refresh(context.Background())

	view := tbl.CompactView(50, 10)
	if !strings.Contains(view, "builder-1") {
		t.Error("compact view should contain agent name")
	}
	if !strings.Contains(view, "1.5k") {
		t.Error("compact view should contain token count")
	}
	if !strings.Contains(view, "45s") {
		t.Logf("compact view output:\n%s", view)
		t.Error("compact view should contain duration value like '45s'")
	}
}

func TestAgentTableStateColors(t *testing.T) {
	sessions := []*agents.AgentSession{
		{AgentName: "w", State: agents.StateWorking, Runtime: runtimes.RuntimeClaude},
		{AgentName: "s", State: agents.StateStalled, Runtime: runtimes.RuntimeClaude},
		{AgentName: "z", State: agents.StateZombie, Runtime: runtimes.RuntimeClaude},
	}
	theme := DefaultTheme()
	tbl := NewAgentTable(&mockSessionLister{sessions: sessions}, theme)
	_ = tbl.Refresh(context.Background())

	view := tbl.View()
	if !strings.Contains(view, "working") {
		t.Error("view should contain 'working' state")
	}
	if !strings.Contains(view, "stalled") {
		t.Error("view should contain 'stalled' state")
	}
	if !strings.Contains(view, "zombie") {
		t.Error("view should contain 'zombie' state")
	}
}

func TestMailSummaryView(t *testing.T) {
	msgs := []*mail.MailMessage{
		{From: "scout-1", To: "coord", Subject: "found files", Type: mail.TypeResult, Read: false},
		{From: "builder-1", To: "coord", Subject: "task done", Type: mail.TypeWorkerDone, Read: true},
	}
	theme := DefaultTheme()
	ms := NewMailSummary(&mockMailStore{messages: msgs}, theme)
	if err := ms.Refresh(); err != nil {
		t.Fatal(err)
	}

	if ms.UnreadCount() != 2 {
		t.Errorf("expected 2 from mock, got %d", ms.UnreadCount())
	}

	view := ms.View()
	if !strings.Contains(view, "Mail") {
		t.Error("view should contain Mail header")
	}
}

func TestMergeQueueView(t *testing.T) {
	entries := []*merge.MergeEntry{
		{BranchName: "cc/builder-1/abc", AgentName: "builder-1", Status: merge.MergePending},
		{BranchName: "cc/builder-2/def", AgentName: "builder-2", Status: merge.MergeMerging},
	}
	theme := DefaultTheme()
	v := NewMergeQueueView(&mockMergeQueue{entries: entries}, theme)
	if err := v.Refresh(); err != nil {
		t.Fatal(err)
	}

	if v.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", v.PendingCount())
	}
	if len(v.Entries()) != 2 {
		t.Errorf("expected 2 entries, got %d", len(v.Entries()))
	}

	view := v.View()
	if !strings.Contains(view, "Merge Queue") {
		t.Error("view should contain Merge Queue header")
	}
	if !strings.Contains(view, "pending") {
		t.Error("view should contain pending status")
	}
}

func TestCostTrackerView(t *testing.T) {
	theme := DefaultTheme()
	ct := NewCostTracker(theme)

	ct.Update([]CostEntry{
		{Capability: "builder", Model: "claude-sonnet-4", InputTokens: 45000, OutputTokens: 12000, Cost: 2.50},
		{Capability: "scout", Model: "gemini-2.5-pro", InputTokens: 12000, OutputTokens: 3000, Cost: 0.75},
	})

	if ct.TotalCost() != 3.25 {
		t.Errorf("expected total cost 3.25, got %.2f", ct.TotalCost())
	}

	totalIn, totalOut := ct.TotalTokens()
	if totalIn != 57000 {
		t.Errorf("expected 57000 input tokens, got %d", totalIn)
	}
	if totalOut != 15000 {
		t.Errorf("expected 15000 output tokens, got %d", totalOut)
	}

	view := ct.View()
	if !strings.Contains(view, "Costs") {
		t.Error("view should contain Costs header")
	}
	if !strings.Contains(view, "$3.25") {
		t.Error("view should contain total cost")
	}
}

func TestCostTrackerEmpty(t *testing.T) {
	theme := DefaultTheme()
	ct := NewCostTracker(theme)

	view := ct.View()
	if !strings.Contains(view, "No cost data") {
		t.Error("empty cost tracker should show 'No cost data'")
	}
}

func TestRenderHelpers(t *testing.T) {
	t.Run("truncate", func(t *testing.T) {
		if got := truncate("hello", 10); got != "hello" {
			t.Errorf("truncate short: got %q", got)
		}
		if got := truncate("hello world", 7); got != "hello.." {
			t.Errorf("truncate long: got %q", got)
		}
		if got := truncate("hi", 2); got != "hi" {
			t.Errorf("truncate exact: got %q", got)
		}
	})

	t.Run("formatTokens", func(t *testing.T) {
		if got := formatTokens(500); got != "500" {
			t.Errorf("formatTokens <1k: got %q", got)
		}
		if got := formatTokens(12500); got != "12.5k" {
			t.Errorf("formatTokens >1k: got %q", got)
		}
	})

	t.Run("renderStatusBar", func(t *testing.T) {
		theme := DefaultTheme()
		bar := renderStatusBar(5, 3, 2, 4.23, theme)
		if !strings.Contains(bar, "Agents: 5") {
			t.Error("status bar should show agent count")
		}
		if !strings.Contains(bar, "Mail: 3 unread") {
			t.Error("status bar should show unread mail")
		}
	})

	t.Run("renderDashHelpBar", func(t *testing.T) {
		theme := DefaultTheme()
		bar := renderHelpBar(theme)
		if !strings.Contains(bar, "Tab: cycle") {
			t.Error("help bar should contain Tab hint")
		}
		if !strings.Contains(bar, "Ctrl+K: palette") {
			t.Error("help bar should contain palette hint")
		}
	})
}

func TestRuntimeDuration(t *testing.T) {
	now := time.Now()
	tests := []struct {
		started  time.Time
		contains string
	}{
		{now.Add(-30 * time.Second), "30s"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-2 * time.Hour), "2h"},
	}

	for _, tt := range tests {
		got := RuntimeDuration(tt.started)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("RuntimeDuration(%v) = %q, want to contain %q", tt.started, got, tt.contains)
		}
	}
}

func TestThemeNotNil(t *testing.T) {
	theme := DefaultTheme()
	if theme == nil {
		t.Fatal("DefaultTheme() returned nil")
	}
	// Verify new fields exist.
	_ = theme.FocusedBorder
	_ = theme.UnfocusedBorder
	_ = theme.PaneTitle
	_ = theme.PaneTitleFocused
	_ = theme.EventLog
	_ = theme.EventState
	_ = theme.EventError
	_ = theme.EventMail
	_ = theme.GitStaged
	_ = theme.GitUnstaged
	_ = theme.GitUntracked
	_ = theme.GitBranch
	_ = theme.FileDir
	_ = theme.FileRegular
	_ = theme.FileCursor
}

func TestEventsPane(t *testing.T) {
	theme := DefaultTheme()
	ep := NewEventsPane(theme)

	if view := ep.View(); !strings.Contains(view, "No events") {
		t.Error("empty events pane should show 'No events'")
	}

	ep.AddEvent(Event{
		Time:    time.Now(),
		Source:  "builder-1",
		Type:    "state",
		Message: "working",
	})

	view := ep.View()
	if !strings.Contains(view, "builder-1") {
		t.Error("events pane should show event source")
	}
	if !strings.Contains(view, "working") {
		t.Error("events pane should show event message")
	}
}

func TestGitStatusPane(t *testing.T) {
	theme := DefaultTheme()
	gs := NewGitStatusPane(theme)

	// Refresh should not fail even outside git repo.
	_ = gs.Refresh()

	view := gs.View()
	if !strings.Contains(view, "Branch:") {
		t.Error("git status pane should show Branch label")
	}
}

func TestCommandPalette(t *testing.T) {
	theme := DefaultTheme()
	cp := NewCommandPalette(theme)

	if cp.Visible() {
		t.Error("palette should start hidden")
	}

	cp.Open()
	if !cp.Visible() {
		t.Error("palette should be visible after Open")
	}

	cp.TypeChar('s')
	sel := cp.Selected()
	if sel == nil {
		t.Error("should have a selected command after typing")
	}

	cp.Close()
	if cp.Visible() {
		t.Error("palette should be hidden after Close")
	}
}

func TestShellSplit(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"echo hello", []string{"echo", "hello"}},
		{"echo 'hello world'", []string{"echo", "hello world"}},
		{`echo "hello world"`, []string{"echo", "hello world"}},
		{"claude --flag1 --flag2", []string{"claude", "--flag1", "--flag2"}},
	}

	for _, tt := range tests {
		got := shellSplit(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("shellSplit(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("shellSplit(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}
