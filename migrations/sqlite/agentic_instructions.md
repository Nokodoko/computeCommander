# migrations/sqlite/ -- Root-Level SQLite Migrations (Mirror)

## Purpose
Mirror copy of `internal/platform/db/migrations/sqlite/`. These files are kept at the root level for reference and manual migration workflows. The authoritative migrations are embedded in the Go binary from `internal/platform/db/migrations/`.

## Technology
- SQL (SQLite dialect)

## Contents
| File | Description |
|------|-------------|
| `001_schema.sql` | Identical to `internal/platform/db/migrations/sqlite/001_schema.sql` -- initial schema with 10 tables |

## Key Functions
N/A -- see `internal/platform/db/agentic_instructions.md` for full schema documentation.

## Logging
N/A

## CRUD Entry Points
- Reference only; the embedded copy in `internal/platform/db/migrations/` is authoritative.
