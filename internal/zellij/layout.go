// Package zellij layout provides KDL layout file generation for the cmdr dashboard.
package zellij

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	TabHash          string // unique hash for this tab instance (shared by all wrapper scripts)
}

// GenerateLayout creates the KDL layout file for the cmdr dashboard.
//
// Each panel is a real zellij pane. The layout produces:
//
//	+-------+------------------------------------------------+----------+
//	|prompt |                                                |          |
//	|(1 row)|                                                | Agents   |
//	+-------+       Agent Session (borderless)               | (64%)    |
//	|Cal25% |            80% width                           |          |
//	+-------+            (focused)                           +----------+
//	|       |                                                | Jira     |
//	|  fp   |                                                | (36%)    |
//	|(rest) |                                                |          |
//	+-------+------------------------------------------------+----------+
//	| EvLog |  Evals  |  OB1   |  TG Viz  |     LazyGit     |
//	| (20%) |  (20%)  | (16%)  |  (20%)   |     (rest)      |
//	+-------+---------+--------+----------+-----------------+
//
// Top row: 75% height — left column (prompt+cal+fp, 7%) | agent (80%, borderless) | right column (Agents+Jira, 13%)
// Bottom row: 25% height — Event Log | Evals | OpenBrain | TrustGraph | LazyGit
//
// The fp pane uses fp-wrapper.sh which watches the per-tab CWD file
// so the file picker updates when the agent switches sessions.
// The lazygit pane also watches the same file to display the current project.
//
// Zellij KDL split_direction semantics:
//   - "vertical"   = children arranged left-to-right (columns)
//   - "horizontal" = children arranged top-to-bottom (rows)

