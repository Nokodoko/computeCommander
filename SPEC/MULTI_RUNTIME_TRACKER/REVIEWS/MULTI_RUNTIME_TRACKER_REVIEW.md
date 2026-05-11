# Spec Review Report — MULTI_RUNTIME_TRACKER

| Field | Value |
|-------|-------|
| Spec | /home/n0ko/Programs/ai/computeCommander/SPEC/MULTI_RUNTIME_TRACKER/MULTI_RUNTIME_TRACKER.md |
| Reviewed | 2026-05-02T00:00:00Z |
| Iteration | 1 of 3 |
| Verdict | **REQUEST_CHANGES** (3 critical, 7 warnings, 4 info) |

## Summary

The spec is structurally complete (all 19 sections populated, 16 tasks with reviewer-independence checkpoints, dependency graph acyclic) and the seven open-question decisions are sound. However, three load-bearing technical claims are factually wrong against the codebase and would mis-route swarm execution: (1) icarus T8 is described as unmerged but is in fact committed to `~/Programs/ai/icarus` main as of `4bf3fb8`/`f7107fb`, which collapses the rationale for T2.5; (2) `internal/agents/reaper/` does not exist — the reaper lives in `internal/commands/status.go:185-232` and the spec proposes a tacit relocation without naming it; (3) `cmdr agent show <name>` is referenced as a verify command but no such subcommand exists today. These are mechanical fixes (re-cite reality and either repurpose T2.5 or delete it), so an iterative rebuild loop is appropriate.

## Dimension Results

| Dimension | Status | Findings | Critical |
|-----------|--------|----------|----------|
| completeness | PASS | 1 | 0 |
| clarity | PASS WITH WARNINGS | 2 | 0 |
| correctness | FAIL | 5 | 3 |
| consistency | PASS WITH WARNINGS | 3 | 0 |
| sdlc | PASS | 1 | 0 |
| actionability | PASS WITH WARNINGS | 2 | 0 |
| rebuild fidelity | N/A | 0 | 0 |

---

## Findings

### CORR-001 [CRITICAL] — icarus T8 is described as unmerged, but it is committed

**Section:** §2 Why; §10 Integration ("icarus T8 contract"); §15 T2.5 rationale; Open Questions Q3.

**Issue:** The spec states `~/Programs/ai/icarus/internal/integration/cmdr/ does not exist on disk` (§2) and that T8 (`internal/integration/cmdr/events.go` + `evals.go`) "ships from the icarus tree ... is not [merged]" (§2 last bullet) and "currently unmerged" (§10). Verified against the icarus repo:

- `~/Programs/ai/icarus/internal/integration/cmdr/` exists and contains `events.go` (22KB), `evals.go` (13KB), `tags.go`, `events_test.go`, `evals_test.go`, `integration_test.go`.
- `git log` shows `4bf3fb8 feat(cmdr): icarus-side emitters + ICARUS_EFFORT/ICARUS_MODEL env hooks + ob1 settings parse [T8]` and `f7107fb merge(cmdr): icarus-side event and eval emitters [T8]` on `main`.

This is the load-bearing premise behind T2.5 (icarus polling fallback) and a cited driver in §11 ("Does NOT add icarus T8 to this spec's task manifest. T8 ships from the icarus repo"). If T8 is in fact merged, the polling fallback's purpose collapses to either:

1. A **defensive secondary path** in case T8 emitters fail at runtime (legitimate but un-stated); or
2. A **dead task** that should be cut.

A reviewer cannot tell which from the current spec text, and a worker dispatched to T2.5 would discover the contradiction during read-scope investigation and stall.

**Suggestion:** Re-investigate icarus T8 merge status (`cd ~/Programs/ai/icarus && git log --oneline -- internal/integration/cmdr/`). Then EITHER:

(a) **Delete T2.5 and the polling fallback.** Update §2 bullet 3 to "icarus T8 emitter is merged; cmdr-side validation only required" and remove §10 "icarus T8 contract" polling-fallback paragraph. Re-run the dependency graph (T2.5 / T2.5R drop out, Phase 2 has 4 tasks instead of 5).

