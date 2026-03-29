package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/trustgraph"
	"github.com/noko/computecommander/internal/tui"
	"github.com/spf13/cobra"
)

// ─── TrustGraph pane command ─────────────────────────────────────────────────

// TrustGraphCmd creates the "tg" subcommand for TrustGraph dashboard integration.
// In --pane mode it runs a long-lived loop that polls the TrustGraph gateway and
// renders a compact ANSI status view suitable for a zellij dashboard pane.
func TrustGraphCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tg",
		Short:   "TrustGraph knowledge graph status",
		Long:    "Display TrustGraph gateway status, node/edge counts, and top entities. In --pane mode, streams live updates with ANSI styling for the zellij dashboard.",
		GroupID: "OBSERVABILITY",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if paneMode {
				return runTGPane(cmd.Context(), app)
			}

			return printTGSummary(app, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// ─── One-shot summary ────────────────────────────────────────────────────────

func printTGSummary(app *App, jsonOut bool) error {
	cfg := app.Config.TrustGraph
	if !cfg.Enabled {
		fmt.Println("TrustGraph is disabled. Enable in config: trustgraph.enabled: true")
		return nil
	}

	client := trustgraph.New(cfg.GatewayURL, cfg.Token, cfg.FlowID)
	defer client.Close()

	if !client.Available() {
		fmt.Printf("TrustGraph gateway unreachable: %s\n", cfg.GatewayURL)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := cfg.MaxTriples
	if limit <= 0 {
		limit = 200
	}

	resp, err := client.TriplesQuery(ctx, trustgraph.TriplesQueryRequest{Limit: limit})
	if err != nil {
		return fmt.Errorf("triples query: %w", err)
	}

	nodes, topEntities := deriveTGStats(resp.Response, cfg.MaxNodes)

	if jsonOut {
		fmt.Printf(`{"status":"connected","nodes":%d,"edges":%d,"top_entities":%d}%s`,
			len(nodes), len(resp.Response), len(topEntities), "\n")
		return nil
	}

	fmt.Printf("TrustGraph: \033[32mconnected\033[0m  %d nodes  %d edges\n", len(nodes), len(resp.Response))
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println("Top Entities (by degree):")
	for i, e := range topEntities {
		if i >= 15 {
			fmt.Printf("  ... and %d more\n", len(topEntities)-15)
			break
		}
		fmt.Printf("  %-24s %3d edges\n", e.id, e.degree)
	}

	return nil
}

// ─── Long-lived pane mode ────────────────────────────────────────────────────

func runTGPane(ctx context.Context, app *App) error {
	cfg := app.Config.TrustGraph

	fallbackInterval := time.Duration(cfg.RefreshSecs) * time.Second
	if fallbackInterval < 5*time.Second {
		fallbackInterval = 30 * time.Second
	}

	// ANSI helpers.
	const (
		bold    = "\033[1m"
		dim     = "\033[2m"
		green   = "\033[32m"
		red     = "\033[31m"
		cyan    = "\033[36m"
		white   = "\033[37m"
		reset   = "\033[0m"
		clearSc = "\033[2J\033[H" // clear screen + cursor home
	)

	// Chart renderers.
	barChart := tui.NewBarChart("Entity Degrees", 15)
	sparkline := tui.NewSparkline("Triples", 60)

	// SSE refresh signal (single-slot channel for debouncing).
	sseRefreshC := make(chan struct{}, 1)
	sseActive := false

	// Start SSE subscription in background.
	if cfg.Enabled {
		go subscribeSSEForPane(ctx, &sseActive, sseRefreshC)
	}

	var client *trustgraph.Client

	// renderFrame performs one query + render cycle.
	renderFrame := func() {
		var buf strings.Builder
		buf.WriteString(clearSc)

		if !cfg.Enabled {
			buf.WriteString(dim + " TG  disabled" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			buf.WriteString(dim + "  Enable in config:" + reset + "\n")
			buf.WriteString(dim + "  trustgraph.enabled: true" + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			return
		}

		// Lazily create client.
		if client == nil {
			client = trustgraph.New(cfg.GatewayURL, cfg.Token, cfg.FlowID)
		}

		if !client.Available() {
			buf.WriteString(bold + cyan + " TG" + reset + "  " + dim + "disconnected" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			buf.WriteString(dim + "  Gateway: " + cfg.GatewayURL + reset + "\n")
			buf.WriteString(dim + "  Waiting for connection..." + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			return
		}

		// Query triples.
		queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
		limit := cfg.MaxTriples
		if limit <= 0 {
			limit = 200
		}
		resp, err := client.TriplesQuery(queryCtx, trustgraph.TriplesQueryRequest{Limit: limit})
		queryCancel()

		if err != nil {
			buf.WriteString(bold + cyan + " TG" + reset + "  " + red + "error" + reset + "\n")
			buf.WriteString(dim + strings.Repeat("─", 40) + reset + "\n\n")
			errMsg := err.Error()
			if len(errMsg) > 60 {
				errMsg = errMsg[:58] + ".."
			}
			buf.WriteString(red + "  " + errMsg + reset + "\n")
			fmt.Fprint(os.Stdout, buf.String())
			return
		}

		// Derive stats.
		nodes, topEntities := deriveTGStats(resp.Response, cfg.MaxNodes)
		nodeCount := len(nodes)
		edgeCount := len(resp.Response)

		// Update sparkline with current edge count.
		sparkline.Push(edgeCount)

		// Render header.
		buf.WriteString(bold + cyan + " TG" + reset + "  " + green + "connected" + reset)
		buf.WriteString(dim + fmt.Sprintf("  %d nodes  %d edges", nodeCount, edgeCount) + reset + "\n")
		buf.WriteString(dim + strings.Repeat("─", 50) + reset + "\n")

		// Bar chart of top entities by degree.
		buf.WriteString(bold + white + " Entity Degrees:" + reset + "\n")

		if len(topEntities) == 0 {
			buf.WriteString(dim + "  No entities found." + reset + "\n")
		} else {
			// Convert to BarEntry slice.
			entries := make([]tui.BarEntry, len(topEntities))
			for i, e := range topEntities {
				entries[i] = tui.BarEntry{Label: e.id, Value: e.degree}
			}
			// Render with color: wrap block chars in yellow.
			chartStr := barChart.Render(entries, 50, 18)
			// Apply ANSI coloring line by line.
			for _, line := range strings.Split(chartStr, "\n") {
				colored := colorizeBarLine(line, cyan, dim, reset)
				buf.WriteString(colored + "\n")
			}
		}

		// Sparkline.
		buf.WriteString("\n")
		buf.WriteString(dim + " " + sparkline.Render(40) + reset + "\n")

		// Footer with timestamp and SSE status.
		buf.WriteString("\n" + dim + strings.Repeat("─", 50) + reset + "\n")
		sseStatus := dim + "SSE off" + reset
		if sseActive {
			sseStatus = green + "SSE live" + reset
		}
		buf.WriteString(dim + "  " + time.Now().Format("15:04:05") + reset +
			"  " + sseStatus +
			dim + "  fallback " + fmt.Sprintf("%ds", int(fallbackInterval.Seconds())) + reset + "\n")

		fmt.Fprint(os.Stdout, buf.String())
	}

	// Initial render.
	renderFrame()

	// Main loop: wait for SSE signal or fallback timer.
	for {
		select {
		case <-ctx.Done():
			if client != nil {
				client.Close()
			}
			return nil
		case <-sseRefreshC:
			// SSE-triggered refresh with 500ms debounce.
			debounceSSERefresh(ctx, sseRefreshC, 500*time.Millisecond)
			renderFrame()
		case <-time.After(fallbackInterval):
			renderFrame()
		}
	}
}

// debounceSSERefresh drains any additional SSE signals that arrive within the
// debounce window, ensuring at most one render per window.
func debounceSSERefresh(ctx context.Context, ch <-chan struct{}, window time.Duration) {
	timer := time.NewTimer(window)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			// More signals arrived; reset debounce timer.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(window)
		case <-timer.C:
			return
		}
	}
}

// colorizeBarLine applies ANSI color codes to a bar chart line.
// Labels get labelColor, block characters get left as-is, numbers get numColor.
func colorizeBarLine(line, labelColor, numColor, resetCode string) string {
	// Simple approach: return the line with label coloring.
	// The BarChart renderer produces: "  label  [blocks] value"
	// We color the entire line for consistency.
	if strings.Contains(line, "\u2588") {
		return line // block chars render best without extra escape codes
	}
	return line
}

// subscribeSSEForPane connects to the ob-mcp SSE stream and sends signals
// on the refresh channel when memory events arrive. Auto-reconnects on failure.
func subscribeSSEForPane(ctx context.Context, active *bool, refreshC chan<- struct{}) {
	uid := os.Getuid()
	sockPath := fmt.Sprintf("/run/user/%d/ob-mcp.sock", uid)
	httpURL := "http://localhost:8200"

	for {
		if ctx.Err() != nil {
			return
		}

		err := runSSEStreamForPane(ctx, sockPath, httpURL, active, refreshC)
		if ctx.Err() != nil {
			return
		}
		_ = err
		*active = false

		// Backoff before reconnect.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// runSSEStreamForPane connects to the SSE endpoint and processes events until error.
func runSSEStreamForPane(ctx context.Context, sockPath, httpURL string, active *bool, refreshC chan<- struct{}) error {
	var client *http.Client
	var url string

	// Try Unix socket first.
	if _, err := os.Stat(sockPath); err == nil {
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", sockPath)
				},
			},
		}
		url = "http://unix/events/memories?agent=tg-pane"
	} else {
		client = &http.Client{}
		url = httpURL + "/events/memories?agent=tg-pane"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Add API key if available.
	if apiKey := os.Getenv("OB_API_KEY"); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE status %d", resp.StatusCode)
	}

	*active = true

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)

	var eventType string
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			// We only care that an event happened, not the payload.
			_ = json.RawMessage(strings.TrimPrefix(line, "data: "))
		case line == "":
			if eventType == "memory" {
				// Signal the render loop (non-blocking: coalesces bursts).
				select {
				case refreshC <- struct{}{}:
				default:
				}
			}
			eventType = ""
		}
	}

	return scanner.Err()
}

