package watchdog

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/internal/zellij"
)

// --- Fakes ---------------------------------------------------------------

// fakeDB implements db.DB for testing.
type fakeDB struct {
	sessions []sessionRow
	queryErr error
}

func (f *fakeDB) Exec(_ context.Context, _ string, _ ...any) error { return nil }
func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (*db.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return nil, nil
}
func (f *fakeDB) QueryRow(_ context.Context, _ string, _ ...any) *db.Row { return nil }
func (f *fakeDB) Close() error                                           { return nil }
func (f *fakeDB) Begin(_ context.Context) (db.Tx, error)                 { return nil, nil }
func (f *fakeDB) Driver() string                                         { return "fake" }

// fakeMailStore implements mail.MailStore for testing.
type fakeMailStore struct {
	sent []*mail.MailMessage
}

func (f *fakeMailStore) Send(msg *mail.MailMessage) error {
	f.sent = append(f.sent, msg)
	return nil
}
func (f *fakeMailStore) Check(_ string, _ mail.CheckOpts) ([]*mail.MailMessage, error) {
	return nil, nil
}
func (f *fakeMailStore) List(_ mail.ListOpts) ([]*mail.MailMessage, error) { return nil, nil }
func (f *fakeMailStore) MarkRead(_ string) error                          { return nil }
func (f *fakeMailStore) Reply(_ string, _ string) error                   { return nil }
func (f *fakeMailStore) Purge(_ mail.PurgeOpts) (int, error)              { return 0, nil }

// fakePaneManager implements zellij.PaneManager for testing.
type fakePaneManager struct {
	keysSent    map[string]string
	closedPanes []string
}

func newFakePaneManager() *fakePaneManager {
	return &fakePaneManager{keysSent: make(map[string]string)}
}

func (f *fakePaneManager) CreatePane(_ zellij.CreatePaneOpts) (*zellij.Pane, error) {
	return &zellij.Pane{}, nil
}
func (f *fakePaneManager) ListPanes() ([]*zellij.Pane, error) { return nil, nil }
func (f *fakePaneManager) SendKeys(paneID string, keys string) error {
	f.keysSent[paneID] = keys
	return nil
}
func (f *fakePaneManager) CapturePaneContent(_ string, _ int) (string, error) { return "", nil }
func (f *fakePaneManager) ClosePane(paneID string) error {
	f.closedPanes = append(f.closedPanes, paneID)
	return nil
}

// Compile-time interface assertions.
var _ db.DB = (*fakeDB)(nil)
var _ mail.MailStore = (*fakeMailStore)(nil)
var _ zellij.PaneManager = (*fakePaneManager)(nil)

// --- Tests ---------------------------------------------------------------

func TestTier0Check_Healthy(t *testing.T) {
	t.Log("Given a session with a living process and recent activity")

	s := sessionRow{
		Agent:        "builder-1",
		State:        "running",
		PID:          os.Getpid(), // current process is always alive
		ZellijPane:   "pane-builder-1",
		LastActivity: time.Now(),
	}

	report := tier0Check(s, 5*time.Minute)

	if report.Status != StatusHealthy {
		t.Errorf("\tExpected status %s, got %s", StatusHealthy, report.Status)
	}
	if len(report.Issues) != 0 {
		t.Errorf("\tExpected 0 issues, got %d: %v", len(report.Issues), report.Issues)
	}
}

func TestTier0Check_Stale(t *testing.T) {
	t.Log("Given a session with activity older than the stale threshold")

	s := sessionRow{
		Agent:        "scout-1",
		State:        "running",
		PID:          os.Getpid(),
		ZellijPane:   "pane-scout-1",
		LastActivity: time.Now().Add(-15 * time.Minute),
	}

	report := tier0Check(s, 10*time.Minute)

	if report.Status == StatusHealthy {
		t.Errorf("\tExpected unhealthy status, got %s", report.Status)
	}
	if !hasCode(report.Issues, "STALE") {
		t.Errorf("\tExpected STALE issue, got %v", report.Issues)
	}
}