(b) **Keep T2.5 as a defensive fallback.** Rewrite §10 "icarus T8 contract" to: "T8 is merged at icarus@4bf3fb8. The poller (§15 T2.5) ships as a defensive secondary write path, env-gated `ICARUS_POLLER=1`, default OFF, used only when T8 emitters fail observable health checks. Remove after one stable release window." Cite the T8 commit SHA explicitly.

Option (a) reduces scope by ~220 LOC and one task; option (b) preserves the safety net but must be honest about why. Either is mechanical; user judgment is not required.

---

### CORR-002 [CRITICAL] — `internal/agents/reaper/` directory does not exist; reaper lives in status.go

**Section:** §2 (mentions `runStatusPane` reaps stale agents); §4 On-Disk Format (lists `internal/agents/reaper/reaper.go` as MODIFIED); §13 Project Infrastructure (same); §15 T2.4 (write-scope `internal/agents/reaper/reaper.go`); §17 Target State (Files modified).

**Issue:** Verified against the codebase:

- `ls internal/agents/reaper/` returns `No such file or directory`.
- The reaper is implemented in `internal/commands/status.go:185-232` (`runStatusPane` and the inline `reapStale` closure at line 215). The spec acknowledges this in §2 ("Reaper is `last_activity`-keyed. `runStatusPane` reaps stale agents using `last_activity`") but then in §4/§13/§15/§17 references `internal/agents/reaper/reaper.go` as a MODIFIED file, not a NEW one.

So the spec is implicitly asking for the reaper to be **extracted** from `runStatusPane` into a new package, but that extraction is never enumerated as work, the new file is mis-labeled "MODIFIED" instead of "NEW", and the read/write scope of T2.4 hides the refactor inside what reads as a localized change.

A worker assigned T2.4 would either (a) try to edit a non-existent file and fail, or (b) silently extract `reapStale` into a new package without explicit license to refactor the surrounding `runStatusPane` (which calls it via a closure with closed-over `app.DB`, `staleThreshold`, `ctx`). The latter is the larger surgery and needs explicit acknowledgement.

**Suggestion:** Pick one model and make it explicit:

(a) **Keep reaper in-place.** Rewrite §4, §13, §15 T2.4 to reference `internal/commands/status.go` (MODIFIED) and add inline tests. Remove the `internal/agents/reaper/` directory references. T2.4 verify command becomes `go test ./internal/commands/... -run TestReaper` (with a new test added).

(b) **Extract reaper into its own package** (cleaner architecture, easier to test runtime-aware logic). Add a new task T2.4a "Extract `reapStale` from `runStatusPane` into `internal/agents/reaper/reaper.go` with no behavioral change; rewire `runStatusPane` to call the new package." Make T2.4 (runtime-aware logic) depend on T2.4a. Add the `reaper.go` file as NEW (not MODIFIED) in §4/§13/§17.

The spec text strongly implies (b) but never says so. Pick one and write it down.

---

### CORR-003 [CRITICAL] — `cmdr agent show <name>` subcommand does not exist

**Section:** §15 T1.2 Verify Command (`cmdr agent show gemini` exits 0); §18 Verification Plan (same); §19 Success Criteria (`cmdr agent show gemini --json | jq -r .runtime`); §18 "agents/gemini.yaml parses as valid YAML" check.

**Issue:** Searched `internal/commands/` and `cmd/cc/main.go` — there is no `agent` cobra command, no `show` subcommand, and no `agentCmd` registration. The closest existing surfaces are:

- `cmdr status` (lists runtime sessions, not agent definitions).
- `internal/commands/agentic.go` (governs `agentic_instructions.md`, not agent YAML).

So the verify command for T1.2 is not runnable today, and the success criterion `cmdr agent show gemini --json | jq -r .runtime outputs gemini` would fail.

