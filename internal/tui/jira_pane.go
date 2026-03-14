package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/noko/computecommander/pkg/integrations/jira"
)

// JiraLister abstracts Jira data access for testability.
type JiraLister interface {
	GetCachedIssues(ctx context.Context, projectKey, status string) ([]jira.JiraIssue, error)
}

// nodeKind identifies the level in the hierarchy.
type nodeKind int

const (
	nodeProject nodeKind = iota
	nodeEpic
	nodeIssue
)

// jiraNode is a single row in the hierarchical view.
type jiraNode struct {
	Kind       nodeKind
	Key        string // project key, epic key, or issue key
	Summary    string
	Status     string
	AgentType  string
	AgentState string
	Assignee   string
	Expanded   bool
	Depth      int
	Children   []*jiraNode
}

// JiraPane renders a columnar Jira task view.
type JiraPane struct {
	lister     JiraLister
	theme      *Theme
	projectKey string // optional project filter; empty = all

	// Hierarchical data.
	roots []*jiraNode // top-level project nodes
	flat  []*jiraNode // flattened visible rows (rebuilt on expand/collapse)

	// Navigation state.
	cursor     int
	pageOffset int
	width      int
	height     int

	// Instance label shown in footer.
	instanceName string

	// Help overlay.
	showHelp bool

	// Sub-task filtering.
	excludeSubTasks bool

	// Multi-select state.
	selectedKeys map[string]bool

	lastErr error
}

// NewJiraPane constructs a JiraPane.
func NewJiraPane(lister JiraLister, theme *Theme) *JiraPane {
	return &JiraPane{
		lister:          lister,
		theme:           theme,
		excludeSubTasks: true,
		selectedKeys:    make(map[string]bool),
	}
}

// Refresh fetches latest issues and rebuilds the hierarchy.
func (p *JiraPane) Refresh(ctx context.Context) error {
	if p.lister == nil {
		return nil
	}

	var issues []jira.JiraIssue
	var err error

	if filteredLister, ok := p.lister.(interface {
		GetCachedIssuesFiltered(context.Context, string, string, bool) ([]jira.JiraIssue, error)
	}); ok {
		issues, err = filteredLister.GetCachedIssuesFiltered(ctx, p.projectKey, "", p.excludeSubTasks)
	} else {
		issues, err = p.lister.GetCachedIssues(ctx, p.projectKey, "")
	}
	if err != nil {
		p.lastErr = err
		return fmt.Errorf("jira pane refresh: %w", err)
	}

	p.lastErr = nil
	p.buildHierarchy(issues)

	// Auto-expand all root project nodes on initial load (no prior expansion state).
	if len(p.roots) > 0 {
		for _, root := range p.roots {
			if !root.Expanded {
				root.Expanded = true
			}
		}
	}

	p.rebuildFlat()

	if p.cursor >= len(p.flat) && len(p.flat) > 0 {
		p.cursor = len(p.flat) - 1
	}
	return nil
}