// Note: pane_frames cannot be set per-layout in zellij. The dashboard
// command toggles frames on after loading the tab via `zellij action toggle-pane-frames`.
func GenerateLayout(opts LayoutOpts) string {
	home, _ := os.UserHomeDir()

	cmdrBin := opts.CmdrBinary
	if cmdrBin == "" {
		cmdrBin = "cmdr"
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	agentPane := buildAgentPane(opts.AgentCommand, opts.AgentWrapperPath, opts.TabHash)

	// Use fp-wrapper.sh for session-switch support. The wrapper watches
	// the per-tab CWD file and restarts fp when the project changes.
	fpWrapperPath := filepath.Join(projectDir, ".computecommander", "scripts", "fp-wrapper.sh")
	lazygitWrapperPath := filepath.Join(projectDir, ".computecommander", "scripts", "lazygit-wrapper.sh")
	if opts.SystemWide {
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

	// Resolve the focus-watcher path (Rust binary or bash script fallback).
	scriptDir := projectDir
	if opts.SystemWide {
		scriptDir = home
	}
	focusWatcherPath, _ := WriteFocusWatcher(scriptDir, projectDir, opts.TabHash)
	if focusWatcherPath == "" {
		focusWatcherPath = filepath.Join(projectDir, ".computecommander", "scripts", "focus-watcher.sh")
	}

	// Wrap the focus-watcher in a restart wrapper so it auto-recovers from crashes.
	isBashWatcher := strings.HasSuffix(focusWatcherPath, ".sh")
	focusWrapperPath, err := WriteFocusWatcherWrapper(scriptDir, focusWatcherPath, opts.TabHash, isBashWatcher)
	if err != nil {
		// Non-fatal: fall back to running the watcher directly without restart protection.
		fmt.Fprintf(os.Stderr, "Warning: failed to generate focus-watcher wrapper: %v\n", err)
		focusWrapperPath = ""
	}

	// Build the focus-watcher pane KDL. Use the restart wrapper if available.
	var focusWatcherPane string
	if focusWrapperPath != "" {
		focusWatcherPane = fmt.Sprintf(`        pane size=1 borderless=true {
            command "bash"
            args "%s"
        }`, focusWrapperPath)
	} else if isBashWatcher {
		focusWatcherPane = fmt.Sprintf(`        pane size=1 borderless=true {
            command "bash"
            args "%s" "%s"
        }`, focusWatcherPath, opts.TabHash)
	} else {
		focusWatcherPane = fmt.Sprintf(`        pane size=1 borderless=true {
            command "%s"
            args "--tab-hash" "%s" "--poll-ms" "250"
        }`, focusWatcherPath, opts.TabHash)
	}

	// Build the TG Viz pane KDL. Uses cmdr tg --pane for ASCII bar/sparkline
	// charts with SSE-driven real-time updates. The env sourcing loads API keys.
	tgVizPane := fmt.Sprintf(`                pane name="TG Viz" size="15%%" {
                    command "zsh"
                    args "-c" "source ~/.zsh/exports/keys.zsh 2>/dev/null; exec %s tg --pane"
                }`, cmdrBin)

	return fmt.Sprintf(`layout {
    cwd "%s"
    default_tab_template {
        pane size=1 borderless=true {
            plugin location="zellij:compact-bar"
        }
        children
    }
    tab name="%s" {
%s
        pane split_direction="horizontal" {
            pane split_direction="vertical" size="75%%" {
                pane split_direction="horizontal" size="14%%" {
                    pane name="Prompt" size=1 borderless=true {
                        command "%s"
                        args "prompt" "--pane"
                    }
                    pane name="Cal" size="25%%" {
                        command "calcurse"
                        args "-C" "%s/.calcurse/conf" "-D" "%s/.calcurse"
                    }
                    pane {
                        command "bash"
                        args "%s" "%s" "%s"
                    }
                }
%s
                pane split_direction="horizontal" size="19%%" {
                    pane name="Agents" size="64%%" {
                        command "%s"
                        args "status" "--pane"%s
                    }
                    pane name="Jira" size="36%%" {
                        command "%s"
                        args "jira" "--pane"%s
                    }
                }
            }
            pane split_direction="vertical" size="25%%" {
                pane name="Event Log" size="20%%" {
                    command "%s"
                    args "feed" "--pane"%s
                }
                pane name="Evals" size="15%%" {
                    command "%s"
                    args "evals" "--pane"%s
                }
                pane name="OB1" size="28%%" {
                    command "%s"
                    args "openbrain" "--pane"%s
                }
%s
                pane {
                    command "bash"
                    args "%s" "%s" "%s"
                }
            }
        }
    }
}
`, projectDir, tabName, focusWatcherPane, cmdrBin, home, home, fpWrapperPath, projectDir, opts.TabHash, agentPane, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, tgVizPane, lazygitWrapperPath, projectDir, opts.TabHash)
}

// findTGVizBinary is no longer needed. The TG Viz pane now uses
// cmdr tg --pane for ASCII bar/sparkline charts with SSE-driven updates,
// replacing the Chrome headless screenshot pipeline.

// WriteFocusWatcher locates the compiled Rust focus-watcher binary and returns
// its path. The binary is expected at plugins/focus-watcher/target/release/focus-watcher
// relative to the Go module root (discovered via the cmdr binary's location).
// Falls back to WriteFocusWatcherBash if the binary is not found.
func WriteFocusWatcher(scriptBaseDir, projectDir, tabHash string) (string, error) {
	// Try to find the Rust binary in several candidate locations.
	var candidates []string

	// Check relative to the running binary (covers dev builds and installed setups).
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			// Installed next to cmdr (e.g., make install copies it here).
			filepath.Join(exeDir, "focus-watcher"),
			// Dev build: cmdr is in the repo root, Rust binary in plugins/ subtree.
			filepath.Join(exeDir, "plugins", "focus-watcher", "target", "release", "focus-watcher"),
		)
	}

	// Check project dir (for cases where projectDir is the computeCommander repo itself).
	candidates = append(candidates,
		filepath.Join(projectDir, "plugins", "focus-watcher", "target", "release", "focus-watcher"),
	)

	// Check home directory install location.
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "focus-watcher"),
		)
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	// Fallback: generate the bash script.
	if os.Getenv("CMDR_FOCUS_WATCHER_REQUIRE_RUST") == "1" {
		return "", fmt.Errorf("Rust focus-watcher binary not found. Build it with: cd plugins/focus-watcher && cargo build --release")
	}
	fmt.Fprintf(os.Stderr, "WARNING: Rust focus-watcher binary not found in any candidate location — falling back to slow bash script.\nTo suppress this, build the binary: cd plugins/focus-watcher && cargo build --release\nTo make this a hard error, set CMDR_FOCUS_WATCHER_REQUIRE_RUST=1\n")
	return WriteFocusWatcherBash(scriptBaseDir, projectDir, tabHash)
}

