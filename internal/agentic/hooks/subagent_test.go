package hooks

import (
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agentic/trace"
	"github.com/noko/computecommander/internal/platform/db"
)

func TestRecordSubagentStart(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	ctx := &SubagentStartContext{
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		TraceID:    "trace-001",
		RunID:      "run-001",
	}

	if err := RecordSubagentStart(traceEngine, ctx); err != nil {
		t.Fatalf("record start: %v", err)
	}
}

func TestRecordSubagentStop(t *testing.T) {
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	ctx := &SubagentStopContext{
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		TraceID:    "trace-001",
		RunID:      "run-001",
		DurationMs: 5000,
		ExitCode:   0,
	}

	if err := RecordSubagentStop(traceEngine, ctx); err != nil {
		t.Fatalf("record stop: %v", err)
	}
}

func TestRecordSubagentStartNilEngine(t *testing.T) {
	if err := RecordSubagentStart(nil, &SubagentStartContext{}); err != nil {
		t.Fatalf("expected no error with nil engine: %v", err)
	}
}

func TestRecordSubagentStopNilEngine(t *testing.T) {
	if err := RecordSubagentStop(nil, &SubagentStopContext{}); err != nil {
		t.Fatalf("expected no error with nil engine: %v", err)
	}
}
