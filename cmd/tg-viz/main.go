// Command tg-viz renders the TrustGraph visualization in a terminal pane.
//
// Rendering pipeline:
//  1. CDP screencast (30fps) -> chafa --format=kitty  (pixel-perfect, low latency)
//  2. CDP screencast (30fps) -> chafa --format=sixels (near-pixel, low latency)
//  3. CDP screencast (30fps) -> chafa symbols          (character-grid fallback)
//  4. Polling screenshot fallback -> chafa             (high latency, 10s intervals)
//  5. carbonyl                                         (last resort)
//
// The CDP screencast pipeline keeps a headless Chrome instance running persistently,
// navigated to the visualization page. Chrome streams JPEG frames via the
// Page.startScreencast CDP event at up to 30fps. Each frame is decoded, converted
// to PNG, and displayed via chafa.
//
// Environment variables:
//
//	N0KOS_PORT           - HTTP port for the viz server (default: 3000)
//	TG_VIZ_POLL_SEC      - Seconds between refreshes for fallback mode (default: 10)
//	TG_VIZ_FPS           - Target framerate for CDP screencast (default: 15)
//	TG_VIZ_QUALITY       - JPEG quality for screencast frames (default: 80)
//	TG_VIZ_CDP           - Force CDP mode: "1" to enable, "0" to disable (default: auto)
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"image/png"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	fps := envIntOrDefault("TG_VIZ_FPS", 15)
	quality := envIntOrDefault("TG_VIZ_QUALITY", 80)
	cdpMode := os.Getenv("TG_VIZ_CDP")
	vizURL := fmt.Sprintf("http://localhost:%s/static/tg-viz.html", port)
	framePath := fmt.Sprintf("/tmp/tg-viz-frame-%d.png", os.Getpid())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

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

	// Try CDP screencast mode first (low latency, continuous streaming).
	if cdpMode != "0" {
		fmt.Fprintf(os.Stderr, "[tg-viz] Attempting CDP screencast mode (target: %dfps, quality: %d)\n", fps, quality)
		err := runCDPScreencast(ctx, chromeBin, display, vizURL, fps, quality)
		if err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "[tg-viz] CDP screencast failed: %v\n", err)
			fmt.Fprintln(os.Stderr, "[tg-viz] Falling back to polling screenshot mode")
		} else {
			return
		}
	}

	// Fallback: polling screenshot mode.
	runScreenshotLoop(ctx, chromeBin, display, vizURL, framePath, pollSec)
}

// ── CDP Screencast ──────────────────────────────────────────────────────────

// cdpMessage represents a CDP protocol message.
type cdpMessage struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// screencastFrameEvent represents a Page.screencastFrame CDP event.
type screencastFrameEvent struct {
	Data      string `json:"data"`      // base64-encoded image data
	SessionID int    `json:"sessionId"` // frame session ID for ack
}

