package holdout

import (
	"context"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupHoldoutDB(t *testing.T) db.DB {
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

func TestHoldoutEngineCreateSpec(t *testing.T) {
	database := setupHoldoutDB(t)
	ctx := context.Background()
	engine := NewHoldoutEngine(database, 0.7)

	spec := &HoldoutSpec{
		BlueprintID: "bp-001",
		Encrypted:   true,
		FilePath:    ".computecommander/holdouts/holdout-test.enc",
		TestCount:   3,
	}

	if err := engine.CreateSpec(ctx, spec); err != nil {
		t.Fatalf("create spec: %v", err)
	}

	if spec.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestHoldoutEngineGetSpec(t *testing.T) {
	database := setupHoldoutDB(t)
	ctx := context.Background()
	engine := NewHoldoutEngine(database, 0.7)

	spec := &HoldoutSpec{
		BlueprintID: "bp-001",
		Encrypted:   true,
		FilePath:    "/tmp/holdout.enc",
		TestCount:   5,
	}
	_ = engine.CreateSpec(ctx, spec)

	got, err := engine.GetSpec(ctx, spec.ID)
	if err != nil {
		t.Fatalf("get spec: %v", err)
	}
	if got.BlueprintID != "bp-001" {
		t.Fatalf("expected bp-001, got %q", got.BlueprintID)
	}
	if got.TestCount != 5 {
		t.Fatalf("expected 5 tests, got %d", got.TestCount)
	}
}

func TestHoldoutEngineGetSpecByBlueprint(t *testing.T) {
	database := setupHoldoutDB(t)
	ctx := context.Background()
	engine := NewHoldoutEngine(database, 0.7)

	for i := 0; i < 3; i++ {
		spec := &HoldoutSpec{
			BlueprintID: "bp-multi",
			Encrypted:   true,
			FilePath:    "/tmp/holdout.enc",
			TestCount:   1,
		}
		_ = engine.CreateSpec(ctx, spec)
	}

	specs, err := engine.GetSpecByBlueprint(ctx, "bp-multi")
	if err != nil {
		t.Fatalf("get by blueprint: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
}

func TestHoldoutEngineRecordResult(t *testing.T) {
	database := setupHoldoutDB(t)
	ctx := context.Background()
	engine := NewHoldoutEngine(database, 0.7)

	// Create spec first for FK
	spec := &HoldoutSpec{
		BlueprintID: "bp-001",
		Encrypted:   true,
		FilePath:    "/tmp/test.enc",
		TestCount:   2,
	}
	_ = engine.CreateSpec(ctx, spec)

	result := &HoldoutResult{
		HoldoutID:   spec.ID,
		BlueprintID: "bp-001",
		AgentID:     "agent-001",
		Score:       0.85,
		TestsPassed: 2,
		TestsTotal:  2,
		Details: []HoldoutTestResult{
			{TestName: "test1", Passed: true, Weight: 0.5},
			{TestName: "test2", Passed: true, Weight: 0.5},
		},
		VerifiedAt: time.Now().UTC(),
	}

	if err := engine.RecordResult(ctx, result); err != nil {
		t.Fatalf("record result: %v", err)
	}

	results, err := engine.GetResults(ctx, "bp-001")
	if err != nil {
		t.Fatalf("get results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 0.85 {
		t.Fatalf("expected score 0.85, got %f", results[0].Score)
	}
}

func TestComputeScore(t *testing.T) {
	tests := []HoldoutTestResult{
		{TestName: "t1", Passed: true, Weight: 0.5},
		{TestName: "t2", Passed: false, Weight: 0.3},
		{TestName: "t3", Passed: true, Weight: 0.2},
	}

	score, passed := ComputeScore(tests)
	if passed != 2 {
		t.Fatalf("expected 2 passed, got %d", passed)
	}
	expected := 0.7 // (0.5 + 0.2) / (0.5 + 0.3 + 0.2)
	if score != expected {
		t.Fatalf("expected score %f, got %f", expected, score)
	}
}

func TestComputeScoreAllPass(t *testing.T) {
	tests := []HoldoutTestResult{
		{TestName: "t1", Passed: true, Weight: 1.0},
	}
	score, passed := ComputeScore(tests)
	if score != 1.0 {
		t.Fatalf("expected 1.0, got %f", score)
	}
	if passed != 1 {
		t.Fatalf("expected 1 passed, got %d", passed)
	}
}

func TestComputeScoreEmpty(t *testing.T) {
	score, passed := ComputeScore(nil)
	if score != 0 || passed != 0 {
		t.Fatalf("expected 0/0, got %f/%d", score, passed)
	}
}

func TestDetectDrift(t *testing.T) {
	engine := NewHoldoutEngine(nil, 0.7)

	if engine.DetectDrift(0.8) {
		t.Fatal("0.8 should not be drift at threshold 0.7")
	}
	if !engine.DetectDrift(0.5) {
		t.Fatal("0.5 should be drift at threshold 0.7")
	}
	if !engine.DetectDrift(0.69) {
		t.Fatal("0.69 should be drift at threshold 0.7")
	}
}

func TestGenerateHoldoutID(t *testing.T) {
	id := GenerateHoldoutID()
	if id[:5] != "hold-" {
		t.Fatalf("expected hold- prefix, got %q", id)
	}
}

func TestGenerateResultID(t *testing.T) {
	id := GenerateResultID()
	if id[:3] != "hr-" {
		t.Fatalf("expected hr- prefix, got %q", id)
	}
}
