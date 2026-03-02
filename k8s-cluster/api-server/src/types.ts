export interface HealthResponse {
  status: "ok" | "error";
  timestamp: string;
  uptime: number;
}

export interface StatusResponse {
  cluster: string;
  version: string;
  services: {
    api: { status: string; replicas: number };
    websocket: { status: string };
    loadBalancer: { status: string };
  };
  timestamp: string;
}

export interface ErrorResponse {
  error: string;
  statusCode: number;
  timestamp: string;
}

export interface ServerConfig {
  port: number;
  wsServerUrl: string;
}

export const DEFAULT_CONFIG: ServerConfig = {
  port: parseInt(process.env.API_PORT || "3000", 10),
  wsServerUrl: process.env.WS_SERVER_URL || "http://localhost:8080",
};
