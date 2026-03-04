# SPEC Review -- ComputeCommander CLI Redesign (Iteration 3 -- Final)

## Verdict: PASS

## Summary

Both iteration 2 issues (N1: cross-process IPC for Git Status pane, N2: `$ZELLIJ_PANE_ID` for frame title rename) have been comprehensively resolved. The spec now contains a dedicated "Cross-Process Session Notification" section with a 5-step file-based IPC protocol, and `$ZELLIJ_PANE_ID` is documented in the `SessionFocusManager` data model, the `PaneIdMap` interface, and the Session Change Protocol narrative. The spec is implementable end-to-end with no critical or high-severity gaps remaining. All carried warnings from iterations 1 and 2 are non-blocking and can be resolved during implementation.

## Fix Verification

### N1: Cross-Process IPC
**VERIFIED.** A dedicated "Cross-Process Session Notification" section has been added at lines 686-696 under Integration. The specification:

- **Line 688**: Explicitly states the problem: "Panes running as separate OS processes (e.g., `cmdr git-status --pane`) cannot receive Go interface callbacks from the `SessionFocusManager`."
- **Line 689**: Defines the mechanism: file-based IPC via `.computecommander/active-session.path`.
- **Lines 690-694**: 5-step protocol:
  1. **Write**: `SessionFocusManager` atomically writes the active session's directory path (temp file + `os.Rename`)
  2. **Watch**: External pane processes watch via `fsnotify`
  3. **Read**: On `fsnotify.Write` event, the external process reads the new path and updates state
  4. **Startup**: Initial launch reads the existing value rather than waiting for a change event
  5. **Cleanup**: `cmdr stop` removes `active-session.path` alongside `cmdr.lock`
- **Line 696**: Rationale note explains why `fsnotify` was chosen over SIGUSR1 (more reliable, no PID tracking needed), with a reference to the existing SIGUSR1 pattern from commit `6e8cbe3` for context.

The fix is also reflected in three other locations:
- **Line 310**: `switchTo()` operation description now says "write active-session file"
- **Line 329**: `SessionFocusManager` narrative paragraph explains both in-process callbacks and cross-process file-based broadcast
- **Lines 789-791**: Session Change Protocol step 2 is now split into `(a) In-process broadcast` and `(b) Cross-process broadcast`, with the file-based mechanism described inline
- **Lines 543-548**: `cmdr git-status --pane` CLI help text documents that the pane watches `.computecommander/active-session.path` via `fsnotify`

The IPC mechanism is fully specified from write to watch to read to startup bootstrap to cleanup. An implementor can build T23 (`cmdr git-status --pane`) without ambiguity.

### N2: $ZELLIJ_PANE_ID
**VERIFIED.** The `$ZELLIJ_PANE_ID` environment variable discovery mechanism is now documented in multiple locations:

- **Lines 316-321**: A new `PaneIdMap` interface in the data model defines the pane ID mapping structure with `fp`, `agentWorkspace`, and `gitStatus` string fields, with a comment: "Zellij pane ID mapping for frame title updates and cross-process coordination"
- **Line 307**: `SessionFocusManager` includes `paneIds: PaneIdMap` field with comment: "captured from `$ZELLIJ_PANE_ID` at launch"
- **Line 329**: Narrative paragraph after the `SessionFocusManager` interface states: "Each pane's zellij pane ID is captured at launch from the `$ZELLIJ_PANE_ID` environment variable and stored in `paneIds` so that frame title updates can target the correct pane via `zellij action rename-pane --pane-id <id> \"<title>\"`"
- **Line 762**: FP pane behavior item 10 explicitly states: "The pane ID is captured at FP pane launch from the `$ZELLIJ_PANE_ID` environment variable (zellij injects this into every pane's environment) and stored in `SessionFocusManager.paneIds.fp`"
- **Line 793**: Session Change Protocol step 3 (FP pane reaction) references: "pane ID from `SessionFocusManager.paneIds.fp`, originally captured from `$ZELLIJ_PANE_ID` at pane launch"

