// Command tg-viz renders the TrustGraph visualization in a terminal pane.
//
// Rendering pipeline (best -> fallback):
//  1. chrome --headless screenshot -> chafa --format=kitty   (pixel-perfect via kitty graphics protocol)
//  2. chrome --headless screenshot -> chafa --format=sixels  (near-pixel via sixel protocol)
//  3. chrome --headless screenshot -> chafa symbols           (character-grid fallback)
//  4. carbonyl                                                (live character-grid, ~200x100 px)
//
// The screenshot pipeline renders the page at 1920x1080 @2x (3840x2160 actual)
// via headless Chrome, then displays the PNG using chafa with the best available
// graphics protocol. The kitty protocol produces the highest quality output.
//
// Environment variables:
//
//	N0KOS_PORT         - HTTP port for the viz server (default: 3000)
//	TG_VIZ_POLL_SEC    - Seconds between refreshes (default: 10)
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// displayMethod represents a terminal image display strategy.
type displayMethod int

const (
	displayNone displayMethod = iota
	displayChafaKitty
	displayChafaSixel
	displayChafaSymbols
	displayCarbonyl
)

func (d displayMethod) String() string {
	switch d {
	case displayChafaKitty:
		return "chafa-kitty"
	case displayChafaSixel:
		return "chafa-sixel"
	case displayChafaSymbols:
		return "chafa-symbols"
	case displayCarbonyl:
		return "carbonyl"
	default:
		return "none"
	}
}

func main() {
	port := envOrDefault("N0KOS_PORT", "3000")
	pollSec := envIntOrDefault("TG_VIZ_POLL_SEC", 10)
	vizURL := fmt.Sprintf("http://localhost:%s/static/tg-viz.html", port)
	framePath := fmt.Sprintf("/tmp/tg-viz-frame-%d.png", os.Getpid())

	// Set up signal handling for clean shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Clean up temp file on exit.
	defer os.Remove(framePath)

	chromeBin := findChrome()
	display := detectDisplay()

	if display == displayNone || chromeBin == "" {
		if chromeBin == "" {
			fmt.Fprintln(os.Stderr, "[tg-viz] No headless Chrome found")
			fmt.Fprintln(os.Stderr, "[tg-viz] Bootstrap with: npx puppeteer browsers install chrome")
		} else {
			fmt.Fprintln(os.Stderr, "[tg-viz] No image display method (need chafa)")
		}
		runCarbonyl(ctx, vizURL)
		return
	}

	runScreenshotLoop(ctx, chromeBin, display, vizURL, framePath, pollSec)
}

// envOrDefault returns the value of the environment variable named by key,
// or defaultVal if the variable is unset or empty.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// envIntOrDefault returns the integer value of the environment variable named
// by key, or defaultVal if the variable is unset, empty, or not a valid integer.
func envIntOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// findChrome searches for a usable headless Chrome binary.
// It checks the puppeteer cache first, then system-installed locations.
func findChrome() string {
	home, _ := os.UserHomeDir()
	candidates := []string{}

	// Puppeteer's cached Chrome (most likely to exist).
	// Glob the versioned directory.
	if home != "" {
		pattern := filepath.Join(home, ".cache", "puppeteer", "chrome", "*", "chrome-linux64", "chrome")
		matches, _ := filepath.Glob(pattern)
		candidates = append(candidates, matches...)
	}

	// System Chromium / Chrome.
	candidates = append(candidates,
		"/usr/bin/chromium",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/google-chrome",
		"/snap/bin/chromium",
	)

	for _, bin := range candidates {
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			// Check executable bit.
			if info.Mode()&0111 != 0 {
				return bin
			}
		}
	}
	return ""
}

// detectDisplay determines the best image display method available.
// Inside zellij, kitty/sixel protocols are swallowed by the multiplexer,
// so we fall back to half-block symbols which render as plain text.
// Outside zellij: chafa kitty -> chafa sixels -> chafa symbols -> none.
func detectDisplay() displayMethod {
	if _, err := exec.LookPath("chafa"); err != nil {
		return displayNone
	}

	// Zellij does not pass through kitty or sixel graphics protocols.
	if os.Getenv("ZELLIJ") != "" {
		return displayChafaSymbols
	}

	return displayChafaKitty
}

