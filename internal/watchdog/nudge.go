package watchdog

import (
	"fmt"
	"log"
	"time"

	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/zellij"
)

// NudgeType classifies the severity of a nudge.
type NudgeType string

const (
	NudgeSoft NudgeType = "soft"
	NudgeHard NudgeType = "hard"
)

// NudgeDecision captures whether and how to nudge an agent.
type NudgeDecision struct {
	Agent           string
	ShouldNudge     bool
	NudgeType       NudgeType
	Reason          string
	ContextSummary  string
	TimeOnTask      time.Duration
	EstimatedEffort time.Duration
}

// Nudger evaluates and executes nudge decisions.
type Nudger struct {
	cfg         config.NudgeConfig
	watchdogCfg config.WatchdogConfig
	panes       zellij.PaneManager
}

// NewNudger creates a Nudger with the given configuration and pane manager.
func NewNudger(nudgeCfg config.NudgeConfig, watchdogCfg config.WatchdogConfig, panes zellij.PaneManager) *Nudger {
	return &Nudger{
		cfg:         nudgeCfg,
		watchdogCfg: watchdogCfg,
		panes:       panes,
	}
}

// EvaluateNudge decides whether and how to nudge an agent based on its session row
// and health report.
func (n *Nudger) EvaluateNudge(session sessionRow, report HealthReport) (*NudgeDecision, error) {
	decision := &NudgeDecision{
		Agent:       session.Agent,
		ShouldNudge: false,
		TimeOnTask:  time.Since(session.LastActivity),
	}

	softTimeout, err := time.ParseDuration(n.cfg.SoftTimeout)
	if err != nil {
		softTimeout = 10 * time.Minute
	}
	hardTimeout, err := time.ParseDuration(n.cfg.HardTimeout)
	if err != nil {
		hardTimeout = 30 * time.Minute
	}

	staleDuration := time.Since(session.LastActivity)

	// If the process is dead, always hard nudge.
	if report.Status == StatusDead {
		decision.ShouldNudge = true
		decision.NudgeType = NudgeHard
		decision.Reason = "agent process is dead"
		decision.ContextSummary = summariseIssues(report.Issues)
		return decision, nil
	}

	// Check loop detection if enabled.
	if n.cfg.LoopDetection.Enabled && session.StalledSince != nil {
		stalledDuration := time.Since(*session.StalledSince)
		loopWindow, err := time.ParseDuration(n.cfg.LoopDetection.Window)
		if err != nil {
			loopWindow = 5 * time.Minute
		}
		if stalledDuration > loopWindow {
			decision.ShouldNudge = true
			decision.NudgeType = NudgeHard
			decision.Reason = fmt.Sprintf("agent stalled for %s (loop detection window %s)", stalledDuration.Round(time.Second), loopWindow)
			decision.ContextSummary = summariseIssues(report.Issues)
			return decision, nil
		}
	}

	// Hard nudge: past hard timeout.
	if staleDuration > hardTimeout {
		decision.ShouldNudge = true
		decision.NudgeType = NudgeHard
		decision.Reason = fmt.Sprintf("no activity for %s exceeds hard timeout %s", staleDuration.Round(time.Second), hardTimeout)
		decision.ContextSummary = summariseIssues(report.Issues)
		return decision, nil
	}

	// Soft nudge: past soft timeout but within hard timeout.
	if staleDuration > softTimeout {
		decision.ShouldNudge = true
		decision.NudgeType = NudgeSoft
		decision.Reason = fmt.Sprintf("no activity for %s exceeds soft timeout %s", staleDuration.Round(time.Second), softTimeout)
		decision.ContextSummary = summariseIssues(report.Issues)
		return decision, nil
	}

	return decision, nil
}

// ExecuteNudge carries out a nudge decision.
func (n *Nudger) ExecuteNudge(decision *NudgeDecision) error {
	if decision == nil || !decision.ShouldNudge {
		return nil
	}

	switch decision.NudgeType {
	case NudgeSoft:
		return n.executeSoftNudge(decision)
	case NudgeHard:
		return n.executeHardNudge(decision)
	default:
		return fmt.Errorf("unknown nudge type: %s", decision.NudgeType)
	}
}

// executeSoftNudge sends a status request to the agent's Zellij pane without
// interrupting the process.
func (n *Nudger) executeSoftNudge(decision *NudgeDecision) error {
	msg := fmt.Sprintf(
		"\n[WATCHDOG] Status check: agent %s has been inactive for %s. Reason: %s. Please report status.\n",
		decision.Agent, decision.TimeOnTask.Round(time.Second), decision.Reason,
	)
	if err := n.panes.SendKeys(decision.Agent, msg); err != nil {
		return fmt.Errorf("soft nudge send keys: %w", err)
	}
	log.Printf("watchdog: soft nudge sent to %s: %s", decision.Agent, decision.Reason)
	return nil
}

// executeHardNudge terminates the agent process and logs that a restart is needed.
// Actual respawn is delegated to the orchestrator.
func (n *Nudger) executeHardNudge(decision *NudgeDecision) error {
	log.Printf("watchdog: hard nudge for %s: terminating process. Reason: %s", decision.Agent, decision.Reason)
	// Close the Zellij pane which kills the process inside it.
	if err := n.panes.ClosePane(decision.Agent); err != nil {
		log.Printf("watchdog: failed to close pane for %s: %v", decision.Agent, err)
		// Continue: pane may already be gone if process is dead.
	}
	log.Printf("watchdog: agent %s requires restart (hard nudge executed)", decision.Agent)
	return nil
}

func summariseIssues(issues []Issue) string {
	if len(issues) == 0 {
		return "no issues detected"
	}
	summary := ""
	for i, iss := range issues {
		if i > 0 {
			summary += "; "
		}
		summary += fmt.Sprintf("[%s] %s", iss.Code, iss.Description)
	}
	return summary
}