The mechanism is fully documented: zellij injects `$ZELLIJ_PANE_ID` into each pane's environment, the application captures it at launch, stores it in `PaneIdMap`, and uses it for targeted `zellij action rename-pane --pane-id <id>` calls. No ambiguity remains.

---

## Dimension 1: Completeness

### Pass

All major features are specified with sufficient detail for implementation. The session management system (SessionFocusManager, PTYManager, cross-process IPC, session change protocol) is the most complex area and is now fully closed:

- In-process session coupling: Go interface callbacks for FP and Agent Workspace panes
- Cross-process session coupling: file-based IPC via `active-session.path` for Git Status pane
- PTY lifecycle: spawn, attach, detach, kill, scrollback, max-concurrent, crash recovery
- Session fallback: MRU ordering when active session is stopped
- Pane ID discovery: `$ZELLIJ_PANE_ID` captured at launch, stored in `PaneIdMap`

### Remaining Warnings (non-blocking)

- **C1-WARN-1 (carried): ExportData references undefined types.** `ExportData` (line 236-249) references `AgentSession[]`, `Event[]`, etc. without defining them. These map to existing DB tables. Adding "See existing SQLite schema in `migrations/`" would close this gap but is not blocking -- an implementor will naturally reference the existing schema.

- **C1-WARN-2 (carried): No error handling spec for session directory failures.** What happens when a session's directory is deleted while the session is active? What happens when two sessions target the same directory? PTY crash recovery is specified (line 826) but filesystem-level edge cases are not. An implementor can handle these as standard error paths.

- **C1-WARN-3 (carried): `cmdr fp` implementation approach ambiguity.** T19 creates a bubbletea component; the KDL layout runs an external `fp` binary. The spec heavily favors the bubbletea approach (session coupling depends on it). An explicit "replaces external `fp` binary" note would eliminate the last trace of ambiguity. Inferrable from context.

- **C1-WARN-4 (carried): DirectorySession persistence undecided.** Sessions appear to be in-memory (managed by `SessionFocusManager` runtime struct). Line 759 says "Sessions persist until explicitly stopped" but this means runtime persistence, not restart persistence. The spec does not state whether sessions survive a `cmdr` restart. The most natural reading is that they are ephemeral (re-created on next launch). Not blocking -- an implementor can default to ephemeral and add persistence later.

- **C1-WARN-5 (new, minor): `active-session.path` not listed in On-Disk Format.** The On-Disk Format section (lines 36-54) does not include `active-session.path` in the `.computecommander/` directory listing. It is documented in the Cross-Process Session Notification section (line 690) and the CLI help text (line 545), so an implementor will know about it. Adding it to the On-Disk Format listing would be more consistent.

---

## Dimension 2: Clarity

### Pass

The spec is well-organized with clear section boundaries. The session management additions from iteration 2 are thorough and use consistent terminology.

### Remaining Warnings (non-blocking)

- **C2-WARN-1 (carried): "cmdr" vs "cc" naming.** Open Question #5 (line 1442) asks whether both should be supported. The suggested default ("Change `Use` to `cmdr`, keep Makefile output as `cmdr`, add `cc` as alias") is reasonable and an implementor can adopt it. Converting this to a decision before implementation would be ideal.

- **C2-WARN-2 (carried): FP pane width 15% (spec) vs 10% (deployed).** The spec standardized on 15% in all internal references. The deployed KDL layout uses 10%. Minor -- pick one during implementation.

- **C2-WARN-3 (carried): `--tui` flag semantics.** Line 417 says `--tui` means "Force in-process TUI (skip wezterm)" but the default behavior is also a TUI. The distinction is wezterm-spawned-zellij vs in-process-bubbletea-only. One clarifying sentence would help, but the intent is inferrable.

- **C2-WARN-4 (carried): Leader key implementation layer.** Whether the leader key is bubbletea-level (only works when TUI pane is focused) or zellij-level (works from any pane). The KDL layout does not include keybindings (per MEMORY.md: "Layout files must NOT contain keybinds blocks"), and T11 says "leader key handler in TUI event loop", so bubbletea-level is the implicit answer. A confirming sentence would help.

