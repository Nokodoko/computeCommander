package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/internal/platform/db"
)

// --- Mock implementations ---

// mockDB satisfies db.DB for testing without a real database.
type mockDB struct{}

func (m *mockDB) Exec(_ context.Context, _ string, _ ...any) error          { return nil }
func (m *mockDB) Query(_ context.Context, _ string, _ ...any) (*db.Rows, error) { return nil, nil }
func (m *mockDB) QueryRow(_ context.Context, _ string, _ ...any) *db.Row    { return nil }
func (m *mockDB) Close() error                                              { return nil }
func (m *mockDB) Begin(_ context.Context) (db.Tx, error)                    { return nil, nil }
func (m *mockDB) Driver() string                                            { return "mock" }

// mockMailStore satisfies mail.MailStore for testing.
type mockMailStore struct {
	messages []*mail.MailMessage
}

func (m *mockMailStore) Send(msg *mail.MailMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockMailStore) Check(_ string, _ mail.CheckOpts) ([]*mail.MailMessage, error) {
	return m.messages, nil
}

func (m *mockMailStore) List(_ mail.ListOpts) ([]*mail.MailMessage, error) {
	return m.messages, nil
}

func (m *mockMailStore) MarkRead(_ string) error   { return nil }
func (m *mockMailStore) Reply(_ string, _ string) error { return nil }
func (m *mockMailStore) Purge(_ mail.PurgeOpts) (int, error) { return 0, nil }

// mockMergeQueue satisfies merge.MergeQueue for testing.
type mockMergeQueue struct {
	entries []*merge.MergeEntry
}

func (m *mockMergeQueue) Enqueue(entry *merge.MergeEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockMergeQueue) Dequeue() (*merge.MergeEntry, error) {
	if len(m.entries) == 0 {
		return nil, merge.ErrQueueEmpty
	}
	entry := m.entries[0]
	m.entries = m.entries[1:]
	return entry, nil
}

func (m *mockMergeQueue) Peek() (*merge.MergeEntry, error) {
	if len(m.entries) == 0 {
		return nil, merge.ErrQueueEmpty
	}
	return m.entries[0], nil
}

func (m *mockMergeQueue) Status(branch string) (*merge.MergeEntry, error) {
	for _, e := range m.entries {
		if e.BranchName == branch {
			return e, nil
		}
	}
	return nil, merge.ErrNotFound
}

func (m *mockMergeQueue) List(_ merge.ListOpts) ([]*merge.MergeEntry, error) {
	return m.entries, nil
}

// newTestGateway creates a Gateway with mock dependencies for testing.
func newTestGateway() *Gateway {
	mdb := &mockDB{}
	// We cannot create a real Spawner without full infra mocks, so we test
	// the routes that do not require Spawner and verify compilation of all handlers.
	return NewGateway(GatewayOpts{
		DB:      mdb,
		Mail:    &mockMailStore{},
		Queue:   &mockMergeQueue{},
		Version: "test-1.0.0",
		StartAt: time.Now(),
	})
}

func TestHealthEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}
	if body["version"] != "test-1.0.0" {
		t.Errorf("expected version 'test-1.0.0', got %v", body["version"])
	}
}

func TestCORSHeaders(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for OPTIONS, got %d", rec.Code)
	}

	origin := rec.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("expected CORS origin '*', got %q", origin)
	}

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods header to be set")
	}
}

func TestRequestIDHeader(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	reqID := rec.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header to be set")
	}
}

func TestListMailEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["count"] != float64(0) {
		t.Errorf("expected count 0, got %v", body["count"])
	}
}

func TestSendMailEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	payload := `{"from":"agent-a","to":"agent-b","subject":"test","body":"hello","type":"status","priority":"normal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["status"] != "sent" {
		t.Errorf("expected status 'sent', got %v", body["status"])
	}
}

func TestMergeQueueEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merge/queue", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["count"] != float64(0) {
		t.Errorf("expected count 0, got %v", body["count"])
	}
}

func TestCostsEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/costs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["currency"] != "USD" {
		t.Errorf("expected currency 'USD', got %v", body["currency"])
	}
}

func TestExtractPathParam(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   string
	}{
		{"/api/v1/agents/scout-1", "/api/v1/agents/", "scout-1"},
		{"/api/v1/agents/", "/api/v1/agents/", ""},
		{"/api/v1/agents/builder-2/extra", "/api/v1/agents/", "builder-2"},
		{"/other/path", "/api/v1/agents/", ""},
	}

	for _, tt := range tests {
		got := extractPathParam(tt.path, tt.prefix)
		if got != tt.want {
			t.Errorf("extractPathParam(%q, %q) = %q, want %q", tt.path, tt.prefix, got, tt.want)
		}
	}
}

func TestSendMailBadJSON(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestGetAgentMissingName(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
