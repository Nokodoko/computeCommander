package gate

import (
	"context"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupGateDB(t *testing.T) db.DB {
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

func TestGatePipelineAllPass(t *testing.T) {
	database := setupGateDB(t)
	runner := NewMockRunner(map[string]MockResult{
		"gofmt -l .":       {Stdout: "", ExitCode: 0},
		"go vet ./...":     {Stdout: "", ExitCode: 0},
		"go test ./...":    {Stdout: "ok", ExitCode: 0},
	})

	pipeline := NewGatePipeline(database, []GateConfig{
		{Name: GateFormat, Command: "gofmt -l .", Enabled: true},
		{Name: GateTypecheck, Command: "go vet ./...", Enabled: true},
		{Name: GateTest, Command: "go test ./...", Enabled: true},
	}, runner)

	ctx := context.Background()
	result, err := pipeline.Run(ctx, "bp-001", "agent-001", 1)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected all gates to pass")
	}
	if len(result.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result.Results))
	}
}

func TestGatePipelineFirstFails(t *testing.T) {
	database := setupGateDB(t)
	runner := NewMockRunner(map[string]MockResult{
		"gofmt -l .":    {Stdout: "bad.go", ExitCode: 1},
		"go vet ./...":  {Stdout: "", ExitCode: 0},
		"go test ./...": {Stdout: "ok", ExitCode: 0},
	})

	pipeline := NewGatePipeline(database, []GateConfig{
		{Name: GateFormat, Command: "gofmt -l .", Enabled: true},
		{Name: GateTypecheck, Command: "go vet ./...", Enabled: true},
		{Name: GateTest, Command: "go test ./...", Enabled: true},
	}, runner)

	ctx := context.Background()
	result, err := pipeline.Run(ctx, "bp-001", "agent-001", 1)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Passed {
		t.Fatal("expected pipeline to fail")
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result (stopped at first failure), got %d", len(result.Results))
	}
	if len(result.Failed) != 1 || result.Failed[0] != "format" {
		t.Fatalf("expected format in failed list, got %v", result.Failed)
	}
}

func TestGatePipelineSkipsDisabled(t *testing.T) {
	database := setupGateDB(t)
	runner := NewMockRunner(map[string]MockResult{
		"go test ./...": {Stdout: "ok", ExitCode: 0},
	})

	pipeline := NewGatePipeline(database, []GateConfig{
		{Name: GateFormat, Command: "gofmt -l .", Enabled: false},
		{Name: GateTest, Command: "go test ./...", Enabled: true},
	}, runner)

	ctx := context.Background()
	result, err := pipeline.Run(ctx, "bp-001", "agent-001", 1)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected pass")
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result (disabled gate skipped), got %d", len(result.Results))
	}
}

func TestGatePipelineHistory(t *testing.T) {
	database := setupGateDB(t)
	runner := NewMockRunner(map[string]MockResult{
		"go test ./...": {Stdout: "ok", ExitCode: 0},
	})

	pipeline := NewGatePipeline(database, []GateConfig{
		{Name: GateTest, Command: "go test ./...", Enabled: true},
	}, runner)

	ctx := context.Background()
	_, _ = pipeline.Run(ctx, "bp-001", "agent-001", 1)
	_, _ = pipeline.Run(ctx, "bp-001", "agent-001", 2)

	history, err := pipeline.History(ctx, "bp-001", 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
}
