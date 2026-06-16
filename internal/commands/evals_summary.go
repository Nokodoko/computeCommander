package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentui"
	"github.com/noko/computecommander/internal/platform/db"
)

// EvalsSummaryCmd returns the "evals-summary" subcommand: a single-shot,
// fixed-shape, optionally-ANSI-colored evals pass/fail summary suitable
// for embedding next to OB1 / TG / agents frames. Mirrors TGSummaryCmd.
//
// Reads the `evals` SQLite table sorted by COALESCE(last_run_at,
// created_at) DESC (matches the existing runEvalsPane sort) via the
// dbEvalSource adapter, which satisfies agentui.EvalSource.
//
// Honours the sessionbanner consumer contract (SPEC phase3.md):
//   - exit code 0 on all failure paths
//   - exactly --lines lines
//   - <= --width visible cols
//   - --no-color emits clean ASCII
func EvalsSummaryCmd(app *App) *cobra.Command {
	var (
		lines   int
		width   int
		noColor bool
		project string
	)
	cmd := &cobra.Command{
		Use:     "evals-summary",
		Short:   "Emit a fixed-shape, embeddable evals pass/fail summary",
		GroupID: "OBSERVABILITY",
		Long: `Single-shot evals summary sized to embed in a ~5-8 line ASCII frame.
Honours NO_COLOR per https://no-color.org. Exits 0 on every failure
path with a single-line degraded marker ("evals: unavailable" on DB
error, "evals: no data" on empty registry) padded to --lines so the
embedding frame size does not shift between renders.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvalsSummary(cmd, app, lines, width, noColor, project)
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 8, "total output lines including header and trailer")
	cmd.Flags().IntVar(&width, "width", 60, "inner width hint, used for column truncation")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "suppress all ANSI colour codes (also honours $NO_COLOR)")
	cmd.Flags().StringVar(&project, "project", "", "limit to a single project name")
	return cmd
}

func runEvalsSummary(cmd *cobra.Command, app *App, lines, width int, noColor bool, project string) error {
	if !noColor {
		if v := os.Getenv("NO_COLOR"); v != "" {
			noColor = true
		}
	}

	var src agentui.EvalSource
	if app != nil && app.DB != nil {
		src = &dbEvalSource{db: app.DB}
	}

	out := agentui.RenderEvals(cmd.Context(), src, agentui.EvalsOpts{
		Lines:   lines,
		Width:   width,
		NoColor: noColor,
		Now:     time.Now(),
		Project: project,
	})
	for _, ln := range out {
		fmt.Fprintln(os.Stdout, ln)
	}
	return nil
}

// dbEvalSource adapts a db.DB to the agentui.EvalSource contract. The
// renderer is the consumer; this adapter is the only place the SQL lives.
//
// Sort matches internal/commands/evals.go:runEvalsPane line 523:
// ORDER BY COALESCE(last_run_at, created_at) DESC.
type dbEvalSource struct {
	db db.DB
}

func (s *dbEvalSource) ListEvals(ctx context.Context, projectFilter string) ([]agentui.EvalRecord, error) {
	query := "SELECT id, project_name, agent_task, eval_type, passed, error_detail, last_run_at, created_at FROM evals"
	var args []any
	if projectFilter != "" {
		query += " WHERE project_name = $1"
		args = append(args, projectFilter)
	}
	query += " ORDER BY COALESCE(last_run_at, created_at) DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []agentui.EvalRecord
	for rows.Next() {
		var (
			id, project, task, evalType   string
			passed                        *bool
			errorDetail, lastRunAt        *string
			createdAt                     string
		)
		if err := rows.Scan(&id, &project, &task, &evalType, &passed, &errorDetail, &lastRunAt, &createdAt); err != nil {
			continue
		}
		rec := agentui.EvalRecord{
			ID:          id,
			ProjectName: project,
			AgentTask:   task,
			EvalType:    evalType,
			Passed:      passed,
			CreatedAt:   createdAt,
		}
		if errorDetail != nil {
			rec.ErrorDetail = *errorDetail
		}
		if lastRunAt != nil {
			rec.LastRunAt = *lastRunAt
		}
		out = append(out, rec)
	}
	return out, nil
}
