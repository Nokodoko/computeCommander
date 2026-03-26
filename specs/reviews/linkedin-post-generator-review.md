# LinkedIn Post Generator -- Spec Review (v2)

**Spec:** `specs/linkedin-post-generator.md`
**Reviewer:** spec-reviewer (automated)
**Date:** 2026-03-26
**Verdict:** READY FOR IMPLEMENTATION -- all blockers resolved, all findings addressed

---

## 1. Summary

The spec has been updated to resolve the single blocker (systemd-vs-Claude-Code session paradox) and all minor consistency issues from the v1 review. The spec is now fully consistent, complete, and ready for Phase 1 implementation.

---

## 2. Changes Since v1 Review

| Finding | Status | Resolution |
|---------|--------|------------|
| **BLOCKER: systemd-vs-session paradox** | RESOLVED | Section 8 now documents two approaches: Phase 1 uses `claude -p` headless session (simple, works today), Phase 2 uses openbrain async proxy (decoupled, fleet-aware). The `claude -p` approach correctly provides access to both Claude LLM and Gmail MCP tools within the systemd-triggered session. |
| **Trend Analyzer phase label** | RESOLVED | Section 3 component table now says "(Phase 1)". Section 7 workflow step [2] updated to remove "(phase 2)" tag. Section 8 Phase 2 no longer lists RSS feed parser (moved to Phase 1 file list where it belongs). |
| **Missing CLI subcommands** | RESOLVED | Section 8 CLI command list now includes `cc linkedin approve <id>`, `cc linkedin reject <id>`, and `cc linkedin stats`. These align with references in Sections 10 (email template actions) and 11 (monthly summary). |
| **trustgraph location TBD** | RESOLVED | Section 4 now references "Docker Compose stack on primary host" with a link to the integration plan at `/home/n0ko/openbrain/PLAN-trustgraph-memory-integration.md`. Description expanded to include GraphRAG, DocumentRAG, and Context Cores. |

---

## 3. Completeness (Re-check)

| Section | Status | Notes |
|---------|--------|-------|
| 1. Why | Complete | No changes needed |
| 2. Design Principles | Complete | No changes needed |
| 3. Architecture | Complete | Trend Analyzer phase label corrected to Phase 1 |
| 4. Content Sources | Complete | trustgraph location resolved with Docker Compose stack + integration plan reference |
| 5. First Post (Rayne) | Complete | No changes needed |
| 6. Topic Queue | Complete | No changes needed |
| 7. Workflow | Complete | Step [2] trending reference updated to remove stale phase tag |
| 8. Implementation Plan | Complete | Blocker resolved. CLI commands expanded. Cron section now documents `claude -p` (Phase 1) and openbrain proxy (Phase 2). Phase 2 section updated. |
| 9. Style Guide | Complete | No changes needed |
| 10. Email Template | Complete | approve/reject commands now defined in Section 8 CLI list |
| 11. Feedback Mechanism | Complete | `cc linkedin stats` now defined in Section 8 CLI list |
| 12. Resolved Questions | Complete | No changes needed |
| 13. Dependencies | Complete | No changes needed |
| 14. Success Metrics | Complete | No changes needed |
| 15. Non-Goals | Complete | No changes needed |

---

## 4. Consistency (Re-check)

All cross-references verified:

- **Trend Analyzer**: Phase 1 in component table (Section 3), Phase 1 file list (Section 8), Phase 1 dependencies (Section 13), workflow step [2] (Section 7). All aligned.
- **CLI commands**: Section 8 now lists 8 subcommands: generate, preview, approve, reject, feedback, history, topics, stats. All references in Sections 10, 11, and the email template match.
- **Delivery mechanism**: Gmail MCP consistently referenced throughout. No stale SMTP references.
- **Content generation**: Consistently described as running inside Claude Code session. The cron section now explicitly shows how systemd triggers this session via `claude -p`.
- **trustgraph**: Location is concrete in Section 4. Topic #6 references are consistent.
- **Phase 2 scope**: Now covers LinkedIn API + openbrain proxy (no longer duplicates RSS trend analysis which is Phase 1).

---

## 5. Correctness (Re-check)

### Blocker Resolution Assessment

The `claude -p` approach is sound:
- `claude -p "<prompt>"` runs a non-interactive Claude Code session that exits on completion
- The headless session has the same MCP tool access as an interactive session (Gmail tools included)
- The Max account provides LLM access without API keys
- `WorkingDirectory` is set correctly so the session can find project files
- The prompt instructs Claude to run the `cc linkedin generate` pipeline, maintaining the Go-native scanning/structuring while delegating generation/delivery to Claude

The openbrain async proxy (Phase 2) is correctly scoped as an enhancement, not a requirement. It depends on the trustgraph+openbrain integration plan progressing, which is a reasonable dependency chain.

### Remaining Observations (non-blocking)

1. **`claude -p` permissions**: The service uses `claude -p` without `--dangerously-skip-permissions`. This means the headless session may prompt for tool-use confirmations. In a non-interactive systemd context, this could hang. The implementation should either use `--dangerously-skip-permissions` or configure a `.claude/settings.json` allowlist for the Gmail MCP tools. This is an implementation detail, not a spec gap.

2. **Email template `{post_id}`**: The template uses `{post_id}` while SQLite uses integer `id`. This is a rendering concern -- the implementation will substitute the integer. Not a spec issue.

3. **Topic #4 dual source**: "openbrain / Claude hooks" as source project is fine -- the scanner can read from both paths. No change needed.

---

## 6. Risk Assessment (Updated)

| Risk | Likelihood | Impact | Mitigation | Status |
|------|-----------|--------|------------|--------|
| systemd cannot access Claude Code session | **Resolved** | N/A | `claude -p` headless session | Fixed in spec |
| Gmail MCP tools unavailable in cron mode | **Resolved** | N/A | Headless session has MCP access | Fixed in spec |
| `claude -p` hangs on permission prompt | Low | Delays cron execution | Use `--dangerously-skip-permissions` or settings allowlist | Implementation detail |
| Rayne confidentiality leak | Low | Reputational/employment risk | Explicit constraints + human approval gate | Unchanged |
| Post quality insufficient | Medium | Low ROI | Feedback loop + style guide | Unchanged |
| overstory misattribution | Low | Reputational risk | Fork disclaimer is explicit | Unchanged |

---

## 7. Verdict

**READY FOR IMPLEMENTATION.** All blockers resolved. All consistency issues fixed. The spec is complete, internally consistent, and provides clear implementation guidance for Phase 1. No further spec work required before coding begins.
