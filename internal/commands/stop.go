package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
)

// StopCmd returns the "stop" command for terminating an agent.
func StopCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop <agent-name>",
		Short:   "Terminate agent",
		Long:    "Stop a running agent session by name. Use --force to kill immediately.",
		GroupID: "CORE",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			force, _ := cmd.Flags().GetBool("force")
			reason, _ := cmd.Flags().GetString("reason")

			err := app.Spawner.Stop(cmd.Context(), name, agents.StopOpts{
				Force:  force,
				Reason: reason,
			})
			if err != nil {
				return fmt.Errorf("stop agent %q: %w", name, err)
			}

			fmt.Printf("Stopped agent %q\n", name)
			return nil
		},
	}

	cmd.Flags().BoolP("force", "f", false, "Force-kill the agent process")
	cmd.Flags().String("reason", "", "Reason for stopping the agent")

	return cmd
}
