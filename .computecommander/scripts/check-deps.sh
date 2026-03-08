#!/bin/sh
# check-deps.sh — verify runtime dependencies for computeCommander
# Exit 0 if all deps found, 1 if any are missing.

ok=true

if ! command -v fp >/dev/null 2>&1; then
    echo "missing: fp (file picker)"
    echo "  install: cargo install fp"
    ok=false
fi

if ! command -v focus-watcher >/dev/null 2>&1; then
    echo "missing: focus-watcher"
    echo "  install: cd plugins/focus-watcher && cargo build --release && cp target/release/focus-watcher ~/.local/bin/"
    ok=false
fi

$ok
