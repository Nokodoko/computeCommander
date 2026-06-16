package agentui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agents"
)

// TestInvariants_AllRenderers cross-exercises every combination of
// Width / Lines / NoColor against the agents and evals renderers (the
// git renderer is tested separately under internal/agentui/git since it
// needs a tmp repo per case).
//
// Invariants asserted for EVERY combo:
//   - Output length is exactly Lines (or nil if Lines <= 0).
//   - Every line is <= Width visible cols (when Width > 0).
//   - NoColor output contains zero ANSI escape bytes.
func TestInvariants_AllRenderers(t *testing.T) {
	widths := []int{20, 30, 44, 60, 80, 120}
	lineCounts := []int{1, 2, 3, 6, 12}
	noColors := []bool{true, false}

	now := time.Date(2026, 5, 17, 14, 32, 7, 0, time.UTC)

	agentsLister := &stubLister{
		sessions: []*agents.AgentSession{
			newSession("alpha", agents.StateWorking, "task-1"),
			newSession("beta", agents.StateBooting, "task-2"),
			newSession("gamma", agents.StateCompleted, "task-3"),
			newSession("delta", agents.StateStalled, "task-4"),
		},
		colors: map[string]string{"alpha": "#FF6B6B", "beta": "#4ECDC4"},
	}

	evalSrc := &stubEvalSource{
		evals: []EvalRecord{
			{ID: "eval-a", AgentTask: "t1", EvalType: "unit_test", Passed: bp(true), LastRunAt: "2026-05-17T14:32:00Z"},
			{ID: "eval-b", AgentTask: "t2", EvalType: "lint", Passed: bp(false), LastRunAt: "2026-05-17T14:20:00Z"},
			{ID: "eval-c", AgentTask: "t3", EvalType: "build", Passed: nil, CreatedAt: "2026-05-17T14:00:00Z"},
		},
	}

	for _, w := range widths {
		for _, n := range lineCounts {
			for _, nc := range noColors {
				if nc {
					t.Setenv("NO_COLOR", "1")
				} else {
					t.Setenv("NO_COLOR", "")
				}

				agentsOpts := AgentsOpts{Lines: n, Width: w, NoColor: nc, Now: now}
				assertInvariants(t, "agents", RenderAgents(context.Background(), agentsLister, agentsOpts), n, w, nc)

				evalsOpts := EvalsOpts{Lines: n, Width: w, NoColor: nc, Now: now}
				assertInvariants(t, "evals", RenderEvals(context.Background(), evalSrc, evalsOpts), n, w, nc)
			}
		}
	}
}

func assertInvariants(t *testing.T, name string, got []string, lines, width int, noColor bool) {
	t.Helper()
	if lines <= 0 {
		if got != nil {
			t.Errorf("[%s w=%d n=%d nc=%v] expected nil for Lines<=0, got %v", name, width, lines, noColor, got)
		}
		return
	}
	if len(got) != lines {
		t.Errorf("[%s w=%d n=%d nc=%v] expected %d lines, got %d", name, width, lines, noColor, lines, len(got))
		return
	}
	for i, ln := range got {
		if v := VisibleLen(ln); width > 0 && v > width {
			t.Errorf("[%s w=%d n=%d nc=%v] line %d width %d > %d: %q", name, width, lines, noColor, i, v, width, ln)
		}
		if noColor && strings.ContainsRune(ln, 0x1b) {
			t.Errorf("[%s w=%d n=%d nc=%v] NoColor line %d contains ANSI: %q", name, width, lines, noColor, i, ln)
		}
	}
}
