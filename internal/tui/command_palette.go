package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PaletteCommand represents a command entry in the command palette.
type PaletteCommand struct {
	Name        string
	Description string
	Category    string
	Action      func() // nil for display-only entries
}

// CommandPalette is an overlay with fuzzy-finding over available commands.
type CommandPalette struct {
	visible   bool
	query     string
	cursor    int
	commands  []PaletteCommand
	filtered  []PaletteCommand
	theme     *Theme
	width     int
	height    int
}

// NewCommandPalette constructs a CommandPalette with the default command set.
func NewCommandPalette(theme *Theme) *CommandPalette {
	cmds := defaultCommands()
	return &CommandPalette{
		commands: cmds,
		filtered: cmds,
		theme:    theme,
	}
}

// defaultCommands returns the built-in command palette entries.
func defaultCommands() []PaletteCommand {
	return []PaletteCommand{
		// Agent management actions.
		{Name: "kill", Description: "Kill the current agent session", Category: "Agent"},
		{Name: "background", Description: "Background the current agent session", Category: "Agent"},
		{Name: "prompt", Description: "Send a prompt to the current agent", Category: "Agent"},
		{Name: "restart", Description: "Restart the current agent session", Category: "Agent"},

		// Core commands.
		{Name: "agents", Description: "View and manage active agents", Category: "Core"},
		{Name: "sling", Description: "Spawn a new agent with a task", Category: "Core"},
		{Name: "stop", Description: "Stop a running agent", Category: "Core"},
		{Name: "nudge", Description: "Send a nudge to a stalled agent", Category: "Coordination"},
		{Name: "mail", Description: "View agent mail / messages", Category: "Messaging"},
		{Name: "merge", Description: "View and manage merge queue", Category: "Merge"},
		{Name: "inspect", Description: "Deep-inspect an agent session", Category: "Observability"},
		{Name: "costs", Description: "View token usage and cost breakdown", Category: "Observability"},
		{Name: "logs", Description: "View system logs", Category: "Observability"},
		{Name: "feed", Description: "Live activity feed", Category: "Observability"},
		{Name: "doctor", Description: "Run diagnostics on the project", Category: "Infrastructure"},
		{Name: "config", Description: "View or edit configuration", Category: "Infrastructure"},
		{Name: "worktree", Description: "Manage agent worktrees", Category: "Infrastructure"},
		{Name: "clean", Description: "Clean up stale resources", Category: "Infrastructure"},
	}
}

// Toggle flips visibility of the command palette.
func (cp *CommandPalette) Toggle() {
	cp.visible = !cp.visible
	if cp.visible {
		cp.query = ""
		cp.cursor = 0
		cp.filtered = cp.commands
	}
}

// Open makes the palette visible and resets state.
func (cp *CommandPalette) Open() {
	cp.visible = true
	cp.query = ""
	cp.cursor = 0
	cp.filtered = cp.commands
}

// Close hides the palette.
func (cp *CommandPalette) Close() {
	cp.visible = false
}

// Visible returns whether the palette is shown.
func (cp *CommandPalette) Visible() bool {
	return cp.visible
}

// SetSize updates the overlay dimensions.
func (cp *CommandPalette) SetSize(w, h int) {
	cp.width = w
	cp.height = h
}

// TypeChar appends a character to the query and re-filters.
func (cp *CommandPalette) TypeChar(ch rune) {
	cp.query += string(ch)
	cp.applyFilter()
}

// Backspace removes the last character from the query.
func (cp *CommandPalette) Backspace() {
	if len(cp.query) > 0 {
		cp.query = cp.query[:len(cp.query)-1]
		cp.applyFilter()
	}
}

// CursorUp moves the selection up.
func (cp *CommandPalette) CursorUp() {
	if cp.cursor > 0 {
		cp.cursor--
	}
}

// CursorDown moves the selection down.
func (cp *CommandPalette) CursorDown() {
	if cp.cursor < len(cp.filtered)-1 {
		cp.cursor++
	}
}

// Selected returns the currently highlighted command, or nil.
func (cp *CommandPalette) Selected() *PaletteCommand {
	if cp.cursor >= 0 && cp.cursor < len(cp.filtered) {
		cmd := cp.filtered[cp.cursor]
		return &cmd
	}
	return nil
}

// applyFilter performs case-insensitive substring matching on the query.
func (cp *CommandPalette) applyFilter() {
	if cp.query == "" {
		cp.filtered = cp.commands
		cp.cursor = 0
		return
	}

	q := strings.ToLower(cp.query)
	var results []PaletteCommand
	for _, cmd := range cp.commands {
		name := strings.ToLower(cmd.Name)
		desc := strings.ToLower(cmd.Description)
		if fuzzyMatch(q, name) || strings.Contains(desc, q) {
			results = append(results, cmd)
		}
	}
	cp.filtered = results
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = max(0, len(cp.filtered)-1)
	}
}

// fuzzyMatch checks if all characters in pattern appear in order in target.
func fuzzyMatch(pattern, target string) bool {
	pi := 0
	for ti := 0; ti < len(target) && pi < len(pattern); ti++ {
		if target[ti] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// View renders the command palette overlay.
func (cp *CommandPalette) View() string {
	if !cp.visible {
		return ""
	}

	w := cp.width
	if w < 40 {
		w = 80
	}
	paletteW := min(w-4, 80)
	listH := min(len(cp.filtered), 12)

	// Input line
	inputStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#333333")).
		Padding(0, 1).
		Width(paletteW - 2)

	prompt := "> " + cp.query + "_"
	inputLine := inputStyle.Render(prompt)

	// Command list
	var listLines []string
	startIdx := 0
	if cp.cursor >= listH {
		startIdx = cp.cursor - listH + 1
	}

	for i := startIdx; i < len(cp.filtered) && i < startIdx+listH; i++ {
		cmd := cp.filtered[i]
		prefix := "  "
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC")).
			Width(paletteW - 2)

		if i == cp.cursor {
			prefix = "> "
			style = style.
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#444466")).
				Bold(true)
		}

		line := prefix + cmd.Name
		// Pad name and add description
		namePad := 16
		if len(cmd.Name)+2 < namePad {
			line += strings.Repeat(" ", namePad-len(cmd.Name)-2)
		} else {
			line += " "
		}
		line += cmd.Description

		listLines = append(listLines, style.Render(truncate(line, paletteW-2)))
	}

	list := strings.Join(listLines, "\n")

	// Preview pane for selected command
	var preview string
	if sel := cp.Selected(); sel != nil {
		previewStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Padding(0, 1).
			Width(paletteW - 2)
		preview = previewStyle.Render("[" + sel.Category + "] " + sel.Description)
	}

	// Help line
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Padding(0, 1)
	help := helpStyle.Render("Enter: select  Esc: close  Up/Down: navigate")

	// Compose palette content
	content := lipgloss.JoinVertical(lipgloss.Left,
		inputLine,
		list,
		preview,
		help,
	)

	// Wrap in a border
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7B61FF")).
		Padding(0, 1).
		Width(paletteW)

	return borderStyle.Render(content)
}