func TestTier0Check_MissingPane(t *testing.T) {
	t.Log("Given a running session with no Zellij pane")

	s := sessionRow{
		Agent:        "builder-2",
		State:        "running",
		PID:          os.Getpid(),
		ZellijPane:   "",
		LastActivity: time.Now(),
	}

	report := tier0Check(s, 5*time.Minute)

	if !hasCode(report.Issues, "PANE_MISSING") {
		t.Errorf("\tExpected PANE_MISSING issue, got %v", report.Issues)
	}
}

func TestTier0Check_DeadProcess(t *testing.T) {
	t.Log("Given a session whose PID does not exist")

	s := sessionRow{
		Agent:        "builder-3",
		State:        "running",
		PID:          999999999,
		ZellijPane:   "pane-builder-3",
		LastActivity: time.Now(),
	}

	report := tier0Check(s, 5*time.Minute)

	if report.Status != StatusDead {
		t.Errorf("\tExpected status %s, got %s", StatusDead, report.Status)
	}
	if !hasCode(report.Issues, "PROC_DEAD") {
		t.Errorf("\tExpected PROC_DEAD issue, got %v", report.Issues)
	}
}

func TestStubClassifier(t *testing.T) {
	t.Log("Given the stub classifier and a dead agent report")

	c := stubClassifier{}
	report := HealthReport{
		Agent:  "test-agent",
		Status: StatusDead,
		Issues: []Issue{{Code: "PROC_DEAD"}},
	}

	result, err := c.Classify(context.Background(), report)
	if err != nil {
		t.Fatalf("\tClassify error: %v", err)
	}
	if result.FailureClass != "process_crash" {
		t.Errorf("\tExpected failure class 'process_crash', got %q", result.FailureClass)
	}
}

func TestNudgeDecision_SoftTimeout(t *testing.T) {
	t.Log("Given an agent past the soft timeout but within hard timeout")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{
			SoftTimeout: "5m",
			HardTimeout: "30m",
			LoopDetection: config.LoopDetection{
				Enabled:   false,
				Window:    "5m",
				Threshold: 3,
			},
		},
		config.WatchdogConfig{},
		panes,
	)

	session := sessionRow{
		Agent:        "builder-4",
		LastActivity: time.Now().Add(-10 * time.Minute),
	}
	report := HealthReport{
		Agent:  "builder-4",
		Status: StatusDegraded,
		Issues: []Issue{{Code: "STALE", Description: "no activity for 10m"}},
	}

	decision, err := nudger.EvaluateNudge(session, report)
	if err != nil {
		t.Fatalf("\tEvaluateNudge error: %v", err)
	}
	if !decision.ShouldNudge {
		t.Fatalf("\tExpected ShouldNudge=true")
	}
	if decision.NudgeType != NudgeSoft {
		t.Errorf("\tExpected NudgeType=%s, got %s", NudgeSoft, decision.NudgeType)
	}
}

func TestNudgeDecision_HardTimeout(t *testing.T) {
	t.Log("Given an agent past the hard timeout")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{
			SoftTimeout: "5m",
			HardTimeout: "15m",
			LoopDetection: config.LoopDetection{
				Enabled: false,
			},
		},
		config.WatchdogConfig{},
		panes,
	)

	session := sessionRow{
		Agent:        "builder-5",
		LastActivity: time.Now().Add(-20 * time.Minute),
	}
	report := HealthReport{
		Agent:  "builder-5",
		Status: StatusDegraded,
		Issues: []Issue{{Code: "STALE", Description: "no activity for 20m"}},
	}

	decision, err := nudger.EvaluateNudge(session, report)
	if err != nil {
		t.Fatalf("\tEvaluateNudge error: %v", err)
	}
	if !decision.ShouldNudge {
		t.Fatalf("\tExpected ShouldNudge=true")
	}
	if decision.NudgeType != NudgeHard {
		t.Errorf("\tExpected NudgeType=%s, got %s", NudgeHard, decision.NudgeType)
	}
}

