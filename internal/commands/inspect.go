package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
)

// InspectCmd returns the "inspect" command for deep agent inspection.
func InspectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect <agent-name>",
		Short:   "Deep agent inspection",
		Long:    "Display detailed information about an agent session, including worktree, pane, and transcript data.",
		GroupID: "OBSERVABILITY",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			lines, _ := cmd.Flags().GetInt("lines")

			// Find the agent session.
			sessions, err := app.Spawner.ListSessions(cmd.Context(), agents.ListOpts{})
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}

			var session *agents.AgentSession
			for _, s := range sessions {
				if s.AgentName == name {
					session = s
					break
				}
			}

			if session == nil {
				return fmt.Errorf("agent %q not found", name)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(session)
			}

			fmt.Printf("Agent: %s\n", session.AgentName)
			fmt.Printf("  Session ID:  %s\n", session.ID)
			fmt.Printf("  Capability:  %s\n", session.Capability)
			fmt.Printf("  State:       %s\n", session.State)
			fmt.Printf("  Runtime:     %s\n", session.Runtime)
			fmt.Printf("  Task:        %s\n", session.TaskID)
			fmt.Printf("  Branch:      %s\n", session.BranchName)
			fmt.Printf("  Worktree:    %s\n", session.WorktreePath)
			fmt.Printf("  Pane:        %s\n", session.ZellijPane)
			fmt.Printf("  PID:         %d\n", session.PID)
			fmt.Printf("  Depth:       %d\n", session.Depth)
			fmt.Printf("  Parent:      %s\n", session.ParentAgent)
			fmt.Printf("  Started:     %s\n", session.StartedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Last Active: %s\n", session.LastActivity.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Escalation:  %d\n", session.EscalationLevel)

			// Capture pane content if available.
			if session.ZellijPane != "" && lines > 0 {
				content, err := app.PaneManager.CapturePaneContent(session.ZellijPane, lines)
				if err != nil {
					fmt.Printf("\n  (Could not capture pane content: %v)\n", err)
				} else {
					fmt.Printf("\n--- Pane Content (last %d lines) ---\n%s\n", lines, content)
				}
			}

			return nil
		},
	}

	cmd.Flags().Int("lines", 20, "Number of pane content lines to capture")

	return cmd
}
