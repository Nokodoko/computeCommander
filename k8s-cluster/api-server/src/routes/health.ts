import type { HealthResponse } from "../types";

const startTime = Date.now();

export function handleHealth(): Response {
  const body: HealthResponse = {
    status: "ok",
    timestamp: new Date().toISOString(),
    uptime: Math.floor((Date.now() - startTime) / 1000),
  };

  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
