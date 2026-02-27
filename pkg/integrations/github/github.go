// Package github provides a stub GitHub integration client for ComputeCommander.
// This package defines the interface for interacting with GitHub's API for
// issue comments, PR status checks, and workflow triggers.
package github

import (
	"context"
	"fmt"
)

// ClientOpts configures a GitHubClient.
type ClientOpts struct {
	// Token is the GitHub personal access token or app token.
	Token string

	// BaseURL overrides the GitHub API URL (for GitHub Enterprise).
	// Defaults to "https://api.github.com" if empty.
	BaseURL string

	// Owner is the repository owner (user or organization).
	Owner string

	// Repo is the repository name.
	Repo string
}

// GitHubClient interacts with the GitHub API.
type GitHubClient struct {
	token   string
	baseURL string
	owner   string
	repo    string
}

// NewGitHubClient creates a new GitHub integration client.
func NewGitHubClient(opts ClientOpts) *GitHubClient {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &GitHubClient{
		token:   opts.Token,
		baseURL: baseURL,
		owner:   opts.Owner,
		repo:    opts.Repo,
	}
}

// IssueComment represents a comment to post on an issue or PR.
type IssueComment struct {
	IssueNumber int    `json:"issueNumber"`
	Body        string `json:"body"`
}

// PRStatus holds the status information for a pull request.
type PRStatus struct {
	Number    int    `json:"number"`
	State     string `json:"state"`
	Mergeable bool   `json:"mergeable"`
	Title     string `json:"title"`
	HeadRef   string `json:"headRef"`
	BaseRef   string `json:"baseRef"`
}

// WorkflowTrigger represents a request to trigger a GitHub Actions workflow.
type WorkflowTrigger struct {
	WorkflowID string            `json:"workflowId"`
	Ref        string            `json:"ref"`
	Inputs     map[string]string `json:"inputs"`
}

// CreateIssueComment posts a comment on a GitHub issue or pull request.
// This is a stub implementation that will be replaced with actual API calls.
func (c *GitHubClient) CreateIssueComment(ctx context.Context, comment IssueComment) error {
	if comment.IssueNumber <= 0 {
		return fmt.Errorf("github: issue number must be positive, got %d", comment.IssueNumber)
	}
	if comment.Body == "" {
		return fmt.Errorf("github: comment body is required")
	}

	// Stub: actual implementation would POST to /repos/{owner}/{repo}/issues/{number}/comments
	_ = ctx
	return nil
}

// GetPRStatus retrieves the status of a pull request.
// This is a stub implementation that will be replaced with actual API calls.
func (c *GitHubClient) GetPRStatus(ctx context.Context, prNumber int) (*PRStatus, error) {
	if prNumber <= 0 {
		return nil, fmt.Errorf("github: PR number must be positive, got %d", prNumber)
	}

	// Stub: actual implementation would GET /repos/{owner}/{repo}/pulls/{number}
	_ = ctx
	return &PRStatus{
		Number:    prNumber,
		State:     "open",
		Mergeable: true,
		Title:     "stub PR",
		HeadRef:   "feature",
		BaseRef:   "main",
	}, nil
}

// TriggerWorkflow dispatches a GitHub Actions workflow run.
// This is a stub implementation that will be replaced with actual API calls.
func (c *GitHubClient) TriggerWorkflow(ctx context.Context, trigger WorkflowTrigger) error {
	if trigger.WorkflowID == "" {
		return fmt.Errorf("github: workflow ID is required")
	}
	if trigger.Ref == "" {
		return fmt.Errorf("github: ref is required")
	}

	// Stub: actual implementation would POST to /repos/{owner}/{repo}/actions/workflows/{id}/dispatches
	_ = ctx
	return nil
}

// Owner returns the configured repository owner.
func (c *GitHubClient) Owner() string { return c.owner }

// Repo returns the configured repository name.
func (c *GitHubClient) Repo() string { return c.repo }
