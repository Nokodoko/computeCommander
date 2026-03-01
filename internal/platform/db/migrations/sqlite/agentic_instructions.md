# internal/platform/db/migrations/sqlite/ -- SQLite Schema Migrations

## Purpose
Contains embedded SQL migration files for creating and evolving the SQLite database schema. Applied automatically by `db.Migrate()` during project initialization and app startup.

## Technology
- SQL (SQLite dialect)
- Embedded via Go `//go:embed` in `internal/platform/db/migrate.go`

## Contents
| File | Description |
|------|-------------|
| `001_schema.sql` | Initial schema: 10 tables (runs, sessions, events, mail, metrics, merge_queue, task_groups, task_group_members, checkpoints, worktrees) with CHECK constraints and indexes |

## Key Functions
N/A -- declarative SQL, no executable functions.

## Data Types
See `internal/platform/db/agentic_instructions.md` for the full table schema.

Tables created: runs, sessions, events, mail, metrics, merge_queue, task_groups, task_group_members, checkpoints, worktrees
Indexes: 13 indexes covering run_id, state, agent_name, to_agent, read, status, created_at

## Logging
N/A

## CRUD Entry Points
- **Create**: Applied by `db.Migrate(database, "sqlite")` during `cc init` and `NewApp()`

## Style Guide
- `CREATE TABLE IF NOT EXISTS` for idempotent application
- `CREATE INDEX IF NOT EXISTS` for safe re-runs
- TEXT type for timestamps (stored as ISO 8601 strings)
- INTEGER for booleans (`read INTEGER NOT NULL DEFAULT 0`)
- CHECK constraints for enum columns (state, capability, status)

**Representative snippet (from `001_schema.sql`):**
```sql
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    agent_name TEXT NOT NULL,
    capability TEXT NOT NULL
        CHECK (capability IN ('scout', 'builder', 'reviewer', 'lead', 'merger', 'coordinator', 'supervisor', 'monitor')),
    state TEXT NOT NULL DEFAULT 'booting'
        CHECK (state IN ('booting', 'working', 'completed', 'stalled', 'zombie')),
    pid INTEGER,
    run_id TEXT REFERENCES runs(id),
    started_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```
