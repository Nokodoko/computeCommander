// Package pi implements the AgentRuntime adapter for the Pi coding agent.
//
// Pi is a TypeScript-based AI coding agent (from pi-mono) that supports:
//   - Multiple LLM providers (Google, Anthropic, OpenAI, etc.)
//   - TypeScript extensions (~/.pi/agent/extensions/*.ts)
//   - JSON-RPC 2.0 mode for programmatic control (--mode rpc)
//   - Session management with continue/resume/fork
//   - Tool selection (read, bash, edit, write, grep, find, ls)
//   - Thinking level configuration (off/minimal/low/medium/high/xhigh)
//
// Pi shares agent definitions, commands, and skills with Claude via symlinks:
//
//	~/.pi/agent/agents/  -> ~/.claude/agents/
//	~/.pi/agent/prompts/ -> ~/.claude/commands/
//	~/.pi/agent/skills/  -> ~/.claude/skills/
//	~/.pi/agent/CLAUDE.md -> ~/.claude/CLAUDE.md
//
// Claude hooks (bash/python) are ported as TypeScript extensions via
// the bidirectional-sync.ts extension.
package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/noko/computecommander/pkg/runtimes"
)

func init() {
	runtimes.RegisterRuntime(New())
}

// Runtime implements runtimes.AgentRuntime for the Pi coding agent.
type Runtime struct{}

// New returns a new Pi agent runtime adapter.
func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimePi
}

func (r *Runtime) InstructionPath() string {
	return ".claude/CLAUDE.md" // Shared with Claude per symlink
}

func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	var parts []string
	parts = append(parts, "pi")

	// Pi uses --print / -p for non-interactive (headless) mode,
	// equivalent to Claude's -p flag.
	if opts.PermissionMode == "bypass" {
		// Pi does not have a permission bypass flag; it trusts all tools
		// by default. No flag needed, but we add --no-extensions to prevent
		// hooks from interfering in headless/worktree contexts.
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

func (r *Runtime) BuildPrintCommand(prompt string, model string) []string {
	args := []string{"pi", "-p"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

// settingsJSON is the structure for Pi's settings.json used to deploy
// configuration into a worktree.
type settingsJSON struct {
	DefaultProvider string `json:"defaultProvider,omitempty"`
	DefaultModel    string `json:"defaultModel,omitempty"`
}

func (r *Runtime) DeployConfig(ctx context.Context, worktreePath string,
	overlay *runtimes.OverlayContent, hooks *runtimes.HooksDef) error {

	// Pi shares the .claude directory structure via symlinks, but for
	// worktree-local overrides we write to the pi agent directory.
	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Write instruction file (shared path with Claude).
	instrPath := filepath.Join(worktreePath, r.InstructionPath())
	content := ""
	if overlay != nil {
		content = overlay.Content
	}
	if err := os.WriteFile(instrPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write instruction file: %w", err)
	}

	// Write Pi-specific settings if hooks provide tool rules.
	// Pi does not use settings.local.json but respects the main settings.json
	// in the PI_CODING_AGENT_DIR or project-local .pi/ directory.
	if hooks != nil && hooks.Rules != nil {
		piDir := filepath.Join(worktreePath, ".pi", "agent")
		if err := os.MkdirAll(piDir, 0o755); err != nil {
			return fmt.Errorf("create .pi/agent dir: %w", err)
		}

		settings := settingsJSON{}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal pi settings: %w", err)
		}

		settingsPath := filepath.Join(piDir, "settings.json")
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			return fmt.Errorf("write pi settings: %w", err)
		}
	}

	return nil
}

func (r *Runtime) DetectReady(paneContent string) runtimes.ReadyState {
	lower := strings.ToLower(paneContent)

	// Pi's interactive mode shows a prompt indicator when ready.
	// The prompt displays the model name and a ">" character.
	if strings.Contains(lower, "> ") || strings.Contains(lower, "╭") {
		return runtimes.ReadyState{Phase: "ready"}
	}

	// Pi shows "Allow" for tool permission prompts (if configured).
	if strings.Contains(lower, "allow") || strings.Contains(lower, "deny") {
		return runtimes.ReadyState{Phase: "dialog", Action: "permission"}
	}

	// Pi may show trust/approval prompts for extensions.
	if strings.Contains(lower, "trust") || strings.Contains(lower, "approve") {
		return runtimes.ReadyState{Phase: "dialog", Action: "trust"}
	}

	return runtimes.ReadyState{Phase: "loading"}
}

func (r *Runtime) ParseTranscript(path string) (*runtimes.TranscriptSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}

	summary := &runtimes.TranscriptSummary{}

	// Pi session files are JSONL (one JSON object per line), similar to
	// Claude's format. Each entry may contain model and usage information.
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if model, ok := entry["model"].(string); ok && model != "" {
			summary.Model = model
		}
		if usage, ok := entry["usage"].(map[string]any); ok {
			if in, ok := usage["input_tokens"].(float64); ok {
				summary.InputTokens += int64(in)
			}
			if out, ok := usage["output_tokens"].(float64); ok {
				summary.OutputTokens += int64(out)
			}
		}
		// Pi also uses "inputTokens" / "outputTokens" in some providers.
		if usage, ok := entry["usage"].(map[string]any); ok {
			if in, ok := usage["inputTokens"].(float64); ok {
				summary.InputTokens += int64(in)
			}
			if out, ok := usage["outputTokens"].(float64); ok {
				summary.OutputTokens += int64(out)
			}
		}
	}

	return summary, nil
}

func (r *Runtime) BuildEnv(model string) map[string]string {
	env := map[string]string{}

	// Pi supports multiple providers. Map common API keys.
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		env["ANTHROPIC_API_KEY"] = key
	}
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		env["GOOGLE_API_KEY"] = key
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		env["OPENAI_API_KEY"] = key
	}

	// Pi uses PI_CODING_AGENT_DIR to locate its config directory.
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		env["PI_CODING_AGENT_DIR"] = dir
	}

	return env
}

func (r *Runtime) RequiresBeaconVerification() bool {
	return false // Pi supports JSON-RPC 2.0 for direct communication
}

func (r *Runtime) Connect(process runtimes.ProcessHandle) runtimes.RuntimeConnection {
	// Pi supports JSON-RPC 2.0 via --mode rpc. The connection implementation
	// will wrap stdin/stdout of the process handle for RPC communication.
	// For now, return nil until the RPC client is implemented.
	return nil
}

// shellQuote wraps a string in single quotes, escaping internal single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
