package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
		Long:    "Remove stale worktrees, purge old messages, kill zombie panes, and clean up other resources.",
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

			// Kill zombie pane processes.
			zombieCount := cleanZombiePanes(dryRun)
			if dryRun {
				fmt.Printf("Would kill %d zombie pane process(es)\n", zombieCount)
			} else if zombieCount > 0 {
				fmt.Printf("Killed %d zombie pane process(es)\n", zombieCount)
			}

			// Prune old completed sessions from the DB (keep last 50).
			if all && app.DB != nil && !dryRun {
				pruned := pruneOldSessions(app)
				if pruned > 0 {
					fmt.Printf("Pruned %d old completed session(s)\n", pruned)
				}

				// Checkpoint and vacuum the WAL.
				_ = app.DB.Exec(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
				fmt.Println("WAL checkpointed and truncated.")
			}

			// Clean up stale cmdr.db if empty.
			if info, err := os.Stat(".computecommander/cmdr.db"); err == nil && info.Size() == 0 {
				if !dryRun {
					_ = os.Remove(".computecommander/cmdr.db")
					fmt.Println("Removed empty cmdr.db")
				} else {
					fmt.Println("Would remove empty cmdr.db")
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
	cmd.Flags().Bool("all", false, "Clean all resource types including mail, old sessions, and zombie processes")

	return cmd
}

// cleanZombiePanes finds and kills orphaned cmdr pane processes.
// These accumulate when zellij tabs are closed without killing the child processes.
// Each zombie holds an open SQLite connection, preventing WAL checkpointing and
// causing the WAL file to grow unbounded.
//
// Strategy: for each pane command pattern, find all matching processes.
// Keep only the 2 most recently started ones (one active dashboard may have
// a current and a reexec'd process). Kill all others.
func cleanZombiePanes(dryRun bool) int {
	patterns := []string{
		"cmdr status --pane",
		"cmdr feed --pane",
		"cmdr mail list --pane",
		"cmdr merge list --pane",
		"cmdr evals --pane",
		"cmdr git-status --pane",
	}

	myPID := os.Getpid()
	killed := 0

	for _, pattern := range patterns {
		out, err := exec.Command("pgrep", "-f", pattern).Output()
		if err != nil {
			continue
		}

		pids := strings.Fields(strings.TrimSpace(string(out)))
		if len(pids) <= 2 {
			continue
		}

		// Kill all but the 2 newest PIDs. Higher PIDs are generally newer.
		// Sort PIDs descending so we keep the highest (newest) ones.
		pidNums := make([]int, 0, len(pids))
		for _, pidStr := range pids {
			pid, err := strconv.Atoi(pidStr)
			if err != nil || pid == myPID {
				continue
			}
			pidNums = append(pidNums, pid)
		}

		// Keep the 2 highest PIDs (newest processes).
		// Sort descending by value.
		for i := 0; i < len(pidNums); i++ {
			for j := i + 1; j < len(pidNums); j++ {
				if pidNums[j] > pidNums[i] {
					pidNums[i], pidNums[j] = pidNums[j], pidNums[i]
				}
			}
		}

		// Kill everything except the first 2 (newest).
		for i := 2; i < len(pidNums); i++ {
			if !dryRun {
				proc, err := os.FindProcess(pidNums[i])
				if err == nil {
					_ = proc.Signal(os.Kill)
				}
			}
			killed++
		}
	}

	return killed
}

// pruneOldSessions removes completed sessions older than the newest 50,
// along with their associated events and mail entries.
func pruneOldSessions(app *App) int {
	ctx := context.Background()

	// Count total completed sessions.
	var total int
	row := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sessions WHERE state = 'completed'")
	_ = row.Scan(&total)

	if total <= 50 {
		return 0
	}

	pruneCount := total - 50

	// Delete the oldest completed sessions (by started_at).
	err := app.DB.Exec(ctx, `
		DELETE FROM sessions WHERE id IN (
			SELECT id FROM sessions WHERE state = 'completed'
			ORDER BY started_at ASC LIMIT ?
		)`, pruneCount)
	if err != nil {
		return 0
	}

	// Clean up orphaned events (events referencing sessions that no longer exist).
	_ = app.DB.Exec(ctx, `
		DELETE FROM events WHERE session_id != '' AND session_id NOT IN (
			SELECT id FROM sessions
		)`)

	return pruneCount
}
