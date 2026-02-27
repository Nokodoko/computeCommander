// Package codex implements the AgentRuntime adapter for Codex CLI.
package codex

import (
	"context"
	"os"
	"strings"

	"github.com/noko/computecommander/pkg/runtimes"
)

func init() {
	runtimes.RegisterRuntime(New())
}

// Runtime implements runtimes.AgentRuntime for Codex CLI (stub).
type Runtime struct{}

// New returns a new Codex CLI runtime adapter.
func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimeCodex
}

func (r *Runtime) InstructionPath() string {
	return "AGENTS.md"
}

func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	var parts []string
	parts = append(parts, "codex")

	if opts.Model != "" {
		parts = append(parts, "--model", opts.Model)
	}

	if opts.SystemPrompt != "" {
		parts = append(parts, opts.SystemPrompt)
	}

	return strings.Join(parts, " ")
}

func (r *Runtime) BuildPrintCommand(prompt string, model string) []string {
	args := []string{"codex"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

func (r *Runtime) DeployConfig(_ context.Context, _ string,
	_ *runtimes.OverlayContent, _ *runtimes.HooksDef) error {
	// Stub: Codex config deployment not yet implemented.
	return nil
}

func (r *Runtime) DetectReady(_ string) runtimes.ReadyState {
	// Codex is headless per spec; assume ready.
	return runtimes.ReadyState{Phase: "ready"}
}

func (r *Runtime) ParseTranscript(_ string) (*runtimes.TranscriptSummary, error) {
	// Stub: transcript parsing not yet implemented.
	return &runtimes.TranscriptSummary{}, nil
}

func (r *Runtime) BuildEnv(model string) map[string]string {
	env := map[string]string{}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		env["OPENAI_API_KEY"] = key
	}
	return env
}

func (r *Runtime) RequiresBeaconVerification() bool {
	return false // Headless per spec
}

func (r *Runtime) Connect(_ runtimes.ProcessHandle) runtimes.RuntimeConnection {
	return nil
}
