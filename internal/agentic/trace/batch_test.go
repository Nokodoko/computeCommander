package trace

import (
	"context"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

func TestBatchWriterAdd(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bw := NewBatchWriter(database, 10, time.Minute)

	event := &TraceEvent{
		ID:         GenerateTraceID(),
		TraceID:    "trace-001",
		SpanID:     "span-001",
		AgentID:    "agent-001",
		AgentName:  "test-agent",
		Capability: "builder",
		EventType:  EventToolCall,
		FilePaths:  []string{},
		CreatedAt:  time.Now().UTC(),
	}

	bw.Add(event)
	if bw.Len() != 1 {
		t.Fatalf("expected 1 buffered event, got %d", bw.Len())
	}
}

func TestBatchWriterFlush(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bw := NewBatchWriter(database, 100, time.Minute)

	for i := 0; i < 5; i++ {
		bw.Add(&TraceEvent{
			ID:         GenerateTraceID(),
			TraceID:    "trace-batch",
			SpanID:     generateSpanID(),
			AgentID:    "agent-001",
			AgentName:  "test-agent",
			Capability: "builder",
			EventType:  EventToolCall,
			FilePaths:  []string{},
			CreatedAt:  time.Now().UTC(),
		})
	}

	if bw.Len() != 5 {
		t.Fatalf("expected 5 buffered events, got %d", bw.Len())
	}

	if err := bw.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if bw.Len() != 0 {
		t.Fatalf("expected 0 buffered events after flush, got %d", bw.Len())
	}

	// Verify events are in the database
	ctx := context.Background()
	rows, err := database.Query(ctx, "SELECT COUNT(*) FROM trace_events WHERE trace_id = ?", "trace-batch")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		var count int
		if err := rows.Scan(&count); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if count != 5 {
			t.Fatalf("expected 5 persisted events, got %d", count)
		}
	}
}

func TestBatchWriterAutoFlush(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Set batch size to 3 so it auto-flushes
	bw := NewBatchWriter(database, 3, time.Minute)

	for i := 0; i < 5; i++ {
		bw.Add(&TraceEvent{
			ID:         GenerateTraceID(),
			TraceID:    "trace-auto",
			SpanID:     generateSpanID(),
			AgentID:    "agent-001",
			AgentName:  "test-agent",
			Capability: "builder",
			EventType:  EventToolCall,
			FilePaths:  []string{},
			CreatedAt:  time.Now().UTC(),
		})
	}

	// The first 3 should have been auto-flushed, leaving 2
	// (or all 5 if the auto-flush completed fast enough)
	if bw.Len() > 3 {
		t.Fatalf("expected auto-flush to reduce buffer, got %d", bw.Len())
	}
}

func TestBatchWriterStop(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bw := NewBatchWriter(database, 100, time.Minute)
	bw.Start(context.Background())

	bw.Add(&TraceEvent{
		ID:         GenerateTraceID(),
		TraceID:    "trace-stop",
		SpanID:     generateSpanID(),
		AgentID:    "agent-001",
		AgentName:  "test-agent",
		Capability: "builder",
		EventType:  EventToolCall,
		FilePaths:  []string{},
		CreatedAt:  time.Now().UTC(),
	})

	if err := bw.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Should have flushed on stop
	if bw.Len() != 0 {
		t.Fatalf("expected 0 buffered events after stop, got %d", bw.Len())
	}
}

func TestBatchWriterEmptyFlush(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bw := NewBatchWriter(database, 100, time.Minute)

	// Flushing an empty buffer should be a no-op
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush empty: %v", err)
	}
}
