package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/agents"
)

// AgentColorStyle returns a lipgloss.Style with the foreground set to the
// given agent color hex. This is the primary way TUI components colorize
// agent names and related text.
func AgentColorStyle(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex))
}

// AgentColorBoldStyle returns a bold lipgloss.Style for the given hex color.
func AgentColorBoldStyle(hex string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Bold(true)
}

// PaletteStyles holds pre-built lipgloss styles for each of the 12 palette colors.
var PaletteStyles [agents.PaletteSize]lipgloss.Style

// CompletedGoldStyle is the universal override style for completed agents.
var CompletedGoldStyle lipgloss.Style

func init() {
	for i, c := range agents.AgentPalette {
		PaletteStyles[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex))
	}
	CompletedGoldStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(agents.CompletedGoldHex)).Bold(true)
}

// StyleForAgent returns the appropriate lipgloss.Style for an agent session,
// applying the gold override for completed agents.
func StyleForAgent(session *agents.AgentSession) lipgloss.Style {
	if session.State == agents.StateCompleted {
		return CompletedGoldStyle
	}
	return AgentColorStyle(session.ColorHex)
}

// StyleForAgentState returns the style based on color hex and state,
// applying gold override for completed state.
func StyleForAgentState(colorHex string, state agents.SessionState) lipgloss.Style {
	if state == agents.StateCompleted {
		return CompletedGoldStyle
	}
	return AgentColorStyle(colorHex)
}
