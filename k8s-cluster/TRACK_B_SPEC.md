# Track B: API Server

## Location
/home/n0ko/Programs/ai/computeCommander/k8s-cluster/api-server/

## Requirements
1. TypeScript API server using Bun runtime
2. GET /health returns { status: "ok", timestamp: <iso>, uptime: <seconds> } with 200
3. GET /api/status returns cluster status info
4. GET /api/connections proxies to WebSocket server for connection count
5. Structured JSON responses
6. Request logging middleware
7. Error handling middleware
8. Port configurable via API_PORT env var (default 3000)
9. Will run as 2 replicas in K8s (stateless design)

## Files to create
- src/server.ts - Main HTTP server
- src/routes/health.ts - Health endpoint handler
- src/routes/status.ts - Status endpoint handler
- src/middleware/logger.ts - Request logging
- src/middleware/error-handler.ts - Error handling
- src/types.ts - TypeScript interfaces
- tests/server.test.ts - Unit tests (Bun test runner)
- tests/routes/health.test.ts - Health endpoint tests
- package.json
- tsconfig.json
- Dockerfile - Multi-stage build for Bun

## Unit Tests Must Cover
- Server starts and listens
- GET /health returns 200 with correct JSON shape
- GET /api/status returns 200
- 404 for unknown routes
- Error handling middleware catches errors
- Logger middleware logs requests
