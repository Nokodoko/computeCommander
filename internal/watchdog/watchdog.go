package watchdog

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/internal/zellij"
)

// WatchdogOpts groups the dependencies needed to construct a Watchdog.
type WatchdogOpts struct {
	DB          db.DB
	MailStore   mail.MailStore
	PaneManager zellij.PaneManager
	WatchdogCfg config.WatchdogConfig
	NudgeCfg    config.NudgeConfig
}

// Watchdog is the main daemon that periodically checks agent health and nudges
// stalled or dead agents.
type Watchdog struct {
	db          db.DB
	mailStore   mail.MailStore
	panes       zellij.PaneManager
	nudger      *Nudger
	classifier  Tier1Classifier
	cfg         config.WatchdogConfig
	nudgeCfg    config.NudgeConfig
}

// NewWatchdog creates a Watchdog with the given options.
func NewWatchdog(opts WatchdogOpts) *Watchdog {
	return &Watchdog{
		db:         opts.DB,
		mailStore:  opts.MailStore,
		panes:      opts.PaneManager,
		nudger:     NewNudger(opts.NudgeCfg, opts.WatchdogCfg, opts.PaneManager),
		classifier: stubClassifier{},
		cfg:        opts.WatchdogCfg,
		nudgeCfg:   opts.NudgeCfg,
	}
}

// SetClassifier replaces the default stub Tier 1 classifier.
func (w *Watchdog) SetClassifier(c Tier1Classifier) {
	w.classifier = c
}

// Run starts the watchdog main loop. It ticks at the configured interval and
// runs CheckAll on each tick. It blocks until the context is cancelled.
func (w *Watchdog) Run(ctx context.Context) error {
	interval := time.Duration(w.cfg.Tier0IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}

	log.Printf("watchdog: starting with interval %s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run an immediate first check.
	if err := w.tick(ctx); err != nil {
		log.Printf("watchdog: initial check error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("watchdog: shutting down")
			return ctx.Err()
		case <-ticker.C:
			if err := w.tick(ctx); err != nil {
				log.Printf("watchdog: tick error: %v", err)
			}
		}
	}
}

// tick performs one full cycle: check all agents, triage, nudge.
func (w *Watchdog) tick(ctx context.Context) error {
	reports, err := w.CheckAll(ctx)
	if err != nil {
		return err
	}

	for i := range reports {
		report := reports[i]
		if report.Status == StatusHealthy {
			continue
		}

		// Tier 1: classify if enabled.
		if w.cfg.Tier1Enabled && w.classifier != nil {
			triage, err := w.classifier.Classify(ctx, report)
			if err != nil {
				log.Printf("watchdog: tier1 classify error for %s: %v", report.Agent, err)
			} else {
				log.Printf("watchdog: tier1 %s: class=%s severity=%s hint=%s",
					report.Agent, triage.FailureClass, triage.Severity, triage.RecoveryHint)
			}
		}

		// Send health_check mail for degraded/critical/dead agents.
		if err := w.sendHealthMail(report); err != nil {
			log.Printf("watchdog: mail error for %s: %v", report.Agent, err)
		}
	}

	return nil
}

// CheckAll inspects every active session and returns a health report for each.
func (w *Watchdog) CheckAll(ctx context.Context) ([]HealthReport, error) {
	sessions, err := listActiveSessions(ctx, w.db)
	if err != nil {
		return nil, fmt.Errorf("check all: %w", err)
	}

	staleThreshold := time.Duration(w.cfg.StaleThresholdMs) * time.Millisecond

	var reports []HealthReport
	for _, s := range sessions {
		report := tier0Check(s, staleThreshold)
		reports = append(reports, report)

		// Evaluate nudge for unhealthy agents.
		if report.Status != StatusHealthy {
			decision, err := w.nudger.EvaluateNudge(s, report)
			if err != nil {
				log.Printf("watchdog: nudge evaluate error for %s: %v", s.Agent, err)
				continue
			}
			if decision.ShouldNudge {
				if err := w.nudger.ExecuteNudge(decision); err != nil {
					log.Printf("watchdog: nudge execute error for %s: %v", s.Agent, err)
				}
			}
		}
	}

	return reports, nil
}

// sendHealthMail sends a health_check message through the mail system.
func (w *Watchdog) sendHealthMail(report HealthReport) error {
	body := fmt.Sprintf("Agent %s health: %s\nIssues:\n", report.Agent, report.Status)
	for _, iss := range report.Issues {
		body += fmt.Sprintf("  - [Tier%d/%s] %s\n", iss.Tier, iss.Code, iss.Description)
	}
	if len(report.Recommendations) > 0 {
		body += "Recommendations:\n"
		for _, rec := range report.Recommendations {
			body += fmt.Sprintf("  - %s\n", rec)
		}
	}

	msg := &mail.MailMessage{
		From:     "watchdog",
		To:       report.Agent,
		Subject:  fmt.Sprintf("Health check: %s (%s)", report.Agent, report.Status),
		Body:     body,
		Priority: healthPriority(report.Status),
		Type:     mail.TypeHealthCheck,
	}

	return w.mailStore.Send(msg)
}

func healthPriority(status HealthStatus) mail.Priority {
	switch status {
	case StatusDead, StatusCritical:
		return mail.PriorityUrgent
	case StatusDegraded:
		return mail.PriorityHigh
	default:
		return mail.PriorityNormal
	}
}
