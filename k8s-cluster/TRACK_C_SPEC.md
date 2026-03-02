# Track C: Load Balancer + K8s Manifests

## Location
/home/n0ko/Programs/ai/computeCommander/k8s-cluster/

## Requirements

### HAProxy Load Balancer
- Least-connection algorithm (leastconn)
- Frontend binds on port 80 for HTTP, port 8080 for WebSocket
- Backend: api-servers (2 replicas on port 3000)
- Backend: ws-servers (1 replica on port 8080)
- Health checks on backends
- Stats page on port 9090
- Config file: haproxy/haproxy.cfg

### K8s Manifests (in k8s/ subdirectory)
All manifests must pass: kubectl apply --dry-run=client -f <file>

1. **namespace.yaml** - Namespace: compute-commander
2. **websocket-deployment.yaml** - WebSocket server, 1 replica, resource limits, readiness/liveness probes
3. **websocket-service.yaml** - ClusterIP service for WS server on port 8080
4. **api-deployment.yaml** - API server, 2 replicas, resource limits, readiness/liveness probes
5. **api-service.yaml** - ClusterIP service for API on port 3000
6. **haproxy-deployment.yaml** - HAProxy, 1 replica, mounts config from ConfigMap
7. **haproxy-service.yaml** - LoadBalancer type service
8. **haproxy-configmap.yaml** - ConfigMap with haproxy.cfg content
9. **kustomization.yaml** - Kustomize file listing all resources

### Files to create
- haproxy/haproxy.cfg
- haproxy/Dockerfile
- k8s/namespace.yaml
- k8s/websocket-deployment.yaml
- k8s/websocket-service.yaml
- k8s/api-deployment.yaml
- k8s/api-service.yaml
- k8s/haproxy-deployment.yaml
- k8s/haproxy-service.yaml
- k8s/haproxy-configmap.yaml
- k8s/kustomization.yaml
- tests/haproxy.test.ts - Unit tests validating HAProxy config parsing
- tests/k8s-validate.test.ts - Tests that validate K8s manifest structure
