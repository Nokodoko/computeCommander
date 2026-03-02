# internal/watchdog/ -- Agent Health Monitoring Daemon

## Purpose
Implements the 3-tier watchdog monitoring system for agent health. Tier 0 performs mechanical checks (process liveness, pane existence, staleness). Tier 1 provides AI-based triage (stub). Tier 2 interfaces with monitor agents. Includes the nudge system for soft/hard intervention on stalled agents.

## Technology
- Go 1.25
- `os` for process signaling (`syscall.Signal`)
- Depends on: `internal/agents`, `internal/mail`, `internal/zellij`, `internal/platform/db`

## Contents
| File | Description |
|------|-------------|
| `watchdog.go` | `Watchdog` struct, `NewWatchdog()`, `Run()` loop with configurable interval, `CheckAll()`, `sendHealthMail()` |
| `health.go` | Tier 0 checks: `checkProcess()` (PID liveness), `checkPane()` (zellij pane existence), `checkStaleness()` (activity timeout). `Tier1Classifier` interface (stub), `Tier2Monitor` interface. `HealthReport`, `Issue`, `PatternAlert` types |
| `nudge.go` | `Nudger` struct: `Nudge()` decides soft vs hard. Soft = send keys to pane via zellij. Hard = close pane (kill process). Escalation level tracking |
| `signal.go` | `syscallSignal()` helper for cross-platform process liveness check via `syscall.Signal(pid, 0)` |
| `watchdog_test.go` | Tests for health checks, nudge decisions, escalation levels, and watchdog loop |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewWatchdog` | `func NewWatchdog(opts WatchdogOpts) *Watchdog` | `*Watchdog` | Creates watchdog with spawner, mail, pane manager, config |
| `Run` | `func (w *Watchdog) Run(ctx context.Context) error` | `error` | Main loop: periodically calls CheckAll, respects context cancellation |
| `CheckAll` | `func (w *Watchdog) CheckAll(ctx context.Context) (*HealthReport, error)` | `*HealthReport, error` | Runs all tier 0 checks on active sessions, returns report with issues |
| `checkProcess` | `func (w *Watchdog) checkProcess(session) *Issue` | `*Issue` | Checks PID liveness via syscall signal 0 |
| `checkPane` | `func (w *Watchdog) checkPane(session) *Issue` | `*Issue` | Verifies zellij pane still exists |
| `checkStaleness` | `func (w *Watchdog) checkStaleness(session) *Issue` | `*Issue` | Detects stale (>threshold) and zombie (>zombie threshold) sessions |
| `Nudge` | `func (n *Nudger) Nudge(session) error` | `error` | Soft nudge (send keys) or hard nudge (close pane) based on escalation |
| `sendHealthMail` | `func (w *Watchdog) sendHealthMail(report) error` | `error` | Sends health_check mail to coordinator with issue summary |

## Data Types

### WatchdogOpts (struct)
Fields: Spawner, Mail, PaneManager, Config (WatchdogConfig), Nudger

### HealthReport (struct)
Fields: Timestamp, Issues ([]Issue), Healthy (int), Total (int)

### Issue (struct)
Fields: AgentName, SessionID, Type (string: "process_dead", "pane_missing", "stale", "zombie"), Severity, Message

### Tier1Classifier (interface)
`Classify(issues []Issue) ([]PatternAlert, error)` -- stub for AI-based triage

### Tier2Monitor (interface)
`HandleAlerts(alerts []PatternAlert) error` -- stub for monitor agent interface

### Nudger (struct)
Fields: panes (PaneManager), softTimeout, hardTimeout

## Logging
- No structured logging; health reports sent via mail system
- Errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `sendHealthMail()` creates health_check messages
- **Read**: `CheckAll()` reads session list and checks health
- **Update**: Nudger updates session state on hard nudge (transitions to zombie/completed)
- **Delete**: Hard nudge closes pane (effectively terminates agent)

## Style Guide
- Configurable intervals via `WatchdogConfig` (tier0_interval_ms, stale_threshold_ms, zombie_threshold_ms)
- Issue types as string constants, not a formal enum
- Soft nudge sends literal keystroke text to zellij pane
- Hard nudge closes pane via PaneManager.ClosePane()
- Context-aware loop with `time.NewTicker`

**Representative snippet (from `watchdog.go`):**
```go
func (w *Watchdog) Run(ctx context.Context) error {
	interval := time.Duration(w.config.Tier0IntervalMs) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			report, err := w.CheckAll(ctx)
			if err != nil {
				continue
			}
			if len(report.Issues) > 0 {
				_ = w.sendHealthMail(report)
			}
		}
	}
}
```
