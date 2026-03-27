# Spec Review Feedback — Iteration 1

Incorporate the following fixes into the next SPEC.md revision:

## Critical Fixes (must address)

1. **Migration numbering: rename 002 to 008.** The codebase already has migrations `001` through `007`. The new migration must be `008_multi_agent.sql`. Global find-replace `002_multi_agent` with `008_multi_agent` across: On-Disk Format, SQL code blocks, Task Manifest T1 (read/write scope and verify command), Verification Plan, Rollback command, and Success Criteria.

2. **Add `internal/agents/types.go` and `internal/agents/spawner.go` to T4 write-scope and Target State.** The `--runtime` flag on `cmdr status` requires a `Runtime` field on `ListOpts` (in `types.go`) and a new WHERE clause in `ListSessions` (in `spawner.go`). Currently neither file is in any task's write-scope or in Target State's modified files list. Add both to T4's write-scope and to the "Files modified" list in Target State.

3. **Acknowledge FeedCmd restructuring in T9.** The existing `FeedCmd` is a leaf command with `RunE`. Adding `feed emit` as a subcommand requires restructuring it to support both default behavior (current `RunE`) and the new `emit` subcommand. T9's description must explicitly state this refactoring need, not just "add `feed emit` subcommand."

## Warnings (should address)

1. **Fix T7 read-scope.** T7 references `scripts/cmdr-agent-wrapper.sh` as read-scope, but this file has been deleted (`.computecommander/scripts/cmdr-agent-wrapper.sh` is `D` in git status, and the path differs). Remove or correct the read-scope entry.

2. **Specify OpenBrain agent event limit.** The Failure Modes section mentions "max 5 recent events" but this constraint is not defined in any implementation-facing section (CLI, Integration). Add the limit to the OpenBrain CLI or Integration section so implementers know to enforce it.

3. **Note that `runOpenBrainPane` needs `*App` parameter.** Currently `runOpenBrainPane(ctx, projectDir)` has no DB access. T5 needs to refactor it to accept `*App` for event queries. State this explicitly in T5's description.

4. **Make Success Criteria self-contained for heartbeat/deregister.** The `<id>` placeholder in two criteria is not shell-executable. Rewrite them as pipelines that first register, then test (matching the integration check pattern).

5. **Update Estimated Size file count.** After adding `types.go` and `spawner.go`, the real count is ~14 files, not 11.

6. **Clarify T5 testing dependency on T10.** T5 queries agent lifecycle events but those events are only emitted by T10. Add a note that T5's functional testing depends on T10, or ensure T8 covers this.