**Suggestion:** Either (a) add a task for the `cmdr agent show` subcommand (it's small — read `agents/<name>.yaml`, JSON-marshal, print) and make T1.2 depend on it; or (b) replace the verify command with something that exists today, e.g. `yq -r '.runtime' agents/gemini.yaml | grep -q '^gemini$'` and `yamllint agents/gemini.yaml`. Option (b) is a pure spec edit; option (a) adds T1.2-pre and ~50 LOC. Prefer (b) unless `cmdr agent show` is wanted for unrelated reasons.

---

### CORR-004 [WARNING] — Migration `012` skips `010_model_session_name.sql` neighbor and `011_linkedin.sql`

**Section:** §4 (`012_runtime_check.sql`); §9 Migration; §13 Project Infrastructure; §15 T1.4; §18 Verification Plan; §19 Success Criteria.

**Issue:** Verified migrations on disk: `001..009`, `010_model_session_name.sql`, `011_linkedin.sql`. The spec uses `012` (correct — next available number) but the verification snippet in §18 shows:

```bash
sqlite3 /tmp/test-cmdr.db < 001_schema.sql
sqlite3 /tmp/test-cmdr.db < 008_multi_agent.sql
sqlite3 /tmp/test-cmdr.db < 010_model_session_name.sql
sqlite3 /tmp/test-cmdr.db < 012_runtime_check.sql
```

This skips `002, 003, 004, 005, 006, 007, 009, 011`. Some are independent (jira, openbrain, linkedin) but `002_system_wide.sql` may add columns that affect the rebuild pattern in `012`. The "table-rebuild" pattern in §"SQLite CHECK constraint syntax" does `INSERT INTO sessions_new SELECT * FROM sessions` — if intervening migrations added columns to `sessions`, the spec's `CREATE TABLE sessions_new` skeleton ("... all existing columns ...") will mismatch and the migration will fail.

**Suggestion:** (a) Make the verify snippet apply ALL migrations (`for f in 0*.sql; do sqlite3 ... < $f; done`). (b) Have T1.4 explicitly enumerate the FULL `sessions` schema as of `010_model_session_name.sql` so the table-rebuild copy is byte-exact. (c) Add a check that `PRAGMA table_info(sessions)` post-migration matches pre-migration column-for-column (just `runtime` constraint changes).

---

### CORR-005 [WARNING] — Bridge line citations need re-verification after T1.5

**Section:** §2 Why; §10 Integration table.

**Issue:** Spec cites `cmdr-bridge.sh:345` (literal `'claude'`), `:295`, `:125-138` (map_capability), `:172-184` (resolve_model), `:189-198` (signal_panes), `:203-212` (emit_event), `:217-226` (send_mail), `:309` (db_id), `:375` (state file write), `:421-470` (do_stop fallbacks), `:540-559` (do_stop sweep), `:606-608`, `:683-686`, `:732`, `:749`, `:763`. Spot-checked:

- `:345` confirmed (`'claude'` literal in INSERT VALUES). 
- `:309` confirmed (`db_id="${session_id}-${agent_name}"`). 
- `:295` `session_id="${session_id:-${CLAUDE_SESSION_ID:-...}}"`. 
- `:125-138` `map_capability()` body. 
- `:683-686` cutoff logic. 

These match. However, the §10 table cites lines `425, 432, 452, 464, 595, 590, 698-699, 749, 763, 553, 529` for changes. Lines `698-699` show the actual `do_session_start` UPDATE; `749` shows the `do_cleanup` session_file naming. These are correct.

The cmdr-bridge.sh file is 869 lines and rapidly evolving. **The risk is not that lines are wrong now; it's that they will drift before T1.5 dispatches.** The spec couples worker output to specific line numbers, which makes the spec brittle.

**Suggestion:** Reframe §10's table by FUNCTION NAME, not line number. Example: instead of `| 309 | db_id="${CMDR_RUNTIME}-${session_id}-${agent_name}" |`, write `| do_start (currently L295-310): db_id assignment | Replace literal "${session_id}-${agent_name}" with "${CMDR_RUNTIME}-${session_id}-${agent_name}" |`. Workers can grep by function name; line numbers will still drift but the change locus is unambiguous.

---

### COMP-001 [WARNING] — JSON Output Format section lacks an error-path example for unknown agent-type

**Section:** §7 JSON Output Format.

**Issue:** Section shows the success and "unknown runtime" cases but not the "unknown agent-type" case. T1.1 implementation falls back to `CapabilityBuilder` per §5 ("default"), so the JSON output for an unknown type should still return `success: true` with `capability: "builder"` — but a strict reviewer might expect an error. The fallback policy is a design call worth explicit JSON contract coverage.

**Suggestion:** Add a third example to §7:

```json
// cmdr capability map --runtime claude --agent-type unknown-type --json
{ "success": true, "command": "capability map", "runtime": "claude",
  "agent_type": "unknown-type", "capability": "builder",
  "warning": "agent_type not found in runtime mapping; defaulted to builder" }
```

Or document explicitly that unknown agent-types silently fall back to builder. Either is fine; ambiguity here causes worker divergence.

---

### CONS-001 [WARNING] — `Failure Modes` table cites a fix that contradicts §5 Lifecycle table

**Section:** §"Failure Modes" row "Reaper false-positive reaps active claude agent because heartbeat_at is null".

**Issue:** Failure-mode fix says: `Reaper logic: 'runtime='claude' → ignore heartbeat_at, use last_activity only'; null-safe SQL`. §5 Lifecycle table says the same. But §3 rule 6 says "Reaper reads `heartbeat_at` for `runtime != 'claude'`, falls back to `last_activity` for `runtime = 'claude'`". The word "falls back" implies a single SQL statement that uses `COALESCE(heartbeat_at, last_activity)` — which would re-introduce the bug because `heartbeat_at` IS NULL for old Claude rows and reaper would compare NULL to cutoff.

The correct semantics (per §"Failure Modes" and the Lifecycle table) is **two disjoint reaper queries**, not one query with COALESCE. Make this explicit.

**Suggestion:** Replace §3 rule 6 prose with: "Reaper runs TWO disjoint queries: (a) `WHERE runtime='claude' AND last_activity < cutoff`, (b) `WHERE runtime != 'claude' AND heartbeat_at IS NOT NULL AND heartbeat_at < cutoff`. NULL `heartbeat_at` is a no-op for non-claude rows (means no heartbeat received yet — too early to reap)." This kills any ambiguity for the T2.4 author.

---

### CONS-002 [WARNING] — §15 lists T1.5 in Phase 1 column but the dependency graph shows it depending on T1.1+T1.4

**Section:** §15 Task Manifest; §16 Dependency Graph; §"Execution Order".

**Issue:** §16 says `Phase 1 (parallel): [T1.1, T1.2, T1.3, T1.4]` and `Phase 1 finalization (after Phase 1): [T1.5]`. §15 lists T1.5 as a Phase 1 row. §"Execution Order" puts T1.5 inside `Phase 1 (Foundation)`. So Phase 1 has both parallelizable tasks AND a serial bottleneck. This is fine but mis-named — Phase 1 is not "parallel," it's "parallel-then-serial." Workers reading just §16 will think `[T1.1, T1.2, T1.3, T1.4]` can spawn together and T1.5 follows; they'll then look at §15 and see T1.5's `Depends On: T1.1, T1.4` and ask whether T1.2/T1.3 can also block. (They can't — T1.5 only needs T1.1+T1.4.)

