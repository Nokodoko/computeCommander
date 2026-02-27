// Package gateway provides an HTTP API gateway for external integrations
// with ComputeCommander's agent orchestration system.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/internal/platform/db"
)

// GatewayOpts configures a Gateway instance.
type GatewayOpts struct {
	DB       db.DB
	Spawner  *agents.Spawner
	Mail     mail.MailStore
	Queue    merge.MergeQueue
	Version  string
	StartAt  time.Time
}

// Gateway is the HTTP API server for ComputeCommander.
type Gateway struct {
	db      db.DB
	spawner *agents.Spawner
	mail    mail.MailStore
	queue   merge.MergeQueue
	version string
	startAt time.Time
	mux     *http.ServeMux
	reqID   atomic.Uint64
}

// NewGateway creates a Gateway from the provided options.
func NewGateway(opts GatewayOpts) *Gateway {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	startAt := opts.StartAt
	if startAt.IsZero() {
		startAt = time.Now()
	}

	g := &Gateway{
		db:      opts.DB,
		spawner: opts.Spawner,
		mail:    opts.Mail,
		queue:   opts.Queue,
		version: version,
		startAt: startAt,
		mux:     http.NewServeMux(),
	}

	g.registerRoutes()
	return g
}

// Start listens on the given address and serves HTTP until ctx is cancelled.
func (g *Gateway) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           g.middleware(g.mux),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return fmt.Errorf("gateway listen: %w", err)
	}
}

// Handler returns the gateway's HTTP handler for testing purposes.
func (g *Gateway) Handler() http.Handler {
	return g.middleware(g.mux)
}

// registerRoutes wires all API endpoints into the mux.
func (g *Gateway) registerRoutes() {
	g.mux.HandleFunc("GET /api/v1/health", g.handleHealth)
	g.mux.HandleFunc("GET /api/v1/status", g.handleStatus)
	g.mux.HandleFunc("GET /api/v1/agents", g.handleListAgents)
	g.mux.HandleFunc("GET /api/v1/agents/", g.handleGetAgent)
	g.mux.HandleFunc("POST /api/v1/agents", g.handleSpawnAgent)
	g.mux.HandleFunc("DELETE /api/v1/agents/", g.handleStopAgent)
	g.mux.HandleFunc("GET /api/v1/mail", g.handleListMail)
	g.mux.HandleFunc("POST /api/v1/mail", g.handleSendMail)
	g.mux.HandleFunc("GET /api/v1/merge/queue", g.handleMergeQueue)
	g.mux.HandleFunc("GET /api/v1/costs", g.handleCosts)
}

// middleware chains logging, CORS, and request ID middleware.
func (g *Gateway) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Request ID
		id := g.reqID.Add(1)
		requestID := fmt.Sprintf("req-%d", id)
		w.Header().Set("X-Request-ID", requestID)

		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Logging
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("[%s] %s %s %d %s", requestID, r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// --- Handlers ---

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": g.version,
		"uptime":  time.Since(g.startAt).String(),
	})
}

func (g *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sessions, err := g.spawner.ListSessions(ctx, agents.ListOpts{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list sessions: %v", err)
		return
	}

	counts := map[string]int{
		"total":     len(sessions),
		"booting":   0,
		"working":   0,
		"completed": 0,
		"stalled":   0,
		"zombie":    0,
	}
	for _, s := range sessions {
		counts[string(s.State)]++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fleet":  counts,
		"uptime": time.Since(g.startAt).String(),
	})
}

func (g *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	opts := agents.ListOpts{}
	if cap := r.URL.Query().Get("capability"); cap != "" {
		opts.Capability = agents.Capability(cap)
	}
	if state := r.URL.Query().Get("state"); state != "" {
		opts.State = agents.SessionState(state)
	}

	sessions, err := g.spawner.ListSessions(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list agents: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents": sessions,
		"count":  len(sessions),
	})
}

func (g *Gateway) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	name := extractPathParam(r.URL.Path, "/api/v1/agents/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "agent name is required")
		return
	}

	ctx := r.Context()
	sessions, err := g.spawner.ListSessions(ctx, agents.ListOpts{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list agents: %v", err)
		return
	}

	for _, s := range sessions {
		if s.AgentName == name {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}

	writeError(w, http.StatusNotFound, "agent %q not found", name)
}

func (g *Gateway) handleSpawnAgent(w http.ResponseWriter, r *http.Request) {
	var req agents.SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode request: %v", err)
		return
	}

	ctx := r.Context()
	result, err := g.spawner.Spawn(ctx, req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "spawn agent: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (g *Gateway) handleStopAgent(w http.ResponseWriter, r *http.Request) {
	name := extractPathParam(r.URL.Path, "/api/v1/agents/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "agent name is required")
		return
	}

	ctx := r.Context()
	if err := g.spawner.Stop(ctx, name, agents.StopOpts{}); err != nil {
		writeError(w, http.StatusInternalServerError, "stop agent: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "stopped",
		"agent":   name,
	})
}

func (g *Gateway) handleListMail(w http.ResponseWriter, r *http.Request) {
	opts := mail.ListOpts{}
	if agent := r.URL.Query().Get("agent"); agent != "" {
		opts.Agent = agent
	}
	if from := r.URL.Query().Get("from"); from != "" {
		opts.From = from
	}
	if unread := r.URL.Query().Get("unread"); unread == "true" {
		opts.Unread = true
	}

	messages, err := g.mail.List(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list mail: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"messages": messages,
		"count":    len(messages),
	})
}

func (g *Gateway) handleSendMail(w http.ResponseWriter, r *http.Request) {
	var msg mail.MailMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeError(w, http.StatusBadRequest, "decode message: %v", err)
		return
	}

	if err := g.mail.Send(&msg); err != nil {
		writeError(w, http.StatusInternalServerError, "send mail: %v", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "sent",
		"id":     msg.ID,
	})
}

func (g *Gateway) handleMergeQueue(w http.ResponseWriter, r *http.Request) {
	opts := merge.ListOpts{}
	if status := r.URL.Query().Get("status"); status != "" {
		s := merge.MergeStatus(status)
		opts.Status = &s
	}

	entries, err := g.queue.List(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list merge queue: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

func (g *Gateway) handleCosts(w http.ResponseWriter, r *http.Request) {
	// Cost tracking is a stub until the metrics system is implemented.
	writeJSON(w, http.StatusOK, map[string]any{
		"total":     0.0,
		"breakdown": map[string]any{},
		"currency":  "USD",
		"note":      "cost tracking not yet implemented",
	})
}

// --- Helpers ---

// extractPathParam extracts a trailing path segment after a prefix.
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	param := strings.TrimPrefix(path, prefix)
	param = strings.TrimSuffix(param, "/")
	// Take only the first segment
	if idx := strings.Index(param, "/"); idx >= 0 {
		param = param[:idx]
	}
	return param
}

// writeJSON serializes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	writeJSON(w, status, map[string]any{
		"error":  msg,
		"status": status,
	})
}
