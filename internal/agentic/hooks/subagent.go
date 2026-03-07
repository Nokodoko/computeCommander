package hooks

import (
	"time"

	"github.com/noko/computecommander/internal/agentic/trace"
)

// SubagentStartContext holds context for recording agent spawn events.
type SubagentStartContext struct {
	AgentID    string
	AgentName  string
	Capability string
	SessionID  string
	RunID      string
	TraceID    string
	ParentSpan string
	ParentID   string // Parent agent's session ID
	Depth      int
	BlueprintID string
}

// RecordSubagentStart records a trace event when a subagent is spawned.
func RecordSubagentStart(traceEngine *trace.TraceEngine, ctx *SubagentStartContext) error {
	if traceEngine == nil {
		return nil
	}

	event := &trace.TraceEvent{
		TraceID:     ctx.TraceID,
		ParentID:    ctx.ParentSpan,
		AgentID:     ctx.AgentID,
		AgentName:   ctx.AgentName,
		Capability:  ctx.Capability,
		EventType:   trace.EventAgentSpawn,
		BlueprintID: ctx.BlueprintID,
		SessionID:   ctx.SessionID,
		RunID:       ctx.RunID,
		CreatedAt:   time.Now().UTC(),
	}

	return traceEngine.Record(event)
}

// SubagentStopContext holds context for recording agent stop events.
type SubagentStopContext struct {
	AgentID     string
	AgentName   string
	Capability  string
	SessionID   string
	RunID       string
	TraceID     string
	ParentSpan  string
	DurationMs  int
	ExitCode    int
	BlueprintID string
}

// RecordSubagentStop records a trace event when a subagent stops.
func RecordSubagentStop(traceEngine *trace.TraceEngine, ctx *SubagentStopContext) error {
	if traceEngine == nil {
		return nil
	}

	exitCode := ctx.ExitCode
	event := &trace.TraceEvent{
		TraceID:        ctx.TraceID,
		ParentID:       ctx.ParentSpan,
		AgentID:        ctx.AgentID,
		AgentName:      ctx.AgentName,
		Capability:     ctx.Capability,
		EventType:      trace.EventAgentStop,
		ToolResultCode: &exitCode,
		DurationMs:     ctx.DurationMs,
		BlueprintID:    ctx.BlueprintID,
		SessionID:      ctx.SessionID,
		RunID:          ctx.RunID,
		CreatedAt:      time.Now().UTC(),
	}

	return traceEngine.Record(event)
}
