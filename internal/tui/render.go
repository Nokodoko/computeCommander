package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// column describes one column in a rendered table.
type column struct {
	Header string
	Width  int
}

// renderTable draws a bordered table from columns and row data.
// Each row is a slice of strings whose length must match columns.
func renderTable(cols []column, rows [][]string, theme *Theme) string {
	if len(cols) == 0 {
		return ""
	}

	// Build header line.
	var headerCells []string
	for _, c := range cols {
		headerCells = append(headerCells, theme.TableHeader.
			Width(c.Width).
			Render(c.Header))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Top, headerCells...)

	// Separator.
	totalWidth := 0
	for _, c := range cols {
		totalWidth += c.Width + 2 // padding
	}
	sep := strings.Repeat("─", totalWidth)

	// Build rows.
	var renderedRows []string
	renderedRows = append(renderedRows, header)
	renderedRows = append(renderedRows, sep)

	for _, row := range rows {
		var cells []string
		for i, c := range cols {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			cells = append(cells, theme.TableRow.
				Width(c.Width).
				Render(val))
		}
		renderedRows = append(renderedRows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	return strings.Join(renderedRows, "\n")
}

// renderStatusBar builds the bottom status bar with summary statistics.
func renderStatusBar(agentCount int, unreadMail int, mergeQueueLen int, totalCost float64, theme *Theme) string {
	parts := []string{
		fmt.Sprintf("Agents: %d", agentCount),
		fmt.Sprintf("Mail: %d unread", unreadMail),
		fmt.Sprintf("Merge Queue: %d pending", mergeQueueLen),
		fmt.Sprintf("Cost: $%.2f", totalCost),
	}
	return theme.StatusBar.Render(strings.Join(parts, "  |  "))
}

// renderHelpBar renders the keybinding hints at the bottom of the dashboard.
func renderHelpBar(theme *Theme) string {
	return theme.HelpBar.Render("Tab: cycle  1-7: jump  Ctrl+K: palette  q: quit")
}

// truncate shortens a string to maxLen, appending ".." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}

// formatTokens renders a token count in human-friendly form (e.g. "12.5k").
func formatTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000.0)
}
