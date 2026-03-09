package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// TraceCmd returns the "trace" command for event timeline viewing.
// It also incorporates the agentic causal traceability subcommands
// (list, show, export, prune) as subcommands.
func TraceCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "trace",
		Short:   "Event timeline and causal traceability",
		Long:    "Display the event timeline for a run or agent session.\nSubcommands provide causal traceability: list, show, export, prune.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName, _ := cmd.Flags().GetString("agent")
			runID, _ := cmd.Flags().GetString("run")
			limit, _ := cmd.Flags().GetInt("limit")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			events, err := queryEvents(cmd.Context(), app, agentName, runID, limit)
			if err != nil {
				return fmt.Errorf("query events: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(events)
			}

			if len(events) == 0 {
				fmt.Println("No events found.")
				return nil
			}

			fmt.Printf("%-20s %-14s %-14s %-12s %s\n", "TIME", "AGENT", "TYPE", "TOOL", "DATA")
			for _, e := range events {
				fmt.Printf("%-20s %-14s %-14s %-12s %s\n",
					truncate(e.CreatedAt, 20),
					truncate(e.Agent, 14),
					truncate(e.EventType, 14),
					truncate(e.ToolName, 12),
					truncate(e.Data, 40),
				)
			}
			return nil
		},
	}

	cmd.Flags().String("agent", "", "Filter by agent name")
	cmd.Flags().String("run", "", "Filter by run ID")
	cmd.Flags().Int("limit", 50, "Max events to display")

	// Merge agentic causal trace subcommands (list, show, export, prune).
	for _, sub := range agenticTraceSubcommands() {
		cmd.AddCommand(sub)
	}

	return cmd
}

// ErrorsCmd returns the "errors" command for aggregated error viewing.
func ErrorsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "errors",
		Short:   "Aggregated error view",
		Long:    "Display errors from agent sessions.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt("limit")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			events, err := queryEventsByLevel(cmd.Context(), app, "error", limit)
			if err != nil {
				return fmt.Errorf("query errors: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(events)
			}

			if len(events) == 0 {
				fmt.Println("No errors found.")
				return nil
			}

			for _, e := range events {
				fmt.Printf("[%s] %s: %s\n", e.CreatedAt, e.Agent, e.Data)
			}
			return nil
		},
	}

	cmd.Flags().Int("limit", 50, "Max errors to display")

	return cmd
}

// ReplayCmd returns the "replay" command for multi-agent replay.
func ReplayCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "replay",
		Short:   "Multi-agent replay",
		Long:    "Replay the timeline of a run, showing agent activity in chronological order.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, _ := cmd.Flags().GetString("run")

			events, err := queryEvents(cmd.Context(), app, "", runID, 0)
			if err != nil {
				return fmt.Errorf("query events: %w", err)
			}

			if len(events) == 0 {
				fmt.Println("No events to replay.")
				return nil
			}

			for _, e := range events {
				fmt.Printf("[%s] %-14s %-14s %s\n", e.CreatedAt, e.Agent, e.EventType, e.Data)
			}
			return nil
		},
	}

	cmd.Flags().String("run", "", "Run ID to replay")

	return cmd
}

// FeedCmd returns the "feed" command for real-time event streaming.
func FeedCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "feed",
		Short:   "Real-time event stream",
		Long:    "Stream events in real-time as agents produce them. Use Ctrl+C to stop.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")

			if paneMode {
				return runFeedPane(cmd, app)
			}

			events, err := queryEvents(cmd.Context(), app, "", "", 20)
			if err != nil {
				return fmt.Errorf("query events: %w", err)
			}

			fmt.Println("Event feed: watching for new events (Ctrl+C to stop)...")
			for _, e := range events {
				fmt.Printf("[%s] %-14s %-14s %s\n", e.CreatedAt, e.Agent, e.EventType, e.Data)
			}
			fmt.Println("(Live streaming requires polling implementation)")
			return nil
		},
	}

	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runFeedPane runs the event feed in long-lived pane mode, polling for new events.
