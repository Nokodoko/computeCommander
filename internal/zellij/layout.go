// Package zellij layout provides KDL layout file generation for the cmdr dashboard.
package zellij

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// generateTabHash creates a unique 8-char hex ID for a dashboard tab instance.
// Each tab gets its own hash even if multiple tabs share the same project dir.
func generateTabHash() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use PID + time nanos for uniqueness.
		return fmt.Sprintf("%08x", os.Getpid())
	}
	return fmt.Sprintf("%x", b)
}

// LayoutOpts configures the KDL layout generation.
type LayoutOpts struct {
	CmdrBinary       string // path to cmdr binary
	SessionPrefix    string // zellij session name prefix
	ProjectDir       string // project root directory
	AgentCommand     string // command to run in the agent session pane
	AgentWrapperPath string // path to generated agent wrapper script (overrides AgentCommand in pane)
	UseWrapper       bool   // when true, auto-generate a wrapper script for session-switch support
	SystemWide       bool   // when true, use system-wide paths and add Ctrl+K keybind
	ProjectID        string // optional project ID filter for --project flag in pane commands
	Version          string // version string shown in the tab name
	TabHash          string // unique hash for this tab instance (used by focus-tracker plugin)
}

// GenerateLayout creates the KDL layout file for the cmdr dashboard.
//
// Each panel is a real zellij pane. The layout produces:
//
//	+------+------------------------------------------+--------+
//	|      |                                          |        |
//	|  fp  |         (borderless, no header)          | Agents |
//	| (10%)|           agent session (67%)            | (23%)  |
//	|      |              (focused)                   |        |
//	+------+------------------------------------------+--------+
//	| Event Log  |    Mail    |  Merge Queue  | Git Status    |
//	|   (25%)    |   (25%)   |    (25%)      |  (25%)        |
//	+-------------------------------------------------------------+
//
// Top row: 67% height — fp (10%) | agent (67%, borderless) | Agents (23%)
// Bottom row: 33% height — Event Log | Mail | Merge Queue | Git Status
//
// The fp pane uses fp-wrapper.sh which watches the focus-tracking active-cwd
// file so the file picker follows whichever agent pane is focused.
// The git-status pane also watches the same file to display the focused project.
//
// Zellij KDL split_direction semantics:
//   - "vertical"   = children arranged left-to-right (columns)
//   - "horizontal" = children arranged top-to-bottom (rows)

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

	agentPane := buildAgentPane(opts.AgentCommand, opts.AgentWrapperPath, opts.TabHash)

	// Use fp-wrapper.sh for focus-tracking support. The wrapper watches
	// /tmp/cmdr-<uid>-active-cwd and restarts fp when the project changes.
	fpWrapperPath := filepath.Join(projectDir, ".computecommander", "scripts", "fp-wrapper.sh")
	lazygitWrapperPath := filepath.Join(projectDir, ".computecommander", "scripts", "lazygit-wrapper.sh")
	if opts.SystemWide {
		home, _ := os.UserHomeDir()
		fpWrapperPath = filepath.Join(home, ".computecommander", "scripts", "fp-wrapper.sh")
		lazygitWrapperPath = filepath.Join(home, ".computecommander", "scripts", "lazygit-wrapper.sh")
	}

	// Build optional --project flag for pane commands.
	projectFlag := ""
	if opts.ProjectID != "" {
		projectFlag = fmt.Sprintf(` "--project" "%s"`, opts.ProjectID)
	}

	tabName := "[CMDR] Dashboard"
	if opts.Version != "" {
		tabName = fmt.Sprintf("[CMDR] Dashboard v%s", opts.Version)
	}

	tabHash := opts.TabHash

	return fmt.Sprintf(`layout {
    cwd "%s"
    tab name="%s" {
        pane size=1 borderless=true {
            plugin location="compact-bar"
        }
        pane size=1 borderless=true {
            plugin location="file:~/.config/zellij/plugins/focus_tracker_v14.wasm" {
                tab_hash "%s"
                project_dir "%s"
            }
        }
        pane split_direction="horizontal" {
            pane split_direction="vertical" size="67%%" {
                pane size="10%%" {
                    command "bash"
                    args "%s" "%s"
                }
%s
                pane name="Agents" size="23%%" {
                    command "%s"
                    args "status" "--pane"%s
                }
            }
            pane split_direction="vertical" size="33%%" {
                pane name="Event Log" size="20%%" {
                    command "%s"
                    args "feed" "--pane"%s
                }
                pane name="Mail" size="20%%" {
                    command "%s"
                    args "mail" "list" "--pane"%s
                }
                pane name="Evals" size="20%%" {
                    command "%s"
                    args "evals" "--pane"%s
                }
                pane name="Merge Queue" size="20%%" {
                    command "%s"
                    args "merge" "list" "--pane"%s
                }
                pane name="LazyGit" size="20%%" {
                    command "bash"
                    args "%s" "%s"
                }
            }
        }
    }
}
`, projectDir, tabName, tabHash, projectDir, fpWrapperPath, projectDir, agentPane, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, lazygitWrapperPath, projectDir)
}


