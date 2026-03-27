package trustgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGraphRAG_Success(t *testing.T) {
	expected := "TrustGraph uses Apache Cassandra for storage."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flow/default/service/graph-rag" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected content-type: %s", ct)
		}

		var req GraphRAGRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Query == "" {
			t.Error("expected non-empty query")
		}

		json.NewEncoder(w).Encode(GraphRAGResponse{Response: expected})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	defer c.Close()

	resp, err := c.GraphRAG(context.Background(), GraphRAGRequest{
		Query:      "What storage does TrustGraph use?",
		Collection: "default",
	})
	if err != nil {
		t.Fatalf("GraphRAG: %v", err)
	}
	if resp.Response != expected {
		t.Errorf("got %q, want %q", resp.Response, expected)
	}
}

func TestGraphRAG_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(GraphRAGResponse{Response: "late"})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.GraphRAG(ctx, GraphRAGRequest{Query: "timeout test"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestGraphRAG_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ServiceError{Error: "internal failure"})
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	defer c.Close()

	_, err := c.GraphRAG(context.Background(), GraphRAGRequest{Query: "error test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTriplesQuery_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flow/default/service/triples" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req TriplesQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Subject == nil {
			t.Error("expected non-nil subject")
		}

		resp := TriplesQueryResponse{
			Response: []Triple{
				{
					Subject:   NewIRITerm("TrustGraph"),
					Predicate: NewIRITerm("uses"),
					Object:    NewIRITerm("Apache Cassandra"),
				},
				{
					Subject:   NewIRITerm("TrustGraph"),
					Predicate: NewIRITerm("uses"),
					Object:    NewIRITerm("Qdrant"),
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	defer c.Close()

	subj := NewIRITerm("TrustGraph")
	resp, err := c.TriplesQuery(context.Background(), TriplesQueryRequest{
		Subject: &subj,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("TriplesQuery: %v", err)
	}
	if len(resp.Response) != 2 {
		t.Fatalf("got %d triples, want 2", len(resp.Response))
	}
	if resp.Response[0].Subject.IRI != "TrustGraph" {
		t.Errorf("got subject %q, want %q", resp.Response[0].Subject.IRI, "TrustGraph")
	}
	if !resp.Response[0].Subject.IsEntity() {
		t.Error("expected subject to be entity")
	}
	if resp.Response[0].Object.DisplayValue() != "Apache Cassandra" {
		t.Errorf("got object %q, want %q", resp.Response[0].Object.DisplayValue(), "Apache Cassandra")
	}
}

func TestGraphEmbeddingsQuery_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flow/default/service/graph-embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req GraphEmbeddingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Vectors) == 0 {
			t.Error("expected non-empty vectors")
		}

		resp := GraphEmbeddingsResponse{
			Entities: []GraphEntity{
				{Entity: NewIRITerm("Apache Cassandra"), Score: 0.92},
				{Entity: NewIRITerm("Qdrant"), Score: 0.87},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	defer c.Close()

	resp, err := c.GraphEmbeddingsQuery(context.Background(), GraphEmbeddingsRequest{
		Vectors: [][]float64{{0.1, 0.2, 0.3}},
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("GraphEmbeddingsQuery: %v", err)
	}
	if len(resp.Entities) != 2 {
		t.Fatalf("got %d entities, want 2", len(resp.Entities))
	}
	if resp.Entities[0].Entity.IRI != "Apache Cassandra" {
		t.Errorf("got entity %q, want %q", resp.Entities[0].Entity.IRI, "Apache Cassandra")
	}
	if resp.Entities[0].Score != 0.92 {
		t.Errorf("got score %f, want 0.92", resp.Entities[0].Score)
	}
}

func TestAuthToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(GraphRAGResponse{Response: "ok"})
	}))
	defer srv.Close()

	c := New(srv.URL, "my-secret-token")
	defer c.Close()

	_, err := c.GraphRAG(context.Background(), GraphRAGRequest{Query: "auth test"})
	if err != nil {
		t.Fatalf("GraphRAG: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("got auth %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

func TestHealthProbe_FlipsAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create client pointed at the running server.
	c := New(srv.URL, "")
	defer c.Close()

	// Server is up, probe should have set available=true.
	if !c.Available() {
		t.Error("expected Available()=true when server is running")
	}

	// Shut down the server.
	srv.Close()

	// Manually trigger a probe to detect the shutdown.
	c.probe()

	if c.Available() {
		t.Error("expected Available()=false after server shutdown")
	}
}

func TestTermDisplayValue(t *testing.T) {
	tests := []struct {
		name string
		term Term
		want string
	}{
		{"IRI", NewIRITerm("http://example.org/foo"), "http://example.org/foo"},
		{"Literal", NewLiteralTerm("hello world"), "hello world"},
		{"Blank", Term{Type: "b", ID: "_:b0"}, "_:b0"},
		{"Unknown", Term{Type: "x"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.term.DisplayValue(); got != tt.want {
				t.Errorf("DisplayValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTermIsEntity(t *testing.T) {
	if !NewIRITerm("foo").IsEntity() {
		t.Error("IRI term should be entity")
	}
	if NewLiteralTerm("bar").IsEntity() {
		t.Error("Literal term should not be entity")
	}
}

func TestTermShortLabel(t *testing.T) {
	tests := []struct {
		name   string
		term   Term
		maxLen int
		want   string
	}{
		{"IRI with path", NewIRITerm("http://example.org/entities/computeCommander"), 16, "computeCommander"},
		{"IRI with path truncated", NewIRITerm("http://example.org/entities/computeCommander"), 12, "computeCom.."},
		{"IRI with hash", NewIRITerm("http://example.org#foo"), 16, "foo"},
		{"Short IRI", NewIRITerm("foo"), 16, "foo"},
		{"Literal", NewLiteralTerm("hello"), 16, "hello"},
		{"Truncated literal", NewLiteralTerm("a very long literal value"), 10, "a very l.."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.term.ShortLabel(tt.maxLen); got != tt.want {
				t.Errorf("ShortLabel(%d) = %q, want %q", tt.maxLen, got, tt.want)
			}
		})
	}
}
