package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/zellij"
)

// HelpCmd returns the "help" command that shows help in a floating pane or stdout.
func HelpCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "help",
		Short:   "Show help (floating pane if UI is running, else stdout)",
		GroupID: "INFO",
		RunE: func(cmd *cobra.Command, args []string) error {
			helpText := `ComputeCommander (cmdr) - Agentic IDE for AI coding agent swarms

LIFECYCLE:
  cmdr                  Open the cmdr interface
  cmdr shutdown         Stop DB + close UI
  cmdr reset            Reset DB to empty + close UI
  cmdr restart          Restart DB + UI

INFORMATION:
  cmdr help             Show this help
  cmdr docs             Open documentation in browser
  cmdr status           Show DB + UI status
  cmdr version          Show version info
  cmdr update           Check for updates

DATA:
  cmdr export           Export all data as JSON
  cmdr backup           Backup database
  cmdr restore <path>   Restore from backup

UTILITY:
  cmdr shell            Open shell in cmdr pane
  cmdr feedback         Open feedback form
  cmdr support          Open support page
  cmdr clear            Clear logs

SETTINGS:
  cmdr theme            Theme management
  cmdr notifications    Notification settings
  cmdr analytics        Usage analytics
  cmdr integrations     Service connections
  cmdr automation       Workflow automation

NAVIGATION:
  cmdr fp               Toggle file picker pane
  cmdr session          Session management

LEADER KEY: Ctrl+Space + key
  ?  help       v  version    u  update     s  shell
  c  clear      e  export     r  restart    b  backup
  R  restore    f  feedback   h  support    t  theme
  n  notify     a  analytics  i  integrate  m  automate
  d  file pick  q  quit       A  access     p  plugins

Documentation: https://github.com/noko/computecommander
`
			// Try to show in floating pane if UI is running.
			if app.PaneManager != nil {
				_, err := app.PaneManager.CreatePane(zellij.CreatePaneOpts{
					Name:     "cmdr-help",
					Floating: true,
					Command:  []string{"sh", "-c", fmt.Sprintf("echo %q; read -n1", helpText)},
				})
				if err == nil {
					return nil
				}
			}

			// Fall back to stdout.
			fmt.Print(helpText)
			return nil
		},
	}
}

// DocsCmd returns the "docs" command that opens documentation in the default browser.
func DocsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "docs",
		Short:   "Open documentation in default browser",
		GroupID: "INFO",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := "https://github.com/noko/computecommander"
			if err := browser.OpenURL(url); err != nil {
				fmt.Printf("Could not open browser. Visit: %s\n", url)
			} else {
				fmt.Printf("Opened %s in browser.\n", url)
			}
			return nil
		},
	}
}

// VersionCmd returns the "version" command with release notes link.
func VersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show version + release notes link",
		GroupID: "INFO",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			releaseURL := fmt.Sprintf("https://github.com/noko/computecommander/releases/tag/v%s", app.Version)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":         true,
					"command":         "version",
					"version":         app.Version,
					"releaseNotesUrl": releaseURL,
				})
			}

			fmt.Printf("cmdr version %s\n", app.Version)
			fmt.Printf("Release notes: %s\n", releaseURL)
			return nil
		},
	}
}

// UpdateCmd returns the "update" command that checks for updates.
func UpdateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		Short:   "Check for updates",
		GroupID: "INFO",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			releaseURL := fmt.Sprintf("https://github.com/noko/computecommander/releases/tag/v%s", app.Version)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":         true,
					"command":         "update",
					"version":         app.Version,
					"releaseNotesUrl": releaseURL,
					"upToDate":        true,
				})
			}

			fmt.Printf("Current version: %s\n", app.Version)
			fmt.Printf("Release notes: %s\n", releaseURL)
			fmt.Println("You are up to date.")
			return nil
		},
	}
}
