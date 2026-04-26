# cmd/ — CLI Binary Entry Points

## Scope

Top-level Go `main` packages. Each subdirectory produces a binary.

**Ownership:** `cmdr_coder` (sole code-edit agent for this subtree per `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` PINNED ORCHESTRATION RULE).

## Subdirectories

| Path | Binary | Purpose |
|------|--------|---------|
| `cc/` | `cmdr` | Main CLI entry point. Cobra command tree. One command per file, exporting `XxxCmd(app *App) *cobra.Command`. |
| `hook-bridge/` | `hook-bridge` | Go-TS bridge multiplexer. Also generates TypeScript types from Go structs via `--generate`. |

## Key Conventions

- **One Cobra command per file** in `cc/`. Adding a command means adding a NEW file, not extending an existing one.
- **App DI container** lives in `internal/commands/`. CLI handlers in `cc/` thin-shell over `internal/commands/<verb>.go`.
- **No raw provider SDK calls** in `cc/`. Runtime adapters live in `pkg/runtimes/`.

## Build / Test

```bash
cd /home/n0ko/Programs/ai/computeCommander && make build           # builds cmdr (also builds Rust focus-watcher)
cd /home/n0ko/Programs/ai/computeCommander && make build-bridge    # builds hook-bridge
cd /home/n0ko/Programs/ai/computeCommander && make generate-types  # regenerates TS types
cd /home/n0ko/Programs/ai/computeCommander && go test ./cmd/...
cd /home/n0ko/Programs/ai/computeCommander && make vet
```

`make build` requires `cargo` (it builds the Rust focus-watcher first). `make lint` is best-effort, NOT a gate.

## Read First

Before touching any file in this subtree:

1. `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md` (project rule + three pinned rules)
2. `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md` (repo-root index)
3. `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` (cmdr_coder spec)
4. `/home/n0ko/Programs/ai/computeCommander/cmd/cc/main.go` (Cobra root setup)
5. The relevant `internal/commands/<verb>.go` for the command being modified.

## Gotchas

- The dashboard smoke (`timeout 5s ./cmdr dashboard --tui`) is required after any change in `cmd/cc/dashboard.go` or `internal/tui/`.
- `cmd/hook-bridge/--generate` regenerates TS types under `k8s-cluster/...`; coordinate with the K8s/TypeScript team before changing the Go structs that feed it.
- Branch is `pi`; `main` may lag.
