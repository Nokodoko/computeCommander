package github

import (
	"context"
	"testing"
)

func TestNewGitHubClient(t *testing.T) {
	c := NewGitHubClient(ClientOpts{
		Token: "test-token",
		Owner: "noko",
		Repo:  "computecommander",
	})

	if c.Owner() != "noko" {
		t.Errorf("expected owner 'noko', got %q", c.Owner())
	}
	if c.Repo() != "computecommander" {
		t.Errorf("expected repo 'computecommander', got %q", c.Repo())
	}
	if c.baseURL != "https://api.github.com" {
		t.Errorf("expected default baseURL, got %q", c.baseURL)
	}
}

func TestNewGitHubClientCustomBaseURL(t *testing.T) {
	c := NewGitHubClient(ClientOpts{
		BaseURL: "https://github.example.com/api/v3",
	})

	if c.baseURL != "https://github.example.com/api/v3" {
		t.Errorf("expected custom baseURL, got %q", c.baseURL)
	}
}

func TestCreateIssueComment(t *testing.T) {
	c := NewGitHubClient(ClientOpts{Token: "t", Owner: "o", Repo: "r"})
	ctx := context.Background()

	// Valid comment succeeds (stub returns nil).
	err := c.CreateIssueComment(ctx, IssueComment{IssueNumber: 1, Body: "test"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Invalid issue number.
	err = c.CreateIssueComment(ctx, IssueComment{IssueNumber: 0, Body: "test"})
	if err == nil {
		t.Error("expected error for zero issue number")
	}

	// Empty body.
	err = c.CreateIssueComment(ctx, IssueComment{IssueNumber: 1, Body: ""})
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestGetPRStatus(t *testing.T) {
	c := NewGitHubClient(ClientOpts{Token: "t", Owner: "o", Repo: "r"})
	ctx := context.Background()

	status, err := c.GetPRStatus(ctx, 42)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if status.Number != 42 {
		t.Errorf("expected PR number 42, got %d", status.Number)
	}
	if status.State != "open" {
		t.Errorf("expected state 'open', got %q", status.State)
	}

	// Invalid PR number.
	_, err = c.GetPRStatus(ctx, -1)
	if err == nil {
		t.Error("expected error for negative PR number")
	}
}

func TestTriggerWorkflow(t *testing.T) {
	c := NewGitHubClient(ClientOpts{Token: "t", Owner: "o", Repo: "r"})
	ctx := context.Background()

	err := c.TriggerWorkflow(ctx, WorkflowTrigger{
		WorkflowID: "ci.yml",
		Ref:        "main",
		Inputs:     map[string]string{"env": "staging"},
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Missing workflow ID.
	err = c.TriggerWorkflow(ctx, WorkflowTrigger{Ref: "main"})
	if err == nil {
		t.Error("expected error for empty workflow ID")
	}

	// Missing ref.
	err = c.TriggerWorkflow(ctx, WorkflowTrigger{WorkflowID: "ci.yml"})
	if err == nil {
		t.Error("expected error for empty ref")
	}
}
