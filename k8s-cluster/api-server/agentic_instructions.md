# k8s-cluster/api-server/ -- Bun HTTP API Server (Track B)

## Purpose
TypeScript HTTP API server running on Bun runtime. Provides health checks, cluster status, and a connection count proxy to the WebSocket server. Designed as stateless for 2-replica K8s deployment.

## Technology
- TypeScript 5.x
- Bun runtime (native `Bun.serve`)
- `bun:test` for unit testing

## Contents
| File | Description |
|------|-------------|
| `src/server.ts` | `createServer()`: Bun HTTP server with route dispatch, graceful shutdown on SIGTERM/SIGINT |
| `src/types.ts` | `HealthResponse`, `StatusResponse`, `ErrorResponse`, `ServerConfig`, `DEFAULT_CONFIG` |
| `src/routes/health.ts` | `handleHealth()`: returns `{ status, timestamp, uptime }` |
| `src/routes/status.ts` | `handleStatus()`: returns cluster service status. `handleConnections()`: proxies to WS server `/health` |
| `src/middleware/logger.ts` | `LogEntry` interface, `logRequest()`, `createLogEntry()` |
| `src/middleware/error-handler.ts` | `createErrorResponse()`, `handleNotFound()` |
| `tests/server.test.ts` | Server lifecycle and route tests |
| `tests/routes/health.test.ts` | Health endpoint response tests |
| `package.json` | Bun dependencies and scripts |
| `tsconfig.json` | TypeScript compiler configuration |
| `Dockerfile` | Multi-stage Bun build |

## Key Functions

| Function | File | Returns | Description |
|----------|------|---------|-------------|
| `createServer` | `server.ts` | `{ server, shutdown }` | Creates Bun HTTP server with route dispatch and shutdown handler |
| `handleHealth` | `routes/health.ts` | `Response` | Returns 200 JSON with status, ISO timestamp, uptime in seconds |
| `handleStatus` | `routes/status.ts` | `Response` | Returns 200 JSON with cluster service status |
| `handleConnections` | `routes/status.ts` | `Promise<Response>` | Fetches connection count from WS server, returns 503 if unreachable |
| `createErrorResponse` | `middleware/error-handler.ts` | `Response` | Creates JSON error response with status code |
| `handleNotFound` | `middleware/error-handler.ts` | `Response` | Returns 404 for unknown routes |
| `logRequest` | `middleware/logger.ts` | `void` | Logs structured request entry to console |
| `createLogEntry` | `middleware/logger.ts` | `LogEntry` | Creates log entry from request, status, and timing |

## Data Types

### ServerConfig (interface)
Fields: port (number, default from `API_PORT` env or 3000), wsServerUrl (string, default from `WS_SERVER_URL` env)

### HealthResponse (interface)
Fields: status ("ok" | "error"), timestamp (ISO string), uptime (seconds)

### StatusResponse (interface)
Fields: cluster, version, services (api, websocket, loadBalancer), timestamp

### ErrorResponse (interface)
Fields: error (string), statusCode (number), timestamp (ISO string)

### LogEntry (interface)
Fields: method, path, status, durationMs, timestamp

## Logging
- Structured request logging: `[timestamp] METHOD /path STATUS durationMs`
- Console.log for server lifecycle events

## CRUD Entry Points
- **Read**: `GET /health` (200), `GET /api/status` (200), `GET /api/connections` (200 or 503)
- 404 for all unknown routes

## Style Guide
- Functional exports: `createServer()` returns `{ server, shutdown }`
- Bun-native `Bun.serve` with `fetch` handler
- Route dispatch via `switch (true)` on path + method
- Graceful shutdown: `process.on("SIGTERM", shutdown)`
- Configuration via environment variables with defaults
- All responses are JSON with `Content-Type: application/json`

**Representative snippet (from `server.ts`):**
```typescript
export function createServer(config: ServerConfig = DEFAULT_CONFIG) {
  const server = Bun.serve({
    port: config.port,
    async fetch(req) {
      const url = new URL(req.url);
      switch (true) {
        case url.pathname === "/health" && req.method === "GET":
          return handleHealth();
        case url.pathname === "/api/status" && req.method === "GET":
          return handleStatus();
        default:
          return handleNotFound(url.pathname);
      }
    },
  });
  return { server, shutdown };
}
```
