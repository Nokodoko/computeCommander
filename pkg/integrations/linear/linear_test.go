package linear

import (
	"context"
	"testing"
)

func TestNewLinearClient(t *testing.T) {
	c := NewLinearClient(ClientOpts{
		APIKey: "lin_key_test",
		TeamID: "TEAM-1",
	})

	if c.TeamID() != "TEAM-1" {
		t.Errorf("expected team ID 'TEAM-1', got %q", c.TeamID())
	}
	if c.baseURL != "https://api.linear.app" {
		t.Errorf("expected default baseURL, got %q", c.baseURL)
	}
}

func TestNewLinearClientCustomBaseURL(t *testing.T) {
	c := NewLinearClient(ClientOpts{
		BaseURL: "https://linear.example.com",
	})

	if c.baseURL != "https://linear.example.com" {
		t.Errorf("expected custom baseURL, got %q", c.baseURL)
	}
}

func TestSyncIssues(t *testing.T) {
	c := NewLinearClient(ClientOpts{APIKey: "k", TeamID: "T"})
	ctx := context.Background()

	issues, err := c.SyncIssues(ctx, SyncOpts{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 stub issues, got %d", len(issues))
	}
}

func TestSyncIssuesNoTeam(t *testing.T) {
	c := NewLinearClient(ClientOpts{APIKey: "k"})
	ctx := context.Background()

	_, err := c.SyncIssues(ctx, SyncOpts{})
	if err == nil {
		t.Error("expected error when team ID is empty")
	}
}

func TestUpdateStatus(t *testing.T) {
	c := NewLinearClient(ClientOpts{APIKey: "k", TeamID: "T"})
	ctx := context.Background()

	err := c.UpdateStatus(ctx, StatusUpdate{
		IssueID: "ISSUE-1",
		Status:  "Done",
		Comment: "completed by agent",
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestUpdateStatusValidation(t *testing.T) {
	c := NewLinearClient(ClientOpts{APIKey: "k", TeamID: "T"})
	ctx := context.Background()

	// Missing issue ID.
	err := c.UpdateStatus(ctx, StatusUpdate{Status: "Done"})
	if err == nil {
		t.Error("expected error for empty issue ID")
	}

	// Missing status.
	err = c.UpdateStatus(ctx, StatusUpdate{IssueID: "ISSUE-1"})
	if err == nil {
		t.Error("expected error for empty status")
	}
}
