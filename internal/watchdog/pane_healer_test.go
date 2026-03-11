package watchdog

import (
	"testing"
	"time"

	"github.com/noko/computecommander/internal/zellij"
)

// mockPaneManager implements zellij.PaneManager for testing.
type mockPaneManager struct {
	panes          []*zellij.Pane
	capturedContent map[string]string
	closedPanes    []string
	createdPanes   []zellij.CreatePaneOpts
}

func newMockPaneManager() *mockPaneManager {
	return &mockPaneManager{
		capturedContent: make(map[string]string),
	}
}

func (m *mockPaneManager) CreatePane(opts zellij.CreatePaneOpts) (*zellij.Pane, error) {
	m.createdPanes = append(m.createdPanes, opts)
	p := &zellij.Pane{ID: "new-" + opts.Name, Name: opts.Name}
	return p, nil
}

func (m *mockPaneManager) ListPanes() ([]*zellij.Pane, error) {
	return m.panes, nil
}

func (m *mockPaneManager) SendKeys(paneID string, keys string) error {
	return nil
}

func (m *mockPaneManager) CapturePaneContent(paneID string, lines int) (string, error) {
	content, ok := m.capturedContent[paneID]
	if !ok {
		return "", nil
	}
	return content, nil
}

func (m *mockPaneManager) ClosePane(paneID string) error {
	m.closedPanes = append(m.closedPanes, paneID)
	return nil
}

func TestPaneHealerHealthyPane(t *testing.T) {
	mock := newMockPaneManager()
	mock.panes = []*zellij.Pane{
		{ID: "pane-1", Name: "Agents", Command: "cmdr status --pane"},
	}
	mock.capturedContent["pane-1"] = "content-frame-1"

	healer := NewPaneHealer(PaneHealerOpts{
		PaneManager:    mock,
		CheckInterval:  100 * time.Millisecond,
		FrozenThreshold: 1 * time.Second,
		MaxRestarts:    3,
	})

	// First check: establishes baseline.
	healer.checkPanes(nil)

	status := healer.GetStatus()
	if status["Agents"] != PaneHealthy {
		t.Errorf("expected healthy, got %s", status["Agents"])
	}

	// Change content — should stay healthy.
	mock.capturedContent["pane-1"] = "content-frame-2"
	healer.checkPanes(nil)

	status = healer.GetStatus()
	if status["Agents"] != PaneHealthy {
		t.Errorf("expected healthy after content change, got %s", status["Agents"])
	}
}

func TestPaneHealerFrozenPane(t *testing.T) {
	mock := newMockPaneManager()
	mock.panes = []*zellij.Pane{
		{ID: "pane-1", Name: "Agents", Command: "cmdr status --pane"},
	}
	mock.capturedContent["pane-1"] = "static-content"

	healer := NewPaneHealer(PaneHealerOpts{
		PaneManager:    mock,
		CheckInterval:  100 * time.Millisecond,
		FrozenThreshold: 50 * time.Millisecond, // Very short for testing.
		MaxRestarts:    3,
	})

	// First check: establishes baseline.
	healer.checkPanes(nil)

	// Wait past threshold.
	time.Sleep(100 * time.Millisecond)

	// Second check: same content, should detect frozen and restart.
	healer.checkPanes(nil)

	// Verify that a close and create happened.
	if len(mock.closedPanes) != 1 {
		t.Errorf("expected 1 close, got %d", len(mock.closedPanes))
	}
	if len(mock.createdPanes) != 1 {
		t.Errorf("expected 1 create, got %d", len(mock.createdPanes))
	}
}

func TestPaneHealerMaxRestarts(t *testing.T) {
	mock := newMockPaneManager()
	mock.panes = []*zellij.Pane{
		{ID: "pane-1", Name: "TestPane", Command: "test-cmd"},
	}
	mock.capturedContent["pane-1"] = "frozen-content"

	healer := NewPaneHealer(PaneHealerOpts{
		PaneManager:    mock,
		CheckInterval:  10 * time.Millisecond,
		FrozenThreshold: 1 * time.Millisecond,
		MaxRestarts:    2,
	})

	// Simulate exceeding max restarts.
	healer.snapshots["TestPane"] = &paneSnapshot{
		ContentHash: hashPaneContent("frozen-content"),
		LastChange:  time.Now().Add(-time.Hour),
		Restarts:    2,
		Command:     "test-cmd",
		Status:      PaneFrozen,
	}

	healer.checkPane(&zellij.Pane{ID: "pane-1", Name: "TestPane"})

	status := healer.GetStatus()
	if status["TestPane"] != PaneAbandoned {
		t.Errorf("expected abandoned after max restarts, got %s", status["TestPane"])
	}
}

func TestPaneHealerHashContent(t *testing.T) {
	h1 := hashPaneContent("hello world")
	h2 := hashPaneContent("hello world")
	h3 := hashPaneContent("different content")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestPaneHealerSkipsUnnamedPanes(t *testing.T) {
	mock := newMockPaneManager()
	mock.panes = []*zellij.Pane{
		{ID: "pane-1", Name: "", Command: "agent"},
	}

	healer := NewPaneHealer(PaneHealerOpts{
		PaneManager:    mock,
		CheckInterval:  100 * time.Millisecond,
		FrozenThreshold: 1 * time.Second,
		MaxRestarts:    3,
	})

	healer.checkPanes(nil)

	status := healer.GetStatus()
	if len(status) != 0 {
		t.Errorf("expected 0 tracked panes (unnamed skipped), got %d", len(status))
	}
}

func TestSplitFields(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"cmdr status --pane", 3},
		{"single", 1},
		{"", 0},
		{"  spaces  between  ", 2},
	}
	for _, tc := range cases {
		got := splitFields(tc.input)
		if len(got) != tc.want {
			t.Errorf("splitFields(%q) returned %d fields, want %d", tc.input, len(got), tc.want)
		}
	}
}
