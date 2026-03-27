# Spec Review Report

| Field | Value |
|-------|-------|
| Spec | specs/multi-agent-tracking.md |
| Reviewed | 2026-03-17T12:00:00Z |
| Iteration | 2 of 3 |
| Verdict | **PASS WITH WARNINGS** (0 critical, 5 warnings, 4 info) |

## Summary

All 5 critical findings from iteration 1 have been addressed. The migration is correctly numbered 008, `ListOpts`/`ListSessions` modifications are covered in T4's write-scope, FeedCmd restructuring is explicitly described in T9, `types.go` and `spawner.go` appear in Target State, and migration naming is consistent throughout. The remaining findings are warnings and informational items that do not block execution.

## Iteration 1 Critical Findings — Resolution Status

| Finding | Status | Evidence |
|---------|--------|----------|
| CORR-001: Migration numbering (002 -> 008) | RESOLVED | All references now use `008_multi_agent.sql`. T1 verify command chains all 7 prior migrations before 008. |
| CORR-002: ListOpts missing Runtime field | RESOLVED | Data Model section shows `Runtime runtimes.RuntimeID` in ListOpts. T4 write-scope includes `types.go` and `spawner.go`. |
| CORR-003: FeedCmd restructuring | RESOLVED | T9 description explicitly states: "Restructure FeedCmd to support subcommands...refactor it into a parent command (with default RunE for backwards-compatible behavior) and add `feed emit` as a child subcommand." |
| CONS-001: types.go/spawner.go missing from scopes | RESOLVED | Both files appear in T4 write-scope and Target State "Files modified" list. |
| CONS-003: Migration name inconsistency | RESOLVED | Global rename to `008_multi_agent.sql` applied across all sections. |

## Dimension Results

| Dimension | Status | Findings | Critical |
|-----------|--------|----------|----------|
| completeness | PASS | 1 | 0 |
| clarity | PASS | 1 | 0 |
| correctness | PASS | 2 | 0 |
| consistency | PASS | 2 | 0 |
| sdlc | PASS | 1 | 0 |
| actionability | PASS | 2 | 0 |
| rebuild fidelity | N/A | 0 | 0 |

## Findings

### Dimension 1: Completeness (COMP)

### COMP-001 [INFO] — Missing explicit evals table schema reference
**Section:** Data Model
**Issue:** The spec references `cmdr evals emit` writing to the evals table but does not note which migration defines it. The evals schema lives in `004_evals.sql`. An implementer unfamiliar with the project would need to discover this independently.
**Suggestion:** Add a one-line note in the Migration section: "The `evals` table is defined in `004_evals.sql`. No schema changes needed for evals; the `cmdr evals emit` changes are purely CLI/documentation."

---

### Dimension 2: Clarity (CLAR)

### CLAR-001 [WARNING] — Heartbeat throttle "max 1 event per 5 min per agent" has no implementation detail
**Section:** Integration > Event Feed Pane Integration
**Issue:** The spec states "cmdr heartbeat emits an agent.heartbeat event (throttled: max 1 event per 5 min per agent to avoid noise)" but does not specify where or how this throttling is implemented. Is it in `register.go`'s heartbeat handler? A SQL `WHERE NOT EXISTS` check? An in-memory timestamp cache? Without this detail, the implementer must design the throttling mechanism from scratch, risking divergent approaches (T10 implementer vs T8 test writer).
**Suggestion:** Add one sentence specifying the mechanism, e.g.: "Throttling is implemented in the heartbeat handler in `register.go` via a SQL check: `SELECT 1 FROM events WHERE session_id = ? AND event_type = 'agent.heartbeat' AND created_at > datetime('now', '-5 minutes')`; if a row exists, skip the INSERT."

---

### Dimension 3: Correctness (CORR)

