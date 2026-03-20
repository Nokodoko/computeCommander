package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/config"
)

// ConnectionStatus represents the state of the MCP server connection.
type ConnectionStatus int

const (
	// StatusDisconnected means the MCP server is not reachable.
	StatusDisconnected ConnectionStatus = iota
	// StatusConnected means the MCP server is reachable and responding.
	StatusConnected
	// StatusError means the MCP server returned an error (auth, etc).
	StatusError
)

// String returns a human-readable status label.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusConnected:
		return "connected"
	case StatusError:
		return "error"
	default:
		return "disconnected"
	}
}

// OpenBrainEntry represents a memory item from the MCP server.
type OpenBrainEntry struct {
	ID         string   `json:"id"`
	Type       string   `json:"item_type"`
	Content    string   `json:"raw_content"`
	Priority   int      `json:"priority"`
	Source     string   `json:"source"`
	Tags       []string `json:"tags"`
	CreatedAt  string   `json:"created_at"`
	Session    string   `json:"session_name"`
}

// OpenBrainPane displays memory entries from the OpenBrain MCP server.
// It replaces the placeholder with a full implementation that renders
// entries with type glyphs and runtime color coding per the spec.
type OpenBrainPane struct {
	theme      *Theme
	cfg        config.OpenBrainConfig
	width      int
	height     int
	entries    []OpenBrainEntry
	status     ConnectionStatus
	lastWrite  time.Time
	totalCount int
	mu         sync.Mutex
	client     *http.Client
}

