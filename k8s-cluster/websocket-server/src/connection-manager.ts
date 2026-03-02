import type { WSConnection } from "./types";

export class ConnectionManager {
  private connections: Map<string, WSConnection> = new Map();
  private maxConnections: number;

  constructor(maxConnections: number = 1000) {
    this.maxConnections = maxConnections;
  }

  add(id: string, ws: WebSocket): WSConnection {
    if (this.connections.size >= this.maxConnections) {
      throw new Error(
        `Max connections (${this.maxConnections}) reached`
      );
    }

    const connection: WSConnection = {
      id,
      ws,
      connectedAt: new Date(),
      lastPing: new Date(),
    };

    this.connections.set(id, connection);
    return connection;
  }

  remove(id: string): boolean {
    return this.connections.delete(id);
  }

  get(id: string): WSConnection | undefined {
    return this.connections.get(id);
  }

  count(): number {
    return this.connections.size;
  }

  getAll(): WSConnection[] {
    return Array.from(this.connections.values());
  }

  updateLastPing(id: string): void {
    const conn = this.connections.get(id);
    if (conn) {
      conn.lastPing = new Date();
    }
  }

  getStaleConnections(timeoutMs: number): WSConnection[] {
    const now = Date.now();
    return this.getAll().filter(
      (conn) => now - conn.lastPing.getTime() > timeoutMs
    );
  }

  clear(): void {
    this.connections.clear();
  }
}