func runFeedPane(cmd *cobra.Command, app *App) error {
	ctx, cancel := paneContext(cmd.Context())
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	// Checkpoint the WAL every 30s to prevent the reader mark from pinning
	// old WAL frames, which would cause the bridge's sqlite3 writes to fail
	// with SQLITE_BUSY when WAL auto-checkpoint kicks in.
	checkpointTicker := time.NewTicker(30 * time.Second)
	defer checkpointTicker.Stop()

	// Watch the SQLite DB file with fsnotify for instant refresh.
	// When any process writes events to the DB, fsnotify fires
	// and we re-render immediately instead of waiting for the ticker.
	dbChanged := watchDBFile(app)

	watcher := newBinaryWatcher()

	colorResolver := app.Spawner.BuildColorResolver(ctx)

	render := func() {
		clearScreen()
		events, err := queryEvents(ctx, app, "", "", 20)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}

		if len(events) == 0 {
			fmt.Println("\033[2mWaiting for events...\033[0m")
			return
		}

		for _, e := range events {
			timeStr := truncate(e.CreatedAt, 19)
			levelColor := "\033[0m"
			switch e.Level {
			case "error":
				levelColor = "\033[31m"
			case "warn":
				levelColor = "\033[33m"
			case "info":
				levelColor = "\033[36m"
			case "debug":
				levelColor = "\033[2m"
			}
			displayName := normalizeEventAgentName(e.Agent)
			agentName := colorizeAgent(truncate(displayName, 12), colorResolver(e.Agent))
			fmt.Printf("%s%-19s\033[0m %s %-12s %s\n",
				levelColor, timeStr,
				agentName,
				truncate(e.EventType, 12),
				truncate(e.Data, 40),
			)
		}
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-dbChanged:
			// DB file changed (fsnotify) — instant refresh.
			render()
		case <-checkpointTicker.C:
			// PASSIVE checkpoint: move WAL frames to the main DB file without
			// blocking writers. Keeps the WAL small so the bridge's sqlite3
			// calls don't hit SQLITE_BUSY during auto-checkpoint.
			_ = app.DB.Exec(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
		case <-ticker.C:
			if watcher.check() {
				watcher.reexec()
			}
			render()
		}
	}
}

func printEventPane(events []eventRow, colorResolver func(string) string) error {
	header := fmt.Sprintf("%s%s── Events (%d) ──%s", ansiBold, ansiCyan, len(events), ansiReset)
	fmt.Println(header)

	if len(events) == 0 {
		fmt.Printf("\n  %sNo events yet.%s\n", ansiDim, ansiReset)
		return nil
	}

	for _, e := range events {
		ts := shortTime(e.CreatedAt)
		displayName := normalizeEventAgentName(e.Agent)
		agentName := truncate(displayName, 16)
		if colorResolver != nil {
			agentName = colorizeAgent(agentName, colorResolver(e.Agent))
		}
		fmt.Printf(" %s%s%s %s %s\n",
			ansiDim, ts, ansiReset,
			agentName,
			truncate(e.Data, 40),
		)
	}
	return nil
}

// normalizeEventAgentName strips a session UUID prefix from compound agent names.
// Events may store names as "<uuid>-<short-name>" while sessions store the short form.
func normalizeEventAgentName(name string) string {
	if len(name) > 37 && name[8] == '-' && name[13] == '-' && name[18] == '-' && name[23] == '-' && name[36] == '-' {
		return name[37:]
	}
	return name
}

// shortTime extracts HH:MM:SS from a timestamp string.
func shortTime(ts string) string {
	if len(ts) >= 19 {
		part := ts[11:19]
		if len(part) == 8 && part[2] == ':' && part[5] == ':' {
			return part
		}
	}
	if len(ts) > 8 {
		return ts[:8]
	}
	return ts
}