**Suggestion:** Rename §16's Phase 1 to "Phase 1a (parallel)" and add explicit "Phase 1b (serial, after T1.1 + T1.4): [T1.5]". Or restructure as "Phase 1 (parallel preconditions): T1.1, T1.2, T1.3, T1.4 / Phase 2 (bridge): T1.5". The current grouping is technically right but mildly confusing.

---

### CONS-003 [INFO] — CLAUDE.md SPEC LAYOUT RULE conflict with `# Project Rules` section

**Section:** Project's CLAUDE.md (out of spec scope, but spec inherits it).

**Issue:** Project CLAUDE.md has both a SPEC LAYOUT RULE (singular `SPEC/<spec_name>/`) and an older `# Project Rules > Specs` block (lowercase `specs/`). The spec correctly uses the new singular layout but its §11 references "the legacy `specs/multi-agent-tracking.md`" without explaining the dual-rule history. Not a spec defect per se; called out so reviewers know the spec is aware of the conflict.

**Suggestion:** Optional. §11 could append: "Per CLAUDE.md SPEC LAYOUT RULE (which supersedes the older `# Project Rules > Specs` lowercase guidance), legacy `specs/` is FROZEN." That's already roughly in §11; the dual-rule provenance just isn't called out.

---

### CLAR-001 [WARNING] — "Cadence: 30 seconds for icarus/gemini. Claude is exempt" lacks a config knob

