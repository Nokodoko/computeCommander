# CMDR_CODER_AGENT — File Review

**Subject:** `/home/n0ko/.claude/agents/cmdr_coder.md`
**Reviewed against:** `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`
**Reviewer:** spec-reviewer (T4 of CMDR_CODER_AGENT rollout)
**Date:** 2026-04-26
**File size:** 356 lines, ~37 KB

---

## Verdict

**PASS WITH WARNINGS** — file satisfies every machine-verifiable success criterion in spec §19, includes all 12 mandated body sections (§4), reproduces the 12 Design Principles (§3) faithfully, embeds the full TypeScript-style activity-entry schema (§6) including the load-bearing `injected_by_orch` and `orchestrator_payload` fields, documents the orchestrator-injection contract (§10), pins the three rules (PINNED ORCHESTRATION, REVIEWER INDEPENDENCE, SPEC LAYOUT) verbatim, and honors all 5 cosmetic warnings from `CMDR_CODER_AGENT_REVIEW.md`. Two minor cosmetic warnings carried forward (see §3 below); none are blocking.

---

## 1. Conformance Matrix

### Frontmatter (spec §4)

| Field | Required | Found | Status |
|-------|----------|-------|--------|
| `name` | `cmdr_coder` | `cmdr_coder` | OK |
| `description` | per spec text (mentions computeCommander, golang-coder fork, ob1, gopls, tree-sitter, scope-lock to repo) | matches; adds Rust focus-watcher mention | OK |
| `tools` | `Read, Write, Edit, Bash, Grep, Glob, LSP` | `Read, Write, Edit, Bash, Grep, Glob, LSP` | OK |
| `model` | `claude-opus-4-7` | `claude-opus-4-7` | OK |
| `color` | `teal` | `teal` | OK |

### Body sections (spec §4)

| Section | Present | Notes |
|---------|---------|-------|
| Mission | yes | scope-locked statement; ob1/git/AST/LSP grounding; build-green cardinal |
| Working Environment | yes | module path, Go version, branch (`pi`), pre-existing WIP callout, ob1 project tag, per-directory anchors |
| Architecture Overview | yes | full directory tree mirroring spec §13, `pkg/` lock noted |
| Tooling | yes | `rg`/`fd`, gopls, rust-analyzer, tree-sitter-go/-rust, ob2 with `-project` flag, make targets, lint-best-effort |
| Workflow Protocol | yes | 11-step canonical sequence per spec §-Domain-Specific-Reference; activity-entry schema with all 16 fields |
| Project Rules | yes | all 12 Design Principles reproduced; "Out of scope (refuse)" subsection |
| Coding Conventions | yes | Go + Rust; Cobra one-per-file; AgentRuntime adapter pattern; dual-DB invariant; KDL pickiness |
| Make Targets | yes | full table with note that `make build` requires cargo and `make lint` is best-effort |
| Success Criteria | yes | `# Outcome` block enumerating all gates |
| Knowledge Growth Protocol | yes | LEARNINGS.md format, scope-locked |
| Gotchas | yes | 18 entries covering Rust dependency, race-flag default, lint best-effort, module path, Go version, rust-analyzer optionality, Cobra, AgentRuntime, dual-DB, KDL, branch `pi`, WIP, legacy specs/, k8s-cluster, cwd reset, reviewer independence, orchestrator pre-injection contract, CLAUDE.md edit-discipline, ob1/ob2 naming reconciliation |
| Cross-Project References | yes | all 5 sister projects (claude-code, pi-mono, monty, crush, icarus) with explicit "do NOT carry icarus assumptions" warning |

### Design Principles 1–12 (spec §3)

All 12 reproduced verbatim or as faithful one-line reductions. Spot-checked:
- #1 scope-lock statement matches refusal text
- #3 names all 7 load-bearing interfaces (AgentRuntime, DB, MailStore, MergeQueue, WorktreeManager, PaneManager, WindowManager)
- #6 specifies `project=computeCommander` namespace (not `--namespace icarus` carryover)
- #11 cardinal correctly cites both `make build` and the cargo focus-watcher build
- #12 reviewer-independence text matches; agent self-refusal phrase included verbatim

### Activity-entry schema (spec §6)

All 16 fields present in the embedded TypeScript interface block:
`task_id`, `agent`, `project`, `session_id`, `ts`, `ob1_keys_read`, `git_revs_read`, `injected_by_orch`, `orchestrator_payload`, `files_touched`, `packages`, `commit_sha`, `outcome`, `outcome_detail`, `ast_queries`, `gopls_calls`. Both load-bearing fields (`injected_by_orch`, `orchestrator_payload`) are explicitly called out in their own paragraph as orchestrator-contract enforcement signals.

### Three pinned rules (spec §4)

| Rule | Present | Verbatim or reduction |
|------|---------|------------------------|
| PINNED ORCHESTRATION RULE | yes | reduction faithful; supersedes language matches |
| REVIEWER INDEPENDENCE RULE | yes | reduction faithful; "two distinct Task calls" shape preserved |
| SPEC LAYOUT RULE | yes | reduction faithful; legacy `specs/` frozen-not-migrated language preserved |

### Five cosmetic warnings from `CMDR_CODER_AGENT_REVIEW.md`

