# pkg/runtimes/codex/ -- Codex CLI Runtime Adapter (Stub)

## Purpose
Stub implementation of the `AgentRuntime` interface for OpenAI's Codex CLI. Self-registers via `init()`. Codex is headless per spec, so readiness detection always returns "ready" and beacon verification is disabled.

## Technology
- Go 1.25
- Self-registers in the global runtime registry

## Contents
| File | Description |
|------|-------------|
| `codex.go` | `Runtime` struct: stub implementations of all `AgentRuntime` methods |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Codex runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"codex"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `"AGENTS.md"` |
| `BuildSpawnCommand` | `func (r *Runtime) BuildSpawnCommand(opts) string` | `string` | Builds `codex --model X` command |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Returns OPENAI_API_KEY from environment |

## Data Types

### Runtime (struct)
Empty struct; all methods are stubs except ID, InstructionPath, BuildSpawnCommand, BuildEnv.

## Logging
N/A

## CRUD Entry Points
- **Create**: `init()` registers runtime in global registry

## Style Guide
- Stub pattern: methods return zero values or nil
- Self-registering via `init()` with `runtimes.RegisterRuntime(New())`

**Representative snippet (from `codex.go`):**
```go
func init() {
	runtimes.RegisterRuntime(New())
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimeCodex
}

func (r *Runtime) DetectReady(_ string) runtimes.ReadyState {
	return runtimes.ReadyState{Phase: "ready"} // Headless per spec
}
```
