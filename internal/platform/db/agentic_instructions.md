# internal/platform/db/ -- Database Abstraction Layer

## Purpose
Provides a unified database abstraction (DB interface) over SQLite and PostgreSQL backends. Includes connection setup, WAL mode for SQLite, pgxpool for Postgres, embedded SQL migrations, and thin wrapper types for rows/transactions.

## Technology
- Go 1.25
- `modernc.org/sqlite` (pure-Go SQLite)
- `github.com/jackc/pgx/v5/pgxpool` (PostgreSQL connection pool)
- `embed` for SQL migration files
- Depends on: `internal/config` (DatabaseConfig)

## Contents
| File | Description |
|------|-------------|
| `db.go` | `DB` interface, `Tx` interface, `Rows`/`Row` wrapper types, `DatabaseConfig`, `NewDB()` factory |
| `sqlite.go` | `sqliteDB` implementation: WAL mode, `database/sql` wrapper, time scanning workaround for modernc.org/sqlite |
| `postgres.go` | `postgresDB` implementation: pgxpool-based, builds DSN from config |
| `migrate.go` | `Migrate()` applies embedded SQL migrations from `migrations/` subdirectory |
| `db_test.go` | Tests for SQLite operations, migrations, and DB interface compliance |
| `migrations/sqlite/001_schema.sql` | SQLite schema: runs, sessions, events, mail, metrics, merge_queue, task_groups, task_group_members, checkpoints, worktrees + indexes |
| `migrations/postgres/001_schema.sql` | PostgreSQL schema: same tables with native types (TIMESTAMPTZ, BIGSERIAL, JSONB, TEXT[]), CHECK constraints, uuid-ossp extension |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewDB` | `func NewDB(cfg DatabaseConfig) (DB, error)` | `DB, error` | Factory: dispatches to `NewSQLite` or `NewPostgres` based on driver |
| `NewSQLite` | `func NewSQLite(path string) (DB, error)` | `DB, error` | Opens SQLite with WAL mode, 1 connection, custom time scanning |
| `NewPostgres` | `func NewPostgres(cfg PostgresConfig) (DB, error)` | `DB, error` | Opens pgxpool from DSN built from config fields |
| `Migrate` | `func Migrate(database DB, driver string) error` | `error` | Reads embedded SQL files from `migrations/{driver}/` and executes them |

## Data Types

### DB (interface)
Methods: `Exec(ctx, query, args...) error`, `Query(ctx, query, args...) (*Rows, error)`, `QueryRow(ctx, query, args...) *Row`, `Close() error`, `Begin(ctx) (Tx, error)`, `Driver() string`

### Tx (interface)
Methods: `Exec(ctx, query, args...) error`, `Query(ctx, query, args...) (*Rows, error)`, `QueryRow(ctx, query, args...) *Row`, `Commit() error`, `Rollback() error`

### Rows (struct wrapper)
Wraps `*sql.Rows` with `Next()`, `Scan()`, `Close()`, `Err()` methods.

### Row (struct wrapper)
Wraps `*sql.Row` with `Scan()` method.

### DatabaseConfig (struct)
Fields: Driver, Postgres (PostgresConfig), SQLite (SQLiteConfig)

## Database Schema (10 tables)
| Table | PK | Purpose |
|-------|-----|---------|
| `runs` | id (TEXT) | Orchestration run tracking |
| `sessions` | id (TEXT) | Agent session lifecycle |
| `events` | id (AUTO) | Audit trail for tool use, spawns, errors |
| `mail` | id (TEXT) | Inter-agent messages |
| `metrics` | id (AUTO) | Token usage, cost, performance per agent |
| `merge_queue` | branch_name | FIFO merge queue entries |
| `task_groups` | id (TEXT) | Task group metadata |
| `task_group_members` | (group_id, issue_id) | Group-to-issue mapping |
| `checkpoints` | id (AUTO) | Session recovery snapshots |
| `worktrees` | path (TEXT) | Git worktree state tracking |

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `NewDB()` / `NewSQLite()` / `NewPostgres()` create connections; `Migrate()` creates tables
- **Read**: `Query()`, `QueryRow()` execute SELECT statements
- **Update**: `Exec()` executes INSERT/UPDATE/DELETE
- **Delete**: `Close()` closes the connection

## Style Guide
- Interface-first: `DB` and `Tx` interfaces with two implementations
- `Rows`/`Row` wrappers to avoid leaking `database/sql` types
- SQLite uses `database/sql` with `modernc.org/sqlite` driver
- Postgres uses `pgx/v5/pgxpool` directly
- Embedded migrations via `//go:embed migrations/**/*.sql`
- SQL uses `?` placeholders (SQLite-compatible; Postgres adapter translates)

**Representative snippet (from `db.go`):**
```go
type DB interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (*Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *Row
	Close() error
	Begin(ctx context.Context) (Tx, error)
	Driver() string
}
```
