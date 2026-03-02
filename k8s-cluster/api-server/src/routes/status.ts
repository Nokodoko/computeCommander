import type { StatusResponse } from "../types";

export function handleStatus(): Response {
  const body: StatusResponse = {
    cluster: "compute-commander",
    version: "1.0.0",
    services: {
      api: { status: "running", replicas: 2 },
      websocket: { status: "running" },
      loadBalancer: { status: "running" },
    },
    timestamp: new Date().toISOString(),
  };

  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

export async function handleConnections(wsServerUrl: string): Promise<Response> {
  try {
    const res = await fetch(`${wsServerUrl}/health`);
    const data = await res.json();
    return new Response(
      JSON.stringify({
        connections: data.connections ?? 0,
        source: wsServerUrl,
        timestamp: new Date().toISOString(),
      }),
      {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }
    );
  } catch (err) {
    return new Response(
      JSON.stringify({
        connections: -1,
        error: "WebSocket server unreachable",
        timestamp: new Date().toISOString(),
      }),
      {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }
    );
  }
}
