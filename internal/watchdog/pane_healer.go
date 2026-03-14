package watchdog

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"github.com/noko/computecommander/internal/zellij"
)

// PaneHealer monitors dashboard panes for frozen/stale states and automatically
// restarts them.
type PaneHealer struct {
	panes       zellij.PaneManager
	interval    time.Duration
	frozenThr   time.Duration
	snapshots   map[string]*paneSnapshot
	maxRestarts int
}

// paneSnapshot tracks the state of a single monitored pane.
type paneSnapshot struct {
	ContentHash string
	LastChange  time.Time
	Restarts    int
	Command     string
	Status      PaneHealthStatus
}

// PaneHealthStatus represents the health state of a pane.
type PaneHealthStatus string

const (
	PaneHealthy    PaneHealthStatus = "healthy"
	PaneFrozen     PaneHealthStatus = "frozen"
	PaneStale      PaneHealthStatus = "stale"
	PaneRestarting PaneHealthStatus = "restarting"
	PaneAbandoned  PaneHealthStatus = "abandoned"
)

// PaneHealerOpts configures the PaneHealer.
type PaneHealerOpts struct {
	PaneManager    zellij.PaneManager
	CheckInterval  time.Duration
	FrozenThreshold time.Duration
	MaxRestarts    int
}

// NewPaneHealer creates a PaneHealer with the given options.
func NewPaneHealer(opts PaneHealerOpts) *PaneHealer {
	interval := opts.CheckInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	frozenThr := opts.FrozenThreshold
	if frozenThr <= 0 {
		frozenThr = 30 * time.Second
	}
	maxRestarts := opts.MaxRestarts
	if maxRestarts <= 0 {
		maxRestarts = 5
	}

	return &PaneHealer{
		panes:       opts.PaneManager,
		interval:    interval,
		frozenThr:   frozenThr,
		snapshots:   make(map[string]*paneSnapshot),
		maxRestarts: maxRestarts,
	}
}

// Run starts the pane health monitoring loop. It blocks until the context is cancelled.
func (h *PaneHealer) Run(ctx context.Context) error {
	if h.panes == nil {
		return fmt.Errorf("pane healer: no pane manager configured")
	}

	log.Printf("pane-healer: starting with interval=%s frozen_threshold=%s max_restarts=%d",
		h.interval, h.frozenThr, h.maxRestarts)

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("pane-healer: shutting down")
			return ctx.Err()
		case <-ticker.C:
			h.checkPanes(ctx)
		}
	}
}

// checkPanes inspects all panes and heals any that are frozen or stale.
func (h *PaneHealer) checkPanes(_ context.Context) {
	panes, err := h.panes.ListPanes()
	if err != nil {
		log.Printf("pane-healer: list panes error: %v", err)
		return
	}

	for _, pane := range panes {
		if pane.Name == "" {
			continue // Skip unnamed panes (like the agent session).
		}

		h.checkPane(pane)
	}
}

// checkPane evaluates a single pane's health and takes action if needed.
func (h *PaneHealer) checkPane(pane *zellij.Pane) {
	snap, exists := h.snapshots[pane.Name]
	if !exists {
		snap = &paneSnapshot{
			LastChange: time.Now(),
			Command:    pane.Command,
			Status:     PaneHealthy,
		}
		h.snapshots[pane.Name] = snap
	}

	// Skip abandoned panes.
	if snap.Status == PaneAbandoned {
		return
	}

	// Capture current pane content.
	content, err := h.panes.CapturePaneContent(pane.ID, 50)
	if err != nil {
		log.Printf("pane-healer: capture error for %s: %v", pane.Name, err)
		return
	}

	currentHash := hashPaneContent(content)

	// Check if content changed.
	if currentHash != snap.ContentHash {
		snap.ContentHash = currentHash
		snap.LastChange = time.Now()
		snap.Status = PaneHealthy
		return
	}

	// Content unchanged — check if frozen.
	staleDuration := time.Since(snap.LastChange)
	if staleDuration > h.frozenThr {
		snap.Status = PaneFrozen
		log.Printf("pane-healer: pane %q frozen for %s (threshold %s)",
			pane.Name, staleDuration.Round(time.Second), h.frozenThr)
		h.healPane(pane, snap)
	}
}

// healPane attempts to restart a frozen or stale pane in-place using SendKeys.
// This sends Ctrl-C to kill the existing process, then re-types the original
// command and presses Enter. This preserves the pane's position in the KDL
// layout, unlike the old ClosePane+CreatePane approach which broke layouts.
func (h *PaneHealer) healPane(pane *zellij.Pane, snap *paneSnapshot) {
	if snap.Restarts >= h.maxRestarts {
		log.Printf("pane-healer: pane %q exceeded max restarts (%d), marking abandoned",
			pane.Name, h.maxRestarts)
		snap.Status = PaneAbandoned
		return
	}

	snap.Status = PaneRestarting
	snap.Restarts++

	// Exponential backoff: 3s, 6s, 12s... up to 60s.
	backoff := time.Duration(3<<uint(snap.Restarts-1)) * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}

	log.Printf("pane-healer: restarting pane %q in-place (attempt %d/%d, backoff %s)",
		pane.Name, snap.Restarts, h.maxRestarts, backoff)

	// Send Ctrl-C to kill the existing process.
	if err := h.panes.SendKeys(pane.ID, "\x03"); err != nil {
		log.Printf("pane-healer: send ctrl-c error for %s: %v", pane.Name, err)
	}

	// Wait for backoff.
	time.Sleep(backoff)

	// Re-type the original command in the existing pane shell.
	cmd := snap.Command
	if cmd == "" {
		cmd = pane.Command
	}

	if cmd != "" {
		if err := h.panes.SendKeys(pane.ID, cmd+"\n"); err != nil {
			log.Printf("pane-healer: send command error for %s: %v", pane.Name, err)
			return
		}
	}

	// Reset state for the restarted process.
	snap.ContentHash = ""
	snap.LastChange = time.Now()
	snap.Status = PaneHealthy

	log.Printf("pane-healer: pane %q restarted in-place successfully", pane.Name)
}

// GetStatus returns the health status of all tracked panes.
func (h *PaneHealer) GetStatus() map[string]PaneHealthStatus {
	result := make(map[string]PaneHealthStatus)
	for name, snap := range h.snapshots {
		result[name] = snap.Status
	}
	return result
}

// hashPaneContent returns a truncated SHA-256 hex of the pane content.
func hashPaneContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}

// splitCommand splits a command string into parts for CreatePaneOpts.
func splitCommand(cmd string) []string {
	// Simple split — does not handle quoted arguments.
	var parts []string
	for _, p := range splitFields(cmd) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// splitFields splits on whitespace (like strings.Fields but in this package).
func splitFields(s string) []string {
	var fields []string
	field := ""
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if field != "" {
				fields = append(fields, field)
				field = ""
			}
		} else {
			field += string(r)
		}
	}
	if field != "" {
		fields = append(fields, field)
	}
	return fields
}
