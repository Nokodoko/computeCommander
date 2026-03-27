#!/usr/bin/env bash
# TrustGraph Visualization -- High-resolution terminal renderer
#
# Rendering pipeline (best → fallback):
#   1. chrome --headless screenshot → wezterm imgcat  (pixel-perfect via iterm2 protocol)
#   2. chrome --headless screenshot → chafa --sixels   (near-pixel via sixel protocol)
#   3. carbonyl half-block chars                       (character-grid, ~200x100 px)
#
# Carbonyl v0.0.3 renders at 1 char = 1px wide x 2px tall (half-block), giving
# roughly 200x100 effective pixels for a 200x50 terminal. This is a hard limit.
#
# The screenshot pipeline bypasses this entirely: headless Chrome renders the page
# at 1920x1080 @2x (3840x2160 actual) and the resulting PNG is displayed via
# wezterm imgcat (iterm2 inline image protocol) or chafa sixel output, both of
# which render at the terminal's native pixel resolution.

set -euo pipefail

N0KOS_PORT="${N0KOS_PORT:-3000}"
VIZ_URL="http://localhost:${N0KOS_PORT}/static/tg-viz.html"
FRAME_PATH="/tmp/tg-viz-frame-$$.png"
POLL_SEC="${TG_VIZ_POLL_SEC:-10}"

# ── Find a usable headless Chrome binary ─────────────────────────────────
find_chrome() {
    local candidates=(
        # Puppeteer's cached Chrome (most likely to exist)
        "$HOME/.cache/puppeteer/chrome"/*/chrome-linux64/chrome
        # System Chromium
        /usr/bin/chromium
        /usr/bin/google-chrome-stable
        /usr/bin/google-chrome
        /snap/bin/chromium
    )
    for bin in "${candidates[@]}"; do
        if [[ -x "$bin" ]]; then
            echo "$bin"
            return 0
        fi
    done
    return 1
}

# ── Detect the best display method ───────────────────────────────────────
# Zellij does NOT pass through iterm2 inline image protocol (wezterm imgcat),
# so inside zellij we must use chafa with kitty or sixel graphics protocol.
detect_display() {
    if [[ -n "${ZELLIJ:-}" ]]; then
        # Inside zellij: wezterm imgcat won't work, use chafa
        if command -v chafa &>/dev/null; then
            echo "chafa"
        else
            echo "none"
        fi
    elif command -v wezterm &>/dev/null; then
        echo "imgcat"
    elif command -v chafa &>/dev/null; then
        echo "chafa"
    else
        echo "none"
    fi
}

# ── Display one frame ────────────────────────────────────────────────────
display_imgcat() {
    printf '\033[2J\033[H'
    wezterm imgcat --width=100% --height=100% "$1" 2>/dev/null || true
}

display_chafa() {
    local cols rows fmt_args=()
    cols=$(tput cols 2>/dev/null || echo 200)
    rows=$(tput lines 2>/dev/null || echo 50)
    # Inside zellij: use half-block symbols (character grid) -- kitty/sixel protocols
    # are passed through to wezterm but render outside pane boundaries.
    # Outside zellij: use kitty for pixel-perfect rendering.
    if [[ -z "${ZELLIJ:-}" ]]; then
        fmt_args=(--format=kitty)
    else
        fmt_args=(--symbols=half --color-space=din99d)
    fi
    printf '\033[2J\033[H'
    chafa "${fmt_args[@]}" --size="${cols}x${rows}" --animate=off "$1" 2>/dev/null \
        || chafa --size="${cols}x${rows}" --animate=off "$1" 2>/dev/null \
        || true
}

# ── Take one headless screenshot ─────────────────────────────────────────
take_screenshot() {
    local chrome_bin="$1" url="$2" out="$3"
    local out_dir out_file
    out_dir=$(dirname "$out")
    out_file=$(basename "$out")

    cd "$out_dir" && "$chrome_bin" --headless=new --no-sandbox --disable-gpu \
        --disable-software-rasterizer --disable-dev-shm-usage \
        --force-device-scale-factor=2 \
        --window-size=1920,1080 \
        --virtual-time-budget=5000 \
        --screenshot="$out_file" \
        "$url" >/dev/null 2>&1
    [[ -f "$out" ]]
}

# ── Screenshot render loop ───────────────────────────────────────────────
run_screenshot_loop() {
    local chrome_bin="$1"
    local display_method="$2"

    echo "[tg-viz] High-res mode: Chrome headless → ${display_method}"
    echo "[tg-viz] Viewport: 1920x1080 @2x = 3840x2160 rendered pixels"
    echo "[tg-viz] Refresh: every ${POLL_SEC}s"
    echo "[tg-viz] Capturing first frame..."

    trap 'rm -f "$FRAME_PATH"; exit 0' EXIT INT TERM

    while true; do
        if take_screenshot "$chrome_bin" "$VIZ_URL" "$FRAME_PATH"; then
            "display_${display_method}" "$FRAME_PATH"
        else
            # Server might not be up yet -- show status
            printf '\033[2J\033[H'
            echo "[tg-viz] Waiting for ${VIZ_URL} ..."
        fi
        sleep "$POLL_SEC"
    done
}

# ── Carbonyl fallback ────────────────────────────────────────────────────
run_carbonyl() {
    if ! command -v carbonyl &>/dev/null; then
        echo "No renderer available."
        echo "Install Chrome: sudo pacman -S chromium"
        echo "  or Carbonyl: yay -S carbonyl-bin"
        echo "  or run: npx puppeteer browsers install chrome"
        sleep infinity
    fi

    echo "[tg-viz] Falling back to carbonyl (character-grid rendering)"
    echo "[tg-viz] Tip: install chromium for pixel-perfect rendering"
    sleep 2
    exec carbonyl "${VIZ_URL}" \
        --no-sandbox \
        --force-device-scale-factor=2 \
        --window-size=3840,2160 \
        --high-dpi-support=1
}

# ── Main ─────────────────────────────────────────────────────────────────
main() {
    local display_method chrome_bin

    display_method=$(detect_display)
    if [[ "$display_method" == "none" ]]; then
        echo "[tg-viz] No image display method (need wezterm or chafa)"
        run_carbonyl
        return
    fi

    chrome_bin=$(find_chrome 2>/dev/null) || chrome_bin=""
    if [[ -z "$chrome_bin" ]]; then
        echo "[tg-viz] No headless Chrome found"
        echo "[tg-viz] Bootstrap with: npx puppeteer browsers install chrome"
        run_carbonyl
        return
    fi

    run_screenshot_loop "$chrome_bin" "$display_method"
}

main "$@"
