package commands

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ANSI color constants for eval types (Dracula palette).
var evalTypeANSI = map[string]string{
	// Manual eval types
	"unit_test":   "\033[38;2;139;233;253m", // #8be9fd cyan
	"integration": "\033[38;2;189;147;249m", // #bd93f9 purple
	"lint":        "\033[38;2;241;250;140m", // #f1fa8c yellow
	"build":       "\033[38;2;80;250;123m",  // #50fa7b green
	"custom":      "\033[38;2;102;217;239m", // #66d9ef blue-cyan
	// Hook predicate types
	"semantic_check":       "\033[38;2;80;250;123m",  // #50fa7b green
	"structural_check":     "\033[38;2;80;250;123m",  // #50fa7b green
	"contains_pattern":     "\033[38;2;80;250;123m",  // #50fa7b green
	"count_check":          "\033[38;2;80;250;123m",  // #50fa7b green
	"ast_check":            "\033[38;2;80;250;123m",  // #50fa7b green
	"negation_check":       "\033[38;2;80;250;123m",  // #50fa7b green
	"test_execution":       "\033[38;2;139;233;253m", // #8be9fd cyan
	"type_check":           "\033[38;2;102;217;239m", // #66d9ef blue-cyan
	"diff_validation":      "\033[38;2;241;250;140m", // #f1fa8c yellow
	"output_pattern_match": "\033[38;2;189;147;249m", // #bd93f9 purple
}

const (
	evalAnsiPass    = "\033[38;2;80;250;123m\033[1m" // #50fa7b bold
	evalAnsiFail    = "\033[38;2;255;85;85m\033[1m"  // #ff5555 bold
	evalAnsiPending = "\033[38;2;98;114;164m"        // #6272a4
)

// generateEvalID creates a unique eval ID in the format "eval-{8 hex chars}".
func generateEvalID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("eval-%08x", os.Getpid())
	}
	return fmt.Sprintf("eval-%x", b)
}

// EvalsCmd returns the "evals" command for managing eval definitions and results.
func EvalsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "evals",
		Short:   "Manage eval definitions and results",
		Long:    "List, add, run, and remove eval definitions. Each eval is a shell command with a pass/fail result.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			if paneMode {
				return runEvalsPane(cmd, app)
			}
			return runEvalsList(cmd, app)
		},
	}

	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")
	cmd.Flags().String("project", "", "Filter by project name")
	cmd.Flags().String("type", "", "Filter by eval type")
	cmd.Flags().Bool("json", false, "JSON output")

	cmd.AddCommand(evalsAddCmd(app))
	cmd.AddCommand(evalsRunCmd(app))
	cmd.AddCommand(evalsRemoveCmd(app))

	return cmd
}

