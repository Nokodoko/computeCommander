package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/worktree"
)

// CleanCmd returns the "clean" command for resource cleanup.
func CleanCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clean",
		Short:   "Cleanup resources",
		Long:    "Remove stale worktrees, purge old messages, and clean up other resources.",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			force, _ := cmd.Flags().GetBool("force")
			all, _ := cmd.Flags().GetBool("all")

			// Clean worktrees.
			wtOpts := worktree.CleanOpts{
				DryRun: dryRun,
				Force:  force,
			}
			wtCount, err := app.WorktreeManager.Clean(wtOpts)
			if err != nil {
				fmt.Printf("Warning: worktree clean: %v\n", err)
			} else if dryRun {
				fmt.Printf("Would remove %d worktree(s)\n", wtCount)
			} else {
				fmt.Printf("Removed %d worktree(s)\n", wtCount)
			}

			// Purge read messages if --all.
			if all {
				mailOpts := mail.PurgeOpts{
					ReadOnly: true,
				}
				if !dryRun {
					mailOpts.Before = time.Now()
				}
				mailCount, err := app.MailStore.Purge(mailOpts)
				if err != nil {
					fmt.Printf("Warning: mail purge: %v\n", err)
				} else if dryRun {
					fmt.Printf("Would purge %d read message(s)\n", mailCount)
				} else {
					fmt.Printf("Purged %d read message(s)\n", mailCount)
				}
			}

			if !dryRun {
				fmt.Println("Cleanup complete.")
			}
			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Preview cleanup without executing")
	cmd.Flags().Bool("force", false, "Force cleanup of all resources")
	cmd.Flags().Bool("all", false, "Clean all resource types including mail")

	return cmd
}
