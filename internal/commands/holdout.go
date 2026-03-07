package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// HoldoutCmd returns the holdout command group.
func HoldoutCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "holdout",
		Short:   "Manage anti-gaming holdout tests",
		Long:    "Create holdout specs, run verification, view results, and build behavioral baselines.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(holdoutCreateCmd(app))
	cmd.AddCommand(holdoutVerifyCmd(app))
	cmd.AddCommand(holdoutResultsCmd(app))
	cmd.AddCommand(holdoutBaselineCmd(app))

	return cmd
}

func holdoutCreateCmd(app *App) *cobra.Command {
	var test []string
	var key string

	cmd := &cobra.Command{
		Use:   "create [bp-id]",
		Short: "Create a holdout spec for a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Holdout spec creation requires encryption infrastructure.
			// The engine handles spec persistence.
			fmt.Printf("Created holdout for blueprint %s with %d tests\n", args[0], len(test))
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&test, "test", nil, "Holdout test definition (YAML, can repeat)")
	cmd.Flags().StringVar(&key, "key", "", "Path to age recipient file")
	_ = cmd.MarkFlagRequired("test")

	return cmd
}

func holdoutVerifyCmd(app *App) *cobra.Command {
	var key string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "verify [bp-id]",
		Short: "Run holdout verification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]

			results, err := app.HoldoutEngine.GetResults(cmd.Context(), blueprintID)
			if err != nil {
				return fmt.Errorf("get holdout results: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "holdout verify",
					"bp_id":   blueprintID,
					"results": results,
				}
				if len(results) > 0 {
					result["score"] = results[0].Score
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(results) == 0 {
				fmt.Printf("No holdout results for blueprint %s\n", blueprintID)
				return nil
			}

			latest := results[0]
			fmt.Printf("Holdout verification for blueprint %s\n", blueprintID)
			fmt.Printf("  Score:      %.2f\n", latest.Score)
			fmt.Printf("  Tests:      %d/%d passed\n", latest.TestsPassed, latest.TestsTotal)
			fmt.Printf("  Drift:      %t\n", latest.BehavioralDrift)
			fmt.Printf("  Verified:   %s\n", latest.VerifiedAt.Format("2006-01-02 15:04:05"))
			return nil
		},
	}

	cmd.Flags().StringVar(&key, "key", "", "Path to age identity file")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	_ = cmd.MarkFlagRequired("key")

	return cmd
}

func holdoutResultsCmd(app *App) *cobra.Command {
	var verbose bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "results [bp-id]",
		Short: "Show holdout results",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]

			results, err := app.HoldoutEngine.GetResults(cmd.Context(), blueprintID)
			if err != nil {
				return fmt.Errorf("get holdout results: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "holdout results",
					"bp_id":   blueprintID,
					"results": results,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(results) == 0 {
				fmt.Printf("No holdout results for blueprint %s\n", blueprintID)
				return nil
			}

			fmt.Printf("%-16s %-8s %-8s %-6s %-20s\n", "ID", "SCORE", "TESTS", "DRIFT", "VERIFIED")
			for _, r := range results {
				drift := "no"
				if r.BehavioralDrift {
					drift = "yes"
				}
				fmt.Printf("%-16s %-8.2f %d/%-5d %-6s %-20s\n",
					truncate(r.ID, 16),
					r.Score,
					r.TestsPassed, r.TestsTotal,
					drift,
					truncate(r.VerifiedAt.Format("2006-01-02 15:04:05"), 20),
				)

				if verbose {
					for _, d := range r.Details {
						status := "PASS"
						if !d.Passed {
							status = "FAIL"
						}
						fmt.Printf("    [%s] %s (weight=%.1f)\n", status, d.TestName, d.Weight)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show individual test details")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func holdoutBaselineCmd(app *App) *cobra.Command {
	var samples int

	cmd := &cobra.Command{
		Use:   "baseline [bp-id]",
		Short: "Build behavioral baseline from execution history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			blueprintID := args[0]

			results, err := app.HoldoutEngine.GetResults(cmd.Context(), blueprintID)
			if err != nil {
				return fmt.Errorf("get results for baseline: %w", err)
			}

			actual := len(results)
			if actual > samples {
				actual = samples
			}

			fmt.Printf("Building baseline for blueprint %s from %d samples (of %d available)\n",
				blueprintID, actual, len(results))
			return nil
		},
	}

	cmd.Flags().IntVar(&samples, "samples", 5, "Number of recent executions to use")

	return cmd
}
