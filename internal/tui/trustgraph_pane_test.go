package tui

import (
	"strings"
	"testing"

	"github.com/noko/computecommander/internal/config"
)

func TestTrustGraphPane_ViewDisconnected(t *testing.T) {
	theme := DefaultTheme()
	cfg := config.TrustGraphConfig{
		Enabled:    false,
		GatewayURL: "http://localhost:8088",
		MaxTriples: 200,
		MaxNodes:   100,
	}

	pane := NewTrustGraphPane(theme, cfg)
	pane.SetSize(80, 24)

	view := pane.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	// Should show disconnected status.
	if !strings.Contains(view, "disconnected") {
		t.Errorf("expected 'disconnected' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Enable in config") {
		t.Errorf("expected config hint in view when disabled, got:\n%s", view)
	}
}

func TestTrustGraphPane_ViewDisconnectedEnabled(t *testing.T) {
	theme := DefaultTheme()
	cfg := config.TrustGraphConfig{
		Enabled:    true,
		GatewayURL: "http://localhost:19999", // non-existent port
		MaxTriples: 200,
		MaxNodes:   100,
	}

	pane := NewTrustGraphPane(theme, cfg)
	defer pane.Close()
	pane.SetSize(80, 24)

	// After construction, the initial probe will fail (port not listening).
	// Refresh should set status to disconnected.
	_ = pane.Refresh()

	view := pane.View()
	if !strings.Contains(view, "disconnected") {
		t.Errorf("expected 'disconnected' in view for unavailable gateway, got:\n%s", view)
	}
	if !strings.Contains(view, "Gateway URL") {
		t.Errorf("expected gateway URL in view, got:\n%s", view)
	}
}

func TestTrustGraphPane_SetSize(t *testing.T) {
	theme := DefaultTheme()
	cfg := config.TrustGraphConfig{Enabled: false}
	pane := NewTrustGraphPane(theme, cfg)

	pane.SetSize(120, 40)
	if pane.width != 120 {
		t.Errorf("width = %d, want 120", pane.width)
	}
	if pane.height != 40 {
		t.Errorf("height = %d, want 40", pane.height)
	}
}

func TestTrustGraphPane_ScrollUpDown(t *testing.T) {
	theme := DefaultTheme()
	cfg := config.TrustGraphConfig{Enabled: false}
	pane := NewTrustGraphPane(theme, cfg)
	pane.SetSize(80, 24)

	// Populate some fake entities to test scrolling.
	pane.mu.Lock()
	pane.topEntities = make([]nodeInfo, 20)
	for i := range pane.topEntities {
		pane.topEntities[i] = nodeInfo{
			ID:     strings.Repeat("x", i+1),
			FullID: strings.Repeat("x", i+1),
			Degree: 20 - i,
		}
	}
	pane.mu.Unlock()

	// Cursor starts at 0.
	if pane.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", pane.cursor)
	}

	pane.ScrollDown()
	if pane.cursor != 1 {
		t.Errorf("after ScrollDown cursor = %d, want 1", pane.cursor)
	}

	pane.ScrollUp()
	if pane.cursor != 0 {
		t.Errorf("after ScrollUp cursor = %d, want 0", pane.cursor)
	}

	// ScrollUp at top should not go negative.
	pane.ScrollUp()
	if pane.cursor != 0 {
		t.Errorf("ScrollUp at top: cursor = %d, want 0", pane.cursor)
	}
}

func TestTrustGraphPane_NilClient(t *testing.T) {
	theme := DefaultTheme()
	cfg := config.TrustGraphConfig{
		Enabled:    false,
		GatewayURL: "",
	}

	pane := NewTrustGraphPane(theme, cfg)
	// Close should not panic with nil client.
	pane.Close()

	// Refresh should not panic with nil client.
	err := pane.Refresh()
	if err != nil {
		t.Errorf("Refresh with nil client: %v", err)
	}
}

func TestTGStatus_String(t *testing.T) {
	tests := []struct {
		status TGStatus
		want   string
	}{
		{TGConnected, "connected"},
		{TGDisconnected, "disconnected"},
		{TGError, "error"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("TGStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}
