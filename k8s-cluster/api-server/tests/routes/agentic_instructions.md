# k8s-cluster/api-server/tests/routes/ -- API Route Tests

## Purpose
Unit tests for individual route handlers using Bun's built-in test runner.

## Technology
- TypeScript 5.x
- `bun:test` (describe/it/expect)

## Contents
| File | Description |
|------|-------------|
| `health.test.ts` | Tests for health endpoint: 200 response, correct JSON shape (status, timestamp, uptime) |

## Key Functions
N/A -- test files only.

## Style Guide
- Direct function invocation (no HTTP server needed for route handler tests)
- Response body parsed via `response.json()`
- Assertions on JSON structure and status codes
