import { describe, it, expect, afterEach } from "bun:test";
import { createServer } from "../src/server";
import type { ServerConfig } from "../src/types";

const TEST_PORT = 13000;
const testConfig: ServerConfig = {
  port: TEST_PORT,
  wsServerUrl: "http://localhost:99999", // intentionally unreachable for isolation
};

describe("API Server", () => {
  let serverInstance: ReturnType<typeof createServer> | null = null;

  afterEach(() => {
    if (serverInstance) {
      serverInstance.shutdown();
      serverInstance = null;
    }
  });

  it("should start and listen on configured port", () => {
    serverInstance = createServer(testConfig);
    expect(serverInstance.server.port).toBe(TEST_PORT);
  });

  it("GET /health should return 200 with correct shape", async () => {
    serverInstance = createServer(testConfig);
    const res = await fetch(`http://localhost:${TEST_PORT}/health`);
    expect(res.status).toBe(200);

    const body = await res.json();
    expect(body.status).toBe("ok");
    expect(body).toHaveProperty("timestamp");
    expect(body).toHaveProperty("uptime");
    expect(typeof body.timestamp).toBe("string");
    expect(typeof body.uptime).toBe("number");
  });

  it("GET /api/status should return 200 with cluster info", async () => {
    serverInstance = createServer(testConfig);
    const res = await fetch(`http://localhost:${TEST_PORT}/api/status`);
    expect(res.status).toBe(200);

    const body = await res.json();
    expect(body.cluster).toBe("compute-commander");
    expect(body.version).toBe("1.0.0");
    expect(body.services).toHaveProperty("api");
    expect(body.services).toHaveProperty("websocket");
    expect(body.services).toHaveProperty("loadBalancer");
    expect(body.services.api.replicas).toBe(2);
  });

  it("should return 404 for unknown routes", async () => {
    serverInstance = createServer(testConfig);
    const res = await fetch(`http://localhost:${TEST_PORT}/nonexistent`);
    expect(res.status).toBe(404);

    const body = await res.json();
    expect(body).toHaveProperty("error");
    expect(body.statusCode).toBe(404);
  });

  it("GET /api/connections should return 503 when WS server unreachable", async () => {
    serverInstance = createServer(testConfig);
    const res = await fetch(`http://localhost:${TEST_PORT}/api/connections`);
    // Should return 503 since WS server is unreachable
    expect(res.status).toBe(503);

    const body = await res.json();
    expect(body).toHaveProperty("error");
    expect(body.connections).toBe(-1);
  });

  it("should handle multiple rapid requests", async () => {
    serverInstance = createServer(testConfig);
    const requests = Array.from({ length: 20 }, () =>
      fetch(`http://localhost:${TEST_PORT}/health`)
    );

    const responses = await Promise.all(requests);
    for (const res of responses) {
      expect(res.status).toBe(200);
    }
  });

  it("should return JSON content-type headers", async () => {
    serverInstance = createServer(testConfig);
    const res = await fetch(`http://localhost:${TEST_PORT}/health`);
    expect(res.headers.get("content-type")).toBe("application/json");
  });
});
