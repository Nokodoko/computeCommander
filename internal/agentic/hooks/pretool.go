// Package hooks provides Go implementations of the agentic foundation hooks
// (PreToolUse, PostToolUse, SubagentStart/Stop) that integrate traceability,
// block rules, and isolation enforcement at the tool boundary.
package hooks

import (
	"context"

	"github.com/noko/computecommander/internal/agentic/block"
	"github.com/noko/computecommander/internal/agentic/trace"
)

// PreToolContext holds the context needed for pre-tool-use evaluation.
type PreToolContext struct {
	AgentID    string
	AgentName  string
	Capability string
	SessionID  string
	RunID      string
	TraceID    string
	ParentSpan string
	Tool       string
	Command    string
	FilePath   string
	Depth      int
	Grants     []string // Explicit grants from isolation manifest
}

// PreToolResult is the outcome of pre-tool-use evaluation.
type PreToolResult struct {
	Allowed     bool   `json:"allowed"`
	RuleID      string `json:"rule_id,omitempty"`
	Message     string `json:"message,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Overridden  bool   `json:"overridden,omitempty"`
	RateLimited bool   `json:"rate_limited,omitempty"`
	TraceEventID string `json:"trace_event_id,omitempty"`
}

// EvaluatePreToolUse checks block rules before a tool executes.
// It records a trace event for the block check and returns whether the tool call is allowed.
func EvaluatePreToolUse(ctx context.Context, blockEngine *block.BlockRuleEngine, traceEngine *trace.TraceEngine, ptc *PreToolContext) (*PreToolResult, error) {
	// If no block engine, allow by default (fail-open for missing engine)
	if blockEngine == nil {
		return &PreToolResult{Allowed: true}, nil
	}

	input := &block.EvalInput{
		Tool:      ptc.Tool,
		Command:   ptc.Command,
		FilePath:  ptc.FilePath,
		AgentID:   ptc.AgentID,
		AgentName: ptc.AgentName,
		Depth:     ptc.Depth,
		Grants:    ptc.Grants,
	}

	evalResult := blockEngine.Evaluate(ctx, input)

	// Determine disposition for trace
	var disposition trace.BlockDisposition
	if !evalResult.Matched {
		disposition = trace.DispositionAllowed
	} else if evalResult.Overridden {
		disposition = trace.DispositionOverridden
	} else if evalResult.Action == block.ActionWarn {
		disposition = trace.DispositionWarned
	} else {
		disposition = trace.DispositionBlocked
	}

	// Record trace event
	if traceEngine != nil {
		traceEvent := &trace.TraceEvent{
			TraceID:          ptc.TraceID,
			ParentID:         ptc.ParentSpan,
			AgentID:          ptc.AgentID,
			AgentName:        ptc.AgentName,
			Capability:       ptc.Capability,
			EventType:        trace.EventBlockCheck,
			ToolName:         ptc.Tool,
			BlockRuleID:      evalResult.RuleID,
			BlockDisposition: disposition,
			SessionID:        ptc.SessionID,
			RunID:            ptc.RunID,
		}
		_ = traceEngine.Record(traceEvent)
	}

	result := &PreToolResult{
		Allowed:     !evalResult.Matched || evalResult.Overridden || evalResult.Action == block.ActionWarn,
		RuleID:      evalResult.RuleID,
		Severity:    string(evalResult.Severity),
		Overridden:  evalResult.Overridden,
		RateLimited: evalResult.RateLimited,
	}

	if evalResult.Matched && !result.Allowed {
		result.Message = evalResult.Message
	}

	return result, nil
}
