# k8s-cluster/haproxy/ -- HAProxy Load Balancer (Track C)

## Purpose
HAProxy configuration and Docker image for load balancing between the API server (2 replicas) and WebSocket server (1 replica). Uses least-connection algorithm, health checks, WebSocket-aware routing, and a stats page.

## Technology
- HAProxy 2.9 (Alpine-based Docker image)
- HAProxy configuration language

## Contents
| File | Description |
|------|-------------|
| `haproxy.cfg` | Full HAProxy config: global settings, defaults, stats listener (port 9090), HTTP frontend (port 80), WS frontend (port 8080), api_servers backend (leastconn, 2 servers), ws_servers backend (leastconn, 1 server) |
| `Dockerfile` | `FROM haproxy:2.9-alpine`, copies config, exposes ports 80/8080/9090 |

## Key Functions
N/A -- declarative configuration.

## Data Types
N/A

## Architecture

### Frontends
| Frontend | Bind | Default Backend | ACL Rules |
|----------|------|-----------------|-----------|
| `http_front` | `*:80` | `api_servers` | Routes `/ws` and WebSocket upgrades to `ws_servers` |
| `ws_front` | `*:8080` | `ws_servers` | Direct WebSocket traffic |

### Backends
| Backend | Algorithm | Servers | Health Check |
|---------|-----------|---------|-------------|
| `api_servers` | leastconn | 2 API server pods (port 3000) | GET /health, 5s interval, 3 failures |
| `ws_servers` | leastconn | 1 WS server pod (port 8080) | GET /health, 5s interval, 3 failures |

### Timeouts
- connect: 5s, client: 50s, server: 50s, tunnel: 1 hour (for long-lived WebSocket connections)

## Logging
- `log stdout format raw local0` (global)
- HTTP logging enabled (`option httplog`)

## CRUD Entry Points
- **Read**: Stats page at `/stats` on port 9090
- **Proxy**: HTTP traffic on port 80, WebSocket on port 8080

## Style Guide
- HAProxy 2.x config syntax
- Server names reference K8s DNS: `{service}.{namespace}.svc.cluster.local:{port}`
- ACL-based routing for WebSocket upgrade detection
- Health checks with `inter 5s fall 3 rise 2`

**Representative snippet (from `haproxy.cfg`):**
```
backend api_servers
    mode http
    balance leastconn
    option httpchk GET /health
    http-check expect status 200
    server api-1 api-server-0.api-server.compute-commander.svc.cluster.local:3000 check inter 5s fall 3 rise 2
    server api-2 api-server-1.api-server.compute-commander.svc.cluster.local:3000 check inter 5s fall 3 rise 2
```
