package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/trustgraph"
)

// TGStatus represents the TrustGraph connection state.
type TGStatus int

const (
	// TGDisconnected means the TrustGraph gateway is not reachable.
	TGDisconnected TGStatus = iota
	// TGConnected means the gateway is reachable and responding.
	TGConnected
	// TGError means the gateway returned an error.
	TGError
)

// String returns a human-readable status label.
func (s TGStatus) String() string {
	switch s {
	case TGConnected:
		return "connected"
	case TGError:
		return "error"
	default:
		return "disconnected"
	}
}

// nodeInfo holds aggregated information about a graph entity for the summary view.
type nodeInfo struct {
	ID     string // display label (short IRI)
	FullID string // full IRI for lookups
	Degree int    // total edge count (as subject + as object)
}

// TrustGraphPane displays TrustGraph knowledge graph data.
// Phase 1 (MVP): Summary View only, showing node/edge counts and top entities.
// Renders as a full-screen overlay when focused (same pattern as JiraPane).
type TrustGraphPane struct {
	theme  *Theme
	cfg    config.TrustGraphConfig
	client *trustgraph.Client
	width  int
	height int

	// Cached graph data (protected by mu).
	mu          sync.Mutex
	status      TGStatus
	triples     []trustgraph.Triple
	nodeCount   int
	edgeCount   int
	topEntities []nodeInfo
	lastRefresh time.Time
	lastError   string

	// Scroll state for summary view.
	scrollOffset int
	cursor       int
}

// NewTrustGraphPane constructs a TrustGraphPane with the given config.
// If TrustGraph is disabled or GatewayURL is empty, the pane shows "disconnected".
func NewTrustGraphPane(theme *Theme, cfg config.TrustGraphConfig) *TrustGraphPane {
	p := &TrustGraphPane{
		theme: theme,
		cfg:   cfg,
	}

	if cfg.Enabled && cfg.GatewayURL != "" {
		p.client = trustgraph.New(cfg.GatewayURL, cfg.Token)
	}

	return p
}

// SetSize updates display dimensions.
func (tg *TrustGraphPane) SetSize(w, h int) {
	tg.width = w
	tg.height = h
}

// Close stops the TrustGraph client's background health probe.
func (tg *TrustGraphPane) Close() {
	if tg.client != nil {
		tg.client.Close()
	}
}

// ScrollDown moves the cursor down in the entity list.
func (tg *TrustGraphPane) ScrollDown() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if tg.cursor < len(tg.topEntities)-1 {
		tg.cursor++
	}
	// Adjust scroll offset to keep cursor visible.
	visibleRows := tg.height - 10 // approximate header/footer overhead
	if visibleRows < 1 {
		visibleRows = 1
	}
	if tg.cursor >= tg.scrollOffset+visibleRows {
		tg.scrollOffset = tg.cursor - visibleRows + 1
	}
}

// ScrollUp moves the cursor up in the entity list.
func (tg *TrustGraphPane) ScrollUp() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if tg.cursor > 0 {
		tg.cursor--
	}
	if tg.cursor < tg.scrollOffset {
		tg.scrollOffset = tg.cursor
	}
}

