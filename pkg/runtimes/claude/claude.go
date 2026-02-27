// Package claude implements the AgentRuntime adapter for Claude Code CLI.
package claude

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

// Runtime implements runtimes.AgentRuntime for Claude Code.
type Runtime struct{}

// New returns a new Claude Code runtime adapter.
func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimeClaude
}

func (r *Runtime) InstructionPath() string {
	return ".claude/CLAUDE.md"
}

func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	var parts []string
	parts = append(parts, "claude")

	if opts.PermissionMode == "bypass" {
		parts = append(parts, "--dangerously-skip-permissions")
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
	args := []string{"claude", "-p"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

// settingsJSON is the structure for .claude/settings.local.json used to
// deploy hooks and permission configuration into a worktree.
type settingsJSON struct {
	Permissions settingsPermissions `json:"permissions"`
}

type settingsPermissions struct {
	Allow []settingsRule `json:"allow,omitempty"`
	Deny  []settingsRule `json:"deny,omitempty"`
}

type settingsRule struct {
	Tool string `json:"tool"`
}

func (r *Runtime) DeployConfig(ctx context.Context, worktreePath string,
	overlay *runtimes.OverlayContent, hooks *runtimes.HooksDef) error {

	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Write instruction file.
	instrPath := filepath.Join(worktreePath, r.InstructionPath())
	content := ""
	if overlay != nil {
		content = overlay.Content
	}
	if err := os.WriteFile(instrPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write instruction file: %w", err)
	}

	// Write settings.local.json with hook rules if provided.
	if hooks != nil && hooks.Rules != nil {
		settings := settingsJSON{}
		for _, tool := range hooks.Rules.AllowedTools {
			settings.Permissions.Allow = append(settings.Permissions.Allow,
				settingsRule{Tool: tool})
		}
		for _, tool := range hooks.Rules.DeniedTools {
			settings.Permissions.Deny = append(settings.Permissions.Deny,
				settingsRule{Tool: tool})
		}

		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal settings: %w", err)
		}

		settingsPath := filepath.Join(claudeDir, "settings.local.json")
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			return fmt.Errorf("write settings: %w", err)
		}
	}

	return nil
}

func (r *Runtime) DetectReady(paneContent string) runtimes.ReadyState {
	lower := strings.ToLower(paneContent)

	// Check for ready indicators (Claude's prompt ready state).
	if strings.Contains(lower, "> ") || strings.Contains(lower, "╭") {
		return runtimes.ReadyState{Phase: "ready"}
	}

	// Check for permission dialog.
	if strings.Contains(lower, "allow") || strings.Contains(lower, "deny") {
		return runtimes.ReadyState{Phase: "dialog", Action: "permission"}
	}

	// Check for trust dialog.
	if strings.Contains(lower, "trust") {
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
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
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
	}

	return summary, nil
}

func (r *Runtime) BuildEnv(model string) map[string]string {
	env := map[string]string{}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		env["ANTHROPIC_API_KEY"] = key
	}
	if model != "" {
		env["CLAUDE_MODEL"] = model
	}
	return env
}

func (r *Runtime) RequiresBeaconVerification() bool {
	return true
}

func (r *Runtime) Connect(process runtimes.ProcessHandle) runtimes.RuntimeConnection {
	return nil // Claude Code does not support RPC
}

// shellQuote wraps a string in single quotes, escaping internal single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
