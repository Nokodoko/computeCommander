// Package goose implements the AgentRuntime adapter for Goose.
package goose

import (
	"context"
	"os"
	"strings"

	"github.com/noko/computecommander/pkg/runtimes"
)

func init() {
	runtimes.RegisterRuntime(New())
}

// Runtime implements runtimes.AgentRuntime for Goose (stub).
type Runtime struct{}

// New returns a new Goose runtime adapter.
func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() runtimes.RuntimeID {
	return runtimes.RuntimeGoose
}

func (r *Runtime) InstructionPath() string {
	return ".goose/instructions.md"
}

func (r *Runtime) BuildSpawnCommand(opts runtimes.SpawnOpts) string {
	var parts []string
	parts = append(parts, "goose")

	if opts.Model != "" {
		parts = append(parts, "--model", opts.Model)
	}

	if opts.SystemPrompt != "" {
		parts = append(parts, opts.SystemPrompt)
	}

	return strings.Join(parts, " ")
}

func (r *Runtime) BuildPrintCommand(prompt string, model string) []string {
	args := []string{"goose"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	return args
}

func (r *Runtime) DeployConfig(_ context.Context, _ string,
	_ *runtimes.OverlayContent, _ *runtimes.HooksDef) error {
	// Stub: Goose config deployment not yet implemented.
	return nil
}

func (r *Runtime) DetectReady(paneContent string) runtimes.ReadyState {
	// Stub: basic detection.
	if strings.Contains(paneContent, ">") {
		return runtimes.ReadyState{Phase: "ready"}
	}
	return runtimes.ReadyState{Phase: "loading"}
}

func (r *Runtime) ParseTranscript(_ string) (*runtimes.TranscriptSummary, error) {
	// Stub: transcript parsing not yet implemented.
	return &runtimes.TranscriptSummary{}, nil
}

func (r *Runtime) BuildEnv(model string) map[string]string {
	env := map[string]string{}
	if key := os.Getenv("GOOSE_API_KEY"); key != "" {
		env["GOOSE_API_KEY"] = key
	}
	return env
}

func (r *Runtime) RequiresBeaconVerification() bool {
	return false // TBD per spec
}

func (r *Runtime) Connect(_ runtimes.ProcessHandle) runtimes.RuntimeConnection {
	return nil
}
