#!/bin/bash
# cmdr-agent-wrapper.sh — Agent pane wrapper with session switch support.
# Runs the default agent command, or resumes a specific session when
# a switch file is detected. After the agent exits, loops back.
set -euo pipefail

SWITCH_FILE="${CMDR_PROJECT_DIR:-.}/.computecommander/session-switch"
AGENT_CMD="${CMDR_AGENT_CMD:-claude --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit}"

while true; do
    if [ -f "$SWITCH_FILE" ]; then
        project_path=$(sed -n '1p' "$SWITCH_FILE")
        session_id=$(sed -n '2p' "$SWITCH_FILE")
        rm -f "$SWITCH_FILE"
        if [ -d "$project_path" ]; then
            cd "$project_path"
            echo "Resuming session $session_id in $project_path..."
            claude --resume "$session_id" --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit || true
        else
            echo "Directory not found: $project_path"
            sleep 2
        fi
    else
        echo "Starting agent session..."
        eval "$AGENT_CMD" || true
    fi
    echo ""
    echo "Session ended. Restarting in 2s... (or switch session with Ctrl+Space → S)"
    sleep 2
done
