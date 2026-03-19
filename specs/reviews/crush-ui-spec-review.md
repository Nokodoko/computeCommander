# Spec Review: crush-ui-spec.md

## Iteration: 2/3

## Verdict: PASS WITH WARNINGS

## Dimension Scores

| Dimension | Score | Weight | Issues |
|-----------|-------|--------|--------|
| Completeness | PASS | 25% | 0 critical, 0 warnings |
| Clarity | PASS | 15% | 0 critical, 0 warnings |
| Correctness | WARN | 25% | 0 critical, 2 warnings |
| Consistency | PASS | 15% | 0 critical, 1 warning |
| SDLC | PASS | 10% | 0 critical, 0 warnings |
| Actionability | PASS | 10% | 0 critical, 0 warnings |

## Previous Critical Findings -- Resolution Status

### CORR-001 [CRITICAL] -- Import paths use `charm.land/` vanity URLs
**Status: RESOLVED.** All import paths in the spec now use `github.com/charmbracelet/bubbles/v2`, `github.com/charmbracelet/bubbletea/v2`, `github.com/charmbracelet/lipgloss/v2`, `github.com/charmbracelet/glamour/v2` consistently. The Tech Stack table (Section 11), Component Replacement Map (Section 4), Charm API Reference, and all code examples use the correct `github.com/charmbracelet/*` module paths.

### CORR-002 [CRITICAL] -- charmtone token names unverifiable
**Status: RESOLVED.** T1 now includes an explicit fallback strategy: "run `go doc github.com/charmbracelet/x/exp/charmtone` to verify token names. If any token name differs from this spec, use the hex values from the comments (e.g., `#AD58B4` for Primary) as `lipgloss.Color()` constants instead." T0 also verifies charmtone token names as a pre-flight step. The hex fallback values are embedded as comments in the palette.go code example.

### CORR-003 [CRITICAL] -- ultraviolet may not exist as a Go module
**Status: RESOLVED.** ultraviolet is now marked `(optional)` in the Tech Stack table. T9 description explicitly says: "Use `ultraviolet` if available (check T0 result); otherwise implement manual `calculateLayout()` with if/else breakpoints at 160/120/80 cols." T0 includes ultraviolet verification. T21 says "Only add ultraviolet if T0 confirmed it resolves."

### CONS-001 [CRITICAL] -- T4 writes `internal/commands/status.go` but missing from Target State
**Status: RESOLVED.** `internal/commands/status.go` now appears in the Target State "Files modified" list (Section 16, line showing it between `openbrain_pane.go` and `styles.go`).

### CONS-002 [CRITICAL] -- T7 and T5 both in Phase 3, but T7 depends on T5
**Status: RESOLVED.** The dependency graph has been completely restructured. T5 is now in Phase 4 (Quick Wins). T7 is in Phase 5 (depends on T5). T8 is in Phase 6 (depends on T7). The cascade is properly sequenced: T5 (Phase 4) -> T7 (Phase 5) -> T8 (Phase 6).

## Findings

### [WARNING] Correctness: Bubbles v2 API examples still based on v1 patterns

- **Location**: Charm API Reference (Section 19), all code examples for table, list, viewport, spinner
- **Issue**: The spec now includes a disclaimer: "API v2 DISCLAIMER: The Bubbles v2 code examples below are based on v1 API patterns and must be adapted to v2 signatures during implementation." This addresses the concern from CORR-005 in iteration 1. However, the examples still show v1 patterns (e.g., `list.New(items, delegate, w, h)`, `paletteDelegate.Render(w io.Writer, m list.Model, index int, item list.Item)`). If v2 has significant constructor or interface changes, implementers will need to rewrite these patterns from `go doc` output. The disclaimer mitigates the risk but does not eliminate it.
- **Fix**: Acceptable as-is for iteration 2. The disclaimer is clear enough that a unix-coder will know to verify. If possible in iteration 3, add a note to each code example block: "Verify constructor signature with `go doc github.com/charmbracelet/bubbles/v2/table.New`" etc.

