// Package jira provides a REST API client for Jira Cloud/Server integration.
package jira

import "time"

// JiraProject represents a cached Jira project.
type JiraProject struct {
	ID           string    `json:"id"       db:"id"`
	InstanceName string    `json:"instance" db:"instance_name"`
	Key          string    `json:"key"      db:"key"`
	Name         string    `json:"name"     db:"name"`
	Description  string    `json:"description" db:"description"`
	Lead         string    `json:"lead"     db:"lead"`
	SyncedAt     time.Time `json:"syncedAt" db:"synced_at"`
}

// JiraEpic represents a cached Jira epic.
type JiraEpic struct {
	ID           string    `json:"id"       db:"id"`
	InstanceName string    `json:"instance" db:"instance_name"`
	ProjectID    string    `json:"projectId" db:"project_id"`
	Key          string    `json:"key"      db:"key"`
	Summary      string    `json:"summary"  db:"summary"`
	Status       string    `json:"status"   db:"status"`
	SyncedAt     time.Time `json:"syncedAt" db:"synced_at"`
}

// JiraIssue represents a cached Jira issue with agent tracking.
type JiraIssue struct {
	// Identity
	ID           string `json:"id"       db:"id"`
	InstanceName string `json:"instance" db:"instance_name"`
	ProjectID    string `json:"projectId" db:"project_id"`
	EpicID       string `json:"epicId"   db:"epic_id"`
	Key          string `json:"key"      db:"key"`

	// Content
	Summary            string   `json:"summary"  db:"summary"`
	Description        string   `json:"description" db:"description"`
	Status             string   `json:"status"   db:"status"`
	IssueType          string   `json:"issueType" db:"issue_type"`
	Priority           string   `json:"priority" db:"priority"`
	Assignee           string   `json:"assignee" db:"assignee"`
	Labels             []string `json:"labels"`
	AcceptanceCriteria string   `json:"acceptanceCriteria" db:"acceptance_criteria"`

	// Agent tracking
	AgentType  string `json:"agentType"  db:"agent_type"`
	AgentState string `json:"agentState" db:"agent_state"`
	SessionID  string `json:"sessionId"  db:"session_id"`
	PromptHash string `json:"promptHash" db:"prompt_hash"`

	// Sync
	SyncedAt time.Time `json:"syncedAt" db:"synced_at"`
}

// SearchResult holds the response from a Jira issue search.
type SearchResult struct {
	StartAt    int              `json:"startAt"`
	MaxResults int              `json:"maxResults"`
	Total      int              `json:"total"`
	Issues     []APIIssue       `json:"issues"`
}

// APIIssue is the raw Jira REST API issue representation.
type APIIssue struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Self   string         `json:"self"`
	Fields APIIssueFields `json:"fields"`
}

// APIIssueFields holds the standard Jira issue fields.
type APIIssueFields struct {
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	Status      APIStatus     `json:"status"`
	IssueType   APIIssueType  `json:"issuetype"`
	Priority    APIPriority   `json:"priority"`
	Assignee    *APIUser      `json:"assignee"`
	Labels      []string      `json:"labels"`
	Project     APIProject    `json:"project"`
	Epic        *APIEpicLink  `json:"epic"`
	Parent      *APIParent    `json:"parent"`
}

// APIStatus represents a Jira status.
type APIStatus struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// APIIssueType represents a Jira issue type.
type APIIssueType struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// APIPriority represents a Jira priority.
type APIPriority struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// APIUser represents a Jira user.
type APIUser struct {
	DisplayName string `json:"displayName"`
	AccountID   string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
}

// APIProject represents a Jira project in an API response.
type APIProject struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Lead        *APIUser `json:"lead"`
}

// APIEpicLink represents the epic field on a Jira issue.
type APIEpicLink struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

// APIParent represents the parent issue (used for epic link in next-gen projects).
type APIParent struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Fields APIParentFields `json:"fields"`
}

// APIParentFields holds limited fields from a parent issue reference.
type APIParentFields struct {
	Summary   string       `json:"summary"`
	Status    APIStatus    `json:"status"`
	IssueType APIIssueType `json:"issuetype"`
}

// APITransition represents a Jira issue transition.
type APITransition struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	To   APIStatus `json:"to"`
}

// TransitionsResponse holds the response from the transitions endpoint.
type TransitionsResponse struct {
	Transitions []APITransition `json:"transitions"`
}

// CommentBody represents a Jira comment payload.
type CommentBody struct {
	Body string `json:"body"`
}

// ProjectListResponse holds the response from the projects endpoint.
type ProjectListResponse struct {
	Values     []APIProject `json:"values"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	IsLast     bool         `json:"isLast"`
}
