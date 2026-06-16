// Package agentui is the shared renderer for cmdr's embeddable subcommands
// (agents-summary, evals-summary, lazygit-summary) and the long-lived
// dashboard panes that emit the same content shapes (status --pane,
// evals --pane).
//
// # Contract
//
// Every renderer in this package honors the same shape contract:
//
//   - Output is []string with EXACTLY opts.Lines entries. Truncates overflow,
//     pads short output with empty strings.
//   - Every line is <= opts.Width VISIBLE columns. ANSI CSI escape sequences
//     do NOT count toward visible width. Cuts on rune boundaries; if a cut
//     occurs inside an active SGR, a trailing "\033[0m" is appended.
//   - opts.NoColor strips ALL ANSI escapes AND Unicode box-drawing.
//     Output becomes pure 7-bit printable ASCII plus newlines.
//   - opts.Lines <= 0 returns nil. opts.Width <= 0 returns DegradedMarker
//     padded to opts.Lines.
//   - Renderers NEVER return a Go error that surfaces to a non-zero exit.
//     Failure modes degrade to a single-line marker ("agents: unavailable",
//     "evals: no data", "git: not a repo") followed by empty padding.
//   - Latency target: < 100 ms per render so the wrapping subcommand stays
//     under the 300 ms p99 budget the sessionbanner consumer relies on.
//   - Determinism: identical input produces identical output. Tests inject
//     time.Now() via opts.Now for stable golden comparisons; production
//     code uses time.Now().
//
// # Consumers
//
// The package is imported by:
//
//   - internal/commands/agents_summary.go    (the new agents-summary subcommand)
//   - internal/commands/evals_summary.go     (the new evals-summary subcommand)
//   - internal/commands/lazygit_summary.go   (the new lazygit-summary subcommand)
//
// In a follow-up phase the long-lived dashboard panes
// (internal/commands/status.go runStatusPane, internal/commands/evals.go
// runEvalsPane) and the bubbletea tui (internal/tui/) will converge onto
// these renderers so dashboard output stays identical pre- and post-
// migration.
package agentui
