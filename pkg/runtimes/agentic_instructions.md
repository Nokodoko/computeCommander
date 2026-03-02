# pkg/runtimes/ -- Pluggable Agent Runtime System

## Purpose
Defines the `AgentRuntime` interface and global registry for pluggable AI coding agent backends. Each runtime adapter (Claude, Gemini, Codex, Pi, Goose) registers itself via `init()` and implements spawning, config deployment, readiness detection, and transcript parsing.

## Technology
- Go 1.25
- `sync.RWMutex`-protected global registry
- No external dependencies in the core package; adapters import their specific tooling

## Contents
| File | Description |
|------|-------------|
| `runtime.go` | `AgentRuntime` interface (10 methods), `RuntimeID` enum (5 runtimes), `SpawnOpts`, `ReadyState`, `OverlayContent`, `HooksDef`, `TranscriptSummary`, `RuntimeConnection`/`ProcessHandle` interfaces, global registry (`RegisterRuntime`/`GetRuntime`) |
| `runtime_test.go` | Tests for registry operations, runtime ID validation |
| `claude/` | Claude Code runtime adapter (fully implemented) |
| `codex/` | Codex CLI runtime adapter (stub) |
| `gemini/` | Gemini CLI runtime adapter (stub) |
| `goose/` | Goose runtime adapter (stub) |
| `pi/` | Pi agent runtime adapter (stub with JSON-RPC 2.0 placeholder) |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `RegisterRuntime` | `func RegisterRuntime(runtime AgentRuntime)` | - | Adds runtime to global registry (thread-safe) |
| `GetRuntime` | `func GetRuntime(id string) (AgentRuntime, error)` | `AgentRuntime, error` | Looks up runtime by string ID |
| `AllRuntimeIDs` | `func AllRuntimeIDs() []RuntimeID` | `[]RuntimeID` | Returns all valid runtime ID constants |

## Data Types

### RuntimeID (string enum)
`claude` | `gemini` | `codex` | `pi` | `goose`

### AgentRuntime (interface)
| Method | Returns | Description |
|--------|---------|-------------|
| `ID()` | `RuntimeID` | Unique identifier |
| `InstructionPath()` | `string` | Relative path to instruction file (e.g., `.claude/CLAUDE.md`) |
| `BuildSpawnCommand(opts)` | `string` | Shell command to spawn an agent |
| `BuildPrintCommand(prompt, model)` | `[]string` | Argv for headless one-shot calls |
| `DeployConfig(ctx, path, overlay, hooks)` | `error` | Deploys instructions and hooks to worktree |
| `DetectReady(paneContent)` | `ReadyState` | Parses pane content for readiness |
| `ParseTranscript(path)` | `*TranscriptSummary, error` | Extracts token usage |
| `BuildEnv(model)` | `map[string]string` | Runtime-specific env vars |
| `RequiresBeaconVerification()` | `bool` | Whether beacon resend loop is needed |
| `Connect(process)` | `RuntimeConnection` | Establishes RPC connection (optional) |

### SpawnOpts (struct)
Fields: Model, PermissionMode, SystemPrompt, AppendPrompt, PromptFile, WorkDir, Env

### OverlayContent (struct)
Fields: Content (markdown text)

### HooksDef (struct)
Fields: AgentName, Capability, WorktreePath, QualityGates, FileScope, Rules

## Logging
- No structured logging; errors returned via `fmt.Errorf`

## CRUD Entry Points
- **Create**: `RegisterRuntime()` registers a new runtime adapter
- **Read**: `GetRuntime()` looks up by ID, `AllRuntimeIDs()` lists all IDs

## Style Guide
- Self-registering: each adapter calls `RegisterRuntime` in `init()`
- Interface-first: `AgentRuntime` defines the contract
- Thread-safe registry via `sync.RWMutex`
- Import for side effects: `_ "pkg/runtimes/claude"` triggers registration

**Representative snippet (from `runtime.go`):**
```go
type AgentRuntime interface {
	ID() RuntimeID
	InstructionPath() string
	BuildSpawnCommand(opts SpawnOpts) string
	BuildPrintCommand(prompt string, model string) []string
	DeployConfig(ctx context.Context, worktreePath string,
		overlay *OverlayContent, hooks *HooksDef) error
	DetectReady(paneContent string) ReadyState
	ParseTranscript(path string) (*TranscriptSummary, error)
	BuildEnv(model string) map[string]string
	RequiresBeaconVerification() bool
	Connect(process ProcessHandle) RuntimeConnection
}
```
