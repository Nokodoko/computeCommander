import { handleHealth } from "./routes/health";
import { handleStatus, handleConnections } from "./routes/status";
import { handleNotFound, createErrorResponse } from "./middleware/error-handler";
import { logRequest, createLogEntry } from "./middleware/logger";
import { DEFAULT_CONFIG, type ServerConfig } from "./types";

export function createServer(config: ServerConfig = DEFAULT_CONFIG) {
  const server = Bun.serve({
    port: config.port,
    async fetch(req) {
      const startTime = performance.now();
      let response: Response;

      try {
        const url = new URL(req.url);
        const path = url.pathname;

        switch (true) {
          case path === "/health" && req.method === "GET":
            response = handleHealth();
            break;
          case path === "/api/status" && req.method === "GET":
            response = handleStatus();
            break;
          case path === "/api/connections" && req.method === "GET":
            response = await handleConnections(config.wsServerUrl);
            break;
          default:
            response = handleNotFound(path);
        }
      } catch (err) {
        response = createErrorResponse(err);
      }

      const logEntry = createLogEntry(req, response.status, startTime);
      logRequest(logEntry);

      return response;
    },
  });

  function shutdown() {
    console.log("Shutting down API server...");
    server.stop();
    console.log("API server stopped.");
  }

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);

  console.log(`API server listening on port ${config.port}`);

  return { server, shutdown };
}

if (import.meta.main) {
  createServer();
}