// WriteFocusWatcherBash generates the legacy focus-watcher shell script that polls
// zellij for the focused pane's CWD and writes it to the per-tab CWD file.
// This is the bash fallback; prefer the Rust binary via WriteFocusWatcher.
// The script uses /proc to read the foreground process's CWD on the focused
// pane's pts device, derived from ZELLIJ_PANE_ID exposed by list-clients.
func WriteFocusWatcherBash(scriptBaseDir, projectDir, tabHash string) (string, error) {
	scriptDir := filepath.Join(scriptBaseDir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "focus-watcher.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — focus-watcher for dynamic pane CWD tracking.
# Do not edit manually; regenerated on each cmdr launch.
#
# Polls zellij for the focused pane, finds the foreground process on its
# pts device via /proc, and writes the CWD to the per-tab CWD file.
# fp-wrapper and lazygit-wrapper watch this file to follow focus.

set -uo pipefail

TAB_HASH=%q
CWD_FILE="/tmp/cmdr-$(id -u)-${TAB_HASH}-cwd"
POLL_INTERVAL="${CMDR_FOCUS_POLL:-2}"
DEBOUNCE_COUNT="${CMDR_FOCUS_DEBOUNCE:-2}"
LAST_CWD=""
LAST_SEEN_PTS=""
STABLE_COUNT=0

# cleanup: nothing persistent to remove beyond what wrappers manage.
trap 'exit 0' EXIT INT TERM

# focused_pane_pts returns the pts device number for the focused pane,
# or empty string if it cannot be determined.
# zellij action list-clients output: CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND
# ZELLIJ_PANE_ID looks like "terminal_N" where N is the pts minor number.
focused_pane_pts() {
    local clients
    clients=$(zellij action list-clients 2>/dev/null) || return
    # Skip header line. Find the row for client 1 (the user's client).
    # Extract terminal_N and strip prefix to get the pts number.
    echo "$clients" | awk 'NR>1 && $1=="1" {
        pane=$2
        sub(/^terminal_/, "", pane)
        print pane
        exit
    }'
}

# fg_cwd_for_pts returns the CWD of the foreground process on /dev/pts/N.
# It scans /proc/*/stat to find a process whose tty_nr matches pts/N
# and whose pgrp equals tpgid (meaning it is in the terminal's fg group).
# Safe stat parsing strips the comm field (which may contain spaces/parens).
fg_cwd_for_pts() {
    local pts_num="$1"
    # pts tty_nr = makedev(136, pts_num) = 136*256 + pts_num
    local tty_nr=$(( 136 * 256 + pts_num ))
    local best_pid=""
    local best_cwd=""

    for stat_file in /proc/[0-9]*/stat; do
        local pid
        pid=${stat_file%%/stat}
        pid=${pid##*/proc/}
        local raw
        raw=$(cat "$stat_file" 2>/dev/null) || continue
        # Strip comm (may contain spaces): remove first '(' to last ')'.
        local clean="${raw/\(*/X }"
        local rest="${raw##*\)}"
        clean="${clean}${rest}"
        # Fields after stripping: pid X state ppid pgrp session tty_nr tpgid ...
        local f_tty f_pgrp f_tpgid
        f_tty=$(echo "$clean"  | awk '{print $7}')
        f_pgrp=$(echo "$clean" | awk '{print $5}')
        f_tpgid=$(echo "$clean" | awk '{print $8}')
        [ "$f_tty" = "$tty_nr" ]   || continue
        [ "$f_pgrp" = "$f_tpgid" ] || continue
        local cwd
        cwd=$(readlink "/proc/$pid/cwd" 2>/dev/null) || continue
        [ -d "$cwd" ] || continue
        # Prefer the process with the longest (most specific) CWD.
        if [ ${#cwd} -gt ${#best_cwd} ]; then
            best_pid="$pid"
            best_cwd="$cwd"
        fi
    done

    echo "$best_cwd"
}

# git_root returns the git toplevel for a directory, or empty string.
git_root() {
    git -C "$1" rev-parse --show-toplevel 2>/dev/null
}

# Main loop: poll for focus changes and update the CWD file.
# Debounce: only update CWD after focus has been stable for DEBOUNCE_COUNT
# consecutive polls. This filters out transient mouse-hover focus changes.
while true; do
    pts=$(focused_pane_pts)
    if [ -n "$pts" ] && [ "$pts" -ge 0 ] 2>/dev/null; then
        # Track focus stability for debounce.
        if [ "$pts" = "$LAST_SEEN_PTS" ]; then
            STABLE_COUNT=$(( STABLE_COUNT + 1 ))
        else
            LAST_SEEN_PTS="$pts"
            STABLE_COUNT=1
        fi

        if [ "$STABLE_COUNT" -ge "$DEBOUNCE_COUNT" ]; then
            cwd=$(fg_cwd_for_pts "$pts")
            if [ -n "$cwd" ] && [ -d "$cwd" ]; then
                # Resolve to git root so fp/lazygit get the project root,
                # not a subdirectory like internal/commands/.
                project=$(git_root "$cwd")
                # Skip non-git dirs and dotfile/config repos (~/.config/*, ~/.claude, etc).
                if [ -n "$project" ] && [ "$project" != "$LAST_CWD" ] \
                   && [[ ! "$project" =~ ^${HOME}/\. ]]; then
                    echo "$project" > "$CWD_FILE"
                    LAST_CWD="$project"
                fi
            fi
        fi
    fi
    sleep "$POLL_INTERVAL"
done
`, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write focus-watcher: %w", err)
	}

	return scriptPath, nil
}

// buildAgentPane returns the KDL block for the center agent session pane.
// If wrapperPath is set, runs the wrapper script via bash for session-switch support.
// The tabHash is appended as an extra argument so that each tab instance has a
// unique terminal_command, allowing correct per-tab CWD file association.
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
// <scriptBaseDir>/.computecommander/scripts/cmdr-agent-wrapper.sh and returns its path.
// The script loops, resuming a queued session from the switch file when present,
// or running agentCmd otherwise.
// scriptBaseDir controls where the script file is written.
// projectDir is the initial working directory for the agent (written to the CWD file on startup).
func WriteAgentWrapper(scriptBaseDir, projectDir, agentCmd, tabHash string) (string, error) {
	scriptDir := filepath.Join(scriptBaseDir, ".computecommander", "scripts")
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

# Set terminal title to show the current project.
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

# Update the per-tab CWD file and tab name when the project changes.
# Also recreates the CWD file if it was deleted (e.g., by a colliding tab exit).
update_project() {
    local project="$1"
    if [ -n "$project" ] && [ -d "$project" ]; then
        if [ "$project" != "$LAST_TRACKED_PROJECT" ] || [ ! -f "$TAB_CWD" ]; then
            echo "$project" > "$TAB_CWD"
            LAST_TRACKED_PROJECT="$project"
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

    # Poll for the switch file while the agent is running.
    # Check every second so the switch feels near-instant.
    sleep 1
done
`, filepath.Join(homeDir(), ".computecommander"), agentCmd, projectDir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write agent wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteFPWrapper generates the fp (file picker) wrapper script that uses
// inotifywait to watch the per-tab CWD file for session-switch updates,
// falling back to polling if inotifywait is unavailable. Returns the path to the generated script.
// scriptBaseDir controls where the script file is written.
// projectDir is the default directory fallback when $1 is not provided.
func WriteFPWrapper(scriptBaseDir, projectDir, tabHash string) (string, error) {
	scriptDir := filepath.Join(scriptBaseDir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "fp-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — fp wrapper with session-switch tracking.
# Do not edit manually; regenerated on each cmdr launch.
# Uses inotifywait on a per-tab CWD file for instant, isolated updates.
set -uo pipefail

DEFAULT_DIR="${1:-%s}"
# $2 is the tab hash passed from the KDL layout args at launch time.
# Fall back to the compile-time hash so existing running instances stay valid.
TAB_HASH="${2:-%s}"
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
    printf '\033]2;fp: %%s\007' "$(basename "$dir")"
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
`, projectDir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write fp wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteLazygitWrapper generates the lazygit wrapper script that uses
// inotifywait to watch the per-tab CWD file for session-switch updates,
// falling back to polling if inotifywait is unavailable. Returns the path to the generated script.
// scriptBaseDir controls where the script file is written.
// projectDir is the default directory fallback when $1 is not provided.
func WriteLazygitWrapper(scriptBaseDir, projectDir, tabHash string) (string, error) {
	scriptDir := filepath.Join(scriptBaseDir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "lazygit-wrapper.sh")
	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — lazygit wrapper with session-switch tracking.
# Do not edit manually; regenerated on each cmdr launch.
# Uses inotifywait on a per-tab CWD file for instant, isolated updates.
set -uo pipefail

DEFAULT_DIR="${1:-%s}"
# $2 is the tab hash passed from the KDL layout args at launch time.
# Fall back to the compile-time hash so existing running instances stay valid.
TAB_HASH="${2:-%s}"
export CMDR_TAB_HASH="$TAB_HASH"
LG_PID=""
HAS_INOTIFY=false
command -v inotifywait >/dev/null 2>&1 && HAS_INOTIFY=true

# Per-tab CWD file: hash generated at layout time, shared with agent-wrapper.
CWD_FILE="/tmp/cmdr-$(id -u)-${TAB_HASH}-cwd"

start_lg() {
    local dir="${1:-$DEFAULT_DIR}"
    [ -d "$dir" ] || dir="$DEFAULT_DIR"
    # Update pane frame title to show the project name.
    local project_name
    project_name=$(basename "$dir")
    printf '\033]2;LazyGit: %%s\007' "$project_name"
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
`, projectDir, tabHash)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write lazygit wrapper: %w", err)
	}

	return scriptPath, nil
}

// WriteFocusWatcherWrapper generates a restart wrapper script for the focus-watcher
// binary or bash fallback. When the focus-watcher crashes or exits, the wrapper
// automatically restarts it after a short delay, preventing the CWD file from going
// stale (which would cause fp and lazygit panes to stop receiving updates).
// Returns the path to the generated wrapper script.
func WriteFocusWatcherWrapper(scriptBaseDir, focusWatcherPath, tabHash string, isBash bool) (string, error) {
	scriptDir := filepath.Join(scriptBaseDir, ".computecommander", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return "", fmt.Errorf("create scripts dir: %w", err)
	}

	scriptPath := filepath.Join(scriptDir, "focus-watcher-wrapper.sh")

	// Build the command line depending on whether it's the Rust binary or bash script.
	var watcherCmd string
	if isBash {
		watcherCmd = fmt.Sprintf(`bash %q %q`, focusWatcherPath, tabHash)
	} else {
		watcherCmd = fmt.Sprintf(`%q --tab-hash %q --poll-ms 250`, focusWatcherPath, tabHash)
	}

	content := fmt.Sprintf(`#!/bin/bash
# Auto-generated by cmdr — focus-watcher restart wrapper.
# Do not edit manually; regenerated on each cmdr launch.
#
# Wraps the focus-watcher (Rust binary or bash fallback) with automatic
# restart on exit. Without this, a crashed focus-watcher leaves the CWD
# file stale, causing fp and lazygit panes to stop receiving updates.
set -uo pipefail

WATCHER_CMD=%q
LOG_FILE="/tmp/cmdr-$(id -u)-focus-watcher.log"
RESTART_DELAY=3
MAX_RAPID_RESTARTS=5
RAPID_WINDOW=30

restart_count=0
window_start=$(date +%%s)

log() {
    echo "[$(date '+%%Y-%%m-%%d %%H:%%M:%%S')] $*" >> "$LOG_FILE"
}

trap 'log "wrapper exiting (signal)"; exit 0' EXIT INT TERM

log "focus-watcher wrapper started"

while true; do
    now=$(date +%%s)
    elapsed=$(( now - window_start ))

    # Reset the rapid-restart counter if we're outside the window.
    if [ "$elapsed" -ge "$RAPID_WINDOW" ]; then
        restart_count=0
        window_start=$now
    fi

    # If we've hit too many rapid restarts, back off longer.
    if [ "$restart_count" -ge "$MAX_RAPID_RESTARTS" ]; then
        log "too many rapid restarts ($restart_count in ${elapsed}s), backing off 30s"
        sleep 30
        restart_count=0
        window_start=$(date +%%s)
    fi

    log "starting focus-watcher (restart #${restart_count})"
    eval $WATCHER_CMD
    exit_code=$?
    log "focus-watcher exited with code $exit_code"

    restart_count=$(( restart_count + 1 ))
    log "restarting in ${RESTART_DELAY}s..."
    sleep "$RESTART_DELAY"
done
`, watcherCmd)

	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write focus-watcher-wrapper: %w", err)
	}

	return scriptPath, nil
}

// cleanStaleCWDFiles removes old per-tab CWD files from /tmp that are older than
// 24 hours. Each dashboard launch creates a new per-tab CWD file with a unique hash;
// old files from closed tabs accumulate over time and should be cleaned up.
func cleanStaleCWDFiles() {
	uid := os.Getuid()
	prefix := fmt.Sprintf("cmdr-%d-", uid)
	entries, err := os.ReadDir("/tmp")
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "-cwd") {
			continue
		}
		// Skip the active-cwd file — it's used as a global fallback.
		if name == prefix+"active-cwd" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join("/tmp", name))
		}
	}
}

