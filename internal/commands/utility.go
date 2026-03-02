package commands

import (
	"fmt"
	"os"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/internal/zellij"
)

// ShellCmd returns the "shell" command for opening a shell in the cmdr interface.
func ShellCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shell",
		Short:   "Open shell in cmdr interface pane",
		Long:    "Open a shell pane in the cmdr interface. Use --agent to open in an agent's worktree.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName, _ := cmd.Root().Flags().GetString("agent")

			var workDir string
			if agentName != "" {
				// Look up the agent's worktree path.
				sessions, err := app.Spawner.ListSessions(cmd.Context(), agents.ListOpts{})
				if err != nil {
					return fmt.Errorf("list sessions: %w", err)
				}
				for _, s := range sessions {
					if s.AgentName == agentName {
						workDir = s.WorktreePath
						break
					}
				}
				if workDir == "" {
					return fmt.Errorf("agent %q not found. Run `cmdr status` to see active agents", agentName)
				}
			}

			if app.PaneManager != nil {
				_, err := app.PaneManager.CreatePane(zellij.CreatePaneOpts{
					Name:     "cmdr-shell",
					Floating: true,
					WorkDir:  workDir,
					Command:  []string{getShell()},
				})
				if err == nil {
					return nil
				}
			}

			fmt.Println("Could not open shell pane. Run your shell manually.")
			return nil
		},
	}

	return cmd
}

// FeedbackCmd returns the "feedback" command that opens the feedback form.
func FeedbackCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "feedback",
		Short:   "Open feedback form in default browser",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := "https://github.com/noko/computecommander/issues/new?template=feedback.md"
			if err := browser.OpenURL(url); err != nil {
				fmt.Printf("Could not open browser. Visit: %s\n", url)
			} else {
				fmt.Printf("Opened feedback form in browser.\n")
			}
			return nil
		},
	}
}

// SupportCmd returns the "support" command that opens the support page.
func SupportCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "support",
		Short:   "Open support page in default browser",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := "https://github.com/noko/computecommander/discussions"
			if err := browser.OpenURL(url); err != nil {
				fmt.Printf("Could not open browser. Visit: %s\n", url)
			} else {
				fmt.Printf("Opened support page in browser.\n")
			}
			return nil
		},
	}
}

// getShell returns the user's preferred shell.
func getShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}
