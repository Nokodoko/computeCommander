# Spec Review: ComputeCommander Evals Pane

**Iteration:** 1/3
**Date:** 2026-03-05
**Verdict:** PASS WITH WARNINGS

## Summary

The spec is well-structured and provides substantial detail for implementing an Evals pane across both the KDL layout and BubbleTea TUI. Most sections are thorough, with complete SQL migrations, Go struct definitions, and CLI specifications. However, there are two critical issues (incorrect pattern claim leading agents to the wrong reference files, and missing test file updates that will cause T8 to fail) and several warnings around inconsistencies between the spec and the actual codebase that could lead to implementation errors.

## Dimension Scores

| Dimension | Rating | Critical | Warnings |
|-----------|--------|----------|----------|
| Completeness | WARN | 1 | 2 |
| Clarity | PASS | 0 | 2 |
| Correctness | WARN | 1 | 3 |
| Consistency | WARN | 0 | 3 |
| SDLC | PASS | 0 | 1 |
| Actionability | PASS | 0 | 1 |

## Findings

### Critical (must fix)

- [C1] **Incorrect pattern claim in Design Principles (Section: Design Principles, line 15).** The spec states: "The new Evals pane mirrors `MergeQueueView` / `MailSummary` in structure: a Go struct with `View()`, `Refresh()`, `SetSize()`." Neither `MergeQueueView` (`internal/tui/merge_view.go`) nor `MailSummary` (`internal/tui/mail_summary.go`) implements `SetSize()`. Verified by searching both files -- zero matches. The panes that DO have `SetSize()` are `EventsPane` (`internal/tui/events_pane.go`), `GitStatusPane` (`internal/tui/git_status.go`), `FilePicker`, and `AgentSession`. The EvalsPane must follow the `EventsPane`/`GitStatusPane` pattern (which includes `SetSize`, `width`, `height` fields). Furthermore, in `updatePaneSizes()` (`dashboard.go` line 427), `SetSize` is called for `eventsPane` and `gitStatus` but NOT for `mail` or `queue`. Task T3's read scope lists `internal/tui/merge_view.go` as the primary reference -- this will mislead the implementing agent. The correct primary references are `internal/tui/events_pane.go` and `internal/tui/git_status.go`.

- [C2] **Missing test file in task write scopes (Section: Task Manifest, T4/T5).** `internal/tui/dashboard_test.go` contains `TestPaneNavigation` (line 171) which hardcodes `PaneGitStatus` as the last pane in wrap-around assertions (lines 190-200: `nextPane(PaneGitStatus)` expects `PaneFilePicker`, and `prevPane(PaneFilePicker)` expects `PaneGitStatus`). After adding `PaneEvals` to the end of the iota block and `paneOrder`, the wrap-around target changes from `PaneGitStatus` to `PaneEvals`, breaking this test. Neither T4 nor T5 includes `internal/tui/dashboard_test.go` in their write scope, and T8's verify command (`go test ./internal/tui/...`) will fail. Additionally, the Failure Modes section (line 531) references `TestPaneCycle` and `TestPaneMetaByID` -- `TestPaneCycle` does not exist; the actual test name is `TestPaneNavigation`.

### Warnings (should fix)

- [W1] **SQL parameter style not specified (Section: Implementation Details, T3).** The spec's `Refresh()` query uses no parameters, but `RunAll()` and the `--pane` mode (T2) will need parameterized queries for UPDATE and filtered SELECT. The codebase uses mixed conventions: `$1` positional params in newer code (`internal/agents/spawner.go`, `internal/tui/dashboard.go`, `internal/commands/status.go`) and `?` params in older code (`internal/mail/sql_store.go`, `internal/merge/queue.go`). The `internal/agents/agentic_instructions.md` explicitly says "SQL uses `$1` positional params (compatible with both postgres and sqlite)." The spec should state which convention to use to prevent the implementing agent from picking the wrong one.

- [W2] **KDL layout Sprintf argument mapping not provided (Section: Implementation Details, T6, lines 628-641).** `GenerateLayout()` (`internal/zellij/layout.go` line 101-146) uses a single large `fmt.Sprintf` with 16 positional arguments. Adding a 5th bottom-row pane requires inserting 2+ new format arguments (`cmdrBin` and `projectFlag`) at the correct position in the Sprintf call. The spec shows the KDL template snippet but does not provide the complete argument list or the positional index where new arguments should be inserted. Given this Sprintf has 16 args already, off-by-one errors are likely without an explicit argument map.

