# Spec Review — CMDR_CODER_AGENT

Spec under review: `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`
Source template: `/home/n0ko/Programs/ai/icarus/SPEC/ICARUS_CODER_AGENT/ICARUS_CODER_AGENT.md`
Reviewer: spec-reviewer (parent workflow Phase 2)
Review date: 2026-04-26

---

## Verdict: **PASS WITH WARNINGS**

The spec is structurally complete, faithful to the icarus_coder template, and adapted to computeCommander's actual stack (Go 1.25 + Rust focus-watcher, Cobra/BubbleTea, dual-DB, Zellij/WezTerm). All three pinned rules carry over correctly with computeCommander-specific paths. Task manifest is internally consistent and verifiable. Five warnings below are cosmetic / clarification-grade; none are blockers for committing the spec to disk.

---

## Findings

### [PASS] Structural completeness (Sections 1–19 + supplementary)

- Title + Summary, Why, Design Principles, On-Disk Format, Data Model, CLI (N/A justified), JSON Output Format (N/A justified), Concurrency Model (N/A justified), Migration (N/A justified), Integration, What It Does NOT Do, Tech Stack, Project Infrastructure, Estimated Size — ALL PRESENT.
- Execution sections 15–19 — Task Manifest, Dependency Graph, Target State, Verification Plan, Success Criteria — ALL PRESENT and machine-checkable.
- Supplementary sections — Agent Assignments, Execution Order, Failure Modes, Open Questions, Domain-Specific Reference, Knowledge Growth Protocol, Gotchas — ALL PRESENT.

### [PASS] Three pinned rules port cleanly

