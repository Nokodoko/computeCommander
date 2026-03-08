package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

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
			paneMode, _ := cmd.Flags().GetBool("pane")
			statusFilter, _ := cmd.Flags().GetString("status")
			projectID, _ := cmd.Flags().GetString("project")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			listOpts := merge.ListOpts{
				ProjectID: projectID,
			}
			if statusFilter != "" {
				s := merge.MergeStatus(statusFilter)
				listOpts.Status = &s
			}

			if paneMode {
				return runMergeListPane(cmd, app, listOpts)
			}

			entries, err := app.MergeQueue.List(listOpts)
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

			// Build agent color resolver for colorized output.
			colorResolver := app.Spawner.BuildColorResolver(cmd.Context())

			fmt.Printf("%-28s %-12s %-10s %-6s\n", "BRANCH", "AGENT", "STATUS", "FILES")
			for _, e := range entries {
				agentName := colorizeAgent(truncate(e.AgentName, 12), colorResolver(e.AgentName))
				fmt.Printf("%-28s %-12s %-10s %-6d\n",
					truncate(e.BranchName, 28),
					agentName,
					truncate(string(e.Status), 10),
					len(e.FilesModified),
				)
			}
			fmt.Printf("\n%d entry(ies)\n", len(entries))
			return nil
		},
	}

	cmd.Flags().String("status", "", "Filter by status (pending, merging, merged, conflict, failed)")
	cmd.Flags().String("project", "", "Filter by project ID")
	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runMergeListPane runs merge list in long-lived pane mode, refreshing periodically.
func runMergeListPane(cmd *cobra.Command, app *App, opts merge.ListOpts) error {
	ctx, cancel := paneContext(cmd.Context())
	defer cancel()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Watch the SQLite DB file with fsnotify for instant refresh.
	// When any process writes merge entries to the DB, fsnotify fires
	// and we re-render immediately instead of waiting for the ticker.
	dbChanged := watchDBFile(app)

	watcher := newBinaryWatcher()

	render := func() {
		clearScreen()
		entries, err := app.MergeQueue.List(opts)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}

		if len(entries) == 0 {
			fmt.Println("\033[2mMerge queue empty.\033[0m")
			return
		}

		fmt.Printf("\033[2m%-28s %-12s %-10s %-6s\033[0m\n", "BRANCH", "AGENT", "STATUS", "FILES")
		for _, e := range entries {
			statusColor := "\033[0m"
			switch e.Status {
			case merge.MergePending:
				statusColor = "\033[33m"
			case merge.MergeMerging:
				statusColor = "\033[36m"
			case merge.MergeMerged:
				statusColor = "\033[32m"
			case merge.MergeConflict, merge.MergeFailed:
				statusColor = "\033[31m"
			}
			fmt.Printf("%-28s %-12s %s%-10s\033[0m %-6d\n",
				truncate(e.BranchName, 28),
				truncate(e.AgentName, 12),
				statusColor,
				truncate(string(e.Status), 10),
				len(e.FilesModified),
			)
		}
		fmt.Printf("\n\033[2m%d entry(ies)\033[0m\n", len(entries))
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
		case <-ticker.C:
			if watcher.check() {
				watcher.reexec()
			}
			render()
		}
	}
}

func printMergePane(entries []*merge.MergeEntry) error {
	pending := 0
	for _, e := range entries {
		if e.Status == merge.MergePending {
			pending++
		}
	}

	header := fmt.Sprintf("%s%s── Merge Queue (%d pending) ──%s", ansiBold, ansiCyan, pending, ansiReset)
	fmt.Println(header)

	if len(entries) == 0 {
		fmt.Printf("\n  %sQueue empty%s\n", ansiDim, ansiReset)
		return nil
	}

	for _, e := range entries {
		statusIcon, statusColor := mergeStatusStyle(e.Status)
		fmt.Printf(" %s%s%s%s %s%-20s%s %s\n",
			statusColor, statusIcon, truncate(string(e.Status), 10), ansiReset,
			ansiBold, truncate(e.BranchName, 20), ansiReset,
			truncate(e.AgentName, 12),
		)
	}
	return nil
}

func mergeStatusStyle(status merge.MergeStatus) (string, string) {
	switch status {
	case merge.MergePending:
		return "◌ ", ansiYellow
	case merge.MergeMerging:
		return "● ", ansiCyan
	case merge.MergeMerged:
		return "✔ ", ansiGreen
	case merge.MergeConflict:
		return "⚠ ", ansiRed
	case merge.MergeFailed:
		return "✖ ", ansiRed
	default:
		return "○ ", ansiDim
	}
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
