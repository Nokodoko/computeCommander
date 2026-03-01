# k8s-cluster/websocket-server/tests/ -- WebSocket Server Tests

## Purpose
Unit tests for the WebSocket server and connection manager using Bun's built-in test runner.

## Technology
- TypeScript 5.x
- `bun:test` (describe/it/expect)

## Contents
| File | Description |
|------|-------------|
| `server.test.ts` | Server lifecycle tests: start/stop, WebSocket connection/upgrade, health endpoint, concurrent connections |
| `connection-manager.test.ts` | ConnectionManager unit tests: add/remove/count, max connection enforcement, stale detection, clear |

## Key Functions
N/A -- test files only.

## Style Guide
- `describe`/`it` blocks
- Server created per test suite with `createServer()`
- Cleanup via `shutdown()` in `afterAll`
