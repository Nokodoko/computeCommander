package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// SyncEngine syncs Jira data from the REST API into the local DB cache.
type SyncEngine struct {
	client       *Client
	db           db.DB
	instanceName string
}

// NewSyncEngine creates a sync engine for a specific Jira instance.
func NewSyncEngine(client *Client, database db.DB, instanceName string) *SyncEngine {
	return &SyncEngine{
		client:       client,
		db:           database,
		instanceName: instanceName,
	}
}

// SyncResult holds the outcome of a sync operation.
type SyncResult struct {
	Instance     string    `json:"instance"`
	ProjectsSync int       `json:"projectsSynced"`
	IssuesSync   int       `json:"issuesSynced"`
	EpicsSync    int       `json:"epicsSynced"`
	SyncedAt     time.Time `json:"syncedAt"`
	Error        string    `json:"error,omitempty"`
}

// SyncProject syncs a single project and its issues from Jira.
func (s *SyncEngine) SyncProject(ctx context.Context, projectKey string) (*SyncResult, error) {
	result := &SyncResult{
		Instance: s.instanceName,
		SyncedAt: time.Now(),
	}

	// Fetch project.
	proj, err := s.client.GetProject(ctx, projectKey)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Upsert project.
	lead := ""
	if proj.Lead != nil {
		lead = proj.Lead.DisplayName
	}
	err = s.db.Exec(ctx, `
		INSERT INTO jira_projects (id, instance_name, key, name, description, lead, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(instance_name, key) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			lead = excluded.lead,
			synced_at = excluded.synced_at`,
		proj.ID, s.instanceName, proj.Key, proj.Name, proj.Description, lead)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("upsert project: %w", err)
	}
	result.ProjectsSync = 1

	// Search all issues in the project.
	jql := fmt.Sprintf("project=%s ORDER BY updated DESC", projectKey)
	searchResult, err := s.client.SearchIssues(ctx, jql, 100, 0)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("search issues: %w", err)
	}

	// First pass: upsert epics so their IDs exist before non-epic issues reference them.
	for _, apiIssue := range searchResult.Issues {
		if apiIssue.Fields.IssueType.Name != "Epic" {
			continue
		}
		if err := s.upsertEpic(ctx, apiIssue, proj.ID); err != nil {
			result.Error = err.Error()
			return result, fmt.Errorf("upsert epic %s: %w", apiIssue.Key, err)
		}
		result.EpicsSync++
	}

	// Second pass: upsert all issues.
	for _, apiIssue := range searchResult.Issues {
		if err := s.upsertIssue(ctx, apiIssue, proj.ID); err != nil {
			result.Error = err.Error()
			return result, fmt.Errorf("upsert issue %s: %w", apiIssue.Key, err)
		}
		result.IssuesSync++
	}

	// Update sync state.
	err = s.db.Exec(ctx, `
		INSERT INTO jira_sync_state (instance_name, last_sync_at, last_sync_status, issues_synced)
		VALUES (?, datetime('now'), 'success', ?)
		ON CONFLICT(instance_name) DO UPDATE SET
			last_sync_at = excluded.last_sync_at,
			last_sync_status = excluded.last_sync_status,
			issues_synced = excluded.issues_synced`,
		s.instanceName, result.IssuesSync)
	if err != nil {
		return result, fmt.Errorf("update sync state: %w", err)
	}

	return result, nil
}

// upsertEpic inserts or updates an Epic-type issue into jira_epics.
func (s *SyncEngine) upsertEpic(ctx context.Context, issue APIIssue, projectID string) error {
	return s.db.Exec(ctx, `
		INSERT INTO jira_epics (id, instance_name, project_id, key, summary, status, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			key = excluded.key,
			summary = excluded.summary,
			status = excluded.status,
			synced_at = excluded.synced_at`,
		issue.ID, s.instanceName, projectID, issue.Key,
		issue.Fields.Summary, issue.Fields.Status.Name)
}

