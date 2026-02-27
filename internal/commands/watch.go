package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// WatchCmd returns the "watch" command for the watchdog daemon.
func WatchCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "watch",
		Short:   "Watchdog daemon (Tier 0)",
		Long:    "Start the watchdog daemon that monitors agent health, detects stalls, and sends nudges.",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Starting watchdog daemon...")
			return app.RunWatchdog(cmd.Context())
		},
	}
}