// NewOpenBrainPane constructs an OpenBrainPane with MCP server configuration.
func NewOpenBrainPane(theme *Theme, cfg config.OpenBrainConfig) *OpenBrainPane {
	return &OpenBrainPane{
		theme: theme,
		cfg:   cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SetSize updates display dimensions.
func (ob *OpenBrainPane) SetSize(w, h int) {
	ob.width = w
	ob.height = h
}

// Refresh fetches the latest entries from the gateway REST endpoint.
func (ob *OpenBrainPane) Refresh() error {
	if !ob.cfg.Enabled || ob.cfg.MCPSseURL == "" {
		ob.mu.Lock()
		ob.status = StatusDisconnected
		ob.mu.Unlock()
		return nil
	}

	url := fmt.Sprintf("%s/api/v1/openbrain/entries?limit=%d", ob.cfg.MCPSseURL, ob.cfg.MaxEntries)
	if ob.cfg.DefaultSince != "" {
		// Convert duration like "72h" to ISO 8601 since timestamp.
		dur, err := time.ParseDuration(ob.cfg.DefaultSince)
		if err == nil {
			since := time.Now().Add(-dur).UTC().Format(time.RFC3339)
			url += "&since=" + since
		}
	}
	if ob.cfg.APIKey != "" {
		url += "&api_key=" + ob.cfg.APIKey
	}

	resp, err := ob.client.Get(url)
	if err != nil {
		ob.mu.Lock()
		ob.status = StatusDisconnected
		ob.mu.Unlock()
		return nil // Graceful degradation: don't return error.
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		ob.mu.Lock()
		ob.status = StatusError
		ob.mu.Unlock()
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		ob.mu.Lock()
		ob.status = StatusError
		ob.mu.Unlock()
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ob.mu.Lock()
		ob.status = StatusError
		ob.mu.Unlock()
		return nil
	}

	var result struct {
		Entries []OpenBrainEntry `json:"entries"`
		Count   int             `json:"count"`
		HasMore bool            `json:"has_more"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		ob.mu.Lock()
		ob.status = StatusError
		ob.mu.Unlock()
		return nil
	}

	ob.mu.Lock()
	ob.entries = result.Entries
	ob.totalCount = result.Count
	ob.status = StatusConnected
	if len(ob.entries) > 0 {
		if t, err := time.Parse(time.RFC3339, ob.entries[0].CreatedAt); err == nil {
			ob.lastWrite = t
		}
	}
	ob.mu.Unlock()

	return nil
}

// HandleEntry accepts a new entry from the SSE stream and prepends it.
func (ob *OpenBrainPane) HandleEntry(e OpenBrainEntry) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Prepend (newest first).
	ob.entries = append([]OpenBrainEntry{e}, ob.entries...)

	// Trim to max entries.
	max := ob.cfg.MaxEntries
	if max <= 0 {
		max = 20
	}
	if len(ob.entries) > max {
		ob.entries = ob.entries[:max]
	}

	ob.totalCount++
	if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil {
		ob.lastWrite = t
	}
}

// View renders the pane content with color-coded entries.
func (ob *OpenBrainPane) View() string {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	w := ob.width
	if w <= 0 {
		w = 60
	}

	var lines []string

	// Header: "Knowledge (N entries, last 72h)"
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF00FF"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	if len(ob.entries) > 0 {
		header := headerStyle.Render(" Knowledge") +
			dimStyle.Render(fmt.Sprintf("  (%d entries)", ob.totalCount))
		lines = append(lines, header)
	} else if ob.status == StatusDisconnected {
		lines = append(lines, dimStyle.Render(" Knowledge  (disconnected)"))
	} else if ob.status == StatusError {
		lines = append(lines, headerStyle.Render(" Knowledge") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("  (auth error)"))
	} else {
		lines = append(lines, dimStyle.Render(" Knowledge  (no entries)"))
	}

	// Render entries.
	for _, e := range ob.entries {
		line := ob.renderEntry(e, w)
		lines = append(lines, line)
	}

	// Status line.
	lines = append(lines, ob.renderStatusLine(w))

	return strings.Join(lines, "\n")
}

// renderEntry renders a single entry line with type glyph and runtime color.
func (ob *OpenBrainPane) renderEntry(e OpenBrainEntry, maxW int) string {
	glyph, glyphColor := obEntryTypeGlyph(e.Type)
	age := obFormatAge(e.CreatedAt)
	rtColor := obRuntimeColor(e.Source)

	// Build the line: " D  2m ago   Switched from PTY embedding...       claude"
	glyphStr := lipgloss.NewStyle().Foreground(glyphColor).Bold(true).Render(glyph)

	// Truncate content for available width.
	// Layout: " G  AGE   CONTENT   SOURCE"
	ageStr := fmt.Sprintf("%-7s", age)
	sourceStr := obSourceShort(e.Source)

	// Available width for content: total - glyph(3) - age(9) - source(10) - spacing(5)
	contentW := maxW - 27
	if contentW < 10 {
		contentW = 10
	}
	content := e.Content
	if len(content) > contentW {
		content = content[:contentW-2] + ".."
	}

	contentPadded := fmt.Sprintf("%-*s", contentW, content)

	sourceStyled := lipgloss.NewStyle().Foreground(rtColor).Render(sourceStr)

	return fmt.Sprintf(" %s  %s %s %s", glyphStr, ageStr, contentPadded, sourceStyled)
}

// renderStatusLine renders the bottom status bar.
func (ob *OpenBrainPane) renderStatusLine(maxW int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	sep := dim.Render(strings.Repeat("-", maxW))

	var statusParts []string
	switch ob.status {
	case StatusConnected:
		statusParts = append(statusParts,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Render("connected"))
	case StatusError:
		statusParts = append(statusParts,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("error"))
	default:
		statusParts = append(statusParts,
			dim.Render("disconnected"))
	}

	statusParts = append(statusParts,
		dim.Render(fmt.Sprintf("%d entries", ob.totalCount)))

	if !ob.lastWrite.IsZero() {
		ago := time.Since(ob.lastWrite)
		var agoStr string
		switch {
		case ago < time.Minute:
			agoStr = "just now"
		case ago < time.Hour:
			agoStr = fmt.Sprintf("%dm ago", int(ago.Minutes()))
		case ago < 24*time.Hour:
			agoStr = fmt.Sprintf("%dh ago", int(ago.Hours()))
		default:
			agoStr = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
		}
		statusParts = append(statusParts,
			dim.Render("last write "+agoStr))
	}

	status := " " + dim.Render("Status") + "  " + strings.Join(statusParts, dim.Render(" | "))

	return sep + "\n" + status
}

// ── Color helpers ────────────────────────────────────────────────────────────

// obEntryTypeGlyph returns the display glyph and color for an entry type.
// Matches SPEC.md and openbrain-rules.md.
func obEntryTypeGlyph(itemType string) (string, lipgloss.Color) {
	switch itemType {
	case "reference", "project":
		return "P", lipgloss.Color("#00FFFF") // Bold cyan
	case "task":
		return "T", lipgloss.Color("#FFFF00") // Bold yellow
	case "event":
		return "E", lipgloss.Color("#AAAAAA") // Dim white
	case "idea", "observation":
		return "?", lipgloss.Color("#00FFFF") // Cyan
	case "decision":
		return "D", lipgloss.Color("#FFFFFF") // Bold white
	case "discovery":
		return "?", lipgloss.Color("#00FFFF") // Cyan
	case "warning":
		return "!", lipgloss.Color("#FFFF00") // Yellow bold
	case "solution":
		return "S", lipgloss.Color("#50fa7b") // Green
	case "context":
		return "~", lipgloss.Color("#555555") // Dim
	default:
		return ".", lipgloss.Color("#555555")
	}
}

// obRuntimeColor returns the lipgloss color for a given runtime/source.
// Matches the spec runtime color map.
func obRuntimeColor(source string) lipgloss.Color {
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "claude"):
		return lipgloss.Color("#5DADE2") // Blue
	case strings.Contains(s, "pi"):
		return lipgloss.Color("#9B59B6") // Magenta
	case strings.Contains(s, "gemini"):
		return lipgloss.Color("#4ECDC4") // Cyan
	case strings.Contains(s, "codex"):
		return lipgloss.Color("#82E0AA") // Green
	case strings.Contains(s, "goose"):
		return lipgloss.Color("#FFB347") // Yellow
	default:
		return lipgloss.Color("#888888") // Dim fallback
	}
}

// obSourceShort returns a shortened source name for display (max 8 chars).
func obSourceShort(source string) string {
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "claude"):
		return "claude"
	case strings.Contains(s, "pi"):
		return "pi"
	case strings.Contains(s, "gemini"):
		return "gemini"
	case strings.Contains(s, "codex"):
		return "codex"
	case strings.Contains(s, "goose"):
		return "goose"
	default:
		if len(source) > 8 {
			return source[:8]
		}
		return source
	}
}

// obFormatAge returns a human-readable age string from an RFC3339 timestamp.
func obFormatAge(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		// Try alternate format.
		t, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return ""
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
