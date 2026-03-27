package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/noko/computecommander/pkg/integrations/jira"
)

// --- Mock ---

type mockJiraLister struct {
	issues []jira.JiraIssue
	err    error
}

func (m *mockJiraLister) GetCachedIssues(_ context.Context, _, _ string) ([]jira.JiraIssue, error) {
	return m.issues, m.err
}

// --- Tests ---

func TestNewJiraPane(t *testing.T) {
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{}, theme)
	if p == nil {
		t.Fatal("NewJiraPane returned nil")
	}
	if p.Cursor() != 0 {
		t.Errorf("initial cursor should be 0, got %d", p.Cursor())
	}
}

func TestJiraPaneRefreshNilLister(t *testing.T) {
	theme := DefaultTheme()
	p := NewJiraPane(nil, theme)
	if err := p.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh with nil lister should not error: %v", err)
	}
}

func TestJiraPaneRefreshError(t *testing.T) {
	theme := DefaultTheme()
	lister := &mockJiraLister{err: fmt.Errorf("api down")}
	p := NewJiraPane(lister, theme)

	err := p.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error from Refresh")
	}
	if !strings.Contains(err.Error(), "api down") {
		t.Errorf("expected 'api down' in error, got %q", err.Error())
	}

	view := p.View()
	if !strings.Contains(view, "Error") {
		t.Error("view should show error when refresh fails")
	}
}

func TestJiraPaneEmptyView(t *testing.T) {
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{}, theme)
	_ = p.Refresh(context.Background())

	view := p.View()
	if !strings.Contains(view, "No Jira issues") {
		t.Error("empty pane should show 'No Jira issues'")
	}
}

func TestJiraPaneHierarchy(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "PROJ-1", ProjectID: "proj1", Summary: "First task", Status: "To Do"},
		{Key: "PROJ-2", ProjectID: "proj1", EpicID: "epic1", Summary: "Second task", Status: "In Progress"},
		{Key: "PROJ-3", ProjectID: "proj1", EpicID: "epic1", Summary: "Third task", Status: "Done"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	// Should have 1 root (proj1).
	if len(p.roots) != 1 {
		t.Fatalf("expected 1 project root, got %d", len(p.roots))
	}

	// Root auto-expanded: project + PROJ-1 (no epic) + epic1 node = 3.
	if len(p.flat) != 3 {
		t.Errorf("expected 3 flat rows (auto-expanded project), got %d", len(p.flat))
	}

	// Navigate to epic and expand it.
	p.CursorDown() // to PROJ-1 (direct child)
	p.CursorDown() // to epic1
	p.Expand()

	// project + PROJ-1 + epic + PROJ-2 + PROJ-3 = 5
	if len(p.flat) != 5 {
		t.Errorf("expected 5 flat rows after expanding epic, got %d", len(p.flat))
	}
}

func TestJiraPaneCursorNavigation(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "A-1", ProjectID: "p1", Summary: "one", Status: "To Do"},
		{Key: "A-2", ProjectID: "p1", Summary: "two", Status: "To Do"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	// Expand to see children.
	p.Expand()

	if p.Cursor() != 0 {
		t.Errorf("cursor should start at 0, got %d", p.Cursor())
	}

	p.CursorDown()
	if p.Cursor() != 1 {
		t.Errorf("cursor should be 1 after down, got %d", p.Cursor())
	}

	p.CursorDown()
	p.CursorDown() // clamp
	if p.Cursor() != 2 {
		t.Errorf("cursor should clamp at 2, got %d", p.Cursor())
	}

	p.CursorUp()
	if p.Cursor() != 1 {
		t.Errorf("cursor should be 1 after up, got %d", p.Cursor())
	}

	p.CursorUp()
	p.CursorUp() // clamp at 0
	if p.Cursor() != 0 {
		t.Errorf("cursor should clamp at 0, got %d", p.Cursor())
	}
}

func TestJiraPaneExpandCollapse(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "B-1", ProjectID: "p1", Summary: "task", Status: "Open"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	// Initially auto-expanded: project + 1 issue = 2.
	if len(p.flat) != 2 {
		t.Fatalf("expected 2 flat rows (auto-expanded), got %d", len(p.flat))
	}

	// Collapse.
	p.Collapse()
	if len(p.flat) != 1 {
		t.Errorf("expected 1 row after collapse, got %d", len(p.flat))
	}

	// Re-expand.
	p.Expand()
	if len(p.flat) != 2 {
		t.Errorf("expected 2 rows after re-expand, got %d", len(p.flat))
	}
}

func TestJiraPaneSelectedKey(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "C-1", ProjectID: "p1", Summary: "task", Status: "Open"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	key := p.SelectedKey()
	if key != "C" {
		t.Errorf("expected selected key 'C', got %q", key)
	}

	p.Expand()
	p.CursorDown()
	key = p.SelectedKey()
	if key != "C-1" {
		t.Errorf("expected selected key 'C-1', got %q", key)
	}
}