// buildHierarchy groups issues into project > epic > issue tree.
func (p *JiraPane) buildHierarchy(issues []jira.JiraIssue) {
	// Preserve expansion state from previous roots.
	expanded := p.expandedSet()

	projectMap := make(map[string]*jiraNode)
	epicMap := make(map[string]*jiraNode)

	for _, issue := range issues {
		// Extract human-readable project key from issue.Key (e.g. "ES-123" → "ES").
		projKey := issue.ProjectID
		if parts := strings.SplitN(issue.Key, "-", 2); len(parts) == 2 && parts[0] != "" {
			projKey = parts[0]
		}
		if projKey == "" {
			projKey = "(none)"
		}
		projNode, ok := projectMap[projKey]
		if !ok {
			projNode = &jiraNode{
				Kind:     nodeProject,
				Key:      projKey,
				Summary:  projKey,
				Expanded: expanded[projKey],
				Depth:    0,
			}
			projectMap[projKey] = projNode
		}

		// Determine parent: epic or project.
		epicKey := issue.EpicID
		if epicKey != "" {
			epicNode, ok := epicMap[epicKey]
			if !ok {
				epicNode = &jiraNode{
					Kind:     nodeEpic,
					Key:      epicKey,
					Summary:  "Epic " + epicKey,
					Expanded: expanded[epicKey],
					Depth:    1,
				}
				epicMap[epicKey] = epicNode
				projNode.Children = append(projNode.Children, epicNode)
			}
			epicNode.Children = append(epicNode.Children, &jiraNode{
				Kind:       nodeIssue,
				Key:        issue.Key,
				Summary:    issue.Summary,
				Status:     issue.Status,
				AgentType:  issue.AgentType,
				AgentState: issue.AgentState,
				Assignee:   issue.Assignee,
				Depth:      2,
			})
		} else {
			projNode.Children = append(projNode.Children, &jiraNode{
				Kind:       nodeIssue,
				Key:        issue.Key,
				Summary:    issue.Summary,
				Status:     issue.Status,
				AgentType:  issue.AgentType,
				AgentState: issue.AgentState,
				Assignee:   issue.Assignee,
				Depth:      1,
			})
		}
	}

	// Collect roots in stable order.
	p.roots = make([]*jiraNode, 0, len(projectMap))
	for _, node := range projectMap {
		p.roots = append(p.roots, node)
	}
}

// expandedSet returns the set of keys that are currently expanded.
func (p *JiraPane) expandedSet() map[string]bool {
	m := make(map[string]bool)
	var walk func([]*jiraNode)
	walk = func(nodes []*jiraNode) {
		for _, n := range nodes {
			if n.Expanded {
				m[n.Key] = true
			}
			walk(n.Children)
		}
	}
	walk(p.roots)
	return m
}

// rebuildFlat walks the tree and produces the visible flat list.
func (p *JiraPane) rebuildFlat() {
	p.flat = p.flat[:0]
	var walk func([]*jiraNode)
	walk = func(nodes []*jiraNode) {
		for _, n := range nodes {
			p.flat = append(p.flat, n)
			if n.Expanded {
				walk(n.Children)
			}
		}
	}
	walk(p.roots)
}

// --- Navigation ---

// CursorUp moves the cursor up and auto-scrolls to keep it visible.
func (p *JiraPane) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
		p.scrollToCursor(p.visibleRows())
	}
}

// CursorDown moves the cursor down and auto-scrolls to keep it visible.
func (p *JiraPane) CursorDown() {
	if p.cursor < len(p.flat)-1 {
		p.cursor++
		p.scrollToCursor(p.visibleRows())
	}
}

// PageDown advances the page offset by maxRows.
func (p *JiraPane) PageDown() {
	maxRows := p.visibleRows()
	p.pageOffset += maxRows
	last := len(p.flat) - 1
	if last < 0 {
		last = 0
	}
	if p.pageOffset > last {
		p.pageOffset = last
	}
	// Move cursor into view.
	if p.cursor < p.pageOffset {
		p.cursor = p.pageOffset
	}
}

// PageUp decreases the page offset by maxRows.
func (p *JiraPane) PageUp() {
	maxRows := p.visibleRows()
	p.pageOffset -= maxRows
	if p.pageOffset < 0 {
		p.pageOffset = 0
	}
	// Move cursor into view.
	if p.cursor >= p.pageOffset+maxRows {
		p.cursor = p.pageOffset + maxRows - 1
		if p.cursor < 0 {
			p.cursor = 0
		}
	}
}

// GoTop moves cursor to the first row.
func (p *JiraPane) GoTop() {
	p.cursor = 0
	p.pageOffset = 0
}

// GoBottom moves cursor to the last row and scrolls to show it.
func (p *JiraPane) GoBottom() {
	if len(p.flat) == 0 {
		return
	}
	p.cursor = len(p.flat) - 1
	p.scrollToCursor(p.visibleRows())
}

// visibleRows returns the number of rows that fit in the pane.
func (p *JiraPane) visibleRows() int {
	h := p.height
	if h < 3 {
		h = 10
	}
	// Reserve last line for footer.
	return min(h-1, 15)
}