**Section:** §4 Tracker Protocol Contract, "Heartbeat" subsection.

**Issue:** Hard-coded 30s heartbeat. §"Failure Modes" mentions `ICARUS_POLLER_INTERVAL` for the poller (T2.5) but no equivalent for the heartbeat loop in the gemini bridge. If a user wants to debug with 5s heartbeats or extend to 120s in CI, they'd patch the bash script. This is a minor friction.

**Suggestion:** Document `CMDR_HEARTBEAT_INTERVAL` env var (default 30s), wire it into both the gemini bash heartbeat loop (§10 "Heartbeat daemon") and the icarus T8 expectation. Or explicitly state "30s is hard-coded; configuration is a follow-up." Either is fine; the silence is what's bad.

---

### CLAR-002 [WARNING] — "Phase 1 finalization (after Phase 1): [T1.5]" but T1.5 needs only T1.1 + T1.4

**Section:** §16 Dependency Graph.

**Issue:** Same surface as CONS-002, but flagged separately for clarity: the words "after Phase 1" imply T1.5 must wait for T1.2 and T1.3 to complete. They don't. T1.2 (gemini.yaml) and T1.3 (palette) are wholly independent of T1.5 (bridge). A reader concludes T1.2/T1.3 are bottlenecks when they aren't.

**Suggestion:** Rephrase as "T1.5 dispatches the moment T1.1 and T1.4 are green; T1.2 and T1.3 may still be in flight."

---

### SDLC-001 [INFO] — Success Criteria mostly map cleanly to predicate types; one is structural-only

**Section:** §19 Success Criteria.

**Issue:** Each criterion maps cleanly:

- `go test ./...` → `count_check` (zero exit) ✓
- `cmdr capability map ...` → `contains_pattern` ✓
- `grep -qE '^CMDR_RUNTIME=' ~/.claude/hooks/cmdr-bridge.sh` → `contains_pattern` ✓
- `No file under pkg/runtimes/ writes the literal 'claude' outside pkg/runtimes/claude/` → `negation_check` ✓
- `All five SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/*.md review files exist and were authored by FRESH cmdr_coder instances (verified by orchestrator routing logs)` → **structural_check + ambiguous predicate**: how does the orchestrator's routing log get scanned? No path is given.

**Suggestion:** Replace the last criterion with a concrete shell check: `wc -l SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/*.md | tail -1 | awk '{print $1}'` AND `find SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/ -name '*.md' | wc -l | grep -q '^[5-9]$'`. The reviewer-independence enforcement happens at orchestrator time and is verified by log inspection, but the success criterion should be machine-checkable in CI.

---

### ACTN-001 [WARNING] — T3.3 verify command is too narrow

**Section:** §15 T3.3.

**Issue:** Verify command is `jq '.hooks.SessionStart' ~/.gemini/settings.json` returns non-null. But the spec's settings.json patch in §10 also adds `PostToolUse` and `SessionEnd`. A worker could ship a partial patch (only SessionStart) and pass T3.3.

**Suggestion:** Tighten to `jq -e '.hooks.SessionStart and .hooks.PostToolUse and .hooks.SessionEnd' ~/.gemini/settings.json` (which is exactly what §18 already uses in the "Layout/Config Validation" check). Just import that to T3.3.

---

### ACTN-002 [INFO] — T1.5 verify command does not exercise the new behavior

**Section:** §15 T1.5.

**Issue:** Verify command is `bats test/cmdr-bridge.bats` exits 0 (existing tests must still pass). But "existing tests" pass on the unmodified bridge; passing them after modification only shows no regression, not new behavior. T2.1, T2.2, T2.3 cover the new behavior — but T1.5 ships first.

