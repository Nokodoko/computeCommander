# agents-md.md Spec Review — Iteration 1/3

**Reviewer:** spec-reviewer
**Date:** 2026-03-17
**Spec:** `specs/agents-md.md`
**Verdict:** PASS WITH WARNINGS

---

## Dimension Scores

| Dimension | Weight | Score | Notes |
|-----------|--------|-------|-------|
| Completeness | 25% | WARN | Missing 6 of 19 standard sections; content specification compensates |
| Clarity | 15% | PASS | Unambiguous task descriptions, explicit file paths |
| Correctness | 25% | WARN | Permissions matrix example has inaccuracies vs actual agent files |
| Consistency | 15% | PASS | Task Manifest aligns with Agent Assignments and Dependency Graph |
| SDLC | 10% | WARN | Git workflow section is contradictory (push to main vs branch) |
| Actionability | 10% | PASS | Unix-coder can implement from manifest + content specification |

---

## 1. Completeness (25%) — WARN

### Present sections (13/19):
1. Why -- present
2. Design Principles -- present
3. On-Disk Format -- present
4. Data Model -- present (marked N/A)
5. CLI -- present (marked N/A)
6. JSON Output Format -- present (marked N/A)
7. Concurrency Model -- present (marked N/A)
8. Migration -- present (marked N/A)
9. Integration -- present
10. What It Does NOT Do -- present
11. Tech Stack -- present
12. Project Infrastructure -- present
13. Estimated Size -- present
14. Task Manifest (section 15) -- present
15. Dependency Graph (section 16) -- present
16. Target State (section 17) -- present
17. Verification Plan (section 18) -- present
18. Success Criteria (section 19) -- present
19. Agent Assignments -- present (unnumbered)

### Missing standard sections:
- No numbered sections 1-14 (sections jump from unnumbered prose to "15. Task Manifest"). The numbering is inconsistent — sections 1-14 exist as content but lack numbers, while 15-19 are numbered.
- **Failure Modes** and **Git Workflow Detail** are present as unnumbered addenda, which is good.
- No **API** or **Endpoints** section, but N/A for a doc file, so acceptable.

### Content specification:
The "Content Specification for agents.md" section (lines 373-457) is thorough and gives the unix-coder exact guidance on all 6 output sections. This compensates for any structural gaps.

### Findings:
- The permissions matrix example (lines 76-83) truncates with `...continues for all directories...` but the Content Specification (lines 396-422) lists all 26 directories. This is adequate.
- Success criteria cover file existence, line count, agent presence, section presence, and permission modes. Missing: no check that the permissions matrix actually has correct RW/R/- values per agent.

---

## 2. Clarity (15%) — PASS

- Task descriptions in the manifest are unambiguous.
- File paths are explicit and correct (`.claude/agents/*.md` exists, all 8 files confirmed).
- The "derived truth" principle (line 18) clearly states agents.md is not canonical.
- No contradictions within the main spec body.

Minor nit: The "Example content structure" block (lines 53-112) mixes prescriptive content with example markdown. A unix-coder might wonder whether to copy verbatim or use as guidance. The Content Specification section (373+) resolves this by being explicitly prescriptive.

---

## 3. Correctness (25%) — WARN

### Critical: Permissions cross-reference

Comparing the spec's Agent Registry table (lines 63-72) and Permissions Matrix example (lines 76-83) against actual agent files:

