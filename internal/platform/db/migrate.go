package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/postgres/*.sql
var postgresFS embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteFS embed.FS

// Migrate applies all pending migrations for the given driver.
// It tracks applied migrations in a _migrations table to ensure idempotency.
func Migrate(d DB, driver string) error {
	var fs embed.FS
	var dir string

	switch driver {
	case "postgres":
		fs = postgresFS
		dir = "migrations/postgres"
	case "sqlite":
		fs = sqliteFS
		dir = "migrations/sqlite"
	default:
		return fmt.Errorf("unsupported migration driver: %s", driver)
	}

	ctx := context.Background()

	// Create the migrations tracking table if it doesn't exist.
	err := d.Exec(ctx, `CREATE TABLE IF NOT EXISTS _migrations (
		name TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create _migrations table: %w", err)
	}

	entries, err := fs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Check if this migration has already been applied.
		var applied string
		err := d.QueryRow(ctx,
			"SELECT name FROM _migrations WHERE name = ?", name,
		).Scan(&applied)
		if err == nil {
			// Already applied, skip.
			continue
		}

		data, err := fs.ReadFile(dir + "/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := d.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		// Record the migration as applied.
		if err := d.Exec(ctx,
			"INSERT INTO _migrations (name) VALUES (?)", name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}

	return nil
}
