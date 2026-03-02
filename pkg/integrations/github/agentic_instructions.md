# pkg/integrations/github/ -- GitHub Integration (Stub)

## Purpose
Stub GitHub client for ComputeCommander. Provides interfaces for creating issue comments, checking PR status, and triggering GitHub Actions workflows. All methods are stubs that will be replaced with actual GitHub API calls.

## Technology
- Go 1.25
- `context` for cancellation
- No external dependencies; stubs only

## Contents
| File | Description |
|------|-------------|
| `github.go` | `GitHubClient` struct, `NewGitHubClient()`, stub methods: `CreateIssueComment()`, `GetPRStatus()`, `TriggerWorkflow()` |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewGitHubClient` | `func NewGitHubClient(opts ClientOpts) *GitHubClient` | `*GitHubClient` | Creates client with owner, repo, API token |
| `CreateIssueComment` | `func (c *GitHubClient) CreateIssueComment(ctx, issueNumber, body) error` | `error` | Stub: posts comment on issue/PR |
| `GetPRStatus` | `func (c *GitHubClient) GetPRStatus(ctx, prNumber) (*PRStatus, error)` | `*PRStatus, error` | Stub: returns PR merge status |
| `TriggerWorkflow` | `func (c *GitHubClient) TriggerWorkflow(ctx, workflowID, ref) error` | `error` | Stub: triggers GitHub Actions workflow dispatch |

## Data Types

### ClientOpts (struct)
Fields: Owner, Repo, Token, BaseURL (defaults to `https://api.github.com`)

### PRStatus (struct)
Fields: Number, State, Mergeable, Title

## Logging
N/A

## CRUD Entry Points
- **Create**: `CreateIssueComment()`, `TriggerWorkflow()` (stubs)
- **Read**: `GetPRStatus()` (stub)

## Style Guide
- Stub pattern: all methods return nil/empty values
- Context parameter on all methods for future HTTP cancellation

**Representative snippet (from `github.go`):**
```go
func (c *GitHubClient) CreateIssueComment(ctx context.Context, issueNumber int, body string) error {
	// Stub: actual implementation would call GitHub REST API.
	_ = ctx
	return nil
}
```
