// Package blueprint provides the blueprint engine for structured task definition,
// CRUD operations, and execution lifecycle management.
package blueprint

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Status represents the lifecycle state of a blueprint.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusPassed    Status = "passed"
	StatusFailed    Status = "failed"
	StatusBlocked   Status = "blocked"
	StatusCancelled Status = "cancelled"
)

// Blueprint defines a structured task specification.
type Blueprint struct {
	ID            string         `json:"id" yaml:"id"`
	Version       int            `json:"version" yaml:"version"`
	Name          string         `json:"name" yaml:"name"`
	Agent         string         `json:"agent" yaml:"agent"`
	Capability    string         `json:"capability" yaml:"capability"`
	ContextGrants []ContextGrant `json:"context" yaml:"context"`
	Inputs        Inputs         `json:"inputs" yaml:"inputs"`
	Outputs       Outputs        `json:"outputs" yaml:"outputs"`
	VerifySteps   []VerifyStep   `json:"verify" yaml:"verify"`
	Gates         []string       `json:"gates" yaml:"gates"`
	DependsOn     []string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	RetryLimit    int            `json:"retry_limit" yaml:"retry_limit"`
	Timeout       string         `json:"timeout" yaml:"timeout"`
	Status        Status         `json:"status,omitempty" yaml:"-"`
	Attempts      int            `json:"attempts,omitempty" yaml:"-"`
	LastError     string         `json:"last_error,omitempty" yaml:"-"`
	CreatedAt     time.Time      `json:"created" yaml:"created"`
	UpdatedAt     time.Time      `json:"updated" yaml:"updated"`
}

// ContextGrant defines a filesystem access grant.
type ContextGrant struct {
	Action string `json:"action" yaml:"action"` // "read" or "write"
	Path   string `json:"path" yaml:"path"`     // Glob pattern
}

// Inputs defines the task inputs.
type Inputs struct {
	Spec         string            `json:"spec" yaml:"spec"`
	Dependencies []string          `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Env          map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// Outputs defines the expected task outputs.
type Outputs struct {
	Files []string `json:"files,omitempty" yaml:"files,omitempty"`
	Tests []string `json:"tests,omitempty" yaml:"tests,omitempty"`
}

// VerifyStep defines a verification command.
type VerifyStep struct {
	Command string `json:"command" yaml:"command"`
	Expect  string `json:"expect" yaml:"expect"` // exit_0, contains, not_contains, regex
	Value   string `json:"value,omitempty" yaml:"value,omitempty"`
}

// BlueprintRun represents a single execution of a blueprint.
type BlueprintRun struct {
	ID          string    `json:"id"`
	BlueprintID string    `json:"blueprint_id"`
	AgentID     string    `json:"agent_id,omitempty"`
	Status      Status    `json:"status"`
	Attempt     int       `json:"attempt"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	TraceID     string    `json:"trace_id,omitempty"`
}

// GenerateBlueprintID creates a new blueprint ID with "bp-" prefix.
func GenerateBlueprintID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "bp-" + hex.EncodeToString(b)
}

// GenerateRunID creates a new blueprint run ID with "bpr-" prefix.
func GenerateRunID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "bpr-" + hex.EncodeToString(b)
}

// ValidStatus checks if a status string is valid.
func ValidStatus(s Status) bool {
	switch s {
	case StatusPending, StatusRunning, StatusPassed, StatusFailed, StatusBlocked, StatusCancelled:
		return true
	}
	return false
}

// ValidTransition checks if a status transition is legal.
func ValidTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusRunning || to == StatusBlocked || to == StatusCancelled
	case StatusRunning:
		return to == StatusPassed || to == StatusFailed || to == StatusCancelled
	case StatusBlocked:
		return to == StatusPending || to == StatusCancelled
	case StatusFailed:
		return to == StatusPending || to == StatusCancelled // allow retry
	}
	return false
}
