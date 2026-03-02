package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
)

// StatusCmd returns the "status" command for fleet overview.
// Enhanced: now includes UI process detection and DB status.
func StatusCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show DB status + UI status + fleet overview",
		Long:    "Display database status, UI process status, and an overview of all agent sessions.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			capability, _ := cmd.Flags().GetString("capability")
			state, _ := cmd.Flags().GetString("state")
			pane, _ := cmd.Flags().GetBool("pane")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			opts := agents.ListOpts{}
			if capability != "" {
				opts.Capability = agents.Capability(capability)
			}
			if state != "" {
				opts.State = agents.SessionState(state)
			}

			if paneMode {
				return runStatusPane(cmd, app, opts)
			}

			sessions, err := app.Spawner.ListSessions(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}

			if jsonOut {
				// Detect UI process status.
				uiRunning, uiSession, uiUptime := detectUIStatus(app)
				dbRunning := app.DB != nil
				dbDriver := "unknown"
				dbPath := ""
				if app.Config != nil {
					dbDriver = app.Config.Database.Driver
					dbPath = app.Config.Database.SQLite.Path
				}
				result := map[string]any{
					"success": true,
					"command": "status",
					"db": map[string]any{
						"running": dbRunning,
						"driver":  dbDriver,
						"path":    dbPath,
					},
					"ui": map[string]any{
						"running": uiRunning,
						"session": uiSession,
						"uptime":  uiUptime,
					},
					"agents":  sessions,
					"count":   len(sessions),
					"version": app.Version,
				}
				if app.Config != nil {
					result["project"] = app.Config.Project.Name
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			// Pane mode: styled output for zellij dashboard Agents pane.
			if pane {
				return printAgentsPane(sessions)
			}

			// Full human-readable output.
			fmt.Println("=== cmdr status ===")
			fmt.Println()

			// Detect UI process status.
			uiRunning, uiSession, uiUptime := detectUIStatus(app)
			dbRunning := app.DB != nil
			dbDriver := "unknown"
			dbPath := ""
			if app.Config != nil {
				dbDriver = app.Config.Database.Driver
				dbPath = app.Config.Database.SQLite.Path
			}

			if dbRunning {
				fmt.Printf("DB:      running (%s)\n", dbDriver)
				if dbPath != "" {
					fmt.Printf("         %s\n", dbPath)
				}
			} else {
				fmt.Println("DB:      not running")
			}

			if uiRunning {
				fmt.Printf("UI:      running (session: %s, uptime: %ds)\n", uiSession, uiUptime)
			} else {
				fmt.Println("UI:      not running")
			}

			fmt.Printf("Version: %s\n", app.Version)
			if app.Config != nil {
				fmt.Printf("Project: %s\n", app.Config.Project.Name)
			}
			fmt.Println()

			if len(sessions) == 0 {
				fmt.Println("No active agents.")
				return nil
			}

			fmt.Printf("%-14s %-12s %-10s %-14s %-8s\n", "NAME", "CAPABILITY", "STATE", "TASK", "RUNTIME")
			for _, s := range sessions {
				fmt.Printf("%-14s %-12s %-10s %-14s %-8s\n",
					truncate(s.AgentName, 14),
					truncate(string(s.Capability), 12),
					truncate(string(s.State), 10),
					truncate(s.TaskID, 14),
					truncate(string(s.Runtime), 8),
				)
			}
			fmt.Printf("\nTotal: %d agent(s)\n", len(sessions))
			return nil
		},
	}

	cmd.Flags().String("capability", "", "Filter by capability")
	cmd.Flags().String("state", "", "Filter by state")
	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runStatusPane runs the status command in long-lived pane mode, refreshing periodically.
func runStatusPane(cmd *cobra.Command, app *App, opts agents.ListOpts) error {
	ctx := cmd.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	render := func() {
		clearScreen()
		sessions, err := app.Spawner.ListSessions(ctx, opts)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}

		if len(sessions) == 0 {
			fmt.Println("\033[2mNo active agents.\033[0m")
			return
		}

		fmt.Printf("\033[2m%-14s %-12s %-10s %-14s\033[0m\n", "NAME", "CAPABILITY", "STATE", "TASK")
		for _, s := range sessions {
			stateColor := "\033[32m" // green for working
			switch s.State {
			case agents.StateZombie:
				stateColor = "\033[31m" // red
			case agents.StateStalled:
				stateColor = "\033[33m" // yellow
			case agents.StateCompleted:
				stateColor = "\033[2m" // dim
			case agents.StateBooting:
				stateColor = "\033[36m" // cyan
			}
			fmt.Printf("%-14s %-12s %s%-10s\033[0m %-14s\n",
				truncate(s.AgentName, 14),
				truncate(string(s.Capability), 12),
				stateColor,
				truncate(string(s.State), 10),
				truncate(s.TaskID, 14),
			)
		}
		fmt.Printf("\n\033[2mTotal: %d agent(s)\033[0m\n", len(sessions))
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			render()
		}
	}
}

// ANSI color helpers.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiCyan    = "\033[36m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiRed     = "\033[31m"
	ansiMagenta = "\033[35m"
)

