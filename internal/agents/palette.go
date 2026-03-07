package agents

// AgentColor defines a named color in the agent palette.
type AgentColor struct {
	Index int
	Name  string
	Hex   string
}

// AgentPalette is the 12-color palette for agent identification.
// Colors are assigned round-robin by spawn order within a run.
var AgentPalette = [12]AgentColor{
	{0, "Coral", "#FF6B6B"},
	{1, "Teal", "#4ECDC4"},
	{2, "Amber", "#FFB347"},
	{3, "Violet", "#9B59B6"},
	{4, "Sky", "#5DADE2"},
	{5, "Lime", "#82E0AA"},
	{6, "Rose", "#F1948A"},
	{7, "Indigo", "#7B68EE"},
	{8, "Peach", "#FFDAB9"},
	{9, "Mint", "#98FB98"},
	{10, "Salmon", "#FA8072"},
	{11, "Lavender", "#E6E6FA"},
}

// CompletedGoldHex is the universal override color for completed agents.
const CompletedGoldHex = "#FFD700"

// DefaultGrayHex is the default color for migrated or unassigned agents.
const DefaultGrayHex = "#808080"

// PaletteSize is the number of colors in the agent palette.
const PaletteSize = 12

// AssignColor returns the palette color for the Nth agent in a run.
// Uses round-robin: agent N gets palette index N % 12.
func AssignColor(spawnIndex int) AgentColor {
	return AgentPalette[spawnIndex%PaletteSize]
}

// ColorForState returns the display hex for an agent given its assigned color
// and current state. Completed agents always render as gold.
func ColorForState(assignedHex string, state SessionState) string {
	if state == StateCompleted {
		return CompletedGoldHex
	}
	return assignedHex
}
