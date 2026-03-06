package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/platform/db"
)

// Eval type styles using the Dracula palette.
// Manual evals (cmdr evals add):
// Hook predicate types (from intent eval system, colors match PREDICATE_NOTIFICATION_COLORS):
var evalTypeStyles = map[string]lipgloss.Style{
	// Manual eval types
	"unit_test":   lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")),
	"integration": lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")),
	"lint":        lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")),
	"build":       lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"custom":      lipgloss.NewStyle().Foreground(lipgloss.Color("#66d9ef")),
	// Hook predicate types
	"semantic_check":       lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"structural_check":     lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"contains_pattern":     lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"count_check":          lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"ast_check":            lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"negation_check":       lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
	"test_execution":       lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")),
	"type_check":           lipgloss.NewStyle().Foreground(lipgloss.Color("#66d9ef")),
	"diff_validation":      lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")),
	"output_pattern_match": lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")),
}

// Status styles.
var (
	evalPassStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Bold(true)
	evalFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Bold(true)
	evalPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))
)

// EvalRow represents a single eval entry from the database.
type EvalRow struct {
	ID          string
	ProjectName string
	AgentTask   string
	EvalType    string
	Command     string
	Passed      *bool  // nil = never run
	ErrorDetail string
	LastRunAt   string
	CreatedAt   string
}

// EvalsPane displays eval definitions and their latest results.
// It follows the EventsPane/GitStatusPane pattern with SetSize() for explicit sizing,
// and uses the database query pattern from MergeQueueView.Refresh().
type EvalsPane struct {
	db            db.DB
	evals         []EvalRow
	theme         *Theme
	width         int
	height        int
	cursor        int
	projectFilter string
}

// NewEvalsPane constructs an EvalsPane.
func NewEvalsPane(database db.DB, theme *Theme) *EvalsPane {
	return &EvalsPane{
		db:    database,
		theme: theme,
	}
}

// SetSize updates display dimensions.
// Note: MailSummary and MergeQueueView do not have SetSize() methods and are not
// sized in updatePaneSizes(). EvalsPane, like EventsPane and GitStatusPane, uses
// explicit sizing for scroll/cursor support.
func (ep *EvalsPane) SetSize(w, h int) {
	ep.width = w
	ep.height = h
}

// Refresh queries the evals table and updates the cached rows.
func (ep *EvalsPane) Refresh() error {
	if ep.db == nil {
		return nil
	}

	ctx := context.Background()
	query := "SELECT id, project_name, agent_task, eval_type, command, passed, error_detail, last_run_at, created_at FROM evals"
	var args []any

	if ep.projectFilter != "" {
		query += " WHERE project_name = $1"
		args = append(args, ep.projectFilter)
	}
	query += " ORDER BY created_at DESC"

	rows, err := ep.db.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("evals refresh: %w", err)
	}
	defer rows.Close()

	var evals []EvalRow
	for rows.Next() {
		var e EvalRow
		var errorDetail, lastRunAt *string
		if err := rows.Scan(&e.ID, &e.ProjectName, &e.AgentTask, &e.EvalType, &e.Command, &e.Passed, &errorDetail, &lastRunAt, &e.CreatedAt); err != nil {
			continue
		}
		if errorDetail != nil {
			e.ErrorDetail = *errorDetail
		}
		if lastRunAt != nil {
			e.LastRunAt = *lastRunAt
		}
		evals = append(evals, e)
	}
	ep.evals = evals
	return nil
}

// View renders the evals pane content.
func (ep *EvalsPane) View() string {
	if len(ep.evals) == 0 {
		return ep.theme.Subtitle.Render("  No evals registered") + "\n\n" +
			evalPendingStyle.Render("  [r] Run All  [a] Add")
	}

	w := ep.width
	if w <= 0 {
		w = 60
	}
	h := ep.height
	if h <= 0 {
		h = 10
	}

	// Reserve 1 line for footer.
	contentH := h - 1
	if contentH < 1 {
		contentH = 1
	}

	var lines []string
	for i, e := range ep.evals {
		if len(lines) >= contentH {
			break
		}

		// Status indicator.
		var status string
		if e.Passed == nil {
			status = evalPendingStyle.Render("---")
		} else if *e.Passed {
			status = evalPassStyle.Render("PASS")
		} else {
			status = evalFailStyle.Render("FAIL")
		}

		// Type with color.
		typeStyle, ok := evalTypeStyles[e.EvalType]
		if !ok {
			typeStyle = evalPendingStyle
		}
		typeName := typeStyle.Render(e.EvalType)

		// Cursor indicator.
		prefix := "  "
		if i == ep.cursor {
			prefix = "> "
		}

		taskStr := truncate(e.AgentTask, 18)
		if taskStr == "" {
			taskStr = truncate(e.ProjectName, 18)
		}

		line := fmt.Sprintf("%s%s %-12s %s", prefix, status, typeName, taskStr)
		if lipgloss.Width(line) > w {
			line = ansiTruncate(line, w)
		}
		lines = append(lines, line)
	}

	// Footer with key hints.
	footer := evalPendingStyle.Render("  [r] Run All  [a] Add")
	lines = append(lines, footer)

	return strings.Join(lines, "\n")
}

// ScrollUp moves the cursor up.
func (ep *EvalsPane) ScrollUp() {
	if ep.cursor > 0 {
		ep.cursor--
	}
}

// ScrollDown moves the cursor down.
func (ep *EvalsPane) ScrollDown() {
	if ep.cursor < len(ep.evals)-1 {
		ep.cursor++
	}
}

// RunAll executes all eval commands sequentially and updates results in the database.
func (ep *EvalsPane) RunAll() error {
	if ep.db == nil {
		return nil
	}
	ctx := context.Background()

	for i := range ep.evals {
		e := &ep.evals[i]

		cmd := exec.CommandContext(ctx, "sh", "-c", e.Command)
		output, err := cmd.CombinedOutput()

		now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		passed := true
		var errorDetail string

		if err != nil {
			passed = false
			errorDetail = strings.TrimSpace(string(output))
			if errorDetail == "" {
				errorDetail = err.Error()
			}
		}

		e.Passed = &passed
		e.ErrorDetail = errorDetail
		e.LastRunAt = now

		if passed {
			_ = ep.db.Exec(ctx,
				"UPDATE evals SET passed = $1, error_detail = NULL, last_run_at = $2 WHERE id = $3",
				true, now, e.ID,
			)
		} else {
			_ = ep.db.Exec(ctx,
				"UPDATE evals SET passed = $1, error_detail = $2, last_run_at = $3 WHERE id = $4",
				false, errorDetail, now, e.ID,
			)
		}
	}

	return nil
}

// EvalCount returns the total number of evals.
func (ep *EvalsPane) EvalCount() int {
	return len(ep.evals)
}

// PassedCount returns the number of evals that have passed.
func (ep *EvalsPane) PassedCount() int {
	count := 0
	for _, e := range ep.evals {
		if e.Passed != nil && *e.Passed {
			count++
		}
	}
	return count
}

// FailedCount returns the number of evals that have failed.
func (ep *EvalsPane) FailedCount() int {
	count := 0
	for _, e := range ep.evals {
		if e.Passed != nil && !*e.Passed {
			count++
		}
	}
	return count
}
