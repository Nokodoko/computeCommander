# Spec Review Report

**Spec:** ./SPEC.md
**Iteration:** 2 of 3
**Date:** 2026-03-12

## Verdict: PASS WITH WARNINGS

## Summary

The spec has been substantially rewritten since iteration 1 and now addresses the original feature request for Jira Integration. It delivers real Jira REST API connectivity, multi-instance YAML config, hierarchical data model (Project > Epic > Task), rate limit batching, machine-readable prompt generation, intent verification, dark factory mode, mnemonic keybinds, and multiple execution modes. All 19 sections are present and substantive. The critical failures from iteration 1 (scope misalignment, TypeScript interfaces, broken pane healer) are resolved. Several warnings remain around edge cases, missing `agentic_instructions.md` files for new packages, and a few minor consistency gaps.

## Dimension Scores

| Dimension | Score | Findings |
|-----------|-------|----------|
| Completeness | PASS | 3 warnings |
| Clarity | PASS | 1 warning |
| Correctness | PASS | 2 warnings |
| Consistency | PASS | 2 warnings |
| SDLC | PASS | 2 warnings |
| Actionability | PASS | 3 warnings |

## Feature Request Alignment

| # | Requirement | Addressed | Location |
|---|-------------|-----------|----------|
| 1 | Real Jira REST API connectivity | YES | Section 4 (Data Model), Section 7 (Concurrency), `pkg/integrations/jira/client.go` |
| 2 | Hierarchical view: Projects > Epics > Tasks | YES | Section 4 (JiraProject, JiraEpic, JiraIssue structs), Section 3 (DB schema) |
| 3 | Machine-readable prompt generation for `/sr --review --loop` | YES | Section 5 (`cmdr jira prompt`, `cmdr jira execute --review`), Section 3 (jira-prompt.tmpl) |
| 4 | Intent-management verification with `# Outcomes` review | YES | Section 9 (Intent Verification Pipeline), `internal/darkfactory/intent.go` |
| 5 | Dark factory mode (full automation) | YES | Section 4 (DarkFactoryConfig, Execution Modes), Section 5 (`cmdr jira factory`), `internal/darkfactory/executor.go` |
| 6 | Multi-instance Jira support with YAML config | YES | Section 3 (config.yaml), Section 4 (JiraConfig, JiraInstance structs), Section 5 (`cmdr jira instances`) |
| 7 | Rate limit batching | YES | Section 7 (Token bucket, adaptive backoff, RateLimiter struct), `pkg/integrations/jira/ratelimit.go` |
| 8 | Mnemonic keybinds | YES | Section 5 (Mnemonic Keybinds table) |
| 9 | Agent/State columns in UI | YES | Section 4 (JiraIssue struct: AgentType, AgentState, SessionID fields), Section 6 (JSON output with agentType/agentState) |
| 10 | Multiple execution modes | YES | Section 4 (Execution Modes table: full_auto, stepped, scoped) |

All 10 feature request items are addressed.

## Critical Findings (must fix)

None. All critical issues from iteration 1 have been resolved.

## Warnings (should fix)

1. **[Section 3, 12] No `agentic_instructions.md` for new packages.** The project convention (visible in `internal/commands/`, `internal/agents/`, `internal/zellij/`, `pkg/integrations/github/`, `pkg/integrations/linear/`) places an `agentic_instructions.md` in every package directory. The spec creates two new packages (`pkg/integrations/jira/`, `internal/darkfactory/`) but does not include `agentic_instructions.md` files in the Target State or Task Manifest. Workers in later tasks (T5, T6) that need to read these packages will lack scope documentation. Add these files to T1 and T6 write scopes respectively.

2. **[Section 4] `SyncedAt` field type mismatch with DB schema.** The Go structs define `SyncedAt time.Time` but the SQL schema stores it as `TEXT NOT NULL DEFAULT (datetime('now'))`. SQLite's `datetime()` produces `YYYY-MM-DD HH:MM:SS` strings, not RFC3339. The sync engine or a custom scanner will need to parse this format. The spec should note the required time format or add a `db:"synced_at"` scan helper. This is a correctness risk that could produce zero-value timestamps silently.

3. **[Section 5] `cmdr jira factory --mode` flag inconsistency.** The `cmdr jira factory` command lists `--max-concurrent` and `--dry-run` flags but does not show `--mode`. However, the DarkFactoryConfig has `execution_mode` and the agent-facing commands section (Section 9) shows `cmdr jira factory --project ENG --mode full_auto`. Either add `--mode` to the factory command definition in Section 5, or clarify that factory always uses the config-file mode.