---

## Dimension 3: Correctness

### Pass

All layout diagrams, KDL blocks, task descriptions, and data models are internally consistent.

### Remaining Warnings (non-blocking)

- **C3-WARN-1 (carried): FP pane width 15% vs 10%.** Spec says 15%, deployed says 10%. Both cannot be correct simultaneously. Low severity.

- **C3-WARN-2 (carried): `ShutdownCmd` naming.** T4 (line 1319) documents the workaround, Success Criteria (line 1412) references it. The fragility is acknowledged but mitigated by documentation.

### Info

- **C3-INFO-1 (carried): Go 1.25 does not exist yet.** Forward reference or aspirational.
- **C3-INFO-2 (carried): CI uses `actions/checkout@v6`.** Current latest is v4.

---

## Dimension 4: Consistency

### Pass

Pane naming, scoping terminology, and cross-references are consistent throughout.

- "Git Status" appears correctly in all 7+ locations (layout diagrams, KDL block, migration table, panels table, task manifest, CHANGELOG, CLI section)
- Session-scoped vs cross-project terminology is used consistently in the pane behavior table (lines 770-778), panels table (lines 1198-1206), and all session-related narrative sections
- The dual notification mechanism (in-process callbacks + cross-process file-based IPC) is consistently described in the data model (line 329), the Session Change Protocol (lines 789-796), the Cross-Process Session Notification section (lines 686-696), and the CLI help text (lines 543-548)

### Remaining Warnings (non-blocking)

- **C4-WARN-1 (carried): "Agents" pane vs "Agent Session"/"Agent Workspace".** These are distinct panes (right sidebar vs center workspace). The spec uses distinct names consistently in the panels table (lines 1198-1206). Residual risk of confusion in casual reading is minimal.

- **C4-WARN-2 (carried): Section numbering non-sequential.** Earlier sections use `##` without numbers; sections 15-19 have numeric prefixes. Cosmetic.

---

## Dimension 5: SDLC (Testing, CI, Deployment, Rollback)

### Pass

The test strategy covers session-scoping explicitly. T22 (line 1337) requires: "verify FP, Git Status, and Agent Workspace all update when session changes." Success criteria (line 1431) adds a machine-verifiable check for this.

### Remaining Warnings (non-blocking)

- **C5-WARN-1 (carried): Rollback plan is `git stash`.** Acceptable for pre-release. Does not address go.mod changes or generated files on user machines.

- **C5-WARN-2 (carried): No performance criteria.** 8 concurrent PTYs with 10k-line scrollback buffers could consume significant memory. A target like "session switch < 200ms" and "memory per PTY < 50MB" would be prudent. Not blocking for implementation.

- **C5-WARN-3 (carried): CI does not validate KDL syntax.** Malformed KDL would only surface at runtime. Adding `zellij setup --check` or a KDL parser test would catch this. Low severity.

- **C5-WARN-4 (carried): Missing directory creation in `cmdr init`.** T15 (line 1330) says "Update cmdr init to generate keybinds.yaml and open interface after DB start" but does not mention creating `backups/`, `layouts/`, `plugins/`, `themes/` directories. `cmdr backup` will fail if `backups/` does not exist. Medium severity but easily caught during implementation.

---

## Dimension 6: Actionability

### Pass

All 25 tasks have clear file scope (read and write), explicit dependencies, and verify commands. The execution order (4 phases plus integration) is aligned with the dependency graph. The new cross-process IPC section and `PaneIdMap` data model give T23 (git-status) and T24 (SessionFocusManager) implementors everything they need.

### Remaining Warnings (non-blocking)

- **C6-WARN-1 (carried): T19 fp approach.** T19 builds a bubbletea component. The deployed KDL runs an external `fp` binary. The bubbletea approach is authoritative (session coupling depends on it). An explicit "replaces external `fp` binary" note in T19 would be ideal.

