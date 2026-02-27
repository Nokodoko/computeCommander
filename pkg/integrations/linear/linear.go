// Package linear provides a stub Linear integration client for ComputeCommander.
// Linear is a project management tool; this integration syncs issues and
// updates status between ComputeCommander agents and Linear projects.
package linear

import (
	"context"
	"fmt"
	"time"
)

// ClientOpts configures a LinearClient.
type ClientOpts struct {
	// APIKey is the Linear API key for authentication.
	APIKey string

	// TeamID is the Linear team ID to scope operations to.
	TeamID string

	// BaseURL overrides the Linear API endpoint.
	// Defaults to "https://api.linear.app" if empty.
	BaseURL string
}

// LinearClient interacts with the Linear API.
type LinearClient struct {
	apiKey  string
	teamID  string
	baseURL string
}

// NewLinearClient creates a new Linear integration client.
func NewLinearClient(opts ClientOpts) *LinearClient {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.linear.app"
	}
	return &LinearClient{
		apiKey:  opts.APIKey,
		teamID:  opts.TeamID,
		baseURL: baseURL,
	}
}

// Issue represents a Linear issue.
type Issue struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Assignee    string    `json:"assignee"`
	Labels      []string  `json:"labels"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// SyncOpts configures issue syncing behavior.
type SyncOpts struct {
	// Since filters to issues updated after this time. Zero means all.
	Since time.Time

	// Statuses filters to issues in these statuses. Empty means all.
	Statuses []string

	// Limit caps the number of issues returned. 0 means no limit.
	Limit int
}

// StatusUpdate represents a status change to apply to a Linear issue.
type StatusUpdate struct {
	// IssueID is the Linear issue identifier.
	IssueID string `json:"issueId"`

	// Status is the new status to set (e.g., "In Progress", "Done").
	Status string `json:"status"`

	// Comment is an optional comment to add with the status change.
	Comment string `json:"comment,omitempty"`
}

// SyncIssues retrieves issues from Linear matching the given options.
// This is a stub implementation that will be replaced with actual GraphQL API calls.
func (c *LinearClient) SyncIssues(ctx context.Context, opts SyncOpts) ([]*Issue, error) {
	if c.teamID == "" {
		return nil, fmt.Errorf("linear: team ID is required for syncing issues")
	}

	// Stub: actual implementation would query Linear's GraphQL API.
	_ = ctx
	return []*Issue{}, nil
}

// UpdateStatus changes the status of a Linear issue.
// This is a stub implementation that will be replaced with actual GraphQL API calls.
func (c *LinearClient) UpdateStatus(ctx context.Context, update StatusUpdate) error {
	if update.IssueID == "" {
		return fmt.Errorf("linear: issue ID is required")
	}
	if update.Status == "" {
		return fmt.Errorf("linear: status is required")
	}

	// Stub: actual implementation would mutate via Linear's GraphQL API.
	_ = ctx
	return nil
}

// TeamID returns the configured team identifier.
func (c *LinearClient) TeamID() string { return c.teamID }