func TestNudgeDecision_DeadProcess(t *testing.T) {
	t.Log("Given a dead agent always gets hard nudge")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{
			SoftTimeout: "5m",
			HardTimeout: "30m",
			LoopDetection: config.LoopDetection{
				Enabled: false,
			},
		},
		config.WatchdogConfig{},
		panes,
	)

	session := sessionRow{
		Agent:        "builder-6",
		LastActivity: time.Now(),
	}
	report := HealthReport{
		Agent:  "builder-6",
		Status: StatusDead,
		Issues: []Issue{{Code: "PROC_DEAD"}},
	}

	decision, err := nudger.EvaluateNudge(session, report)
	if err != nil {
		t.Fatalf("\tEvaluateNudge error: %v", err)
	}
	if !decision.ShouldNudge {
		t.Fatalf("\tExpected ShouldNudge=true")
	}
	if decision.NudgeType != NudgeHard {
		t.Errorf("\tExpected NudgeType=%s, got %s", NudgeHard, decision.NudgeType)
	}
}

func TestNudgeDecision_NoNudgeNeeded(t *testing.T) {
	t.Log("Given a healthy agent within timeout")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{
			SoftTimeout: "10m",
			HardTimeout: "30m",
			LoopDetection: config.LoopDetection{
				Enabled: false,
			},
		},
		config.WatchdogConfig{},
		panes,
	)

	session := sessionRow{
		Agent:        "builder-7",
		LastActivity: time.Now().Add(-2 * time.Minute),
	}
	report := HealthReport{
		Agent:  "builder-7",
		Status: StatusDegraded,
		Issues: []Issue{{Code: "PANE_MISSING"}},
	}

	decision, err := nudger.EvaluateNudge(session, report)
	if err != nil {
		t.Fatalf("\tEvaluateNudge error: %v", err)
	}
	if decision.ShouldNudge {
		t.Errorf("\tExpected ShouldNudge=false, got true")
	}
}

func TestExecuteSoftNudge(t *testing.T) {
	t.Log("Given a soft nudge decision, keys are sent to the pane")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{SoftTimeout: "5m", HardTimeout: "30m"},
		config.WatchdogConfig{},
		panes,
	)

	decision := &NudgeDecision{
		Agent:       "builder-8",
		ShouldNudge: true,
		NudgeType:   NudgeSoft,
		Reason:      "stale",
		TimeOnTask:  10 * time.Minute,
	}

	if err := nudger.ExecuteNudge(decision); err != nil {
		t.Fatalf("\tExecuteNudge error: %v", err)
	}

	if _, ok := panes.keysSent["builder-8"]; !ok {
		t.Errorf("\tExpected keys sent to pane 'builder-8'")
	}
}

func TestExecuteHardNudge(t *testing.T) {
	t.Log("Given a hard nudge decision, the pane is closed")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{SoftTimeout: "5m", HardTimeout: "30m"},
		config.WatchdogConfig{},
		panes,
	)

	decision := &NudgeDecision{
		Agent:       "builder-9",
		ShouldNudge: true,
		NudgeType:   NudgeHard,
		Reason:      "dead process",
		TimeOnTask:  35 * time.Minute,
	}

	if err := nudger.ExecuteNudge(decision); err != nil {
		t.Fatalf("\tExecuteNudge error: %v", err)
	}

	if len(panes.closedPanes) != 1 || panes.closedPanes[0] != "builder-9" {
		t.Errorf("\tExpected pane 'builder-9' closed, got %v", panes.closedPanes)
	}
}

func TestExecuteNudge_Nil(t *testing.T) {
	t.Log("Given a nil decision, no error")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{SoftTimeout: "5m", HardTimeout: "30m"},
		config.WatchdogConfig{},
		panes,
	)

	if err := nudger.ExecuteNudge(nil); err != nil {
		t.Errorf("\tExpected nil error for nil decision, got %v", err)
	}
}

func TestExecuteNudge_NoNudge(t *testing.T) {
	t.Log("Given a decision with ShouldNudge=false, no action")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{SoftTimeout: "5m", HardTimeout: "30m"},
		config.WatchdogConfig{},
		panes,
	)

	decision := &NudgeDecision{
		Agent:       "builder-10",
		ShouldNudge: false,
	}

	if err := nudger.ExecuteNudge(decision); err != nil {
		t.Errorf("\tExpected nil error for no-nudge decision, got %v", err)
	}
	if len(panes.keysSent) > 0 || len(panes.closedPanes) > 0 {
		t.Errorf("\tExpected no actions taken")
	}
}

