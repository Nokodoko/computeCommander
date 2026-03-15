package tui

import (
	"fmt"
	"strings"
)

// OpenBrainPane is a placeholder data pane for the OpenBrain integration.
// It renders a static "OpenBrain: coming soon" message centered in the pane.
// No DB access. Implements SetSize, Refresh (no-op), and View.
type OpenBrainPane struct {
	theme  *Theme
	width  int
	height int
}

// NewOpenBrainPane constructs an OpenBrainPane.
func NewOpenBrainPane(theme *Theme) *OpenBrainPane {
	return &OpenBrainPane{
		theme: theme,
	}
}

// SetSize updates display dimensions.
func (ob *OpenBrainPane) SetSize(w, h int) {
	ob.width = w
	ob.height = h
}

// Refresh is a no-op for the placeholder pane.
func (ob *OpenBrainPane) Refresh() error {
	return nil
}

// View renders the placeholder content centered in the pane.
func (ob *OpenBrainPane) View() string {
	w := ob.width
	h := ob.height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 10
	}

	msg := "OpenBrain: coming soon"

	// Center horizontally.
	pad := (w - len(msg)) / 2
	if pad < 0 {
		pad = 0
	}
	centered := fmt.Sprintf("%s%s", strings.Repeat(" ", pad), msg)

	// Center vertically.
	var lines []string
	topPad := (h - 1) / 2
	for i := 0; i < topPad; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, ob.theme.Subtitle.Render(centered))
	for len(lines) < h {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
