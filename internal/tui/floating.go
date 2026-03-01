package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FloatingPane renders content in a floating overlay style.
// When running in the zellij-managed layout, actual floating panes are spawned
// via the zellij CLI. This renderer handles the in-process TUI fallback.
type FloatingPane struct {
	title   string
	content string
	width   int
	height  int
	theme   *Theme
	visible bool
}

// NewFloatingPane creates a new floating pane renderer.
func NewFloatingPane(title string, theme *Theme) *FloatingPane {
	return &FloatingPane{
		title: title,
		theme: theme,
	}
}

// Show makes the floating pane visible with the given content.
func (f *FloatingPane) Show(content string) {
	f.content = content
	f.visible = true
}

// Hide dismisses the floating pane.
func (f *FloatingPane) Hide() {
	f.visible = false
	f.content = ""
}

// IsVisible returns whether the pane is currently shown.
func (f *FloatingPane) IsVisible() bool {
	return f.visible
}

// SetSize updates the pane dimensions.
func (f *FloatingPane) SetSize(width, height int) {
	f.width = width
	f.height = height
}

// View renders the floating pane overlay.
func (f *FloatingPane) View() string {
	if !f.visible {
		return ""
	}

	paneWidth := f.width * 60 / 100
	if paneWidth < 40 {
		paneWidth = 40
	}
	paneHeight := f.height * 60 / 100
	if paneHeight < 10 {
		paneHeight = 10
	}

	// Title bar.
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FFFF")).
		Background(lipgloss.Color("#333333")).
		Width(paneWidth).
		Align(lipgloss.Center)

	// Content area.
	contentStyle := lipgloss.NewStyle().
		Width(paneWidth).
		Height(paneHeight - 3). // Leave room for title and footer.
		Padding(1, 2)

	// Footer.
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Width(paneWidth).
		Align(lipgloss.Center)

	// Border wrapper.
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Width(paneWidth + 2).
		Height(paneHeight)

	title := titleStyle.Render(fmt.Sprintf(" %s ", f.title))
	content := contentStyle.Render(f.content)
	footer := footerStyle.Render("[Esc to close]")

	inner := lipgloss.JoinVertical(lipgloss.Left, title, content, footer)
	return borderStyle.Render(inner)
}

// RenderHelpContent generates the help text for the floating help pane.
func RenderHelpContent() string {
	return `ComputeCommander (cmdr) - Agentic IDE

LEADER KEY: Ctrl+Space
  ?  Help           v  Version
  u  Update         s  Shell
  c  Clear logs     e  Export
  r  Restart        b  Backup
  R  Restore        f  Feedback
  h  Support        t  Theme
  n  Notifications  a  Analytics
  i  Integrations   m  Automation
  d  File picker    q  Quit
  A  Accessibility  p  Plugins

NAVIGATION:
  Tab     Cycle bottom panes
  j/k     Move cursor up/down
  1-4     Switch bottom pane view`
}

// RenderVersionContent generates the version display content.
func RenderVersionContent(version string) string {
	releaseURL := fmt.Sprintf("https://github.com/noko/computecommander/releases/tag/v%s", version)
	return fmt.Sprintf("cmdr version %s\n\nRelease notes:\n%s", version, releaseURL)
}

// RenderConfirmContent generates a confirmation dialog content.
func RenderConfirmContent(action string) string {
	return fmt.Sprintf("Are you sure you want to %s?\n\n  [y] Yes    [n/Esc] Cancel", action)
}

// RenderExportPreview generates an export preview.
func RenderExportPreview(tables []string, totalRows int) string {
	var sb strings.Builder
	sb.WriteString("Export Preview\n\n")
	sb.WriteString(fmt.Sprintf("Tables: %s\n", strings.Join(tables, ", ")))
	sb.WriteString(fmt.Sprintf("Total rows: %d\n\n", totalRows))
	sb.WriteString("Press Enter to export, Esc to cancel")
	return sb.String()
}
