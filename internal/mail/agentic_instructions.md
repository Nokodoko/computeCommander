# internal/mail/ -- Agent-to-Agent Messaging System

## Purpose
Implements the inter-agent mail system for ComputeCommander. Provides a priority-ordered message queue with broadcast address expansion, thread-based replies, and filtered queries. Messages are persisted in the database (SQLite or Postgres).

## Technology
- Go 1.25
- `encoding/json` for payload serialization
- Depends on: `internal/platform/db`

## Contents
| File | Description |
|------|-------------|
| `types.go` | `MessageType` enum (12 types), `Priority` enum (4 levels), broadcast address constants, `IsBroadcast()`, `MailMessage` struct |
| `store.go` | `MailStore` interface (Send, Check, List, MarkRead, Reply, Purge), `CheckOpts`, `ListOpts`, `PurgeOpts` |
| `sql_store.go` | `sqlStore` implementation: SQL-backed store with broadcast expansion, priority-ordered queries, thread replies |
| `store_test.go` | Tests for send, check, list, mark read, reply, purge, and broadcast expansion |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewMailStore` | `func NewMailStore(database db.DB, resolver AgentResolver) MailStore` | `MailStore` | Creates SQL-backed mail store with optional broadcast resolver |
| `Send` | `func (s *sqlStore) Send(msg *MailMessage) error` | `error` | Inserts message; expands broadcast addresses to individual rows |
| `Check` | `func (s *sqlStore) Check(agent string, opts CheckOpts) ([]*MailMessage, error)` | `[]*MailMessage, error` | Returns unread messages for agent, ordered by priority DESC then time ASC |
| `List` | `func (s *sqlStore) List(opts ListOpts) ([]*MailMessage, error)` | `[]*MailMessage, error` | Returns messages matching agent, from, type, thread, unread filters |
| `MarkRead` | `func (s *sqlStore) MarkRead(id string) error` | `error` | Updates read flag to 1 |
| `Reply` | `func (s *sqlStore) Reply(id string, body string) error` | `error` | Creates threaded reply: swaps from/to, prepends "Re:", links thread |
| `Purge` | `func (s *sqlStore) Purge(opts PurgeOpts) (int, error)` | `int, error` | Deletes messages matching agent, before time, read-only filters; returns count |

## Data Types

### MessageType (string enum)
Semantic: `status`, `question`, `result`, `error`
Protocol: `worker_done`, `merge_ready`, `merged`, `merge_failed`, `escalation`, `health_check`, `dispatch`, `assign`

### Priority (string enum)
`low` (0) | `normal` (1) | `high` (2) | `urgent` (3) -- numeric weights for SQL ORDER BY DESC

### MailMessage (struct)
Fields: ID, From, To, Subject, Body, Priority, Type, ThreadID, Payload (json.RawMessage), Read, CreatedAt

### AgentResolver (interface)
`ResolveAddress(addr string) ([]string, error)` -- expands broadcast addresses to agent names

### Broadcast Addresses
`@all`, `@builders`, `@scouts`, `@reviewers`, `@leads`, `@workers`

## Logging
- No structured logging; errors returned via `fmt.Errorf` with contextual prefixes

## CRUD Entry Points
- **Create**: `Send()` inserts new messages (one per recipient for broadcasts)
- **Read**: `Check()` reads unread by agent, `List()` reads with filters
- **Update**: `MarkRead()` marks a message as read
- **Delete**: `Purge()` deletes messages matching criteria

## Style Guide
- Interface-first design: `MailStore` interface with `sqlStore` implementation
- SQL uses `?` positional params (compatible with both postgres and sqlite)
- Priority ordering via SQL CASE expression
- Timestamps stored as RFC3339 strings, with fallback to SQLite datetime format
- Import order: stdlib, internal packages

**Representative snippet (from `sql_store.go`):**
```go
func (s *sqlStore) Check(agent string, opts CheckOpts) ([]*MailMessage, error) {
	ctx := context.Background()

	var clauses []string
	var args []any

	clauses = append(clauses, "to_agent = ?")
	args = append(args, agent)

	clauses = append(clauses, "read = 0")

	if opts.Type != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, string(opts.Type))
	}

	query := `SELECT id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at
		FROM mail WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY
			CASE priority
				WHEN 'urgent' THEN 3
				WHEN 'high' THEN 2
				WHEN 'normal' THEN 1
				WHEN 'low' THEN 0
				ELSE 1
			END DESC,
			created_at ASC`

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	return s.queryMessages(ctx, query, args...)
}
```
