# k8s-cluster/api-server/src/routes/ -- API Route Handlers

## Purpose
Route handler functions for the API server. Each file exports pure functions that return `Response` objects.

## Technology
- TypeScript 5.x
- Bun-native `Response` constructor

## Contents
| File | Description |
|------|-------------|
| `health.ts` | `handleHealth()`: returns `{ status: "ok", timestamp, uptime }` with 200 |
| `status.ts` | `handleStatus()`: returns cluster service overview. `handleConnections(wsUrl)`: proxies to WS server /health for connection count; returns 503 on failure |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `handleHealth` | `() => Response` | `Response` (200) | Health check with uptime tracking via module-level `startTime` |
| `handleStatus` | `() => Response` | `Response` (200) | Static cluster status (version, services) |
| `handleConnections` | `(wsServerUrl: string) => Promise<Response>` | `Response` (200/503) | Fetches `/health` from WS server, extracts connection count |

## Logging
N/A (logging handled by middleware layer)

## Style Guide
- Pure functions returning `Response`
- JSON serialization via `JSON.stringify`
- Module-level `startTime` for uptime calculation
- Async only where needed (`handleConnections` does HTTP fetch)

**Representative snippet (from `health.ts`):**
```typescript
const startTime = Date.now();

export function handleHealth(): Response {
  const body: HealthResponse = {
    status: "ok",
    timestamp: new Date().toISOString(),
    uptime: Math.floor((Date.now() - startTime) / 1000),
  };
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
```