// buildAgentPane returns the KDL block for the center agent session pane.
// If wrapperPath is set, runs the wrapper script via bash for session-switch support.
// The tabHash is appended as an extra argument so that the terminal_command seen by
// zellij is unique per tab, enabling the focus-tracker plugin to distinguish between
// multiple instances of the same wrapper script via pgrep.
// Falls back to running agentCmd directly, or a plain shell pane if both are empty.
func buildAgentPane(agentCmd, wrapperPath, tabHash string) string {
	// NOTE: The returned string is inserted via %s into GenerateLayout's Sprintf.
	// Sprintf does NOT re-process %s substitutions, so use literal "%" (not "%%").
	if wrapperPath != "" {
		return fmt.Sprintf("                pane size=\"67%%\" focus=true borderless=true {\n                    command \"bash\"\n                    args \"%s\" \"%s\"\n                }", wrapperPath, tabHash)
	}

	if agentCmd == "" {
		return "                pane size=\"67%\" focus=true borderless=true"
	}

	parts := strings.Fields(agentCmd)
	if len(parts) == 0 {
		return "                pane size=\"67%\" focus=true borderless=true"
	}

	cmd := parts[0]
	if len(parts) == 1 {
		return fmt.Sprintf("                pane size=\"67%%\" focus=true borderless=true {\n                    command \"%s\"\n                }", cmd)
	}

	quotedArgs := make([]string, len(parts)-1)
	for i, arg := range parts[1:] {
		quotedArgs[i] = fmt.Sprintf("\"%s\"", arg)
	}
	argsStr := strings.Join(quotedArgs, " ")

	return fmt.Sprintf("                pane size=\"67%%\" focus=true borderless=true {\n                    command \"%s\"\n                    args %s\n                }", cmd, argsStr)
}

