# pkg/runtimes/pi/ -- Pi Agent Runtime Adapter

## Purpose
Full implementation of the `AgentRuntime` interface for the Pi coding agent. Self-registers via `init()`. Shares instruction path with Claude (`.claude/CLAUDE.md`) via symlinks. Supports multiple LLM providers (Google, Anthropic, OpenAI), TypeScript extensions, JSON-RPC 2.0 mode, and session management. At parity with the Claude runtime adapter for all `AgentRuntime` methods.

## Technology
- Go 1.25
- Self-registers in the global runtime registry
- Pi CLI flags: `--model`, `-p`, `--append-system-prompt`, `--no-extensions`, `--mode rpc`
- Shared config via symlinks: `~/.pi/agent/agents/` -> `~/.claude/agents/`
- Future: JSON-RPC 2.0 client for `--mode rpc` communication

## Contents
| File | Description |
|------|-------------|
| `pi.go` | `Runtime` struct: full implementations of all `AgentRuntime` methods matching Claude adapter parity |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Pi runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"pi"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `".claude/CLAUDE.md"` (shared with Claude via symlink) |
| `BuildSpawnCommand` | `func (r *Runtime) BuildSpawnCommand(opts) string` | `string` | Full spawn with -p, --model, --append-system-prompt, --no-extensions, prompt file redirect |
| `BuildPrintCommand` | `func (r *Runtime) BuildPrintCommand(prompt, model) []string` | `[]string` | Headless one-shot: `pi -p --model <model> <prompt>` |
| `DeployConfig` | `func (r *Runtime) DeployConfig(ctx, path, overlay, hooks) error` | `error` | Writes .claude/CLAUDE.md + .pi/agent/settings.json for worktree isolation |
| `DetectReady` | `func (r *Runtime) DetectReady(paneContent) ReadyState` | `ReadyState` | Detects ready (">", box chars), permission dialogs, trust/approve dialogs |
| `ParseTranscript` | `func (r *Runtime) ParseTranscript(path) (*TranscriptSummary, error)` | `*TranscriptSummary, error` | JSONL parsing with dual token field support (snake_case and camelCase) |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Multi-provider: ANTHROPIC_API_KEY, GOOGLE_API_KEY, OPENAI_API_KEY, PI_CODING_AGENT_DIR |
| `Connect` | `func (r *Runtime) Connect(process) RuntimeConnection` | `RuntimeConnection` | Returns nil (JSON-RPC 2.0 client pending implementation) |

## Data Types

### Runtime (struct)
Empty struct; all methods are receiver functions.

### settingsJSON (struct)
Pi-specific settings for worktree deployment. Fields: DefaultProvider, DefaultModel.

## Logging
N/A -- errors returned via `fmt.Errorf`

## CRUD Entry Points
- **Create**: `init()` registers runtime in global registry

## Style Guide
- At parity with `claude/claude.go` -- same method coverage and test depth
- Shares CLAUDE.md instruction path with Claude runtime via symlink
- `RequiresBeaconVerification()` returns false (Pi uses JSON-RPC per spec)
- Multi-provider env support reflects Pi's provider-agnostic architecture
- `shellQuote()` helper matches Claude adapter pattern

**Representative snippet (from `pi.go`):**
```go
func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	var parts []string
	parts = append(parts, "pi")
	if opts.PermissionMode == "bypass" {
		parts = append(parts, "--no-extensions")
	}
	parts = append(parts, "-p")
	if opts.Model != "" {
		parts = append(parts, "--model", opts.Model)
	}
	if opts.AppendPrompt != "" {
		parts = append(parts, "--append-system-prompt", shellQuote(opts.AppendPrompt))
	}
	if opts.PromptFile != "" {
		parts = append(parts, "<", opts.PromptFile)
	} else if opts.SystemPrompt != "" {
		parts = append(parts, shellQuote(opts.SystemPrompt))
	}
	return strings.Join(parts, " ")
}
```
