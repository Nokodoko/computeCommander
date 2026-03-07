package blueprint

import (
	"context"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupBPDB(t *testing.T) db.DB {
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

func TestBlueprintEngineCRUD(t *testing.T) {
	database := setupBPDB(t)
	ctx := context.Background()
	engine := NewBlueprintEngine(database)

	bp := &Blueprint{
		Name:       "Test Blueprint",
		Agent:      "unix-coder",
		Capability: "builder",
		RetryLimit: 3,
		Timeout:    "30m",
		Gates:      []string{"lint", "test"},
		DependsOn:  []string{},
		ContextGrants: []ContextGrant{
			{Action: "read", Path: "internal/*.go"},
		},
		VerifySteps: []VerifyStep{
			{Command: "go test ./...", Expect: "exit_0"},
		},
	}

	if err := engine.Create(ctx, bp); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := engine.Get(ctx, bp.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Test Blueprint" {
		t.Fatalf("expected name 'Test Blueprint', got %q", got.Name)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected pending status, got %q", got.Status)
	}
}

func TestBlueprintEngineList(t *testing.T) {
	database := setupBPDB(t)
	ctx := context.Background()
	engine := NewBlueprintEngine(database)

	for i := 0; i < 3; i++ {
		bp := &Blueprint{
			Name:       "BP " + string(rune('A'+i)),
			Agent:      "unix-coder",
			Capability: "builder",
			Gates:      []string{},
			DependsOn:  []string{},
			ContextGrants: []ContextGrant{},
			VerifySteps:   []VerifyStep{},
		}
		if err := engine.Create(ctx, bp); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	bps, err := engine.List(ctx, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bps) != 3 {
		t.Fatalf("expected 3 blueprints, got %d", len(bps))
	}
}

func TestBlueprintEngineUpdateStatus(t *testing.T) {
	database := setupBPDB(t)
	ctx := context.Background()
	engine := NewBlueprintEngine(database)

	bp := &Blueprint{
		Name:       "Status Test",
		Agent:      "unix-coder",
		Capability: "builder",
		Gates:      []string{},
		DependsOn:  []string{},
		ContextGrants: []ContextGrant{},
		VerifySteps:   []VerifyStep{},
	}
	if err := engine.Create(ctx, bp); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Valid transition: pending -> running
	if err := engine.UpdateStatus(ctx, bp.ID, StatusPending, StatusRunning, ""); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := engine.Get(ctx, bp.ID)
	if got.Status != StatusRunning {
		t.Fatalf("expected running, got %q", got.Status)
	}

	// Invalid transition: running -> pending
	if err := engine.UpdateStatus(ctx, bp.ID, StatusRunning, StatusPending, ""); err == nil {
		t.Fatal("expected error for invalid transition")
	}
}

func TestBlueprintEngineHasCycle(t *testing.T) {
	database := setupBPDB(t)
	ctx := context.Background()
	engine := NewBlueprintEngine(database)

	// Create blueprints with no cycles
	bp1 := &Blueprint{ID: "bp-001", Name: "BP1", Agent: "a", Capability: "builder", Gates: []string{}, DependsOn: []string{}, ContextGrants: []ContextGrant{}, VerifySteps: []VerifyStep{}}
	bp2 := &Blueprint{ID: "bp-002", Name: "BP2", Agent: "a", Capability: "builder", Gates: []string{}, DependsOn: []string{"bp-001"}, ContextGrants: []ContextGrant{}, VerifySteps: []VerifyStep{}}
	_ = engine.Create(ctx, bp1)
	_ = engine.Create(ctx, bp2)

	hasCycle, err := engine.HasCycle(ctx)
	if err != nil {
		t.Fatalf("has cycle: %v", err)
	}
	if hasCycle {
		t.Fatal("expected no cycle")
	}
}

func TestBlueprintEngineDelete(t *testing.T) {
	database := setupBPDB(t)
	ctx := context.Background()
	engine := NewBlueprintEngine(database)

	bp := &Blueprint{Name: "Delete Me", Agent: "a", Capability: "builder", Gates: []string{}, DependsOn: []string{}, ContextGrants: []ContextGrant{}, VerifySteps: []VerifyStep{}}
	_ = engine.Create(ctx, bp)

	if err := engine.Delete(ctx, bp.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := engine.Get(ctx, bp.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
