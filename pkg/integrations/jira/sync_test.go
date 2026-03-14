package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupTestDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewDB(db.DatabaseConfig{Driver: "sqlite"})
	if err != nil {
		t.Fatalf("create test DB: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate test DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSyncProject(t *testing.T) {
	database := setupTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/ENG":
			json.NewEncoder(w).Encode(APIProject{
				ID:   "10001",
				Key:  "ENG",
				Name: "Engineering",
				Lead: &APIUser{DisplayName: "Alice"},
			})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(SearchResult{
				Total: 2,
				Issues: []APIIssue{
					{
						ID:  "10100",
						Key: "ENG-1",
						Fields: APIIssueFields{
							Summary:   "First task",
							Status:    APIStatus{Name: "To Do"},
							IssueType: APIIssueType{Name: "Story"},
							Priority:  APIPriority{Name: "High"},
							Labels:    []string{"backend"},
							Project:   APIProject{ID: "10001", Key: "ENG"},
						},
					},
					{
						ID:  "10101",
						Key: "ENG-2",
						Fields: APIIssueFields{
							Summary:   "Second task",
							Status:    APIStatus{Name: "In Progress"},
							IssueType: APIIssueType{Name: "Task"},
							Priority:  APIPriority{Name: "Medium"},
							Labels:    []string{},
							Project:   APIProject{ID: "10001", Key: "ENG"},
						},
					},
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "tok",
		HTTPClient: srv.Client(),
	})

	engine := NewSyncEngine(client, database, "test-instance")
	result, err := engine.SyncProject(context.Background(), "ENG")
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	if result.ProjectsSync != 1 {
		t.Errorf("expected 1 project synced, got %d", result.ProjectsSync)
	}
	if result.IssuesSync != 2 {
		t.Errorf("expected 2 issues synced, got %d", result.IssuesSync)
	}
}

func TestSyncGetCachedIssues(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/ENG":
			json.NewEncoder(w).Encode(APIProject{ID: "10001", Key: "ENG", Name: "Engineering"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(SearchResult{
				Total: 1,
				Issues: []APIIssue{
					{
						ID:  "10100",
						Key: "ENG-1",
						Fields: APIIssueFields{
							Summary:   "Test task",
							Status:    APIStatus{Name: "To Do"},
							IssueType: APIIssueType{Name: "Story"},
							Priority:  APIPriority{Name: "High"},
							Labels:    []string{"auth"},
							Project:   APIProject{ID: "10001", Key: "ENG"},
						},
					},
				},
			})
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	engine := NewSyncEngine(client, database, "test-instance")

	if _, err := engine.SyncProject(ctx, "ENG"); err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	issues, err := engine.GetCachedIssues(ctx, "", "")
	if err != nil {
		t.Fatalf("GetCachedIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Key != "ENG-1" {
		t.Errorf("expected ENG-1, got %s", issues[0].Key)
	}
	if issues[0].Summary != "Test task" {
		t.Errorf("expected 'Test task', got %s", issues[0].Summary)
	}
}

func TestSyncEmptyProject(t *testing.T) {
	database := setupTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/EMPTY":
			json.NewEncoder(w).Encode(APIProject{ID: "10002", Key: "EMPTY", Name: "Empty"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: []APIIssue{}})
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	engine := NewSyncEngine(client, database, "test-instance")

	result, err := engine.SyncProject(context.Background(), "EMPTY")
	if err != nil {
		t.Fatalf("SyncProject: %v", err)
	}
	if result.IssuesSync != 0 {
		t.Errorf("expected 0 issues synced for empty project, got %d", result.IssuesSync)
	}
}

func TestSyncIncrementalUpdate(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/ENG":
			json.NewEncoder(w).Encode(APIProject{ID: "10001", Key: "ENG", Name: "Engineering"})
		case r.URL.Path == "/rest/api/3/search/jql":
			callCount++
			summary := "Original"
			if callCount > 1 {
				summary = "Updated"
			}
			json.NewEncoder(w).Encode(SearchResult{
				Total: 1,
				Issues: []APIIssue{
					{
						ID:  "10100",
						Key: "ENG-1",
						Fields: APIIssueFields{
							Summary:   summary,
							Status:    APIStatus{Name: "To Do"},
							IssueType: APIIssueType{Name: "Story"},
							Priority:  APIPriority{Name: "High"},
							Project:   APIProject{ID: "10001", Key: "ENG"},
						},
					},
				},
			})
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	engine := NewSyncEngine(client, database, "test-instance")

	// First sync.
	if _, err := engine.SyncProject(ctx, "ENG"); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync should update.
	if _, err := engine.SyncProject(ctx, "ENG"); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	issue, err := engine.GetCachedIssue(ctx, "ENG-1")
	if err != nil {
		t.Fatalf("GetCachedIssue: %v", err)
	}
	if issue.Summary != "Updated" {
		t.Errorf("expected 'Updated', got %s", issue.Summary)
	}
}

func TestSyncStateTracking(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/ENG":
			json.NewEncoder(w).Encode(APIProject{ID: "10001", Key: "ENG", Name: "Engineering"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(SearchResult{Total: 0, Issues: []APIIssue{}})
		}
	}))
	defer srv.Close()

	client := NewClient(ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	engine := NewSyncEngine(client, database, "test-instance")

	if _, err := engine.SyncProject(ctx, "ENG"); err != nil {
		t.Fatalf("SyncProject: %v", err)
	}

	// Verify sync state was recorded.
	row := database.QueryRow(ctx,
		"SELECT last_sync_status, issues_synced FROM jira_sync_state WHERE instance_name = ?",
		"test-instance")
	var status string
	var count int
	if err := row.Scan(&status, &count); err != nil {
		t.Fatalf("query sync state: %v", err)
	}
	if status != "success" {
		t.Errorf("expected status 'success', got %s", status)
	}
}
