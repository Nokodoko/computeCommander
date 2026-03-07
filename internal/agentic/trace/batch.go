package trace

import (
	"context"
	"sync"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// BatchWriter buffers trace events in memory and flushes them to the database
// in batches to minimize SQLite lock contention.
type BatchWriter struct {
	db            db.DB
	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	buffer  []*TraceEvent
	stopCh  chan struct{}
	stopped bool
}

// NewBatchWriter creates a new BatchWriter with the given batch size and flush interval.
func NewBatchWriter(database db.DB, batchSize int, flushInterval time.Duration) *BatchWriter {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	return &BatchWriter{
		db:            database,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		buffer:        make([]*TraceEvent, 0, batchSize),
		stopCh:        make(chan struct{}),
	}
}

// Start begins the background flush loop. It periodically flushes buffered events.
func (bw *BatchWriter) Start(ctx context.Context) {
	go bw.flushLoop(ctx)
}

// Stop flushes any remaining events and stops the background loop.
func (bw *BatchWriter) Stop() error {
	bw.mu.Lock()
	if bw.stopped {
		bw.mu.Unlock()
		return nil
	}
	bw.stopped = true
	bw.mu.Unlock()

	close(bw.stopCh)
	return bw.Flush()
}

// Add appends a trace event to the buffer. If the buffer is full, it triggers
// an immediate flush.
func (bw *BatchWriter) Add(event *TraceEvent) {
	bw.mu.Lock()
	bw.buffer = append(bw.buffer, event)
	shouldFlush := len(bw.buffer) >= bw.batchSize
	bw.mu.Unlock()

	if shouldFlush {
		_ = bw.Flush()
	}
}

// Flush writes all buffered events to the database in a single transaction.
func (bw *BatchWriter) Flush() error {
	bw.mu.Lock()
	if len(bw.buffer) == 0 {
		bw.mu.Unlock()
		return nil
	}
	events := bw.buffer
	bw.buffer = make([]*TraceEvent, 0, bw.batchSize)
	bw.mu.Unlock()

	ctx := context.Background()
	tx, err := bw.db.Begin(ctx)
	if err != nil {
		// Re-add events to buffer on failure
		bw.mu.Lock()
		bw.buffer = append(events, bw.buffer...)
		bw.mu.Unlock()
		return err
	}

	for _, e := range events {
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

		if err := tx.Exec(ctx, query,
			e.ID, e.TraceID, nilIfEmpty(e.ParentID), e.SpanID,
			e.AgentID, e.AgentName, e.Capability,
			string(e.EventType), nilIfEmpty(e.ToolName), nilIfEmpty(e.ToolInputHash),
			e.ToolResultCode, nilIfEmpty(e.ToolResultSummary),
			nilIfEmpty(e.BlockRuleID), blockDisp,
			nilIfEmpty(e.BlueprintID), toJSONArray(e.FilePaths), e.DurationMs,
			e.CreatedAt.Format(time.RFC3339), nilIfEmpty(e.SessionID), nilIfEmpty(e.RunID),
		); err != nil {
			_ = tx.Rollback()
			// Re-add events to buffer on failure
			bw.mu.Lock()
			bw.buffer = append(events, bw.buffer...)
			bw.mu.Unlock()
			return err
		}
	}

	return tx.Commit()
}

// Len returns the number of events currently buffered.
func (bw *BatchWriter) Len() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return len(bw.buffer)
}

func (bw *BatchWriter) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(bw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = bw.Flush()
		case <-bw.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}
