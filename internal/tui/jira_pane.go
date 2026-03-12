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
	Kind      nodeKind
	Key       string // project key, epic key, or issue key
	Summary   string
	Status    string
	AgentType string
	AgentState string
	Expanded  bool
	Depth     int
	Children  []*jiraNode
}

// JiraPane renders a hierarchical Jira task view.
type JiraPane struct {
	lister     JiraLister
	theme      *Theme
	projectKey string // optional project filter; empty = all

	// Hierarchical data.
	roots []*jiraNode // top-level project nodes
	flat  []*jiraNode // flattened visible rows (rebuilt on expand/collapse)

	// Navigation state.
	cursor int
	width  int
	height int

	// Help overlay.
	showHelp bool

	lastErr error
}

// NewJiraPane constructs a JiraPane.
func NewJiraPane(lister JiraLister, theme *Theme) *JiraPane {
	return &JiraPane{
		lister: lister,
		theme:  theme,
	}
}

// Refresh fetches latest issues and rebuilds the hierarchy.
func (p *JiraPane) Refresh(ctx context.Context) error {
	if p.lister == nil {
		return nil
	}

	issues, err := p.lister.GetCachedIssues(ctx, p.projectKey, "")
	if err != nil {
		p.lastErr = err
		return fmt.Errorf("jira pane refresh: %w", err)
	}

	p.lastErr = nil
	p.buildHierarchy(issues)
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
		// Ensure project node exists.
		projKey := issue.ProjectID
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

// CursorUp moves the cursor up.
func (p *JiraPane) CursorUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

// CursorDown moves the cursor down.
func (p *JiraPane) CursorDown() {
	if p.cursor < len(p.flat)-1 {
		p.cursor++
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

// --- Rendering ---

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
	h := p.height
	if h < 3 {
		h = 10
	}

	// Reserve last line for key hints.
	maxRows := h - 1

	var lines []string
	for i, n := range p.flat {
		if i >= maxRows {
			lines = append(lines, p.theme.Subtitle.Render(
				fmt.Sprintf("  ... +%d more", len(p.flat)-maxRows)))
			break
		}
		lines = append(lines, p.renderNode(n, i, w))
	}

	// Key hints footer.
	lines = append(lines, p.theme.HelpBar.Render("j/k:nav  l/h:expand  s:sync  x:exec  ?:help"))

	return strings.Join(lines, "\n")
}

// renderNode renders a single tree node as a line.
func (p *JiraPane) renderNode(n *jiraNode, idx, width int) string {
	prefix := "  "
	if idx == p.cursor {
		prefix = "> "
	}

	indent := strings.Repeat("  ", n.Depth)

	var expandIcon string
	switch {
	case len(n.Children) > 0 && n.Expanded:
		expandIcon = "v "
	case len(n.Children) > 0:
		expandIcon = "> "
	default:
		expandIcon = "  "
	}

	statusIcon := p.statusIcon(n.Status)
	agent := ""
	if n.AgentState != "" {
		agent = fmt.Sprintf(" [%s]", p.renderAgentState(n.AgentState))
	}

	// Key column.
	keyWidth := 12
	key := truncate(n.Key, keyWidth)

	// Summary gets remaining space.
	usedWidth := len(prefix) + len(indent) + len(expandIcon) + keyWidth + 2 + len(statusIcon) + 1
	summaryWidth := width - usedWidth - 10 // reserve space for agent state
	if summaryWidth < 10 {
		summaryWidth = 10
	}
	summary := truncate(n.Summary, summaryWidth)

	line := fmt.Sprintf("%s%s%s%-*s %s %s%s",
		prefix, indent, expandIcon,
		keyWidth, key,
		statusIcon,
		summary,
		agent,
	)

	if len(line) > width {
		line = line[:width]
	}

	return line
}

// statusIcon returns a styled status indicator.
func (p *JiraPane) statusIcon(status string) string {
	lower := strings.ToLower(status)
	switch {
	case lower == "in progress" || lower == "active":
		return p.theme.StateWorking.Render("*")
	case lower == "done" || lower == "completed":
		return p.theme.StateCompleted.Render("~")
	case lower == "blocked" || lower == "failed":
		return p.theme.StateZombie.Render("!")
	case lower == "to do" || lower == "open":
		return p.theme.StateBooting.Render("o")
	case lower == "":
		return " "
	default:
		return p.theme.StateStalled.Render("?")
	}
}

// renderAgentState applies theme colors to agent state strings.
func (p *JiraPane) renderAgentState(state string) string {
	lower := strings.ToLower(state)
	switch {
	case lower == "working":
		return p.theme.StateWorking.Render(state)
	case lower == "booting":
		return p.theme.StateBooting.Render(state)
	case lower == "completed":
		return p.theme.StateCompleted.Render(state)
	case lower == "stalled":
		return p.theme.StateStalled.Render(state)
	default:
		return state
	}
}

// renderHelp returns the help overlay content.
func (p *JiraPane) renderHelp() string {
	lines := []string{
		p.theme.Title.Render("Jira Pane Help"),
		"",
		"  j/k       Navigate up/down",
		"  Enter/l   Expand / drill into",
		"  h/Esc     Collapse / go up",
		"  s         Sync from Jira",
		"  e         Edit/generate prompt",
		"  p         Preview generated prompt",
		"  x         Execute task (spawn agent)",
		"  i         Switch Jira instance",
		"  f         Dark factory mode",
		"  v         Verify intent/outcomes",
		"  ?         Toggle this help",
		"",
		p.theme.Subtitle.Render("  Press ? to close"),
	}
	return strings.Join(lines, "\n")
}