// dashboardStartTime returns the start time of the current dashboard session
// by reading the modification time of the cmdr.lock file. Returns zero time
// if the lock file cannot be read.
func dashboardStartTime() time.Time {
	lockPath := filepath.Join(".computecommander", "cmdr.lock")
	info, err := os.Stat(lockPath)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// filterPaneSessions removes completed agents from previous dashboard sessions.
// Agents that completed during the current session (started after dashStart) are
// kept visible. Agents that were already completed before the dashboard launched
// are excluded so a fresh session starts with a clean Agents pane.
func filterPaneSessions(sessions []*agents.AgentSession, dashStart time.Time) []*agents.AgentSession {
	if dashStart.IsZero() {
		return sessions
	}
	filtered := make([]*agents.AgentSession, 0, len(sessions))
	for _, s := range sessions {
		// Keep all non-completed agents (working, booting, stalled, zombie).
		if s.State != agents.StateCompleted {
			filtered = append(filtered, s)
			continue
		}
		// Keep completed agents only if they started during or after the current
		// dashboard session (i.e. they completed during this session).
		if !s.StartedAt.Before(dashStart) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// printAgentsPane renders the styled Agents pane matching the dashboard screenshot.
// It shows the header and column names at the top, then the most recent agents
// (tail of the list) so the latest activity is always visible without scrolling.
// Completed agents from previous dashboard sessions are filtered out so a new
// session starts with a clean pane.
func printAgentsPane(sessions []*agents.AgentSession) error {
	// Filter out completed agents from previous dashboard sessions.
	dashStart := dashboardStartTime()
	sessions = filterPaneSessions(sessions, dashStart)

	// Count active agents.
	active := 0
	for _, s := range sessions {
		if s.State == agents.StateWorking || s.State == agents.StateBooting {
			active++
		}
	}

	// Styled header: ── Agents (N active) ──
	header := fmt.Sprintf("%s%s── Agents (%d active) ──%s", ansiBold, ansiCyan, active, ansiReset)
	fmt.Println(header)

	if len(sessions) == 0 {
		fmt.Printf("\n  %sNo agents running.%s\n", ansiDim, ansiReset)
		fmt.Printf("%sPress / to search, Enter to refresh%s\n", ansiDim, ansiReset)
		return nil
	}

	// Table header.
	fmt.Printf(" %s%-16s %-12s %-10s %-10s %-14s%s\n",
		ansiDim, "Name", "Capability", "State", "Duration", "Task", ansiReset)

	// Detect terminal height to show only what fits.
	// Reserve 4 lines: header, column header, footer, + zellij pane border.
	termHeight := terminalHeight()
	if termHeight <= 0 {
		termHeight = 15 // conservative fallback for zellij pane
	}
	maxRows := termHeight - 4
	if maxRows < 1 {
		maxRows = 1
	}

	// Show only the most recent agents (tail of list).
	start := 0
	if len(sessions) > maxRows {
		start = len(sessions) - maxRows
	}

	for _, s := range sessions[start:] {
		stateIcon, stateColor := stateStyle(s.State)
		dur := formatAgentDuration(s)

		fmt.Printf(" %s%-16s%s %-12s %s%s%-10s%s %-10s %-14s\n",
			ansiBold, truncate(s.AgentName, 16), ansiReset,
			truncate(string(s.Capability), 12),
			stateColor, stateIcon, truncate(string(s.State), 9), ansiReset,
			dur,
			truncate(s.TaskID, 14),
		)
	}

	fmt.Printf("%sPress / to search, Enter to refresh%s\n", ansiDim, ansiReset)
	return nil
}

// stateStyle returns an icon and ANSI color for agent states.
func stateStyle(state agents.SessionState) (icon string, color string) {
	switch state {
	case agents.StateWorking:
		return "● ", ansiGreen
	case agents.StateBooting:
		return "◌ ", ansiYellow
	case agents.StateCompleted:
		return "✔ ", ansiGreen
	case agents.StateStalled:
		return "⚠ ", ansiYellow
	case agents.StateZombie:
		return "✖ ", ansiRed
	default:
		return "○ ", ansiDim
	}
}

// formatAgentDuration returns a human-readable duration string for a session.
func formatAgentDuration(s *agents.AgentSession) string {
	if s.StartedAt.IsZero() {
		return "-"
	}
	d := time.Since(s.StartedAt)
	if d < 0 {
		d = 0
	}
	m := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

// terminalHeight returns the terminal height in rows, or 0 if unavailable.
func terminalHeight() int {
	type winsize struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	var ws winsize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if err != 0 || ws.Row == 0 {
		return 0
	}
	return int(ws.Row)
}

// detectUIStatus checks if the zellij UI session is running.
func detectUIStatus(app *App) (running bool, session string, uptime int64) {
	// Check for lock file.
	lockPath := filepath.Join(".computecommander", "cmdr.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false, "", 0
	}

	parts := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(parts) < 1 {
		return false, "", 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return false, "", 0
	}

	// Check if PID is alive.
	if !isProcessAlive(pid) {
		// Stale lock file.
		_ = os.Remove(lockPath)
		return false, "", 0
	}

	prefix := "cc"
	if app.Config != nil {
		prefix = app.Config.Zellij.SessionPrefix
	}
	session = fmt.Sprintf("%s-dashboard", prefix)

	return true, session, 0
}

// isProcessAlive checks if a process with the given PID is running.
func isProcessAlive(pid int) bool {
	// Use kill -0 to check if process exists without sending a signal.
	cmd := exec.Command("kill", "-0", strconv.Itoa(pid))
	return cmd.Run() == nil
}

// truncate shortens a string to maxLen, adding ".." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 2 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}
