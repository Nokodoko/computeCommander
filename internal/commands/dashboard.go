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
	var projectFlag string
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
				return app.RunDashboardWithProject(cmd.Context(), agentCmd, projectFlag)
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

			// Use "cmdr" from PATH for dashboard sub-panes so the layout
			// always references the installed binary, not the build artifact.
			cmdrBin := "cmdr"

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
				UseWrapper:   true,
				SystemWide:   app.Config != nil && app.Config.Version >= 2,
				ProjectID:    projectFlag,
				Version:      app.Version,
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
			// Inject resolved endpoints/auth into the spawned environment so
			// every pane subprocess (cmdr openbrain --pane, cmdr tg --pane, ...)
			// dials the correct endpoints regardless of whether the launching
			// shell was interactive. A non-interactive launch otherwise lacks
			// CMDR_TG_GATEWAY (interactive-only) and OB_API_KEY (login-only),
			// causing OB1 401 / TG localhost failures in the pane processes.
			c.Env = os.Environ()
			if app.Config != nil {
				if gw := app.Config.TrustGraph.ResolveGatewayURL(); gw != "" {
					c.Env = append(c.Env, "CMDR_TG_GATEWAY="+gw)
				}
			}
			if key := os.Getenv("OB_API_KEY"); key != "" {
				c.Env = append(c.Env, "OB_API_KEY="+key)
			}
			if err := c.Run(); err != nil {
				return err
			}

			// Pane frames are now enabled globally (pane_frames true in config).
			// No toggle needed — frames stay on by default.

			// Register the primary agent session and emit a dashboard event
			// so the status/feed panes have data to display immediately.
			registerDashboardSession(cmd.Context(), app, projectDir)

			return nil
		},
	}
	cmd.Flags().BoolVar(&useTUI, "tui", false, "Use the in-process bubbletea TUI instead of the KDL zellij layout")
	cmd.Flags().StringVar(&agentCmd, "agent-cmd", "", "Override the agent command (default: from config or built-in)")
	cmd.Flags().StringVar(&projectFlag, "project", "", "Filter dashboard to a specific project ID")
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
// It also cleans up old "primary" dashboard sessions to prevent unbounded
// accumulation in the sessions table.
// Errors are silently ignored -- this is best-effort telemetry.
func registerDashboardSession(ctx context.Context, app *App, projectDir string) {
	if app.DB == nil {
		return
	}

	now := time.Now()
	runID := fmt.Sprintf("run-dash-%d", now.UnixNano())
	sessionID := fmt.Sprintf("dash-%d", now.UnixNano())

	// Clean up old completed dashboard sessions. Each dashboard launch inserts
	// a "primary" session; without cleanup these accumulate indefinitely and
	// clutter the Agents pane. Keep only the 3 most recent for history.
	_ = app.DB.Exec(ctx,
		`DELETE FROM sessions WHERE agent_name = 'primary' AND task_id = 'dashboard'
		 AND id NOT IN (
			SELECT id FROM sessions WHERE agent_name = 'primary' AND task_id = 'dashboard'
			ORDER BY started_at DESC LIMIT 3
		 )`,
	)

	// Create a run so the foreign-key on sessions/events is satisfied.
	_ = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO runs (id, started_at, agent_count, status)
		VALUES (?, ?, 1, 'active')`,
		runID, now,
	)

	// Mark any previous "primary" working sessions as completed so the new
	// session is the only working one.
	_ = app.DB.Exec(ctx,
		`UPDATE sessions SET state = 'completed' WHERE agent_name = 'primary' AND state = 'working'`,
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