// Refresh fetches the latest graph data from the TrustGraph gateway.
// It queries for all triples (up to MaxTriples) and derives node/edge stats.
func (tg *TrustGraphPane) Refresh() error {
	if !tg.cfg.Enabled || tg.client == nil {
		tg.mu.Lock()
		tg.status = TGDisconnected
		tg.mu.Unlock()
		return nil
	}

	// Check availability before making API calls.
	if !tg.client.Available() {
		tg.mu.Lock()
		tg.status = TGDisconnected
		tg.mu.Unlock()
		return nil
	}

	// Respect refresh interval to avoid hammering the gateway.
	tg.mu.Lock()
	if !tg.lastRefresh.IsZero() && time.Since(tg.lastRefresh) < time.Duration(tg.cfg.RefreshSecs)*time.Second {
		tg.mu.Unlock()
		return nil
	}
	tg.mu.Unlock()

	// Fetch triples with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limit := tg.cfg.MaxTriples
	if limit <= 0 {
		limit = 200
	}

	resp, err := tg.client.TriplesQuery(ctx, trustgraph.TriplesQueryRequest{
		Limit: limit,
	})
	if err != nil {
		tg.mu.Lock()
		tg.status = TGError
		tg.lastError = err.Error()
		tg.mu.Unlock()
		return nil // Graceful degradation: don't propagate error.
	}

	// Derive node/edge statistics from triples.
	nodes := make(map[string]*nodeInfo)
	for _, triple := range resp.Response {
		// Count subject as a node.
		sLabel := triple.Subject.ShortLabel(32)
		sID := triple.Subject.DisplayValue()
		if n, ok := nodes[sID]; ok {
			n.Degree++
		} else {
			nodes[sID] = &nodeInfo{ID: sLabel, FullID: sID, Degree: 1}
		}

		// Count object as a node (if it's an entity, not a literal).
		if triple.Object.IsEntity() {
			oLabel := triple.Object.ShortLabel(32)
			oID := triple.Object.DisplayValue()
			if n, ok := nodes[oID]; ok {
				n.Degree++
			} else {
				nodes[oID] = &nodeInfo{ID: oLabel, FullID: oID, Degree: 1}
			}
		}
	}

	// Sort by degree (descending) for top entities.
	topEntities := make([]nodeInfo, 0, len(nodes))
	for _, n := range nodes {
		topEntities = append(topEntities, *n)
	}
	sort.Slice(topEntities, func(i, j int) bool {
		return topEntities[i].Degree > topEntities[j].Degree
	})

	maxNodes := tg.cfg.MaxNodes
	if maxNodes <= 0 {
		maxNodes = 100
	}
	if len(topEntities) > maxNodes {
		topEntities = topEntities[:maxNodes]
	}

	tg.mu.Lock()
	tg.triples = resp.Response
	tg.nodeCount = len(nodes)
	tg.edgeCount = len(resp.Response)
	tg.topEntities = topEntities
	tg.status = TGConnected
	tg.lastRefresh = time.Now()
	tg.lastError = ""
	tg.mu.Unlock()

	return nil
}