// upsertIssue inserts or updates a single issue in the cache.
func (s *SyncEngine) upsertIssue(ctx context.Context, issue APIIssue, projectID string) error {
	// Resolve epic_id only from IDs that exist in jira_epics to satisfy FK.
	var epicID *string
	var candidateEpicID string
	if issue.Fields.Parent != nil && issue.Fields.Parent.Fields.IssueType.Name == "Epic" {
		candidateEpicID = issue.Fields.Parent.ID
	}
	if issue.Fields.Epic != nil {
		candidateEpicID = issue.Fields.Epic.ID
	}
	if candidateEpicID != "" {
		// Verify epic exists in local cache before setting FK.
		row := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM jira_epics WHERE id = ?", candidateEpicID)
		var count int
		if err := row.Scan(&count); err == nil && count > 0 {
			epicID = &candidateEpicID
		}
	}

	// Resolve parent_key for non-epic parents (sub-task → Story/Task/Bug).
	var parentKey *string
	if issue.Fields.Parent != nil && issue.Fields.Parent.Fields.IssueType.Name != "Epic" {
		pk := issue.Fields.Parent.Key
		parentKey = &pk
	}

	assignee := ""
	if issue.Fields.Assignee != nil {
		assignee = issue.Fields.Assignee.DisplayName
	}

	labels, _ := json.Marshal(issue.Fields.Labels)
	if issue.Fields.Labels == nil {
		labels = []byte("[]")
	}

	return s.db.Exec(ctx, `
		INSERT INTO jira_issues (id, instance_name, project_id, epic_id, key, summary,
			description, status, issue_type, priority, assignee, labels, parent_key, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			epic_id = excluded.epic_id,
			key = excluded.key,
			summary = excluded.summary,
			description = excluded.description,
			status = excluded.status,
			issue_type = excluded.issue_type,
			priority = excluded.priority,
			assignee = excluded.assignee,
			labels = excluded.labels,
			parent_key = excluded.parent_key,
			synced_at = excluded.synced_at
		ON CONFLICT(instance_name, key) DO UPDATE SET
			epic_id = excluded.epic_id,
			summary = excluded.summary,
			description = excluded.description,
			status = excluded.status,
			issue_type = excluded.issue_type,
			priority = excluded.priority,
			assignee = excluded.assignee,
			labels = excluded.labels,
			parent_key = excluded.parent_key,
			synced_at = excluded.synced_at`,
		issue.ID, s.instanceName, projectID, epicID, issue.Key,
		issue.Fields.Summary, issue.Fields.Description,
		issue.Fields.Status.Name, issue.Fields.IssueType.Name,
		issue.Fields.Priority.Name, assignee, string(labels), parentKey)
}

// SyncOpts configures a sync operation.
type SyncOpts struct {
	ProjectKey      string
	ExcludeSubTasks bool
	MaxResults      int // 0 = use default (100)
}

// SyncProjectWithOpts syncs a project using paginated search with optional sub-task filtering.
func (s *SyncEngine) SyncProjectWithOpts(ctx context.Context, opts SyncOpts) (*SyncResult, error) {
	result := &SyncResult{
		Instance: s.instanceName,
		SyncedAt: time.Now(),
	}

	// Fetch project.
	proj, err := s.client.GetProject(ctx, opts.ProjectKey)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Upsert project.
	lead := ""
	if proj.Lead != nil {
		lead = proj.Lead.DisplayName
	}
	err = s.db.Exec(ctx, `
		INSERT INTO jira_projects (id, instance_name, key, name, description, lead, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(instance_name, key) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			lead = excluded.lead,
			synced_at = excluded.synced_at`,
		proj.ID, s.instanceName, proj.Key, proj.Name, proj.Description, lead)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("upsert project: %w", err)
	}
	result.ProjectsSync = 1

	// Paginated search.
	jql := fmt.Sprintf("project=%s ORDER BY updated DESC", opts.ProjectKey)
	searchResult, err := s.client.SearchIssuesPaginated(ctx, jql, opts.ExcludeSubTasks)
	if err != nil && len(searchResult.Issues) == 0 {
		result.Error = err.Error()
		return result, fmt.Errorf("search issues: %w", err)
	}

	// First pass: upsert epics.
	for _, apiIssue := range searchResult.Issues {
		if apiIssue.Fields.IssueType.Name != "Epic" {
			continue
		}
		if uErr := s.upsertEpic(ctx, apiIssue, proj.ID); uErr != nil {
			result.Error = uErr.Error()
			return result, fmt.Errorf("upsert epic %s: %w", apiIssue.Key, uErr)
		}
		result.EpicsSync++
	}

	// Second pass: upsert all issues.
	for _, apiIssue := range searchResult.Issues {
		if uErr := s.upsertIssue(ctx, apiIssue, proj.ID); uErr != nil {
			result.Error = uErr.Error()
			return result, fmt.Errorf("upsert issue %s: %w", apiIssue.Key, uErr)
		}
		result.IssuesSync++
	}

	// Update sync state.
	err = s.db.Exec(ctx, `
		INSERT INTO jira_sync_state (instance_name, last_sync_at, last_sync_status, issues_synced)
		VALUES (?, datetime('now'), 'success', ?)
		ON CONFLICT(instance_name) DO UPDATE SET
			last_sync_at = excluded.last_sync_at,
			last_sync_status = excluded.last_sync_status,
			issues_synced = excluded.issues_synced`,
		s.instanceName, result.IssuesSync)
	if err != nil {
		return result, fmt.Errorf("update sync state: %w", err)
	}

	return result, nil
}

