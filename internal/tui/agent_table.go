package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/pkg/runtimes"
)

// SessionLister fetches agent sessions. Satisfied by *agents.Spawner.
type SessionLister interface {
	ListSessions(ctx context.Context, opts agents.ListOpts) ([]*agents.AgentSession, error)
}

// AgentTable renders the agent session table view.
type AgentTable struct {
	sessions []*agents.AgentSession
	lister   SessionLister
	cursor   int
	theme    *Theme
}

// NewAgentTable constructs an AgentTable.
func NewAgentTable(lister SessionLister, theme *Theme) *AgentTable {
	return &AgentTable{
		lister: lister,
		theme:  theme,
	}
}

// Refresh fetches the latest sessions from the database.
func (t *AgentTable) Refresh(ctx context.Context) error {
	sessions, err := t.lister.ListSessions(ctx, agents.ListOpts{})
	if err != nil {
		return fmt.Errorf("agent table refresh: %w", err)
	}
	t.sessions = sessions
	if t.cursor >= len(t.sessions) && len(t.sessions) > 0 {
		t.cursor = len(t.sessions) - 1
	}
	return nil
}

// Sessions returns the current snapshot.
func (t *AgentTable) Sessions() []*agents.AgentSession {
	return t.sessions
}

// Cursor returns the current cursor index.
func (t *AgentTable) Cursor() int {
	return t.cursor
}

// CursorUp moves the selection up.
func (t *AgentTable) CursorUp() {
	if t.cursor > 0 {
		t.cursor--
	}
}

// CursorDown moves the selection down.
func (t *AgentTable) CursorDown() {
	if t.cursor < len(t.sessions)-1 {
		t.cursor++
	}
}

// Selected returns the currently selected session, or nil.
func (t *AgentTable) Selected() *agents.AgentSession {
	if t.cursor >= 0 && t.cursor < len(t.sessions) {
		return t.sessions[t.cursor]
	}
	return nil
}

// View renders the agent table as a string.
func (t *AgentTable) View() string {
	cols := []column{
		{Header: "Name", Width: 12},
		{Header: "Capability", Width: 12},
		{Header: "State", Width: 10},
		{Header: "Task", Width: 12},
		{Header: "Runtime", Width: 8},
		{Header: "Tokens", Width: 8},
	}

	var rows [][]string
	for i, s := range t.sessions {
		stateStr := t.renderState(s.State)
		rt := formatRuntimeID(s.Runtime)
		tokens := formatTokens(s.InputTokens + s.OutputTokens)

		// Use a cursor prefix on the name column only.
		namePrefix := "  "
		if i == t.cursor {
			namePrefix = "> "
		}

		row := []string{
			namePrefix + truncate(s.AgentName, cols[0].Width-2),
			truncate(string(s.Capability), cols[1].Width),
			stateStr,
			truncate(s.TaskID, cols[3].Width),
			rt,
			tokens,
		}

		rows = append(rows, row)
	}

	title := fmt.Sprintf("Agents (%d active)", len(t.sessions))
	table := renderTable(cols, rows, t.theme)
	return t.theme.Title.Render(title) + "\n" + table
}

// renderState applies color-coded styling based on session state.
func (t *AgentTable) renderState(state agents.SessionState) string {
	switch state {
	case agents.StateWorking:
		return t.theme.StateWorking.Render(string(state))
	case agents.StateBooting:
		return t.theme.StateBooting.Render(string(state))
	case agents.StateStalled:
		return t.theme.StateStalled.Render(string(state))
	case agents.StateZombie:
		return t.theme.StateZombie.Render(string(state))
	case agents.StateCompleted:
		return t.theme.StateCompleted.Render(string(state))
	default:
		return string(state)
	}
}

// formatRuntimeID returns a short display string for the runtime ID.
func formatRuntimeID(rt runtimes.RuntimeID) string {
	return string(rt)
}

// RuntimeDuration returns a human-readable duration since started.
func RuntimeDuration(started time.Time) string {
	d := time.Since(started).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
