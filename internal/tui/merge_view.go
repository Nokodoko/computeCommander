package tui

import (
	"fmt"
	"strings"

	"github.com/noko/computecommander/internal/merge"
)

// MergeQueueView renders the merge queue status.
type MergeQueueView struct {
	queue         merge.MergeQueue
	entries       []*merge.MergeEntry
	theme         *Theme
	colorResolver AgentColorResolver
	width         int
	height        int
}

// NewMergeQueueView constructs a MergeQueueView.
func NewMergeQueueView(queue merge.MergeQueue, theme *Theme) *MergeQueueView {
	return &MergeQueueView{
		queue: queue,
		theme: theme,
	}
}

// SetSize updates display dimensions.
func (v *MergeQueueView) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// SetColorResolver sets the function used to resolve agent colors for agent names.
func (v *MergeQueueView) SetColorResolver(resolver AgentColorResolver) {
	v.colorResolver = resolver
}

// Refresh fetches the latest queue entries.
func (v *MergeQueueView) Refresh() error {
	entries, err := v.queue.List(merge.ListOpts{})
	if err != nil {
		return fmt.Errorf("merge queue refresh: %w", err)
	}
	v.entries = entries
	return nil
}

// Entries returns the current queue snapshot.
func (v *MergeQueueView) Entries() []*merge.MergeEntry {
	return v.entries
}

// PendingCount returns the number of entries in pending state.
func (v *MergeQueueView) PendingCount() int {
	count := 0
	for _, e := range v.entries {
		if e.Status == merge.MergePending {
			count++
		}
	}
	return count
}

// View renders the merge queue as a string.
func (v *MergeQueueView) View() string {
	var b strings.Builder

	title := fmt.Sprintf("Merge Queue (%d entries)", len(v.entries))
	b.WriteString(v.theme.Title.Render(title))
	b.WriteString("\n")

	if len(v.entries) == 0 {
		b.WriteString(v.theme.Subtitle.Render("  Queue empty"))
		return b.String()
	}

	cols := []column{
		{Header: "Branch", Width: 24},
		{Header: "Agent", Width: 12},
		{Header: "Status", Width: 10},
		{Header: "Files", Width: 6},
	}

	var rows [][]string
	for _, e := range v.entries {
		statusStr := v.renderMergeStatus(e.Status)
		fileCount := fmt.Sprintf("%d", len(e.FilesModified))

		// Color the agent name using their assigned palette color.
		agentName := truncate(e.AgentName, cols[1].Width)
		if v.colorResolver != nil {
			if hex := v.colorResolver(e.AgentName); hex != "" {
				agentName = AgentColorStyle(hex).Render(agentName)
			}
		}

		rows = append(rows, []string{
			truncate(e.BranchName, cols[0].Width),
			agentName,
			statusStr,
			fileCount,
		})
	}

	b.WriteString(renderTable(cols, rows, v.theme))
	return b.String()
}

// CompactView returns a one-line summary for the status bar.
func (v *MergeQueueView) CompactView() string {
	pending := v.PendingCount()
	return fmt.Sprintf("Merge Queue: %d pending", pending)
}

// renderMergeStatus applies color-coded styling to merge status.
func (v *MergeQueueView) renderMergeStatus(status merge.MergeStatus) string {
	switch status {
	case merge.MergePending:
		return v.theme.MergePending.Render(string(status))
	case merge.MergeMerging:
		return v.theme.MergeMerging.Render(string(status))
	case merge.MergeMerged:
		return v.theme.MergeMerged.Render(string(status))
	case merge.MergeConflict:
		return v.theme.MergeConflict.Render(string(status))
	case merge.MergeFailed:
		return v.theme.MergeFailed.Render(string(status))
	default:
		return string(status)
	}
}
