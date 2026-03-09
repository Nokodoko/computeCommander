package agents

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// newTestDB opens an in-memory SQLite DB and runs all migrations.
func newTestDB(t *testing.T) db.DB {
	t.Helper()
	d, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := db.Migrate(d, "sqlite"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// insertTestSession writes a minimal session row with a known color_index.
func insertTestSession(t *testing.T, d db.DB, id, agentName string, colorIndex int, colorHex string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	err := d.Exec(ctx,
		`INSERT INTO sessions
			(id, agent_name, capability, task_id, runtime, color_index, color_hex, started_at, last_activity)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, agentName, "builder", "task-1", "claude", colorIndex, colorHex, now, now,
	)
	if err != nil {
		t.Fatalf("insertTestSession(%s): %v", id, err)
	}
}

// TestAgentsGetUniqueColorIndexes verifies that successive calls to
// countSessionsInRun return strictly increasing values as sessions are
// inserted, ensuring each agent receives a distinct color_index.
func TestAgentsGetUniqueColorIndexes(t *testing.T) {
	d := newTestDB(t)
	s := NewSpawner(SpawnerOpts{DB: d})
	ctx := context.Background()

	agents := []struct {
		id   string
		name string
	}{
		{"sess-1", "scout"},
		{"sess-2", "builder"},
		{"sess-3", "reviewer"},
		{"sess-4", "lead"},
	}

	seen := make(map[int]string) // color_index → agent name

	for _, a := range agents {
		idx, err := s.countSessionsInRun(ctx, a.name)
		if err != nil {
			t.Fatalf("countSessionsInRun(%s): %v", a.name, err)
		}

		if prev, exists := seen[idx]; exists {
			t.Errorf("color_index %d assigned to both %q and %q", idx, prev, a.name)
		}
		seen[idx] = a.name

		color := AssignColor(idx)
		insertTestSession(t, d, a.id, a.name, color.Index, color.Hex)
	}
}

// TestAgentsSkipLegacyColorZero verifies that when multiple existing active sessions
// all have color_index=0 (legacy migration default), new agents still receive distinct
// non-zero indexes rather than colliding at index 0.
func TestAgentsSkipLegacyColorZero(t *testing.T) {
	d := newTestDB(t)
	s := NewSpawner(SpawnerOpts{DB: d})
	ctx := context.Background()

	// Simulate legacy migrated sessions: all have color_index=0.
	legacyAgents := []struct{ id, name string }{
		{"old-1", "primary"},
		{"old-2", "supervisor"},
		{"old-3", "worker"},
	}
	for _, a := range legacyAgents {
		insertTestSession(t, d, a.id, a.name, 0, DefaultGrayHex)
	}

	// Now spawn new agents — they must each get a unique, non-colliding index.
	seen := make(map[int]string)
	for i, name := range []string{"alpha", "beta", "gamma"} {
		id := fmt.Sprintf("new-%d", i)
		idx, err := s.countSessionsInRun(ctx, name)
		if err != nil {
			t.Fatalf("countSessionsInRun(%s): %v", name, err)
		}
		if prev, exists := seen[idx]; exists {
			t.Errorf("color_index %d assigned to both %q and %q", idx, prev, name)
		}
		seen[idx] = name
		color := AssignColor(idx)
		insertTestSession(t, d, id, name, color.Index, color.Hex)
	}
}

// TestBuildColorResolverReturnsDifferentColors verifies that BuildColorResolver
// maps distinct agent names to distinct color hex strings.
func TestBuildColorResolverReturnsDifferentColors(t *testing.T) {
	d := newTestDB(t)
	s := NewSpawner(SpawnerOpts{DB: d})
	ctx := context.Background()

	entries := []struct {
		id    string
		name  string
		index int
	}{
		{"sess-a", "alpha", 0},
		{"sess-b", "beta", 1},
		{"sess-c", "gamma", 2},
	}

	for _, e := range entries {
		color := AssignColor(e.index)
		insertTestSession(t, d, e.id, e.name, color.Index, color.Hex)
	}

	resolve := s.BuildColorResolver(ctx)

	hexes := make(map[string]string) // hex → agent name
	for _, e := range entries {
		hex := resolve(e.name)
		if hex == "" {
			t.Errorf("resolve(%q) returned empty string", e.name)
			continue
		}
		if prev, exists := hexes[hex]; exists {
			t.Errorf("color %q shared by %q and %q", hex, prev, e.name)
		}
		hexes[hex] = e.name
	}
}

// TestBuildColorResolverFallsBackToAgentColors verifies that BuildColorResolver
// finds colors from the agent_colors history table when no active session exists.
func TestBuildColorResolverFallsBackToAgentColors(t *testing.T) {
	d := newTestDB(t)
	s := NewSpawner(SpawnerOpts{DB: d})
	ctx := context.Background()

	// Insert run rows required by agent_colors FK constraint.
	for i := range []string{"archivist", "oracle"} {
		runID := fmt.Sprintf("run-%d", i)
		if err := d.Exec(ctx, `INSERT INTO runs (id, status) VALUES ($1, $2)`, runID, "active"); err != nil {
			t.Fatalf("insert run %s: %v", runID, err)
		}
	}

	// Insert directly into agent_colors (no session row).
	for i, name := range []string{"archivist", "oracle"} {
		color := AssignColor(i)
		err := d.Exec(ctx,
			`INSERT INTO agent_colors (agent_name, run_id, color_index, color_hex) VALUES ($1,$2,$3,$4)`,
			name, fmt.Sprintf("run-%d", i), color.Index, color.Hex,
		)
		if err != nil {
			t.Fatalf("insert agent_colors(%s): %v", name, err)
		}
	}

	resolve := s.BuildColorResolver(ctx)

	archivistHex := resolve("archivist")
	oracleHex := resolve("oracle")

	if archivistHex == "" {
		t.Error("resolve(archivist) returned empty string")
	}
	if oracleHex == "" {
		t.Error("resolve(oracle) returned empty string")
	}
	if archivistHex == oracleHex {
		t.Errorf("archivist and oracle share the same color %q", archivistHex)
	}
}
