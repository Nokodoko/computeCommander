import { describe, it, expect, beforeEach } from "bun:test";
import { ConnectionManager } from "../src/connection-manager";

describe("ConnectionManager", () => {
  let manager: ConnectionManager;

  beforeEach(() => {
    manager = new ConnectionManager(100);
  });

  it("should start with 0 connections", () => {
    expect(manager.count()).toBe(0);
  });

  it("should add a connection", () => {
    const mockWs = {} as WebSocket;
    const conn = manager.add("test-1", mockWs);
    expect(conn.id).toBe("test-1");
    expect(manager.count()).toBe(1);
  });

  it("should remove a connection", () => {
    const mockWs = {} as WebSocket;
    manager.add("test-1", mockWs);
    expect(manager.count()).toBe(1);
    const removed = manager.remove("test-1");
    expect(removed).toBe(true);
    expect(manager.count()).toBe(0);
  });

  it("should return false when removing non-existent connection", () => {
    const removed = manager.remove("non-existent");
    expect(removed).toBe(false);
  });

  it("should get a connection by id", () => {
    const mockWs = {} as WebSocket;
    manager.add("test-1", mockWs);
    const conn = manager.get("test-1");
    expect(conn).toBeDefined();
    expect(conn!.id).toBe("test-1");
  });

  it("should return undefined for non-existent connection", () => {
    const conn = manager.get("non-existent");
    expect(conn).toBeUndefined();
  });

  it("should get all connections", () => {
    const mockWs = {} as WebSocket;
    manager.add("test-1", mockWs);
    manager.add("test-2", mockWs);
    manager.add("test-3", mockWs);
    const all = manager.getAll();
    expect(all.length).toBe(3);
  });

  it("should track connection count accurately", () => {
    const mockWs = {} as WebSocket;
    for (let i = 0; i < 50; i++) {
      manager.add(`conn-${i}`, mockWs);
    }
    expect(manager.count()).toBe(50);

    for (let i = 0; i < 25; i++) {
      manager.remove(`conn-${i}`);
    }
    expect(manager.count()).toBe(25);
  });

  it("should handle 100 concurrent connections", () => {
    const mockWs = {} as WebSocket;
    for (let i = 0; i < 100; i++) {
      manager.add(`conn-${i}`, mockWs);
    }
    expect(manager.count()).toBe(100);
  });

  it("should throw when max connections reached", () => {
    const mockWs = {} as WebSocket;
    for (let i = 0; i < 100; i++) {
      manager.add(`conn-${i}`, mockWs);
    }
    expect(() => manager.add("overflow", mockWs)).toThrow(
      "Max connections (100) reached"
    );
  });

  it("should update last ping timestamp", () => {
    const mockWs = {} as WebSocket;
    manager.add("test-1", mockWs);
    const before = manager.get("test-1")!.lastPing;

    // Small delay to ensure time difference
    const later = new Date(before.getTime() + 1000);
    manager.updateLastPing("test-1");
    const after = manager.get("test-1")!.lastPing;
    expect(after.getTime()).toBeGreaterThanOrEqual(before.getTime());
  });

  it("should find stale connections", () => {
    const mockWs = {} as WebSocket;
    const conn = manager.add("stale-1", mockWs);
    // Manually set lastPing to the past
    conn.lastPing = new Date(Date.now() - 60000);

    manager.add("fresh-1", mockWs);

    const stale = manager.getStaleConnections(30000);
    expect(stale.length).toBe(1);
    expect(stale[0].id).toBe("stale-1");
  });

  it("should clear all connections", () => {
    const mockWs = {} as WebSocket;
    manager.add("test-1", mockWs);
    manager.add("test-2", mockWs);
    manager.clear();
    expect(manager.count()).toBe(0);
  });
});