func TestJiraPaneView(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "D-1", ProjectID: "p1", Summary: "Build feature", Status: "In Progress", AgentType: "builder", AgentState: "working"},
		{Key: "D-2", ProjectID: "p1", Summary: "Write tests", Status: "To Do"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())
	p.SetSize(80, 20)

	// Expand to see issues.
	p.Expand()

	view := p.View()
	if !strings.Contains(view, "D-1") {
		t.Error("view should contain issue key D-1")
	}
	if !strings.Contains(view, "Build feature") {
		t.Error("view should contain issue summary")
	}
	if !strings.Contains(view, "working") {
		t.Error("view should show agent state")
	}
	if !strings.Contains(view, "j/k:nav") {
		t.Error("view should contain key hints footer")
	}
}

func TestJiraPaneHelp(t *testing.T) {
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{}, theme)

	p.ToggleHelp()
	view := p.View()
	if !strings.Contains(view, "Jira Pane Help") {
		t.Error("help overlay should contain title")
	}
	if !strings.Contains(view, "j/k") {
		t.Error("help should list keybinds")
	}

	p.ToggleHelp()
	view = p.View()
	if strings.Contains(view, "Jira Pane Help") {
		t.Error("help should be hidden after second toggle")
	}
}

func TestJiraPaneSetSize(t *testing.T) {
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{}, theme)
	p.SetSize(100, 30)
	if p.width != 100 {
		t.Errorf("expected width 100, got %d", p.width)
	}
	if p.height != 30 {
		t.Errorf("expected height 30, got %d", p.height)
	}
}

func TestJiraPaneIssueCount(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "E-1", ProjectID: "p1", Summary: "one", Status: "Open"},
		{Key: "E-2", ProjectID: "p1", Summary: "two", Status: "Open"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	// Auto-expanded: project + 2 issues = 3 visible rows.
	if p.IssueCount() != 3 {
		t.Errorf("expected 3 visible rows (auto-expanded), got %d", p.IssueCount())
	}
}

func TestJiraPaneStatusIcons(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "F-1", ProjectID: "p1", Summary: "in prog", Status: "In Progress"},
		{Key: "F-2", ProjectID: "p1", Summary: "done", Status: "Done"},
		{Key: "F-3", ProjectID: "p1", Summary: "blocked", Status: "Blocked"},
		{Key: "F-4", ProjectID: "p1", Summary: "todo", Status: "To Do"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())
	p.SetSize(80, 20)
	p.Expand()

	view := p.View()
	// The view should contain styled status icons. Just verify the issues appear.
	if !strings.Contains(view, "in prog") {
		t.Error("view should contain 'in prog' summary")
	}
	if !strings.Contains(view, "done") {
		t.Error("view should contain 'done' summary")
	}
}

func TestJiraPaneCollapseMovesToParent(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "G-1", ProjectID: "p1", EpicID: "epic1", Summary: "child", Status: "Open"},
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())

	// Expand all.
	p.Expand() // expand project
	p.CursorDown()
	p.Expand() // expand epic
	p.CursorDown() // on the issue

	// Collapse from issue should move to epic.
	p.Collapse()
	sel := p.selected()
	if sel == nil || sel.Kind != nodeEpic {
		t.Error("collapse from issue should move cursor to parent epic")
	}
}

func TestJiraPaneRefreshPreservesExpansion(t *testing.T) {
	issues := []jira.JiraIssue{
		{Key: "H-1", ProjectID: "p1", Summary: "task", Status: "Open"},
	}
	theme := DefaultTheme()
	lister := &mockJiraLister{issues: issues}
	p := NewJiraPane(lister, theme)
	_ = p.Refresh(context.Background())

	// Expand project.
	p.Expand()
	if len(p.flat) != 2 {
		t.Fatalf("expected 2 rows after expand, got %d", len(p.flat))
	}

	// Refresh should preserve expansion.
	_ = p.Refresh(context.Background())
	if len(p.flat) != 2 {
		t.Errorf("expected 2 rows after refresh (expansion preserved), got %d", len(p.flat))
	}
}

func TestJiraPaneHeightClamp(t *testing.T) {
	var issues []jira.JiraIssue
	for i := 0; i < 30; i++ {
		issues = append(issues, jira.JiraIssue{
			Key:       fmt.Sprintf("Z-%d", i),
			ProjectID: "p1",
			Summary:   fmt.Sprintf("task %d", i),
			Status:    "Open",
		})
	}
	theme := DefaultTheme()
	p := NewJiraPane(&mockJiraLister{issues: issues}, theme)
	_ = p.Refresh(context.Background())
	p.Expand()
	p.SetSize(80, 10)

	view := p.View()
	lines := strings.Split(view, "\n")
	// Should clamp to height (10 lines = 9 rows + 1 footer).
	if len(lines) > 11 { // some tolerance for footer
		t.Errorf("expected at most ~11 lines, got %d", len(lines))
	}
	// The page indicator shows [start-end/total] when rows overflow.
	if !strings.Contains(view, "/") {
		t.Error("should show page indicator when rows overflow")
	}
}
