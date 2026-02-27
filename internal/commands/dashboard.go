package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// DashboardCmd returns the "dashboard" command that spawns a new wezterm window
// with the zellij dashboard layout, falling back to the in-process TUI.
func DashboardCmd(app *App) *cobra.Command {
	var tuiOnly bool
	cmd := &cobra.Command{
		Use:     "dashboard",
		Short:   "Launch the cmdr dashboard in a new window",
		Long:    "Spawn a new wezterm window running the zellij dashboard layout with agent picker, mail, merge queue, events, and feed panes.\n\nUse --tui to force the in-process TUI (useful when running inside an existing terminal/pane).",
		GroupID: "CORE",
		Aliases: []string{"dash"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// --tui flag or CC_DASHBOARD_TUI env var forces in-process TUI
			if !tuiOnly {
				tuiOnly = os.Getenv("CC_DASHBOARD_TUI") == "1"
			}
			if !tuiOnly && app.Spawner != nil {
				err := app.Spawner.SpawnDashboard(cmd.Context())
				if err == nil {
					return nil
				}
				if app.WindowManager != nil {
					return err
				}
			}
			return app.RunDashboard(cmd.Context())
		},
	}
	cmd.Flags().BoolVar(&tuiOnly, "tui", false, "Force in-process TUI (skip wezterm window spawn)")
	return cmd
}
