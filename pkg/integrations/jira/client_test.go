package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/project/ENG" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(APIProject{
			ID:   "10001",
			Key:  "ENG",
			Name: "Engineering",
		})
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "test-token",
		HTTPClient: srv.Client(),
	})

	proj, err := c.GetProject(context.Background(), "ENG")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if proj.Key != "ENG" {
		t.Errorf("expected key ENG, got %s", proj.Key)
	}
	if proj.Name != "Engineering" {
		t.Errorf("expected name Engineering, got %s", proj.Name)
	}
}

func TestBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
		}
		if user != "admin" || pass != "secret" {
			t.Errorf("unexpected credentials: %s:%s", user, pass)
		}
		json.NewEncoder(w).Encode(APIProject{ID: "1", Key: "TEST", Name: "Test"})
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "basic",
		Username:   "admin",
		Password:   "secret",
		HTTPClient: srv.Client(),
	})

	_, err := c.GetProject(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("GetProject with basic auth: %v", err)
	}
}

func TestSearchIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(SearchResult{
			Total: 1,
			Issues: []APIIssue{
				{
					ID:  "10100",
					Key: "ENG-142",
					Fields: APIIssueFields{
						Summary: "Add OAuth2 token refresh",
						Status:  APIStatus{Name: "In Progress"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "tok",
		HTTPClient: srv.Client(),
	})

	result, err := c.SearchIssues(context.Background(), "project=ENG", 50, 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 total, got %d", result.Total)
	}
	if result.Issues[0].Key != "ENG-142" {
		t.Errorf("expected ENG-142, got %s", result.Issues[0].Key)
	}
}

func TestAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Invalid credentials"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "bad-token",
		HTTPClient: srv.Client(),
	})

	_, err := c.GetProject(context.Background(), "ENG")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if got := err.Error(); !contains(got, "authentication failed") {
		t.Errorf("expected auth error, got: %s", got)
	}
}

func TestRateLimitResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "3")
		json.NewEncoder(w).Encode(APIProject{ID: "1", Key: "TEST", Name: "Test"})
	}))
	defer srv.Close()

	limiter := NewRateLimiter(10, 20)
	c := NewClient(ClientOpts{
		BaseURL:     srv.URL,
		AuthType:    "pat",
		Token:       "tok",
		HTTPClient:  srv.Client(),
		RateLimiter: limiter,
	})

	_, err := c.GetProject(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Limiter should be reduced since remaining < 5.
	if !limiter.IsReduced() {
		t.Error("expected limiter to be in reduced mode")
	}
}

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/ENG-142" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(APIIssue{
			ID:  "10100",
			Key: "ENG-142",
			Fields: APIIssueFields{
				Summary:     "Add OAuth2 refresh",
				Description: "Implement token refresh flow",
				Status:      APIStatus{Name: "To Do"},
				IssueType:   APIIssueType{Name: "Story"},
				Priority:    APIPriority{Name: "High"},
				Labels:      []string{"auth", "security"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "tok",
		HTTPClient: srv.Client(),
	})

	issue, err := c.GetIssue(context.Background(), "ENG-142")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Fields.Summary != "Add OAuth2 refresh" {
		t.Errorf("expected summary 'Add OAuth2 refresh', got %s", issue.Fields.Summary)
	}
	if len(issue.Fields.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(issue.Fields.Labels))
	}
}

func TestTransitionIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/ENG-142/transitions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(ClientOpts{
		BaseURL:    srv.URL,
		AuthType:   "pat",
		Token:      "tok",
		HTTPClient: srv.Client(),
	})

	err := c.TransitionIssue(context.Background(), "ENG-142", "31")
	if err != nil {
		t.Fatalf("TransitionIssue: %v", err)
	}
}

func TestNetworkTimeout(t *testing.T) {
	// Use a cancelled context to simulate timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewClient(ClientOpts{
		BaseURL:  "http://localhost:1", // unreachable
		AuthType: "pat",
		Token:    "tok",
	})

	_, err := c.GetProject(ctx, "ENG")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
