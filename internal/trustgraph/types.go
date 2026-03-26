// Package trustgraph provides a typed HTTP client for the TrustGraph REST gateway.
//
// This is a bootstrap copy from openbrain/mcp/internal/trustgraph.
// Post-MVP, this will be replaced by an import from the shared module
// github.com/noko/trustgraph-go.
package trustgraph

// ── RDF Term Types ───────────────────────────────────────────────────────────
//
// TrustGraph's REST gateway uses a typed RDF term format throughout its wire
// protocol. The discriminator field "t" indicates the term type:
//   "i" = IRI (entity/URI) -- value in "i" field
//   "l" = Literal (string/number) -- value in "v" field, optional "dt"/"ln"
//   "b" = Blank node -- id in "d" field
//   "t" = Quoted triple (RDF-star) -- nested triple in "tr" field
//
// Both requests (triples query) and responses (triples, graph-embeddings)
// use this format. The Go bridge must serialize and deserialize accordingly.

// Term represents a typed RDF term in TrustGraph's wire format.
// Used for triples request/response AND graph-embeddings response.
type Term struct {
	Type     string `json:"t"`            // "i" (IRI), "l" (literal), "b" (blank), "t" (triple)
	IRI      string `json:"i,omitempty"`  // set when Type="i"
	Value    string `json:"v,omitempty"`  // set when Type="l" (literal value)
	Datatype string `json:"dt,omitempty"` // optional, set when Type="l" (XSD datatype)
	Language string `json:"ln,omitempty"` // optional, set when Type="l" (language tag)
	ID       string `json:"d,omitempty"`  // set when Type="b" (blank node ID)
}

// NewIRITerm creates a Term for an IRI/entity reference.
func NewIRITerm(iri string) Term {
	return Term{Type: "i", IRI: iri}
}

// NewLiteralTerm creates a Term for a literal value.
func NewLiteralTerm(value string) Term {
	return Term{Type: "l", Value: value}
}

// DisplayValue returns a human-readable string regardless of term type.
func (t Term) DisplayValue() string {
	switch t.Type {
	case "i":
		return t.IRI
	case "l":
		return t.Value
	case "b":
		return t.ID
	default:
		return ""
	}
}

// ShortLabel returns a truncated display label for UI rendering.
// IRIs are shortened to the local name (after last / or #).
func (t Term) ShortLabel(maxLen int) string {
	val := t.DisplayValue()
	if t.Type == "i" {
		// Extract local name from IRI.
		for i := len(val) - 1; i >= 0; i-- {
			if val[i] == '/' || val[i] == '#' {
				val = val[i+1:]
				break
			}
		}
	}
	if len(val) > maxLen {
		if maxLen < 4 {
			return val[:maxLen]
		}
		return val[:maxLen-2] + ".."
	}
	return val
}

// IsEntity returns true if this term is an IRI (entity reference).
func (t Term) IsEntity() bool {
	return t.Type == "i"
}

// ── Graph RAG ────────────────────────────────────────────────────────────────

// GraphRAGRequest is the request body for the TrustGraph graph-rag service.
type GraphRAGRequest struct {
	Query           string `json:"query"`
	User            string `json:"user,omitempty"`
	Collection      string `json:"collection,omitempty"`
	EntityLimit     int    `json:"entity_limit,omitempty"`
	TripleLimit     int    `json:"triple_limit,omitempty"`
	MaxSubgraphSize int    `json:"max_subgraph_size,omitempty"`
	MaxPathLength   int    `json:"max_path_length,omitempty"`
}

// GraphRAGResponse is the response from the TrustGraph graph-rag service.
type GraphRAGResponse struct {
	Response string `json:"response"`
}

// ── Triples Query ────────────────────────────────────────────────────────────

// TriplesQueryRequest is the request body for the TrustGraph triples query service.
type TriplesQueryRequest struct {
	Subject   *Term `json:"s,omitempty"`
	Predicate *Term `json:"p,omitempty"`
	Object    *Term `json:"o,omitempty"`
	Limit     int   `json:"limit,omitempty"`
}

// Triple represents a single RDF triple from TrustGraph.
type Triple struct {
	Subject   Term `json:"s"`
	Predicate Term `json:"p"`
	Object    Term `json:"o"`
}

// TriplesQueryResponse is the response from the TrustGraph triples query service.
type TriplesQueryResponse struct {
	Response []Triple `json:"response"`
}

// ── Embeddings ───────────────────────────────────────────────────────────────

// EmbeddingsRequest is the request body for the TrustGraph embeddings service.
type EmbeddingsRequest struct {
	Text string `json:"text"`
}

// EmbeddingsResponse is the response from the TrustGraph embeddings service.
type EmbeddingsResponse struct {
	Vectors [][]float64 `json:"vectors"`
}

// ── Graph Embeddings Query ───────────────────────────────────────────────────

// GraphEmbeddingsRequest is the request body for the TrustGraph graph-embeddings service.
type GraphEmbeddingsRequest struct {
	Vectors [][]float64 `json:"vectors"`
	Limit   int         `json:"limit,omitempty"`
}

// GraphEntity represents an entity result from graph-embeddings vector search.
// The Entity field uses the typed Term format: {"t": "i", "i": "..."} for IRIs.
type GraphEntity struct {
	Entity Term    `json:"entity"`
	Score  float64 `json:"score,omitempty"`
}

// GraphEmbeddingsResponse is the response from the TrustGraph graph-embeddings service.
type GraphEmbeddingsResponse struct {
	Entities []GraphEntity `json:"entities"`
}

// ── Error ────────────────────────────────────────────────────────────────────

// ServiceError is the error response from TrustGraph services.
type ServiceError struct {
	Error string `json:"error"`
}
