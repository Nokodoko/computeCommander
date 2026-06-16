package agentui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agents"
)

// stubLister implements AgentLister for unit tests so RenderAgents can be
// exercised without a live DB.
type stubLister struct {
	sessions []*agents.AgentSession
	err      error
	colors   map[string]string
}

func (s *stubLister) ListSessions(_ context.Context, _ agents.ListOpts) ([]*agents.AgentSession, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sessions, nil
}

func (s *stubLister) BuildColorResolver(_ context.Context) func(string) string {
	return func(name string) string {
		if s.colors == nil {
			return ""
		}
		return s.colors[name]
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 17, 14, 32, 7, 0, time.UTC)
}

func newSession(name string, state agents.SessionState, taskID string) *agents.AgentSession {
	return &agents.AgentSession{
		ID:           name + "-id",
		AgentName:    name,
		Capability:   agents.CapBuilder,
		State:        state,
		TaskID:       taskID,
		Model:        "claude-opus-4-6",
		LastActivity: time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC),
	}
}

// assertShape asserts the standard contract: exactly Lines lines, each
// <= Width visible cols.
func assertShape(t *testing.T, got []string, opts AgentsOpts) {
	t.Helper()
	if len(got) != opts.Lines {
		t.Errorf("expected %d lines, got %d: %q", opts.Lines, len(got), got)
	}
	for i, ln := range got {
		if v := VisibleLen(ln); v > opts.Width {
			t.Errorf("line %d visible width %d > Width %d: %q", i, v, opts.Width, ln)
		}
	}
}

func TestRenderAgents_Empty(t *testing.T) {
	lister := &stubLister{}
	opts := AgentsOpts{Lines: 8, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderAgents(context.Background(), lister, opts)
	assertShape(t, got, opts)
	if !strings.HasPrefix(got[0], "Agents") {
		t.Errorf("first line should be header, got %q", got[0])
	}
	if !strings.Contains(strings.Join(got, "\n"), "no agents") {
		t.Errorf("empty case should contain 'no agents' marker:\n%s", strings.Join(got, "\n"))
	}
	if got[len(got)-1] != "updated 14:32:07" {
		t.Errorf("last line should be trailer 'updated 14:32:07', got %q", got[len(got)-1])
	}
}

func TestRenderAgents_ThreeAgentsWide(t *testing.T) {
	lister := &stubLister{
		sessions: []*agents.AgentSession{
			newSession("alpha", agents.StateWorking, "task-1"),
			newSession("beta", agents.StateBooting, "task-2"),
			newSession("gamma", agents.StateCompleted, "task-3"),
		},
	}
	opts := AgentsOpts{Lines: 8, Width: 80, NoColor: true, Now: fixedNow()}
	got := RenderAgents(context.Background(), lister, opts)
	assertShape(t, got, opts)

	body := strings.Join(got, "\n")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(body, name) {
			t.Errorf("expected agent %q in body:\n%s", name, body)
		}
	}
	// Header reports active count = working+booting = 2.
	if !strings.Contains(got[0], "2 active") {
		t.Errorf("header should say '2 active', got %q", got[0])
	}
	// No ANSI bytes when NoColor.
	if strings.Contains(body, "\033") {
		t.Errorf("NoColor output must not contain ANSI escapes:\n%s", body)
	}
}

func TestRenderAgents_ThreeAgentsNarrow(t *testing.T) {
	lister := &stubLister{
		sessions: []*agents.AgentSession{
			newSession("alpha", agents.StateWorking, "task-1"),
			newSession("beta", agents.StateBooting, "task-2"),
		},
	}
	opts := AgentsOpts{Lines: 6, Width: 44, NoColor: true, Now: fixedNow()}
	got := RenderAgents(context.Background(), lister, opts)
	assertShape(t, got, opts)
	body := strings.Join(got, "\n")
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "beta") {
		t.Errorf("compact output missing agent names:\n%s", body)
	}
}

func TestRenderAgents_DBError(t *testing.T) {
	lister := &stubLister{err: errors.New("db down")}
	opts := AgentsOpts{Lines: 5, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderAgents(context.Background(), lister, opts)
	assertShape(t, got, opts)
	if got[0] != "agents: unavailable" {
		t.Errorf("DB error case must produce 'agents: unavailable', got %q", got[0])
	}
}

func TestRenderAgents_NilLister(t *testing.T) {
	opts := AgentsOpts{Lines: 3, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderAgents(context.Background(), nil, opts)
	assertShape(t, got, opts)
	if got[0] != "agents: unavailable" {
		t.Errorf("nil lister must produce 'agents: unavailable', got %q", got[0])
	}
}

func TestRenderAgents_ZeroLines(t *testing.T) {
	lister := &stubLister{}
	got := RenderAgents(context.Background(), lister, AgentsOpts{Lines: 0, Width: 60})
	if got != nil {
		t.Errorf("Lines=0 must return nil, got %v", got)
	}
}

func TestRenderAgents_ZeroWidth(t *testing.T) {
	lister := &stubLister{}
	opts := AgentsOpts{Lines: 4, Width: 0}
	got := RenderAgents(context.Background(), lister, opts)
	if len(got) != 4 {
		t.Fatalf("Lines=4 Width=0 must return 4 lines, got %d", len(got))
	}
	if got[0] != "agents: unavailable" {
		t.Errorf("Width=0 must produce degraded marker, got %q", got[0])
	}
}

func TestRenderAgents_ColorPath(t *testing.T) {
	lister := &stubLister{
		sessions: []*agents.AgentSession{newSession("alpha", agents.StateWorking, "task-1")},
		colors:   map[string]string{"alpha": "#FF6B6B"},
	}
	// Color path: NoColor=false, no NO_COLOR env.
	t.Setenv("NO_COLOR", "")
	opts := AgentsOpts{Lines: 4, Width: 80, NoColor: false, Now: fixedNow()}
	got := RenderAgents(context.Background(), lister, opts)
	body := strings.Join(got, "\n")
	if !strings.Contains(body, "\033[38;2;255;107;107m") {
		t.Errorf("expected 24-bit SGR for #FF6B6B in colored output:\n%s", body)
	}
}

func TestRenderAgents_Deterministic(t *testing.T) {
	lister := &stubLister{
		sessions: []*agents.AgentSession{
			newSession("alpha", agents.StateWorking, "task-1"),
			newSession("beta", agents.StateWorking, "task-2"),
		},
	}
	opts := AgentsOpts{Lines: 6, Width: 80, NoColor: true, Now: fixedNow()}
	a := RenderAgents(context.Background(), lister, opts)
	b := RenderAgents(context.Background(), lister, opts)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("RenderAgents must be deterministic with injected Now:\n%v\n---\n%v", a, b)
	}
}
