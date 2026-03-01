# k8s-cluster/k8s/ -- Kubernetes Manifests (Track C)

## Purpose
Kubernetes resource manifests for deploying the ComputeCommander cluster infrastructure. Managed by Kustomize. All resources deploy to the `compute-commander` namespace.

## Technology
- Kubernetes API (apps/v1, v1)
- Kustomize for resource composition

## Contents
| File | Description |
|------|-------------|
| `kustomization.yaml` | Kustomize root: lists all resources, sets namespace and common labels |
| `namespace.yaml` | Namespace `compute-commander` |
| `api-deployment.yaml` | API server Deployment: 2 replicas, resource limits (100m-500m CPU, 128-256Mi), readiness/liveness probes on /health:3000 |
| `api-service.yaml` | ClusterIP Service for API server on port 3000 |
| `websocket-deployment.yaml` | WebSocket server Deployment: 1 replica, resource limits, probes on /health:8080 |
| `websocket-service.yaml` | ClusterIP Service for WebSocket server on port 8080 |
| `haproxy-deployment.yaml` | HAProxy Deployment: 1 replica, mounts config from ConfigMap |
| `haproxy-service.yaml` | LoadBalancer Service for HAProxy (ports 80, 8080) |
| `haproxy-configmap.yaml` | ConfigMap containing haproxy.cfg content |

## Key Functions
N/A -- declarative YAML manifests.

## Data Types

### Resource Specifications
| Resource | Replicas | CPU (req/limit) | Memory (req/limit) | Probes |
|----------|----------|-----------------|---------------------|--------|
| api-server | 2 | 100m / 500m | 128Mi / 256Mi | readiness (5s init, 10s period), liveness (10s init, 15s period) |
| websocket-server | 1 | 100m / 500m | 128Mi / 256Mi | readiness + liveness on /health:8080 |
| haproxy | 1 | - | - | - |

### Environment Variables
| Deployment | Variable | Value |
|------------|----------|-------|
| api-server | `API_PORT` | `"3000"` |
| api-server | `WS_SERVER_URL` | `http://websocket-server.compute-commander.svc.cluster.local:8080` |
| websocket-server | `WS_PORT` | `"8080"` |

## Logging
N/A

## CRUD Entry Points
- **Create**: `kubectl apply -k .` deploys all resources
- **Read**: `kubectl get all -n compute-commander`
- **Delete**: `kubectl delete -k .`

## Style Guide
- Kustomize-managed: all resources listed in `kustomization.yaml`
- Common labels: `app.kubernetes.io/part-of: compute-commander`, `app.kubernetes.io/managed-by: kustomize`
- `terminationGracePeriodSeconds: 30` for graceful shutdown
- Service names match backend references in HAProxy config

**Representative snippet (from `api-deployment.yaml`):**
```yaml
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: api-server
          image: api-server:latest
          ports:
            - containerPort: 3000
          readinessProbe:
            httpGet:
              path: /health
              port: 3000
            initialDelaySeconds: 5
            periodSeconds: 10
```
