# Spec Review Feedback (Iteration 1)

## Critical Fixes Required

1. **[Section: Design Principles, line 15]** Fix incorrect pattern reference. The spec claims EvalsPane mirrors `MergeQueueView` / `MailSummary` but neither has `SetSize()`. The correct pattern references are `EventsPane` and `GitStatusPane`.

   Current: "The new Evals pane mirrors `MergeQueueView` / `MailSummary` in structure: a Go struct with `View()`, `Refresh()`, `SetSize()`, registered in `Dashboard`, wired via `NewDashboard()`, rendered in `View()`."

   Correct: "The new Evals pane mirrors `EventsPane` / `GitStatusPane` in structure: a Go struct with `View()`, `Refresh()`, `SetSize(w, h int)`, registered in `Dashboard`, wired via `NewDashboard()`, rendered in `View()`. It is queried from the database like `MergeQueueView.Refresh()`, but sized explicitly like `EventsPane.SetSize()`."

   Also update Task T3's read scope:

   Current: `internal/tui/merge_view.go`, `internal/tui/events_pane.go`, `internal/tui/theme.go`, `internal/platform/db/db.go`

   Correct: `internal/tui/events_pane.go`, `internal/tui/git_status.go`, `internal/tui/merge_view.go`, `internal/tui/theme.go`, `internal/platform/db/db.go`

   (Move `events_pane.go` and add `git_status.go` as the primary structural references; keep `merge_view.go` for the DB query pattern only.)

2. **[Section: Task Manifest, T4]** Add `internal/tui/dashboard_test.go` to T4's write scope and update the task description to include fixing `TestPaneNavigation` wrap-around assertions.

   Current T4 write scope: `internal/tui/pane.go`, `internal/tui/render.go`

   Correct T4 write scope: `internal/tui/pane.go`, `internal/tui/render.go`, `internal/tui/dashboard_test.go`

   Add to T4 description: "Update `TestPaneNavigation` in `dashboard_test.go` to expect `PaneEvals` (not `PaneGitStatus`) as the last pane in wrap-around assertions."

   Also in the Failure Modes section (line 531), fix the test name reference:

   Current: "Tests in `dashboard_test.go` (`TestPaneCycle`, `TestPaneMetaByID`) fail"

   Correct: "Tests in `dashboard_test.go` (`TestPaneNavigation`, `TestPaneMetaByID`) fail"

## Warnings to Address

1. **[Section: Implementation Details, T2/T3]** Add a note specifying SQL parameter convention. Add after the `Refresh()` query specification:

   "All SQL queries in both `evals.go` (CLI) and `evals_pane.go` (TUI) must use `$1` positional parameter style, matching the project convention established in `internal/agents/spawner.go` and `internal/agents/agentic_instructions.md`."

2. **[Section: Implementation Details, T6, lines 628-641]** Add an explicit Sprintf argument position guide for the new pane. After the KDL template snippet, add:

   "In the `GenerateLayout()` Sprintf call, the new Evals pane requires two additional arguments after the existing lazygit arguments: `cmdrBin` (for the command path) and `projectFlag` (for the optional `--project` flag). The full argument list for the Sprintf becomes: `projectDir, tabName, tabHash, projectDir, fpWrapperPath, projectDir, agentPane, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, cmdrBin, projectFlag, lazygitWrapperPath, projectDir, cmdrBin, projectFlag` (last two are new)."

3. **[Section: Implementation Details, T5]** Add a note explaining the `updatePaneSizes()` asymmetry. After the `d.evals.SetSize(bottomPaneW-2, bottomH-3)` instruction, add:

   "Note: `MailSummary` and `MergeQueueView` do not have `SetSize()` methods and are not sized in `updatePaneSizes()`. They render without explicit dimensions, relying on `RenderPane()` to constrain content. `EvalsPane`, like `EventsPane` and `GitStatusPane`, uses explicit sizing for scroll/cursor support."

4. **[Section: Implementation Details, T2]** Expand T2 implementation details with a code skeleton:

   ```
   T2's `--pane` mode should follow this pattern from `runGitStatusPane`:
   - Use `app.DB` for all database queries
   - Ticker loop with 3-second interval
   - `clearScreen()` before each render
   - ANSI color output using the `evalTypeANSI` and `ansiPass/ansiFail/ansiPending` constants defined in the Color Scheme section
   - The `--pane` mode is a completely separate code path from the TUI `EvalsPane` component; it does NOT import or call any `internal/tui` code
   ```

5. **[Section: Integration, line 327]** Add a note acknowledging the KDL vs TUI bottom-row asymmetry:

   "Note: The 4th bottom pane differs between modes -- KDL uses LazyGit (interactive git TUI) while TUI uses Git Status (read-only git summary). This asymmetry is inherited from the existing architecture. The new Evals pane is the 5th pane in both modes."

6. **[Section: All]** Fix section numbering. Either number all sections 1-23, or remove the numbers from sections 15-19 and use titles only.

7. **[Section: Color Scheme]** Remove or soften the external hook file references. Change the provenance note from asserting the colors "match" specific hook files to simply defining the Dracula palette subset used:

   Current: "Eval type colors are drawn from the existing hook dunst notification palette to ensure visual consistency..."

   Correct: "Eval type colors use a Dracula palette subset. These colors are consistent with the project's notification system."

8. **[Section: Implementation Details, T5, line 610]** Add a note confirming `DashboardOpts` does not need modification:

   "The `DashboardOpts` struct already has a `DB db.DB` field (dashboard.go line 27), so no changes to the opts struct are needed. Pass `opts.DB` directly to `NewEvalsPane()`."
