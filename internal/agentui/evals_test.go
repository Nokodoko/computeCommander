package agentui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubEvalSource struct {
	evals []EvalRecord
	err   error
}

func (s *stubEvalSource) ListEvals(_ context.Context, _ string) ([]EvalRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.evals, nil
}

func bp(v bool) *bool { return &v }

func TestRenderEvals_Empty(t *testing.T) {
	src := &stubEvalSource{}
	opts := EvalsOpts{Lines: 6, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderEvals(context.Background(), src, opts)
	if len(got) != 6 {
		t.Fatalf("expected 6 lines, got %d: %v", len(got), got)
	}
	if got[0] != "evals: no data" {
		t.Errorf("empty case must produce 'evals: no data', got %q", got[0])
	}
}

func TestRenderEvals_PassFailMix(t *testing.T) {
	src := &stubEvalSource{
		evals: []EvalRecord{
			{ID: "eval-aaa", ProjectName: "cc", AgentTask: "stdlib", EvalType: "unit_test", Passed: bp(true), LastRunAt: "2026-05-17T14:32:04Z"},
			{ID: "eval-bbb", ProjectName: "cc", AgentTask: "smoke", EvalType: "integration", Passed: bp(false), LastRunAt: "2026-05-17T14:20:00Z"},
			{ID: "eval-ccc", ProjectName: "cc", AgentTask: "vet", EvalType: "lint", Passed: bp(true), LastRunAt: "2026-05-17T14:31:22Z"},
			{ID: "eval-ddd", ProjectName: "cc", AgentTask: "pending", EvalType: "custom", Passed: nil, CreatedAt: "2026-05-17T14:00:00Z"},
		},
	}
	opts := EvalsOpts{Lines: 8, Width: 80, NoColor: true, Now: fixedNow()}
	got := RenderEvals(context.Background(), src, opts)
	if len(got) != 8 {
		t.Fatalf("expected 8 lines, got %d", len(got))
	}
	for i, ln := range got {
		if v := VisibleLen(ln); v > opts.Width {
			t.Errorf("line %d width %d > %d: %q", i, v, opts.Width, ln)
		}
	}
	body := strings.Join(got, "\n")
	if !strings.Contains(got[0], "4 registered") || !strings.Contains(got[0], "2 pass / 1 fail") {
		t.Errorf("header should show counts, got %q", got[0])
	}
	for _, want := range []string{"eval-aaa", "eval-bbb", "eval-ccc", "eval-ddd", "PASS", "FAIL"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "\033") {
		t.Errorf("NoColor body contains ANSI:\n%s", body)
	}
}

func TestRenderEvals_OverflowIndicator(t *testing.T) {
	// 14 evals, lines=4 → header + 1 row + overflow indicator + trailer.
	evals := make([]EvalRecord, 14)
	for i := range evals {
		evals[i] = EvalRecord{
			ID: "eval-" + strings.Repeat("x", 3), AgentTask: "task", EvalType: "unit_test", Passed: bp(true),
			LastRunAt: "2026-05-17T14:32:04Z",
		}
	}
	src := &stubEvalSource{evals: evals}
	opts := EvalsOpts{Lines: 4, Width: 80, NoColor: true, Now: fixedNow()}
	got := RenderEvals(context.Background(), src, opts)
	if len(got) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(got))
	}
	body := strings.Join(got, "\n")
	if !strings.Contains(body, "more") {
		t.Errorf("body should include overflow '... +N more':\n%s", body)
	}
}

func TestRenderEvals_DBError(t *testing.T) {
	src := &stubEvalSource{err: errors.New("db locked")}
	opts := EvalsOpts{Lines: 5, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderEvals(context.Background(), src, opts)
	if got[0] != "evals: unavailable" {
		t.Errorf("DB error must produce 'evals: unavailable', got %q", got[0])
	}
}

func TestRenderEvals_NilSource(t *testing.T) {
	opts := EvalsOpts{Lines: 3, Width: 60, NoColor: true, Now: fixedNow()}
	got := RenderEvals(context.Background(), nil, opts)
	if got[0] != "evals: unavailable" {
		t.Errorf("nil source must produce 'evals: unavailable', got %q", got[0])
	}
}

func TestRenderEvals_ZeroLines(t *testing.T) {
	src := &stubEvalSource{}
	if got := RenderEvals(context.Background(), src, EvalsOpts{Lines: 0, Width: 60}); got != nil {
		t.Errorf("Lines=0 must return nil, got %v", got)
	}
}

func TestRenderEvals_ZeroWidth(t *testing.T) {
	src := &stubEvalSource{}
	got := RenderEvals(context.Background(), src, EvalsOpts{Lines: 3, Width: 0})
	if len(got) != 3 || got[0] != "evals: unavailable" {
		t.Errorf("Width=0 must produce degraded marker padded, got %v", got)
	}
}

func TestRenderEvals_ColorPath(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	src := &stubEvalSource{evals: []EvalRecord{
		{ID: "eval-xxx", AgentTask: "t", EvalType: "unit_test", Passed: bp(true), LastRunAt: "2026-05-17T14:32:04Z"},
	}}
	opts := EvalsOpts{Lines: 4, Width: 80, NoColor: false, Now: fixedNow()}
	got := RenderEvals(context.Background(), src, opts)
	body := strings.Join(got, "\n")
	// PASS uses the 8-color green from the palette (\033[32m). Type column
	// uses the 24-bit Dracula palette per eval type (#8be9fd → unit_test cyan).
	if !strings.Contains(body, "\033[32m") {
		t.Errorf("expected ANSI green for PASS:\n%s", body)
	}
	if !strings.Contains(body, "\033[38;2;139;233;253m") {
		t.Errorf("expected 24-bit cyan for unit_test type:\n%s", body)
	}
}

func TestFormatEvalAgoStr(t *testing.T) {
	now := time.Date(2026, 5, 17, 14, 32, 7, 0, time.UTC)
	cases := []struct {
		ts   string
		want string
	}{
		{"2026-05-17T14:32:04Z", "3s ago"},
		{"2026-05-17 14:32:04", "3s ago"}, // SQLite space-separated variant
		{"2026-05-17T14:20:07Z", "12m ago"},
		{"2026-05-17T12:32:07Z", "2h ago"},
		{"2026-05-15T14:32:07Z", "2d ago"},
		{"", "-"},
		{"garbage", "-"},
	}
	for _, tc := range cases {
		if got := formatEvalAgoStr(tc.ts, now); got != tc.want {
			t.Errorf("formatEvalAgoStr(%q) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}
