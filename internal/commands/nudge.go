package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NudgeCmd returns the "nudge" command for sending a nudge to an agent.
func NudgeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "nudge <agent-name>",
		Short:   "Send nudge to agent",
		Long:    "Send a status-check nudge to an agent's Zellij pane.",
		GroupID: "MESSAGING",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			message, _ := cmd.Flags().GetString("message")

			if message == "" {
				message = fmt.Sprintf("[CLI] Manual nudge sent to agent %s. Please report status.", name)
			}

			if err := app.PaneManager.SendKeys(name, "\n"+message+"\n"); err != nil {
				return fmt.Errorf("nudge agent %q: %w", name, err)
			}

			fmt.Printf("Nudge sent to %s\n", name)
			return nil
		},
	}

	cmd.Flags().StringP("message", "m", "", "Custom nudge message")

	return cmd
}
