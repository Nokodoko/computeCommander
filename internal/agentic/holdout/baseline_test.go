package holdout

import (
	"context"
	"math"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupBaselineDB(t *testing.T) db.DB {
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

func TestBaselineStoreCRUD(t *testing.T) {
	database := setupBaselineDB(t)
	ctx := context.Background()
	store := NewBaselineStore(database)

	b := &BehavioralBaseline{
		BlueprintID: "bp-001",
		Agent:       "unix-coder",
		Capability:  "builder",
		Metrics: BaselineMetrics{
			ToolCallCount:  50,
			FileTouchCount: 10,
			DurationMs:     30000,
			RetryCount:     1,
			TestCoverage:   85.5,
		},
		SampleCount: 5,
	}

	if err := store.Create(ctx, b); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(ctx, "bp-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Metrics.ToolCallCount != 50 {
		t.Fatalf("expected 50 tool calls, got %d", got.Metrics.ToolCallCount)
	}
	if got.Tolerance != 0.3 {
		t.Fatalf("expected default tolerance 0.3, got %f", got.Tolerance)
	}
}

func TestBaselineStoreUpdate(t *testing.T) {
	database := setupBaselineDB(t)
	ctx := context.Background()
	store := NewBaselineStore(database)

	b := &BehavioralBaseline{
		BlueprintID: "bp-update",
		Agent:       "unix-coder",
		Capability:  "builder",
		Metrics:     BaselineMetrics{ToolCallCount: 10},
		SampleCount: 1,
	}
	_ = store.Create(ctx, b)

	b.Metrics.ToolCallCount = 20
	b.SampleCount = 2
	if err := store.Update(ctx, b); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := store.Get(ctx, "bp-update")
	if got.Metrics.ToolCallCount != 20 {
		t.Fatalf("expected 20, got %d", got.Metrics.ToolCallCount)
	}
	if got.SampleCount != 2 {
		t.Fatalf("expected sample count 2, got %d", got.SampleCount)
	}
}

func TestComputeDriftPerfectMatch(t *testing.T) {
	baseline := BaselineMetrics{
		ToolCallCount:  50,
		FileTouchCount: 10,
		DurationMs:     30000,
		RetryCount:     1,
		TestCoverage:   85.0,
	}
	actual := baseline // Exact same

	score := ComputeDrift(baseline, actual, 0.3)
	if score != 1.0 {
		t.Fatalf("expected 1.0 for perfect match, got %f", score)
	}
}

func TestComputeDriftWithinTolerance(t *testing.T) {
	baseline := BaselineMetrics{
		ToolCallCount:  100,
		FileTouchCount: 10,
		DurationMs:     30000,
		RetryCount:     2,
		TestCoverage:   85.0,
	}
	actual := BaselineMetrics{
		ToolCallCount:  110, // 10% deviation
		FileTouchCount: 12,  // 20% deviation
		DurationMs:     33000,
		RetryCount:     2,
		TestCoverage:   80.0,
	}

	score := ComputeDrift(baseline, actual, 0.3)
	if score < 0.9 {
		t.Fatalf("expected score >= 0.9 within tolerance, got %f", score)
	}
}

func TestComputeDriftBeyondTolerance(t *testing.T) {
	baseline := BaselineMetrics{
		ToolCallCount:  100,
		FileTouchCount: 10,
		DurationMs:     30000,
		RetryCount:     1,
		TestCoverage:   85.0,
	}
	actual := BaselineMetrics{
		ToolCallCount:  200, // 100% deviation
		FileTouchCount: 50,  // 400% deviation
		DurationMs:     90000,
		RetryCount:     10,
		TestCoverage:   20.0,
	}

	score := ComputeDrift(baseline, actual, 0.3)
	if score > 0.5 {
		t.Fatalf("expected score < 0.5 for large deviation, got %f", score)
	}
}

func TestMetricScore(t *testing.T) {
	// Perfect match
	if metricScore(100, 100, 0.3) != 1.0 {
		t.Fatal("expected 1.0 for perfect match")
	}

	// Within tolerance
	if metricScore(100, 120, 0.3) != 1.0 {
		t.Fatal("expected 1.0 within tolerance")
	}

	// Both zero
	if metricScore(0, 0, 0.3) != 1.0 {
		t.Fatal("expected 1.0 for both zero")
	}

	// Expected zero, actual non-zero
	score := metricScore(0, 50, 0.3)
	if score != 0.5 {
		t.Fatalf("expected 0.5, got %f", score)
	}

	// Far beyond tolerance
	score = metricScore(100, 300, 0.3)
	if score > 0.5 {
		t.Fatalf("expected < 0.5 for 200%% deviation, got %f", score)
	}

	// Check score is never negative
	score = metricScore(100, 1000, 0.3)
	if score < 0 || math.IsNaN(score) {
		t.Fatalf("score should be >= 0, got %f", score)
	}
}
