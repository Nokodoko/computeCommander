# CLAUDE.md — computeCommander

## PINNED ORCHESTRATION RULE — SUPERSEDES global CLAUDE.md

**All code authorship, modification, refactoring, and code review for `/home/n0ko/Programs/ai/computeCommander` MUST be delegated to the `cmdr_coder` agent.**

- DO NOT use `unix-coder` for any work in this tree.
- DO NOT use `golang-coder` for any work in this tree. (`cmdr_coder` is the project-specific fork.)
- DO NOT use `general-purpose` / `claude-agent` / `code-review` / `review-all` for code edits in this tree.
- DO use `cmdr_coder` exclusively.

Spec authoring (`spec-builder`), spec review (`spec-reviewer`), exploration, and non-Go documentation work are NOT constrained by this rule. Rust edits to `plugins/focus-watcher/` are also dispatched to `cmdr_coder` — the agent foundation is `golang-coder` but its scope-lock includes the Rust plugin subtree because the focus-watcher protocol is part of the project's contract.

## REVIEWER INDEPENDENCE RULE — SUPERSEDES self-review

**The `cmdr_coder` instance that wrote or edited code MUST NOT review its own work. All code reviews in this tree MUST be performed by a SEPARATELY SPAWNED `cmdr_coder` instance — same agent definition, distinct Task invocation, no shared conversation or context with the author.**

- DO NOT chain a review prompt onto the authoring instance's session.
- DO NOT pass the author's reasoning, scratch notes, or in-flight conversation to the reviewer.
- DO issue two distinct `Task(subagent_type="cmdr_coder")` calls — one for authorship, one for review.
- The reviewer instance receives ONLY: (a) the diff or files-under-review, (b) the spec / requirements, (c) relevant `agentic_instructions.md` files, (d) git history (orchestrator-injected).
- Self-review attempts (same instance acting on its own diff) are a routing violation; the orchestrator MUST reject and re-route to a fresh instance.

**Rationale:** confirmation bias and shared context defeat review. A clean-context reviewer reproduces the user's perspective and catches defects the author rationalized.

## SPEC LAYOUT RULE — SUPERSEDES `specs/` (lowercase) layout

**All spec-related files for this repo MUST live under `/home/n0ko/Programs/ai/computeCommander/SPEC/<spec_name>/` (singular `SPEC`, per-spec subdirectory).**

- `<spec_name>` is the spec's canonical short identifier (e.g., `CMDR_CODER_AGENT`, `SESSION_PERSISTENCE`).
- Inside `SPEC/<spec_name>/` the layout is:
  - `<spec_name>.md` — the authoritative spec document.
  - `REVIEWS/<spec_name>_REVIEW.md` — spec-of-spec review(s).
  - `REVIEWS/<spec_name>_FILE_REVIEW.md` — review(s) of artifacts produced by the spec.
  - `phase*.md` (lowercase `phase` prefix) — phase-specific plans, checklists, and reviews.
  - Any other spec-scoped artifacts (diagrams, ADRs, triage notes) live in subfolders here, NOT at the repo root.
- Authors MUST NOT scatter `phase*`, `*_SPEC.md`, or `*_REVIEW.md` files at the repo root or in unrelated directories.
- Orchestrators routing spec work MUST create the `SPEC/<spec_name>/` directory if absent.
- This rule SUPERSEDES the prior `specs/` (lowercase) layout described in the existing CLAUDE.md "Specs" section below.
- The legacy lowercase `specs/` directory IS PRESENT and contains historical specs (`computecommander-v1.md`, `linkedin-post-generator.md`, `multi-agent-tracking.md`, etc.). It is FROZEN — `SPEC/<spec_name>/` is canonical going forward; do NOT migrate existing `specs/*` files in this rule's introduction commit.

---

# Project Rules

## Specs

All specification files live in `specs/`. Never create spec files in the repo root.

- Name specs with a descriptive identifier: `<topic>.md` (e.g., `session-naming.md`, `dashboard-v2.md`)
- Reviews, feedback, and validation artifacts go in `specs/reviews/`, prefixed with the spec they reviewed: `reviews/<topic>-review.md`, `reviews/<topic>-validation-errors.md`
- Update `specs/index.md` when adding or removing specs
