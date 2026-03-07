package gate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellRunner implements CommandRunner by executing shell commands.
type ShellRunner struct {
	WorkDir string
}

// NewShellRunner creates a ShellRunner that executes commands in the given directory.
func NewShellRunner(workDir string) *ShellRunner {
	return &ShellRunner{WorkDir: workDir}
}

// Run executes a shell command and returns stdout, stderr, exit code.
func (r *ShellRunner) Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if r.WorkDir != "" {
		cmd.Dir = r.WorkDir
	}

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), -1, err
	}

	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// DefaultGateConfigs returns the standard quality gate pipeline.
func DefaultGateConfigs() []GateConfig {
	return []GateConfig{
		{Name: GateFormat, Command: "gofmt -l .", Enabled: true, Timeout: 2 * time.Minute},
		{Name: GateLint, Command: "golangci-lint run", Enabled: true, Timeout: 5 * time.Minute},
		{Name: GateTypecheck, Command: "go vet ./...", Enabled: true, Timeout: 5 * time.Minute},
		{Name: GateTest, Command: "go test ./...", Enabled: true, Timeout: 10 * time.Minute},
		{Name: GateSecurity, Command: "gosec ./...", Enabled: false, Timeout: 5 * time.Minute},
	}
}

// GateOrder returns the canonical execution order of gates.
func GateOrder() []GateName {
	return []GateName{GateFormat, GateLint, GateTypecheck, GateTest, GateSecurity}
}

// MockRunner is a test double for CommandRunner.
type MockRunner struct {
	Results map[string]MockResult
}

// MockResult holds preconfigured output for a command.
type MockResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// NewMockRunner creates a MockRunner with preset results.
func NewMockRunner(results map[string]MockResult) *MockRunner {
	return &MockRunner{Results: results}
}

// Run returns the preset result for the given command.
func (m *MockRunner) Run(_ context.Context, command string) (string, string, int, error) {
	if r, ok := m.Results[command]; ok {
		return r.Stdout, r.Stderr, r.ExitCode, r.Err
	}
	return "", fmt.Sprintf("mock: command %q not configured", command), 1, nil
}