// scrollToCursor adjusts pageOffset so cursor stays within the visible window.
func (p *JiraPane) scrollToCursor(maxRows int) {
	if p.cursor < p.pageOffset {
		p.pageOffset = p.cursor
	} else if p.cursor >= p.pageOffset+maxRows {
		p.pageOffset = p.cursor - maxRows + 1
	}
}

// Expand expands the selected node (or drills into it).
func (p *JiraPane) Expand() {
	if n := p.selected(); n != nil && len(n.Children) > 0 {
		n.Expanded = true
		p.rebuildFlat()
	}
}

// Collapse collapses the selected node or moves to parent.
func (p *JiraPane) Collapse() {
	n := p.selected()
	if n == nil {
		return
	}
	if n.Expanded && len(n.Children) > 0 {
		n.Expanded = false
		p.rebuildFlat()
		return
	}
	// Move cursor to parent.
	if n.Depth > 0 {
		for i := p.cursor - 1; i >= 0; i-- {
			if p.flat[i].Depth < n.Depth {
				p.cursor = i
				break
			}
		}
	}
}

// ToggleHelp toggles the help overlay.
func (p *JiraPane) ToggleHelp() {
	p.showHelp = !p.showHelp
}

// Selected returns the currently selected node, or nil.
func (p *JiraPane) selected() *jiraNode {
	if p.cursor >= 0 && p.cursor < len(p.flat) {
		return p.flat[p.cursor]
	}
	return nil
}

// SelectedKey returns the key of the selected node.
func (p *JiraPane) SelectedKey() string {
	if n := p.selected(); n != nil {
		return n.Key
	}
	return ""
}

// ToggleSelect toggles selection on an issue key.
func (p *JiraPane) ToggleSelect(key string) {
	if key == "" {
		return
	}
	if p.selectedKeys[key] {
		delete(p.selectedKeys, key)
	} else {
		p.selectedKeys[key] = true
	}
}

// SelectedKeys returns the list of selected issue keys.
func (p *JiraPane) SelectedKeys() []string {
	keys := make([]string, 0, len(p.selectedKeys))
	for k := range p.selectedKeys {
		keys = append(keys, k)
	}
	return keys
}

// ClearSelection resets all selections.
func (p *JiraPane) ClearSelection() {
	p.selectedKeys = make(map[string]bool)
}

// SelectAll selects all visible issue nodes.
func (p *JiraPane) SelectAll() {
	for _, n := range p.flat {
		if n.Kind == nodeIssue {
			p.selectedKeys[n.Key] = true
		}
	}
}

// IsSelected returns true if the given key is selected.
func (p *JiraPane) IsSelected(key string) bool {
	return p.selectedKeys[key]
}

// HasSelection returns true if any keys are selected.
func (p *JiraPane) HasSelection() bool {
	return len(p.selectedKeys) > 0
}

// AllIssueKeys returns the keys of all visible issue-type nodes.
func (p *JiraPane) AllIssueKeys() []string {
	var keys []string
	for _, n := range p.flat {
		if n.Kind == nodeIssue {
			keys = append(keys, n.Key)
		}
	}
	return keys
}

// Cursor returns the current cursor index.
func (p *JiraPane) Cursor() int {
	return p.cursor
}

// IssueCount returns the total number of visible rows.
func (p *JiraPane) IssueCount() int {
	return len(p.flat)
}