// LogsCmd returns the "logs" command for querying agent logs.
// Enhanced with --follow and --lines flags.
func LogsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs",
		Short:   "Show DB logs + UI logs",
		Long:    "Query event logs from the database, filterable by agent, level, and time.\nUse --follow to stream logs in real-time, --lines to control output count.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName, _ := cmd.Flags().GetString("agent")
			level, _ := cmd.Flags().GetString("level")
			lines, _ := cmd.Flags().GetInt("lines")
			follow, _ := cmd.Flags().GetBool("follow")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			// Use --lines as the limit (defaults to 50).
			limit := lines

			var events []eventRow
			var err error
			if level != "" {
				events, err = queryEventsByLevel(cmd.Context(), app, level, limit)
			} else {
				events, err = queryEvents(cmd.Context(), app, agentName, "", limit)
			}
			if err != nil {
				return fmt.Errorf("query logs: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(events)
			}

			if len(events) == 0 {
				fmt.Println("No log entries found.")
			} else {
				for _, e := range events {
					fmt.Printf("[%s] [%s] %s: %s %s\n", e.CreatedAt, e.Level, e.Agent, e.EventType, e.Data)
				}
			}

			// Follow mode: poll for new events.
			if follow {
				fmt.Println("\n--- Following logs (Ctrl+C to stop) ---")
				lastCount := len(events)
				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()

				for {
					select {
					case <-cmd.Context().Done():
						return nil
					case <-ticker.C:
						var newEvents []eventRow
						if level != "" {
							newEvents, err = queryEventsByLevel(cmd.Context(), app, level, limit)
						} else {
							newEvents, err = queryEvents(cmd.Context(), app, agentName, "", limit)
						}
						if err != nil {
							continue
						}
						if len(newEvents) > lastCount {
							for _, e := range newEvents[lastCount:] {
								fmt.Printf("[%s] [%s] %s: %s %s\n", e.CreatedAt, e.Level, e.Agent, e.EventType, e.Data)
							}
							lastCount = len(newEvents)
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().String("agent", "", "Filter by agent name")
	cmd.Flags().String("level", "", "Filter by level (debug, info, warn, error)")
	cmd.Flags().IntP("lines", "n", 50, "Number of lines to display")
	cmd.Flags().BoolP("follow", "f", false, "Stream logs in real-time")

	return cmd
}

// CostsCmd returns the "costs" command for token/cost analysis.
func CostsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "costs",
		Short:   "Token/cost analysis",
		Long:    "Display token usage and estimated costs across agent sessions.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			metrics, err := queryMetrics(cmd.Context(), app)
			if err != nil {
				return fmt.Errorf("query costs: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(metrics)
			}

			if len(metrics) == 0 {
				fmt.Println("No cost data available.")
				return nil
			}

			fmt.Printf("%-14s %-12s %-10s %-10s %-10s\n", "AGENT", "MODEL", "INPUT", "OUTPUT", "COST")
			for _, m := range metrics {
				fmt.Printf("%-14s %-12s %-10d %-10d $%-9.4f\n",
					truncate(m.Agent, 14),
					truncate(m.Model, 12),
					m.InputTokens,
					m.OutputTokens,
					m.EstimatedCost,
				)
			}
			return nil
		},
	}

	cmd.Flags().String("run", "", "Filter by run ID")

	return cmd
}

// MetricsCmd returns the "metrics" command for session metrics.
func MetricsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "metrics",
		Short:   "Session metrics",
		Long:    "Display performance metrics for agent sessions.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			metrics, err := queryMetrics(cmd.Context(), app)
			if err != nil {
				return fmt.Errorf("query metrics: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(metrics)
			}

			if len(metrics) == 0 {
				fmt.Println("No metrics data available.")
				return nil
			}

			fmt.Printf("%-14s %-12s %-10s %-12s %-10s %-10s\n",
				"AGENT", "CAPABILITY", "DURATION", "MODEL", "INPUT", "OUTPUT")
			for _, m := range metrics {
				fmt.Printf("%-14s %-12s %-10dms %-12s %-10d %-10d\n",
					truncate(m.Agent, 14),
					truncate(m.Capability, 12),
					m.DurationMs,
					truncate(m.Model, 12),
					m.InputTokens,
					m.OutputTokens,
				)
			}
			return nil
		},
	}

	return cmd
}