4. **[Section 7] `time.AfterFunc` in RateLimiter creates a goroutine leak risk.** The `AdaptFromHeaders` method uses `time.AfterFunc(retryAfter, func() { r.limiter.SetLimit(r.baseRate) })` but the timer is not tracked or cancellable. If `AdaptFromHeaders` is called multiple times with `retryAfter > 0` before the first timer fires, multiple timers accumulate. The spec should note that previous timers should be cancelled or that the `RateLimiter` struct should hold a `*time.Timer` field for cancellation.

5. **[Section 8] Migration fallback behavior underspecified.** The spec says `cmdr jira` falls back to `task_groups` when no Jira instances are configured (line 549), but the existing `jira.go` reads from `task_groups` today. The spec should clarify: does T5 preserve the existing `task_groups` read path as a fallback within the rewritten `jira.go`, or does it delegate to `cmdr group` commands? The current wording could lead to two implementations of `task_groups` reading logic.

6. **[Section 9] Hooks JSON block uses wrong format.** The hooks integration (lines 583-599) uses a JSON format for hooks configuration, but Claude Code hooks are configured in `~/.claude/settings.json` under a specific schema. The spec should either reference the actual settings.json path and format, or note that this is pseudo-config showing the intended hook behavior rather than a literal configuration block.

7. **[Section 9] `cmdr jira check-completion --session $SESSION_ID` not in CLI section.** The SubagentStop hook references this command, but it is not listed in the CLI commands in Section 5. Workers implementing T5 will not know this command exists unless they read Section 9. Add it to Section 5 or note it as an internal-only command.

8. **[Section 15] T2 depends on T4 but dependency graph shows T2 depends on T1 and T4.** This is correct in the Task Manifest table (T2 depends on T1, T4) and correct in the Dependency Graph (Phase 2 after Phase 1). However, T2's read scope lists `internal/platform/db/db.go` but not the migration file `005_jira_cache.sql`. The sync engine needs to know the exact table schema to write SQL queries. Add `internal/platform/db/migrations/sqlite/005_jira_cache.sql` to T2's read scope.

9. **[Section 15] T6 read scope missing `internal/darkfactory/` for intent.go.** T6 writes both `executor.go` and `intent.go` to `internal/darkfactory/`, and lists several read dependencies. However, T6 does not list `internal/agents/types.go` in its read scope, which it will need for `SpawnRequest`, `SpawnResult`, `AgentSession`, and `SessionState` types referenced by the executor.

10. **[Section 17] Target State line counts may be low.** The `client.go` estimate is ~200 lines for a full Jira REST API client covering 8 operations (search, get issue, get project, list epics, transition, comment, list fields, bulk search) plus authentication for 3 auth types plus response parsing. This is likely closer to 300-400 lines. Similarly `executor.go` at ~250 lines must implement sync-prompt-spawn-verify-transition pipeline with concurrency control. Underestimates are not blocking but set incorrect expectations for workers.

11. **[Section 19] Success criteria missing dark factory verification.** The success criteria check for file existence and CLI help output, but do not verify that `cmdr jira factory --help` works or that the dark factory executor can be instantiated. Add: `./cmdr jira factory --help 2>&1 | grep -q 'project'`.

## Positive Observations

- The spec is a dramatic improvement from iteration 1. Every critical finding has been addressed, and the spec now genuinely implements Jira integration rather than a local DB pane.
- Data models are proper Go structs with correct `json`, `yaml`, and `db` tags matching existing project conventions.
- The pane healer fix correctly uses `SendKeys` to restart processes in existing panes, matching the `zellij.PaneManager` interface that already exposes `SendKeys(paneID string, keys string) error`.
- The rate limiter design using `golang.org/x/time/rate.Limiter` with adaptive header-based backoff is well-specified with concrete code.
- The dependency graph is logically sound: Phase 1 (client, config, migration, healer) is fully parallel, Phase 2 (sync) blocks correctly on client + schema, Phase 3 (CLI, dark factory, template) fans out after sync.
- The issue lifecycle mapping (Jira Status to cmdr Agent State) is clear and covers all terminal states.
- Failure modes are comprehensive with detection and recovery strategies for each failure type.
- Test coverage guidance is detailed per test file with specific test cases enumerated.
- The CLI command pattern `JiraCmd(app *App) *cobra.Command` matches the existing codebase convention exactly.
- The `pkg/integrations/jira/` placement follows the existing pattern set by `pkg/integrations/github/` and `pkg/integrations/linear/`.
- JSON output formats are fully specified with success, error, and factory status examples.
- The T9 verify command is now concrete (`test -f REVIEW-T9.md && grep -q 'APPROVED' REVIEW-T9.md`), resolving the validation error from iteration 1.
- The worktree base branch is correctly set to `main`.
- Section numbering is consistent (1-19).
