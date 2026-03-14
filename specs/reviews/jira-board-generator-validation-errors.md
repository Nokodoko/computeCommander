# Spec Validation Errors

| Field | Value |
|-------|-------|
| Spec | /home/n0ko/Programs/ai/computeCommander/SPEC.md |
| Validated | 2026-03-11T00:00:00Z |
| Result | **VALIDATION_FAILED** (1 failure out of 10 checks) |

## Failures

### Check 5: Non-empty Verify Commands

**Status:** FAIL
**Details:** Task T9 (code-review) has `--` as its Verify Command, which is a prohibited placeholder value — every task row must have a non-empty, non-placeholder Verify Command.

## All Results

| Check | Name | Status |
|-------|------|--------|
| 1 | Section presence | PASS |
| 2 | Write-scope collisions | PASS |
| 3 | Acyclic dependencies | PASS |
| 4 | Known agent roster | PASS |
| 5 | Non-empty Verify Commands | FAIL |
| 6 | Write-scope in Target State | PASS |
| 7 | Target State in write-scope | PASS |
| 8 | Task count consistency | PASS |
| 9 | Success Criteria checkboxes | PASS |
| 10 | Rebuild-specific checks | SKIP |
