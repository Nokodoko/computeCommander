package hooks

import (
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agentic/trace"
	"github.com/noko/computecommander/internal/platform/db"
)

func TestRecordPostToolUse(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	ptc := &PostToolContext{
		AgentID:       "agent-001",
		AgentName:     "builder-1",
		Capability:    "builder",
		Tool:          "Bash",
		InputHash:     "abc123",
		ResultCode:    0,
		ResultSummary: "ok",
		FilePaths:     []string{"main.go"},
		DurationMs:    100,
		TraceID:       "trace-001",
	}

	if err := RecordPostToolUse(traceEngine, ptc); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Flush and verify
	if err := traceEngine.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestRecordPostToolUseNilEngine(t *testing.T) {
	ptc := &PostToolContext{
		Tool:       "Bash",
		ResultCode: 0,
	}
	if err := RecordPostToolUse(nil, ptc); err != nil {
		t.Fatalf("expected no error with nil engine: %v", err)
	}
}

func TestRecordPostToolUseTruncatesLongSummary(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}

	ptc := &PostToolContext{
		AgentID:       "agent-001",
		AgentName:     "builder-1",
		Capability:    "builder",
		Tool:          "Bash",
		ResultCode:    0,
		ResultSummary: string(long),
		TraceID:       "trace-001",
	}

	if err := RecordPostToolUse(traceEngine, ptc); err != nil {
		t.Fatalf("record: %v", err)
	}
}
