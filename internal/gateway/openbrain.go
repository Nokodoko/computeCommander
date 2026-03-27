package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/noko/computecommander/internal/config"
)

// OpenBrainProxy holds the configuration needed to proxy requests to the
// OpenBrain MCP server. It is initialised from the Config.OpenBrain section.
type OpenBrainProxy struct {
	cfg    config.OpenBrainConfig
	client *http.Client
}

// NewOpenBrainProxy creates a proxy that forwards requests to the MCP server.
func NewOpenBrainProxy(cfg config.OpenBrainConfig) *OpenBrainProxy {
	return &OpenBrainProxy{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// RegisterRoutes adds the OpenBrain proxy endpoints to the gateway mux.
// Called from Gateway.registerRoutes when OpenBrain is enabled.
func (p *OpenBrainProxy) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/openbrain/entries", p.handleEntries)
	mux.HandleFunc("GET /api/v1/openbrain/stream", p.handleStream)
	mux.HandleFunc("GET /api/v1/openbrain/status", p.handleStatus)
}

// handleEntries proxies to the MCP server's /api/v1/openbrain/entries endpoint.
func (p *OpenBrainProxy) handleEntries(w http.ResponseWriter, r *http.Request) {
	target := fmt.Sprintf("%s/api/v1/openbrain/entries", p.cfg.MCPSseURL)

	// Forward query parameters.
	if rawQuery := r.URL.RawQuery; rawQuery != "" {
		target += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("build request: %v", err),
		})
		return
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  fmt.Sprintf("mcp server unreachable: %v", err),
			"status": "disconnected",
		})
		return
	}
	defer resp.Body.Close()

	// Copy headers.
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("openbrain proxy: copy error: %v", err)
	}
}

// handleStream proxies the SSE stream from the MCP server's /events/memories endpoint.
func (p *OpenBrainProxy) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	target := fmt.Sprintf("%s/events/memories", p.cfg.MCPSseURL)

	// Forward query parameters.
	if rawQuery := r.URL.RawQuery; rawQuery != "" {
		target += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("build request: %v", err), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	// Use a client without timeout for long-lived SSE connections.
	sseClient := &http.Client{
		Timeout: 0,
	}
	resp, err := sseClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":  fmt.Sprintf("mcp sse unreachable: %v", err),
			"status": "disconnected",
		})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			flusher.Flush()
		}
		if err != nil {
			return
		}
	}
}

// openBrainStatusResponse is the typed response for the status endpoint.
type openBrainStatusResponse struct {
	Status     string `json:"status"`
	EntryCount int    `json:"entry_count"`
	LastWrite  string `json:"last_write"`
	MCPURL     string `json:"mcp_url"`
}

// handleStatus returns the connection health for the OpenBrain MCP server.
func (p *OpenBrainProxy) handleStatus(w http.ResponseWriter, r *http.Request) {
	target := fmt.Sprintf("%s/api/v1/openbrain/entries?limit=1", p.cfg.MCPSseURL)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, openBrainStatusResponse{
			Status: "error",
			MCPURL: p.cfg.MCPSseURL,
		})
		return
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, openBrainStatusResponse{
			Status: "disconnected",
			MCPURL: p.cfg.MCPSseURL,
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusOK, openBrainStatusResponse{
			Status: fmt.Sprintf("error_%d", resp.StatusCode),
			MCPURL: p.cfg.MCPSseURL,
		})
		return
	}

	var body struct {
		Entries []json.RawMessage `json:"entries"`
		Count   int               `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusOK, openBrainStatusResponse{
			Status: "connected",
			MCPURL: p.cfg.MCPSseURL,
		})
		return
	}

	lastWrite := ""
	if len(body.Entries) > 0 {
		var entry struct {
			CapturedAt time.Time `json:"captured_at"`
		}
		if json.Unmarshal(body.Entries[0], &entry) == nil {
			lastWrite = entry.CapturedAt.Format(time.RFC3339)
		}
	}

	writeJSON(w, http.StatusOK, openBrainStatusResponse{
		Status:     "connected",
		EntryCount: body.Count,
		LastWrite:  lastWrite,
		MCPURL:     p.cfg.MCPSseURL,
	})
}