### [WARNING] Correctness: T0 and T21 have circular dependency risk

- **Location**: Task Manifest (T0, T21), Dependency Graph Phase 1
- **Issue**: T0 ("Pre-flight dependency verification: `go get` all new dependencies") and T21 ("Update go.mod: add bubbles/v2, lipgloss/v2, bubbletea/v2, charmtone, glamour/v2") are placed in Phase 1 as parallel tasks. Both write to `go.mod` and `go.sum`. Running `go get` in T0 will modify `go.mod`, and T21 also modifies `go.mod`. If run in parallel, they will conflict on the same file. In practice, T0 and T21 do substantially the same thing (adding dependencies to go.mod). The distinction between "verify they resolve" (T0) and "add them to go.mod" (T21) is unclear when `go get` does both.
- **Fix**: Merge T0 and T21 into a single task, or make T21 depend on T0 (sequential, not parallel). T0 can do the `go get` and verification, and T21 becomes a no-op or is removed. Alternatively, clarify that T0 is read-only verification (e.g., `go list -m ... @latest` without `-mod=mod`) and T21 does the actual `go get`.

### [WARNING] Consistency: Estimated file count discrepancy persists

- **Location**: Estimated Size (Section 13), Target State (Section 16)
- **Issue**: Estimated Size now says `~25` files total. Target State lists 3 new files + 22 modified files (including `internal/tui/palette.go` which was added per CORR-004 feedback) = 25 files, plus `go.mod` and `go.sum` = 27 file touches. The `~25` is close but understates by 2 if counting `go.mod`/`go.sum`.
- **Fix**: Minor. Either update to `~27` or note that `go.mod`/`go.sum` are excluded from the count as infrastructure files. Not blocking.

### [INFO] Completeness: Previous CORR-004 (existing palette.go not addressed) resolved

- **Location**: Task Manifest T2, Target State Section 16
- **Issue**: T2 now explicitly says "Also migrate `internal/tui/palette.go` (existing `AgentColorStyle`, `PaletteStyles`, `CompletedGoldStyle`) -- move color definitions to `internal/ui/palette/palette.go` and update all callers, then delete the old file or re-export from the new package." Target State lists `internal/tui/palette.go` as modified. Well handled.

### [INFO] Completeness: Previous COMP-001 (Data Model N/A label) resolved

- **Location**: Section 4
- **Issue**: Now reads "No data model changes -- this is a pure visual refactor. All data models remain unchanged. The following visual mapping tables define the transformation." Clean.

### [INFO] Actionability: Previous ACTN-001 (recommend /swarm over /pai) resolved

- **Location**: Execution Order section
- **Issue**: Now recommends `/swarm` with rationale: "Phase-gated parallel fan-out. The 8 phases have clear sequential dependencies with parallelism within each phase."

### [INFO] SDLC: Previous SDLC-001 (grep patterns too literal) partially resolved

- **Location**: Success Criteria Section 18
- **Issue**: Grep patterns now use `grep -qE 'bubbles.*v2.*table'` flexible patterns instead of literal import path strings. This addresses the concern about import path format differences. The charmtone count check (`grep -c 'charmtone'`) is also format-agnostic.

### [INFO] Consistency: Previous CONS-003 (file count ~23 vs 24) resolved

- **Location**: Estimated Size Section 13
- **Issue**: Updated from `~23` to `~25`. Close enough to the actual count. See minor warning above about go.mod/go.sum.

## Summary

The revision addressed all 5 critical findings from iteration 1. Import paths are corrected to `github.com/charmbracelet/*`. Charmtone token fallback is explicit. Ultraviolet is optional with manual fallback. `status.go` is in Target State. The dependency graph cascade (T5->T7->T8) is properly sequenced across phases.

Two new warnings surfaced: the T0/T21 parallel write conflict on `go.mod` and the persistent v2 API uncertainty (mitigated by disclaimer). Neither is blocking -- the T0/T21 issue is a phasing optimization that a supervisor can resolve at execution time, and the v2 disclaimer is sufficient for an experienced implementer.

The spec is ready for execution with the noted caveats.
