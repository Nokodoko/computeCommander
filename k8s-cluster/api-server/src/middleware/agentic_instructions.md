# k8s-cluster/api-server/src/middleware/ -- API Server Middleware

## Purpose
Request logging and error handling middleware for the API server. Used by the server's `fetch` handler to log requests and produce structured JSON error responses.

## Technology
- TypeScript 5.x
- `performance.now()` for request timing

## Contents
| File | Description |
|------|-------------|
| `logger.ts` | `LogEntry` interface, `logRequest()` (console.log structured output), `createLogEntry()` (builds entry from request + timing) |
| `error-handler.ts` | `createErrorResponse()` (generic JSON error), `handleNotFound()` (404 response) |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `logRequest` | `(entry: LogEntry) => void` | `void` | Logs `[timestamp] METHOD /path STATUS durationMs` |
| `createLogEntry` | `(req, status, startTime) => LogEntry` | `LogEntry` | Builds structured log entry from request metadata |
| `createErrorResponse` | `(error, statusCode?) => Response` | `Response` | Returns JSON `{ error, statusCode, timestamp }` |
| `handleNotFound` | `(path: string) => Response` | `Response` | Returns 404 with "Route not found: /path" |

## Data Types

### LogEntry (interface)
Fields: method, path, status, durationMs, timestamp

## Logging
- `logRequest()` writes to `console.log` in structured format

## Style Guide
- Pure functions, no side effects except logging
- Default status code 500 for unspecified errors
- Error message extraction: `error instanceof Error ? error.message : "Internal Server Error"`

**Representative snippet (from `logger.ts`):**
```typescript
export function logRequest(entry: LogEntry): void {
  console.log(
    `[${entry.timestamp}] ${entry.method} ${entry.path} ${entry.status} ${entry.durationMs}ms`
  );
}
```
