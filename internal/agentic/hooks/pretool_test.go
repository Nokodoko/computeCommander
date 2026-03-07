package hooks

import (
	"context"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/agentic/block"
	"github.com/noko/computecommander/internal/agentic/trace"
	"github.com/noko/computecommander/internal/platform/db"
)

func setupHookDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestEvaluatePreToolUseAllowed(t *testing.T) {
	database := setupHookDB(t)
	ctx := context.Background()

	blockEngine := block.NewBlockRuleEngine(database)
	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	ptc := &PreToolContext{
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		Tool:       "Bash",
		Command:    "go test ./...",
		TraceID:    "trace-001",
	}

	result, err := EvaluatePreToolUse(ctx, blockEngine, traceEngine, ptc)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed for non-matching rule")
	}
}

func TestEvaluatePreToolUseBlocked(t *testing.T) {
	database := setupHookDB(t)
	ctx := context.Background()

	blockEngine := block.NewBlockRuleEngine(database)
	rules, _ := block.ParseRules([]byte(`
version: 1
rules:
  - id: no-force-push
    description: "Block force push"
    tool: Bash
    match:
      command: "git push.*--force"
    action: block
    message: "Force push prohibited"
    severity: critical
`))
	blockEngine.LoadRules(rules)

	traceEngine := trace.NewTraceEngine(database, 100, time.Minute)

	ptc := &PreToolContext{
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		Tool:       "Bash",
		Command:    "git push --force origin main",
		TraceID:    "trace-001",
	}

	result, err := EvaluatePreToolUse(ctx, blockEngine, traceEngine, ptc)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected blocked")
	}
	if result.RuleID != "no-force-push" {
		t.Fatalf("expected rule no-force-push, got %q", result.RuleID)
	}
}

func TestEvaluatePreToolUseNilEngine(t *testing.T) {
	ctx := context.Background()

	result, err := EvaluatePreToolUse(ctx, nil, nil, &PreToolContext{Tool: "Bash"})
	if err != nil {
		t.Fatalf("evaluate nil engine: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed with nil engine")
	}
}

func TestEvaluatePreToolUseOverridden(t *testing.T) {
	database := setupHookDB(t)
	ctx := context.Background()

	blockEngine := block.NewBlockRuleEngine(database)
	rules, _ := block.ParseRules([]byte(`
version: 1
rules:
  - id: no-secret-read
    description: "Block .env"
    tool: Read
    match:
      file_path: ".*\\.env$"
    action: block
    message: "Reading secrets prohibited"
    severity: critical
    override: grant
`))
	blockEngine.LoadRules(rules)

	ptc := &PreToolContext{
		AgentID:    "agent-001",
		AgentName:  "builder-1",
		Capability: "builder",
		Tool:       "Read",
		FilePath:   "config/.env",
		TraceID:    "trace-001",
		Grants:     []string{"no-secret-read"},
	}

	result, err := EvaluatePreToolUse(ctx, blockEngine, nil, ptc)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected allowed with override grant")
	}
	if !result.Overridden {
		t.Fatal("expected overridden flag")
	}
}