// runCDPScreencast launches Chrome with remote debugging and uses the CDP
// Page.startScreencast API for continuous frame streaming at target FPS.
func runCDPScreencast(ctx context.Context, chromeBin string, display displayMethod, vizURL string, fps, quality int) error {
	// Find an available port for Chrome debugging.
	debugPort, err := findFreePort()
	if err != nil {
		return fmt.Errorf("find debug port: %w", err)
	}

	// Launch Chrome with remote debugging.
	chromeCtx, chromeCancel := context.WithCancel(ctx)
	defer chromeCancel()

	userDataDir := fmt.Sprintf("/tmp/tg-viz-chrome-%d", os.Getpid())
	defer os.RemoveAll(userDataDir)

	chromeArgs := []string{
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-software-rasterizer",
		"--disable-dev-shm-usage",
		"--force-device-scale-factor=2",
		"--window-size=1920,1080",
		fmt.Sprintf("--remote-debugging-port=%d", debugPort),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		vizURL,
	}

	chromeCmd := exec.CommandContext(chromeCtx, chromeBin, chromeArgs...)
	chromeCmd.Stdout = nil
	chromeCmd.Stderr = nil
	if err := chromeCmd.Start(); err != nil {
		return fmt.Errorf("start chrome: %w", err)
	}
	defer func() {
		chromeCancel()
		_ = chromeCmd.Wait()
	}()

	// Wait for Chrome DevTools to become available.
	wsURL, err := waitForDevTools(ctx, debugPort, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect devtools: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[tg-viz] CDP connected: %s\n", wsURL)
	fmt.Fprintf(os.Stderr, "[tg-viz] Screencast: %dfps, quality=%d, display=%s\n", fps, quality, display)

	// Connect WebSocket to CDP.
	conn, err := dialWebSocket(ctx, wsURL)
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	defer conn.Close()

	// Enable Page domain.
	if err := cdpSend(conn, 1, "Page.enable", nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	if _, err := cdpRecv(conn); err != nil {
		return fmt.Errorf("Page.enable response: %w", err)
	}

	// Start screencast.
	screencastParams := map[string]any{
		"format":        "jpeg",
		"quality":       quality,
		"maxWidth":      1920,
		"maxHeight":     1080,
		"everyNthFrame": 1,
	}
	paramsJSON, _ := json.Marshal(screencastParams)
	if err := cdpSend(conn, 2, "Page.startScreencast", paramsJSON); err != nil {
		return fmt.Errorf("Page.startScreencast: %w", err)
	}
	if _, err := cdpRecv(conn); err != nil {
		return fmt.Errorf("Page.startScreencast response: %w", err)
	}

	fmt.Fprintln(os.Stderr, "[tg-viz] Screencast streaming started")

	// Frame display loop.
	var (
		frameBuf bytes.Buffer
		mu       sync.Mutex
		frameNum int64
	)

	// Rate limiter: enforce target FPS.
	minFrameInterval := time.Second / time.Duration(fps)
	lastDisplay := time.Time{}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		msg, err := cdpRecv(conn)
		if err != nil {
			return fmt.Errorf("cdp recv: %w", err)
		}

		if msg.Method != "Page.screencastFrame" {
			continue
		}

		var frame screencastFrameEvent
		if err := json.Unmarshal(msg.Params, &frame); err != nil {
			fmt.Fprintf(os.Stderr, "[tg-viz] bad frame: %v\n", err)
			continue
		}

		// Acknowledge the frame immediately so Chrome keeps sending.
		ackParams, _ := json.Marshal(map[string]int{"sessionId": frame.SessionID})
		_ = cdpSend(conn, 3, "Page.screencastFrameAck", ackParams)

		// Rate limit display.
		now := time.Now()
		if now.Sub(lastDisplay) < minFrameInterval {
			continue
		}
		lastDisplay = now

		// Decode base64 JPEG -> PNG for chafa.
		imgData, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			continue
		}

		// Convert JPEG to PNG.
		img, err := jpeg.Decode(bytes.NewReader(imgData))
		if err != nil {
			continue
		}

		mu.Lock()
		frameBuf.Reset()
		if err := png.Encode(&frameBuf, img); err != nil {
			mu.Unlock()
			continue
		}

		// Write frame to temp file and display.
		frameFile := fmt.Sprintf("/tmp/tg-viz-cdp-%d.png", os.Getpid())
		if err := os.WriteFile(frameFile, frameBuf.Bytes(), 0o644); err != nil {
			mu.Unlock()
			continue
		}
		mu.Unlock()

		if err := displayFrame(display, frameFile); err != nil {
			fmt.Fprintf(os.Stderr, "[tg-viz] display error: %v\n", err)
		}

		frameNum++
		if frameNum%60 == 0 {
			fmt.Fprintf(os.Stderr, "[tg-viz] frames: %d\n", frameNum)
		}
	}
}

// findFreePort finds an available TCP port.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// waitForDevTools polls the Chrome DevTools JSON endpoint until a WebSocket
// URL is available or the timeout expires.
func waitForDevTools(ctx context.Context, port int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		var result struct {
			WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			time.Sleep(200 * time.Millisecond)
			continue
		}
		resp.Body.Close()

		if result.WebSocketDebuggerURL != "" {
			return result.WebSocketDebuggerURL, nil
		}

		time.Sleep(200 * time.Millisecond)
	}

	return "", fmt.Errorf("timeout waiting for Chrome DevTools on port %d", port)
}

// ── Minimal WebSocket Client ─────────────────────────────────────────────────
// Implements just enough of RFC 6455 for CDP communication.
// Using a minimal implementation to avoid adding gorilla/websocket dependency.

type wsConn struct {
	conn net.Conn
	buf  []byte
}

func dialWebSocket(ctx context.Context, wsURL string) (*wsConn, error) {
	// Parse ws:// URL.
	u := strings.TrimPrefix(wsURL, "ws://")
	hostPort := u
	path := "/"
	if idx := strings.Index(u, "/"); idx >= 0 {
		hostPort = u[:idx]
		path = u[idx:]
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return nil, err
	}

	// HTTP upgrade handshake.
	handshake := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n", path, hostPort)
	if _, err := conn.Write([]byte(handshake)); err != nil {
		conn.Close()
		return nil, err
	}

	// Read response (just need to get past the headers).
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(string(buf[:n]), "101") {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake failed: %s", string(buf[:n]))
	}

	return &wsConn{conn: conn, buf: make([]byte, 0, 1<<20)}, nil
}

func (ws *wsConn) Close() error {
	return ws.conn.Close()
}