// termSize returns the current terminal size (columns, rows).
func termSize() (int, int) {
	cols := 200
	rows := 50

	cmd := exec.Command("tput", "cols")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			cols = n
		}
	}

	cmd = exec.Command("tput", "lines")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
			rows = n
		}
	}

	return cols, rows
}

// displayFrame renders a screenshot PNG to the terminal using the given method.
func displayFrame(method displayMethod, path string) error {
	cols, rows := termSize()
	size := fmt.Sprintf("%dx%d", cols, rows)

	// Clear screen and move cursor to top-left.
	fmt.Print("\033[2J\033[H")

	var args []string
	switch method {
	case displayChafaKitty:
		args = []string{"--format=kitty", "--size=" + size, "--animate=off", path}
	case displayChafaSixel:
		args = []string{"--format=sixels", "--size=" + size, "--animate=off", path}
	case displayChafaSymbols:
		args = []string{"--symbols=half", "--color-space=din99d", "--size=" + size, "--animate=off", path}
	default:
		return fmt.Errorf("unsupported display method: %s", method)
	}

	cmd := exec.Command("chafa", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// takeScreenshot captures a headless Chrome screenshot of the given URL.
func takeScreenshot(ctx context.Context, chromeBin, url, outPath string) error {
	outDir := filepath.Dir(outPath)
	outFile := filepath.Base(outPath)

	args := []string{
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-software-rasterizer",
		"--disable-dev-shm-usage",
		"--force-device-scale-factor=2",
		"--window-size=1920,1080",
		"--virtual-time-budget=5000",
		"--screenshot=" + outFile,
		url,
	}

	cmd := exec.CommandContext(ctx, chromeBin, args...)
	cmd.Dir = outDir
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("chrome screenshot failed: %w", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("screenshot file not created: %w", err)
	}
	return nil
}

// runScreenshotLoop continuously captures and displays screenshots.
func runScreenshotLoop(ctx context.Context, chromeBin string, method displayMethod, vizURL, framePath string, pollSec int) {
	fmt.Fprintf(os.Stderr, "[tg-viz] High-res mode: Chrome headless -> %s\n", method)
	fmt.Fprintf(os.Stderr, "[tg-viz] Viewport: 1920x1080 @2x = 3840x2160 rendered pixels\n")
	fmt.Fprintf(os.Stderr, "[tg-viz] Refresh: every %ds\n", pollSec)
	fmt.Fprintf(os.Stderr, "[tg-viz] Capturing first frame...\n")

	ticker := time.NewTicker(time.Duration(pollSec) * time.Second)
	defer ticker.Stop()

	// Capture and display immediately, then on each tick.
	captureAndDisplay := func() {
		if err := takeScreenshot(ctx, chromeBin, vizURL, framePath); err != nil {
			// Server might not be up yet -- show status.
			fmt.Print("\033[2J\033[H")
			fmt.Fprintf(os.Stderr, "[tg-viz] Waiting for %s ...\n", vizURL)
			return
		}
		if err := displayFrame(method, framePath); err != nil {
			// If the preferred method fails, try falling back.
			fmt.Fprintf(os.Stderr, "[tg-viz] display error (%s): %v\n", method, err)
		}
	}

	captureAndDisplay()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			captureAndDisplay()
		}
	}
}

// runCarbonyl launches carbonyl as a fallback renderer.
func runCarbonyl(ctx context.Context, vizURL string) {
	carbonylBin, err := exec.LookPath("carbonyl")
	if err != nil {
		fmt.Fprintln(os.Stderr, "No renderer available.")
		fmt.Fprintln(os.Stderr, "Install Chrome: sudo pacman -S chromium")
		fmt.Fprintln(os.Stderr, "  or Carbonyl: yay -S carbonyl-bin")
		fmt.Fprintln(os.Stderr, "  or run: npx puppeteer browsers install chrome")
		// Block until signal.
		<-ctx.Done()
		return
	}

	fmt.Fprintln(os.Stderr, "[tg-viz] Falling back to carbonyl (character-grid rendering)")
	fmt.Fprintln(os.Stderr, "[tg-viz] Tip: install chromium for pixel-perfect rendering")
	time.Sleep(2 * time.Second)

	cmd := exec.CommandContext(ctx, carbonylBin, vizURL,
		"--no-sandbox",
		"--force-device-scale-factor=2",
		"--window-size=3840,2160",
		"--high-dpi-support=1",
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "[tg-viz] carbonyl exited: %v\n", err)
	}
}
