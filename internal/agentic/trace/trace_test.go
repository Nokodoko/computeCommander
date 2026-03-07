package trace

import (
	"context"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupTestDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestGenerateTraceID(t *testing.T) {
	id := GenerateTraceID()
	if len(id) == 0 {
		t.Fatal("expected non-empty trace ID")
	}
	if id[:4] != "trc-" {
		t.Fatalf("expected trc- prefix, got %q", id)
	}
	// Check uniqueness
	id2 := GenerateTraceID()
	if id == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestGenerateRunID(t *testing.T) {
	id := GenerateRunID()
	if len(id) == 0 {
		t.Fatal("expected non-empty run ID")
	}
	if id[:4] != "run-" {
		t.Fatalf("expected run- prefix, got %q", id)
	}
}

func TestHashInput(t *testing.T) {
	hash := HashInput("test input")
	if len(hash) != 64 { // SHA-256 produces 64 hex chars
		t.Fatalf("expected 64 char hash, got %d", len(hash))
	}
	// Same input should produce same hash
	hash2 := HashInput("test input")
	if hash != hash2 {
		t.Fatal("expected deterministic hash")
	}
	// Different input should produce different hash
	hash3 := HashInput("different input")
	if hash == hash3 {
		t.Fatal("expected different hash for different input")
	}
}

func TestTruncateSummary(t *testing.T) {
	short := "hello"
	if TruncateSummary(short, 500) != short {
		t.Fatal("short string should not be truncated")
	}

	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}
	result := TruncateSummary(string(long), 500)
	if len(result) != 500 {
		t.Fatalf("expected 500 chars, got %d", len(result))
	}
}

func TestTraceEngineRecordAndQuery(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	te := NewTraceEngine(database, 100, time.Second)

	event := &TraceEvent{
		TraceID:    "trace-001",
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		EventType:  EventToolCall,
		ToolName:   "Bash",
		DurationMs: 150,
		RunID:      "run-001",
	}

	if err := te.RecordSync(ctx, event); err != nil {
		t.Fatalf("record sync: %v", err)
	}

	events, err := te.Query(ctx, QueryOpts{AgentName: "builder-1", Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].AgentName != "builder-1" {
		t.Fatalf("expected agent builder-1, got %q", events[0].AgentName)
	}
	if events[0].EventType != EventToolCall {
		t.Fatalf("expected tool_call event, got %q", events[0].EventType)
	}
}

func TestTraceEngineQueryByTraceID(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	te := NewTraceEngine(database, 100, time.Second)

	traceID := "trace-unique"
	for i := 0; i < 3; i++ {
		event := &TraceEvent{
			TraceID:    traceID,
			AgentID:    "agent-001",
			AgentName:  "builder-1",
			Capability: "builder",
			EventType:  EventToolCall,
			ToolName:   "Bash",
			DurationMs: 100,
		}
		if err := te.RecordSync(ctx, event); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// Add an event with a different trace ID
	other := &TraceEvent{
		TraceID:    "trace-other",
		AgentID:    "agent-002",
		AgentName:  "scout-1",
		Capability: "scout",
		EventType:  EventAgentSpawn,
		DurationMs: 50,
	}
	if err := te.RecordSync(ctx, other); err != nil {
		t.Fatalf("record other: %v", err)
	}

	events, err := te.Query(ctx, QueryOpts{TraceID: traceID})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
}

func TestTraceEnginePrune(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	te := NewTraceEngine(database, 100, time.Second)

	// Insert an old event manually
	oldTime := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	err := database.Exec(ctx, `INSERT INTO trace_events
		(id, trace_id, span_id, agent_id, agent_name, capability, event_type, file_paths, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"trc-old", "trace-old", "span-old", "agent-old", "old-agent", "builder", "tool_call", "[]", 0, oldTime)
	if err != nil {
		t.Fatalf("insert old event: %v", err)
	}

	// Insert a recent event
	recentEvent := &TraceEvent{
		TraceID:    "trace-recent",
		AgentID:    "agent-new",
		AgentName:  "new-agent",
		Capability: "builder",
		EventType:  EventToolCall,
		DurationMs: 0,
	}
	if err := te.RecordSync(ctx, recentEvent); err != nil {
		t.Fatalf("record recent: %v", err)
	}

	// Dry run
	count, err := te.Prune(ctx, 24*time.Hour, true)
	if err != nil {
		t.Fatalf("prune dry run: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 prunable, got %d", count)
	}

	// Actual prune
	count, err = te.Prune(ctx, 24*time.Hour, false)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 pruned, got %d", count)
	}

	// Verify only recent event remains
	events, err := te.Query(ctx, QueryOpts{Limit: 100})
	if err != nil {
		t.Fatalf("query after prune: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 remaining event, got %d", len(events))
	}
}

func TestParseJSONArray(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"[]", 0},
		{"", 0},
		{"null", 0},
		{`["a","b","c"]`, 3},
		{`["single"]`, 1},
	}
	for _, tt := range tests {
		result := parseJSONArray(tt.input)
		if len(result) != tt.expected {
			t.Errorf("parseJSONArray(%q): expected %d elements, got %d", tt.input, tt.expected, len(result))
		}
	}
}

func TestToJSONArray(t *testing.T) {
	result := toJSONArray([]string{})
	if result != "[]" {
		t.Fatalf("expected [], got %q", result)
	}

	result = toJSONArray([]string{"a", "b"})
	if result != `["a","b"]` {
		t.Fatalf("expected [\"a\",\"b\"], got %q", result)
	}
}

func TestConvertPlaceholders(t *testing.T) {
	input := "SELECT * FROM t WHERE a = $1 AND b = $2"
	expected := "SELECT * FROM t WHERE a = ? AND b = ?"
	result := convertPlaceholders(input)
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
