package agentui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/agents"
)

// AgentLister is the dependency contract for RenderAgents. *agents.Spawner
// from internal/agents satisfies it; tests pass a stub implementation. By
// taking the interface here, internal/agentui has no inbound dependency on
// internal/commands and can be exercised without a live App / DB.
type AgentLister interface {
	ListSessions(ctx context.Context, opts agents.ListOpts) ([]*agents.AgentSession, error)
	BuildColorResolver(ctx context.Context) func(string) string
}

// AgentsOpts is the contract surface of RenderAgents.
type AgentsOpts struct {
	// Lines is the exact line count of the output. Truncates overflow,
	// pads short output. <= 0 returns nil.
	Lines int
	// Width is the maximum visible columns per line (ANSI excluded).
	// <= 0 collapses to the degraded marker.
	Width int
	// NoColor strips ALL ANSI / Unicode-decorative output.
	NoColor bool
	// Now is the trailer timestamp source. Tests inject a fixed time;
	// production uses time.Now().
	Now time.Time
	// Filter (optional) further constrains the agents fetched from the
	// lister. Mirrors agents.ListOpts on the underlying Spawner.
	Filter agents.ListOpts
}

// RenderAgents fetches active agent sessions via lister and renders them
// as exactly opts.Lines lines, each <= opts.Width visible cols. On any
// fetch failure, returns the "agents: unavailable" degraded marker padded
// to opts.Lines. NEVER returns an error.
//
// The caller is responsible for handing this []string to stdout via
// fmt.Fprintln. The renderer itself is pure: no I/O beyond the lister
// dependency.
func RenderAgents(ctx context.Context, lister AgentLister, opts AgentsOpts) []string {
	if opts.Lines <= 0 {
		return nil
	}
	if opts.Width <= 0 {
		return DegradedMarker(LabelAgents, opts.Lines)
	}
	if lister == nil {
		return DegradedMarker(LabelAgents, opts.Lines)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	sessions, err := lister.ListSessions(ctx, opts.Filter)
	if err != nil {
		return DegradedMarker(LabelAgents, opts.Lines)
	}

	pal := NewPalette(opts.NoColor)
	bs := NewBoxStyle(pal)

	colorResolver := lister.BuildColorResolver(ctx)

	// Count active agents (working/booting) for the header.
	active := 0
	for _, s := range sessions {
		if s != nil && (s.State == agents.StateWorking || s.State == agents.StateBooting) {
			active++
		}
	}

	// Sort: most recent activity first so the user's freshest work is at the
	// top regardless of how many lines fit. Deterministic tiebreak by ID.
	sortedSessions := make([]*agents.AgentSession, 0, len(sessions))
	for _, s := range sessions {
		if s != nil {
			sortedSessions = append(sortedSessions, s)
		}
	}
	sort.SliceStable(sortedSessions, func(i, j int) bool {
		ai := sortedSessions[i].LastActivity
		aj := sortedSessions[j].LastActivity
		if !ai.Equal(aj) {
			return ai.After(aj)
		}
		return sortedSessions[i].ID < sortedSessions[j].ID
	})

	out := make([]string, 0, opts.Lines)

	// Line 1: header — "Agents · N active".
	header := pal.Bold + "Agents" + pal.Reset +
		pal.Dim + bs.Sep + fmt.Sprintf("%d active", active) + pal.Reset
	out = append(out, Truncate(header, opts.Width))

	// Line N: trailer — "updated HH:MM:SS".
	// Reserve one line for the trailer; everything between header and trailer
	// is the agent-row budget.
	rowBudget := max(opts.Lines-2, 0) // -1 header, -1 trailer

	if len(sortedSessions) == 0 && rowBudget > 0 {
		empty := pal.Dim + "no agents" + pal.Reset
		out = append(out, Truncate(empty, opts.Width))
		for i := 1; i < rowBudget; i++ {
			out = append(out, "")
		}
	} else {
		// Width-adaptive: < 60 cols collapses to a compact 3-col view
		// (name | state | task). Mirrors the compactPaneWidthThreshold
		// logic in internal/commands/status.go.
		compact := opts.Width < 60
		shown := 0
		for _, s := range sortedSessions {
			if shown >= rowBudget {
				break
			}
			row := renderAgentRow(s, opts.Width, compact, colorResolver, pal)
			out = append(out, Truncate(row, opts.Width))
			shown++
		}
		// Pad row area if fewer sessions than budget.
		for shown < rowBudget {
			out = append(out, "")
			shown++
		}
	}

	if opts.Lines >= 2 {
		trailer := pal.Dim + "updated " + now.Format("15:04:05") + pal.Reset
		out = append(out, Truncate(trailer, opts.Width))
	}

	return PadOrTruncate(out, opts.Lines)
}

// renderAgentRow formats a single agent session row. In compact mode it
// emits three columns (name | state | task); in wide mode it emits five
// (name | cap | state | model | task).
func renderAgentRow(s *agents.AgentSession, width int, compact bool, colorResolver func(string) string, pal Palette) string {
	stateColor := stateANSIColor(s.State, pal)

	var sb strings.Builder
	sb.WriteString("  ")

	if compact {
		// Compact: name(12) state(8) task(remaining)
		nameWidth := 12
		stateWidth := 8
		// Visible budget for the row prefix: "  " + name + " " + state + " "
		// Everything after is the task column.
		taskBudget := max(width-2-nameWidth-1-stateWidth-1, 4)
		nameRaw := truncASCII(s.AgentName, nameWidth)
		nameColored := colorizeAgentName(nameRaw, colorResolver, pal)
		sb.WriteString(padRightVisible(nameColored, nameWidth))
		sb.WriteString(" ")
		sb.WriteString(stateColor)
		sb.WriteString(padRightAscii(truncASCII(string(s.State), stateWidth), stateWidth))
		sb.WriteString(pal.Reset)
		sb.WriteString(" ")
		sb.WriteString(truncASCII(s.TaskID, taskBudget))
		return sb.String()
	}

	// Wide: name(14) cap(10) state(10) model(14) task(remaining)
	nameWidth := 14
	capWidth := 10
	stateWidth := 10
	modelWidth := 14
	// "  " + name + " " + cap + " " + state + " " + model + " " + task
	taskBudget := max(width-2-nameWidth-1-capWidth-1-stateWidth-1-modelWidth-1, 4)
	nameRaw := truncASCII(s.AgentName, nameWidth)
	nameColored := colorizeAgentName(nameRaw, colorResolver, pal)
	sb.WriteString(padRightVisible(nameColored, nameWidth))
	sb.WriteString(" ")
	sb.WriteString(padRightAscii(truncASCII(string(s.Capability), capWidth), capWidth))
	sb.WriteString(" ")
	sb.WriteString(stateColor)
	sb.WriteString(padRightAscii(truncASCII(string(s.State), stateWidth), stateWidth))
	sb.WriteString(pal.Reset)
	sb.WriteString(" ")
	sb.WriteString(padRightAscii(truncASCII(formatModelShort(s.Model), modelWidth), modelWidth))
	sb.WriteString(" ")
	sb.WriteString(truncASCII(s.TaskID, taskBudget))
	return sb.String()
}

// stateANSIColor returns the SGR prefix for a session state. Closed by the
// caller with pal.Reset. Empty string in NoColor mode.
func stateANSIColor(state agents.SessionState, pal Palette) string {
	switch state {
	case agents.StateWorking:
		return pal.Green
	case agents.StateBooting:
		return pal.Cyan
	case agents.StateCompleted:
		return pal.Dim
	case agents.StateStalled:
		return pal.Yellow
	case agents.StateZombie:
		return pal.Red
	default:
		return pal.Dim
	}
}

// colorizeAgentName wraps the agent name in its assigned palette color. In
// NoColor mode returns the name as-is. The colorResolver dependency comes
// from agents.Spawner.BuildColorResolver and returns a hex string (or "").
func colorizeAgentName(name string, colorResolver func(string) string, pal Palette) string {
	if pal.Reset == "" || colorResolver == nil {
		return name
	}
	hex := colorResolver(name)
	if hex == "" {
		return name
	}
	escape := pal.Hex24(hex)
	if escape == "" {
		return name
	}
	return escape + name + pal.Reset
}

// formatModelShort strips the "claude-" prefix for compact display.
// Mirrors internal/commands/status.go:formatModelShort.
func formatModelShort(model string) string {
	if model == "" {
		return "-"
	}
	short := strings.TrimPrefix(model, "claude-")
	if short == "" {
		return model
	}
	return short
}

// truncASCII is a width-aware truncate operating on UTF-8 rune count.
// Appends ".." when truncating if width >= 3.
func truncASCII(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if VisibleLen(s) <= w {
		return s
	}
	if w <= 2 {
		// Truncate to first w runes.
		out := make([]byte, 0, w)
		runes := 0
		for i := 0; i < len(s); {
			c := s[i]
			runeLen := 1
			switch {
			case c < 0x80:
				runeLen = 1
			case c >= 0xf0:
				runeLen = 4
			case c >= 0xe0:
				runeLen = 3
			case c >= 0xc0:
				runeLen = 2
			}
			if i+runeLen > len(s) {
				runeLen = len(s) - i
			}
			if runes >= w {
				break
			}
			out = append(out, s[i:i+runeLen]...)
			runes++
			i += runeLen
		}
		return string(out)
	}
	// Truncate to w-2 + ".."
	cut := Truncate(s, w-2)
	return cut + ".."
}

// padRightAscii pads s on the right with spaces to width w. Assumes s is
// plain (no ANSI). If s is wider than w, returned as-is (caller has
// already truncated).
func padRightAscii(s string, w int) string {
	vis := VisibleLen(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

// padRightVisible pads s on the right with spaces to w VISIBLE columns,
// preserving ANSI sequences in the input.
func padRightVisible(s string, w int) string {
	vis := VisibleLen(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}
