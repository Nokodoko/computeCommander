# Feature Request: List Non-Sub-Task Issues via Jira REST API

## Summary

Add the ability to list only top-level Jira issues (Stories, Tasks, Bugs, Epics) while excluding sub-tasks from results. This applies to both the CLI `cmdr jira` command and the BubbleTea Jira TUI pane. The implementation filters at the JQL level using `issuetype not in subTaskIssueTypes()` so the API returns only parent-level work items.

## Why

The current `SyncProject` in `pkg/integrations/jira/sync.go` fetches **all** issues with `project=SAN ORDER BY updated DESC`. This includes sub-tasks, which clutter the board view and make it hard to see the actual work breakdown at the story/task level. Board 103 on `eccoselect-sandbox.atlassian.net` groups sub-tasks under parent stories, but the TUI pane shows them as flat siblings, breaking the mental model.

Users need:
- A clean list of actionable top-level tickets
- Sub-tasks hidden by default (viewable on drill-down into a parent)
- Consistent behavior between CLI list and TUI pane

---

## Specification

### 1. API Layer Changes

**File:** `pkg/integrations/jira/client.go`

#### 1.1 New Method: `SearchIssuesExcludeSubTasks`

```go
// SearchIssuesExcludeSubTasks searches for issues excluding sub-task types.
// It appends `AND issuetype not in subTaskIssueTypes()` to the provided JQL.
// Supports pagination via startAt parameter.
func (c *Client) SearchIssuesExcludeSubTasks(
    ctx context.Context,
    jql string,
    maxResults int,
    startAt int,
) (*SearchResult, error)
```

**JQL construction:**
- If `jql` is empty: `issuetype not in subTaskIssueTypes() ORDER BY Rank ASC`
- If `jql` is provided: `(<jql>) AND issuetype not in subTaskIssueTypes()`
- The `subTaskIssueTypes()` JQL function is a built-in Jira function that dynamically resolves all sub-task issue types (including custom ones). This is preferred over hardcoding `issuetype != Sub-task` because:
  - Custom sub-task types are automatically excluded
  - Works across Jira Cloud and Server
  - Forward-compatible with scheme changes

**Pagination:**
- `startAt` parameter (0-indexed offset) for iterating through large result sets
- `maxResults` capped at 100 per Jira API limits
- Returns `SearchResult` which already contains `StartAt`, `MaxResults`, `Total` for the caller to paginate

**Fields requested:**
```
summary, description, status, issuetype, priority, assignee, labels, project, epic, parent
```

#### 1.2 New Method: `SearchIssuesPaginated`

```go
// SearchIssuesPaginated fetches all pages of a JQL search, returning the
// complete issue list. Stops when all results are fetched or ctx is cancelled.
func (c *Client) SearchIssuesPaginated(
    ctx context.Context,
    jql string,
    excludeSubTasks bool,
) ([]APIIssue, error)
```

Internally calls `SearchIssues` or `SearchIssuesExcludeSubTasks` in a loop, incrementing `startAt` by `maxResults` (50) until `startAt >= total`.

#### 1.3 Update Existing `SearchIssues`

Add `startAt` parameter to the existing method signature:

```go
func (c *Client) SearchIssues(ctx context.Context, jql string, maxResults int, startAt int) (*SearchResult, error)
```

The POST body to `/rest/api/3/search/jql` already supports `startAt`:

```json
{
    "jql": "...",
    "maxResults": 50,
    "startAt": 0,
    "fields": ["summary", "description", "status", "issuetype", "priority", "assignee", "labels", "project", "epic", "parent"]
}
```

### 2. Sync Layer Changes

**File:** `pkg/integrations/jira/sync.go`

#### 2.1 Update `SyncProject`

Add an `excludeSubTasks` option to `SyncProject`. Two approaches (use option B):

**Option B: SyncOpts struct**

```go
type SyncOpts struct {
    ProjectKey      string
    ExcludeSubTasks bool // default: true
    MaxResults      int  // 0 = use default (100)
}

func (s *SyncEngine) SyncProjectWithOpts(ctx context.Context, opts SyncOpts) (*SyncResult, error)
```

