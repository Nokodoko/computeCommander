import type { ErrorResponse } from "../types";

export function createErrorResponse(
  error: unknown,
  statusCode: number = 500
): Response {
  const message =
    error instanceof Error ? error.message : "Internal Server Error";

  const body: ErrorResponse = {
    error: message,
    statusCode,
    timestamp: new Date().toISOString(),
  };

  return new Response(JSON.stringify(body), {
    status: statusCode,
    headers: { "Content-Type": "application/json" },
  });
}

export function handleNotFound(path: string): Response {
  return createErrorResponse(
    new Error(`Route not found: ${path}`),
    404
  );
}
