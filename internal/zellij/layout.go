// Package zellij layout provides KDL layout file generation for the cmdr dashboard.
package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LayoutOpts configures the KDL layout generation.
type LayoutOpts struct {
	CmdrBinary       string // path to cmdr binary
	SessionPrefix    string // zellij session name prefix
	ProjectDir       string // project root directory
	AgentCommand     string // command to run in the agent session pane
	AgentWrapperPath string // path to generated agent wrapper script (overrides AgentCommand in pane)
	UseWrapper       bool   // when true, auto-generate a wrapper script for session-switch support
}

// GenerateLayout creates the KDL layout file for the cmdr dashboard.
//
// Each panel is a real zellij pane. The layout produces:
//
//	+------+------------------------------------------+--------+
//	|      |                                          |        |
//	|  fp  |         (borderless, no header)          | Agents |
//	| (10%)|           agent session (65%)            | (25%)  |
//	|      |              (focused)                   |        |
//	+------+------------------------------------------+--------+
//	| Event Log  |    Mail    |  Merge Queue  | Git Status    |
//	|   (25%)    |   (25%)   |    (25%)      |  (25%)        |
//	+-------------------------------------------------------------+
//
// Top row: 67% height — fp (10%) | agent (65%, borderless) | Agents (25%)
// Bottom row: 33% height — Event Log | Mail | Merge Queue | Git Status
//
// Zellij KDL split_direction semantics:
//   - "vertical"   = children arranged left-to-right (columns)
//   - "horizontal" = children arranged top-to-bottom (rows)
//
// Note: pane_frames cannot be set per-layout in zellij. The dashboard
// command toggles frames on after loading the tab via `zellij action toggle-pane-frames`.
func GenerateLayout(opts LayoutOpts) string {
	cmdrBin := opts.CmdrBinary
	if cmdrBin == "" {
		cmdrBin = "cmdr"
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	agentPane := buildAgentPane(opts.AgentCommand, opts.AgentWrapperPath)

	return fmt.Sprintf(`layout {
    cwd "%s"
    tab name="[CMDR] Dashboard" {
        pane split_direction="horizontal" {
            pane split_direction="vertical" size="67%%" {
                pane name="fp" size="10%%" {
                    command "fp"
                    args "%s"
                }
%s
                pane name="Agents" size="25%%" {
                    command "%s"
                    args "status" "--pane"
                }
            }
            pane split_direction="vertical" size="33%%" {
                pane name="Event Log" size="25%%" {
                    command "%s"
                    args "feed" "--pane"
                }
                pane name="Mail" size="25%%" {
                    command "%s"
                    args "mail" "list" "--pane"
                }
                pane name="Merge Queue" size="25%%" {
                    command "%s"
                    args "merge" "list" "--pane"
                }
                pane name="Git Status" size="25%%" {
                    command "%s"
                    args "git-status" "--pane"
                }
            }
        }
    }
}
`, projectDir, projectDir, agentPane, cmdrBin, cmdrBin, cmdrBin, cmdrBin, cmdrBin)
}

// buildAgentPane returns the KDL block for the center agent session pane.
// If wrapperPath is set, runs the wrapper script via bash for session-switch support.
// Falls back to running agentCmd directly, or a plain shell pane if both are empty.
func buildAgentPane(agentCmd, wrapperPath string) string {
	// NOTE: The returned string is inserted via %s into GenerateLayout's Sprintf.
	// Sprintf does NOT re-process %s substitutions, so use literal "%" (not "%%").
	if wrapperPath != "" {
		return fmt.Sprintf("                pane size=\"65%%\" focus=true borderless=true {\n                    command \"bash\"\n                    args \"%s\"\n                }", wrapperPath)
	}

	if agentCmd == "" {
		return "                pane size=\"65%\" focus=true borderless=true"
	}

	parts := strings.Fields(agentCmd)
	if len(parts) == 0 {
		return "                pane size=\"65%\" focus=true borderless=true"
	}

	cmd := parts[0]
	if len(parts) == 1 {
		return fmt.Sprintf("                pane size=\"65%%\" focus=true borderless=true {\n                    command \"%s\"\n                }", cmd)
	}

	quotedArgs := make([]string, len(parts)-1)
	for i, arg := range parts[1:] {
		quotedArgs[i] = fmt.Sprintf("\"%s\"", arg)
	}
	argsStr := strings.Join(quotedArgs, " ")

	return fmt.Sprintf("                pane size=\"65%%\" focus=true borderless=true {\n                    command \"%s\"\n                    args %s\n                }", cmd, argsStr)
}

// WriteAgentWrapper generates the agent pane wrapper script at
// <dir>/.computecommander/scripts/cmdr-agent-wrapper.sh and returns its path.
// The script loops, resuming a queued session from the switch file when present,
// or running agentCmd otherwise.
func WriteAgentWrapper(dir, agentCmd string) (string, error) {
	scriptDir := filepath.Join(dir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "cmdr-agent-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — agent pane wrapper with session switch support.
# Do not edit manually; regenerated on each cmdr launch.
set -uo pipefail

SWITCH_FILE=%q
AGENT_CMD=%q
AGENT_PID=""

# Kill the running agent process (and its children) when we detect a switch.
kill_agent() {
    if [ -n "$AGENT_PID" ] && kill -0 "$AGENT_PID" 2>/dev/null; then
        # Kill the entire process group spawned by the agent.
        kill -TERM -- -"$AGENT_PID" 2>/dev/null || kill -TERM "$AGENT_PID" 2>/dev/null
        wait "$AGENT_PID" 2>/dev/null
        AGENT_PID=""
    fi
}

# Ensure we clean up child processes on exit.
trap 'kill_agent; exit 0' EXIT INT TERM

while true; do
    if [ -f "$SWITCH_FILE" ]; then
        # A session switch was requested — kill any running agent first.
        kill_agent

        project_path=$(sed -n '1p' "$SWITCH_FILE")
        session_id=$(sed -n '2p' "$SWITCH_FILE")
        rm -f "$SWITCH_FILE"
        if [ -d "$project_path" ]; then
            cd "$project_path"
            printf '\033[36mResuming session %%s in %%s...\033[0m\n' "$session_id" "$project_path"
            claude --resume "$session_id" --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit &
            AGENT_PID=$!
        else
            printf '\033[31mDirectory not found: %%s\033[0m\n' "$project_path"
            sleep 2
            continue
        fi
    elif [ -z "$AGENT_PID" ] || ! kill -0 "$AGENT_PID" 2>/dev/null; then
        # No agent running and no switch file — start the default agent.
        if [ -n "$AGENT_PID" ]; then
            # Previous agent exited normally.
            wait "$AGENT_PID" 2>/dev/null
            AGENT_PID=""
            printf '\n\033[2mSession ended. Restarting in 2s...\033[0m\n'
            sleep 2
        fi
        eval "$AGENT_CMD" &
        AGENT_PID=$!
    fi

    # Poll for the switch file while the agent is running.
    # Check every second so the switch feels near-instant.
    sleep 1
done
`, filepath.Join(dir, ".computecommander", "session-switch"), agentCmd)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write agent wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteLayout generates and writes the KDL layout file to the given path.
// When opts.UseWrapper is true and opts.AgentCommand is set, it generates
// a wrapper script for session-switch support and uses that in the layout.
// Otherwise the agent command is embedded directly in the KDL layout.
func WriteLayout(path string, opts LayoutOpts) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}

	// Only generate the wrapper script when explicitly requested.
	if opts.UseWrapper && opts.AgentCommand != "" && opts.AgentWrapperPath == "" {
		projectDir := opts.ProjectDir
		if projectDir == "" {
			projectDir, _ = os.Getwd()
		}
		wrapperPath, err := WriteAgentWrapper(projectDir, opts.AgentCommand)
		if err != nil {
			// Non-fatal: fall back to running the agent command directly.
			_ = err
		} else {
			opts.AgentWrapperPath = wrapperPath
		}
	}

	content := GenerateLayout(opts)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write layout file: %w", err)
	}

	return nil
}

// DefaultLayoutPath returns the default path for the dashboard layout file.
func DefaultLayoutPath() string {
	return filepath.Join(".computecommander", "layouts", "cmdr-dashboard.kdl")
}

// SessionOpts configures a zellij session launch.
type SessionOpts struct {
	SessionName string // zellij session name
	LayoutPath  string // path to the KDL layout file
	WorkDir     string // working directory
}

// LaunchSession opens the cmdr dashboard layout in zellij.
//
// When already inside zellij (the normal case: wezterm runs zellij),
// adds a new tab to the EXISTING session via `zellij action new-tab`.
// This avoids nesting and lets the user's keybinds work normally.
//
// When outside zellij, creates a standalone session with --layout.
func LaunchSession(opts SessionOpts) error {
	zellijBin, err := exec.LookPath("zellij")
	if err != nil {
		return fmt.Errorf("zellij not found in PATH: %w", err)
	}

	if opts.SessionName == "" {
		opts.SessionName = "cc-dashboard"
	}

	// Inside zellij: add a tab in the current session (no nesting).
	if IsInsideZellij() {
		cmd := exec.Command(zellijBin, "action", "new-tab", "--layout", opts.LayoutPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Outside zellij: create a new session.
	cmd := exec.Command(zellijBin, "--session", opts.SessionName, "--layout", opts.LayoutPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	return cmd.Run()
}

// IsInsideZellij returns true if the process is running inside a zellij session.
func IsInsideZellij() bool {
	return os.Getenv("ZELLIJ") != "" || os.Getenv("ZELLIJ_SESSION_NAME") != ""
}

// ZellijAvailable returns true if the zellij binary is on PATH.
func ZellijAvailable() bool {
	_, err := exec.LookPath("zellij")
	return err == nil
}
