// Package watchdog provides tiered health monitoring and smart nudge capabilities
// for ComputeCommander agent sessions.
package watchdog

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// HealthStatus classifies the overall health of an agent session.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusDegraded HealthStatus = "degraded"
	StatusCritical HealthStatus = "critical"
	StatusDead     HealthStatus = "dead"
)

// Issue represents a single problem found during a health check.
type Issue struct {
	Tier        int
	Code        string
	Description string
	DetectedAt  time.Time
}

// HealthReport summarises the health of a single agent session.
type HealthReport struct {
	Agent           string
	Status          HealthStatus
	Issues          []Issue
	Recommendations []string
	CheckedAt       time.Time
}

// sessionRow holds the columns we read from the sessions table.
type sessionRow struct {
	Agent        string
	State        string
	PID          int
	ZellijPane   string
	LastActivity time.Time
	StalledSince *time.Time
}

// listActiveSessions returns all sessions that are not in a terminal state.
func listActiveSessions(ctx context.Context, database db.DB) ([]sessionRow, error) {
	rows, err := database.Query(ctx,
		"SELECT agent, state, pid, zellij_pane, last_activity, stalled_since FROM sessions WHERE state NOT IN ('completed', 'failed', 'cancelled')")
	if err != nil {
		return nil, fmt.Errorf("list active sessions: %w", err)
	}
	defer rows.Close()

	var sessions []sessionRow
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.Agent, &s.State, &s.PID, &s.ZellijPane, &s.LastActivity, &s.StalledSince); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session rows: %w", err)
	}
	return sessions, nil
}

// --- Tier 0: Mechanical Daemon ------------------------------------------

// tier0Check performs process liveness, Zellij pane existence, and staleness checks.
func tier0Check(s sessionRow, staleThreshold time.Duration) HealthReport {
	report := HealthReport{
		Agent:     s.Agent,
		Status:    StatusHealthy,
		CheckedAt: time.Now(),
	}

	// Process liveness: verify PID exists.
	if s.PID > 0 {
		if !processAlive(s.PID) {
			report.Issues = append(report.Issues, Issue{
				Tier:        0,
				Code:        "PROC_DEAD",
				Description: fmt.Sprintf("pid %d is not running", s.PID),
				DetectedAt:  time.Now(),
			})
			report.Recommendations = append(report.Recommendations, "Respawn agent process in existing worktree")
		}
	}

	// Zellij pane check: a pane name must be set for running agents.
	if s.ZellijPane == "" && s.State == "running" {
		report.Issues = append(report.Issues, Issue{
			Tier:        0,
			Code:        "PANE_MISSING",
			Description: "running agent has no zellij pane assigned",
			DetectedAt:  time.Now(),
		})
		report.Recommendations = append(report.Recommendations, "Re-create Zellij pane for agent")
	}

	// Staleness check: last_activity older than threshold.
	if staleThreshold > 0 {
		staleDuration := time.Since(s.LastActivity)
		if staleDuration > staleThreshold {
			report.Issues = append(report.Issues, Issue{
				Tier:        0,
				Code:        "STALE",
				Description: fmt.Sprintf("no activity for %s (threshold %s)", staleDuration.Round(time.Second), staleThreshold),
				DetectedAt:  time.Now(),
			})
			report.Recommendations = append(report.Recommendations, "Send soft nudge to check agent status")
		}
	}

	// Roll up status.
	switch {
	case len(report.Issues) == 0:
		report.Status = StatusHealthy
	case hasCode(report.Issues, "PROC_DEAD"):
		report.Status = StatusDead
	case len(report.Issues) > 1:
		report.Status = StatusCritical
	default:
		report.Status = StatusDegraded
	}

	return report
}

// processAlive checks whether a process with the given PID exists.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check liveness.
	err = proc.Signal(os.Signal(signalZero))
	return err == nil
}

// signalZero is syscall signal 0 used for process liveness testing.
// Defined as a variable so tests can override it if needed.
var signalZero = syscallSignal(0)

func hasCode(issues []Issue, code string) bool {
	for _, iss := range issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

// --- Tier 1: AI Triage (stub) ------------------------------------------

// Tier1Classifier classifies failures and suggests recovery actions.
// The AI call is stubbed: implementations should provide an LLM-backed version.
type Tier1Classifier interface {
	Classify(ctx context.Context, report HealthReport) (*TriageResult, error)
}

// TriageResult is the output of Tier 1 AI triage.
type TriageResult struct {
	FailureClass string
	Severity     string
	RecoveryHint string
}

// stubClassifier is the default no-op Tier 1 implementation.
type stubClassifier struct{}

func (stubClassifier) Classify(_ context.Context, report HealthReport) (*TriageResult, error) {
	if report.Status == StatusDead {
		return &TriageResult{
			FailureClass: "process_crash",
			Severity:     "high",
			RecoveryHint: "Restart the agent in the same worktree.",
		}, nil
	}
	return &TriageResult{
		FailureClass: "unknown",
		Severity:     "low",
		RecoveryHint: "Monitor and re-evaluate.",
	}, nil
}

// --- Tier 2: Monitor Agent (interface only) -----------------------------

// Tier2Monitor defines the interface for a long-running monitor agent that
// detects patterns and intervenes proactively. Implementation is deferred to
// the monitor agent runtime.
type Tier2Monitor interface {
	// Start begins the continuous patrol loop.
	Start(ctx context.Context) error

	// DetectPatterns analyses recent health reports for recurring issues.
	DetectPatterns(ctx context.Context, reports []HealthReport) ([]PatternAlert, error)

	// Intervene takes a proactive action based on a detected pattern.
	Intervene(ctx context.Context, alert PatternAlert) error
}

// PatternAlert is a warning raised by Tier 2 pattern detection.
type PatternAlert struct {
	PatternName string
	Agents      []string
	Description string
	Severity    string
	DetectedAt  time.Time
}
