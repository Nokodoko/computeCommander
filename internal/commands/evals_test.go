package commands

import (
	"testing"
	"time"
)

// TestFormatEvalAgo covers the two timestamp formats that land in the evals
// table (T-separated from Go writers, space-separated from SQLite's
// datetime('now')) and the duration-bucket transitions used by the live
// Evals pane in the zellij dashboard. A regression here would surface as
// either a blank "ago" column or a constantly-misreported duration.
func TestFormatEvalAgo(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		ts   string
		want string
	}{
		{"empty -> dash", "", "-"},
		{"unparseable -> dash", "not-a-date", "-"},
		{"future timestamp clamps to 0s", "2026-04-29T12:00:30Z", "0s ago"},
		{"5s ago T-form", "2026-04-29T11:59:55Z", "5s ago"},
		{"5s ago space-form", "2026-04-29 11:59:55", "5s ago"},
		{"30s ago", "2026-04-29T11:59:30Z", "30s ago"},
		{"3m ago", "2026-04-29T11:57:00Z", "3m ago"},
		{"59m ago", "2026-04-29T11:01:00Z", "59m ago"},
		{"2h ago", "2026-04-29T10:00:00Z", "2h ago"},
		{"23h ago", "2026-04-28T13:00:00Z", "23h ago"},
		{"2d ago", "2026-04-27T12:00:00Z", "2d ago"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatEvalAgo(tc.ts, now)
			if got != tc.want {
				t.Errorf("formatEvalAgo(%q) = %q, want %q", tc.ts, got, tc.want)
			}
		})
	}
}
