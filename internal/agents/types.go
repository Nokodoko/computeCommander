// Package agents provides agent lifecycle management including spawning,
// overlay generation, guard enforcement, and session tracking.
package agents

import (
	"time"

	"github.com/noko/computecommander/pkg/runtimes"
)

// Capability represents the role an agent plays in the hierarchy.
type Capability string

const (
	CapScout       Capability = "scout"
	CapBuilder     Capability = "builder"
	CapReviewer    Capability = "reviewer"
	CapLead        Capability = "lead"
	CapMerger      Capability = "merger"
	CapCoordinator Capability = "coordinator"
	CapSupervisor  Capability = "supervisor"
	CapMonitor     Capability = "monitor"
)

// AllCapabilities returns every valid capability value.
func AllCapabilities() []Capability {
	return []Capability{
		CapScout, CapBuilder, CapReviewer, CapLead,
		CapMerger, CapCoordinator, CapSupervisor, CapMonitor,
	}
}

// ValidCapability returns true if c is a recognized capability.
func ValidCapability(c Capability) bool {
	for _, v := range AllCapabilities() {
		if v == c {
			return true
		}
	}
	return false
}

// SessionState represents the lifecycle state of an agent session.
type SessionState string

const (
	StateBooting   SessionState = "booting"
	StateWorking   SessionState = "working"
	StateCompleted SessionState = "completed"
	StateStalled   SessionState = "stalled"
	StateZombie    SessionState = "zombie"
)

// AllSessionStates returns every valid session state.
func AllSessionStates() []SessionState {
	return []SessionState{
		StateBooting, StateWorking, StateCompleted,
		StateStalled, StateZombie,
	}
}

// ValidSessionState returns true if s is a recognized state.
func ValidSessionState(s SessionState) bool {
	for _, v := range AllSessionStates() {
		if v == s {
			return true
		}
	}
	return false
}

// AgentSession matches the sessions table in the database schema (spec section 4.1).
type AgentSession struct {
	ID              string             `json:"id" db:"id"`
	AgentName       string             `json:"agentName" db:"agent_name"`
	Capability      Capability         `json:"capability" db:"capability"`
	WorktreePath    string             `json:"worktreePath" db:"worktree_path"`
	BranchName      string             `json:"branchName" db:"branch_name"`
	TaskID          string             `json:"taskId" db:"task_id"`
	ZellijPane      string             `json:"zellijPane" db:"zellij_pane"`
	ZellijSession   string             `json:"zellijSession" db:"zellij_session"`
	State           SessionState       `json:"state" db:"state"`
	PID             int                `json:"pid" db:"pid"`
	ParentAgent     string             `json:"parentAgent" db:"parent_agent"`
	Depth           int                `json:"depth" db:"depth"`
	RunID           string             `json:"runId" db:"run_id"`
	StartedAt       time.Time          `json:"startedAt" db:"started_at"`
	LastActivity    time.Time          `json:"lastActivity" db:"last_activity"`
	EscalationLevel int                `json:"escalationLevel" db:"escalation_level"`
	StalledSince    *time.Time         `json:"stalledSince" db:"stalled_since"`
	TranscriptPath  string             `json:"transcriptPath" db:"transcript_path"`
	Runtime         runtimes.RuntimeID `json:"runtime" db:"runtime"`

	// v2: Project and color fields
	ProjectID  string `json:"projectId" db:"project_id"`
	ColorIndex int    `json:"colorIndex" db:"color_index"`
	ColorHex   string `json:"colorHex" db:"color_hex"`

	// v3: Model and session name fields (migration 010)
	Model       string `json:"model" db:"model"`
	SessionName string `json:"sessionName" db:"session_name"`

	// Token usage aggregated from the metrics table (populated by ListSessions JOIN).
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
}

// DisplayColorHex returns the color hex to use for rendering, applying the
// gold override for completed agents.
func (s *AgentSession) DisplayColorHex() string {
	return ColorForState(s.ColorHex, s.State)
}

// SpawnRequest contains the parameters for spawning a new agent (spec section 3.1.1).
type SpawnRequest struct {
	TaskID     string             `json:"taskId"`
	Capability Capability         `json:"capability"`
	Name       string             `json:"name"`
	Runtime    runtimes.RuntimeID `json:"runtime"`
	Parent     string             `json:"parent"`
	Depth      int                `json:"depth"`
	FileScope  []string           `json:"fileScope"`
	SpecPath   string             `json:"specPath"`
	SkipScout  bool               `json:"skipScout"`
	SkipReview bool               `json:"skipReview"`
	MaxAgents  int                `json:"maxAgents"`
	ProjectID  string             `json:"projectId"`
}

// SpawnResult is the output from a successful spawn.
type SpawnResult struct {
	Session       *AgentSession `json:"session"`
	WorktreePath  string        `json:"worktreePath"`
	ZellijPane    string        `json:"zellijPane"`
	ZellijSession string        `json:"zellijSession"`
	PID           int           `json:"pid"`
}

// StopOpts configures agent stopping behavior.
type StopOpts struct {
	Force  bool // Force-kill the process
	Reason string
}

// ListOpts filters agent session listing.
type ListOpts struct {
	RunID      string
	Capability Capability
	State      SessionState
	Parent     string
	ProjectID  string
	Runtime    runtimes.RuntimeID // Filter by runtime (e.g., "pi", "claude")
}
