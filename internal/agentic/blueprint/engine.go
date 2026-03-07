package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// BlueprintEngine manages blueprint CRUD, execution, validation, and dependency graph.
type BlueprintEngine struct {
	db db.DB
}

// NewBlueprintEngine creates a new BlueprintEngine.
func NewBlueprintEngine(database db.DB) *BlueprintEngine {
	return &BlueprintEngine{db: database}
}

// Create persists a new blueprint to the database.
func (e *BlueprintEngine) Create(ctx context.Context, bp *Blueprint) error {
	if bp.ID == "" {
		bp.ID = GenerateBlueprintID()
	}
	if bp.Status == "" {
		bp.Status = StatusPending
	}
	now := time.Now().UTC()
	if bp.CreatedAt.IsZero() {
		bp.CreatedAt = now
	}
	bp.UpdatedAt = now

	contextJSON, _ := json.Marshal(bp.ContextGrants)
	inputsJSON, _ := json.Marshal(bp.Inputs)
	outputsJSON, _ := json.Marshal(bp.Outputs)
	verifyJSON, _ := json.Marshal(bp.VerifySteps)
	gatesJSON, _ := json.Marshal(bp.Gates)
	depsJSON, _ := json.Marshal(bp.DependsOn)

	query := `INSERT INTO blueprints (
		id, version, name, agent, capability,
		context_grants, inputs, outputs, verify_steps, gates, depends_on,
		retry_limit, timeout, status, attempts, last_error,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	return e.db.Exec(ctx, query,
		bp.ID, bp.Version, bp.Name, bp.Agent, bp.Capability,
		string(contextJSON), string(inputsJSON), string(outputsJSON),
		string(verifyJSON), string(gatesJSON), string(depsJSON),
		bp.RetryLimit, bp.Timeout, string(bp.Status), bp.Attempts,
		nilIfEmpty(bp.LastError),
		bp.CreatedAt.Format(time.RFC3339), bp.UpdatedAt.Format(time.RFC3339),
	)
}

// Get retrieves a blueprint by ID.
func (e *BlueprintEngine) Get(ctx context.Context, id string) (*Blueprint, error) {
	query := `SELECT id, version, name, agent, capability,
		context_grants, inputs, outputs, verify_steps, gates, depends_on,
		retry_limit, timeout, status, attempts, last_error,
		created_at, updated_at
		FROM blueprints WHERE id = ?`

	row := e.db.QueryRow(ctx, query, id)

	var bp Blueprint
	var contextJSON, inputsJSON, outputsJSON, verifyJSON, gatesJSON, depsJSON string
	var lastError *string
	var createdAt, updatedAt string

	if err := row.Scan(
		&bp.ID, &bp.Version, &bp.Name, &bp.Agent, &bp.Capability,
		&contextJSON, &inputsJSON, &outputsJSON, &verifyJSON, &gatesJSON, &depsJSON,
		&bp.RetryLimit, &bp.Timeout, &bp.Status, &bp.Attempts, &lastError,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, fmt.Errorf("get blueprint %s: %w", id, err)
	}

	_ = json.Unmarshal([]byte(contextJSON), &bp.ContextGrants)
	_ = json.Unmarshal([]byte(inputsJSON), &bp.Inputs)
	_ = json.Unmarshal([]byte(outputsJSON), &bp.Outputs)
	_ = json.Unmarshal([]byte(verifyJSON), &bp.VerifySteps)
	_ = json.Unmarshal([]byte(gatesJSON), &bp.Gates)
	_ = json.Unmarshal([]byte(depsJSON), &bp.DependsOn)
	if lastError != nil {
		bp.LastError = *lastError
	}
	bp.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	bp.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &bp, nil
}

// List retrieves blueprints with optional filtering.
func (e *BlueprintEngine) List(ctx context.Context, status Status, agent string) ([]*Blueprint, error) {
	query := "SELECT id, version, name, agent, capability, status, attempts, created_at, updated_at FROM blueprints"
	var conditions []string
	var args []any

	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(status))
	}
	if agent != "" {
		conditions = append(conditions, "agent = ?")
		args = append(args, agent)
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, c := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += c
		}
	}
	query += " ORDER BY created_at DESC"

	rows, err := e.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list blueprints: %w", err)
	}
	defer rows.Close()

	var blueprints []*Blueprint
	for rows.Next() {
		var bp Blueprint
		var createdAt, updatedAt string
		if err := rows.Scan(
			&bp.ID, &bp.Version, &bp.Name, &bp.Agent, &bp.Capability,
			&bp.Status, &bp.Attempts, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan blueprint: %w", err)
		}
		bp.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		bp.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		blueprints = append(blueprints, &bp)
	}

	return blueprints, rows.Err()
}

// UpdateStatus atomically transitions a blueprint's status using compare-and-swap.
func (e *BlueprintEngine) UpdateStatus(ctx context.Context, id string, from, to Status, lastError string) error {
	if !ValidTransition(from, to) {
		return fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	query := `UPDATE blueprints
		SET status = ?, version = version + 1, updated_at = ?, last_error = ?
		WHERE id = ? AND status = ?`

	now := time.Now().UTC().Format(time.RFC3339)
	return e.db.Exec(ctx, query, string(to), now, nilIfEmpty(lastError), id, string(from))
}

// IncrementAttempts increments the attempt counter for a blueprint.
func (e *BlueprintEngine) IncrementAttempts(ctx context.Context, id string) error {
	query := `UPDATE blueprints SET attempts = attempts + 1, updated_at = ? WHERE id = ?`
	now := time.Now().UTC().Format(time.RFC3339)
	return e.db.Exec(ctx, query, now, id)
}

// Delete removes a blueprint by ID.
func (e *BlueprintEngine) Delete(ctx context.Context, id string) error {
	return e.db.Exec(ctx, "DELETE FROM blueprints WHERE id = ?", id)
}

// GetDependencyGraph returns a map of blueprint ID to its dependency IDs.
func (e *BlueprintEngine) GetDependencyGraph(ctx context.Context) (map[string][]string, error) {
	query := "SELECT id, depends_on FROM blueprints"
	rows, err := e.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query dependency graph: %w", err)
	}
	defer rows.Close()

	graph := make(map[string][]string)
	for rows.Next() {
		var id, depsJSON string
		if err := rows.Scan(&id, &depsJSON); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		var deps []string
		_ = json.Unmarshal([]byte(depsJSON), &deps)
		graph[id] = deps
	}

	return graph, rows.Err()
}

// HasCycle detects cycles in the dependency graph.
func (e *BlueprintEngine) HasCycle(ctx context.Context) (bool, error) {
	graph, err := e.GetDependencyGraph(ctx)
	if err != nil {
		return false, err
	}

	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		inStack[node] = true

		for _, dep := range graph[node] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if inStack[dep] {
				return true
			}
		}

		inStack[node] = false
		return false
	}

	for node := range graph {
		if !visited[node] {
			if hasCycle(node) {
				return true, nil
			}
		}
	}

	return false, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
