// Package icarus implements the AgentRuntime adapter for the Icarus
// Go-native coding agent harness.
//
// Icarus is a sister-project Go binary (`/home/n0ko/Programs/ai/icarus`) that
// provides a charm-bracelet REPL/TUI, a TypedBus hook system, and a multi-
// provider LLM client (Anthropic, OpenAI, Gemini, Ollama, LMStudio). It speaks
// the same provider-agnostic effort dialect as cmdr's pi runtime
// (off / minimal / low / medium / high / xhigh) per icarus_cc_parity §5.
//
// This adapter is the cmdr-side of the icarus_cc_parity §7 integration. The
// icarus-side emitters (T8) consume the contract this file ships:
//
//   Binary:  icarus
//   Modes:   `icarus run "<prompt>"` (one-shot, headless)
//            `icarus repl`           (interactive)
//   Flags:   --model <id>
//            --effort <off|minimal|low|medium|high|xhigh>
//            --append-system-prompt <text>
//   Env:     ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY,
//            OB_API_KEY, ICARUS_HOME, ICARUS_SESSION_ID
//
// Effort pass-through: callers populate SpawnOpts.Env["ICARUS_EFFORT"]
// (or set the ICARUS_EFFORT environment variable in the caller's env), and
// BuildSpawnCommand materializes that into a `--effort <value>` CLI flag.
// This keeps the AgentRuntime interface unchanged while threading the
// runtime-specific effort knob through cmdr's existing spawn pipeline.
package icarus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/noko/computecommander/pkg/runtimes"
)

// RuntimeIcarus is the string identifier registered in the runtime registry.
// Mirrors runtimes.RuntimeIcarus and is exported here for callers that import
// the icarus subpackage directly (e.g., agent yaml loaders that resolve a
// runtime by name without depending on the parent package's enum).
const RuntimeIcarus = "icarus"

// validEffortLevels matches icarus_cc_parity §5 — the effort enum threaded
// through every provider in the icarus binary. Values outside this set are
// dropped from the spawn command rather than passed through, so a malformed
// caller input cannot poison the icarus CLI invocation.
var validEffortLevels = map[string]struct{}{
	"off":     {},
	"minimal": {},
	"low":     {},
	"medium":  {},
	"high":    {},
	"xhigh":   {},
}

// Runtime implements runtimes.AgentRuntime for the Icarus coding agent.
type Runtime struct{}

// New returns a new Icarus runtime adapter.
func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimeIcarus
}

// InstructionPath returns the relative path to icarus's instruction file.
// Icarus reads project-local CLAUDE.md (the file is also consumed by Claude
// Code and shared with pi via symlink), keeping the worktree-deployed
// instruction surface consistent across runtimes.
func (r *Runtime) InstructionPath() string {
	return ".claude/CLAUDE.md"
}

