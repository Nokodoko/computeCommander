#!/usr/bin/env bash
# TrustGraph Visualization - Carbonyl wrapper for zellij pane

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
VIZ_HTML="${SCRIPT_DIR}/tg-viz.html"

if ! command -v carbonyl &>/dev/null; then
    echo "carbonyl not installed. Install: yay -S carbonyl-bin"
    sleep infinity
fi

exec carbonyl "file://${VIZ_HTML}" --no-sandbox