// SetSize sets the pane dimensions.
func (p *JiraPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetProject restricts Refresh to a specific Jira project key.
// An empty string fetches issues from all projects.
func (p *JiraPane) SetProject(key string) {
	p.projectKey = key
}

// ProjectKey returns the current project filter key.
func (p *JiraPane) ProjectKey() string {
	return p.projectKey
}

// SetExcludeSubTasks sets whether sub-tasks are hidden.
func (p *JiraPane) SetExcludeSubTasks(exclude bool) {
	p.excludeSubTasks = exclude
}

// ToggleSubTasks toggles sub-task visibility.
func (p *JiraPane) ToggleSubTasks() {
	p.excludeSubTasks = !p.excludeSubTasks
}

// ExcludeSubTasks returns the current sub-task filter state.
func (p *JiraPane) ExcludeSubTasks() bool {
	return p.excludeSubTasks
}

// SetInstance sets the instance label shown in the footer.
func (p *JiraPane) SetInstance(name string) {
	p.instanceName = name
}

// --- Rendering ---

// Column widths for the table layout.
const (
	colAgent    = 6
	colState    = 8
	colKey      = 10
	colStatus   = 14
	colAssignee = 12
)

// View renders the Jira pane as a string.
func (p *JiraPane) View() string {
	if p.showHelp {
		return p.renderHelp()
	}

	if p.lastErr != nil {
		return p.theme.StateZombie.Render(fmt.Sprintf("Error: %v", p.lastErr))
	}

	if len(p.flat) == 0 {
		return p.theme.Subtitle.Render("  No Jira issues\n  Press 's' to sync")
	}

	w := p.width
	if w < 40 {
		w = 60
	}

	// Reserve 1 row for the column header and 1 for the footer.
	maxRows := p.visibleRows() - 1
	if maxRows < 1 {
		maxRows = 1
	}
	total := len(p.flat)

	// Clamp pageOffset in case flat shrunk after a refresh.
	if p.pageOffset >= total {
		p.pageOffset = max(0, total-1)
	}

	end := p.pageOffset + maxRows
	if end > total {
		end = total
	}

	var lines []string

	// Column header.
	lines = append(lines, p.renderHeader(w))

	for i := p.pageOffset; i < end; i++ {
		lines = append(lines, p.renderNode(p.flat[i], i, w))
	}

	// Footer: instance label (left) + key hints + page indicator (right).
	instLabel := ""
	if p.instanceName != "" {
		instLabel = p.theme.Subtitle.Render(p.instanceName) + "  "
	}
	pageInfo := ""
	if total > maxRows {
		pageInfo = fmt.Sprintf(" [%d-%d/%d]", p.pageOffset+1, end, total)
	}
	subTaskLabel := " [sub-tasks: visible]"
	if p.excludeSubTasks {
		subTaskLabel = " [sub-tasks: hidden]"
	}
	lines = append(lines, instLabel+p.theme.HelpBar.Render("j/k:nav  space:sel  t:subtasks  e:prompt  E:batch  s:sync  ?:help"+subTaskLabel+pageInfo))

	return strings.Join(lines, "\n")
}

// renderHeader returns the styled column header line.
func (p *JiraPane) renderHeader(width int) string {
	summaryWidth := p.summaryWidth(width)
	header := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %-*s",
		colAgent, "AGENT",
		colState, "STATE",
		colKey, "KEY",
		summaryWidth, "SUMMARY",
		colStatus, "STATUS",
		colAssignee, "ASSIGNEE",
	)
	if len(header) > width {
		header = header[:width]
	}
	return p.theme.Subtitle.Render(header)
}

// summaryWidth calculates the flexible summary column width.
func (p *JiraPane) summaryWidth(totalWidth int) int {
	// 2 chars for cursor prefix + spaces between columns (5 gaps of 1).
	fixed := 2 + colAgent + 1 + colState + 1 + colKey + 1 + colStatus + 1 + colAssignee
	w := totalWidth - fixed
	if w < 10 {
		w = 10
	}
	return w
}

