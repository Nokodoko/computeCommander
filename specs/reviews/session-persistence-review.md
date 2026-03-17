# Spec Review: Session State Persistence and Restore

**Spec path:** `.computecommander/specs/session-persistence.md`
**Reviewed:** 2026-03-07
**Iteration:** 1 of 3

---

## Verdict: PASS WITH WARNINGS

The spec is well-structured, grounded in the existing codebase, and actionable. There are no critical blockers. Three warnings should be addressed before implementation.

---

## Dimension 1: Completeness

**Rating: PASS**

The spec covers:
- Problem statement and goal
- Current architecture analysis (accurate -- verified against `session_manager.go` and `main.go`)
- State file schema with versioning
- Save triggers (graceful, autosave, signal)
- Restore flow with opt-in semantics
- Edge cases (corrupt, missing, stale, concurrent, multi-instance)
- Testing plan with specific test names
- Non-goals

No major gaps found.

---

## Dimension 2: Clarity

**Rating: PASS**

- The spec uses concrete file paths, function signatures, and JSON schema
- The CLI interface section is clear and concise
- Implementation plan maps directly to source files

---

## Dimension 3: Correctness

**Rating: PASS WITH WARNINGS**

### Warning 1: `--force` flag collision risk

The spec proposes adding `--force` to the root command. Currently `--force` is a local flag on `init` only (line 296 of `main.go`). However, adding `--force` as a root flag could shadow the `init --force` flag, depending on cobra's flag resolution.

**Recommendation:** Use `--restore-force` or make `--force` a local flag on root only (not persistent). Alternatively, combine into a single `--restore=force` value.

### Warning 2: `CreateSession` vs raw state restoration

The restore flow calls `SessionManager.CreateSession(directory, runtime)` for each saved session. However, `CreateSession` generates a new ID (`dsess-XXXXXXXX` via `sm.nextID++`), which means restored sessions get different IDs than they had before the crash. This breaks any external references to session IDs (e.g., in the DB's `sessions` table via `AgentSessionID`).

**Recommendation:** Add a `RestoreSession(sess *DirectorySession)` method that directly inserts into the map with the original ID, rather than going through `CreateSession`.

### Warning 3: Config dir resolution is underspecified

The spec says the state file goes in `<configDir>/session-state.json` where configDir is `.computecommander/` or `~/.computecommander/`. But it does not specify how to determine which one to use. The config loading code (`config.go` lines 486-527) tries system-wide first, then per-project. The state file should follow the same resolution order, but the spec should state this explicitly.

**Recommendation:** Add a sentence: "The state file path uses the same config directory that was resolved during `LoadSystemConfig` / `LoadConfig`. The resolved config dir should be exposed via `Config.ConfigDir` or passed explicitly to `SaveState`/`LoadState`."

---

## Dimension 4: Consistency

**Rating: PASS**

- The spec is consistent with existing patterns (cobra flags, `App` methods, `SessionManager` API style)
- The autosave goroutine pattern is consistent with how `watchdog` runs background work
- Signal handling is consistent with existing `PersistentPostRun` cleanup

---

## Dimension 5: SDLC (Security, Dependencies, Lifecycle)

**Rating: PASS**

- No new dependencies introduced
- State file contains directory paths and session metadata only -- no secrets
- The spec explicitly lists "no encryption" as a non-goal, which is appropriate for v1
- The temp-file + rename write pattern prevents partial-write corruption
- PID liveness check via `/proc/<pid>/` is Linux-specific but acceptable given the target platform (Linux 6.18.7)

---

## Dimension 6: Actionability

**Rating: PASS**

- Implementation plan maps to 4 specific files with clear responsibilities
- Function signatures are provided
- Test plan includes 5 named unit tests and 1 integration test
- The spec is implementable by a single developer in one session

---

## Summary of Findings

| # | Severity | Dimension | Finding |
|---|----------|-----------|---------|
| 1 | Warning | Correctness | `--force` flag may shadow `init --force` -- use `--restore-force` or scoped local flag |
| 2 | Warning | Correctness | `CreateSession` generates new IDs -- use a dedicated `RestoreSession` that preserves original IDs |
| 3 | Warning | Correctness | Config dir resolution for state file path is underspecified -- clarify resolution order |

**Total: 0 critical, 3 warnings, 0 info**
