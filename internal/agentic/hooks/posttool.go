package hooks

import (
	"time"

	"github.com/noko/computecommander/internal/agentic/trace"
)

// PostToolContext holds the context needed for post-tool-use recording.
type PostToolContext struct {
	AgentID       string
	AgentName     string
	Capability    string
	SessionID     string
	RunID         string
	TraceID       string
	ParentSpan    string
	Tool          string
	InputHash     string
	ResultCode    int
	ResultSummary string
	FilePaths     []string
	DurationMs    int
	BlueprintID   string
}

// RecordPostToolUse records a trace event after a tool completes execution.
func RecordPostToolUse(traceEngine *trace.TraceEngine, ptc *PostToolContext) error {
	if traceEngine == nil {
		return nil
	}

	resultCode := ptc.ResultCode
	event := &trace.TraceEvent{
		TraceID:           ptc.TraceID,
		ParentID:          ptc.ParentSpan,
		AgentID:           ptc.AgentID,
		AgentName:         ptc.AgentName,
		Capability:        ptc.Capability,
		EventType:         trace.EventToolCall,
		ToolName:          ptc.Tool,
		ToolInputHash:     ptc.InputHash,
		ToolResultCode:    &resultCode,
		ToolResultSummary: trace.TruncateSummary(ptc.ResultSummary, 500),
		BlueprintID:       ptc.BlueprintID,
		FilePaths:         ptc.FilePaths,
		DurationMs:        ptc.DurationMs,
		SessionID:         ptc.SessionID,
		RunID:             ptc.RunID,
		CreatedAt:         time.Now().UTC(),
	}

	return traceEngine.Record(event)
}
