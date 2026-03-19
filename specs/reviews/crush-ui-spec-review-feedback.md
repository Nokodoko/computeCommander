# Spec Review Feedback — Iteration 1

Incorporate the following fixes into the next crush-ui-spec.md revision:

## Critical Fixes (must address)

1. **Verify and fix all `charm.land/` import paths.** The spec uses `charm.land/bubbles/v2/table`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/glamour/v2` throughout. Run `go list -m charm.land/bubbles/v2@latest` to verify these resolve. If not, replace with `github.com/charmbracelet/bubbles/v2`, etc. Update every import path reference in: Tech Stack table, Component Replacement Map, Charm API Reference code examples, palette.go code example, and all task descriptions that mention import paths.

2. **Add charmtone verification and fallback to T1.** Add a step to T1's description: "After `go get github.com/charmbracelet/x/exp/charmtone`, run `go doc` to verify token names. If any token name differs, use the hex values from the comments as `lipgloss.Color` constants." This prevents T1 from blocking all downstream tasks if a single token name is wrong.

3. **Resolve ultraviolet dependency risk in T9.** Update T9 description to: "Add responsive breakpoints to dashboard. Use `ultraviolet` if available; otherwise implement manual `calculateLayout()` with if/else breakpoints at 160/120/80 cols." Move ultraviolet to an "optional" note in Tech Stack rather than a required dependency.

4. **Add `internal/commands/status.go` to Target State.** T4's write-scope includes this file but it is missing from the "Files modified" list in Target State Section 16.

5. **Fix Phase 3 dependency conflict: T7 depends on T5.** Move T7 from Phase 3 to Phase 4 in the Dependency Graph. Then move T8 (depends on T7) from Phase 4 to Phase 5. Recalculate phases 4-5 assignments for the full cascade.

## Warnings (should address)

1. **Address existing `internal/tui/palette.go`.** The codebase already has this file with `AgentColorStyle`, `PaletteStyles`, and `CompletedGoldStyle`. Add it to Target State as modified or deleted, and add migration steps to T2 or a new task.

2. **Add API disclaimer to Charm API Reference.** The Bubbles v2 code examples use v1 API patterns. Add a note: "API examples are based on v1 patterns and must be adapted to v2 signatures during implementation."

3. **Fix Estimated Size file count.** Change `~23` to `~24` to match Target State (3 new + 21 modified).

4. **Resolve Open Question #1 before execution.** The v1-vs-v2 question blocks the entire component replacement phase. Add a pre-flight verification task (T0) or resolve the question with a definitive answer in the spec.

5. **Make Success Criteria grep patterns more flexible.** Change `grep -q 'bubbles/v2/table'` to `grep -q 'bubbles.*v2.*table'` (or similar) to avoid false negatives if import paths differ from spec.

6. **Fix Data Model section framing.** Remove "Not applicable" label since the section contains substantial migration maps. Reframe as "No data model changes -- visual mapping tables below."