When `ExcludeSubTasks` is true, the JQL becomes:
```
project=SAN AND issuetype not in subTaskIssueTypes() ORDER BY updated DESC
```

#### 2.2 Update `GetCachedIssues`

Add `excludeSubTasks` filter to the SQL query:

```go
func (s *SyncEngine) GetCachedIssues(ctx context.Context, projectKey, status string, excludeSubTasks bool) ([]JiraIssue, error)
```

When `excludeSubTasks` is true, add to the WHERE clause:
```sql
AND issue_type NOT IN ('Sub-task', 'Subtask')
```

Note: This filters from the local cache. The `issue_type` column is already populated from `apiIssue.Fields.IssueType.Name` during sync.

### 3. CLI Changes

**File:** `internal/commands/jira.go`

#### 3.1 New Flag: `--no-subtasks`

Add to the root `jira` command and the `jira list` subcommand:

```go
cmd.Flags().Bool("no-subtasks", true, "Exclude sub-tasks from results (default: true)")
```

Default is `true` -- users must explicitly pass `--no-subtasks=false` to see sub-tasks.

#### 3.2 CLI Output Format

Terminal table output for `cmdr jira --no-subtasks`:

```
KEY        STATUS         PRIORITY   ASSIGNEE       LABELS        SUMMARY
SAN-1      To Do          High       John Doe       backend,api   Implement user auth
SAN-2      In Progress    Medium     Jane Smith     frontend      Dashboard redesign
SAN-5      Done           Low        -              docs          Update API docs
---
3 issues (sub-tasks excluded)
```

JSON output for `cmdr jira --json --no-subtasks`:

```json
{
    "issues": [
        {
            "key": "SAN-1",
            "summary": "Implement user auth",
            "status": "To Do",
            "priority": "High",
            "assignee": "John Doe",
            "labels": ["backend", "api"],
            "issueType": "Story",
            "parentKey": null
        }
    ],
    "total": 3,
    "excludeSubTasks": true,
    "project": "SAN"
}
```

### 4. TUI Pane Changes

**File:** `internal/tui/jira_pane.go`

#### 4.1 JiraLister Interface Update

```go
type JiraLister interface {
    GetCachedIssues(ctx context.Context, projectKey, status string) ([]jira.JiraIssue, error)
    GetCachedIssuesFiltered(ctx context.Context, projectKey, status string, excludeSubTasks bool) ([]jira.JiraIssue, error)
}
```

Preserve backward compatibility by keeping the existing method. `GetCachedIssuesFiltered` is the new preferred call.

#### 4.2 JiraPane State

Add to `JiraPane`:

```go
type JiraPane struct {
    // ... existing fields ...
    excludeSubTasks bool // default: true
}
```

#### 4.3 Toggle Keybind

Add `t` keybind to toggle sub-task visibility:

```
t    Toggle sub-task visibility (currently: hidden)
```

When toggled:
- Re-fetch from cache with updated filter
- Rebuild hierarchy
- Show status in footer: `[sub-tasks: hidden]` or `[sub-tasks: visible]`

#### 4.4 Refresh Update

```go
func (p *JiraPane) Refresh(ctx context.Context) error {
    // Use filtered method when available, fall back to existing
    if filteredLister, ok := p.lister.(interface {
        GetCachedIssuesFiltered(context.Context, string, string, bool) ([]jira.JiraIssue, error)
    }); ok {
        issues, err = filteredLister.GetCachedIssuesFiltered(ctx, p.projectKey, "", p.excludeSubTasks)
    } else {
        issues, err = p.lister.GetCachedIssues(ctx, p.projectKey, "")
    }
    // ... rest unchanged
}
```

### 5. Data Model

No schema changes required. The `jira_issues` table already has `issue_type TEXT` which stores values like `"Story"`, `"Task"`, `"Bug"`, `"Epic"`, `"Sub-task"`.

Filtering is done at:
1. **API level** (JQL `issuetype not in subTaskIssueTypes()`) during sync
2. **Cache level** (SQL `WHERE issue_type NOT IN (...)`) during display

### 6. Authentication