**Suggestion:** Add a positive check: T1.5's verify should also include `grep -qE '^CMDR_RUNTIME=' ~/.claude/hooks/cmdr-bridge.sh` and `grep -qE 'active-\$\{CMDR_RUNTIME\}-' ~/.claude/hooks/cmdr-bridge.sh` (these are already in §19 Success Criteria — promote them to per-task verifiers).

---

### COMP-002 [INFO] — Concurrency Model section omits cross-runtime DB lock interaction

**Section:** §8 Concurrency Model.

**Issue:** Section covers per-runtime advisory locks correctly. Doesn't mention what happens if two runtimes write to `sessions` simultaneously and SQLite's WAL+busy_timeout=5000ms is exceeded. The existing claude bridge handles this with `2>/dev/null || log "WARN" "Failed to insert"` which silently drops the row. Two concurrent runtimes increase the chance of this happening. Worth explicit mention.

**Suggestion:** Add to §8 "Conflict Resolution": "SQLite WAL + 5000ms busy_timeout handles N=3 concurrent writers (claude + icarus + gemini) comfortably on consumer hardware. If contention exceeds 5s, the existing bridge silently drops the row (logged as WARN). Multi-runtime regression test T4.2 should run for ≥30s with all three runtimes spawning simultaneously to flush this out."

---

## Decision-Quality Audit (Q1-Q7)

The seven open-question decisions are evaluated below. Findings are not assigned IDs because the decisions themselves are sound; they are documented here for audit.

| # | Decision | Verdict | Rationale |
|---|----------|---------|-----------|
| Q1 | Per-runtime bash forks | **HOLDS** | Failure-blast-radius argument is sound; aligns with existing claude bridge shape; avoids 1-2 quarter Go-daemon migration. Risk: maintenance cost of N forks scales linearly with runtimes; future work should consolidate. The spec acknowledges this in §11 ("Does NOT replatform"). Acceptable. |
| Q2 | Gemini hook events: SessionStart/PostToolUse/SessionEnd, no SubagentStart | **HOLDS** | Verified `gemini hooks --help` only lists `migrate` but the underlying event surface aligns. Heartbeat goroutine (setsid bash) covers the activity gap. Risk: Gemini's event payload shape may diverge from Claude's; T3.2 author needs to inspect a live Gemini hook fire before forking. Spec calls this out in "Failure Modes" row. Acceptable. |
| Q3 | Polling fallback for icarus T8 | **CONDITIONAL on CORR-001** | If T8 is in fact merged (it appears to be), this decision needs re-justification or the task needs to be cut. See CORR-001. |
| Q4 | Heartbeat-driven reaper for non-Claude | **HOLDS** | Migration 008 added the column for exactly this. Two-disjoint-query semantics (per CONS-001) is correct. |
| Q5 | Composite session IDs `{runtime}-{session_id}-{agent_name}` | **HOLDS WITH CAVEAT** | Eliminates collision class. The 64-char VARCHAR ceiling (§4 contract) is tighter now: `claude-${UUID(36)}-${agent(40)}` = 83 chars, will be truncated. The spec says "max 64 chars" but doesn't enumerate the truncation strategy. cmdr-bridge.sh already truncates to 64 (`db_id="${db_id:0:64}"` at L311), so a runtime-prefixed ID will lose the tail of agent_name. This is acceptable for collision-avoidance (UUIDs are unique enough in 36 chars) but worth calling out. **Action:** Add a note to §4 that 64-char truncation is preserved and the policy is "prefix wins, agent_name suffix is truncated." |
| Q6 | `agents/gemini.yaml` with `WebFetch` | **HOLDS** | Mirrors icarus.yaml shape; adds Gemini's web grounding capability. Trivial. |
| Q7 | Single-char colored badge vs runtime column | **HOLDS** | Saves horizontal space (1 char + 1 space vs ~8 chars for a column). Information loss is acceptable because the badge is colored. Risk: low-color terminals lose the information entirely (dumb terminals, log files). **Action:** §6 should document `--no-color` fallback: when colors are disabled, print `[C]`/`[I]`/`[G]` (3 chars) instead of single colored char. Otherwise the runtime info is invisible in piped output. |

