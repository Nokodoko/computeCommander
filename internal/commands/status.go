package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

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
			paneMode, _ := cmd.Flags().GetBool("pane")
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

			if paneMode {
				return runStatusPane(cmd, app, opts)
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
	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runStatusPane runs the status command in long-lived pane mode, refreshing periodically.
func runStatusPane(cmd *cobra.Command, app *App, opts agents.ListOpts) error {
	ctx := cmd.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	render := func() {
		clearScreen()
		sessions, err := app.Spawner.ListSessions(ctx, opts)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}

		if len(sessions) == 0 {
			fmt.Println("\033[2mNo active agents.\033[0m")
			return
		}

		fmt.Printf("\033[2m%-14s %-12s %-10s %-14s\033[0m\n", "NAME", "CAPABILITY", "STATE", "TASK")
		for _, s := range sessions {
			stateColor := "\033[32m" // green for working
			switch s.State {
			case agents.StateZombie:
				stateColor = "\033[31m" // red
			case agents.StateStalled:
				stateColor = "\033[33m" // yellow
			case agents.StateCompleted:
				stateColor = "\033[2m" // dim
			case agents.StateBooting:
				stateColor = "\033[36m" // cyan
			}
			fmt.Printf("%-14s %-12s %s%-10s\033[0m %-14s\n",
				truncate(s.AgentName, 14),
				truncate(string(s.Capability), 12),
				stateColor,
				truncate(string(s.State), 10),
				truncate(s.TaskID, 14),
			)
		}
		fmt.Printf("\n\033[2mTotal: %d agent(s)\033[0m\n", len(sessions))
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			render()
		}
	}
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
