# Spec Validation Errors

| Field | Value |
|-------|-------|
| Spec | specs/crush-ui-spec.md |
| Validated | 2026-03-17T00:00:00Z |
| Result | **VALIDATION_FAILED** (2 failures out of 10 checks) |

## Failures

### Check 2: Write-scope collisions

**Status:** FAIL
**Details:** Tasks T5 and T7 both write to `internal/tui/render.go` without dependency ordering (both depend on T2 only). Tasks T10 and T12 both write to `internal/tui/dashboard.go` without dependency ordering (T10 depends on T4+T9, T12 depends on T9+T11; neither depends on the other). Additionally, T5 and T8 both write to `internal/tui/render.go` -- while T8 depends on T7, T8 has no dependency on T5.

### Check 6: Write-scope in Target State

**Status:** FAIL
**Details:** File `internal/commands/status.go` in Task T4 write-scope but not in Target State (section 16). T4's write column lists `internal/commands/status.go` but the Target State "Files modified" list does not include it.

## All Results

| Check | Name | Status |
|-------|------|--------|
| 1 | Section presence | PASS |
| 2 | Write-scope collisions | FAIL |
| 3 | Acyclic dependencies | PASS |
| 4 | Known agent roster | PASS |
| 5 | Non-empty Verify Commands | PASS |
| 6 | Write-scope in Target State | FAIL |
| 7 | Target State in write-scope | PASS |
| 8 | Task count consistency | PASS |
| 9 | Success Criteria checkboxes | PASS |
| 10 | Rebuild-specific checks | PASS (not a rebuild spec) |