| Agent | Spec Says | Actual (from .claude/agents/) | Match? |
|-------|-----------|-------------------------------|--------|
| cmdr-agent | Read-Write | RW on internal/agents/, commands (sling,stop,status,inspect,coordinator), pkg/runtimes/, agents/*.yaml, plugins/focus-tracker/, internal/agentic/ | PARTIAL |
| cmdr-bridge | Read-Only | Read-Only (Write: NONE) | OK |
| cmdr-coder | Read-Write | RW on cmd/, internal/, pkg/, plugins/, scripts/, agents/, templates/, migrations/. RO on go.mod, go.sum, Makefile, etc. | OK |
| cmdr-jira | Read-Only | Read-Only (Write: NONE) | OK |
| cmdr-openbrain-agent | Read-Only | Read-Only (Write: NONE) | OK |
| cmdr-reviewer | Read-Only | Read-Only (Write: NONE) | OK |
| cmdr-security | Read-Only | Read-Only (Write: NONE) | OK |
| cmdr-ux-agent | Read-Only | Read-Only (Write: NONE) | OK |

### Specific permissions matrix issues:

**Issue 1 — cmdr-agent scope is narrower than cmdr-coder but spec doesn't capture this.**
The spec's example matrix (line 79) shows `cmdr-agent` with `-` for `cmd/` — this is correct (cmdr-agent does NOT own cmd/ broadly). But the example shows `cmdr-agent` with `RW` for `internal/agents/` and `internal/agentic/` and `RW` for `internal/commands/` — however, cmdr-agent only owns 5 specific command files (sling, stop, status, inspect, coordinator), not all of `internal/commands/`. The matrix format (directory-level granularity) cannot express "RW on 5 files in internal/commands/". This should be noted somewhere.

**Issue 2 — cmdr-reviewer scope.**
The spec matrix example shows `cmdr-reviewer` with `R` for `cmd/` and `internal/agents/`. This is correct per the actual file: "ALL .go files under cmd/, internal/, pkg/" plus ".rs files under plugins/". However, the reviewer also reads `agents/*.yaml` and `go.mod, Makefile` — these should appear in the matrix.

**Issue 3 — cmdr-bridge reads more than shown.**
cmdr-bridge owns files across `plugins/focus-watcher/`, `plugins/focus-tracker/`, `internal/zellij/`, `internal/commands/session_picker.go`, and `scripts/`. The example matrix only shows `internal/commands/` with `R`. The full matrix in the deliverable needs to capture all directories bridge reads from.

**Issue 4 — cmdr-ux-agent reads internal/agents/palette.go.**
The ux-agent file explicitly lists `internal/agents/palette.go` and `internal/agents/color_test.go` as owned files. The matrix should show `R` for `internal/agents/` for cmdr-ux-agent.

### File path correctness:
- All 8 agent files exist at the listed paths. Confirmed.
- `agentic_instructions.md` exists at project root. Confirmed.
- `specs/index.md` exists. Confirmed.

### Git workflow correctness:
See SDLC section below — the git workflow has a logical contradiction.

---

## 4. Consistency (15%) — PASS

- Task Manifest has 3 tasks (T1, T2, T3) assigned to unix-coder, code-review, unix-coder.
- Agent Assignments table (lines 305-309) matches exactly.
- Dependency Graph (lines 243-251) matches the Depends On column in the manifest.
- Execution Order (lines 313-322) is consistent with both.
- Estimated Size (2 files, ~252 LOC) aligns with Target State (1 new file, 1 modified).

No inconsistencies found.

---

## 5. SDLC (10%) — WARN

### Git workflow contradiction (lines 336-371):

The spec describes two contradictory approaches:
1. **Line 359:** `git push origin main` — pushing directly to main
2. **Lines 361-364:** Create `docs/agents-md` branch, push, create PR into main

Line 371 says "The preferred approach is to create a `docs/agents-md` branch." However, lines 343-349 describe stashing a feature branch, switching to main, then creating agents.md — which assumes T1 runs on a feature branch. This creates confusion:

- If the worker is on main already (no feature branch), the stash is unnecessary.
- If the worker is on a feature branch, T1 creates agents.md there, then T3 must move it to a new branch off main.

**Recommendation:** Remove the "push to main" option entirely. Specify clearly: T3 creates `docs/agents-md` from `main`, cherry-picks or recreates agents.md there, then PRs into main.

### Verification plan:
- T1 verification (`test -f agents.md && wc -l`) is executable and sound.
- T2 verification (`echo "Review complete"`) is a no-op — should be code-review agent's finding output path.
- T3 verification uses `gh pr list --head main` which would fail for a branch-based PR (the head would be `docs/agents-md`, not `main`).

### Rollback:
`git checkout main -- agents.md` and `git revert HEAD` are both viable. Adequate.

---

## 6. Actionability (10%) — PASS

The Content Specification section (lines 373-457) gives a unix-coder everything needed:
- Exact section structure (6 sections with names)
- Header text (nearly verbatim)
- All 8 agents with metadata
- Complete directory list for the permissions matrix (26 entries)
- Usage guidelines decision tree
- Context-engine routing table with keywords
- Worker output protocol format

A unix-coder can implement from the manifest + content specification alone without needing to re-read the prose sections.

The read scope in T1 correctly lists all 8 agent files plus `agentic_instructions.md`.

---

## Summary of Findings

### Must Fix (for iteration 2):

1. **Permissions matrix granularity note.** Add a footnote or column that cmdr-agent has RW on only 5 specific files in `internal/commands/`, not the full directory. Without this, the matrix implies full directory RW.

2. **Git workflow: pick one approach.** Remove the "push to main" option. Specify branch-based PR workflow only. Fix the T3 verify command to match (`--head docs/agents-md`).

3. **T3 verify command fix.** `gh pr list --head main` will never match a PR from `docs/agents-md`. Change to `gh pr list --head docs/agents-md`.

### Should Fix:

4. **cmdr-ux-agent reads internal/agents/.** Add `R` for cmdr-ux-agent in the `internal/agents/` row of the example matrix.

5. **cmdr-bridge directory coverage.** The full matrix should show cmdr-bridge reads from `plugins/focus-watcher/`, `plugins/focus-tracker/`, `internal/zellij/`, `scripts/`, and `internal/commands/` (session_picker.go).

6. **Success criteria gap.** Add a check that validates the permissions matrix has correct values (not just that it exists). Example: `grep -P 'cmdr-coder.*RW' agents.md`.

### Nice to Have:

7. **Section numbering.** Number sections 1-14 to match the 19-section standard, even for N/A sections.

8. **T2 verify command.** Replace `echo "Review complete"` with something that checks the review output file exists.

---

## Verdict: PASS WITH WARNINGS

The spec is implementable as-is. The content specification is thorough and gives clear guidance. However, the permissions matrix example has inaccuracies that could propagate into the deliverable, and the git workflow section has a contradiction that could confuse the unix-coder. These should be addressed in iteration 2.