// BuildSpawnCommand assembles the shell command to launch icarus in headless
// (one-shot) mode. The resulting argv mirrors:
//
//	icarus run --model <model> --effort <level> --append-system-prompt '...' < prompt-file
//
// Effort resolution order:
//  1. SpawnOpts.Env["ICARUS_EFFORT"] (caller-supplied per-spawn override)
//  2. parent process ICARUS_EFFORT env var
//  3. omitted (icarus binary applies its settings.json default)
func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	parts := []string{"icarus", "run"}

	if opts.Model != "" {
		parts = append(parts, "--model", opts.Model)
	}

	if effort := resolveEffort(opts); effort != "" {
		parts = append(parts, "--effort", effort)
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

// BuildPrintCommand returns argv for a headless one-shot icarus invocation
// suitable for use with exec.Command (no shell quoting / redirection).
func (r *Runtime) BuildPrintCommand(prompt string, model string) []string {
	args := []string{"icarus", "run"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

// settingsJSON is the structure for icarus's settings.json used to deploy
// per-worktree configuration. Mirrors the shape declared in
// icarus_cc_parity §5 (the effort knob block) and §3.4 (ob1 api key).
type settingsJSON struct {
	Providers *providersBlock `json:"providers,omitempty"`
	OB1       *ob1Block       `json:"ob1,omitempty"`
}

type providersBlock struct {
	DefaultEffort string `json:"default_effort,omitempty"`
}

type ob1Block struct {
	APIKeyEnv string `json:"api_key_env,omitempty"`
}

// DeployConfig writes the worktree-local instruction overlay (.claude/CLAUDE.md)
// and a placeholder .icarus/settings.json so icarus picks up consistent
// defaults when invoked inside the worktree. Hook rules from cmdr are not
// translated into icarus tool-permission settings here — icarus's TypedBus
// permission model lives outside this adapter and will be wired in T8.
func (r *Runtime) DeployConfig(ctx context.Context, worktreePath string,
	overlay *runtimes.OverlayContent, hooks *runtimes.HooksDef) error {

	claudeDir := filepath.Join(worktreePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return fmt.Errorf("create .claude dir: %w", err)
	}

	// Write the shared instruction file (icarus reads CLAUDE.md).
	instrPath := filepath.Join(worktreePath, r.InstructionPath())
	content := ""
	if overlay != nil {
		content = overlay.Content
	}
	if err := os.WriteFile(instrPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write instruction file: %w", err)
	}

	// Write per-worktree icarus settings if the caller provided hook rules.
	// We do NOT translate AllowedTools / DeniedTools here because icarus's
	// permission model differs from Claude Code's; that translation belongs
	// in T8 alongside the icarus-side cmdr emitters.
	if hooks != nil && hooks.Rules != nil {
		icarusDir := filepath.Join(worktreePath, ".icarus")
		if err := os.MkdirAll(icarusDir, 0o755); err != nil {
			return fmt.Errorf("create .icarus dir: %w", err)
		}

		settings := settingsJSON{
			OB1: &ob1Block{APIKeyEnv: "OB_API_KEY"},
		}
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal icarus settings: %w", err)
		}

		settingsPath := filepath.Join(icarusDir, "settings.json")
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			return fmt.Errorf("write icarus settings: %w", err)
		}
	}

	return nil
}

// DetectReady inspects pane content to decide whether icarus is ready for
// input. The charm-bracelet REPL prints a `> ` prompt once the welcome card
// finishes rendering; the welcome card itself uses box-drawing characters.
func (r *Runtime) DetectReady(paneContent string) runtimes.ReadyState {
	lower := strings.ToLower(paneContent)

	// Charm REPL prompt indicators.
	if strings.Contains(lower, "> ") || strings.Contains(lower, "╭") {
		return runtimes.ReadyState{Phase: "ready"}
	}

	// Permission / trust dialogs.
	if strings.Contains(lower, "allow") || strings.Contains(lower, "deny") {
		return runtimes.ReadyState{Phase: "dialog", Action: "permission"}
	}
	if strings.Contains(lower, "trust") || strings.Contains(lower, "approve") {
		return runtimes.ReadyState{Phase: "dialog", Action: "trust"}
	}

	return runtimes.ReadyState{Phase: "loading"}
}

// ParseTranscript reads icarus's JSONL session transcript and aggregates
// token usage. Icarus emits per-line records mirroring Claude/pi (the
// `model` field plus a `usage` map with `input_tokens` / `output_tokens` —
// snake_case is canonical in icarus).
func (r *Runtime) ParseTranscript(path string) (*runtimes.TranscriptSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}

	summary := &runtimes.TranscriptSummary{}
	for _, line := range strings.Split(string(data), "\n") {
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

// BuildEnv returns the runtime-specific environment variables icarus expects.
// Multi-provider keys mirror the icarus settings.json provider block.
// OB_API_KEY is forwarded so icarus's pkg/ob1client can authenticate
// (icarus_cc_parity §4).
func (r *Runtime) BuildEnv(model string) map[string]string {
	env := map[string]string{}
	for _, k := range []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GOOGLE_API_KEY",
		"OB_API_KEY",
		"ICARUS_HOME",
	} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	if model != "" {
		env["ICARUS_MODEL"] = model
	}
	return env
}

// RequiresBeaconVerification returns false. Icarus uses TypedBus events for
// readiness signaling and does not need cmdr's beacon resend loop.
func (r *Runtime) RequiresBeaconVerification() bool {
	return false
}

// Connect returns nil. Icarus does not yet expose a JSON-RPC channel for
// in-process control; the icarus_cc_parity spec keeps T7 limited to
// process-spawn integration.
func (r *Runtime) Connect(process runtimes.ProcessHandle) runtimes.RuntimeConnection {
	return nil
}

// resolveEffort selects the effort level for this spawn from (in order):
// per-spawn env override, parent-process env, then empty (let icarus apply
// its settings.json default).
func resolveEffort(opts runtimes.SpawnOpts) string {
	if opts.Env != nil {
		if v, ok := opts.Env["ICARUS_EFFORT"]; ok && validEffort(v) {
			return v
		}
	}
	if v := os.Getenv("ICARUS_EFFORT"); v != "" && validEffort(v) {
		return v
	}
	return ""
}

func validEffort(v string) bool {
	_, ok := validEffortLevels[strings.ToLower(strings.TrimSpace(v))]
	return ok
}

// shellQuote wraps a string in single quotes, escaping internal single quotes.
// Mirrors the helper in pkg/runtimes/{claude,pi}.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
