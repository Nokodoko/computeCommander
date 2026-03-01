# k8s-cluster/api-server/src/ -- API Server Source

## Purpose
TypeScript source files for the Bun HTTP API server. Contains the main server entry point, route handlers, middleware, and type definitions.

## Technology
- TypeScript 5.x
- Bun-native `Bun.serve` API

## Contents
| File | Description |
|------|-------------|
| `server.ts` | Main entry point: `createServer()` with route dispatch (health, status, connections, 404), graceful shutdown |
| `types.ts` | `HealthResponse`, `StatusResponse`, `ErrorResponse`, `ServerConfig` interfaces and `DEFAULT_CONFIG` constant |
| `routes/` | Route handler subdirectory |
| `middleware/` | Middleware subdirectory |

## Key Functions
See `k8s-cluster/api-server/agentic_instructions.md` for full function listing.

## Data Types
See `k8s-cluster/api-server/agentic_instructions.md` for full type definitions.

## Style Guide
- `import.meta.main` guard for direct execution
- Exports `createServer()` for testing
- Route matching via `switch (true)` pattern
