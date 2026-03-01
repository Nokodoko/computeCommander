package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/tui"
)

// sessionManager is a package-level session manager shared across session commands.
// In a full implementation, this would be stored in the App struct and initialized
// at startup. For now, we use a package-level instance.
var sessionManager = tui.NewSessionManager()

// SessionCmd returns the "session" command group for directory session management.
func SessionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Short:   "Session management",
		GroupID: "CORE",
	}

	cmd.AddCommand(SessionListCmd(app))
	cmd.AddCommand(SessionSwitchCmd(app))
	cmd.AddCommand(SessionStopCmd(app))

	return cmd
}

// FpCmd returns the "fp" command for opening/toggling the file picker pane.
func FpCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fp",
		Short: "Open/toggle file picker pane",
		Long:  "Open or toggle the file picker pane for directory navigation and session launching.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			startPath, _ := cmd.Flags().GetString("path")
			if startPath == "" {
				startPath, _ = os.Getwd()
			}

			return tui.RunFilePicker(startPath)
		},
	}

	cmd.Flags().String("path", "", "Start browsing from directory (default: cwd)")
	return cmd
}

// SessionListCmd returns the "session list" command.
func SessionListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all active directory sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			sessions := sessionManager.ListSessions(false)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":  true,
					"command":  "session list",
					"sessions": sessions,
					"count":    len(sessions),
				})
			}

			if len(sessions) == 0 {
				fmt.Println("No active sessions.")
				return nil
			}

			fmt.Printf("%-12s %-30s %-10s %-8s\n", "ID", "DIRECTORY", "RUNTIME", "ACTIVE")
			for _, s := range sessions {
				active := "no"
				if s.Active {
					active = "yes"
				}
				fmt.Printf("%-12s %-30s %-10s %-8s\n",
					truncate(s.ID, 12),
					truncate(s.Directory, 30),
					truncate(s.Runtime, 10),
					active,
				)
			}
			fmt.Printf("\nTotal: %d session(s)\n", len(sessions))
			return nil
		},
	}
}

// SessionSwitchCmd returns the "session switch" command.
func SessionSwitchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <id|path>",
		Short: "Switch agent_session pane to a different directory session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			create, _ := cmd.Flags().GetBool("create")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			// Try to switch to existing session.
			sess := sessionManager.SwitchSession(target)
			if sess != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": true,
						"command": "session switch",
						"session": sess,
					})
				}
				fmt.Printf("Switched to session %s (%s)\n", sess.ID, sess.Directory)
				return nil
			}

			// No existing session. Create one if --create flag is set.
			if create {
				runtime := "claude"
				if app.Config != nil {
					runtime = app.Config.Defaults.Runtime
				}
				sess = sessionManager.CreateSession(target, runtime)
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": true,
						"command": "session switch",
						"created": true,
						"session": sess,
					})
				}
				fmt.Printf("Created and switched to new session %s (%s)\n", sess.ID, sess.Directory)
				return nil
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success": false,
					"command": "session switch",
					"error":   fmt.Sprintf("no session found for %q. Use --create to start one.", target),
				})
			}
			return fmt.Errorf("no session found for %q. Use --create to start one", target)
		},
	}

	cmd.Flags().Bool("create", false, "Create a new session if none exists for this directory")
	return cmd
}

// SessionStopCmd returns the "session stop" command.
func SessionStopCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <id|path>",
		Short: "Stop a directory session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if err := sessionManager.StopSession(target); err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "session stop",
						"error":   err.Error(),
					})
				}
				return err
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success": true,
					"command": "session stop",
				})
			}

			fmt.Printf("Session stopped for %s\n", target)
			return nil
		},
	}
}
