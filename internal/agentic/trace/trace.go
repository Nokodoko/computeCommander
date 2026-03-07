// Package trace provides causal traceability for agent actions.
// Every tool invocation, agent spawn, mail send, and quality gate check
// is recorded with a trace ID linking to the parent action, enabling
// full causal chain reconstruction.
package trace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// TraceEventType enumerates the types of traceable events.
type TraceEventType string

const (
	EventToolCall          TraceEventType = "tool_call"
	EventAgentSpawn        TraceEventType = "agent_spawn"
	EventAgentStop         TraceEventType = "agent_stop"
	EventMailSend          TraceEventType = "mail_send"
	EventMailReceive       TraceEventType = "mail_receive"
	EventMergeAttempt      TraceEventType = "merge_attempt"
	EventMergeComplete     TraceEventType = "merge_complete"
	EventGateCheck         TraceEventType = "gate_check"
	EventBlockCheck        TraceEventType = "block_check"
	EventBlueprintStart    TraceEventType = "blueprint_start"
	EventBlueprintComplete TraceEventType = "blueprint_complete"
	EventHoldoutVerify     TraceEventType = "holdout_verify"
	EventContextInject     TraceEventType = "context_inject"
	EventError             TraceEventType = "error"
)

// BlockDisposition describes the outcome of a block rule check.
type BlockDisposition string

const (
	DispositionAllowed    BlockDisposition = "allowed"
	DispositionBlocked   BlockDisposition = "blocked"
	DispositionOverridden BlockDisposition = "overridden"
	DispositionWarned    BlockDisposition = "warned"
)

// TraceEvent represents a single traceable action in the causal chain.
type TraceEvent struct {
	ID                string           `json:"id"`
	TraceID           string           `json:"trace_id"`
	ParentID          string           `json:"parent_id,omitempty"`
	SpanID            string           `json:"span_id"`
	AgentID           string           `json:"agent_id"`
	AgentName         string           `json:"agent_name"`
	Capability        string           `json:"capability"`
	EventType         TraceEventType   `json:"event_type"`
	ToolName          string           `json:"tool_name,omitempty"`
	ToolInputHash     string           `json:"tool_input_hash,omitempty"`
	ToolResultCode    *int             `json:"tool_result_code,omitempty"`
	ToolResultSummary string           `json:"tool_result_summary,omitempty"`
	BlockRuleID       string           `json:"block_rule_id,omitempty"`
	BlockDisposition  BlockDisposition `json:"block_disposition,omitempty"`
	BlueprintID       string           `json:"blueprint_id,omitempty"`
	FilePaths         []string         `json:"file_paths"`
	DurationMs        int              `json:"duration_ms"`
	CreatedAt         time.Time        `json:"created_at"`
	SessionID         string           `json:"session_id,omitempty"`
	RunID             string           `json:"run_id,omitempty"`
}

// TraceEngine manages trace event recording and querying.
type TraceEngine struct {
	db    db.DB
	batch *BatchWriter
}

// NewTraceEngine creates a new TraceEngine with the given database and batch configuration.
func NewTraceEngine(database db.DB, batchSize int, flushInterval time.Duration) *TraceEngine {
	te := &TraceEngine{db: database}
	te.batch = NewBatchWriter(database, batchSize, flushInterval)
	return te
}

// Start begins the batch writer's background flush loop.
func (te *TraceEngine) Start(ctx context.Context) {
	te.batch.Start(ctx)
}

// Stop flushes remaining events and stops the batch writer.
func (te *TraceEngine) Stop() error {
	return te.batch.Stop()
}

