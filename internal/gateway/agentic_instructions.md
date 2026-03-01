# internal/gateway/ -- HTTP API Gateway

## Purpose
Provides an HTTP REST API gateway for external integrations with ComputeCommander's agent orchestration system. Exposes endpoints at `/api/v1/` for health checks, fleet status, agent CRUD, mail operations, merge queue queries, and cost tracking.

## Technology
- Go 1.25
- `net/http` with Go 1.22+ method-pattern routing (`"GET /api/v1/..."`)
- `encoding/json` for JSON request/response
- Depends on: `internal/agents`, `internal/mail`, `internal/merge`, `internal/platform/db`

## Contents
| File | Description |
|------|-------------|
| `gateway.go` | `Gateway` struct, `NewGateway()`, `Start()`, route registration, middleware chain (request ID, CORS, logging), all HTTP handlers |
| `gateway_test.go` | Tests for handler responses, middleware behavior, and error formatting |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewGateway` | `func NewGateway(opts GatewayOpts) *Gateway` | `*Gateway` | Creates gateway with DB, spawner, mail, queue; registers routes |
| `Start` | `func (g *Gateway) Start(ctx context.Context, addr string) error` | `error` | Listens on addr, serves HTTP until ctx cancelled, graceful shutdown |
| `Handler` | `func (g *Gateway) Handler() http.Handler` | `http.Handler` | Returns the gateway's HTTP handler for testing |
| `handleHealth` | `func (g *Gateway) handleHealth(w, r)` | - | Returns status, version, uptime |
| `handleStatus` | `func (g *Gateway) handleStatus(w, r)` | - | Returns fleet counts by state |
| `handleListAgents` | `func (g *Gateway) handleListAgents(w, r)` | - | Lists agents with ?capability and ?state filters |
| `handleSpawnAgent` | `func (g *Gateway) handleSpawnAgent(w, r)` | - | POST JSON SpawnRequest, returns SpawnResult (201) |
| `handleStopAgent` | `func (g *Gateway) handleStopAgent(w, r)` | - | DELETE /api/v1/agents/{name} |
| `handleListMail` | `func (g *Gateway) handleListMail(w, r)` | - | Lists mail with ?agent, ?from, ?unread filters |
| `handleSendMail` | `func (g *Gateway) handleSendMail(w, r)` | - | POST JSON MailMessage |
| `handleMergeQueue` | `func (g *Gateway) handleMergeQueue(w, r)` | - | Lists merge queue entries with ?status filter |
| `handleCosts` | `func (g *Gateway) handleCosts(w, r)` | - | Stub: returns zero costs |

## Data Types

### GatewayOpts (struct)
Fields: DB, Spawner, Mail, Queue, Version, StartAt

### Gateway (struct)
Fields: db, spawner, mail, queue, version, startAt, mux, reqID (atomic counter)

### responseWriter (struct)
Wraps `http.ResponseWriter` to capture status code for logging.

## Logging
- Request logging via `log.Printf("[%s] %s %s %d %s", reqID, method, path, status, duration)`
- JSON encode errors via `log.Printf("json encode error: %v", err)`

## CRUD Entry Points
- **Create**: `POST /api/v1/agents` (spawn), `POST /api/v1/mail` (send)
- **Read**: `GET /api/v1/health`, `GET /api/v1/status`, `GET /api/v1/agents`, `GET /api/v1/agents/{name}`, `GET /api/v1/mail`, `GET /api/v1/merge/queue`, `GET /api/v1/costs`
- **Update**: N/A
- **Delete**: `DELETE /api/v1/agents/{name}` (stop)

## Style Guide
- Middleware chain: request ID (atomic counter) -> CORS -> logging
- All responses are JSON via `writeJSON()` helper
- Errors returned as JSON `{"error": "...", "status": 500}` via `writeError()`
- Path params extracted via `extractPathParam()` string helper
- HTTP timeouts: read header 10s, write 30s, idle 60s

**Representative snippet (from `gateway.go`):**
```go
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
```
