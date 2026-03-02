package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/tui"
	"github.com/noko/computecommander/internal/zellij"
)

// DashboardCmd returns the "dashboard" command that launches the KDL
// multi-pane zellij layout by default.
//
// When inside zellij (which is always the case for this user), it runs
// `zellij action new-tab --layout <path>` to open the dashboard as a new tab
// in the existing session. Pass --tui to use the in-process bubbletea TUI instead.
func DashboardCmd(app *App) *cobra.Command {
	var useTUI bool
	var agentCmd string
	cmd := &cobra.Command{
		Use:     "dashboard",
		Short:   "Launch the cmdr dashboard",
		Long: `Launch the KDL multi-pane dashboard layout in zellij.

By default, opens a new zellij tab with the multi-pane KDL layout.
Pass --tui to use the in-process bubbletea TUI instead.
Use --agent-cmd to override the default agent command in the center pane.`,
		GroupID: "CORE",
		Aliases: []string{"dash"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Explicit --tui flag: use bubbletea TUI.
			if useTUI {
				return app.RunDashboardWithCmd(cmd.Context(), agentCmd)
			}

			// Default: KDL layout via zellij.
			if !zellijAvailable() {
				fmt.Fprintln(os.Stderr, "zellij not found in PATH; falling back to bubbletea TUI")
				return app.RunDashboardWithCmd(cmd.Context(), agentCmd)
			}

			// Resolve the agent command: CLI flag > config > default.
			resolvedAgentCmd := agentCmd
			if resolvedAgentCmd == "" && app.Config != nil && app.Config.Agents.DefaultCommand != "" {
				resolvedAgentCmd = app.Config.Agents.DefaultCommand
			}
			if resolvedAgentCmd == "" {
				resolvedAgentCmd = tui.DefaultAgentCommand
			}

			// Resolve the cmdr binary path for dashboard sub-panes.
			cmdrBin, err := os.Executable()
			if err != nil || cmdrBin == "" {
				cmdrBin = "cmdr"
			}

			// Resolve project directory.
			projectDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			// Resolve layout path from config, with fallback to project-local default.
			layoutPath := ""
			if app.Config != nil {
				layoutPath = app.Config.Zellij.DashboardLayout
			}
			if layoutPath == "" {
				layoutPath = filepath.Join(".computecommander", "layouts", "cmdr-dashboard.kdl")
			}

			// Expand tilde in layout path.
			layoutPath = expandTildePath(layoutPath)

			// Make layout path absolute for the zellij command.
			absLayout, err := filepath.Abs(layoutPath)
			if err != nil {
				absLayout = layoutPath
			}

			// (Re)generate the layout file with the resolved agent command.
			// This ensures the KDL layout always contains the correct
			// claude CLI command in the main agent session pane.
			if writeErr := zellij.WriteLayout(absLayout, zellij.LayoutOpts{
				CmdrBinary:   cmdrBin,
				ProjectDir:   projectDir,
				AgentCommand: resolvedAgentCmd,
			}); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to regenerate layout: %v\n", writeErr)
				// Fall through — if the file already exists we can still use it.
			}

			// Verify the layout file exists.
			if _, err := os.Stat(absLayout); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Layout file not found: %s\nFalling back to bubbletea TUI.\n", absLayout)
				return app.RunDashboardWithCmd(cmd.Context(), agentCmd)
			}

			// Build and print the zellij command, then execute it.
			zellijArgs := []string{"action", "new-tab", "--layout", absLayout}
			fmt.Fprintf(os.Stderr, ">>> zellij %s\n", strings.Join(zellijArgs, " "))

			c := exec.CommandContext(cmd.Context(), "zellij", zellijArgs...)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}

			// Enable pane frames for the dashboard tab. The global config
			// has pane_frames: false, and zellij has no per-layout override.
			// Wait briefly for the new tab to become active, then toggle.
			time.Sleep(200 * time.Millisecond)
			toggle := exec.CommandContext(cmd.Context(), "zellij", "action", "toggle-pane-frames")
			toggle.Stdin = os.Stdin
			toggle.Stdout = os.Stdout
			toggle.Stderr = os.Stderr
			if err := toggle.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Note: could not enable pane frames: %v\n", err)
				fmt.Fprintln(os.Stderr, "Toggle manually with Ctrl+P then z")
			}

			// Register the primary agent session and emit a dashboard event
			// so the status/feed panes have data to display immediately.
			registerDashboardSession(cmd.Context(), app, projectDir)

			return nil
		},
	}
	cmd.Flags().BoolVar(&useTUI, "tui", false, "Use the in-process bubbletea TUI instead of the KDL zellij layout")
	cmd.Flags().StringVar(&agentCmd, "agent-cmd", "", "Override the agent command (default: from config or built-in)")
	return cmd
}

// expandTildePath replaces a leading ~ with the user's home directory.
func expandTildePath(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// zellijAvailable returns true if the zellij binary is on PATH.
func zellijAvailable() bool {
	_, err := exec.LookPath("zellij")
	return err == nil
}

// registerDashboardSession inserts a run, session, and event so the
// status/feed dashboard panes have data to display immediately.
// Errors are silently ignored — this is best-effort telemetry.
func registerDashboardSession(ctx context.Context, app *App, projectDir string) {
	if app.DB == nil {
		return
	}

	now := time.Now()
	runID := fmt.Sprintf("run-dash-%d", now.UnixNano())
	sessionID := fmt.Sprintf("dash-%d", now.UnixNano())

	// Create a run so the foreign-key on sessions/events is satisfied.
	_ = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO runs (id, started_at, agent_count, status)
		VALUES (?, ?, 1, 'active')`,
		runID, now,
	)

	// Insert a session for the primary agent running in the center pane.
	_ = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO sessions
			(id, agent_name, capability, worktree_path, branch_name,
			 task_id, zellij_pane, state, pid, parent_agent, depth, run_id,
			 started_at, last_activity, escalation_level, stalled_since,
			 transcript_path, runtime)
		VALUES (?, 'primary', 'lead', ?, '', 'dashboard', 'center', 'working',
			0, '', 0, ?, ?, ?, 0, NULL, '', 'claude')`,
		sessionID, projectDir, runID, now, now,
	)

	// Emit a dashboard_started event so the feed pane has something to show.
	_ = app.DB.Exec(ctx,
		`INSERT INTO events (agent_name, event_type, tool_name, data, level, run_id, created_at)
		VALUES ('system', 'dashboard_started', '', 'Dashboard launched', 'info', ?, ?)`,
		runID, now,
	)
}