---

## Reviewer-Independence Audit

§"Agent Assignments" and §15 task table both correctly call out FRESH `cmdr_coder` reviewer instances for T1.1R, T1.5R, T2.5R, T3.3R, T4.2R. The reviews are scoped to artifacts (not the author's reasoning) per CLAUDE.md REVIEWER INDEPENDENCE RULE. Reviewer count (5) covers all critical authorship: Go (T1.1), bridge (T1.5), poller (T2.5), gemini bridge (T3.3 review covers T3.2+T3.3), and integration (T4.2R). T1.2 (YAML), T1.3 (palette), T1.4 (migration), T2.1-T2.4, T3.1, T4.1 lack their own reviewer-independence checkpoint but those are smaller-scope changes. **No critical gap.**

One minor improvement: T1.4 (migration 012) is structurally important and silently fails if the table-rebuild copy mismatches the live schema (see CORR-004). Recommend adding T1.4R as an explicit reviewer-independence checkpoint. Severity: nice-to-have.

---

## Migration-Safety Audit

§9 covers the runtime CHECK constraint correctly: existing rows have `runtime='claude'` (the default), which is in the IN clause, so the constraint won't reject them. The composite-ID rollout is the larger migration concern — pre-migration rows have `id='${session_id}-${agent_name}'` (no `claude-` prefix), and §9's table correctly identifies that `do_stop` fallback resolvers must keep the old form (`SELECT id FROM sessions WHERE id LIKE '${session_id}-%'`) AND add the new form (`'claude-${session_id}-%'`). This is back-compat correct.

One concern: the rollout window is unspecified. Pre-migration Claude rows with old IDs will eventually age out via reaper at 10min, but during the rollout, two ID forms coexist. The spec says "After 24h, running `do_session_start` will reap any pre-migration rows still stuck in `working`. No manual cleanup required." 24h is reasonable; not a finding.

**Migration safety: PASS.**

---

## Regression-Coverage Audit (T2.1)

§19 specifies T2.1 as: "bats regression test simulates 2 concurrent sessions (1 claude + 1 icarus), kills the claude session, verifies icarus row remains `working` in DB". This directly covers the 2026-03-07 SessionEnd cleanup-bomb class. Test design is sound. One enhancement: also verify the inverse (kill the icarus session, claude row remains). The spec mentions T2.2 covers `do_session_start` scoping and `do_cleanup` scoping but doesn't enumerate the matrix. Recommend extending T2.1 to be a 2x2 matrix: `(claude-killed, icarus-survives) AND (icarus-killed, claude-survives)`. Severity: nice-to-have.

---

## Final Verdict

**REQUEST_CHANGES** — three critical correctness defects (CORR-001 icarus T8 status, CORR-002 reaper directory, CORR-003 cmdr agent show) require mechanical fixes in spec text. None requires user judgment; an iterative spec-builder rebuild loop is appropriate. After the rebuild, a re-review should be a quick PASS.

**Critical fixes required:**
1. Re-cite icarus T8 merge status; either delete T2.5 or repurpose as defensive fallback.
2. Resolve `internal/agents/reaper/` ambiguity: pick in-place vs extracted, name the refactor explicitly.
3. Replace `cmdr agent show` verify commands with shell-checkable equivalents (`yq` / `yamllint`).

**Warnings recommended (not blocking):**
4. Cite cmdr-bridge.sh changes by function name, not line number.
5. Disambiguate reaper SQL semantics (two disjoint queries, not COALESCE).
6. Tighten T3.3 verify command to cover all three hook events.
7. Add `cmdr capability map` JSON example for unknown agent-type fallback.
8. Promote §19 grep-based success checks into T1.5's verify command.
9. Document `CMDR_HEARTBEAT_INTERVAL` env knob (or explicitly defer).
10. Document 64-char `id` truncation policy for composite session IDs.
