// Package holdout provides the anti-gaming holdout verification system.
// Holdout tests are encrypted and invisible to agents, run post-execution
// to detect pattern-matching vs genuine understanding.
package holdout

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// HoldoutSpec defines encrypted holdout tests for a blueprint.
type HoldoutSpec struct {
	ID          string `json:"id"`
	BlueprintID string `json:"blueprint_id"`
	Encrypted   bool   `json:"encrypted"`
	FilePath    string `json:"file_path"`
	TestCount   int    `json:"test_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// HoldoutTest defines a single holdout test (decrypted content).
type HoldoutTest struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"` // unit, integration, behavioral
	Command string  `json:"command"`
	Expect  string  `json:"expect"` // exit_0, contains, not_contains, regex
	Value   string  `json:"value,omitempty"`
	Weight  float64 `json:"weight"`
}

// HoldoutResult records the outcome of holdout verification.
type HoldoutResult struct {
	ID              string              `json:"id"`
	HoldoutID       string              `json:"holdout_id"`
	BlueprintID     string              `json:"blueprint_id"`
	AgentID         string              `json:"agent_id"`
	Score           float64             `json:"score"`
	TestsPassed     int                 `json:"tests_passed"`
	TestsTotal      int                 `json:"tests_total"`
	BehavioralDrift bool                `json:"behavioral_drift"`
	Details         []HoldoutTestResult `json:"details"`
	VerifiedAt      time.Time           `json:"verified_at"`
	TraceID         string              `json:"trace_id,omitempty"`
}

// HoldoutTestResult records an individual holdout test outcome.
type HoldoutTestResult struct {
	TestName string  `json:"test_name"`
	Passed   bool    `json:"passed"`
	Actual   string  `json:"actual"`
	Expected string  `json:"expected"`
	Weight   float64 `json:"weight"`
}

// HoldoutEngine manages holdout creation, verification, and result storage.
type HoldoutEngine struct {
	db             db.DB
	driftThreshold float64
}

// NewHoldoutEngine creates a new HoldoutEngine.
func NewHoldoutEngine(database db.DB, driftThreshold float64) *HoldoutEngine {
	if driftThreshold <= 0 {
		driftThreshold = 0.7
	}
	return &HoldoutEngine{
		db:             database,
		driftThreshold: driftThreshold,
	}
}