// RunCmd returns the "run" command for run management.
func RunCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run",
		Short:   "Run management",
		Long:    "List and inspect orchestration runs.",
		GroupID: "OBSERVABILITY",
	}

	cmd.AddCommand(runListCmd(app))

	return cmd
}

func runListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			runs, err := queryRuns(cmd.Context(), app)
			if err != nil {
				return fmt.Errorf("list runs: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(runs)
			}

			if len(runs) == 0 {
				fmt.Println("No runs found.")
				return nil
			}

			fmt.Printf("%-20s %-10s %-8s %-20s\n", "ID", "STATUS", "AGENTS", "STARTED")
			for _, r := range runs {
				fmt.Printf("%-20s %-10s %-8d %-20s\n",
					truncate(r.ID, 20),
					truncate(r.Status, 10),
					r.AgentCount,
					truncate(r.StartedAt, 20),
				)
			}
			return nil
		},
	}

	return cmd
}

// --- DB query helpers for observability commands ---

type eventRow struct {
	Agent     string `json:"agent"`
	EventType string `json:"eventType"`
	ToolName  string `json:"toolName"`
	Data      string `json:"data"`
	Level     string `json:"level"`
	CreatedAt string `json:"createdAt"`
}

func queryEvents(ctx context.Context, app *App, agent, runID string, limit int) ([]eventRow, error) {
	query := "SELECT agent_name, event_type, COALESCE(tool_name, ''), COALESCE(data, ''), level, created_at FROM events WHERE 1=1"
	var qargs []any

	if agent != "" {
		query += " AND agent_name = ?"
		qargs = append(qargs, agent)
	}
	if runID != "" {
		query += " AND run_id = ?"
		qargs = append(qargs, runID)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := app.DB.Query(ctx, query, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.Agent, &e.EventType, &e.ToolName, &e.Data, &e.Level, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func queryEventsByLevel(ctx context.Context, app *App, level string, limit int) ([]eventRow, error) {
	query := "SELECT agent_name, event_type, COALESCE(tool_name, ''), COALESCE(data, ''), level, created_at FROM events WHERE level = ? ORDER BY created_at DESC"
	var qargs []any
	qargs = append(qargs, level)

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := app.DB.Query(ctx, query, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.Agent, &e.EventType, &e.ToolName, &e.Data, &e.Level, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type metricsRow struct {
	Agent         string  `json:"agent"`
	Capability    string  `json:"capability"`
	Model         string  `json:"model"`
	DurationMs    int     `json:"durationMs"`
	InputTokens   int64   `json:"inputTokens"`
	OutputTokens  int64   `json:"outputTokens"`
	EstimatedCost float64 `json:"estimatedCost"`
}

func queryMetrics(ctx context.Context, app *App) ([]metricsRow, error) {
	query := `SELECT agent_name, capability, COALESCE(model_used, ''), COALESCE(duration_ms, 0),
		input_tokens, output_tokens, COALESCE(estimated_cost_usd, 0.0)
		FROM metrics ORDER BY started_at DESC LIMIT 100`

	rows, err := app.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []metricsRow
	for rows.Next() {
		var m metricsRow
		if err := rows.Scan(&m.Agent, &m.Capability, &m.Model, &m.DurationMs,
			&m.InputTokens, &m.OutputTokens, &m.EstimatedCost); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

type runRow struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	AgentCount int    `json:"agentCount"`
	StartedAt  string `json:"startedAt"`
}

func queryRuns(ctx context.Context, app *App) ([]runRow, error) {
	query := "SELECT id, status, agent_count, started_at FROM runs ORDER BY started_at DESC LIMIT 50"
	rows, err := app.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []runRow
	for rows.Next() {
		var r runRow
		if err := rows.Scan(&r.ID, &r.Status, &r.AgentCount, &r.StartedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}
