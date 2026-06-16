package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentui"
)

// AgentsSummaryCmd returns the "agents-summary" subcommand: a single-shot,
// fixed-shape, optionally-ANSI-colored agent fleet summary suitable for
// embedding in a small ASCII frame next to the existing OB1 / TG status
// frames. Mirrors the shape of TGSummaryCmd in internal/commands/tg_summary.go.
//
// Reads `app.Spawner.ListSessions` + `app.Spawner.BuildColorResolver` via
// the agentui.AgentLister interface so the renderer stays decoupled from
// the live App.
//
// The command MUST honour these contracts (see SPEC/CMDR_TO_AGENT_UI_MIGRATION/phase3.md):
//   - exit code 0 on ALL failure paths
//   - exactly --lines lines on stdout
//   - every line <= --width visible columns
//   - --no-color emits zero ANSI escape bytes
//   - latency budget < 300ms per invocation
func AgentsSummaryCmd(app *App) *cobra.Command {
	var (
		lines   int
		width   int
		noColor bool
	)
	cmd := &cobra.Command{
		Use:     "agents-summary",
		Short:   "Emit a fixed-shape, embeddable agent fleet summary",
		GroupID: "OBSERVABILITY",
		Long: `Single-shot summary of active agent sessions in the cmdr project DB,
sized to embed in a ~5-8 line ASCII frame. Honours NO_COLOR per
https://no-color.org. Exits 0 on every failure path with a single-line
degraded marker ("agents: unavailable") padded to --lines so the
embedding frame size does not shift between renders.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentsSummary(cmd, app, lines, width, noColor)
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 8, "total output lines including header and trailer")
	cmd.Flags().IntVar(&width, "width", 60, "inner width hint, used for column truncation")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "suppress all ANSI colour codes (also honours $NO_COLOR)")
	return cmd
}

// runAgentsSummary is the AgentsSummaryCmd RunE body. Single-shot, no
// ticker, no fsnotify, no orphan checker.
//
// Returns nil on every failure path so cobra exits 0. The renderer
// degrades to "agents: unavailable" instead.
func runAgentsSummary(cmd *cobra.Command, app *App, lines, width int, noColor bool) error {
	// Honour $NO_COLOR alongside --no-color (mirrors tg_summary.go behaviour).
	if !noColor {
		if v := os.Getenv("NO_COLOR"); v != "" {
			noColor = true
		}
	}

	var lister agentui.AgentLister
	if app != nil && app.Spawner != nil {
		lister = app.Spawner
	}

	out := agentui.RenderAgents(cmd.Context(), lister, agentui.AgentsOpts{
		Lines:   lines,
		Width:   width,
		NoColor: noColor,
		Now:     time.Now(),
	})
	for _, ln := range out {
		fmt.Fprintln(os.Stdout, ln)
	}
	return nil
}
