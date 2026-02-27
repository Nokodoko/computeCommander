// Package runtimes defines the AgentRuntime interface and supporting types
// for pluggable AI coding agent backends (spec section 7.1).
package runtimes

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// RuntimeID identifies a supported agent runtime.
type RuntimeID string

const (
	RuntimeClaude RuntimeID = "claude"
	RuntimeGemini RuntimeID = "gemini"
	RuntimeCodex  RuntimeID = "codex"
	RuntimePi     RuntimeID = "pi"
	RuntimeGoose  RuntimeID = "goose"
)

// SpawnOpts configures agent process spawning.
type SpawnOpts struct {
	Model          string            // Model identifier
	PermissionMode string            // "bypass" | "ask"
	SystemPrompt   string            // Optional prefix
	AppendPrompt   string            // Optional suffix
	PromptFile     string            // File path for long prompts
	WorkDir        string            // Working directory
	Env            map[string]string // Additional environment variables
}

// ReadyState represents agent TUI initialization phases.
type ReadyState struct {
	Phase  string // "loading" | "dialog" | "ready"
	Action string // Dialog action if Phase == "dialog"
}

// OverlayContent contains runtime-agnostic instructions.
type OverlayContent struct {
	Content string // Full markdown text
}

// QualityGate defines a quality gate command for hooks.
type QualityGate struct {
	Name        string
	Command     string
	Description string
}

// HookRules contains hook rule configuration.
type HookRules struct {
	AllowedTools []string
	DeniedTools  []string
	FilePatterns []string
}

// HooksDef contains guard/hook configuration.
type HooksDef struct {
	AgentName    string
	Capability   string
	WorktreePath string
	QualityGates []QualityGate
	FileScope    []string
	Rules        *HookRules
}

// TranscriptSummary contains parsed token usage.
type TranscriptSummary struct {
	InputTokens  int64
	OutputTokens int64
	Model        string
}

// ConnectionState for RPC-capable runtimes.
type ConnectionState struct {
	Status      string  // "idle" | "working" | "error"
	CurrentTool *string // Tool in progress
}

// RuntimeConnection for direct RPC communication.
type RuntimeConnection interface {
	SendPrompt(text string) error
	FollowUp(text string) error
	Abort() error
	GetState() (*ConnectionState, error)
	Close() error
}

// ProcessHandle for RPC-capable runtimes.
type ProcessHandle interface {
	Stdin() io.Writer
	Stdout() io.Reader
}

// AgentRuntime is the contract all runtime adapters must implement.
type AgentRuntime interface {
	// ID returns the unique runtime identifier.
	ID() RuntimeID

	// InstructionPath returns the relative path to instruction file.
	// e.g., ".claude/CLAUDE.md" or "AGENTS.md"
	InstructionPath() string

	// BuildSpawnCommand returns the shell command to spawn an agent.
	BuildSpawnCommand(opts SpawnOpts) string

	// BuildPrintCommand returns argv for headless one-shot AI calls.
	BuildPrintCommand(prompt string, model string) []string

	// DeployConfig deploys instructions and hooks to a worktree.
	DeployConfig(ctx context.Context, worktreePath string,
		overlay *OverlayContent, hooks *HooksDef) error

	// DetectReady parses pane content to determine agent readiness.
	DetectReady(paneContent string) ReadyState

	// ParseTranscript extracts token usage from session transcript.
	ParseTranscript(path string) (*TranscriptSummary, error)

	// BuildEnv returns runtime-specific environment variables.
	BuildEnv(model string) map[string]string

	// RequiresBeaconVerification returns whether the beacon resend loop is needed.
	RequiresBeaconVerification() bool

	// Connect establishes RPC connection (optional, nil if not supported).
	Connect(process ProcessHandle) RuntimeConnection
}

// registry holds registered runtimes keyed by string ID.
var (
	registryMu sync.RWMutex
	registry   = make(map[string]AgentRuntime)
)

// RegisterRuntime registers an AgentRuntime in the global registry.
func RegisterRuntime(runtime AgentRuntime) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[string(runtime.ID())] = runtime
}

// GetRuntime returns a registered AgentRuntime by string ID.
func GetRuntime(id string) (AgentRuntime, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	rt, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("unknown runtime: %q", id)
	}
	return rt, nil
}

// AllRuntimeIDs returns all valid runtime ID constants.
func AllRuntimeIDs() []RuntimeID {
	return []RuntimeID{
		RuntimeClaude,
		RuntimeGemini,
		RuntimeCodex,
		RuntimePi,
		RuntimeGoose,
	}
}