// Record adds a trace event to the batch for eventual persistence.
func (te *TraceEngine) Record(event *TraceEvent) error {
	if event.ID == "" {
		event.ID = GenerateTraceID()
	}
	if event.SpanID == "" {
		event.SpanID = generateSpanID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.FilePaths == nil {
		event.FilePaths = []string{}
	}
	te.batch.Add(event)
	return nil
}

// RecordSync immediately persists a trace event without batching.
func (te *TraceEngine) RecordSync(ctx context.Context, event *TraceEvent) error {
	if event.ID == "" {
		event.ID = GenerateTraceID()
	}
	if event.SpanID == "" {
		event.SpanID = generateSpanID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.FilePaths == nil {
		event.FilePaths = []string{}
	}
	return insertTraceEvent(ctx, te.db, event)
}

// QueryOpts configures trace event queries.
type QueryOpts struct {
	AgentName string
	TraceID   string
	EventType TraceEventType
	Since     time.Duration
	Limit     int
	RunID     string
}

// Query retrieves trace events matching the given options.
func (te *TraceEngine) Query(ctx context.Context, opts QueryOpts) ([]*TraceEvent, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if opts.AgentName != "" {
		conditions = append(conditions, fmt.Sprintf("agent_name = $%d", argIdx))
		args = append(args, opts.AgentName)
		argIdx++
	}
	if opts.TraceID != "" {
		conditions = append(conditions, fmt.Sprintf("trace_id = $%d", argIdx))
		args = append(args, opts.TraceID)
		argIdx++
	}
	if opts.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, string(opts.EventType))
		argIdx++
	}
	if opts.RunID != "" {
		conditions = append(conditions, fmt.Sprintf("run_id = $%d", argIdx))
		args = append(args, opts.RunID)
		argIdx++
	}
	if opts.Since > 0 {
		since := time.Now().UTC().Add(-opts.Since)
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, since.Format(time.RFC3339))
		argIdx++
	}

	query := "SELECT id, trace_id, parent_id, span_id, agent_id, agent_name, capability, " +
		"event_type, tool_name, tool_input_hash, tool_result_code, tool_result_summary, " +
		"block_rule_id, block_disposition, blueprint_id, file_paths, duration_ms, " +
		"created_at, session_id, run_id FROM trace_events"

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	// Convert $N placeholders to ? for SQLite compatibility
	query = convertPlaceholders(query)

	rows, err := te.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query trace events: %w", err)
	}
	defer rows.Close()

	var events []*TraceEvent
	for rows.Next() {
		e := &TraceEvent{}
		var parentID, toolName, toolInputHash, toolResultSummary *string
		var toolResultCode *int
		var blockRuleID, blockDisp, blueprintID, sessionID, runID *string
		var filePaths, createdAt string

		if err := rows.Scan(
			&e.ID, &e.TraceID, &parentID, &e.SpanID,
			&e.AgentID, &e.AgentName, &e.Capability,
			&e.EventType, &toolName, &toolInputHash, &toolResultCode, &toolResultSummary,
			&blockRuleID, &blockDisp, &blueprintID, &filePaths, &e.DurationMs,
			&createdAt, &sessionID, &runID,
		); err != nil {
			return nil, fmt.Errorf("scan trace event: %w", err)
		}

		if parentID != nil {
			e.ParentID = *parentID
		}
		if toolName != nil {
			e.ToolName = *toolName
		}
		if toolInputHash != nil {
			e.ToolInputHash = *toolInputHash
		}
		if toolResultCode != nil {
			e.ToolResultCode = toolResultCode
		}
		if toolResultSummary != nil {
			e.ToolResultSummary = *toolResultSummary
		}
		if blockRuleID != nil {
			e.BlockRuleID = *blockRuleID
		}
		if blockDisp != nil {
			e.BlockDisposition = BlockDisposition(*blockDisp)
		}
		if blueprintID != nil {
			e.BlueprintID = *blueprintID
		}
		if sessionID != nil {
			e.SessionID = *sessionID
		}
		if runID != nil {
			e.RunID = *runID
		}

		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.FilePaths = parseJSONArray(filePaths)

		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace events: %w", err)
	}

	return events, nil
}

