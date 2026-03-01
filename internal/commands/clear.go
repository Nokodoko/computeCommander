package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// ClearCmd returns the "clear" command for clearing DB logs and UI logs.
// This is distinct from CleanCmd (clean.go) which handles resource cleanup
// like stale worktrees and temp files.
func ClearCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "clear",
		Short:   "Clear DB logs + UI logs",
		Long:    "Clear event logs from the database. This is distinct from `clean` which handles resource cleanup.\nRequires confirmation unless --force is specified.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("Clear all event logs?") {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "clear",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Clear events table.
			if err := app.DB.Exec(cmd.Context(), "DELETE FROM events"); err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "clear",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("clear events: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success": true,
					"command": "clear",
				})
			}

			fmt.Println("Logs cleared.")
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}
