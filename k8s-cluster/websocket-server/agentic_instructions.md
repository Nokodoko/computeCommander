# k8s-cluster/websocket-server/ -- Bun WebSocket Server (Track A)

## Purpose
TypeScript WebSocket server running on Bun runtime. Handles concurrent WebSocket connections with tracking, heartbeat/ping-pong health monitoring, graceful shutdown, and an HTTP health endpoint. Designed for 1-replica K8s deployment.

## Technology
- TypeScript 5.x
- Bun runtime (native `Bun.serve` with WebSocket upgrade)
- `bun:test` for unit testing

## Contents
| File | Description |
|------|-------------|
| `src/server.ts` | `createServer()`: Bun server with HTTP health endpoint and WebSocket upgrade at `/ws`, heartbeat timer, graceful shutdown |
| `src/connection-manager.ts` | `ConnectionManager` class: Map-based connection tracking, add/remove/count/getAll/updateLastPing/getStaleConnections/clear |
| `src/types.ts` | `WSConnection`, `ServerConfig`, `ServerStatus` interfaces, `DEFAULT_CONFIG` |
| `tests/server.test.ts` | Server lifecycle, WebSocket connection, health endpoint tests |
| `tests/connection-manager.test.ts` | ConnectionManager unit tests |
| `package.json` | Bun dependencies and scripts |
| `tsconfig.json` | TypeScript compiler configuration |
| `Dockerfile` | Multi-stage Bun build |

## Key Functions

| Function | File | Returns | Description |
|----------|------|---------|-------------|
| `createServer` | `server.ts` | `{ server, manager, shutdown }` | Creates Bun server with WS upgrade, health endpoint, heartbeat timer |
| `ConnectionManager.add` | `connection-manager.ts` | `WSConnection` | Adds connection; throws if max reached |
| `ConnectionManager.remove` | `connection-manager.ts` | `boolean` | Removes connection by ID |
| `ConnectionManager.count` | `connection-manager.ts` | `number` | Returns active connection count |
| `ConnectionManager.getStaleConnections` | `connection-manager.ts` | `WSConnection[]` | Returns connections exceeding timeout |
| `ConnectionManager.clear` | `connection-manager.ts` | `void` | Removes all connections (shutdown) |

## Data Types

### WSConnection (interface)
Fields: id (string), ws (WebSocket), connectedAt (Date), lastPing (Date)

### ServerConfig (interface)
Fields: port (number, default from `WS_PORT` env or 8080), maxConnections (1000), heartbeatInterval (30000ms), heartbeatTimeout (10000ms)

### ServerStatus (interface)
Fields: status ("ok" | "error"), connections (number), uptime (number), timestamp (string)

### ConnectionManager (class)
Private: connections (Map<string, WSConnection>), maxConnections (number)

## Logging
- Console.log for server lifecycle ("Shutting down...", "stopped.")

## CRUD Entry Points
- **Create**: WS upgrade at `/ws` creates a connection
- **Read**: `GET /health` returns `{ status, connections, uptime, timestamp }`
- **Update**: Message receipt updates `lastPing` timestamp
- **Delete**: WS close removes connection; heartbeat timer closes stale connections

## Style Guide
- `createServer()` returns `{ server, manager, shutdown }` for testing access
- `import.meta.main` guard for direct execution
- UUID-based connection IDs via `crypto.randomUUID()`
- Ping/pong: client sends `{ type: "ping" }`, server responds `{ type: "pong", timestamp }`
- Max connections enforced: close with code 1013 ("Try Again Later")
- Heartbeat check interval configurable; stale connections closed with code 1001

**Representative snippet (from `connection-manager.ts`):**
```typescript
export class ConnectionManager {
  private connections: Map<string, WSConnection> = new Map();

  add(id: string, ws: WebSocket): WSConnection {
    if (this.connections.size >= this.maxConnections) {
      throw new Error(`Max connections (${this.maxConnections}) reached`);
    }
    const connection: WSConnection = { id, ws, connectedAt: new Date(), lastPing: new Date() };
    this.connections.set(id, connection);
    return connection;
  }

  getStaleConnections(timeoutMs: number): WSConnection[] {
    const now = Date.now();
    return this.getAll().filter(conn => now - conn.lastPing.getTime() > timeoutMs);
  }
}
```
