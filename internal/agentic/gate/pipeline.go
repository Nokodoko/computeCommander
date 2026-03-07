// Package gate provides the quality gate pipeline that runs mandatory checks
// (lint, typecheck, test, security, format) before agent output is accepted.
package gate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// GateName is a recognized quality gate type.
type GateName string

const (
	GateLint      GateName = "lint"
	GateTypecheck GateName = "typecheck"
	GateTest      GateName = "test"
	GateSecurity  GateName = "security"
	GateFormat    GateName = "format"
)

// ValidGateName checks if a gate name is valid.
func ValidGateName(name string) bool {
	switch GateName(name) {
	case GateLint, GateTypecheck, GateTest, GateSecurity, GateFormat:
		return true
	}
	return false
}

// GateResult records the outcome of a single quality gate check.
type GateResult struct {
	ID            string   `json:"id"`
	BlueprintID   string   `json:"blueprint_id"`
	AgentID       string   `json:"agent_id"`
	GateName      GateName `json:"gate_name"`
	Passed        bool     `json:"passed"`
	Command       string   `json:"command"`
	ExitCode      int      `json:"exit_code"`
	StdoutExcerpt string   `json:"stdout_excerpt"`
	StderrExcerpt string   `json:"stderr_excerpt"`
	DurationMs    int      `json:"duration_ms"`
	Attempt       int      `json:"attempt"`
	CreatedAt     time.Time `json:"created_at"`
	TraceID       string   `json:"trace_id,omitempty"`
}

// GateConfig defines a single gate in the pipeline.
type GateConfig struct {
	Name    GateName `json:"name"`
	Command string   `json:"command"`
	Enabled bool     `json:"enabled"`
	Timeout time.Duration `json:"-"`
}

// GatePipeline manages the quality gate pipeline: running gates in sequence,
// recording results, and enforcing pass/fail logic.
type GatePipeline struct {
	db     db.DB
	gates  []GateConfig
	runner CommandRunner
}

// CommandRunner abstracts shell command execution for testability.
type CommandRunner interface {
	Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error)
}

// NewGatePipeline creates a new pipeline with the given gates.
func NewGatePipeline(database db.DB, gates []GateConfig, runner CommandRunner) *GatePipeline {
	return &GatePipeline{
		db:     database,
		gates:  gates,
		runner: runner,
	}
}

// PipelineResult is the aggregate outcome of running all gates.
type PipelineResult struct {
	Passed  bool           `json:"passed"`
	Results []*GateResult  `json:"results"`
	Failed  []string       `json:"failed,omitempty"`  // Names of failed gates
}

// Run executes all enabled gates in sequence. Gates run sequentially to avoid
// interleaved output. Stops at the first failure.
func (p *GatePipeline) Run(ctx context.Context, blueprintID, agentID string, attempt int) (*PipelineResult, error) {
	result := &PipelineResult{Passed: true}

	for _, gate := range p.gates {
		if !gate.Enabled {
			continue
		}

		gr, err := p.runGate(ctx, gate, blueprintID, agentID, attempt)
		if err != nil {
			return nil, err
		}

		result.Results = append(result.Results, gr)

		if !gr.Passed {
			result.Passed = false
			result.Failed = append(result.Failed, string(gate.Name))
			break // Stop at first failure
		}
	}

	return result, nil
}

// runGate executes a single gate configuration and records the result.
// Context cancel functions are properly cleaned up per invocation.
func (p *GatePipeline) runGate(ctx context.Context, gate GateConfig, blueprintID, agentID string, attempt int) (*GateResult, error) {
	gateCtx := ctx
	if gate.Timeout > 0 {
		var cancel context.CancelFunc
		gateCtx, cancel = context.WithTimeout(ctx, gate.Timeout)
		defer cancel()
	}

	start := time.Now()
	stdout, stderr, exitCode, err := p.runner.Run(gateCtx, gate.Command)
	duration := time.Since(start)

	gr := &GateResult{
		ID:            generateGateID(),
		BlueprintID:   blueprintID,
		AgentID:       agentID,
		GateName:      gate.Name,
		Passed:        exitCode == 0 && err == nil,
		Command:       gate.Command,
		ExitCode:      exitCode,
		StdoutExcerpt: truncate(stdout, 2000),
		StderrExcerpt: truncate(stderr, 2000),
		DurationMs:    int(duration.Milliseconds()),
		Attempt:       attempt,
		CreatedAt:     time.Now().UTC(),
	}

	if err != nil {
		gr.Passed = false
		gr.StderrExcerpt = truncate(err.Error(), 2000)
	}

	if recordErr := p.RecordResult(ctx, gr); recordErr != nil {
		return nil, fmt.Errorf("record gate result: %w", recordErr)
	}

	return gr, nil
}