// View renders the TrustGraph Summary View content.
func (tg *TrustGraphPane) View() string {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	w := tg.width
	if w <= 0 {
		w = 60
	}

	var lines []string

	// ── Header ──────────────────────────────────────────────────────────
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5DADE2"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	redStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))

	statusStr := ""
	switch tg.status {
	case TGConnected:
		statusStr = greenStyle.Render("connected")
	case TGError:
		statusStr = redStyle.Render("error")
	default:
		statusStr = dimStyle.Render("disconnected")
	}

	header := headerStyle.Render(" TG") + "  " + statusStr
	if tg.status == TGConnected {
		header += dimStyle.Render(fmt.Sprintf("  %d nodes  %d edges", tg.nodeCount, tg.edgeCount))
	}
	lines = append(lines, header)

	// Separator.
	sep := dimStyle.Render(strings.Repeat("─", w))
	lines = append(lines, sep)

	if tg.status == TGDisconnected {
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  TrustGraph gateway is not available."))
		if !tg.cfg.Enabled {
			lines = append(lines, dimStyle.Render("  Enable in config: trustgraph.enabled: true"))
		} else {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  Gateway URL: %s", tg.cfg.GatewayURL)))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  Waiting for connection..."))
		return strings.Join(lines, "\n")
	}

	if tg.status == TGError {
		lines = append(lines, "")
		lines = append(lines, redStyle.Render("  Error connecting to TrustGraph:"))
		if tg.lastError != "" {
			errMsg := tg.lastError
			if len(errMsg) > w-4 {
				errMsg = errMsg[:w-6] + ".."
			}
			lines = append(lines, redStyle.Render("  "+errMsg))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render(fmt.Sprintf("  Gateway URL: %s", tg.cfg.GatewayURL)))
		return strings.Join(lines, "\n")
	}

	// ── Top Entities ────────────────────────────────────────────────────
	entityHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	entityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5DADE2"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50fa7b"))
	countStyle := dimStyle

	lines = append(lines, entityHeaderStyle.Render(" Top Entities (by degree):"))
	lines = append(lines, "")

	if len(tg.topEntities) == 0 {
		lines = append(lines, dimStyle.Render("  No entities found."))
	} else {
		// Calculate visible range.
		visibleRows := tg.height - 10
		if visibleRows < 5 {
			visibleRows = 5
		}
		endIdx := tg.scrollOffset + visibleRows
		if endIdx > len(tg.topEntities) {
			endIdx = len(tg.topEntities)
		}
		startIdx := tg.scrollOffset
		if startIdx < 0 {
			startIdx = 0
		}

		// Find max entity name length for alignment.
		maxNameLen := 0
		for _, e := range tg.topEntities[startIdx:endIdx] {
			if len(e.ID) > maxNameLen {
				maxNameLen = len(e.ID)
			}
		}
		if maxNameLen > 24 {
			maxNameLen = 24
		}

		for i := startIdx; i < endIdx; i++ {
			e := tg.topEntities[i]
			name := e.ID
			if len(name) > 24 {
				name = name[:22] + ".."
			}

			prefix := "  "
			nameStyled := entityStyle.Render(fmt.Sprintf("%-*s", maxNameLen, name))
			if i == tg.cursor {
				prefix = "> "
				nameStyled = cursorStyle.Render(fmt.Sprintf("%-*s", maxNameLen, name))
			}

			line := prefix + nameStyled + "  " + countStyle.Render(fmt.Sprintf("%3d edges", e.Degree))
			lines = append(lines, line)
		}

		if endIdx < len(tg.topEntities) {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more entities", len(tg.topEntities)-endIdx)))
		}
	}

	// ── Recent Triples ──────────────────────────────────────────────────
	lines = append(lines, "")
	lines = append(lines, entityHeaderStyle.Render(" Recent triples:"))
	lines = append(lines, "")

	recentCount := 5
	if len(tg.triples) < recentCount {
		recentCount = len(tg.triples)
	}

	predStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	for i := 0; i < recentCount; i++ {
		t := tg.triples[i]
		subj := t.Subject.ShortLabel(16)
		pred := t.Predicate.ShortLabel(16)
		obj := t.Object.ShortLabel(16)

		line := "  " + entityStyle.Render(subj) + " " +
			predStyle.Render("--"+pred+"-->") + " " +
			entityStyle.Render(obj)
		lines = append(lines, line)
	}

	if len(tg.triples) == 0 {
		lines = append(lines, dimStyle.Render("  No triples found."))
	}

	// ── Status Bar ──────────────────────────────────────────────────────
	lines = append(lines, "")
	lines = append(lines, sep)

	var statusParts []string
	statusParts = append(statusParts, greenStyle.Render("connected"))
	statusParts = append(statusParts, dimStyle.Render(fmt.Sprintf("%d nodes", tg.nodeCount)))
	statusParts = append(statusParts, dimStyle.Render(fmt.Sprintf("%d edges", tg.edgeCount)))

	if !tg.lastRefresh.IsZero() {
		ago := time.Since(tg.lastRefresh)
		var agoStr string
		switch {
		case ago < time.Minute:
			agoStr = fmt.Sprintf("%ds ago", int(ago.Seconds()))
		case ago < time.Hour:
			agoStr = fmt.Sprintf("%dm ago", int(ago.Minutes()))
		default:
			agoStr = fmt.Sprintf("%dh ago", int(ago.Hours()))
		}
		statusParts = append(statusParts, dimStyle.Render(agoStr))
	}

	statusLine := " " + dimStyle.Render("Status") + "  " + strings.Join(statusParts, dimStyle.Render(" | "))
	lines = append(lines, statusLine)

	// Help bar for full-screen overlay.
	lines = append(lines, "")
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	lines = append(lines, helpStyle.Render(" j/k:scroll  r:refresh  Tab:back  q:quit"))

	return strings.Join(lines, "\n")
}
