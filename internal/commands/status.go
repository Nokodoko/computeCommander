package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
)

// StatusCmd returns the "status" command for fleet overview.
func StatusCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Fleet status overview",
		Long:    "Display an overview of all agent sessions and their current state.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			capability, _ := cmd.Flags().GetString("capability")
			state, _ := cmd.Flags().GetString("state")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			opts := agents.ListOpts{}
			if capability != "" {
				opts.Capability = agents.Capability(capability)
			}
			if state != "" {
				opts.State = agents.SessionState(state)
			}

			sessions, err := app.Spawner.ListSessions(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"agents": sessions,
					"count":  len(sessions),
				})
			}

			if len(sessions) == 0 {
				fmt.Println("No active agents.")
				return nil
			}

			fmt.Printf("%-14s %-12s %-10s %-14s %-8s\n", "NAME", "CAPABILITY", "STATE", "TASK", "RUNTIME")
			for _, s := range sessions {
				fmt.Printf("%-14s %-12s %-10s %-14s %-8s\n",
					truncate(s.AgentName, 14),
					truncate(string(s.Capability), 12),
					truncate(string(s.State), 10),
					truncate(s.TaskID, 14),
					truncate(string(s.Runtime), 8),
				)
			}
			fmt.Printf("\nTotal: %d agent(s)\n", len(sessions))
			return nil
		},
	}

	cmd.Flags().String("capability", "", "Filter by capability")
	cmd.Flags().String("state", "", "Filter by state")

	return cmd
}

// truncate shortens a string to maxLen, adding ".." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 2 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}
