package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentic/gate"
)

// GateCmd returns the gate command group.
func GateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "gate",
		Short:   "Manage quality gates",
		Long:    "List configured quality gates, run them against blueprint output, and view history.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(gateListCmd(app))
	cmd.AddCommand(gateRunCmd(app))
	cmd.AddCommand(gateHistoryCmd(app))

	return cmd
}

func gateListCmd(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured quality gates",
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.GatePipeline == nil {
				if jsonOut {
					result := map[string]any{
						"success": true,
						"command": "gate list",
						"gates":   []any{},
						"count":   0,
					}
					return json.NewEncoder(os.Stdout).Encode(result)
				}
				fmt.Println("No gate pipeline configured.")
				return nil
			}

			gates := app.GatePipeline.ListGates()

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "gate list",
					"gates":   gates,
					"count":   len(gates),
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(gates) == 0 {
				fmt.Println("No quality gates configured.")
				return nil
			}

			fmt.Printf("%-12s %-8s %s\n", "NAME", "ENABLED", "COMMAND")
			for _, g := range gates {
				enabled := "yes"
				if !g.Enabled {
					enabled = "no"
				}
				fmt.Printf("%-12s %-8s %s\n",
					truncate(string(g.Name), 12),
					enabled,
					truncate(g.Command, 50),
				)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func gateRunCmd(app *App) *cobra.Command {
	var gateName string
	var verbose bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "run [bp-id]",
		Short: "Run quality gates for a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.GatePipeline == nil {
				return fmt.Errorf("no gate pipeline configured")
			}

			blueprintID := args[0]

			if gateName != "" {
				// Run a single named gate.
				result, err := app.GatePipeline.RunSingle(
					cmd.Context(),
					gate.GateName(gateName),
					blueprintID, "", 1,
				)
				if err != nil {
					return fmt.Errorf("run gate %s: %w", gateName, err)
				}

				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(result)
				}

				status := "PASS"
				if !result.Passed {
					status = "FAIL"
				}
				fmt.Printf("[%s] %s (%dms)\n", status, result.GateName, result.DurationMs)
				if verbose && result.StdoutExcerpt != "" {
					fmt.Printf("  stdout: %s\n", result.StdoutExcerpt)
				}
				if verbose && result.StderrExcerpt != "" {
					fmt.Printf("  stderr: %s\n", result.StderrExcerpt)
				}
				return nil
			}

			// Run the full pipeline.
			result, err := app.GatePipeline.Run(cmd.Context(), blueprintID, "", 1)
			if err != nil {
				return fmt.Errorf("run gate pipeline: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			for _, gr := range result.Results {
				status := "PASS"
				if !gr.Passed {
					status = "FAIL"
				}
				fmt.Printf("[%s] %s (%dms)\n", status, gr.GateName, gr.DurationMs)
				if verbose && gr.StdoutExcerpt != "" {
					fmt.Printf("  stdout: %s\n", gr.StdoutExcerpt)
				}
				if verbose && gr.StderrExcerpt != "" {
					fmt.Printf("  stderr: %s\n", gr.StderrExcerpt)
				}
			}

			if result.Passed {
				fmt.Println("\nAll gates passed.")
			} else {
				fmt.Printf("\nFailed gates: %v\n", result.Failed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&gateName, "gate", "", "Run specific gate only")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show full stdout/stderr")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func gateHistoryCmd(app *App) *cobra.Command {
	var limit int
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "history [bp-id]",
		Short: "Show gate results for a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.GatePipeline == nil {
				return fmt.Errorf("no gate pipeline configured")
			}

			results, err := app.GatePipeline.History(cmd.Context(), args[0], limit)
			if err != nil {
				return fmt.Errorf("query gate history: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "gate history",
					"bp_id":   args[0],
					"results": results,
					"count":   len(results),
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(results) == 0 {
				fmt.Printf("No gate history for blueprint %s\n", args[0])
				return nil
			}

			fmt.Printf("%-12s %-6s %-8s %-20s\n", "GATE", "PASS", "EXIT", "TIME")
			for _, gr := range results {
				passed := "yes"
				if !gr.Passed {
					passed = "no"
				}
				fmt.Printf("%-12s %-6s %-8d %-20s\n",
					truncate(string(gr.GateName), 12),
					passed,
					gr.ExitCode,
					truncate(gr.CreatedAt.Format("2006-01-02 15:04:05"), 20),
				)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Max results")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}
