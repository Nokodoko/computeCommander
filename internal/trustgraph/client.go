package trustgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// healthProbeInterval is how often the background probe checks TrustGraph.
const healthProbeInterval = 30 * time.Second

// Client wraps TrustGraph's REST gateway at /api/v1/flow/{flow}/service/{kind}.
//
// This is a bootstrap copy from openbrain/mcp/internal/trustgraph.
// Post-MVP, this will be replaced by an import from the shared module
// github.com/noko/trustgraph-go.
type Client struct {
	baseURL    string       // e.g., "http://localhost:8088"
	token      string       // API key for gateway auth (matches GATEWAY_SECRET)
	flowID     string       // TrustGraph flow ID (default: "default")
	httpClient *http.Client // shared, goroutine-safe
	available  atomic.Bool  // availability cache, updated by health probe
	lastCheck  atomic.Int64 // unix timestamp of last health check
	cancel     context.CancelFunc
}

// New creates a TrustGraph Client. Starts a background health probe goroutine
// that checks availability every 30 seconds via TCP connect.
// The flowID parameter specifies which TrustGraph flow to query (e.g., "default").
func New(baseURL string, token string, flowID ...string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second

	fid := "default"
	if len(flowID) > 0 && flowID[0] != "" {
		fid = flowID[0]
	}

	c := &Client{
		baseURL: baseURL,
		token:   token,
		flowID:  fid,
		httpClient: &http.Client{
			Transport: transport,
			// Defensive backstop so a stalled read cannot outlive a sane
			// bound regardless of caller context. Intentionally larger than
			// the default per-query context deadline (TGQueryTimeout, 30s)
			// — callers should still set their own context deadline; this
			// only fires if they neglect to.
			Timeout: 60 * time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// Run initial probe synchronously so Available() is accurate on first call.
	c.probe()

	go c.healthLoop(ctx)

	return c
}

// flowServicePath returns the flow-scoped service path for the given service kind.
// TrustGraph's gateway routes per-flow services at /api/v1/flow/{flow}/service/{kind}.
func (c *Client) flowServicePath(kind string) string {
	return fmt.Sprintf("/api/v1/flow/%s/service/%s", c.flowID, kind)
}

// GraphRAG sends a graph-rag query. Returns the LLM-generated response
// informed by graph context.
func (c *Client) GraphRAG(ctx context.Context, req GraphRAGRequest) (GraphRAGResponse, error) {
	var resp GraphRAGResponse
	if err := c.post(ctx, c.flowServicePath("graph-rag"), req, &resp); err != nil {
		return GraphRAGResponse{}, err
	}
	return resp, nil
}

// TriplesQuery queries knowledge graph triples by subject-predicate-object pattern.
func (c *Client) TriplesQuery(ctx context.Context, req TriplesQueryRequest) (TriplesQueryResponse, error) {
	var resp TriplesQueryResponse
	if err := c.post(ctx, c.flowServicePath("triples"), req, &resp); err != nil {
		return TriplesQueryResponse{}, err
	}
	return resp, nil
}

// GraphEmbeddingsQuery finds entities by vector similarity search.
func (c *Client) GraphEmbeddingsQuery(ctx context.Context, req GraphEmbeddingsRequest) (GraphEmbeddingsResponse, error) {
	var resp GraphEmbeddingsResponse
	if err := c.post(ctx, c.flowServicePath("graph-embeddings"), req, &resp); err != nil {
		return GraphEmbeddingsResponse{}, err
	}
	return resp, nil
}

// Embeddings generates vector embeddings for text via TrustGraph's embeddings service.
func (c *Client) Embeddings(ctx context.Context, text string) (EmbeddingsResponse, error) {
	var resp EmbeddingsResponse
	if err := c.post(ctx, c.flowServicePath("embeddings"), EmbeddingsRequest{Text: text}, &resp); err != nil {
		return EmbeddingsResponse{}, err
	}
	return resp, nil
}

// Available returns the cached availability status.
func (c *Client) Available() bool {
	return c.available.Load()
}

// Close stops the background health probe.
func (c *Client) Close() {
	c.cancel()
}

// ── Internal ─────────────────────────────────────────────────────────────────

// post sends a JSON POST request to the TrustGraph gateway and decodes the response.
func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("trustgraph: marshal request: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("trustgraph: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trustgraph: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("trustgraph: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var svcErr ServiceError
		if json.Unmarshal(respBody, &svcErr) == nil && svcErr.Error != "" {
			return fmt.Errorf("trustgraph: %s (HTTP %d)", svcErr.Error, resp.StatusCode)
		}
		return fmt.Errorf("trustgraph: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, dest); err != nil {
		return fmt.Errorf("trustgraph: decode response: %w", err)
	}
	return nil
}

// healthLoop runs the periodic health probe until the context is cancelled.
func (c *Client) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(healthProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.probe()
		}
	}
}

// probe checks TrustGraph availability via TCP connect to the gateway host:port.
func (c *Client) probe() {
	c.lastCheck.Store(time.Now().Unix())

	// Extract host:port from baseURL for TCP connect probe.
	host := c.baseURL
	// Strip scheme.
	if idx := len("http://"); len(host) > idx && host[:idx] == "http://" {
		host = host[idx:]
	} else if idx := len("https://"); len(host) > idx && host[:idx] == "https://" {
		host = host[idx:]
	}
	// Strip path.
	for i, ch := range host {
		if ch == '/' {
			host = host[:i]
			break
		}
	}
	// Add default port if missing.
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = host + ":80"
	}

	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		if c.available.Load() {
			slog.Info("trustgraph: gateway became unavailable", "host", host, "error", err)
		}
		c.available.Store(false)
		return
	}
	conn.Close()

	if !c.available.Load() {
		slog.Info("trustgraph: gateway became available", "host", host)
	}
	c.available.Store(true)
}