// GetTraceChain returns the full causal chain for a trace ID, ordered by creation time.
func (te *TraceEngine) GetTraceChain(ctx context.Context, traceID string, maxDepth int) ([]*TraceEvent, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}
	return te.Query(ctx, QueryOpts{
		TraceID: traceID,
		Limit:   maxDepth * 10, // allow room for broad trees
	})
}

// Export writes all events for a trace to NDJSON format.
func (te *TraceEngine) Export(ctx context.Context, traceID string) ([]*TraceEvent, error) {
	return te.Query(ctx, QueryOpts{
		TraceID: traceID,
		Limit:   10000,
	})
}

// Prune deletes trace events older than the given duration.
func (te *TraceEngine) Prune(ctx context.Context, olderThan time.Duration, dryRun bool) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)

	if dryRun {
		query := "SELECT COUNT(*) FROM trace_events WHERE created_at < ?"
		row := te.db.QueryRow(ctx, query, cutoff)
		var count int
		if err := row.Scan(&count); err != nil {
			return 0, fmt.Errorf("count prunable events: %w", err)
		}
		return count, nil
	}

	// Count first, then delete
	countQuery := "SELECT COUNT(*) FROM trace_events WHERE created_at < ?"
	row := te.db.QueryRow(ctx, countQuery, cutoff)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count prunable events: %w", err)
	}

	if count > 0 {
		deleteQuery := "DELETE FROM trace_events WHERE created_at < ?"
		if err := te.db.Exec(ctx, deleteQuery, cutoff); err != nil {
			return 0, fmt.Errorf("prune trace events: %w", err)
		}
	}

	return count, nil
}

// GenerateTraceID creates a new trace event ID with the "trc-" prefix.
func GenerateTraceID() string {
	return "trc-" + randomHex(16)
}

// GenerateRunID creates a new run ID with the "run-" prefix.
func GenerateRunID() string {
	return "run-" + randomHex(16)
}

func generateSpanID() string {
	return randomHex(16)
}

func randomHex(n int) string {
	b := make([]byte, n/2+1)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// HashInput creates a SHA-256 hash of tool input for deduplication.
func HashInput(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// TruncateSummary truncates a result string to maxLen characters.
func TruncateSummary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// insertTraceEvent persists a single trace event to the database.
func insertTraceEvent(ctx context.Context, database db.DB, e *TraceEvent) error {
	query := `INSERT INTO trace_events (
		id, trace_id, parent_id, span_id, agent_id, agent_name, capability,
		event_type, tool_name, tool_input_hash, tool_result_code, tool_result_summary,
		block_rule_id, block_disposition, blueprint_id, file_paths, duration_ms,
		created_at, session_id, run_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var blockDisp *string
	if e.BlockDisposition != "" {
		s := string(e.BlockDisposition)
		blockDisp = &s
	}

	return database.Exec(ctx, query,
		e.ID, e.TraceID, nilIfEmpty(e.ParentID), e.SpanID,
		e.AgentID, e.AgentName, e.Capability,
		string(e.EventType), nilIfEmpty(e.ToolName), nilIfEmpty(e.ToolInputHash),
		e.ToolResultCode, nilIfEmpty(e.ToolResultSummary),
		nilIfEmpty(e.BlockRuleID), blockDisp,
		nilIfEmpty(e.BlueprintID), toJSONArray(e.FilePaths), e.DurationMs,
		e.CreatedAt.Format(time.RFC3339), nilIfEmpty(e.SessionID), nilIfEmpty(e.RunID),
	)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toJSONArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	parts := make([]string, len(arr))
	for i, s := range arr {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func parseJSONArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" || s == "null" {
		return []string{}
	}
	// Simple JSON array parser for string arrays
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return []string{}
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// convertPlaceholders converts $N style placeholders to ? for SQLite compatibility.
func convertPlaceholders(query string) string {
	result := query
	for i := 20; i >= 1; i-- {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i), "?")
	}
	return result
}
