package db

import (
	"context"
	"testing"
)

func newTestDB(t *testing.T) DB {
	t.Helper()
	d, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := Migrate(d, "sqlite"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNewSQLiteMemory(t *testing.T) {
	d, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	defer d.Close()

	if d.Driver() != "sqlite" {
		t.Errorf("expected driver 'sqlite', got %q", d.Driver())
	}
}

func TestMigrateSQLite(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Verify all 10 tables exist.
	tables := []string{
		"runs", "sessions", "events", "mail", "metrics",
		"merge_queue", "task_groups", "task_group_members",
		"checkpoints", "worktrees",
	}
	for _, table := range tables {
		var name string
		err := d.QueryRow(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	d := newTestDB(t)

	// Running migrate again should not fail (IF NOT EXISTS).
	if err := Migrate(d, "sqlite"); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestRunsCRUD(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Insert
	err := d.Exec(ctx,
		"INSERT INTO runs (id, agent_count, status) VALUES (?, ?, ?)",
		"run-1", 3, "active",
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	// Read
	var id, status string
	var count int
	err = d.QueryRow(ctx, "SELECT id, agent_count, status FROM runs WHERE id = ?", "run-1").
		Scan(&id, &count, &status)
	if err != nil {
		t.Fatalf("select run: %v", err)
	}
	if id != "run-1" || count != 3 || status != "active" {
		t.Errorf("unexpected run: id=%s count=%d status=%s", id, count, status)
	}

	// Update
	err = d.Exec(ctx, "UPDATE runs SET status = ? WHERE id = ?", "completed", "run-1")
	if err != nil {
		t.Fatalf("update run: %v", err)
	}

	err = d.QueryRow(ctx, "SELECT status FROM runs WHERE id = ?", "run-1").Scan(&status)
	if err != nil {
		t.Fatalf("select after update: %v", err)
	}
	if status != "completed" {
		t.Errorf("expected 'completed', got %q", status)
	}

	// Delete
	err = d.Exec(ctx, "DELETE FROM runs WHERE id = ?", "run-1")
	if err != nil {
		t.Fatalf("delete run: %v", err)
	}

	var cnt int
	err = d.QueryRow(ctx, "SELECT COUNT(*) FROM runs").Scan(&cnt)
	if err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if cnt != 0 {
		t.Errorf("expected 0 runs, got %d", cnt)
	}
}

func TestSessionsCRUD(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Create a run first (foreign key).
	err := d.Exec(ctx, "INSERT INTO runs (id, status) VALUES (?, ?)", "run-1", "active")
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	err = d.Exec(ctx, `INSERT INTO sessions (id, agent_name, capability, task_id, run_id, runtime)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-1", "builder-1", "builder", "task-42", "run-1", "claude",
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	var name, cap, state string
	err = d.QueryRow(ctx, "SELECT agent_name, capability, state FROM sessions WHERE id = ?", "sess-1").
		Scan(&name, &cap, &state)
	if err != nil {
		t.Fatalf("select session: %v", err)
	}
	if name != "builder-1" || cap != "builder" || state != "booting" {
		t.Errorf("unexpected session: name=%s cap=%s state=%s", name, cap, state)
	}
}

func TestMailCRUD(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.Exec(ctx, `INSERT INTO mail (id, from_agent, to_agent, subject, body, type)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"mail-1", "coord", "builder-1", "task assignment", "build auth module", "dispatch",
	)
	if err != nil {
		t.Fatalf("insert mail: %v", err)
	}

	var subject, body string
	var read int
	err = d.QueryRow(ctx, "SELECT subject, body, read FROM mail WHERE id = ?", "mail-1").
		Scan(&subject, &body, &read)
	if err != nil {
		t.Fatalf("select mail: %v", err)
	}
	if subject != "task assignment" || read != 0 {
		t.Errorf("unexpected mail: subject=%q read=%d", subject, read)
	}

	// Mark as read
	err = d.Exec(ctx, "UPDATE mail SET read = 1 WHERE id = ?", "mail-1")
	if err != nil {
		t.Fatalf("update mail: %v", err)
	}
}

func TestQueryMultipleRows(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := d.Exec(ctx, "INSERT INTO runs (id, status) VALUES (?, ?)",
			"run-"+string(rune('a'+i)), "active")
		if err != nil {
			t.Fatalf("insert run %d: %v", i, err)
		}
	}

	rows, err := d.Query(ctx, "SELECT id FROM runs ORDER BY id")
	if err != nil {
		t.Fatalf("query runs: %v", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(ids) != 5 {
		t.Errorf("expected 5 rows, got %d", len(ids))
	}
}

func TestTransaction(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Commit path
	tx, err := d.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = tx.Exec(ctx, "INSERT INTO runs (id, status) VALUES (?, ?)", "run-tx", "active")
	if err != nil {
		t.Fatalf("tx exec: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var id string
	err = d.QueryRow(ctx, "SELECT id FROM runs WHERE id = ?", "run-tx").Scan(&id)
	if err != nil {
		t.Fatalf("select after commit: %v", err)
	}

	// Rollback path
	tx2, err := d.Begin(ctx)
	if err != nil {
		t.Fatalf("begin 2: %v", err)
	}
	err = tx2.Exec(ctx, "INSERT INTO runs (id, status) VALUES (?, ?)", "run-rb", "active")
	if err != nil {
		t.Fatalf("tx2 exec: %v", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var cnt int
	err = d.QueryRow(ctx, "SELECT COUNT(*) FROM runs WHERE id = ?", "run-rb").Scan(&cnt)
	if err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if cnt != 0 {
		t.Errorf("expected 0 after rollback, got %d", cnt)
	}
}

func TestNewDBFactory(t *testing.T) {
	cfg := DatabaseConfig{
		Driver: "sqlite",
	}
	d, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB sqlite: %v", err)
	}
	defer d.Close()

	if d.Driver() != "sqlite" {
		t.Errorf("expected 'sqlite', got %q", d.Driver())
	}
}

func TestNewDBAutoFallback(t *testing.T) {
	// Auto mode with no Postgres available should fall back to SQLite.
	cfg := DatabaseConfig{
		Driver: "auto",
		Postgres: PostgresConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "nonexistent_test_db",
			User:     "nobody",
			Password: "wrong",
			SSLMode:  "disable",
		},
	}
	d, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB auto: %v", err)
	}
	defer d.Close()

	if d.Driver() != "sqlite" {
		t.Errorf("expected fallback to 'sqlite', got %q", d.Driver())
	}
}

func TestNewDBInvalidDriver(t *testing.T) {
	cfg := DatabaseConfig{Driver: "mysql"}
	_, err := NewDB(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestWorktreesCRUD(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.Exec(ctx, `INSERT INTO worktrees (path, branch, agent_name, task_id)
		VALUES (?, ?, ?, ?)`,
		"/tmp/wt-1", "feat/auth", "builder-1", "task-42",
	)
	if err != nil {
		t.Fatalf("insert worktree: %v", err)
	}

	var path, branch, state string
	err = d.QueryRow(ctx, "SELECT path, branch, state FROM worktrees WHERE path = ?", "/tmp/wt-1").
		Scan(&path, &branch, &state)
	if err != nil {
		t.Fatalf("select worktree: %v", err)
	}
	if branch != "feat/auth" || state != "active" {
		t.Errorf("unexpected worktree: branch=%s state=%s", branch, state)
	}
}

func TestForeignKeyConstraint(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Inserting a session with a non-existent run_id should fail with FK on.
	err := d.Exec(ctx, `INSERT INTO sessions (id, agent_name, capability, task_id, run_id, runtime)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-fk", "agent", "builder", "task-1", "nonexistent-run", "claude",
	)
	if err == nil {
		t.Error("expected foreign key constraint violation")
	}
}

func TestCheckpointsCRUD(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	// Need a run and session first.
	d.Exec(ctx, "INSERT INTO runs (id, status) VALUES (?, ?)", "run-cp", "active")
	d.Exec(ctx, `INSERT INTO sessions (id, agent_name, capability, task_id, run_id, runtime)
		VALUES (?, ?, ?, ?, ?, ?)`, "sess-cp", "builder-1", "builder", "task-10", "run-cp", "claude")

	err := d.Exec(ctx, `INSERT INTO checkpoints (agent_name, task_id, session_id, progress_summary, current_branch, pending_work)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"builder-1", "task-10", "sess-cp", "implemented auth module", "feat/auth", "add tests",
	)
	if err != nil {
		t.Fatalf("insert checkpoint: %v", err)
	}

	var summary, branch string
	err = d.QueryRow(ctx, "SELECT progress_summary, current_branch FROM checkpoints WHERE session_id = ?", "sess-cp").
		Scan(&summary, &branch)
	if err != nil {
		t.Fatalf("select checkpoint: %v", err)
	}
	if summary != "implemented auth module" || branch != "feat/auth" {
		t.Errorf("unexpected checkpoint: summary=%q branch=%q", summary, branch)
	}
}
