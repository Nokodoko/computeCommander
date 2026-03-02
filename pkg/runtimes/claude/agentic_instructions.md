# pkg/runtimes/claude/ -- Claude Code Runtime Adapter

## Purpose
Implements the `AgentRuntime` interface for Claude Code (Anthropic's CLI). This is the primary and most fully implemented runtime adapter. Handles spawning with `--dangerously-skip-permissions`, deploying `.claude/CLAUDE.md` instructions and `settings.local.json` hooks, detecting readiness from pane content, and parsing transcripts for token usage.

## Technology
- Go 1.25
- `os/exec` for Claude CLI spawning
- `encoding/json` for settings.local.json generation
- Self-registers via `init()` in the global registry

## Contents
| File | Description |
|------|-------------|
| `claude.go` | `Runtime` struct implementing all `AgentRuntime` methods: ID, InstructionPath, BuildSpawnCommand, DeployConfig, DetectReady, ParseTranscript, BuildEnv, Connect |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `New` | `func New() *Runtime` | `*Runtime` | Creates new Claude runtime adapter |
| `ID` | `func (r *Runtime) ID() RuntimeID` | `RuntimeID` | Returns `"claude"` |
| `InstructionPath` | `func (r *Runtime) InstructionPath() string` | `string` | Returns `".claude/CLAUDE.md"` |
| `BuildSpawnCommand` | `func (r *Runtime) BuildSpawnCommand(opts) string` | `string` | Builds `claude --dangerously-skip-permissions --model X -p "prompt"` command |
| `DeployConfig` | `func (r *Runtime) DeployConfig(ctx, path, overlay, hooks) error` | `error` | Writes `.claude/CLAUDE.md` (overlay content) and `.claude/settings.local.json` (allowed/denied tools, file patterns) |
| `DetectReady` | `func (r *Runtime) DetectReady(paneContent) ReadyState` | `ReadyState` | Parses pane for `>`, `Y/n`, loading indicators |
| `ParseTranscript` | `func (r *Runtime) ParseTranscript(path) (*TranscriptSummary, error)` | `*TranscriptSummary, error` | Reads JSONL transcript, sums input/output tokens |
| `BuildEnv` | `func (r *Runtime) BuildEnv(model) map[string]string` | `map[string]string` | Returns ANTHROPIC_API_KEY from environment |
| `RequiresBeaconVerification` | `func (r *Runtime) RequiresBeaconVerification() bool` | `bool` | Returns true (Claude needs beacon resend loop) |

## Data Types

### Runtime (struct)
Empty struct; all state is passed via method parameters.

## Logging
- No structured logging; errors returned via `fmt.Errorf`

## CRUD Entry Points
- **Create**: `DeployConfig()` creates `.claude/CLAUDE.md` and `.claude/settings.local.json` in the worktree
- **Read**: `DetectReady()` reads pane content, `ParseTranscript()` reads JSONL files

## Style Guide
- Self-registering: `init()` calls `runtimes.RegisterRuntime(New())`
- Permission mode: `--dangerously-skip-permissions` for automated operation
- Settings file format matches Claude Code's `settings.local.json` schema
- Readiness detection heuristics: checks for prompt indicators (`>`) and dialog patterns (`Y/n`)

**Representative snippet (from `claude.go`):**
```go
func (r *Runtime) DeployConfig(ctx context.Context, worktreePath string,
	overlay *runtimes.OverlayContent, hooks *runtimes.HooksDef) error {

	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Write CLAUDE.md with overlay content.
	claudeMD := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte(overlay.Content), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	// Write settings.local.json with hooks/guards.
	settings := buildSettings(hooks)
	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	return os.WriteFile(settingsPath, data, 0o644)
}
```
