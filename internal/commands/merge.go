package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/merge"
)

// MergeCmd returns the "merge" command group for merge queue management.
func MergeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "merge",
		Short:   "Merge agent branches",
		Long:    "Manage the merge queue: enqueue branches, list the queue, execute merges.",
		GroupID: "MERGE",
	}

	cmd.AddCommand(mergeEnqueueCmd(app))
	cmd.AddCommand(mergeListCmd(app))
	cmd.AddCommand(mergeStatusCmd(app))
	cmd.AddCommand(mergeRunCmd(app))

	return cmd
}

func mergeEnqueueCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enqueue <branch>",
		Short: "Enqueue a branch for merging",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			taskID, _ := cmd.Flags().GetString("task")
			agentName, _ := cmd.Flags().GetString("agent")

			if taskID == "" {
				return fmt.Errorf("--task is required")
			}
			if agentName == "" {
				return fmt.Errorf("--agent is required")
			}

			entry := &merge.MergeEntry{
				BranchName: branch,
				TaskID:     taskID,
				AgentName:  agentName,
			}

			if err := app.MergeQueue.Enqueue(entry); err != nil {
				return fmt.Errorf("enqueue: %w", err)
			}

			fmt.Printf("Enqueued branch %q for merging\n", branch)
			return nil
		},
	}

	cmd.Flags().String("task", "", "Task ID (required)")
	cmd.Flags().String("agent", "", "Agent name (required)")

	return cmd
}

func mergeListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List merge queue entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			statusFilter, _ := cmd.Flags().GetString("status")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			opts := merge.ListOpts{}
			if statusFilter != "" {
				s := merge.MergeStatus(statusFilter)
				opts.Status = &s
			}

			entries, err := app.MergeQueue.List(opts)
			if err != nil {
				return fmt.Errorf("list merge queue: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(entries)
			}

			if len(entries) == 0 {
				fmt.Println("Merge queue is empty.")
				return nil
			}

			fmt.Printf("%-28s %-12s %-10s %-6s\n", "BRANCH", "AGENT", "STATUS", "FILES")
			for _, e := range entries {
				fmt.Printf("%-28s %-12s %-10s %-6d\n",
					truncate(e.BranchName, 28),
					truncate(e.AgentName, 12),
					truncate(string(e.Status), 10),
					len(e.FilesModified),
				)
			}
			fmt.Printf("\n%d entry(ies)\n", len(entries))
			return nil
		},
	}

	cmd.Flags().String("status", "", "Filter by status (pending, merging, merged, conflict, failed)")

	return cmd
}

func mergeStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status <branch>",
		Short: "Check status of a branch in the merge queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := app.MergeQueue.Status(args[0])
			if err != nil {
				return fmt.Errorf("merge status: %w", err)
			}

			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(entry)
			}

			fmt.Printf("Branch:  %s\n", entry.BranchName)
			fmt.Printf("Agent:   %s\n", entry.AgentName)
			fmt.Printf("Task:    %s\n", entry.TaskID)
			fmt.Printf("Status:  %s\n", entry.Status)
			if entry.ResolvedTier != nil {
				fmt.Printf("Tier:    %s\n", *entry.ResolvedTier)
			}
			return nil
		},
	}
}

func mergeRunCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute next merge from the queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _ := cmd.Flags().GetString("target")
			if target == "" {
				target = app.Config.Project.CanonicalBranch
			}

			entry, err := app.MergeQueue.Dequeue()
			if err != nil {
				return fmt.Errorf("dequeue: %w", err)
			}

			result, err := app.MergeExecutor.Execute(entry, target)
			if err != nil {
				return fmt.Errorf("execute merge: %w", err)
			}

			if result.Success {
				fmt.Printf("Merged %s into %s (tier: %s)\n", entry.BranchName, target, result.Tier)
			} else {
				fmt.Printf("Merge failed for %s (tier: %s)\n", entry.BranchName, result.Tier)
				if len(result.ConflictFiles) > 0 {
					fmt.Println("Conflict files:")
					for _, f := range result.ConflictFiles {
						fmt.Printf("  - %s\n", f)
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().String("target", "", "Target branch (default: canonical branch from config)")

	return cmd
}
