# pkg/runtimes/icarus/ -- Icarus Agent Runtime Adapter

## Purpose
Full implementation of the `AgentRuntime` interface for the Icarus Go-native coding agent harness (sister project at `/home/n0ko/Programs/ai/icarus`). Self-registers via `init()`. Shares the instruction path with Claude (`.claude/CLAUDE.md`) so the same overlay file works across runtimes. Threads icarus's provider-agnostic effort knob (off / minimal / low / medium / high / xhigh, per icarus_cc_parity §5) through cmdr's existing spawn pipeline via the `ICARUS_EFFORT` environment variable.

## Technology
- Go 1.25
- Self-registers in the global runtime registry (`pkg/runtimes/`)
- Icarus CLI flags consumed by this adapter: `icarus run --model <id> --effort <level> --append-system-prompt <text> [< prompt-file]`
- Per-worktree settings written to `.icarus/settings.json` (default ob1 api-key env: `OB_API_KEY`)
- Effort enum mirrors cmdr's pi `--thinking-level` so the two binaries speak the same dialect

## Contents
| File | Description |
|------|-------------|
| `icarus.go` | `Runtime` struct: `ID`, `InstructionPath`, `BuildSpawnCommand`, `BuildPrintCommand`, `DeployConfig`, `DetectReady`, `ParseTranscript`, `BuildEnv`, `RequiresBeaconVerification`, `Connect`. Exports `RuntimeIcarus` string constant for direct callers. |
| `init.go` | `init()` calling `runtimes.RegisterRuntime(New())` so blank-import side-effect registration works. |
| `icarus_test.go` | Table-driven tests covering all `AgentRuntime` methods, effort resolution order, transcript parsing, and registry round-trip. |

## Key Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `New` | `func New() *Runtime` | Creates a new icarus runtime adapter. |
| `BuildSpawnCommand` | `func (r *Runtime) BuildSpawnCommand(opts SpawnOpts) string` | Assembles `icarus run --model X --effort Y --append-system-prompt Z [< prompt-file]`. Effort resolved from `opts.Env["ICARUS_EFFORT"]`, then process env. Invalid effort levels are dropped (not passed through). |
| `DeployConfig` | `func (r *Runtime) DeployConfig(ctx, path, overlay, hooks) error` | Writes `.claude/CLAUDE.md` and a placeholder `.icarus/settings.json` (with `ob1.api_key_env: OB_API_KEY`) when hooks are non-nil. |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | Forwards `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY`, `OB_API_KEY`, `ICARUS_HOME` from the parent process; sets `ICARUS_MODEL` if a model is provided. |

## Data Types

### Runtime (struct)
Empty struct; all methods are receiver functions matching pi/claude shape.

### settingsJSON / providersBlock / ob1Block
Internal types for the per-worktree `.icarus/settings.json` shape.

## CRUD Entry Points
- **Create**: `init()` in `init.go` registers the runtime in the global registry.

## Style Guide
- At parity with `pi/pi.go` -- same method coverage, same shell-quote helper, same transcript-parsing approach.
- `RequiresBeaconVerification()` returns false (icarus uses TypedBus events, not the beacon protocol).
- Effort knob is the only runtime-specific flag passed through; tools/file-scope are server-side enforcement (T8).

## T8 Contract (icarus-side emitter expectations)

The `BuildSpawnCommand` argv that this adapter produces is the contract icarus's
emitter side (`internal/integration/cmdr/` in the icarus tree, T8) consumes.
Stable shape:

| Surface | Value |
|---------|-------|
| Binary  | `icarus` |
| Mode    | `run` (one-shot, headless) |
| Flags   | `--model <id>` (optional), `--effort <off\|minimal\|low\|medium\|high\|xhigh>` (optional), `--append-system-prompt <text>` (optional), positional prompt OR `< prompt-file` redirection |
| Env (forwarded by `BuildEnv`) | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY`, `OB_API_KEY`, `ICARUS_HOME`, `ICARUS_MODEL` |
| Env (consumed by `BuildSpawnCommand`) | `ICARUS_EFFORT` (per-spawn or process env) |