| # | Warning | Honored | How |
|---|---------|---------|-----|
| W1 | Use exact ob CLI flag spelling | yes | `-project <path>` (single-dash) used throughout; ob2 binary path noted; ob1 vs ob2 naming reconciled in Gotchas |
| W2 | Specify CLI subcommand list | yes | Tooling table enumerates serialize/refresh/search/gate/list/status/show |
| W3 | Use unique-sentinel approach for CLAUDE.md edits | yes | Gotcha entry "CLAUDE.md edit-discipline" names the sentinel `Update specs/index.md when adding or removing specs` |
| W4 | rust-analyzer optionality | yes | Tooling table marks rust-analyzer OPTIONAL; Gotcha repeats; plugins/agentic_instructions.md echoes |
| W5 | `make lint` is best-effort, NOT a gate | yes | Tooling table, Make Targets table, and a dedicated Gotcha all state best-effort + not-a-gate |

### Spec §19 machine-verifiable checklist (file-scope subset)

| Check | Result |
|-------|--------|
| `test -f /home/n0ko/.claude/agents/cmdr_coder.md` | OK (file exists) |
| `rg -q '^name: cmdr_coder'` | OK |
| `rg -q '^color: teal'` | OK |
| `rg -q 'tree-?sitter'` | OK |
| `rg -q 'gopls'` | OK |
| `rg -q 'ob1'` | OK |
| `rg -q 'computeCommander'` | OK |
| `rg -q 'tools:.*LSP'` | OK |
| `rg -q 'model: claude-opus-4-7'` | OK |

All file-scope success criteria from §19 exit 0.

---

## 2. Negative Checks (icarus-content carryover)

The spec's "Port Notice" forbids carrying over icarus-specific content (langchaingo, parity harness, JSONL session, sidecar, `Nokodoko/...` module paths). Audited:

- `langchaingo` — appears ONCE, in the Cross-Project References row for `icarus`, in the explicit warning "Do NOT carry icarus assumptions (langchaingo, parity harness, JSONL session, sidecar) into computeCommander." This is a contextual *anti-pattern callout*, not carryover. ACCEPTABLE.
- `parity harness` — same row, same anti-pattern callout. ACCEPTABLE.
- `JSONL session` — same row, same anti-pattern callout. ACCEPTABLE.
- `sidecar` — same row, same anti-pattern callout. ACCEPTABLE.
- `icarus_coder` references — appear in (a) "Out of scope (refuse)" forbidding edits to the sister agent file, and (b) Cross-Project References row. Both correct. ACCEPTABLE.
- Module path — `github.com/noko/computecommander` everywhere; no `Nokodoko/...` slipped in. OK.

No carryover violations.

---

## 3. Findings

### Findings — informational (not blocking)

| # | Severity | Finding | Recommendation |
|---|----------|---------|----------------|
| F1 | info | The agent file embeds Spec §10's orchestrator-injection contract well, but the example prompt-envelope YAML (spec §10's `task_id / file_scope / read_scope / prior_ob1_entries / git_context / success_criteria` block) is NOT reproduced in the agent file. The agent is left to infer envelope shape from the prose. | OPTIONAL: in a follow-up, add an "Orchestrator Prompt Envelope (Expected Shape)" subsection under Workflow Protocol, copying the YAML block from spec §10. Not required for PASS. |
| F2 | info | The `Tooling` table's ob CLI row says "ob2 subcommands: serialize, refresh, search, gate, list, status, show (no `capture`)". Spec §10's Agent-facing commands block uses `ob1 read --project <name> --key <key>` and `ob1 list --project <name> --prefix <prefix>`. The agent file correctly notes that ob2 lacks a `capture` subcommand and recommends `search`/`list`. However, the spec writer may want to update the spec itself to match ob2 reality (separate task; out of scope here). | OPTIONAL: file a follow-up to reconcile spec §10's ob1 invocation examples with the installed ob2 CLI's flag/subcommand surface. Out of scope for THIS file review. |
| F3 | info | Workflow Protocol step 9 (ob1 write) gracefully degrades to "fall back to writing the entry to the activity directory and surface it for the orchestrator to ingest" if ob2 lacks a write subcommand on this host. ob2's `serialize` / `refresh` are project-index commands, not entry-write commands. The fallback is a reasonable defensive posture. | OPTIONAL: clarify in a follow-up whether ob2 has an explicit entry-write subcommand on this host (it does not, per `ob2 --help`). The agent's defensive fallback is correct as written. |

### Findings — none blocking

No FAIL findings. No PASS-with-conditions findings beyond the three informational items above.

---

## 4. Verdict Summary

**PASS WITH WARNINGS.**

- All §19 file-scope success criteria pass.
- All 12 §4 body sections present.
- All 12 §3 Design Principles reproduced.
- All 16 §6 activity-entry fields present, including both load-bearing fields (`injected_by_orch`, `orchestrator_payload`).
- All three pinned rules (PINNED ORCHESTRATION, REVIEWER INDEPENDENCE, SPEC LAYOUT) embedded.
- All 5 cosmetic warnings from the parent spec review honored.
- No icarus-content carryover violations.
- Three informational findings (F1–F3) — all OPTIONAL follow-ups, none blocking commit.

**Recommendation:** Proceed to Phase 4 (commit T1+T2+T3 outputs) and Phase 5 (T5 self-test).
