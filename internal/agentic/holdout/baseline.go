package holdout

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// BehavioralBaseline holds reference metrics for detecting drift.
type BehavioralBaseline struct {
	ID             string          `json:"id"`
	BlueprintID    string          `json:"blueprint_id"`
	Agent          string          `json:"agent"`
	Capability     string          `json:"capability"`
	Metrics        BaselineMetrics `json:"metrics"`
	Tolerance      float64         `json:"tolerance"`
	DriftThreshold float64         `json:"drift_threshold"`
	SampleCount    int             `json:"sample_count"`
	LastUpdated    time.Time       `json:"last_updated"`
}

// BaselineMetrics holds the reference execution metrics.
type BaselineMetrics struct {
	ToolCallCount  int     `json:"tool_call_count"`
	FileTouchCount int     `json:"file_touch_count"`
	DurationMs     int     `json:"duration_ms"`
	RetryCount     int     `json:"retry_count"`
	TestCoverage   float64 `json:"test_coverage"`
}

// BaselineStore manages behavioral baseline CRUD.
type BaselineStore struct {
	db db.DB
}

// NewBaselineStore creates a new BaselineStore.
func NewBaselineStore(database db.DB) *BaselineStore {
	return &BaselineStore{db: database}
}

// Create persists a new behavioral baseline.
func (s *BaselineStore) Create(ctx context.Context, b *BehavioralBaseline) error {
	if b.ID == "" {
		b.ID = GenerateBaselineID()
	}
	if b.LastUpdated.IsZero() {
		b.LastUpdated = time.Now().UTC()
	}
	if b.Tolerance == 0 {
		b.Tolerance = 0.3
	}
	if b.DriftThreshold == 0 {
		b.DriftThreshold = 0.7
	}

	metricsJSON, _ := json.Marshal(b.Metrics)

	query := `INSERT INTO behavioral_baselines (
		id, blueprint_id, agent, capability, metrics,
		tolerance, drift_threshold, sample_count, last_updated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	return s.db.Exec(ctx, query,
		b.ID, b.BlueprintID, b.Agent, b.Capability, string(metricsJSON),
		b.Tolerance, b.DriftThreshold, b.SampleCount,
		b.LastUpdated.Format(time.RFC3339),
	)
}

// Get retrieves a baseline by blueprint ID.
func (s *BaselineStore) Get(ctx context.Context, blueprintID string) (*BehavioralBaseline, error) {
	query := `SELECT id, blueprint_id, agent, capability, metrics,
		tolerance, drift_threshold, sample_count, last_updated
		FROM behavioral_baselines WHERE blueprint_id = ?`

	row := s.db.QueryRow(ctx, query, blueprintID)

	var b BehavioralBaseline
	var metricsJSON, lastUpdated string
	if err := row.Scan(
		&b.ID, &b.BlueprintID, &b.Agent, &b.Capability, &metricsJSON,
		&b.Tolerance, &b.DriftThreshold, &b.SampleCount, &lastUpdated,
	); err != nil {
		return nil, fmt.Errorf("get baseline for %s: %w", blueprintID, err)
	}

	_ = json.Unmarshal([]byte(metricsJSON), &b.Metrics)
	b.LastUpdated, _ = time.Parse(time.RFC3339, lastUpdated)

	return &b, nil
}

// Update modifies an existing baseline.
func (s *BaselineStore) Update(ctx context.Context, b *BehavioralBaseline) error {
	b.LastUpdated = time.Now().UTC()
	metricsJSON, _ := json.Marshal(b.Metrics)

	query := `UPDATE behavioral_baselines
		SET metrics = ?, tolerance = ?, drift_threshold = ?,
		    sample_count = ?, last_updated = ?
		WHERE id = ?`

	return s.db.Exec(ctx, query,
		string(metricsJSON), b.Tolerance, b.DriftThreshold,
		b.SampleCount, b.LastUpdated.Format(time.RFC3339), b.ID,
	)
}

// Delete removes a baseline.
func (s *BaselineStore) Delete(ctx context.Context, id string) error {
	return s.db.Exec(ctx, "DELETE FROM behavioral_baselines WHERE id = ?", id)
}

// ComputeDrift compares actual metrics against the baseline and returns a drift score.
// Returns a score between 0.0 (total drift) and 1.0 (perfect match).
func ComputeDrift(baseline BaselineMetrics, actual BaselineMetrics, tolerance float64) float64 {
	if tolerance <= 0 {
		tolerance = 0.3
	}

	scores := []float64{
		metricScore(float64(baseline.ToolCallCount), float64(actual.ToolCallCount), tolerance),
		metricScore(float64(baseline.FileTouchCount), float64(actual.FileTouchCount), tolerance),
		metricScore(float64(baseline.DurationMs), float64(actual.DurationMs), tolerance),
		metricScore(float64(baseline.RetryCount), float64(actual.RetryCount), tolerance),
		metricScore(baseline.TestCoverage, actual.TestCoverage, tolerance),
	}

	var total float64
	for _, s := range scores {
		total += s
	}
	return total / float64(len(scores))
}

// metricScore computes how close actual is to expected, within tolerance.
// Returns 1.0 if within tolerance, scaling down to 0.0 as deviation increases.
func metricScore(expected, actual, tolerance float64) float64 {
	if expected == 0 {
		if actual == 0 {
			return 1.0
		}
		return 0.5 // Can't compute ratio, give half credit
	}

	ratio := actual / expected
	deviation := math.Abs(1.0 - ratio)

	if deviation <= tolerance {
		return 1.0
	}

	// Linear decay beyond tolerance
	excess := deviation - tolerance
	score := 1.0 - (excess / tolerance)
	if score < 0 {
		return 0.0
	}
	return score
}

// GenerateBaselineID creates a baseline ID.
func GenerateBaselineID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "base-" + hex.EncodeToString(b)
}