// renderNode renders a single node as a table row or section header.
func (p *JiraPane) renderNode(n *jiraNode, idx, width int) string {
	cursor := "  "
	if idx == p.cursor {
		cursor = "> "
	}

	// Project and epic nodes render as collapsible section headers.
	if n.Kind == nodeProject || n.Kind == nodeEpic {
		expandIcon := "v "
		if !n.Expanded {
			expandIcon = "> "
		}
		header := fmt.Sprintf("%s%s%s", cursor, expandIcon, n.Key)
		if n.Summary != n.Key {
			header += "  " + n.Summary
		}
		if len(header) > width {
			header = header[:width]
		}
		if n.Kind == nodeProject {
			return p.theme.Title.Render(header)
		}
		return p.theme.Subtitle.Render(header)
	}

	// Issue rows: flat columnar table (no indentation).
	summaryWidth := p.summaryWidth(width)

	// Show checkbox prefix when any selections exist.
	selectPrefix := ""
	if p.HasSelection() {
		if p.IsSelected(n.Key) {
			selectPrefix = "[x] "
		} else {
			selectPrefix = "[ ] "
		}
		summaryWidth -= 4 // accommodate checkbox prefix
		if summaryWidth < 6 {
			summaryWidth = 6
		}
	}

	agent := "-"
	if n.AgentType != "" {
		agent = truncate(n.AgentType, colAgent)
	}

	state := "-"
	stateStyled := state
	if n.AgentState != "" {
		state = truncate(n.AgentState, colState)
		stateStyled = p.renderAgentState(state)
	}

	key := truncate(n.Key, colKey)
	summary := truncate(n.Summary, summaryWidth)
	statusStyled := p.renderStatus(n.Status)
	assignee := truncate(n.Assignee, colAssignee)
	if assignee == "" {
		assignee = "-"
	}

	_ = state // stateStyled used below

	line := fmt.Sprintf("%s%s%-*s %-*s %-*s %-*s %-*s %-*s",
		cursor,
		selectPrefix,
		colAgent, agent,
		colState, stateStyled,
		colKey, key,
		summaryWidth, summary,
		colStatus, statusStyled,
		colAssignee, assignee,
	)

	if len(line) > width+50 { // allow for ANSI escape overhead
		line = line[:width+50]
	}

	return line
}

// renderStatus applies theme coloring to a Jira status string.
func (p *JiraPane) renderStatus(status string) string {
	s := truncate(status, colStatus)
	if s == "" {
		return fmt.Sprintf("%-*s", colStatus, "-")
	}
	lower := strings.ToLower(status)
	padded := fmt.Sprintf("%-*s", colStatus, s)
	switch {
	case lower == "in progress" || lower == "active":
		return p.theme.StateWorking.Render(padded)
	case lower == "done" || lower == "completed":
		return p.theme.StateCompleted.Render(padded)
	case lower == "blocked" || lower == "failed":
		return p.theme.StateZombie.Render(padded)
	case lower == "to do" || lower == "open":
		return p.theme.StateBooting.Render(padded)
	default:
		return p.theme.StateStalled.Render(padded)
	}
}

// renderAgentState applies theme colors to agent state strings.
func (p *JiraPane) renderAgentState(state string) string {
	padded := fmt.Sprintf("%-*s", colState, state)
	lower := strings.ToLower(state)
	switch {
	case lower == "working":
		return p.theme.StateWorking.Render(padded)
	case lower == "booting":
		return p.theme.StateBooting.Render(padded)
	case lower == "completed":
		return p.theme.StateCompleted.Render(padded)
	case lower == "stalled":
		return p.theme.StateStalled.Render(padded)
	default:
		return padded
	}
}

// renderHelp returns the help overlay content.
func (p *JiraPane) renderHelp() string {
	lines := []string{
		p.theme.Title.Render("Jira Pane Help"),
		"",
		"  j/k         Navigate up/down",
		"  gg/G        Jump to top/bottom",
		"  n/pgdn/C-d  Page down",
		"  N/pgup/C-u  Page up",
		"  Enter/l     Expand / drill into",
		"  h/Esc       Collapse / go up",
		"  o           View issue detail",
		"  t           Toggle sub-task visibility",
		"  s           Sync from Jira",
		"  e           Execute prompt for ticket",
		"  E           Batch execute (selected/all)",
		"  space       Toggle select ticket",
		"  u           Undo last prompt execution",
		"  L           View execution log",
		"  p           Select project",
		"  P           Preview generated prompt",
		"  x           Execute task (spawn agent)",
		"  i           Switch Jira instance",
		"  f           Dark factory mode",
		"  v           Verify intent/outcomes",
		"  ?           Toggle this help",
		"  q           Quit / close pane",
		"",
		p.theme.Subtitle.Render("  Press ? to close"),
	}
	return strings.Join(lines, "\n")
}