- [W3] **`updatePaneSizes()` inconsistency not addressed (Section: Implementation Details, T5, line 613).** The spec says to add `d.evals.SetSize(bottomPaneW-2, bottomH-3)` in `updatePaneSizes()`. This is correct. However, the existing `mail` and `queue` components do NOT have SetSize called on them in `updatePaneSizes()` (they render without explicit sizing). The spec does not explain this asymmetry, which could confuse the implementer into thinking they need to add SetSize calls for mail/queue as well, or conversely, that EvalsPane doesn't need SetSize because other bottom panes don't have it.

- [W4] **T2 implementation details are sparse compared to T3 (Section: Implementation Details).** T2's `--pane` mode runs as a separate OS process (`cmdr evals --pane`) with its own ticker loop querying the database independently. The spec says "Follows the `GitStatusCmd` / `FeedCmd` pattern for `--pane` mode with a ticker loop" (line 169) but provides no code skeleton for T2, while T3 gets a full struct definition, method signatures, and column widths. The T2 implementer must: (a) understand that `--pane` mode is a completely separate code path from the TUI EvalsPane, (b) use `app.DB` for queries, (c) implement `clearScreen()` + render loop, (d) format output with ANSI colors matching the lipgloss styles. These are all inferrable from the referenced files but should be more explicit.

- [W5] **Bottom row pane count mismatch between KDL and TUI (Section: Integration, line 327).** In KDL mode, the 4th bottom pane is "LazyGit" (a bash wrapper). In TUI mode, the 4th bottom pane is "Git Status" (an in-process component). The spec adds "Evals" as the 5th pane in both modes, making KDL have 5 panes (Event Log, Mail, Merge Queue, LazyGit, Evals) and TUI have 5 panes (Events, Mail, Merge Queue, Git Status, Evals). This asymmetry is inherited from the existing codebase and is not a spec bug, but the spec does not acknowledge it. An implementer might be confused by the discrepancy.

- [W6] **Section numbering is inconsistent.** The spec has 23 top-level `##` sections total but numbers only sections 15-19. Sections 1-14 are implied by ordering, and sections 20-23 (Agent Assignments, Execution Order, Failure Modes, Implementation Details) are unnumbered. This makes it difficult to reference specific sections by number.

- [W7] **Color scheme references external hook files that are not in the repo.** The Color Scheme section references `~/.claude/hooks/intent/eval_loop.py:PREDICATE_NOTIFICATION_COLORS`, `intent-eval-posttool.py`, and `intent-build-verify.py` as the source of truth for color values. These files are user-specific and not part of the computeCommander repository. The hex color values are explicitly provided (so implementation doesn't depend on the files), but the provenance claim is unverifiable by another implementer.

- [W8] **`NewEvalsPane` needs DB handle from `NewDashboard` but spec doesn't show DashboardOpts change.** The spec's T5 instructions (line 610) say to add `evals: NewEvalsPane(opts.DB, theme),` in `NewDashboard()`. This works because `DashboardOpts` already has a `DB db.DB` field (dashboard.go line 27). However, the spec does not mention that `DashboardOpts` does NOT need modification, which is good -- but an explicit "DashboardOpts already has DB; no change needed" note would prevent unnecessary modifications.

### Notes (informational)

- [N1] **Go 1.25 reference.** The Tech Stack section says "Go 1.25". As of March 2026, this is plausible if the project tracks the latest Go release. No action needed.

- [N2] **Migration 002 and 003 exist as untracked files.** Both `internal/platform/db/migrations/sqlite/002_system_wide.sql` and `003_agentic_foundation.sql` show as untracked (`??`) in git status. If these haven't been committed when an agent runs T1, migration 004 would numerically follow untracked files. This won't break the embed directive (`//go:embed migrations/sqlite/*.sql`) since it embeds all `.sql` files in the directory, but if 002/003 are not committed, the 004 migration could run on a database that lacks the 002/003 tables. This is a project state issue, not a spec issue.

- [N3] **No new Go dependencies confirmed.** All required packages (`os/exec`, `database/sql`, `crypto/rand`, `charmbracelet/lipgloss`, etc.) are already imported elsewhere in the project.

- [N4] **The `_migrations` table's `applied_at` default uses `datetime('now')` (SQLite syntax).** This is fine for SQLite but would break on PostgreSQL if the same DDL were used. The `Migrate()` function uses the same DDL for both drivers (line 37-40 of migrate.go). However, since the migrations tracking table already exists on any initialized database, this is not a concern for the new 004 migration.
