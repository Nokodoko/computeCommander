# k8s-cluster/ -- Kubernetes Cluster Infrastructure

## Purpose
Self-contained Kubernetes deployment infrastructure for ComputeCommander. Contains three tracks: (A) WebSocket server for real-time agent communication, (B) API server for HTTP endpoints, (C) HAProxy load balancer + K8s manifests. All services run on Bun (TypeScript) and deploy to the `compute-commander` namespace.

## Technology
- TypeScript with Bun runtime
- HAProxy 2.9 for load balancing
- Kubernetes manifests (Kustomize)
- Docker multi-stage builds

## Contents
| File/Dir | Description |
|----------|-------------|
| `api-server/` | Track B: Bun-based HTTP API server (health, status, connections proxy) |
| `websocket-server/` | Track A: Bun-based WebSocket server with connection management |
| `haproxy/` | Track C: HAProxy load balancer config and Dockerfile |
| `k8s/` | Track C: Kubernetes manifests (namespace, deployments, services, configmap, kustomization) |
| `tests/` | Integration tests: HAProxy config validation, K8s manifest structure validation |
| `TRACK_A_SPEC.md` | WebSocket server specification |
| `TRACK_B_SPEC.md` | API server specification |
| `TRACK_C_SPEC.md` | Load balancer + K8s manifests specification |

## Key Functions
N/A at this level -- see subdirectory `agentic_instructions.md` files.

## Data Types
N/A at this level.

## Architecture

### Service Topology
```
Client --> HAProxy (port 80/8080)
             |
             +--> api-server (2 replicas, port 3000)
             |        |
             |        +--> GET /health
             |        +--> GET /api/status
             |        +--> GET /api/connections --> websocket-server /health
             |
             +--> websocket-server (1 replica, port 8080)
                      |
                      +--> WS /ws (upgrade)
                      +--> GET /health
```

### Load Balancing
- HAProxy uses `leastconn` algorithm for both backends
- HTTP frontend on port 80, WebSocket frontend on port 8080
- Path-based routing: `/ws` and WebSocket upgrades route to ws-servers
- Health checks: GET /health with 5s interval, 3 failures to mark down
- Stats page on port 9090

## Logging
- API server: structured request logging via middleware (`[timestamp] METHOD /path STATUS durationMs`)
- WebSocket server: console.log for lifecycle events

## CRUD Entry Points
- **API**: GET /health, GET /api/status, GET /api/connections
- **WebSocket**: WS /ws (connect), GET /health
- **K8s**: `kubectl apply -k k8s/` deploys all resources

## Style Guide
- TypeScript with strict mode (`tsconfig.json`)
- Bun-native APIs (`Bun.serve`, `bun:test`)
- Functional exports (`createServer()` returning `{ server, shutdown }`)
- Graceful shutdown on SIGTERM/SIGINT
- Port configuration via environment variables