// runEvalsList lists all evals with their latest results.
func runEvalsList(cmd *cobra.Command, app *App) error {
	ctx := cmd.Context()
	projectFilter, _ := cmd.Flags().GetString("project")
	typeFilter, _ := cmd.Flags().GetString("type")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	query := "SELECT id, project_name, agent_task, eval_type, passed, error_detail, last_run_at, created_at FROM evals"
	var conditions []string
	var queryArgs []any
	argIdx := 1

	if projectFilter != "" {
		conditions = append(conditions, fmt.Sprintf("project_name = $%d", argIdx))
		queryArgs = append(queryArgs, projectFilter)
		argIdx++
	}
	if typeFilter != "" {
		conditions = append(conditions, fmt.Sprintf("eval_type = $%d", argIdx))
		queryArgs = append(queryArgs, typeFilter)
		argIdx++
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"
	_ = argIdx // silence unused warning

	rows, err := app.DB.Query(ctx, query, queryArgs...)
	if err != nil {
		if jsonOutput {
			return printJSON(map[string]any{"success": false, "command": "evals", "error": err.Error()})
		}
		return fmt.Errorf("query evals: %w", err)
	}
	defer rows.Close()

	type evalEntry struct {
		ID          string  `json:"id"`
		ProjectName string  `json:"project_name"`
		AgentTask   string  `json:"agent_task"`
		EvalType    string  `json:"eval_type"`
		Passed      *bool   `json:"passed"`
		ErrorDetail *string `json:"error_detail"`
		LastRunAt   *string `json:"last_run_at"`
		CreatedAt   string  `json:"created_at"`
	}

	var evals []evalEntry
	for rows.Next() {
		var e evalEntry
		if err := rows.Scan(&e.ID, &e.ProjectName, &e.AgentTask, &e.EvalType, &e.Passed, &e.ErrorDetail, &e.LastRunAt, &e.CreatedAt); err != nil {
			continue
		}
		evals = append(evals, e)
	}

	if jsonOutput {
		return printJSON(map[string]any{
			"success": true,
			"command": "evals",
			"evals":   evals,
			"count":   len(evals),
		})
	}

	if len(evals) == 0 {
		fmt.Println("No evals registered.")
		return nil
	}

	// Table output.
	fmt.Printf("%-14s %-16s %-20s %-12s %-8s\n", "ID", "Project", "Task", "Type", "Result")
	fmt.Println(strings.Repeat("-", 74))
	for _, e := range evals {
		result := evalAnsiPending + "NEVER RUN" + ansiReset
		if e.Passed != nil {
			if *e.Passed {
				result = evalAnsiPass + "PASS" + ansiReset
			} else {
				result = evalAnsiFail + "FAIL" + ansiReset
			}
		}

		typeColor := evalTypeANSI[e.EvalType]
		typeName := typeColor + e.EvalType + ansiReset

		fmt.Printf("%-14s %-16s %-20s %-12s %s\n",
			truncate(e.ID, 14),
			truncate(e.ProjectName, 16),
			truncate(e.AgentTask, 20),
			typeName,
			result,
		)
	}

	return nil
}

// evalsAddCmd creates the "evals add" subcommand.
func evalsAddCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a new eval",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			project, _ := cmd.Flags().GetString("project")
			task, _ := cmd.Flags().GetString("task")
			evalType, _ := cmd.Flags().GetString("type")
			command, _ := cmd.Flags().GetString("command")
			jsonOutput, _ := cmd.Flags().GetBool("json")

			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if command == "" {
				return fmt.Errorf("--command is required")
			}

			// Validate eval type.
			validTypes := map[string]bool{
				"unit_test": true, "integration": true, "lint": true,
				"build": true, "custom": true,
			}
			if !validTypes[evalType] {
				return fmt.Errorf("invalid eval type %q; must be one of: unit_test, integration, lint, build, custom", evalType)
			}

			id := generateEvalID()
			err := app.DB.Exec(ctx,
				"INSERT INTO evals (id, project_name, agent_task, eval_type, command) VALUES ($1, $2, $3, $4, $5)",
				id, project, task, evalType, command,
			)
			if err != nil {
				if jsonOutput {
					return printJSON(map[string]any{"success": false, "command": "evals add", "error": err.Error()})
				}
				return fmt.Errorf("insert eval: %w", err)
			}

			if jsonOutput {
				return printJSON(map[string]any{"success": true, "command": "evals add", "id": id})
			}
			fmt.Printf("Added eval %s\n", id)
			return nil
		},
	}

	cmd.Flags().String("project", "", "Project name (required)")
	cmd.Flags().String("task", "", "Agent task description")
	cmd.Flags().String("type", "custom", "Eval type: unit_test, integration, lint, build, custom")
	cmd.Flags().String("command", "", "Shell command to execute (required)")
	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

// evalsRunCmd creates the "evals run" subcommand.
func evalsRunCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run all evals sequentially",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			projectFilter, _ := cmd.Flags().GetString("project")
			idFilter, _ := cmd.Flags().GetString("id")
			jsonOutput, _ := cmd.Flags().GetBool("json")

			query := "SELECT id, command FROM evals"
			var queryArgs []any
			argIdx := 1

			if idFilter != "" {
				query += fmt.Sprintf(" WHERE id = $%d", argIdx)
				queryArgs = append(queryArgs, idFilter)
				argIdx++
			} else if projectFilter != "" {
				query += fmt.Sprintf(" WHERE project_name = $%d", argIdx)
				queryArgs = append(queryArgs, projectFilter)
				argIdx++
			}
			query += " ORDER BY created_at"
			_ = argIdx

			rows, err := app.DB.Query(ctx, query, queryArgs...)
			if err != nil {
				return fmt.Errorf("query evals: %w", err)
			}
			defer rows.Close()

			type evalRun struct {
				ID      string
				Command string
			}
			var toRun []evalRun
			for rows.Next() {
				var e evalRun
				if err := rows.Scan(&e.ID, &e.Command); err != nil {
					continue
				}
				toRun = append(toRun, e)
			}

			type runResult struct {
				ID         string `json:"id"`
				Passed     bool   `json:"passed"`
				Error      string `json:"error,omitempty"`
				DurationMs int64  `json:"duration_ms"`
			}

			var results []runResult
			passed, failed := 0, 0

			for _, e := range toRun {
				start := time.Now()
				execCmd := exec.CommandContext(ctx, "sh", "-c", e.Command)
				output, err := execCmd.CombinedOutput()
				duration := time.Since(start).Milliseconds()

				r := runResult{
					ID:         e.ID,
					DurationMs: duration,
				}

				now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
				if err != nil {
					r.Passed = false
					errDetail := strings.TrimSpace(string(output))
					if errDetail == "" {
						errDetail = err.Error()
					}
					r.Error = errDetail
					failed++

					_ = app.DB.Exec(ctx,
						"UPDATE evals SET passed = $1, error_detail = $2, last_run_at = $3 WHERE id = $4",
						false, errDetail, now, e.ID,
					)
				} else {
					r.Passed = true
					passed++

					_ = app.DB.Exec(ctx,
						"UPDATE evals SET passed = $1, error_detail = NULL, last_run_at = $2 WHERE id = $3",
						true, now, e.ID,
					)
				}

				results = append(results, r)

				if !jsonOutput {
					status := evalAnsiPass + "PASS" + ansiReset
					if !r.Passed {
						status = evalAnsiFail + "FAIL" + ansiReset
					}
					fmt.Printf("  %s %s (%dms)\n", status, e.ID, duration)
				}
			}

			if jsonOutput {
				return printJSON(map[string]any{
					"success": true,
					"command": "evals run",
					"results": results,
					"passed":  passed,
					"failed":  failed,
					"total":   len(results),
				})
			}

			fmt.Printf("\nResults: %d passed, %d failed, %d total\n", passed, failed, len(results))
			return nil
		},
	}

	cmd.Flags().String("project", "", "Filter by project name")
	cmd.Flags().String("id", "", "Run a single eval by ID")
	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

