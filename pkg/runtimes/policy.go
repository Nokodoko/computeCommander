package runtimes

import "time"

// CompletedTTL returns how long a completed session of the given runtime
// should remain visible in dashboard views after its last_activity timestamp.
//
// Single source of truth for the per-runtime completion display window.
// Consumed by both the KDL pane status view (internal/commands/status.go's
// filterPaneSessions) and the BubbleTea TUI agent table
// (internal/tui/agent_table.go's filterLiveSessions).
//
// Rationale: the pi extension does not heartbeat during idle gaps; honor a
// longer window so idle pi REPLs remain visible after they transition to
// completed. Other runtimes use a shorter default so finished entries do not
// accumulate as stale.
func CompletedTTL(runtime RuntimeID) time.Duration {
	switch runtime {
	case RuntimePi:
		return 30 * time.Minute
	default:
		return 5 * time.Minute
	}
}