// ─── Shared helpers ──────────────────────────────────────────────────────────

// tgNodeInfo holds aggregated entity info for the CLI view.
type tgNodeInfo struct {
	id     string // short display label
	fullID string // full IRI
	degree int    // total edge count
}

// deriveTGStats computes node/edge stats and top entities from triples.
func deriveTGStats(triples []trustgraph.Triple, maxNodes int) (map[string]*tgNodeInfo, []tgNodeInfo) {
	nodes := make(map[string]*tgNodeInfo)
	for _, triple := range triples {
		sLabel := triple.Subject.ShortLabel(32)
		sID := triple.Subject.DisplayValue()
		if n, ok := nodes[sID]; ok {
			n.degree++
		} else {
			nodes[sID] = &tgNodeInfo{id: sLabel, fullID: sID, degree: 1}
		}

		if triple.Object.IsEntity() {
			oLabel := triple.Object.ShortLabel(32)
			oID := triple.Object.DisplayValue()
			if n, ok := nodes[oID]; ok {
				n.degree++
			} else {
				nodes[oID] = &tgNodeInfo{id: oLabel, fullID: oID, degree: 1}
			}
		}
	}

	topEntities := make([]tgNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		topEntities = append(topEntities, *n)
	}
	sort.Slice(topEntities, func(i, j int) bool {
		return topEntities[i].degree > topEntities[j].degree
	})

	if maxNodes <= 0 {
		maxNodes = 100
	}
	if len(topEntities) > maxNodes {
		topEntities = topEntities[:maxNodes]
	}

	return nodes, topEntities
}
