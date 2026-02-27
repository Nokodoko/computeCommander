package tui

import (
	"fmt"
	"strings"
)

// CostEntry holds token usage and cost for a single capability/model pair.
type CostEntry struct {
	Capability   string
	Model        string
	InputTokens  int64
	OutputTokens int64
	Cost         float64
}

// CostTracker aggregates and displays token usage and cost data.
type CostTracker struct {
	entries    []CostEntry
	totalCost  float64
	theme      *Theme
}

// NewCostTracker constructs a CostTracker.
func NewCostTracker(theme *Theme) *CostTracker {
	return &CostTracker{
		theme: theme,
	}
}

// Update replaces the cost entries with fresh data.
func (c *CostTracker) Update(entries []CostEntry) {
	c.entries = entries
	c.totalCost = 0
	for _, e := range entries {
		c.totalCost += e.Cost
	}
}

// TotalCost returns the aggregate cost across all entries.
func (c *CostTracker) TotalCost() float64 {
	return c.totalCost
}

// TotalTokens returns total input and output tokens.
func (c *CostTracker) TotalTokens() (input int64, output int64) {
	for _, e := range c.entries {
		input += e.InputTokens
		output += e.OutputTokens
	}
	return
}

// View renders the cost breakdown as a string.
func (c *CostTracker) View() string {
	var b strings.Builder

	totalIn, totalOut := c.TotalTokens()
	title := fmt.Sprintf("Costs ($%.2f total, %s in / %s out)",
		c.totalCost, formatTokens(totalIn), formatTokens(totalOut))
	b.WriteString(c.theme.Title.Render(title))
	b.WriteString("\n")

	if len(c.entries) == 0 {
		b.WriteString(c.theme.Subtitle.Render("  No cost data"))
		return b.String()
	}

	cols := []column{
		{Header: "Capability", Width: 12},
		{Header: "Model", Width: 18},
		{Header: "Input", Width: 10},
		{Header: "Output", Width: 10},
		{Header: "Cost", Width: 10},
	}

	var rows [][]string
	for _, e := range c.entries {
		rows = append(rows, []string{
			truncate(e.Capability, cols[0].Width),
			truncate(e.Model, cols[1].Width),
			formatTokens(e.InputTokens),
			formatTokens(e.OutputTokens),
			fmt.Sprintf("$%.2f", e.Cost),
		})
	}

	b.WriteString(renderTable(cols, rows, c.theme))
	return b.String()
}

// CompactView returns a one-line cost summary for the status bar.
func (c *CostTracker) CompactView() string {
	return fmt.Sprintf("Cost: $%.2f", c.totalCost)
}
