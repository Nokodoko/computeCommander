package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/worktree"
)

// WorktreeCmd returns the "worktree" command group for worktree management.
func WorktreeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "worktree",
		Short:   "Worktree management",
		Long:    "Manage git worktrees used by agents.",
		GroupID: "INFRASTRUCTURE",
		Aliases: []string{"wt"},
	}

	cmd.AddCommand(worktreeListCmd(app))
	cmd.AddCommand(worktreeStatusCmd(app))
	cmd.AddCommand(worktreeCleanCmd(app))
	cmd.AddCommand(worktreeRemoveCmd(app))

	return cmd
}

func worktreeListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			wts, err := app.WorktreeManager.List()
			if err != nil {
				return fmt.Errorf("list worktrees: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(wts)
			}

			if len(wts) == 0 {
				fmt.Println("No worktrees found.")
				return nil
			}

			fmt.Printf("%-40s %-24s %-10s\n", "PATH", "BRANCH", "STATE")
			for _, wt := range wts {
				fmt.Printf("%-40s %-24s %-10s\n",
					truncate(wt.Path, 40),
					truncate(wt.Branch, 24),
					truncate(string(wt.State), 10),
				)
			}
			fmt.Printf("\n%d worktree(s)\n", len(wts))
			return nil
		},
	}
}

func worktreeStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status <path>",
		Short: "Show detailed worktree status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			status, err := app.WorktreeManager.Status(args[0])
			if err != nil {
				return fmt.Errorf("worktree status: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(status)
			}

			fmt.Printf("Path:     %s\n", status.Path)
			fmt.Printf("Branch:   %s\n", status.Branch)
			fmt.Printf("State:    %s\n", status.State)
			fmt.Printf("Clean:    %v\n", status.IsClean)
			fmt.Printf("Commits:  %d\n", status.CommitCount)
			return nil
		},
	}
}

func worktreeCleanCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove completed/orphaned worktrees",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")

			opts := worktree.CleanOpts{
				DryRun: dryRun,
				Force:  force,
			}

			count, err := app.WorktreeManager.Clean(opts)
			if err != nil {
				return fmt.Errorf("clean worktrees: %w", err)
			}

			if dryRun {
				fmt.Printf("Would remove %d worktree(s)\n", count)
			} else {
				fmt.Printf("Removed %d worktree(s)\n", count)
			}
			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Preview what would be removed without actually removing")
	cmd.Flags().Bool("force", false, "Force removal even with uncommitted changes")

	return cmd
}

func worktreeRemoveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <path>",
		Short: "Remove a specific worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")

			if err := app.WorktreeManager.Remove(args[0], force); err != nil {
				return fmt.Errorf("remove worktree: %w", err)
			}

			fmt.Printf("Removed worktree %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Force removal even with uncommitted changes")

	return cmd
}