// evalsRemoveCmd creates the "evals remove" subcommand.
func evalsRemoveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an eval definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id := args[0]
			jsonOutput, _ := cmd.Flags().GetBool("json")

			// Check if the eval exists first.
			var existingID string
			err := app.DB.QueryRow(ctx, "SELECT id FROM evals WHERE id = $1", id).Scan(&existingID)
			if err != nil {
				errMsg := fmt.Sprintf("eval not found: %s", id)
				if jsonOutput {
					return printJSON(map[string]any{"success": false, "command": "evals remove", "error": errMsg})
				}
				return fmt.Errorf("%s", errMsg)
			}

			if err := app.DB.Exec(ctx, "DELETE FROM evals WHERE id = $1", id); err != nil {
				if jsonOutput {
					return printJSON(map[string]any{"success": false, "command": "evals remove", "error": err.Error()})
				}
				return fmt.Errorf("delete eval: %w", err)
			}

			if jsonOutput {
				return printJSON(map[string]any{"success": true, "command": "evals remove", "id": id})
			}
			fmt.Printf("Removed eval %s\n", id)
			return nil
		},
	}

	cmd.Flags().Bool("json", false, "JSON output")

	return cmd
}

// runEvalsPane runs evals in long-lived pane mode, refreshing every 3 seconds.
func runEvalsPane(cmd *cobra.Command, app *App) error {
	ctx := cmd.Context()
	projectFilter, _ := cmd.Flags().GetString("project")
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	watcher := newBinaryWatcher()

	render := func() {
		clearScreen()

		query := "SELECT id, project_name, agent_task, eval_type, passed, error_detail, last_run_at FROM evals"
		var queryArgs []any
		if projectFilter != "" {
			query += " WHERE project_name = $1"
			queryArgs = append(queryArgs, projectFilter)
		}
		query += " ORDER BY created_at DESC"

		rows, err := app.DB.Query(ctx, query, queryArgs...)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}
		defer rows.Close()

		fmt.Printf("\033[1mEvals\033[0m\n\n")

		count := 0
		for rows.Next() {
			var id, project, task, evalType string
			var passed *bool
			var errorDetail, lastRunAt *string
			if err := rows.Scan(&id, &project, &task, &evalType, &passed, &errorDetail, &lastRunAt); err != nil {
				continue
			}
			count++

			// Status indicator.
			status := evalAnsiPending + "---" + ansiReset
			if passed != nil {
				if *passed {
					status = evalAnsiPass + "PASS" + ansiReset
				} else {
					status = evalAnsiFail + "FAIL" + ansiReset
				}
			}

			// Type color.
			typeColor := evalTypeANSI[evalType]
			typeName := typeColor + evalType + ansiReset

			fmt.Printf("  %s %-12s %s %s\n", status, typeName, truncate(id, 14), truncate(task, 30))
		}

		if count == 0 {
			fmt.Println("  No evals registered.")
		}

		fmt.Printf("\n\033[2m[r] Run All  [a] Add\033[0m\n")
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if watcher.check() {
				watcher.reexec()
			}
			render()
		}
	}
}

// printJSON marshals v to JSON and writes to stdout.
func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
