# Architecture Review: Jira Integration for Task Management

**Reviewer:** code-review agent
**Date:** 2026-03-12
**Verdict:** APPROVED

## Scope

Reviewed all files produced by tasks T1-T8 of the Jira Integration spec.

## Files Reviewed

### pkg/integrations/jira/ (T1, T2)
- `types.go` - Data model structs matching spec section 4
- `client.go` - REST API client with rate-limiting integration
- `ratelimit.go` - Token bucket with adaptive backoff from headers
- `sync.go` - Jira-to-DB cache sync engine
- `prompt.go` - Deterministic prompt generation from issues
- `client_test.go` - httptest-based API tests (8 tests)
- `ratelimit_test.go` - Rate limiter behavior tests (5 tests)
- `sync_test.go` - Sync engine tests with in-memory SQLite (5 tests)
- `prompt_test.go` - Prompt generation tests (4 tests)

### internal/darkfactory/ (T6)
- `executor.go` - Dark factory orchestration loop with concurrency control
- `intent.go` - Intent verification with predicate classification
- `executor_test.go` - Executor tests with mock Jira server (4 tests)
- `intent_test.go` - Intent verification tests (7 tests)

### internal/config/config.go (T3)
- Added `JiraConfig`, `JiraInstance`, `JiraAuth`, `JiraRateLimitCfg`, `DarkFactoryConfig` structs
- Added defaults in `DefaultConfig()`
- All existing tests continue to pass

### internal/platform/db/migrations/ (T4)
- `sqlite/005_jira_cache.sql` - 4 tables, 5 indexes
- `postgres/005_jira_cache.sql` - Postgres variant with TIMESTAMPTZ

### internal/commands/jira.go (T5)
- Full CLI rewrite with 7 subcommands: show, sync, prompt, execute, transition, instances, factory
- Backward-compatible fallback to task_groups when no Jira instances configured
- JSON output support on all commands

### internal/watchdog/pane_healer.go (T7)
- Changed healPane from ClosePane+CreatePane to SendKeys (Ctrl-C + re-type command)
- Preserves KDL layout positioning

### .computecommander/templates/jira-prompt.tmpl (T8)
- Default Go template with Outcomes section

## Architecture Assessment

### Strengths

1. **Multi-instance by design.** All data paths include `instance_name` partitioning from day one.
2. **Rate limiting is first-class.** Token bucket with adaptive headers is properly wired through every API call.
3. **Backward compatibility.** The CLI falls back to `task_groups` when no Jira instances are configured. Zero breakage for existing users.
4. **Deterministic prompts.** Same issue + same template = same SHA-256 hash. Template is Go stdlib `text/template`.
5. **Clean package boundaries.** `pkg/integrations/jira/` has no dependency on `internal/commands/` or `internal/darkfactory/`. Dark factory depends on jira pkg, not the reverse.
6. **Pane healer fix.** SendKeys approach correctly preserves KDL layout positioning per spec design principle 7.
7. **Go structs, not interfaces.** All data models are Go structs with json/yaml tags per spec design principle 8.

### Observations

1. **No `golang.org/x/time` vendor.** The dependency was added via `go get`. Module downloads work as expected.
2. **Foreign key handling.** Epic ID is nullable (`*string`) to avoid FK constraint violations when issues have no epic parent. Correctly handles both classic and next-gen Jira project structures.
3. **Sync engine uses ON CONFLICT upsert.** Incremental syncs update existing rows without duplicate-key errors.
4. **Dark factory executor** uses goroutine pool with semaphore for concurrency control. Stepped mode generates and validates without spawning agents.

### Test Coverage

| Package | Tests | Status |
|---------|-------|--------|
| pkg/integrations/jira | 22 | PASS |
| internal/darkfactory | 11 | PASS |
| internal/commands (Jira) | 6 | PASS |
| internal/config | 13 | PASS |
| internal/platform/db | 14 | PASS |
| internal/watchdog | 5 | PASS |

### Verification Commands

All spec verification commands pass:
- `go build ./...` - Clean build
- `go vet ./...` - No issues
- `go test ./pkg/integrations/jira/... -count=1` - PASS
- `go test ./internal/darkfactory/... -count=1` - PASS
- `go test ./internal/commands/... -count=1 -run TestJira` - PASS
- `go test ./internal/config/... -count=1` - PASS
- `go test ./internal/platform/db/... -count=1` - PASS
- `go test ./internal/watchdog/... -count=1 -run TestPaneHealer` - PASS
- `cmdr jira --help` contains sync, execute, factory - PASS

## Conclusion

The implementation follows the spec's design principles, data model, CLI surface, and file layout. All new packages have test coverage. No circular dependencies. Backward compatibility with existing task_groups is maintained. APPROVED for merge.
