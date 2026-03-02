import { ConnectionManager } from "./connection-manager";
import { DEFAULT_CONFIG, type ServerConfig, type ServerStatus } from "./types";

export function createServer(config: ServerConfig = DEFAULT_CONFIG) {
  const manager = new ConnectionManager(config.maxConnections);
  const startTime = Date.now();
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null;

  const server = Bun.serve({
    port: config.port,
    fetch(req, server) {
      const url = new URL(req.url);

      // Health check endpoint
      if (url.pathname === "/health" && req.method === "GET") {
        const status: ServerStatus = {
          status: "ok",
          connections: manager.count(),
          uptime: Math.floor((Date.now() - startTime) / 1000),
          timestamp: new Date().toISOString(),
        };
        return new Response(JSON.stringify(status), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }

      // WebSocket upgrade
      if (url.pathname === "/ws") {
        const upgraded = server.upgrade(req, {
          data: { id: crypto.randomUUID() },
        });
        if (!upgraded) {
          return new Response("WebSocket upgrade failed", { status: 400 });
        }
        return undefined;
      }

      return new Response("Not Found", { status: 404 });
    },
    websocket: {
      open(ws) {
        const id = (ws.data as { id: string }).id;
        try {
          manager.add(id, ws as unknown as WebSocket);
        } catch (err) {
          ws.close(1013, "Max connections reached");
        }
      },
      message(ws, message) {
        const id = (ws.data as { id: string }).id;
        manager.updateLastPing(id);

        // Echo message back
        if (typeof message === "string") {
          try {
            const parsed = JSON.parse(message);
            if (parsed.type === "ping") {
              ws.send(JSON.stringify({ type: "pong", timestamp: Date.now() }));
              return;
            }
          } catch {
            // Not JSON, echo as-is
          }
          ws.send(message);
        }
      },
      close(ws) {
        const id = (ws.data as { id: string }).id;
        manager.remove(id);
      },
    },
  });

  // Start heartbeat checker
  heartbeatTimer = setInterval(() => {
    const stale = manager.getStaleConnections(config.heartbeatTimeout);
    for (const conn of stale) {
      try {
        conn.ws.close(1001, "Heartbeat timeout");
      } catch {
        // Connection already closed
      }
      manager.remove(conn.id);
    }
  }, config.heartbeatInterval);

  function shutdown() {
    console.log("Shutting down WebSocket server...");
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer);
      heartbeatTimer = null;
    }
    for (const conn of manager.getAll()) {
      try {
        conn.ws.close(1001, "Server shutting down");
      } catch {
        // Ignore
      }
    }
    manager.clear();
    server.stop();
    console.log("WebSocket server stopped.");
  }

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);

  console.log(`WebSocket server listening on port ${config.port}`);

  return { server, manager, shutdown };
}

// Start server if run directly
if (import.meta.main) {
  createServer();
}
