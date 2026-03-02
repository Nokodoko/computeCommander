export interface WSConnection {
  id: string;
  ws: WebSocket;
  connectedAt: Date;
  lastPing: Date;
}

export interface ServerConfig {
  port: number;
  maxConnections: number;
  heartbeatInterval: number;
  heartbeatTimeout: number;
}

export interface ServerStatus {
  status: "ok" | "error";
  connections: number;
  uptime: number;
  timestamp: string;
}

export const DEFAULT_CONFIG: ServerConfig = {
  port: parseInt(process.env.WS_PORT || "8080", 10),
  maxConnections: 1000,
  heartbeatInterval: 30000,
  heartbeatTimeout: 10000,
};
