# pkg/runtimes/pi/ -- Pi Agent Runtime Adapter (Stub)

## Purpose
Stub implementation of the `AgentRuntime` interface for the Pi agent (JSON-RPC 2.0 capable). Self-registers via `init()`. Shares instruction path with Claude (`.claude/CLAUDE.md`). Uses ANTHROPIC_API_KEY. RPC connection stub placeholder exists but is not yet implemented.

## Technology
- Go 1.25
- Self-registers in the global runtime registry
- Future: JSON-RPC 2.0 for direct RPC communication

## Contents
| File | Description |
|------|-------------|
| `pi.go` | `Runtime` struct: stub implementations of all `AgentRuntime` methods, with Connect() returning nil (RPC not yet implemented) |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Pi runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"pi"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `".claude/CLAUDE.md"` (shared with Claude per spec) |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Returns ANTHROPIC_API_KEY from environment |
| `Connect` | `func (r *Runtime) Connect(process) RuntimeConnection` | `RuntimeConnection` | Returns nil (JSON-RPC 2.0 not yet implemented) |

## Data Types

### Runtime (struct)
Empty struct; stub implementation.

## Logging
N/A

## CRUD Entry Points
- **Create**: `init()` registers runtime in global registry

## Style Guide
- Same stub pattern as other runtime adapters
- Shares CLAUDE.md instruction path with Claude runtime
- `RequiresBeaconVerification()` returns false (Pi uses JSON-RPC per spec)

**Representative snippet (from `pi.go`):**
```go
func (r *Runtime) Connect(process runtimes.ProcessHandle) runtimes.RuntimeConnection {
	// Stub: Pi supports JSON-RPC 2.0 but not yet implemented.
	return nil
}
```