// GetCachedIssuesFiltered retrieves issues from the local cache with optional sub-task filtering.
func (s *SyncEngine) GetCachedIssuesFiltered(ctx context.Context, projectKey, status string, excludeSubTasks bool) ([]JiraIssue, error) {
	query := `SELECT id, instance_name, project_id, COALESCE(epic_id,''), key, summary,
		COALESCE(description,''), status, issue_type, COALESCE(priority,''),
		COALESCE(assignee,''), COALESCE(labels,'[]'),
		COALESCE(acceptance_criteria,''),
		COALESCE(parent_key,''),
		COALESCE(agent_type,''), COALESCE(agent_state,''),
		COALESCE(session_id,''), COALESCE(prompt_hash,'')
		FROM jira_issues WHERE instance_name = ?`
	args := []any{s.instanceName}

	if projectKey != "" {
		query += ` AND project_id IN (SELECT id FROM jira_projects WHERE key = ? AND instance_name = ?)`
		args = append(args, projectKey, s.instanceName)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	if excludeSubTasks {
		query += ` AND issue_type NOT IN ('Sub-task', 'Subtask')`
	}

	query += ` ORDER BY key`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cached issues: %w", err)
	}
	defer rows.Close()

	var issues []JiraIssue
	for rows.Next() {
		var i JiraIssue
		var labelsStr string
		if err := rows.Scan(
			&i.ID, &i.InstanceName, &i.ProjectID, &i.EpicID, &i.Key,
			&i.Summary, &i.Description, &i.Status, &i.IssueType,
			&i.Priority, &i.Assignee, &labelsStr,
			&i.AcceptanceCriteria, &i.ParentKey,
			&i.AgentType, &i.AgentState,
			&i.SessionID, &i.PromptHash,
		); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		_ = json.Unmarshal([]byte(labelsStr), &i.Labels)
		if i.Labels == nil {
			i.Labels = []string{}
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// GetCachedIssues retrieves issues from the local cache, optionally filtered.
func (s *SyncEngine) GetCachedIssues(ctx context.Context, projectKey, status string) ([]JiraIssue, error) {
	query := `SELECT id, instance_name, project_id, COALESCE(epic_id,''), key, summary,
		COALESCE(description,''), status, issue_type, COALESCE(priority,''),
		COALESCE(assignee,''), COALESCE(labels,'[]'),
		COALESCE(acceptance_criteria,''),
		COALESCE(parent_key,''),
		COALESCE(agent_type,''), COALESCE(agent_state,''),
		COALESCE(session_id,''), COALESCE(prompt_hash,'')
		FROM jira_issues WHERE instance_name = ?`
	args := []any{s.instanceName}

	if projectKey != "" {
		query += ` AND project_id IN (SELECT id FROM jira_projects WHERE key = ? AND instance_name = ?)`
		args = append(args, projectKey, s.instanceName)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}

	query += ` ORDER BY key`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cached issues: %w", err)
	}
	defer rows.Close()

	var issues []JiraIssue
	for rows.Next() {
		var i JiraIssue
		var labelsStr string
		if err := rows.Scan(
			&i.ID, &i.InstanceName, &i.ProjectID, &i.EpicID, &i.Key,
			&i.Summary, &i.Description, &i.Status, &i.IssueType,
			&i.Priority, &i.Assignee, &labelsStr,
			&i.AcceptanceCriteria, &i.ParentKey,
			&i.AgentType, &i.AgentState,
			&i.SessionID, &i.PromptHash,
		); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		_ = json.Unmarshal([]byte(labelsStr), &i.Labels)
		if i.Labels == nil {
			i.Labels = []string{}
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// GetCachedIssue retrieves a single issue from the local cache by key.
func (s *SyncEngine) GetCachedIssue(ctx context.Context, issueKey string) (*JiraIssue, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, instance_name, project_id, COALESCE(epic_id,''), key, summary,
			COALESCE(description,''), status, issue_type, COALESCE(priority,''),
			COALESCE(assignee,''), COALESCE(labels,'[]'),
			COALESCE(acceptance_criteria,''),
			COALESCE(parent_key,''),
			COALESCE(agent_type,''), COALESCE(agent_state,''),
			COALESCE(session_id,''), COALESCE(prompt_hash,'')
		FROM jira_issues WHERE key = ? AND instance_name = ?`,
		issueKey, s.instanceName)

	var i JiraIssue
	var labelsStr string
	if err := row.Scan(
		&i.ID, &i.InstanceName, &i.ProjectID, &i.EpicID, &i.Key,
		&i.Summary, &i.Description, &i.Status, &i.IssueType,
		&i.Priority, &i.Assignee, &labelsStr,
		&i.AcceptanceCriteria, &i.ParentKey,
		&i.AgentType, &i.AgentState,
		&i.SessionID, &i.PromptHash,
	); err != nil {
		return nil, fmt.Errorf("issue %s not found: %w", issueKey, err)
	}
	_ = json.Unmarshal([]byte(labelsStr), &i.Labels)
	if i.Labels == nil {
		i.Labels = []string{}
	}
	return &i, nil
}

// GetSubTasks retrieves all sub-tasks whose parent_key matches the given issue key.
func (s *SyncEngine) GetSubTasks(ctx context.Context, parentKey string) ([]JiraIssue, error) {
	query := `SELECT id, instance_name, project_id, COALESCE(epic_id,''), key, summary,
		COALESCE(description,''), status, issue_type, COALESCE(priority,''),
		COALESCE(assignee,''), COALESCE(labels,'[]'),
		COALESCE(acceptance_criteria,''),
		COALESCE(parent_key,''),
		COALESCE(agent_type,''), COALESCE(agent_state,''),
		COALESCE(session_id,''), COALESCE(prompt_hash,'')
		FROM jira_issues WHERE parent_key = ? AND instance_name = ?
		ORDER BY key`

	rows, err := s.db.Query(ctx, query, parentKey, s.instanceName)
	if err != nil {
		return nil, fmt.Errorf("query sub-tasks for %s: %w", parentKey, err)
	}
	defer rows.Close()

	var issues []JiraIssue
	for rows.Next() {
		var i JiraIssue
		var labelsStr string
		if err := rows.Scan(
			&i.ID, &i.InstanceName, &i.ProjectID, &i.EpicID, &i.Key,
			&i.Summary, &i.Description, &i.Status, &i.IssueType,
			&i.Priority, &i.Assignee, &labelsStr,
			&i.AcceptanceCriteria, &i.ParentKey,
			&i.AgentType, &i.AgentState,
			&i.SessionID, &i.PromptHash,
		); err != nil {
			return nil, fmt.Errorf("scan sub-task: %w", err)
		}
		_ = json.Unmarshal([]byte(labelsStr), &i.Labels)
		if i.Labels == nil {
			i.Labels = []string{}
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}