- **C6-WARN-2 (carried): Open questions block tasks.** Questions 1-4 block T5, question 5 blocks T1, question 7 blocks T9/T19. Suggested defaults are provided and are reasonable. An implementor can adopt them as decisions. Converting them to decisions before starting Phase 1 is recommended but not blocking.

- **C6-WARN-3 (carried): T24/T25 dependency ordering.** T24 (SessionFocusManager) depends on T19 and T20. T25 (PTYManager) depends on T9 and T19. These are consumers of the FP and session commands, which makes sense -- the managers need the components they coordinate to exist first. The alternative (making managers foundational) would require stub interfaces in the consumers. The current ordering is defensible.

---

## Remaining Warnings Summary

All remaining warnings are non-blocking. They represent documentation polish, edge case coverage, or decisions that can be made during implementation.

| ID | Summary | Severity | Recommendation |
|----|---------|----------|----------------|
| C1-WARN-1 | ExportData references undefined types | Info | Add "See `migrations/`" reference |
| C1-WARN-2 | No session directory failure handling | Warning | Handle as standard error paths during implementation |
| C1-WARN-3 | fp approach ambiguity (bubbletea vs external) | Info | Add "replaces external `fp` binary" to T19 |
| C1-WARN-4 | DirectorySession persistence undecided | Warning | Default to ephemeral; add persistence if needed later |
| C1-WARN-5 | `active-session.path` missing from On-Disk Format | Info | Add to directory listing for consistency |
| C2-WARN-1 | cmdr vs cc naming (Open Question #5) | Warning | Adopt suggested default before implementation |
| C2-WARN-2 | FP pane width: 15% vs 10% | Info | Pick one during implementation |
| C2-WARN-3 | `--tui` flag semantics | Info | Add clarifying sentence |
| C2-WARN-4 | Leader key layer (bubbletea vs zellij) | Warning | Confirm bubbletea-level with one sentence |
| C3-WARN-1 | FP width discrepancy | Info | Same as C2-WARN-2 |
| C3-WARN-2 | ShutdownCmd naming fragility | Info | Already documented in T4 and success criteria |
| C4-WARN-1 | Agents vs Agent Session naming | Info | Panels table disambiguates |
| C4-WARN-2 | Section numbering non-sequential | Info | Cosmetic |
| C5-WARN-1 | Rollback plan incomplete | Info | Acceptable for pre-release |
| C5-WARN-2 | No performance criteria | Warning | Add targets during implementation if needed |
| C5-WARN-3 | CI does not validate KDL | Info | Add KDL validation test |
| C5-WARN-4 | Missing init directory creation | Warning | Add to T15 scope during implementation |
| C6-WARN-1 | T19 fp approach note | Info | One sentence addition |
| C6-WARN-2 | Open questions block tasks | Warning | Adopt suggested defaults |
| C6-WARN-3 | T24/T25 dependency ordering | Info | Current ordering is defensible |

---

## Final Assessment

The spec is **implementable**. All critical and high-severity issues from iterations 1 and 2 have been resolved. The session management system -- the most architecturally complex addition -- is fully specified across five interconnected sections: the `SessionFocusManager` data model (lines 299-329), the Session Change Protocol (lines 784-803), the Cross-Process Session Notification mechanism (lines 686-696), the PTY Swapping Mechanics (lines 816-827), and the `cmdr git-status --pane` CLI definition (lines 543-548). The dual notification strategy (in-process Go callbacks for FP/Agent Workspace + file-based fsnotify IPC for Git Status) is well-reasoned and consistent with the project's existing patterns. The `$ZELLIJ_PANE_ID` pane discovery mechanism is documented in the data model, the narrative, and the behavioral specification.

The 20 remaining warnings are all low-to-medium severity, non-blocking, and addressable during implementation without spec revision. An implementation team can begin Phase 1 (T1, T3, T16) immediately.

---

*Review iteration: 3 of 3 (final)*
*Reviewer: spec-reviewer*
*Date: 2026-03-03*
*Verdict: PASS*
