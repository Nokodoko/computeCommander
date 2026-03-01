# pkg/runtimes/goose/ -- Goose Runtime Adapter (Stub)

## Purpose
Stub implementation of the `AgentRuntime` interface for Block's Goose agent. Self-registers via `init()`. Basic readiness detection checks for `>` prompt indicator.

## Technology
- Go 1.25
- Self-registers in the global runtime registry

## Contents
| File | Description |
|------|-------------|
| `goose.go` | `Runtime` struct: stub implementations of all `AgentRuntime` methods |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Goose runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"goose"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `".goose/instructions.md"` |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Returns GOOSE_API_KEY from environment |

## Data Types

### Runtime (struct)
Empty struct; stub implementation.

## Logging
N/A

## CRUD Entry Points
- **Create**: `init()` registers runtime in global registry

## Style Guide
- Same stub pattern as other runtime adapters
- Uses GOOSE_API_KEY for authentication

**Representative snippet (from `goose.go`):**
```go
func (r *Runtime) InstructionPath() string {
	return ".goose/instructions.md"
}
```
