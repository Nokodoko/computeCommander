# Spec Review Feedback — Iteration 2

All 4 critical issues from iteration 1 have been resolved. No critical fixes remain. The spec is ready for swarm execution.

## Critical Fixes (must address)

None.

## Warnings (should address)

1. **Formally resolve remaining open questions.** Open Questions #2, #3, #4 still have "Suggested Default" status. Mark them `[RESOLVED]` and integrate the decisions into the relevant spec sections (e.g., add "No custom Jira fields in v1" to section 10 "What It Does NOT Do"). This eliminates any worker hesitation.

2. **T4 verify command lacks functional validation.** T4 (renderer) verifies only via `go build`. Consider noting in the Task Manifest that functional validation of the renderer is deferred to T11 (tests) and T13 (code review). Alternatively, no change is needed — the current verify command is valid, just minimal.

3. **Section numbering is cosmetic only.** The H1 title serves as section 1, making the numbered `##` headers offset by 1 from the 19-section template. This does not affect execution but may confuse automated section validators.

4. **T11 scheduling is conservative.** T11 could start as soon as T2+T3 complete, rather than waiting for all of Phase 2. The Execution Order annotation already notes this. Consider moving T11 to Phase 2b for maximum parallelism if execution speed matters.
