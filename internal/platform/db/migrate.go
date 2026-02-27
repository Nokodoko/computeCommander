package db

import (
	"context"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/postgres/*.sql
var postgresFS embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteFS embed.FS

// Migrate applies all pending migrations for the given driver.
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

	entries, err := fs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	ctx := context.Background()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if err := d.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}
