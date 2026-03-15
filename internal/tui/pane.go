package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PaneID identifies a pane in the dashboard layout.
type PaneID int

const (
	PaneFilePicker   PaneID = iota // 0 - PTY: fp process
	PaneAgentSession               // 1 - PTY: claude agent
	PaneAgents                     // 2 - Data: agent table from DB
	PaneEvents                     // 3 - Data: event feed from DB
	PaneEvals                      // 4 - Data: evals from DB
	PaneMergeQueue                 // 5 - Data: merge queue from DB
	PaneJira                       // 6 - Data: Jira API cache
	PaneOpenBrain                  // 7 - Data: OpenBrain status (placeholder)
	PaneLazyGit                    // 8 - PTY: lazygit process
)

// PaneMeta holds display metadata for a pane.
type PaneMeta struct {
	ID    PaneID
	Title string
	// FocusKey is the shortcut shown in the help bar.
	FocusKey string
}

// AllPanes returns metadata for every dashboard pane in display order.
// Keys 1-7 map to grid panes in order, 9=Jira (full-screen overlay), 0=LazyGit.
// Key 8 is intentionally unassigned (PaneMail and PaneGitStatus removed from grid).
func AllPanes() []PaneMeta {
	return []PaneMeta{
		{ID: PaneFilePicker, Title: "File Picker", FocusKey: "1"},
		{ID: PaneAgentSession, Title: "Agent Session", FocusKey: "2"},
		{ID: PaneAgents, Title: "Agents", FocusKey: "3"},
		{ID: PaneEvents, Title: "Events", FocusKey: "4"},
		{ID: PaneEvals, Title: "Evals", FocusKey: "5"},
		{ID: PaneMergeQueue, Title: "Merge Queue", FocusKey: "6"},
		{ID: PaneOpenBrain, Title: "OpenBrain", FocusKey: "7"},
		{ID: PaneJira, Title: "Jira", FocusKey: "9"},
		{ID: PaneLazyGit, Title: "LazyGit", FocusKey: "0"},
	}
}

// paneOrder defines the tab-cycling order for pane navigation.
// Follows the grid layout: top row L-R, then bottom row L-R, then overlays.
var paneOrder = []PaneID{
	PaneFilePicker,
	PaneAgentSession,
	PaneAgents,
	PaneEvents,
	PaneEvals,
	PaneMergeQueue,
	PaneOpenBrain,
	PaneLazyGit,
	PaneJira,
}

// nextPane returns the next pane in tab order.
func nextPane(current PaneID) PaneID {
	for i, p := range paneOrder {
		if p == current {
			return paneOrder[(i+1)%len(paneOrder)]
		}
	}
	return paneOrder[0]
}

// prevPane returns the previous pane in tab order.
func prevPane(current PaneID) PaneID {
	for i, p := range paneOrder {
		if p == current {
			idx := (i - 1 + len(paneOrder)) % len(paneOrder)
			return paneOrder[idx]
		}
	}
	return paneOrder[0]
}

// paneMetaByID returns the PaneMeta for the given PaneID.
func paneMetaByID(id PaneID) PaneMeta {
	for _, p := range AllPanes() {
		if p.ID == id {
			return p
		}
	}
	return PaneMeta{ID: id, Title: "Unknown"}
}

// ansiTruncate truncates a string to maxWidth visible characters, preserving
// ANSI escape sequences. It walks the string rune by rune, skipping over
// escape sequences when counting width, and appends a final SGR reset if
// any SGR codes were active.
func ansiTruncate(s string, maxWidth int) string {
	var b strings.Builder
	width := 0
	inEsc := false
	runes := []rune(s)
	hasSGR := false

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inEsc {
			b.WriteRune(r)
			// CSI sequences end at a letter in 0x40-0x7E range.
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
				if r == 'm' {
					hasSGR = true
				}
			}
			continue
		}

		if r == '\033' {
			// Start of an escape sequence — always include it.
			b.WriteRune(r)
			inEsc = true
			continue
		}

		// Normal visible character.
		width++
		if width > maxWidth {
			break
		}
		b.WriteRune(r)
	}

	// Reset SGR if we had any active styling.
	if hasSGR {
		b.WriteString("\033[0m")
	}

	return b.String()
}

// RenderPane wraps content in a bordered frame with a title.
// The focused parameter controls the border and title styling.
func RenderPane(content string, meta PaneMeta, focused bool, width, height int, theme *Theme) string {
	// Select border and title styles based on focus.
	var borderStyle lipgloss.Style
	var titleStyle lipgloss.Style
	if focused {
		borderStyle = theme.FocusedBorder
		titleStyle = theme.PaneTitleFocused
	} else {
		borderStyle = theme.UnfocusedBorder
		titleStyle = theme.PaneTitle
	}

	// Compute inner dimensions (subtract border + padding).
	innerW := width - 2 // border left + right
	innerH := height - 2 // border top + bottom
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}

	// Build title with focus key hint.
	title := titleStyle.Render(fmt.Sprintf("[%s] %s", meta.FocusKey, meta.Title))

	// Pad or truncate content to fill the inner area.
	lines := strings.Split(content, "\n")

	// Reserve first line for title.
	innerH--
	if innerH < 0 {
		innerH = 0
	}

	// Truncate or pad lines to fit.
	var paddedLines []string
	paddedLines = append(paddedLines, title)
	for i := 0; i < innerH; i++ {
		if i < len(lines) {
			line := lines[i]
			// Use ANSI-aware width measurement so escape sequences
			// don't count toward the visible width.
			if lipgloss.Width(line) > innerW {
				line = ansiTruncate(line, innerW)
			}
			paddedLines = append(paddedLines, line)
		} else {
			paddedLines = append(paddedLines, "")
		}
	}

	inner := strings.Join(paddedLines, "\n")

	return borderStyle.
		Width(width - 2).
		Height(height - 2).
		Render(inner)
}
