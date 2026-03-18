#!/bin/sh
# cmdr-agent-counter.sh - Universal agent counter (DB-backed)
# Counts all working agents across all runtimes from sessions table.
# Replaces the pgrep-based counting approach with a DB query via cmdr status.
#
# Usage: Called by dwmblocks or similar status bar integrations.
# Output: Writes agent count to /tmp/claude_subagent_count and signals dwmblocks.

COUNT=$(cmdr status --json 2>/dev/null | jq '.sessions | map(select(.state == "working")) | length' 2>/dev/null || echo 0)
printf "%s" "$COUNT" > /tmp/claude_subagent_count
pkill -RTMIN+12 dwmblocks 2>/dev/null || true
