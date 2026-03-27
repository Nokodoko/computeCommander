# Spec Validation Report

| Field | Value |
|-------|-------|
| Spec | `specs/openbrain-rules.md` |
| Validated | 2026-03-18T22:45:00Z |
| Result | **PASS** (0 errors, 2 advisories) |

## Structural Checks

| Check | Status | Notes |
|-------|--------|-------|
| H1 title present | PASS | `# OpenBrain Rules Overhaul` |
| Description line after title | PASS | One-line summary present |
| Why section | PASS | `## Why` with 3 enumerated problems |
| Design Principles | PASS | `## Design Principles` with 4 numbered principles |
| Data Model / Schema | PASS | `## Database Schema` with SQLite + Postgres |
| API / Interface definition | PASS | Write API and Read API fully specified with CLI syntax |
| Implementation Tasks | PASS | T1-T8 with estimates |
| Success Criteria | PASS | 5 testable criteria |
| Backwards Compatibility | PASS | Explicit section |
| On-Disk Format | PASS | File tree showing changed files |
| Color Coding | PASS | Runtime color map + type indicators |
| Pane Layout Changes | PASS | Current vs new layout documented |
| Agent Guidelines | PASS | Embedded markdown block for CLAUDE.md |
| Performance constraints | PASS | Table with 5 metrics and targets |
| Deduplication strategy | PASS | UNIQUE index + INSERT OR IGNORE |
| Error handling policy | PASS | Non-zero exit + stderr, never panic |
| Project name derivation | PASS | 3-level precedence chain documented |
| JSON output schema | PASS | Example JSON response for read --json |
| TTL support | PASS | --ttl flag + expires_at column |
| Test coverage spec | PASS | 11 test cases enumerated in T7 |

## Advisories

### ADV-001 — Why section says "two problems" but lists three

**Line:** 7
**Issue:** The text reads "OpenBrain currently has two problems:" but then enumerates three items (noise, absent reads, missing color coding). Minor prose inconsistency.
**Impact:** None — the content is complete, just the count word is wrong.
**Fix:** Change "two problems" to "three problems."

### ADV-002 — No explicit "Out of Scope" section

**Issue:** The spec does not have a dedicated "What It Does NOT Do" or "Out of Scope" section. Items like `cmdr openbrain list`, `cmdr openbrain delete`, multi-project cross-queries, and real-time streaming of knowledge entries are implicitly out of scope but not stated.
**Impact:** Low — the spec is clear about what it does, but an explicit out-of-scope list helps prevent scope creep during implementation.

## Summary

All critical structural elements are present. The spec addresses both review criticals (dedup strategy, migration numbering) and all four warnings (error handling, project name derivation, expires_at usage, JSON schema). Two minor advisories remain (prose count mismatch, missing out-of-scope section) — neither blocks implementation.
