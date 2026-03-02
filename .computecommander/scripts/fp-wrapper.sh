#!/bin/bash
# fp-wrapper.sh — file picker that follows the active agent session's CWD.
set -uo pipefail

CWD_FILE="/home/n0ko/Programs/ai/computeCommander/.computecommander/active-cwd"
DEFAULT_DIR="/home/n0ko/Programs/ai/computeCommander"
FP_PID=""

start_fp() {
    local dir="${1:-$DEFAULT_DIR}"
    [ -d "$dir" ] || dir="$DEFAULT_DIR"
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

# Initial directory.
CURRENT_DIR="$DEFAULT_DIR"
if [ -f "$CWD_FILE" ]; then
    CURRENT_DIR=$(cat "$CWD_FILE")
fi
start_fp "$CURRENT_DIR"

# Poll for CWD changes.
while true; do
    sleep 2
    if [ -f "$CWD_FILE" ]; then
        NEW_DIR=$(cat "$CWD_FILE")
        if [ "$NEW_DIR" != "$CURRENT_DIR" ] && [ -d "$NEW_DIR" ]; then
            CURRENT_DIR="$NEW_DIR"
            kill_fp
            start_fp "$CURRENT_DIR"
        fi
    fi
    # Restart fp if it died.
    if [ -n "$FP_PID" ] && ! kill -0 "$FP_PID" 2>/dev/null; then
        wait "$FP_PID" 2>/dev/null
        start_fp "$CURRENT_DIR"
    fi
done