### CORR-001 [WARNING] — `AgentSession` struct missing `HeartbeatAt` field
**Section:** Data Model
**Issue:** The spec's Data Model defines `heartbeat_at?: string` in the TypeScript interface and the migration adds a `heartbeat_at` column, but the existing Go struct `AgentSession` in `internal/agents/types.go` has no `HeartbeatAt` field. No task in the Task Manifest adds this field to the Go struct. T4 modifies `types.go` to add `Runtime` to `ListOpts`, but adding `HeartbeatAt` to `AgentSession` is not mentioned anywhere.
**Suggestion:** Add `HeartbeatAt *time.Time` to the `AgentSession` struct modification scope in T4 (or T1/T2). Also update `ListSessions` scan columns in `spawner.go` to include `heartbeat_at`. Without this, `cmdr heartbeat` can write the column but `cmdr status` cannot read it back.

### CORR-002 [WARNING] — `ListSessions` SELECT does not include `heartbeat_at`
**Section:** Data Model > ListOpts
**Issue:** The spec shows the runtime WHERE clause addition to `ListSessions` but does not address that the SELECT column list and `rows.Scan()` in `spawner.go` must also be updated to include `heartbeat_at`. The existing code has a `hasV2` detection pattern with two separate scan paths (lines 258-271, 322-348). Adding `heartbeat_at` requires updating both scan paths or adding a `hasV3` check.
**Suggestion:** Note in T4's description that `ListSessions` needs both the WHERE clause for runtime AND the SELECT/Scan for `heartbeat_at`. Alternatively, if `heartbeat_at` is only needed by the heartbeat command (not status display), document that explicitly so the implementer knows not to add it to the scan.

---

### Dimension 4: Consistency (CONS)

### CONS-001 [WARNING] — Estimated Size total is 930 LOC but sum of rows is ~910
**Section:** Estimated Size
**Issue:** The table rows sum to approximately 910 LOC (250+20+120+40+25+80+80+20+30+15+250), but the total states "~930". This is minor but may confuse an estimator.
**Suggestion:** Update the total to match the actual sum, or add the ~20 LOC gap to an existing row.

### CONS-002 [INFO] — Estimated Size file count says 14 but table has 11 rows
**Section:** Estimated Size
**Issue:** The table has 11 data rows (some with 0 files for "Register event emission"), and the total says 14 files. Counting the non-zero file entries: 1+2+1+1+2+1+1+1+0+1+2 = 13. Target State lists 5 created + 9 modified = 14. The mismatch is because "Register event emission" has 0 files (it's part of register.go) and the test row counts 2 but Target State has 2 test files (register_test.go and openbrain_test.go).
**Suggestion:** Update the Estimated Size file count column to align with Target State, or add a footnote explaining that some modifications are counted in other rows.

---

### Dimension 5: SDLC Alignment (SDLC)

### SDLC-001 [INFO] — Success Criteria are well-formed but some are structural checks only
**Section:** Success Criteria
**Issue:** Several criteria use `grep -q` to check for string presence (e.g., `grep -q 'agent.registered' internal/commands/register.go`). These are `contains_pattern` predicates that verify the string exists in source code but not that the functionality works. The integration check pipeline at the end is more robust.
**Suggestion:** No change required -- the integration check covers functional validation. The grep checks serve as fast structural gates during per-task verification.

---

### Dimension 6: Actionability (ACTN)

### ACTN-001 [WARNING] — T5 testing note is helpful but T5's verify command does not exercise agent events
**Section:** Task Manifest
**Issue:** T5 description correctly notes "functional testing of live agent events depends on T10" and that "T8 unit tests should use pre-seeded DB rows." However, T5's verify command is only `grep -qE 'agents|no-agents'` in help output. If the Activity section rendering has bugs, T5 will still pass. T8 catches this, but there is a gap window between T5 completion and T8 execution where broken rendering goes undetected.
**Suggestion:** Consider adding a build-only verify to T5: `go build ./cmd/cc && go vet ./internal/commands/` to at least catch compile errors in the OpenBrain changes.

### ACTN-002 [INFO] — Open questions all have suggested defaults -- consider marking resolved
**Section:** Open Questions
**Issue:** All 3 open questions have "Suggested Default" values that are reasonable and non-blocking. These are effectively resolved decisions.
**Suggestion:** Mark as "Resolved" with the suggested defaults to make it explicit for implementers.
