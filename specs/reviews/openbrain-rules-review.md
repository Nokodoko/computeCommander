# Spec Review Report

| Field | Value |
|-------|-------|
| Spec | `specs/openbrain-rules.md` |
| Reviewed | 2026-03-18T22:40:00Z |
| Iteration | 1 of 3 |
| Verdict | **PASS WITH ISSUES** (2 critical, 4 warnings, 3 info) |

## Summary

The spec is well-structured and addresses a real problem (noise in OpenBrain pane, no session-start reads). Schema design is clean, CLI API is practical, and the color coding system is well thought out. Two critical issues need resolution before implementation: (1) the `cmdr openbrain write` command has no deduplication strategy, and (2) the migration number 009 conflicts with potential concurrent spec work. Four warnings around missing error handling details, unclear project name derivation, incomplete test coverage spec, and a missing `--format` flag for read output.

## Dimension Results

| Dimension | Status | Findings | Critical |
|-----------|--------|----------|----------|
| completeness | WARN | 3 | 1 |
| clarity | PASS | 2 | 0 |
| correctness | WARN | 2 | 1 |
| consistency | PASS | 1 | 0 |
| sdlc | PASS | 1 | 0 |
| actionability | PASS | 0 | 0 |

## Findings

---

### Dimension 1: Completeness

### COMP-001 [CRITICAL] — No deduplication strategy for write command

**Section:** What Gets Written (Write Rules) > Write API
**Issue:** If an agent calls `cmdr openbrain write --type decision --summary "Switched to zellij"` twice (e.g., on retry, or two agents discover the same thing), two identical entries are inserted. With multiple agents running concurrently, duplicate knowledge entries will accumulate. The spec provides no guidance on dedup — no UNIQUE constraint, no upsert logic, no content hashing.
**Suggestion:** Add a dedup strategy. Options: (a) UNIQUE constraint on `(project_name, entry_type, summary)` with INSERT OR IGNORE, (b) content hash column with upsert, or (c) explicit guidance that duplicates are acceptable and will be handled by the prune command. Pick one and document it.

### COMP-002 [WARNING] — Missing error handling for write failures

**Section:** T2: `cmdr openbrain write` Subcommand
**Issue:** The task description says "Insert into openbrain_entries" and "Signal panes for refresh" but does not specify behavior when the DB is unavailable (e.g., local.db locked by another process, disk full). Should write fail silently (to avoid blocking agent work) or return a non-zero exit code? This matters because agents will call this from hooks/scripts.
**Suggestion:** Add explicit error handling policy: "Write failures MUST return non-zero exit code and print to stderr, but MUST NOT block agent execution. Callers should use `cmdr openbrain write ... || true` if they want fire-and-forget semantics."

### COMP-003 [INFO] — No mention of `cmdr openbrain list` or `cmdr openbrain delete`

**Section:** Implementation Tasks
**Issue:** The spec defines `write`, `read`, and `prune` subcommands but no way to list all entries interactively or delete a specific entry by ID. These are useful for debugging and manual curation. Not blocking — can be added later.
**Suggestion:** Consider adding `cmdr openbrain list [--all]` and `cmdr openbrain delete --id <N>` as future work items or explicitly note them as out of scope.

---

### Dimension 2: Clarity

### CLAR-001 [WARNING] — Project name derivation is underspecified

**Section:** Write API, Read API
**Issue:** Both commands say "defaults to cwd-based detection" and reference "reuse `find_cmdr_db` logic." But `find_cmdr_db` finds the database file, not a project name. How is the project name derived from cwd? Is it the directory basename? The git remote name? A `.computecommander/project.json` field? Different derivation strategies would produce different `project_name` values, fragmenting queries.
**Suggestion:** Specify exactly: "Project name is derived from the basename of the nearest parent directory containing `.computecommander/`, or if none found, the basename of the git root, or if no git root, the cwd basename."

### CLAR-002 [INFO] — "Signal panes for refresh" mechanism not specified

**Section:** T2
**Issue:** The task says "Signal panes for refresh" after write. The current codebase uses fsnotify on the DB file (see `watchDBFile` in `pane.go`). If the write goes to the same `local.db`, the existing fsnotify mechanism should auto-detect the change. But this isn't stated explicitly.
**Suggestion:** Add: "Pane refresh is automatic via existing fsnotify on `local.db`. No additional signaling mechanism needed — the SQLite write triggers a file change event."

---

### Dimension 3: Correctness

### CORR-001 [CRITICAL] — Migration number 009 may conflict

**Section:** Database Schema
**Issue:** The spec hardcodes "Migration 009" but the current codebase has migrations up to 008. If any other feature branch adds a migration before this lands, there will be a collision. Migration numbering should be determined at implementation time, not spec time.
**Suggestion:** Change to "Migration NNN" or "next available migration number" and note that the implementer must check the latest migration number at build time.

### CORR-002 [WARNING] — `expires_at` column defined but never set by write command

**Section:** Database Schema vs Write API
**Issue:** The schema includes `expires_at TEXT` and T8 mentions "auto-prune entries where `expires_at < now()`", but the `cmdr openbrain write` CLI has no `--expires-in` or `--ttl` flag. The column will always be NULL. Either remove it or add the flag.
**Suggestion:** Add `--ttl <duration>` flag to the write command (e.g., `--ttl 7d` sets `expires_at` to now + 7 days), or document that `expires_at` is reserved for future use and NULL means "never expires."

---

### Dimension 4: Consistency

### CONS-001 [WARNING] — Read output format inconsistency between text and JSON modes

**Section:** What Gets Read > Session Start Read
**Issue:** The text output example shows `[decision] 2h ago (claude) ...` but the JSON output is not specified. T3 says "Output in text mode (human-readable for context injection) and JSON mode" but only text is shown. The `context-inject.py` integration in T6 uses `--json` mode, so the JSON schema must be defined for the hook to parse it reliably.
**Suggestion:** Add a JSON output example:
```json
{
  "entries": [
    {"type": "decision", "summary": "...", "detail": "...", "runtime": "claude", "age": "2h", "created_at": "..."}
  ],
  "count": 4,
  "project": "computeCommander"
}
```

---

### Dimension 5: SDLC

### SDLC-001 [INFO] — Test task T7 is thin on edge cases

**Section:** T7: Tests
**Issue:** T7 lists four test cases but misses: (a) write with invalid entry_type, (b) read with no entries (empty result), (c) concurrent writes, (d) read with `--since` boundary (exactly at boundary), (e) prune with no expired entries. These are standard SQLite edge cases.
**Suggestion:** Expand T7 to include at minimum: invalid type rejection, empty result set, and boundary condition for `--since` filter.

## Next Steps

1. Address COMP-001 (dedup strategy) — pick an approach and document it
2. Address CORR-001 (migration number) — make it dynamic
3. Resolve the four warnings (error handling, project name derivation, expires_at usage, JSON schema)
4. Re-review after iteration
