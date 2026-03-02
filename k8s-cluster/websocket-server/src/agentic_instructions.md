# k8s-cluster/websocket-server/src/ -- WebSocket Server Source

## Purpose
TypeScript source files for the Bun WebSocket server. Contains the main server entry point, connection management, and type definitions.

## Technology
- TypeScript 5.x
- Bun-native `Bun.serve` with WebSocket support

## Contents
| File | Description |
|------|-------------|
| `server.ts` | Main entry point: `createServer()` with HTTP health endpoint + WS upgrade at `/ws`, heartbeat timer, graceful shutdown |
| `connection-manager.ts` | `ConnectionManager` class: Map-based connection tracking with max limit, stale detection, ping tracking |
| `types.ts` | `WSConnection`, `ServerConfig`, `ServerStatus` interfaces and `DEFAULT_CONFIG` constant |

## Key Functions
See `k8s-cluster/websocket-server/agentic_instructions.md` for full function listing.

## Style Guide
- `import.meta.main` guard for direct execution
- Exports `createServer()` for testing access to server, manager, and shutdown
