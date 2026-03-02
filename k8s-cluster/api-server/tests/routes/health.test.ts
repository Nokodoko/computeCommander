import { describe, it, expect } from "bun:test";
import { handleHealth } from "../../src/routes/health";

describe("Health Route", () => {
  it("should return a Response with status 200", () => {
    const res = handleHealth();
    expect(res.status).toBe(200);
  });

  it("should return JSON with correct shape", async () => {
    const res = handleHealth();
    const body = await res.json();

    expect(body.status).toBe("ok");
    expect(typeof body.timestamp).toBe("string");
    expect(typeof body.uptime).toBe("number");
    expect(body.uptime).toBeGreaterThanOrEqual(0);
  });

  it("should return valid ISO timestamp", async () => {
    const res = handleHealth();
    const body = await res.json();

    const date = new Date(body.timestamp);
    expect(date.toISOString()).toBe(body.timestamp);
  });

  it("should return application/json content-type", () => {
    const res = handleHealth();
    expect(res.headers.get("content-type")).toBe("application/json");
  });
});
