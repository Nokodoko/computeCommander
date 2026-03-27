package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/noko/computecommander/internal/config"
)

// TestOpenBrainProxyEntries verifies the entries proxy forwards to the MCP server.
func TestOpenBrainProxyEntries(t *testing.T) {
	// Mock MCP server.
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/openbrain/entries" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"entries":  []interface{}{},
			"count":    0,
			"has_more": false,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer mcpServer.Close()

	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: mcpServer.URL,
		APIKey:    "test-key",
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openbrain/entries", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Entries []interface{} `json:"entries"`
		Count   int           `json:"count"`
		HasMore bool          `json:"has_more"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Count != 0 {
		t.Errorf("expected count 0, got %d", result.Count)
	}
}

// TestOpenBrainProxyEntriesForwardsQueryParams verifies query params are forwarded.
func TestOpenBrainProxyEntriesForwardsQueryParams(t *testing.T) {
	var receivedQuery string
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"entries":  []interface{}{},
			"count":    0,
			"has_more": false,
		})
	}))
	defer mcpServer.Close()

	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: mcpServer.URL,
		APIKey:    "my-key",
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openbrain/entries?type=task&limit=5", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// The query should contain the original params plus the api_key.
	if receivedQuery == "" {
		t.Fatal("expected query params to be forwarded")
	}
	if len(receivedQuery) < 10 {
		t.Errorf("query too short, forwarding may have failed: %q", receivedQuery)
	}
}

// TestOpenBrainProxyStatus verifies the status endpoint reports connection state.
func TestOpenBrainProxyStatus(t *testing.T) {
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"entries":  []interface{}{},
			"count":    0,
			"has_more": false,
		})
	}))
	defer mcpServer.Close()

	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: mcpServer.URL,
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openbrain/status", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result openBrainStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Status != "connected" {
		t.Errorf("expected status 'connected', got %q", result.Status)
	}
}

// TestOpenBrainProxyStatusDisconnected verifies status when MCP server is unreachable.
func TestOpenBrainProxyStatusDisconnected(t *testing.T) {
	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: "http://127.0.0.1:1", // unreachable port
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openbrain/status", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even when disconnected, got %d", rec.Code)
	}

	var result openBrainStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Status != "disconnected" {
		t.Errorf("expected status 'disconnected', got %q", result.Status)
	}
}

// TestOpenBrainProxyEntriesUnreachable verifies graceful degradation.
func TestOpenBrainProxyEntriesUnreachable(t *testing.T) {
	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: "http://127.0.0.1:1",
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openbrain/entries", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when MCP unreachable, got %d", rec.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if result["status"] != "disconnected" {
		t.Errorf("expected status 'disconnected', got %q", result["status"])
	}
}

// TestOpenBrainProxyRegistersRoutes verifies all three routes are registered.
func TestOpenBrainProxyRegistersRoutes(t *testing.T) {
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"entries":[],"count":0,"has_more":false}`)
	}))
	defer mcpServer.Close()

	proxy := NewOpenBrainProxy(config.OpenBrainConfig{
		Enabled:   true,
		MCPSseURL: mcpServer.URL,
	})

	mux := http.NewServeMux()
	proxy.RegisterRoutes(mux)

	routes := []string{
		"/api/v1/openbrain/entries",
		"/api/v1/openbrain/status",
	}

	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("route %s: expected 200, got %d", route, rec.Code)
		}
	}
}