// WriteLayout generates and writes the KDL layout file to the given path.
// Also generates the fp-wrapper and lazygit-wrapper scripts for session-switch tracking.
// When opts.UseWrapper is true and opts.AgentCommand is set, it generates
// a wrapper script for session-switch support and uses that in the layout.
// Otherwise the agent command is embedded directly in the KDL layout.
func WriteLayout(path string, opts LayoutOpts) error {
	// Clean up stale per-tab CWD files from previous dashboard sessions.
	cleanStaleCWDFiles()

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
	// All wrapper scripts share the same hash so they use the same per-tab CWD file.
	tabHash := generateTabHash()
	opts.TabHash = tabHash

	// Generate the fp-wrapper script for session-switch tracking.
	if _, err := WriteFPWrapper(scriptDir, projectDir, tabHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate fp-wrapper: %v\n", err)
	}

	// Generate the lazygit-wrapper script for session-switch tracking.
	if _, err := WriteLazygitWrapper(scriptDir, projectDir, tabHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate lazygit-wrapper: %v\n", err)
	}

	// Note: WriteFocusWatcher is called from GenerateLayout() to resolve the
	// focus-watcher path (Rust binary or bash fallback) and embed it in the KDL.

	// Only generate the wrapper script when explicitly requested.
	// Use scriptDir (not projectDir) so the wrapper lands in the same
	// directory as fp-wrapper and lazygit-wrapper. When SystemWide,
	// scriptDir is $HOME; otherwise it's the project directory.
	if opts.UseWrapper && opts.AgentCommand != "" && opts.AgentWrapperPath == "" {
		wrapperPath, err := WriteAgentWrapper(scriptDir, projectDir, opts.AgentCommand, tabHash)
		if err != nil {
			// Non-fatal: fall back to running the agent command directly.
			_ = err
		} else {
			opts.AgentWrapperPath = wrapperPath
		}

		// When SystemWide, clean up any stale agent-wrapper left in the
		// project-local scripts dir to prevent old versions from interfering.
		// Only remove if projectDir differs from scriptDir to avoid deleting
		// the wrapper we just wrote.
		if opts.SystemWide && projectDir != scriptDir {
			staleWrapper := filepath.Join(projectDir, ".computecommander", "scripts", "cmdr-agent-wrapper.sh")
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
