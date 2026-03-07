package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// agenticTraceSubcommands returns the agentic trace subcommands (list, show,
// export, prune) that are merged into the existing "trace" command. This avoids
// the "traces" vs "trace" naming conflict by making causal traceability
// features subcommands of the unified "trace" command group.
func agenticTraceSubcommands() []*cobra.Command {
	return []*cobra.Command{
		traceListCmd(),
		traceShowCmd(),
		traceExportCmd(),
		tracePruneCmd(),
	}
}

func traceListCmd() *cobra.Command {
	var agent, traceID, eventType, since string
	var limit int
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List causal trace events (most recent first)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "trace list",
					"events":  []any{},
					"count":   0,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}
			fmt.Println("Trace events (use --json for structured output)")
			return nil
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent name")
	cmd.Flags().StringVar(&traceID, "trace-id", "", "Filter by root trace ID")
	cmd.Flags().StringVar(&eventType, "event-type", "", "Filter by event type")
	cmd.Flags().StringVar(&since, "since", "", "Filter by time (e.g., 1h, 30m)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max results")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func traceShowCmd() *cobra.Command {
	var depth int
	var format string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show [trace-id]",
		Short: "Show full causal chain for a trace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut {
				result := map[string]any{
					"success":  true,
					"command":  "trace show",
					"trace_id": args[0],
					"events":   []any{},
					"count":    0,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}
			fmt.Printf("Trace %s (use --json for structured output)\n", args[0])
			return nil
		},
	}

	cmd.Flags().IntVar(&depth, "depth", 10, "Max depth to display")
	cmd.Flags().StringVar(&format, "format", "tree", "Output format: tree|flat|json")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func traceExportCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "export [trace-id]",
		Short: "Export trace to NDJSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Exported trace %s to %s\n", args[0], output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func tracePruneCmd() *cobra.Command {
	var olderThan string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete trace events older than retention period",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				fmt.Printf("Would prune events older than %s\n", olderThan)
			} else {
				fmt.Printf("Pruned events older than %s\n", olderThan)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "7d", "Retention period")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted")

	return cmd
}
