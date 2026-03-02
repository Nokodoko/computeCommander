package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/noko/computecommander/internal/keybinds"
)

// LeaderKeyHandler manages the leader key state machine for the TUI.
// After Ctrl+Space is pressed, the next key is looked up in the keybind config
// and dispatched to the action registry.
type LeaderKeyHandler struct {
	config   *keybinds.Config
	registry *keybinds.Registry
	active   bool
}

// NewLeaderKeyHandler creates a new leader key handler.
func NewLeaderKeyHandler(cfg *keybinds.Config, reg *keybinds.Registry) *LeaderKeyHandler {
	return &LeaderKeyHandler{
		config:   cfg,
		registry: reg,
	}
}

// IsActive returns true if the leader key has been pressed and we're waiting
// for the follow-up key.
func (h *LeaderKeyHandler) IsActive() bool {
	return h.active
}

// Activate sets the handler into leader-key-active mode.
func (h *LeaderKeyHandler) Activate() {
	h.active = true
}

// Deactivate clears the leader-key-active state.
func (h *LeaderKeyHandler) Deactivate() {
	h.active = false
}

// HandleKey processes a key press when the leader key is active.
// Returns the action name that was dispatched (or empty string),
// and a bubbletea command if the action triggered one.
func (h *LeaderKeyHandler) HandleKey(key string) (string, tea.Cmd) {
	h.active = false

	if h.config == nil {
		return "", nil
	}

	action := h.config.LookupAction(key)
	if action == "" {
		return "", nil
	}

	// Special actions that affect the TUI directly.
	switch action {
	case "quit":
		return action, tea.Quit
	}

	// Try to execute the action via the registry.
	if h.registry != nil && h.registry.HasAction(action) {
		_ = h.registry.Execute(action)
	}

	return action, nil
}

// IsLeaderKey returns true if the key message is the leader key (Ctrl+Space).
func IsLeaderKey(msg tea.KeyMsg) bool {
	key := msg.String()
	// Ctrl+Space sends different codes depending on the terminal:
	// - "ctrl+@" (most terminals)
	// - "ctrl+ " (some terminals)
	return key == "ctrl+@" || key == "ctrl+ "
}