// RunSingle executes a single named gate.
func (p *GatePipeline) RunSingle(ctx context.Context, gateName GateName, blueprintID, agentID string, attempt int) (*GateResult, error) {
	for _, gate := range p.gates {
		if gate.Name != gateName {
			continue
		}

		start := time.Now()
		stdout, stderr, exitCode, err := p.runner.Run(ctx, gate.Command)
		duration := time.Since(start)

		gr := &GateResult{
			ID:            generateGateID(),
			BlueprintID:   blueprintID,
			AgentID:       agentID,
			GateName:      gate.Name,
			Passed:        exitCode == 0 && err == nil,
			Command:       gate.Command,
			ExitCode:      exitCode,
			StdoutExcerpt: truncate(stdout, 2000),
			StderrExcerpt: truncate(stderr, 2000),
			DurationMs:    int(duration.Milliseconds()),
			Attempt:       attempt,
			CreatedAt:     time.Now().UTC(),
		}

		if err != nil {
			gr.Passed = false
			gr.StderrExcerpt = truncate(err.Error(), 2000)
		}

		if recordErr := p.RecordResult(ctx, gr); recordErr != nil {
			return nil, fmt.Errorf("record gate result: %w", recordErr)
		}

		return gr, nil
	}

	return nil, fmt.Errorf("gate %q not found in pipeline", gateName)
}

// RecordResult persists a gate result to the database.
func (p *GatePipeline) RecordResult(ctx context.Context, gr *GateResult) error {
	query := `INSERT INTO gate_results (
		id, blueprint_id, agent_id, gate_name, passed,
		command, exit_code, stdout_excerpt, stderr_excerpt,
		duration_ms, attempt, created_at, trace_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	passed := 0
	if gr.Passed {
		passed = 1
	}

	return p.db.Exec(ctx, query,
		gr.ID, gr.BlueprintID, gr.AgentID, string(gr.GateName), passed,
		gr.Command, gr.ExitCode, gr.StdoutExcerpt, gr.StderrExcerpt,
		gr.DurationMs, gr.Attempt, gr.CreatedAt.Format(time.RFC3339),
		nilIfEmpty(gr.TraceID),
	)
}

// History retrieves gate results for a blueprint.
func (p *GatePipeline) History(ctx context.Context, blueprintID string, limit int) ([]*GateResult, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `SELECT id, blueprint_id, agent_id, gate_name, passed,
		command, exit_code, stdout_excerpt, stderr_excerpt,
		duration_ms, attempt, created_at, trace_id
		FROM gate_results WHERE blueprint_id = ?
		ORDER BY created_at DESC LIMIT ?`

	rows, err := p.db.Query(ctx, query, blueprintID, limit)
	if err != nil {
		return nil, fmt.Errorf("query gate history: %w", err)
	}
	defer rows.Close()

	var results []*GateResult
	for rows.Next() {
		gr := &GateResult{}
		var passed int
		var traceID *string
		var createdAt string
		if err := rows.Scan(
			&gr.ID, &gr.BlueprintID, &gr.AgentID, &gr.GateName, &passed,
			&gr.Command, &gr.ExitCode, &gr.StdoutExcerpt, &gr.StderrExcerpt,
			&gr.DurationMs, &gr.Attempt, &createdAt, &traceID,
		); err != nil {
			return nil, fmt.Errorf("scan gate result: %w", err)
		}
		gr.Passed = passed == 1
		gr.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if traceID != nil {
			gr.TraceID = *traceID
		}
		results = append(results, gr)
	}

	return results, rows.Err()
}

// ListGates returns the configured gates.
func (p *GatePipeline) ListGates() []GateConfig {
	result := make([]GateConfig, len(p.gates))
	copy(result, p.gates)
	return result
}

func generateGateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "gate-" + hex.EncodeToString(b)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