1. **PINNED ORCHESTRATION RULE** — names `cmdr_coder`, bans `unix-coder`/`golang-coder`/`general-purpose`/`claude-agent`/`code-review`/`review-all` for code edits in this tree. Carve-out for `spec-builder`/`spec-reviewer` preserved. Carve-out added for Rust `plugins/focus-watcher/` (still routed to `cmdr_coder` despite Go foundation) — this is a sensible adaptation.
2. **REVIEWER INDEPENDENCE RULE** — same shape as icarus: separately-spawned instance, no shared context, two distinct Task calls, reviewer receives only diff/spec/aid/git. Rationale preserved verbatim.
3. **SPEC LAYOUT RULE** — adapted: lowercase `specs/` is the legacy here (vs. icarus's `specs/`), uppercase `SPEC/<spec_name>/` is canonical going forward. Legacy is FROZEN, not migrated, with a follow-up open question (#4) flagged for the index.md pointer.

### [PASS] Orchestrator-injects-context behavior is preserved AND elevated

The spec correctly carries over the icarus_coder behavior of consulting ob1 + git history, AND elevates orchestrator pre-injection from an implicit step to an explicit Integration contract (§10 "Orchestrator-Injected Context"). The activity-entry shape adds `injected_by_orch: boolean` and `orchestrator_payload: string[]` — these are NEW fields not in the icarus spec, which is appropriate given the user's explicit emphasis on this behavior in the prompt. The data model extension is clean.

### [PASS] Tooling adapted to computeCommander stack

- `gopls` retained for Go (correct — project IS Go).
- `tree-sitter-go` retained for Go AST.
- `rust-analyzer` and `tree-sitter-rust` ADDED for `plugins/focus-watcher/` — correct adaptation since computeCommander has a Rust subtree icarus does not.
- Make targets correctly enumerated from the actual Makefile (`build`, `build-focus-watcher`, `build-bridge`, `test`, `vet`, `lint`, `install`, `install-bridge`, `generate-types`, `clean`).
- No `parity harness` / `httprr` / `langchaingo` / `JSONL session` carry-over from icarus. Content discipline maintained.

### [PASS] Cardinal rule adapted

icarus's "parity harness must stay green" → computeCommander's "build must stay green AND focus-watcher must keep building". Concrete, testable, project-appropriate. The Rust dependency is called out in §10 Tech Stack, §13 Make Targets, the Gotchas section, AND the Failure Modes table.

### [PASS] Cross-project references include icarus

`icarus` is correctly listed as a sister project (5 total: claude-code, pi-mono, monty, crush, icarus). The "what to learn" line is detailed: structural template source, divergence-analysis reference, and an explicit "do NOT carry icarus assumptions" warning.

### [PASS] Color choice

`teal` is chosen, distinct from icarus's `amber`. Per the supervisor's prompt directive to pick a non-amber color (suggested teal/cyan/violet), this is compliant.

### [PASS] Task manifest is internally consistent

- 5 tasks (T1–T5), agents from the known roster (`claude-agent`, `unix-coder`, `spec-reviewer`, `cmdr_coder`).
- Dependency graph is acyclic: T1 → {T2, T3} → T4 → T5.
- No two tasks write to the same file (T1: `~/.claude/agents/cmdr_coder.md`; T2: `CLAUDE.md`; T3: 5 `agentic_instructions.md` files; T4: `REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md`; T5: nothing on disk).
- Every task has a non-empty Verify Command exiting 0 on success.
- Target State (§17) matches Task Manifest write-scopes exactly. WIP file `.computecommander/scripts/tg-viz.html` correctly listed as out-of-scope.

### [PASS] Success Criteria are all shell-checkable

24 criteria, all `test` / `rg -q` / `cd ... && make ...` predicates. No subjective language. The EXEC NOTE correctly carves out the WIP file from the build/test gates.

### [PASS] Failure Modes table is comprehensive

12 failure rows including: missing color, missing sections, content drift from icarus, destroyed CLAUDE.md content, empty stubs, missing scope-anchor reference, missing T1 file, missing cargo, broken build, ob1 unreachable, missing gopls/tree-sitter, layout migration confusion, AND — uniquely — the orchestrator-injection contract violation.

---

## Warnings (non-blocking)

### [WARN-1] Open Question #3 — ob1 CLI flag (`--namespace` vs `--project`)

The spec consistently uses `ob1 ... --project computeCommander` while icarus uses `ob1 ... --namespace icarus`. If the actual ob1 CLI has only one flag, the agent file (T1) and stubs (T3) will need to substitute. The spec acknowledges this in Open Questions. **Recommendation:** before T1 runs, the orchestrator should verify the actual flag by running `ob1 --help` and patch the spec or the T1 prompt accordingly. Non-blocking because the activity-entry shape (project tag) is invariant either way.

### [WARN-2] T1 commit policy for `~/.claude/agents/cmdr_coder.md`

T1 writes outside the computeCommander repo. The spec correctly flags that the commit policy follows `~/.claude/`'s own conventions, but does not specify what those are. **Recommendation:** the parent workflow should confirm whether `~/.claude/` is a tracked git repo at all and, if so, whether T1's commit lands there or stays uncommitted. This is a parent-workflow concern and outside the spec's scope, but should be answered before T1 runs.

### [WARN-3] T2 PREPEND semantics

T2 is described as "PREPEND the verbatim three-rule block ... to the existing CLAUDE.md". The verify command checks `rg -q 'specs/' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md` to confirm the existing "Specs" section was preserved — but `specs/` is a substring that also appears in the SPEC LAYOUT RULE's text ("legacy lowercase `specs/` directory IS PRESENT"). The verify is therefore not a strong guarantee that the original content was preserved; it would pass even if the original was destroyed and only the prepended block survived. **Recommendation:** strengthen the verify to check for a unique sentinel from the original CLAUDE.md, e.g., `rg -q 'Update .*specs/index\.md' /home/n0ko/Programs/ai/computeCommander/CLAUDE.md`. Cosmetic — does not block the spec.

### [WARN-4] T3 root-file update is a 5th-of-5 with different semantics

T3 mixes one UPDATE (root `agentic_instructions.md`) with four CREATE-NEW operations. The verify command treats them uniformly (`test -s` on all five). The root file already exists (~7.5 KB) so the `test -s` check is trivially met regardless of whether the new "cmdr_coder scope anchor" section was actually appended. The spec adds a secondary `rg -q 'cmdr_coder' /home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` check, which IS sufficient. **Recommendation:** none required — the secondary check carries the load. Noting for clarity.

### [WARN-5] `make lint` is best-effort but appears in agent body MUST-RUN list

The spec correctly notes `make lint` is best-effort (golangci-lint may be missing). However, the canonical Workflow Protocol step 7 says "run `make build`, then `go test ./...`, then `make vet`". Lint is omitted from the canonical workflow but mentioned elsewhere. **Recommendation:** confirm in T1 that the agent file's Workflow Protocol matches: `make build` → `go test ./...` → `make vet` (NOT `make lint`). The agent should optionally run `make lint` and treat its absence as informational. Already handled correctly in the spec text; flagging so T1 doesn't accidentally elevate lint to a gate.

---

## Summary

| Dimension | Status |
|-----------|--------|
| Structure (sections 1–19 + supplementary) | PASS |
| Three pinned rules port | PASS |
| Orchestrator-injects-context behavior | PASS (extended cleanly) |
| Tooling adaptation (gopls, tree-sitter, +rust-analyzer) | PASS |
| Cardinal rule adapted | PASS |
| Cross-project refs include icarus | PASS |
| Color choice (teal, non-amber) | PASS |
| Task Manifest + Dependency Graph + Target State coherence | PASS |
| Success Criteria machine-verifiable | PASS |
| Content discipline (no icarus carry-over) | PASS |

**Verdict: PASS WITH WARNINGS.** Five cosmetic / clarification warnings; none block commit. Proceed to Phase 3 (commit) per the parent workflow plan.
