package darkfactory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/pkg/integrations/jira"
)

func setupDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewDB(db.DatabaseConfig{Driver: "sqlite"})
	if err != nil {
		t.Fatalf("create DB: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestExecutorSteppedMode(t *testing.T) {
	database := setupDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/TEST":
			json.NewEncoder(w).Encode(jira.APIProject{ID: "1", Key: "TEST", Name: "Test"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(jira.SearchResult{
				Total: 1,
				Issues: []jira.APIIssue{
					{
						ID:  "100",
						Key: "TEST-1",
						Fields: jira.APIIssueFields{
							Summary:   "Test task",
							Status:    jira.APIStatus{Name: "To Do"},
							IssueType: jira.APIIssueType{Name: "Story"},
							Priority:  jira.APIPriority{Name: "High"},
							Project:   jira.APIProject{ID: "1", Key: "TEST"},
						},
					},
				},
			})
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "tok",
		HTTPClient: srv.Client(),
	})

	syncEngine := jira.NewSyncEngine(client, database, "test")
	promptGen := jira.NewPromptGenerator("")

	cfg := &config.DarkFactoryConfig{
		Enabled:            true,
		ExecutionMode:      "stepped",
		MaxConcurrentTasks: 2,
		UATTimeout:         "5m",
	}

	executor := NewExecutor(ExecutorOpts{
		DB:         database,
		Config:     cfg,
		SyncEngine: syncEngine,
		PromptGen:  promptGen,
	})

	err := executor.Run(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	status := executor.Status("TEST")
	if status.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", status.Completed)
	}
	if status.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", status.Failed)
	}
}

func TestExecutorEmptyProject(t *testing.T) {
	database := setupDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/EMPTY":
			json.NewEncoder(w).Encode(jira.APIProject{ID: "2", Key: "EMPTY", Name: "Empty"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(jira.SearchResult{Total: 0, Issues: []jira.APIIssue{}})
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	syncEngine := jira.NewSyncEngine(client, database, "test")
	promptGen := jira.NewPromptGenerator("")

	executor := NewExecutor(ExecutorOpts{
		DB:         database,
		Config:     &config.DarkFactoryConfig{ExecutionMode: "stepped", MaxConcurrentTasks: 1},
		SyncEngine: syncEngine,
		PromptGen:  promptGen,
	})

	err := executor.Run(context.Background(), "EMPTY")
	if err != nil {
		t.Fatalf("Run on empty project: %v", err)
	}

	status := executor.Status("EMPTY")
	if status.Completed != 0 {
		t.Errorf("expected 0 completed for empty project, got %d", status.Completed)
	}
}

func TestExecutorWithVerifier(t *testing.T) {
	database := setupDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/api/3/project/TEST":
			json.NewEncoder(w).Encode(jira.APIProject{ID: "1", Key: "TEST", Name: "Test"})
		case r.URL.Path == "/rest/api/3/search/jql":
			json.NewEncoder(w).Encode(jira.SearchResult{
				Total: 1,
				Issues: []jira.APIIssue{
					{
						ID:  "100",
						Key: "TEST-1",
						Fields: jira.APIIssueFields{
							Summary:   "Test with outcomes",
							Status:    jira.APIStatus{Name: "To Do"},
							IssueType: jira.APIIssueType{Name: "Story"},
							Priority:  jira.APIPriority{Name: "High"},
							Project:   jira.APIProject{ID: "1", Key: "TEST"},
						},
					},
				},
			})
		}
	}))
	defer srv.Close()

	client := jira.NewClient(jira.ClientOpts{BaseURL: srv.URL, AuthType: "pat", Token: "tok", HTTPClient: srv.Client()})
	syncEngine := jira.NewSyncEngine(client, database, "test")
	promptGen := jira.NewPromptGenerator("")

	// Verifier with no objectives dir (will pass with default score).
	verifier := NewIntentVerifier(t.TempDir())

	executor := NewExecutor(ExecutorOpts{
		DB:         database,
		Config:     &config.DarkFactoryConfig{ExecutionMode: "stepped", MaxConcurrentTasks: 1},
		SyncEngine: syncEngine,
		PromptGen:  promptGen,
		Verifier:   verifier,
	})

	err := executor.Run(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("Run with verifier: %v", err)
	}
}

func TestExecutorStatus(t *testing.T) {
	executor := &Executor{
		config:    &config.DarkFactoryConfig{ExecutionMode: "stepped"},
		active:    make(map[string]*TaskState),
		completed: 5,
		failed:    1,
	}

	status := executor.Status("TEST")
	if status.Completed != 5 {
		t.Errorf("expected 5 completed, got %d", status.Completed)
	}
	if status.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", status.Failed)
	}
	if status.Mode != "stepped" {
		t.Errorf("expected mode 'stepped', got %s", status.Mode)
	}
}