func TestNewWatchdog(t *testing.T) {
	t.Log("Given valid options, NewWatchdog returns a non-nil Watchdog")

	fdb := &fakeDB{}
	fms := &fakeMailStore{}
	fp := newFakePaneManager()

	w := NewWatchdog(WatchdogOpts{
		DB:          fdb,
		MailStore:   fms,
		PaneManager: fp,
		WatchdogCfg: config.WatchdogConfig{
			Tier0Enabled:     true,
			Tier0IntervalMs:  1000,
			StaleThresholdMs: 300000,
		},
		NudgeCfg: config.NudgeConfig{
			SoftTimeout: "10m",
			HardTimeout: "30m",
		},
	})

	if w == nil {
		t.Fatal("\tExpected non-nil Watchdog")
	}
	if w.db != fdb {
		t.Error("\tExpected db to be set")
	}
	if w.mailStore != fms {
		t.Error("\tExpected mailStore to be set")
	}
}

func TestHealthPriority(t *testing.T) {
	t.Log("Given different health statuses, priority maps correctly")

	tests := []struct {
		status   HealthStatus
		expected mail.Priority
	}{
		{StatusDead, mail.PriorityUrgent},
		{StatusCritical, mail.PriorityUrgent},
		{StatusDegraded, mail.PriorityHigh},
		{StatusHealthy, mail.PriorityNormal},
	}

	for _, tt := range tests {
		got := healthPriority(tt.status)
		if got != tt.expected {
			t.Errorf("\tStatus %s: expected priority %s, got %s", tt.status, tt.expected, got)
		}
	}
}

func TestSummariseIssues(t *testing.T) {
	t.Log("Given issues, summariseIssues returns a semicolon-delimited string")

	issues := []Issue{
		{Code: "PROC_DEAD", Description: "pid 123 not running"},
		{Code: "STALE", Description: "no activity for 10m"},
	}

	summary := summariseIssues(issues)
	expected := "[PROC_DEAD] pid 123 not running; [STALE] no activity for 10m"
	if summary != expected {
		t.Errorf("\tExpected %q, got %q", expected, summary)
	}
}

func TestSummariseIssues_Empty(t *testing.T) {
	t.Log("Given no issues, summariseIssues returns fallback")

	summary := summariseIssues(nil)
	if summary != "no issues detected" {
		t.Errorf("\tExpected 'no issues detected', got %q", summary)
	}
}

func TestHasCode(t *testing.T) {
	t.Log("Given a slice of issues, hasCode finds matches")

	issues := []Issue{
		{Code: "PROC_DEAD"},
		{Code: "STALE"},
	}

	if !hasCode(issues, "PROC_DEAD") {
		t.Error("\tExpected hasCode to find PROC_DEAD")
	}
	if !hasCode(issues, "STALE") {
		t.Error("\tExpected hasCode to find STALE")
	}
	if hasCode(issues, "PANE_MISSING") {
		t.Error("\tExpected hasCode NOT to find PANE_MISSING")
	}
}

func TestNudgeLoopDetection(t *testing.T) {
	t.Log("Given loop detection enabled and stalled_since past the window")

	panes := newFakePaneManager()
	nudger := NewNudger(
		config.NudgeConfig{
			SoftTimeout: "10m",
			HardTimeout: "60m",
			LoopDetection: config.LoopDetection{
				Enabled:   true,
				Window:    "5m",
				Threshold: 3,
			},
		},
		config.WatchdogConfig{},
		panes,
	)

	stalledSince := time.Now().Add(-10 * time.Minute)
	session := sessionRow{
		Agent:        "builder-loop",
		LastActivity: time.Now().Add(-3 * time.Minute),
		StalledSince: &stalledSince,
	}
	report := HealthReport{
		Agent:  "builder-loop",
		Status: StatusDegraded,
		Issues: []Issue{{Code: "STALE"}},
	}

	decision, err := nudger.EvaluateNudge(session, report)
	if err != nil {
		t.Fatalf("\tEvaluateNudge error: %v", err)
	}
	if !decision.ShouldNudge {
		t.Fatalf("\tExpected ShouldNudge=true for loop detection")
	}
	if decision.NudgeType != NudgeHard {
		t.Errorf("\tExpected NudgeType=%s for loop, got %s", NudgeHard, decision.NudgeType)
	}
}
