# pkg/integrations/linear/ -- Linear Integration (Stub)

## Purpose
Stub Linear (project management) integration client for ComputeCommander. Provides interfaces for syncing issues and updating issue status between ComputeCommander agents and Linear projects via GraphQL API.

## Technology
- Go 1.25
- `context` for cancellation
- No external dependencies; stubs only (future: Linear GraphQL API)

## Contents
| File | Description |
|------|-------------|
| `linear.go` | `LinearClient` struct, `NewLinearClient()`, stub methods: `SyncIssues()`, `UpdateStatus()`, `Issue`/`SyncOpts`/`StatusUpdate` types |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewLinearClient` | `func NewLinearClient(opts ClientOpts) *LinearClient` | `*LinearClient` | Creates client with API key, team ID, base URL |
| `SyncIssues` | `func (c *LinearClient) SyncIssues(ctx, opts SyncOpts) ([]*Issue, error)` | `[]*Issue, error` | Stub: retrieves issues filtered by time, status, limit |
| `UpdateStatus` | `func (c *LinearClient) UpdateStatus(ctx, update StatusUpdate) error` | `error` | Stub: changes issue status with optional comment |
| `TeamID` | `func (c *LinearClient) TeamID() string` | `string` | Returns configured team identifier |

## Data Types

### Issue (struct)
Fields: ID, Title, Description, Status, Priority, Assignee, Labels, CreatedAt, UpdatedAt

### SyncOpts (struct)
Fields: Since (time filter), Statuses (status filter), Limit

### StatusUpdate (struct)
Fields: IssueID, Status, Comment

### ClientOpts (struct)
Fields: APIKey, TeamID, BaseURL (defaults to `https://api.linear.app`)

## Logging
N/A

## CRUD Entry Points
- **Read**: `SyncIssues()` (stub)
- **Update**: `UpdateStatus()` (stub)

## Style Guide
- Stub pattern: returns empty slices and nil errors
- Validation on required fields (TeamID, IssueID, Status)
- BaseURL defaults to production Linear API

**Representative snippet (from `linear.go`):**
```go
func (c *LinearClient) SyncIssues(ctx context.Context, opts SyncOpts) ([]*Issue, error) {
	if c.teamID == "" {
		return nil, fmt.Errorf("linear: team ID is required for syncing issues")
	}
	// Stub: actual implementation would query Linear's GraphQL API.
	return []*Issue{}, nil
}
```
