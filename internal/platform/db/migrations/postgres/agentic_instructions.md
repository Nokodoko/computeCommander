# internal/platform/db/migrations/postgres/ -- PostgreSQL Schema Migrations

## Purpose
Contains embedded SQL migration files for creating and evolving the PostgreSQL database schema. Applied automatically by `db.Migrate()` when the database driver is configured as `"postgres"`.

## Technology
- SQL (PostgreSQL dialect)
- Embedded via Go `//go:embed` in `internal/platform/db/migrate.go`

## Contents
| File | Description |
|------|-------------|
| `001_schema.sql` | Initial schema: 10 tables with PostgreSQL-native types (TIMESTAMPTZ, BIGSERIAL, JSONB, TEXT[]), CHECK constraints, and indexes; enables uuid-ossp extension |

## Key Functions
N/A -- declarative SQL, no executable functions.

## Data Types
Same logical schema as SQLite but with PostgreSQL-native types:
- `TIMESTAMPTZ` instead of TEXT for timestamps
- `BIGSERIAL` instead of INTEGER AUTOINCREMENT for auto-incrementing IDs
- `JSONB` instead of TEXT for structured data (tool_args, data, payload)
- `TEXT[]` instead of TEXT for array columns (files_modified, mulch_domains)
- `VARCHAR(N)` with explicit length limits
- `DECIMAL(10, 4)` for monetary amounts
- `BOOLEAN` instead of INTEGER for read flag

## Logging
N/A

## CRUD Entry Points
- **Create**: Applied by `db.Migrate(database, "postgres")` during app initialization

## Style Guide
- `CREATE EXTENSION IF NOT EXISTS "uuid-ossp"` at top
- `CREATE TABLE IF NOT EXISTS` for idempotent application
- More granular indexing than SQLite (additional indexes on event_type, capability, agent_name, started_at)
- CHECK constraints match SQLite schema for enum consistency

**Representative snippet (from `001_schema.sql`):**
```sql
CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    run_id VARCHAR(64) REFERENCES runs(id),
    agent_name VARCHAR(128) NOT NULL,
    event_type VARCHAR(32) NOT NULL
        CHECK (event_type IN ('tool_start', 'tool_end', 'session_start', 'session_end',
                              'mail_sent', 'mail_received', 'spawn', 'error', 'custom')),
    tool_args JSONB,
    data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```