// CreateSpec persists a holdout spec metadata record.
func (e *HoldoutEngine) CreateSpec(ctx context.Context, spec *HoldoutSpec) error {
	if spec.ID == "" {
		spec.ID = GenerateHoldoutID()
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	}

	query := `INSERT INTO holdout_specs (id, blueprint_id, encrypted, file_path, test_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	encrypted := 0
	if spec.Encrypted {
		encrypted = 1
	}

	return e.db.Exec(ctx, query,
		spec.ID, spec.BlueprintID, encrypted, spec.FilePath, spec.TestCount,
		spec.CreatedAt.Format(time.RFC3339),
	)
}

// GetSpec retrieves a holdout spec by ID.
func (e *HoldoutEngine) GetSpec(ctx context.Context, id string) (*HoldoutSpec, error) {
	query := `SELECT id, blueprint_id, encrypted, file_path, test_count, created_at
		FROM holdout_specs WHERE id = ?`

	row := e.db.QueryRow(ctx, query, id)

	var spec HoldoutSpec
	var encrypted int
	var createdAt string
	if err := row.Scan(
		&spec.ID, &spec.BlueprintID, &encrypted, &spec.FilePath, &spec.TestCount, &createdAt,
	); err != nil {
		return nil, fmt.Errorf("get holdout spec %s: %w", id, err)
	}

	spec.Encrypted = encrypted == 1
	spec.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return &spec, nil
}

// GetSpecByBlueprint retrieves holdout specs for a blueprint.
func (e *HoldoutEngine) GetSpecByBlueprint(ctx context.Context, blueprintID string) ([]*HoldoutSpec, error) {
	query := `SELECT id, blueprint_id, encrypted, file_path, test_count, created_at
		FROM holdout_specs WHERE blueprint_id = ? ORDER BY created_at DESC`

	rows, err := e.db.Query(ctx, query, blueprintID)
	if err != nil {
		return nil, fmt.Errorf("query holdout specs: %w", err)
	}
	defer rows.Close()

	var specs []*HoldoutSpec
	for rows.Next() {
		var spec HoldoutSpec
		var encrypted int
		var createdAt string
		if err := rows.Scan(
			&spec.ID, &spec.BlueprintID, &encrypted, &spec.FilePath, &spec.TestCount, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan holdout spec: %w", err)
		}
		spec.Encrypted = encrypted == 1
		spec.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		specs = append(specs, &spec)
	}

	return specs, rows.Err()
}

// RecordResult persists a holdout verification result.
func (e *HoldoutEngine) RecordResult(ctx context.Context, result *HoldoutResult) error {
	if result.ID == "" {
		result.ID = GenerateResultID()
	}
	if result.VerifiedAt.IsZero() {
		result.VerifiedAt = time.Now().UTC()
	}

	detailsJSON, _ := json.Marshal(result.Details)

	drift := 0
	if result.BehavioralDrift {
		drift = 1
	}

	query := `INSERT INTO holdout_results (
		id, holdout_id, blueprint_id, agent_id, score,
		tests_passed, tests_total, behavioral_drift, details,
		verified_at, trace_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	return e.db.Exec(ctx, query,
		result.ID, result.HoldoutID, result.BlueprintID, result.AgentID,
		result.Score, result.TestsPassed, result.TestsTotal, drift,
		string(detailsJSON), result.VerifiedAt.Format(time.RFC3339),
		nilIfEmpty(result.TraceID),
	)
}

// GetResults retrieves holdout results for a blueprint.
func (e *HoldoutEngine) GetResults(ctx context.Context, blueprintID string) ([]*HoldoutResult, error) {
	query := `SELECT id, holdout_id, blueprint_id, agent_id, score,
		tests_passed, tests_total, behavioral_drift, details,
		verified_at, trace_id
		FROM holdout_results WHERE blueprint_id = ?
		ORDER BY verified_at DESC`

	rows, err := e.db.Query(ctx, query, blueprintID)
	if err != nil {
		return nil, fmt.Errorf("query holdout results: %w", err)
	}
	defer rows.Close()

	var results []*HoldoutResult
	for rows.Next() {
		var r HoldoutResult
		var drift int
		var detailsJSON string
		var verifiedAt string
		var traceID *string
		if err := rows.Scan(
			&r.ID, &r.HoldoutID, &r.BlueprintID, &r.AgentID, &r.Score,
			&r.TestsPassed, &r.TestsTotal, &drift, &detailsJSON,
			&verifiedAt, &traceID,
		); err != nil {
			return nil, fmt.Errorf("scan holdout result: %w", err)
		}
		r.BehavioralDrift = drift == 1
		_ = json.Unmarshal([]byte(detailsJSON), &r.Details)
		r.VerifiedAt, _ = time.Parse(time.RFC3339, verifiedAt)
		if traceID != nil {
			r.TraceID = *traceID
		}
		results = append(results, &r)
	}

	return results, rows.Err()
}

// ComputeScore calculates the weighted score from test results.
func ComputeScore(results []HoldoutTestResult) (float64, int) {
	if len(results) == 0 {
		return 0, 0
	}
	var totalWeight float64
	var weightedScore float64
	passed := 0

	for _, r := range results {
		totalWeight += r.Weight
		if r.Passed {
			weightedScore += r.Weight
			passed++
		}
	}

	if totalWeight == 0 {
		return 0, passed
	}
	return weightedScore / totalWeight, passed
}

// DetectDrift returns true if the score indicates behavioral drift.
func (e *HoldoutEngine) DetectDrift(score float64) bool {
	return score < e.driftThreshold
}

// GenerateHoldoutID creates a holdout spec ID.
func GenerateHoldoutID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "hold-" + hex.EncodeToString(b)
}

// GenerateResultID creates a holdout result ID.
func GenerateResultID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "hr-" + hex.EncodeToString(b)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
