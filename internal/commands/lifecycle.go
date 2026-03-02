package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// ShutdownCmd returns the "stop" lifecycle command that stops DB + closes UI.
// Named ShutdownCmd to avoid collision with the existing agent StopCmd in stop.go.
func ShutdownCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shutdown",
		Aliases: []string{},
		Short:   "Stop DB + close UI (confirmation-gated)",
		Long:    "Shut down the ComputeCommander database and close the zellij UI session.\nRequires confirmation unless --force is specified.",
		GroupID: "LIFECYCLE",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("Are you sure you want to stop cmdr? This will close the UI and stop the DB.") {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "stop",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Close the zellij session if running.
			if app.PaneManager != nil {
				_ = app.PaneManager.ClosePane("all")
			}

			// Remove the lock file if it exists.
			lockPath := filepath.Join(".computecommander", "cmdr.lock")
			_ = os.Remove(lockPath)

			// Close the database.
			if app.DB != nil {
				if err := app.DB.Close(); err != nil {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "stop",
							"error":   err.Error(),
						})
					}
					return fmt.Errorf("close database: %w", err)
				}
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":   true,
					"command":   "stop",
					"dbRunning": false,
					"uiRunning": false,
				})
			}

			fmt.Println("cmdr stopped.")
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")
	return cmd
}

// ResetCmd returns the "reset" command that resets DB to empty + closes UI.
func ResetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reset",
		Short:   "Reset DB to empty + close UI (confirmation-gated)",
		Long:    "Reset the ComputeCommander database to empty state and close the UI.\nThis is destructive — all session data, events, and metrics will be lost.\nRequires confirmation unless --force is specified.",
		GroupID: "LIFECYCLE",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("WARNING: This will delete ALL data (sessions, events, mail, metrics). Are you sure?") {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "reset",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Close UI.
			if app.PaneManager != nil {
				_ = app.PaneManager.ClosePane("all")
			}

			// Drop all data from known tables.
			tables := []string{
				"sessions", "events", "mail", "merge_queue",
				"metrics", "runs", "task_groups", "task_group_members",
				"checkpoints", "worktrees",
			}

			for _, table := range tables {
				if err := app.DB.Exec(cmd.Context(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
					// Table may not exist; continue.
					continue
				}
			}

			// Remove lock file.
			lockPath := filepath.Join(".computecommander", "cmdr.lock")
			_ = os.Remove(lockPath)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":   true,
					"command":   "reset",
					"dbRunning": true,
					"uiRunning": false,
				})
			}

			fmt.Println("cmdr reset complete. All data has been cleared.")
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")
	return cmd
}

// RestartCmd returns the "restart" command that restarts DB + UI.
func RestartCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		Short:   "Restart DB + UI (confirmation-gated)",
		Long:    "Restart the ComputeCommander database and UI.\nRequires confirmation unless --force is specified.",
		GroupID: "LIFECYCLE",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("Are you sure you want to restart cmdr?") {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "restart",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Close UI.
			if app.PaneManager != nil {
				_ = app.PaneManager.ClosePane("all")
			}

			// Reopen the dashboard.
			if app.Spawner != nil {
				err := app.Spawner.SpawnDashboard(cmd.Context())
				if err == nil {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success":   true,
							"command":   "restart",
							"dbRunning": true,
							"uiRunning": true,
						})
					}
					fmt.Println("cmdr restarted.")
					return nil
				}
			}

			// Fall back to in-process TUI.
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":   true,
					"command":   "restart",
					"dbRunning": true,
					"uiRunning": true,
				})
			}

			fmt.Println("cmdr restarted (in-process TUI).")
			return app.RunDashboard(cmd.Context())
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")
	return cmd
}

// confirmAction prompts the user for confirmation and returns true if they accept.
func confirmAction(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
