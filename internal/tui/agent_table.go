package tui

import (
	"context"
	"fmt"
	"strings"
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
	sessions  []*agents.AgentSession
	lister    SessionLister
	cursor    int
	theme     *Theme
	dashStart time.Time // when the dashboard was started; used to filter stale completed sessions
}

// NewAgentTable constructs an AgentTable.
func NewAgentTable(lister SessionLister, theme *Theme) *AgentTable {
	return &AgentTable{
		lister:    lister,
		theme:     theme,
		dashStart: time.Now(),
	}
}

// Refresh fetches the latest sessions from the database and filters out
// stale entries from previous dashboard sessions.
func (t *AgentTable) Refresh(ctx context.Context) error {
	sessions, err := t.lister.ListSessions(ctx, agents.ListOpts{})
	if err != nil {
		return fmt.Errorf("agent table refresh: %w", err)
	}
	sessions = t.filterLiveSessions(sessions)
	t.sessions = sessions
	if t.cursor >= len(t.sessions) && len(t.sessions) > 0 {
		t.cursor = len(t.sessions) - 1
	}
	return nil
}

// filterLiveSessions removes sessions that should not appear in the agents pane:
//   - Completed/zombie sessions from before the dashboard was started are hidden,
//     since they belong to a previous run and are stale.
//   - Working/booting sessions whose last_activity is older than staleThreshold
//     are also excluded, as they likely represent ghost entries from crashed
//     processes where the SubagentStop hook failed to fire.
//
// This mirrors the filterPaneSessions() logic in internal/commands/status.go
// so the TUI dashboard behaves consistently with the KDL pane-mode status view.
func (t *AgentTable) filterLiveSessions(sessions []*agents.AgentSession) []*agents.AgentSession {
	if t.dashStart.IsZero() {
		return sessions
	}

	// Threshold: sessions with no activity for this long and in an active state
	// from before the dashboard started are considered stale ghosts.
	const staleThreshold = 10 * time.Minute

	filtered := make([]*agents.AgentSession, 0, len(sessions))
	for _, s := range sessions {
		// Skip filtering for sessions without a real StartedAt (e.g. in-memory mocks).
		if s.StartedAt.IsZero() {
			filtered = append(filtered, s)
			continue
		}

		switch s.State {
		case agents.StateCompleted, agents.StateZombie:
			// Keep completed/zombie agents only if they started during this
			// dashboard session. Old completed agents from previous runs are
			// stale and should not clutter the pane.
			if s.StartedAt.Before(t.dashStart) {
				continue
			}
		case agents.StateWorking, agents.StateBooting:
			// If an agent has had no activity for staleThreshold AND it
			// started before the dashboard, it is almost certainly a ghost
			// entry from a previous run whose process died without updating
			// the DB. Skip it so the pane stays clean; the dashboard reaper
			// will transition these to completed in the DB.
			if !s.LastActivity.IsZero() && time.Since(s.LastActivity) > staleThreshold && s.StartedAt.Before(t.dashStart) {
				continue
			}
		}
		filtered = append(filtered, s)
	}
	return filtered
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
		{Header: "Duration", Width: 10},
		{Header: "Task", Width: 12},
		{Header: "Runtime", Width: 8},
		{Header: "Tokens", Width: 8},
	}

	var rows [][]string
	for i, s := range t.sessions {
		stateStr := t.renderState(s.State)
		rt := formatRuntimeID(s.Runtime)
		tokens := formatTokens(s.InputTokens + s.OutputTokens)
		dur := RuntimeDuration(s.StartedAt)

		// Use a cursor prefix on the name column only.
		namePrefix := "  "
		if i == t.cursor {
			namePrefix = "> "
		}

		// Color the agent name using their assigned palette color.
		agentStyle := StyleForAgent(s)
		coloredName := agentStyle.Render(truncate(s.AgentName, cols[0].Width-2))

		row := []string{
			namePrefix + coloredName,
			truncate(string(s.Capability), cols[1].Width),
			stateStr,
			dur,
			truncate(s.TaskID, cols[4].Width),
			rt,
			tokens,
		}

		rows = append(rows, row)
	}

	title := fmt.Sprintf("Agents (%d active)", len(t.sessions))
	table := renderTable(cols, rows, t.theme)
	return t.theme.Title.Render(title) + "\n" + table
}

// CompactView renders a compact agent list suitable for the sidebar pane.
func (t *AgentTable) CompactView(width, height int) string {
	if len(t.sessions) == 0 {
		return t.theme.Subtitle.Render("  No agents")
	}

	var lines []string
	for i, s := range t.sessions {
		if height > 0 && i >= height {
			break
		}
		stateStr := t.renderState(s.State)
		tokens := formatTokens(s.InputTokens + s.OutputTokens)
		dur := RuntimeDuration(s.StartedAt)
		// Color the agent name using their assigned palette color.
		agentStyle := StyleForAgent(s)
		name := agentStyle.Render(truncate(s.AgentName, width-28))

		prefix := "  "
		if i == t.cursor {
			prefix = "> "
		}

		line := fmt.Sprintf("%s%s %s %s %s", prefix, name, stateStr, dur, tokens)
		if len(line) > width {
			line = line[:width]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
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
// Returns "-" if the start time is zero (agent not yet started).
func RuntimeDuration(started time.Time) string {
	if started.IsZero() {
		return "-"
	}
	d := time.Since(started).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
