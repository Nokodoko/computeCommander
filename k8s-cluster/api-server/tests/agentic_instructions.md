# k8s-cluster/api-server/tests/ -- API Server Tests

## Purpose
Unit tests for the API server using Bun's built-in test runner. Covers server lifecycle, route responses, and error handling.

## Technology
- TypeScript 5.x
- `bun:test` (describe/it/expect)

## Contents
| File | Description |
|------|-------------|
| `server.test.ts` | Server lifecycle tests: start/stop, route dispatch, 404 handling |
| `routes/` | Route-specific test subdirectory |

## Key Functions
N/A -- test files only.

## Style Guide
- `describe`/`it` blocks matching function names
- `expect().toBe()` / `expect().toEqual()` assertions
- Server created and stopped per test suite
