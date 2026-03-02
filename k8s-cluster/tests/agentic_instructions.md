# k8s-cluster/tests/ -- Cluster Integration Tests

## Purpose
Integration-level tests that validate HAProxy configuration parsing and Kubernetes manifest structure. Uses Bun's test runner.

## Technology
- TypeScript 5.x
- `bun:test` (describe/it/expect)

## Contents
| File | Description |
|------|-------------|
| `haproxy.test.ts` | Tests that parse and validate HAProxy config: frontend/backend existence, leastconn algorithm, health checks, port bindings |
| `k8s-validate.test.ts` | Tests that validate K8s manifest structure: required fields, labels, resource limits, probe configuration, namespace consistency |
| `package.json` | Test dependencies |

## Key Functions
N/A -- test files only.

## Style Guide
- Integration-level: validates config files, not running services
- File I/O to read config files for parsing assertions
- Structure validation via YAML parsing and field assertions
