# pkg/runtimes/gemini/ -- Gemini CLI Runtime Adapter (Stub)

## Purpose
Stub implementation of the `AgentRuntime` interface for Google's Gemini CLI. Self-registers via `init()`. Basic readiness detection checks for `>` prompt indicator.

## Technology
- Go 1.25
- Self-registers in the global runtime registry

## Contents
| File | Description |
|------|-------------|
| `gemini.go` | `Runtime` struct: stub implementations of all `AgentRuntime` methods |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Gemini runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"gemini"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `".gemini/GEMINI.md"` |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Returns GOOGLE_API_KEY from environment |

## Data Types

### Runtime (struct)
Empty struct; stub implementation.

## Logging
N/A

## CRUD Entry Points
- **Create**: `init()` registers runtime in global registry

## Style Guide
- Same stub pattern as other runtime adapters
- Uses GOOGLE_API_KEY for authentication

**Representative snippet (from `gemini.go`):**
```go
func (r *Runtime) InstructionPath() string {
	return ".gemini/GEMINI.md"
}
```
