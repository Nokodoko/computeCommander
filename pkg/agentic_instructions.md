# pkg/ — Public Packages (LOCKED at 2)

## Scope

Public Go packages importable from outside this repo. **LOCKED** at 2 entry points per cmdr_coder spec §10 / §13. New top-level public packages require a NEW spec, not an inline edit.

**Ownership:** `cmdr_coder` (sole code-edit agent for this subtree).

## Subdirectories

| Path | Purpose |
|------|---------|
| `runtimes/` | `AgentRuntime` interface + 5 self-registering adapters (Claude, Gemini, Codex, Pi, Goose). New runtime support adds a NEW package under `pkg/runtimes/<name>/` with its own `init()` calling `RegisterRuntime()`, NOT by editing `cmd/cc`. |
| `integrations/` | GitHub, Linear, Webhook stubs. |

## Key Abstractions

| Symbol | Defined In | Notes |
|--------|-----------|-------|
| `AgentRuntime` (interface) | `runtimes/runtime.go` | The universal adapter for any AI runtime. 5 implementations today; each self-registers via `init()` → `RegisterRuntime(name, factory)`. |
| `RegisterRuntime` | `runtimes/runtime.go` | Registry function called from each adapter's `init()`. |

## Build / Test

```bash
cd /home/n0ko/Programs/ai/computeCommander && go test ./pkg/...
cd /home/n0ko/Programs/ai/computeCommander && go test ./pkg/... -race
cd /home/n0ko/Programs/ai/computeCommander && make vet
cd /home/n0ko/Programs/ai/computeCommander && make build         # the cmdr binary depends on pkg/runtimes
```

Edits to `pkg/runtimes` MUST run `make build && go test ./... && make vet` before commit (per cmdr_coder Project Rule #11).

## Read First

Before touching any file in this subtree:

1. `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md`
2. `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md`
3. `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` (§9 abstraction discipline; §10 architecture lock)
4. `pkg/runtimes/runtime.go` (the `AgentRuntime` interface — load-bearing)
5. Any existing adapter (e.g., `pkg/runtimes/claude/`) for the registration + factory pattern.

## Gotchas

- **`pkg/` is LOCKED at 2 packages.** Adding a top-level package requires a new spec under `SPEC/<spec_name>/`, not an inline edit. Refuse with `Blocked: pkg/ lock — new top-level pkg requires a spec`.
- **Adapter registration pattern:** new runtimes self-register via `init()` calling `RegisterRuntime()`. Do NOT bypass with direct calls from `cmd/cc`.
- **`AgentRuntime` interface refactors are blast-radius critical.** All 5 adapters implement it. Always run `findReferences` via gopls + tree-sitter-go before changing a method signature.
- **Speculative interfaces are forbidden** (cmdr_coder Project Rule #9). The 5+ existing adapters justify `AgentRuntime`; do not generalize further without a second use case.