No changes. Existing auth flow using `ClientOpts`:

- `JIRA_BASE_URL` env var -> `ClientOpts.BaseURL` (e.g., `https://eccoselect-sandbox.atlassian.net`)
- `JIRA_EMAIL` env var -> `ClientOpts.Username`
- `JIRA_API_TOKEN` env var -> `ClientOpts.Password`
- `ClientOpts.AuthType = "basic"`

### 7. Error Handling

| Scenario | Handling |
|----------|----------|
| `subTaskIssueTypes()` not supported (old Jira Server) | Fallback to `issuetype != Sub-task AND issuetype != Subtask` |
| JQL syntax error from API | Return error with JQL string for debugging |
| Empty result set | Display "No issues found (sub-tasks excluded). Try --no-subtasks=false" |
| Pagination mid-stream failure | Return partial results with error annotation |
| Rate limit (HTTP 429) | Existing `RateLimiter` + `Retry-After` header handling applies |
| Auth failure (HTTP 401/403) | Existing `decodeResponse` error path applies |

### 8. Edge Cases

1. **All issues are sub-tasks:** Returns empty list with informative message
2. **Mixed custom sub-task types:** `subTaskIssueTypes()` handles this natively
3. **Project has no issues:** Returns empty list (same as today)
4. **Sub-task without parent:** Still excluded (filter is type-based, not hierarchy-based)
5. **Concurrent sync + display:** Cache read uses snapshot isolation (SQLite WAL mode)
6. **Large projects (1000+ issues):** Pagination via `SearchIssuesPaginated` handles this

### 9. Files Modified

| File | Change |
|------|--------|
| `pkg/integrations/jira/client.go` | Add `SearchIssuesExcludeSubTasks`, `SearchIssuesPaginated`, update `SearchIssues` signature with `startAt` |
| `pkg/integrations/jira/client_test.go` | Tests for new methods, pagination, JQL construction |
| `pkg/integrations/jira/sync.go` | Add `SyncOpts`, `SyncProjectWithOpts`, update `GetCachedIssues` with filter param |
| `pkg/integrations/jira/sync_test.go` | Tests for filtered sync and cache queries |
| `internal/tui/jira_pane.go` | Add `excludeSubTasks` state, toggle keybind, filtered refresh |
| `internal/tui/jira_pane_test.go` | Tests for toggle behavior, filtered display |
| `internal/commands/jira.go` | Add `--no-subtasks` flag, wire through to list and pane |
| `internal/commands/jira_test.go` | Tests for flag parsing, filtered output |

### 10. Testing Strategy

1. **Unit tests:** Mock `Client.do()` with canned JQL responses to verify filter application
2. **Cache tests:** Insert mixed issue types into SQLite, verify `GetCachedIssues(excludeSubTasks=true)` returns only parents
3. **TUI tests:** Verify `JiraPane` with `excludeSubTasks=true` shows correct node count
4. **Integration test (manual):** Against `eccoselect-sandbox.atlassian.net` project SAN, compare `cmdr jira` vs `cmdr jira --no-subtasks=false` output
5. **Pagination test:** Mock 150 issues across 3 pages, verify all collected

### 11. Implementation Order

1. Add `startAt` to `SearchIssues` (backward-compatible signature change)
2. Add `SearchIssuesExcludeSubTasks` method
3. Add `SearchIssuesPaginated` helper
4. Add `SyncOpts` + `SyncProjectWithOpts`
5. Update `GetCachedIssues` with `excludeSubTasks` param
6. Add `--no-subtasks` CLI flag
7. Update `JiraPane` with toggle and filtered refresh
8. Write tests for all layers
9. Manual verification against sandbox

# Outcome

- `cmdr jira` lists only top-level issues by default (sub-tasks excluded)
- `cmdr jira --no-subtasks=false` shows all issues including sub-tasks
- TUI pane `t` keybind toggles sub-task visibility
- Pagination works for projects with >50 issues
- All existing tests continue to pass
- New tests cover filter logic, pagination, and edge cases
- No breaking changes to existing `SearchIssues` callers (startAt defaults to 0)
