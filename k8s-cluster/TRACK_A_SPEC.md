# Track A: WebSocket Server

## Location
/home/n0ko/Programs/ai/computeCommander/k8s-cluster/websocket-server/

## Requirements
1. TypeScript WebSocket server using Bun runtime
2. Must handle 100 concurrent connections
3. Connection tracking (count active connections)
4. Heartbeat/ping-pong for connection health
5. Graceful shutdown on SIGTERM/SIGINT
6. Health check endpoint on HTTP (GET /health returns 200)
7. Port configurable via WS_PORT env var (default 8080)

## Files to create
- src/server.ts - Main WebSocket server
- src/connection-manager.ts - Connection tracking & management
- src/types.ts - TypeScript interfaces
- tests/server.test.ts - Unit tests (Bun test runner)
- tests/connection-manager.test.ts - Unit tests
- package.json - With bun dependencies
- tsconfig.json
- Dockerfile - Multi-stage build for Bun

## Unit Tests Must Cover
- Server starts and listens on configured port
- Accepts WebSocket connections
- Tracks connection count accurately
- Handles 100 concurrent connections
- Graceful connection close
- Health endpoint returns 200
- Connection manager add/remove/count
