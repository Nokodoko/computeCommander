#!/usr/bin/env bash
# TrustGraph Visualization - Carbonyl wrapper for zellij pane

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VIZ_HTML="${SCRIPT_DIR}/tg-viz.html"

if ! command -v carbonyl &>/dev/null; then
    echo "carbonyl not installed. Install: yay -S carbonyl-bin"
    sleep infinity
fi

# Resolve the TG gateway from the env contract; default matches the Go const
# config.DefaultMontyGateway (single source of truth) = http://10.0.0.1:8088 (monty).
GW="${CMDR_TG_GATEWAY:-http://10.0.0.1:8088}"
exec carbonyl "file://${VIZ_HTML}?gw=${GW}" --no-sandbox
