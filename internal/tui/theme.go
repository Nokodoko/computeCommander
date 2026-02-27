// Package tui provides the ComputeCommander TUI dashboard built on
// charmbracelet/bubbletea and charmbracelet/lipgloss.
package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette and styles used across the dashboard.
type Theme struct {
	Title       lipgloss.Style
	Subtitle    lipgloss.Style
	Border      lipgloss.Style
	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	StatusBar   lipgloss.Style
	HelpBar     lipgloss.Style

	// State colours for agent status rendering.
	StateWorking   lipgloss.Style
	StateBooting   lipgloss.Style
	StateStalled   lipgloss.Style
	StateZombie    lipgloss.Style
	StateCompleted lipgloss.Style

	// Merge status colours.
	MergePending  lipgloss.Style
	MergeMerging  lipgloss.Style
	MergeMerged   lipgloss.Style
	MergeConflict lipgloss.Style
	MergeFailed   lipgloss.Style
}

// DefaultTheme returns the standard ComputeCommander colour scheme.
func DefaultTheme() *Theme {
	green := lipgloss.Color("#00FF00")
	yellow := lipgloss.Color("#FFFF00")
	red := lipgloss.Color("#FF0000")
	cyan := lipgloss.Color("#00FFFF")
	white := lipgloss.Color("#FFFFFF")
	gray := lipgloss.Color("#808080")
	magenta := lipgloss.Color("#FF00FF")

	return &Theme{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(cyan).
			Padding(0, 1),
		Subtitle: lipgloss.NewStyle().
			Foreground(gray),
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyan).
			Padding(0, 1),
		TableHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(white).
			Padding(0, 1),
		TableRow: lipgloss.NewStyle().
			Foreground(white).
			Padding(0, 1),
		StatusBar: lipgloss.NewStyle().
			Foreground(white).
			Background(lipgloss.Color("#333333")).
			Padding(0, 1),
		HelpBar: lipgloss.NewStyle().
			Foreground(gray).
			Padding(0, 1),
		StateWorking: lipgloss.NewStyle().
			Foreground(green).
			Bold(true),
		StateBooting: lipgloss.NewStyle().
			Foreground(cyan),
		StateStalled: lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true),
		StateZombie: lipgloss.NewStyle().
			Foreground(red).
			Bold(true),
		StateCompleted: lipgloss.NewStyle().
			Foreground(gray),
		MergePending: lipgloss.NewStyle().
			Foreground(yellow),
		MergeMerging: lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true),
		MergeMerged: lipgloss.NewStyle().
			Foreground(green),
		MergeConflict: lipgloss.NewStyle().
			Foreground(magenta).
			Bold(true),
		MergeFailed: lipgloss.NewStyle().
			Foreground(red).
			Bold(true),
	}
}