// writeFrame sends a WebSocket text frame (client must mask).
func (ws *wsConn) writeFrame(data []byte) error {
	frame := make([]byte, 0, 14+len(data))
	frame = append(frame, 0x81) // FIN + text opcode

	// Payload length + mask bit.
	if len(data) < 126 {
		frame = append(frame, byte(len(data))|0x80)
	} else if len(data) < 65536 {
		frame = append(frame, 126|0x80, byte(len(data)>>8), byte(len(data)))
	} else {
		frame = append(frame, 127|0x80,
			0, 0, 0, 0,
			byte(len(data)>>24), byte(len(data)>>16), byte(len(data)>>8), byte(len(data)))
	}

	// Masking key (all zeros for simplicity -- Chrome doesn't care).
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	frame = append(frame, mask[:]...)

	// Masked payload.
	masked := make([]byte, len(data))
	for i, b := range data {
		masked[i] = b ^ mask[i%4]
	}
	frame = append(frame, masked...)

	_, err := ws.conn.Write(frame)
	return err
}

// readFrame reads one WebSocket frame payload. Handles continuation and large frames.
func (ws *wsConn) readFrame() ([]byte, error) {
	header := make([]byte, 2)
	if _, err := readFull(ws.conn, header); err != nil {
		return nil, err
	}

	masked := header[1]&0x80 != 0
	payloadLen := int(header[1] & 0x7f)

	switch payloadLen {
	case 126:
		ext := make([]byte, 2)
		if _, err := readFull(ws.conn, ext); err != nil {
			return nil, err
		}
		payloadLen = int(ext[0])<<8 | int(ext[1])
	case 127:
		ext := make([]byte, 8)
		if _, err := readFull(ws.conn, ext); err != nil {
			return nil, err
		}
		payloadLen = int(ext[4])<<24 | int(ext[5])<<16 | int(ext[6])<<8 | int(ext[7])
	}

	var maskKey [4]byte
	if masked {
		if _, err := readFull(ws.conn, maskKey[:]); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := readFull(ws.conn, payload); err != nil {
		return nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return payload, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func cdpSend(ws *wsConn, id int, method string, params json.RawMessage) error {
	msg := cdpMessage{ID: id, Method: method, Params: params}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return ws.writeFrame(data)
}

func cdpRecv(ws *wsConn) (*cdpMessage, error) {
	data, err := ws.readFrame()
	if err != nil {
		return nil, err
	}
	var msg cdpMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ── Screenshot Polling Fallback ─────────────────────────────────────────────

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func findChrome() string {
	home, _ := os.UserHomeDir()
	candidates := []string{}

	if home != "" {
		pattern := filepath.Join(home, ".cache", "puppeteer", "chrome", "*", "chrome-linux64", "chrome")
		matches, _ := filepath.Glob(pattern)
		candidates = append(candidates, matches...)
	}

	candidates = append(candidates,
		"/usr/bin/chromium",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/google-chrome",
		"/snap/bin/chromium",
	)

	for _, bin := range candidates {
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			if info.Mode()&0111 != 0 {
				return bin
			}
		}
	}
	return ""
}

func detectDisplay() displayMethod {
	if _, err := exec.LookPath("chafa"); err != nil {
		return displayNone
	}
	if os.Getenv("ZELLIJ") != "" {
		return displayChafaSymbols
	}
	return displayChafaKitty
}

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

func displayFrame(method displayMethod, path string) error {
	cols, rows := termSize()
	size := fmt.Sprintf("%dx%d", cols, rows)

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

func runScreenshotLoop(ctx context.Context, chromeBin string, method displayMethod, vizURL, framePath string, pollSec int) {
	fmt.Fprintf(os.Stderr, "[tg-viz] Polling mode: Chrome headless -> %s (every %ds)\n", method, pollSec)

	ticker := time.NewTicker(time.Duration(pollSec) * time.Second)
	defer ticker.Stop()

	captureAndDisplay := func() {
		if err := takeScreenshot(ctx, chromeBin, vizURL, framePath); err != nil {
			fmt.Print("\033[2J\033[H")
			fmt.Fprintf(os.Stderr, "[tg-viz] Waiting for %s ...\n", vizURL)
			return
		}
		if err := displayFrame(method, framePath); err != nil {
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

func runCarbonyl(ctx context.Context, vizURL string) {
	carbonylBin, err := exec.LookPath("carbonyl")
	if err != nil {
		fmt.Fprintln(os.Stderr, "No renderer available.")
		fmt.Fprintln(os.Stderr, "Install Chrome: sudo pacman -S chromium")
		fmt.Fprintln(os.Stderr, "  or Carbonyl: yay -S carbonyl-bin")
		fmt.Fprintln(os.Stderr, "  or run: npx puppeteer browsers install chrome")
		<-ctx.Done()
		return
	}

	fmt.Fprintln(os.Stderr, "[tg-viz] Falling back to carbonyl (character-grid rendering)")
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
