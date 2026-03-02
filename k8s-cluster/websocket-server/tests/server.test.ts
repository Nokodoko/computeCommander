import { describe, it, expect, afterEach } from "bun:test";
import { createServer } from "../src/server";
import type { ServerConfig } from "../src/types";

let portCounter = 18080;

function nextConfig(maxConnections = 100): ServerConfig {
  return {
    port: portCounter++,
    maxConnections,
    heartbeatInterval: 60000,
    heartbeatTimeout: 30000,
  };
}

describe("WebSocket Server", () => {
  let serverInstance: ReturnType<typeof createServer> | null = null;

  afterEach(() => {
    if (serverInstance) {
      serverInstance.shutdown();
      serverInstance = null;
    }
  });

  it("should start and listen on configured port", () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    expect(serverInstance.server.port).toBe(config.port);
  });

  it("should respond with 200 on /health", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    const res = await fetch(`http://localhost:${config.port}/health`);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.status).toBe("ok");
    expect(body).toHaveProperty("connections");
    expect(body).toHaveProperty("uptime");
    expect(body).toHaveProperty("timestamp");
  });

  it("should return 404 for unknown routes", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    const res = await fetch(`http://localhost:${config.port}/unknown`);
    expect(res.status).toBe(404);
  });

  it("should accept WebSocket connections", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    const ws = new WebSocket(`ws://localhost:${config.port}/ws`);

    await new Promise<void>((resolve, reject) => {
      ws.onopen = () => resolve();
      ws.onerror = (e) => reject(e);
    });

    expect(serverInstance.manager.count()).toBe(1);
    ws.close();

    await new Promise((r) => setTimeout(r, 100));
    expect(serverInstance.manager.count()).toBe(0);
  });

  it("should echo messages back", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    const ws = new WebSocket(`ws://localhost:${config.port}/ws`);

    await new Promise<void>((resolve) => {
      ws.onopen = () => resolve();
    });

    const response = await new Promise<string>((resolve) => {
      ws.onmessage = (e) => resolve(e.data as string);
      ws.send("hello");
    });

    expect(response).toBe("hello");
    ws.close();
    await new Promise((r) => setTimeout(r, 100));
  });

  it("should respond to ping with pong", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);
    const ws = new WebSocket(`ws://localhost:${config.port}/ws`);

    await new Promise<void>((resolve) => {
      ws.onopen = () => resolve();
    });

    const response = await new Promise<string>((resolve) => {
      ws.onmessage = (e) => resolve(e.data as string);
      ws.send(JSON.stringify({ type: "ping" }));
    });

    const parsed = JSON.parse(response);
    expect(parsed.type).toBe("pong");
    expect(parsed).toHaveProperty("timestamp");
    ws.close();
    await new Promise((r) => setTimeout(r, 100));
  });

  it("should handle 100 concurrent connections", async () => {
    const config = nextConfig(1000);
    serverInstance = createServer(config);
    const connections: WebSocket[] = [];

    // Open 100 connections
    const openPromises = Array.from({ length: 100 }, () => {
      return new Promise<WebSocket>((resolve, reject) => {
        const ws = new WebSocket(`ws://localhost:${config.port}/ws`);
        ws.onopen = () => {
          connections.push(ws);
          resolve(ws);
        };
        ws.onerror = (e) => reject(e);
      });
    });

    await Promise.all(openPromises);
    await new Promise((r) => setTimeout(r, 300));

    // Verify via direct manager access
    expect(serverInstance.manager.count()).toBe(100);

    // Verify via health endpoint
    const res = await fetch(`http://localhost:${config.port}/health`);
    const body = await res.json();
    expect(body.connections).toBe(100);

    // Close all
    for (const ws of connections) {
      ws.close();
    }
    await new Promise((r) => setTimeout(r, 500));
    expect(serverInstance.manager.count()).toBe(0);
  });

  it("should track connection count accurately after add/remove", async () => {
    const config = nextConfig();
    serverInstance = createServer(config);

    const connections: WebSocket[] = [];
    for (let i = 0; i < 5; i++) {
      const ws = new WebSocket(`ws://localhost:${config.port}/ws`);
      await new Promise<void>((resolve) => {
        ws.onopen = () => resolve();
      });
      connections.push(ws);
    }
    expect(serverInstance.manager.count()).toBe(5);

    // Close 3
    for (let i = 0; i < 3; i++) {
      connections[i].close();
    }
    await new Promise((r) => setTimeout(r, 200));
    expect(serverInstance.manager.count()).toBe(2);

    // Close remaining
    for (let i = 3; i < 5; i++) {
      connections[i].close();
    }
    await new Promise((r) => setTimeout(r, 200));
    expect(serverInstance.manager.count()).toBe(0);
  });
});