// homeDir returns the user's home directory, falling back to ".".
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// WriteAgentWrapper generates the agent pane wrapper script at
// <dir>/.computecommander/scripts/cmdr-agent-wrapper.sh and returns its path.
// The script loops, resuming a queued session from the switch file when present,
// or running agentCmd otherwise.
func WriteAgentWrapper(dir, agentCmd, tabHash string) (string, error) {
	scriptDir := filepath.Join(dir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "cmdr-agent-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — agent pane wrapper with session switch support.
# Do not edit manually; regenerated on each cmdr launch.
set -uo pipefail

SWITCH_DIR=%q
AGENT_CMD=%q
INITIAL_DIR=%q
TAB_HASH=%q
export CMDR_TAB_HASH="$TAB_HASH"
AGENT_PID=""
LAST_TRACKED_PROJECT=""

# Per-tab CWD file: unique per tab instance (hash generated at layout time).
TAB_CWD="/tmp/cmdr-$(id -u)-${TAB_HASH}-cwd"

# Per-tab switch file: prevents race condition between multiple dashboard tabs.
SWITCH_FILE="${SWITCH_DIR}/session-switch-${TAB_HASH}"
# Global switch file: checked as fallback for external tools.
# Claimed atomically via mv to prevent race conditions between tabs.
GLOBAL_SWITCH_FILE="${SWITCH_DIR}/session-switch"
CLAIMED_SWITCH_FILE="${SWITCH_DIR}/session-switch-claimed-${TAB_HASH}"

# Write initial project dir so fp/lazygit start correctly.
echo "$INITIAL_DIR" > "$TAB_CWD"
LAST_TRACKED_PROJECT="$INITIAL_DIR"

# Set terminal title so focus-tracker plugin can read the project path
# from PaneInfo.title without any pgrep or /proc lookups.
printf '\033]2;CMDR:%%s\007' "$INITIAL_DIR"

# Kill the running agent process and ALL its descendants.
kill_agent() {
    if [ -n "$AGENT_PID" ] && kill -0 "$AGENT_PID" 2>/dev/null; then
        # Kill child processes first (handles eval subshell + claude).
        local children
        children=$(pgrep -P "$AGENT_PID" 2>/dev/null)
        for cpid in $children; do
            kill -TERM "$cpid" 2>/dev/null
        done
        # Then kill the agent process itself.
        kill -TERM "$AGENT_PID" 2>/dev/null
        wait "$AGENT_PID" 2>/dev/null
        AGENT_PID=""
    fi
}

# Resolve a directory to its project root (git root), or return it as-is.
# Returns empty string if the resolved path is ~/.claude (not a real project).
resolve_project_root() {
    local dir="$1"
    [ -d "$dir" ] || return
    # ~/.claude is not a real project root — skip it entirely.
    if [ "$dir" = "$HOME/.claude" ] || [ "$dir" = "$HOME/.claude/" ]; then
        return
    fi
    local root
    root=$(cd "$dir" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null)
    if [ -n "$root" ] && [ "$root" != "$HOME/.claude" ]; then
        echo "$root"
    else
        # If git root is ~/.claude, the process is inside a claude workdir — not a project.
        # If no git root, use the raw directory (but still skip ~/.claude).
        if [ -z "$root" ]; then
            echo "$dir"
        fi
        # If root == ~/.claude, return nothing (skip update).
    fi
}

# Update the per-tab CWD file, tab name, and fp pane title when the project changes.
# Also recreates the CWD file if it was deleted (e.g., by a colliding tab exit).
update_project() {
    local project="$1"
    if [ -n "$project" ] && [ -d "$project" ]; then
        if [ "$project" != "$LAST_TRACKED_PROJECT" ] || [ ! -f "$TAB_CWD" ]; then
            echo "$project" > "$TAB_CWD"
            LAST_TRACKED_PROJECT="$project"
            # Update terminal title so focus-tracker plugin sees the new project path.
            printf '\033]2;CMDR:%%s\007' "$project"
            zellij action rename-tab "[CMDR] $(basename "$project")" 2>/dev/null || true
        fi
    fi
}

# Clean up child processes on exit. Don't delete the CWD file —
# stale files are harmless and get filtered by liveness checks.
trap 'kill_agent; exit 0' EXIT INT TERM

while true; do
    # Check per-tab switch file first (targeted).
    ACTIVE_SWITCH=""
    if [ -f "$SWITCH_FILE" ]; then
        ACTIVE_SWITCH="$SWITCH_FILE"
    elif [ -f "$GLOBAL_SWITCH_FILE" ]; then
        # Atomically claim the global switch file via rename.
        # Only one tab can successfully mv; losers see "No such file".
        if mv "$GLOBAL_SWITCH_FILE" "$CLAIMED_SWITCH_FILE" 2>/dev/null; then
            ACTIVE_SWITCH="$CLAIMED_SWITCH_FILE"
        fi
    fi

    if [ -n "$ACTIVE_SWITCH" ]; then
        # A session switch was requested — kill any running agent first.
        kill_agent

        project_path=$(sed -n '1p' "$ACTIVE_SWITCH")
        session_id=$(sed -n '2p' "$ACTIVE_SWITCH")
        rm -f "$ACTIVE_SWITCH"
        if [ -d "$project_path" ]; then
            cd "$project_path"
            # Update per-tab CWD so fp and lazygit follow the new project.
            update_project "$project_path"
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

    # Track the agent's actual working directory via /proc.
    # This catches project changes even without an explicit session-switch,
    # e.g. when the agent navigates to a new project during a session.
    if [ -n "$AGENT_PID" ] && kill -0 "$AGENT_PID" 2>/dev/null; then
        AGENT_CWD=$(readlink "/proc/$AGENT_PID/cwd" 2>/dev/null)
        if [ -n "$AGENT_CWD" ]; then
            PROJECT_ROOT=$(resolve_project_root "$AGENT_CWD")
            # Only update if resolve_project_root returned a valid path.
            # It returns empty when CWD is ~/.claude (not a real project).
            if [ -n "$PROJECT_ROOT" ]; then
                update_project "$PROJECT_ROOT"
            fi
        fi
    fi

    # Poll for the switch file while the agent is running.
    # Check every second so the switch feels near-instant.
    sleep 1
done
`, filepath.Join(homeDir(), ".computecommander"), agentCmd, dir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write agent wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteFPWrapper generates the fp (file picker) wrapper script that uses
// inotifywait for event-driven focus tracking, falling back to polling if
// inotifywait is unavailable. Returns the path to the generated script.
func WriteFPWrapper(dir, tabHash string) (string, error) {
	scriptDir := filepath.Join(dir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "fp-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — fp wrapper with event-driven focus tracking.
# Do not edit manually; regenerated on each cmdr launch.
# Uses inotifywait on a per-tab CWD file for instant, isolated updates.
set -uo pipefail

DEFAULT_DIR="${1:-%s}"
TAB_HASH=%q
export CMDR_TAB_HASH="$TAB_HASH"
FP_PID=""
HAS_INOTIFY=false
command -v inotifywait >/dev/null 2>&1 && HAS_INOTIFY=true

# Per-tab CWD file: hash generated at layout time, shared with agent-wrapper.
CWD_FILE="/tmp/cmdr-$(id -u)-${TAB_HASH}-cwd"

start_fp() {
    local dir="${1:-$DEFAULT_DIR}"
    [ -d "$dir" ] || dir="$DEFAULT_DIR"
    # Update the pane title to reflect the current project.
    # Uses terminal escape sequence so it works from within this pane
    # regardless of which pane has focus (unlike zellij action rename-pane).
    printf '\033]2;%%s\007' "$(basename "$dir")"
    fp "$dir" &
    FP_PID=$!
}

kill_fp() {
    if [ -n "$FP_PID" ] && kill -0 "$FP_PID" 2>/dev/null; then
        kill "$FP_PID" 2>/dev/null
        wait "$FP_PID" 2>/dev/null
        FP_PID=""
    fi
}

trap 'kill_fp; exit 0' EXIT INT TERM

# Start with the project dir from $1 (layout argument).
CURRENT_DIR="$DEFAULT_DIR"
start_fp "$CURRENT_DIR"

# Wait for the per-tab CWD file to appear (agent-wrapper creates it).
while [ ! -f "$CWD_FILE" ]; do sleep 1; done

while true; do
    if $HAS_INOTIFY; then
        inotifywait -qq -e close_write -t 10 "$CWD_FILE" 2>/dev/null || true
    else
        sleep 2
    fi

    if [ -f "$CWD_FILE" ]; then
        NEW_DIR="$(tr -d '\n' < "$CWD_FILE" 2>/dev/null)"
        if [ -n "$NEW_DIR" ] && [ "$NEW_DIR" != "$CURRENT_DIR" ] && [ -d "$NEW_DIR" ]; then
            CURRENT_DIR="$NEW_DIR"
            kill_fp
            # start_fp updates the pane title via terminal escape sequence (printf '\033]2;...\007')
            # which works because no KDL name= attribute overrides it.
            start_fp "$CURRENT_DIR"
        fi
    fi

    # Restart fp if it died.
    if [ -n "$FP_PID" ] && ! kill -0 "$FP_PID" 2>/dev/null; then
        wait "$FP_PID" 2>/dev/null
        start_fp "$CURRENT_DIR"
    fi
done
`, dir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write fp wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteLazygitWrapper generates the lazygit wrapper script that uses
// inotifywait for event-driven focus tracking, falling back to polling if
// inotifywait is unavailable. Returns the path to the generated script.
func WriteLazygitWrapper(dir, tabHash string) (string, error) {
	scriptDir := filepath.Join(dir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "lazygit-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — lazygit wrapper with event-driven focus tracking.
# Do not edit manually; regenerated on each cmdr launch.
# Uses inotifywait on a per-tab CWD file for instant, isolated updates.
set -uo pipefail

DEFAULT_DIR="${1:-%s}"
TAB_HASH=%q
export CMDR_TAB_HASH="$TAB_HASH"
LG_PID=""
HAS_INOTIFY=false
command -v inotifywait >/dev/null 2>&1 && HAS_INOTIFY=true

# Per-tab CWD file: hash generated at layout time, shared with agent-wrapper.
CWD_FILE="/tmp/cmdr-$(id -u)-${TAB_HASH}-cwd"

start_lg() {
    local dir="${1:-$DEFAULT_DIR}"
    [ -d "$dir" ] || dir="$DEFAULT_DIR"
    if [ -d "$dir/.git" ] || git -C "$dir" rev-parse --git-dir >/dev/null 2>&1; then
        lazygit -p "$dir" &
        LG_PID=$!
    else
        echo "Not a git repository: $dir"
        LG_PID=""
    fi
}

kill_lg() {
    if [ -n "$LG_PID" ] && kill -0 "$LG_PID" 2>/dev/null; then
        kill "$LG_PID" 2>/dev/null
        wait "$LG_PID" 2>/dev/null
        LG_PID=""
    fi
}

trap 'kill_lg; exit 0' EXIT INT TERM

# Start with the project dir from $1 (layout argument).
CURRENT_DIR="$DEFAULT_DIR"
start_lg "$CURRENT_DIR"

# Wait for the per-tab CWD file to appear (agent-wrapper creates it).
while [ ! -f "$CWD_FILE" ]; do sleep 1; done

while true; do
    if $HAS_INOTIFY; then
        inotifywait -qq -e close_write -t 10 "$CWD_FILE" 2>/dev/null || true
    else
        sleep 2
    fi

    if [ -f "$CWD_FILE" ]; then
        NEW_DIR="$(tr -d '\n' < "$CWD_FILE" 2>/dev/null)"
        if [ -n "$NEW_DIR" ] && [ "$NEW_DIR" != "$CURRENT_DIR" ] && [ -d "$NEW_DIR" ]; then
            CURRENT_DIR="$NEW_DIR"
            kill_lg
            start_lg "$CURRENT_DIR"
        fi
    fi

    if [ -n "$LG_PID" ] && ! kill -0 "$LG_PID" 2>/dev/null; then
        wait "$LG_PID" 2>/dev/null
        start_lg "$CURRENT_DIR"
    fi
done
`, dir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write lazygit wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteLayout generates and writes the KDL layout file to the given path.
// Also generates the fp-wrapper and focus-watcher scripts for focus-tracking.
// When opts.UseWrapper is true and opts.AgentCommand is set, it generates
// a wrapper script for session-switch support and uses that in the layout.
// Otherwise the agent command is embedded directly in the KDL layout.
func WriteLayout(path string, opts LayoutOpts) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	// Determine script generation directory.
	// When SystemWide, scripts go to ~/.computecommander/ (matching layout paths).
	// Otherwise they go to the project directory.
	scriptDir := projectDir
	if opts.SystemWide {
		if home, err := os.UserHomeDir(); err == nil {
			scriptDir = home
		}
	}

	// Generate a unique tab hash for this dashboard instance.
	// All three wrappers and the focus-tracker plugin share the same hash
	// so they all use the same per-tab CWD file.
	tabHash := generateTabHash()
	opts.TabHash = tabHash

	// Generate the fp-wrapper script for focus-tracking.
	if _, err := WriteFPWrapper(scriptDir, tabHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate fp-wrapper: %v\n", err)
	}

	// Generate the lazygit-wrapper script for focus-tracking.
	if _, err := WriteLazygitWrapper(scriptDir, tabHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate lazygit-wrapper: %v\n", err)
	}

	// Only generate the wrapper script when explicitly requested.
	if opts.UseWrapper && opts.AgentCommand != "" && opts.AgentWrapperPath == "" {
		wrapperPath, err := WriteAgentWrapper(projectDir, opts.AgentCommand, tabHash)
		if err != nil {
			// Non-fatal: fall back to running the agent command directly.
			_ = err
		} else {
			opts.AgentWrapperPath = wrapperPath
		}

		// When SystemWide, also clean up any stale agent-wrapper at the
		// system-wide scripts dir to prevent old versions from interfering.
		if opts.SystemWide {
			staleWrapper := filepath.Join(scriptDir, ".computecommander", "scripts", "cmdr-agent-wrapper.sh")
			if _, err := os.Stat(staleWrapper); err == nil {
				_ = os.Remove(staleWrapper)
			}
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

// SystemLayoutPath returns the system-wide layout path at ~/.computecommander/layouts/.
func SystemLayoutPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".computecommander", "layouts", "cmdr-dashboard.kdl")
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
