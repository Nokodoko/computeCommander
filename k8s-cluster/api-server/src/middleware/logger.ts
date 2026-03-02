export interface LogEntry {
  method: string;
  path: string;
  status: number;
  durationMs: number;
  timestamp: string;
}

export function logRequest(entry: LogEntry): void {
  console.log(
    `[${entry.timestamp}] ${entry.method} ${entry.path} ${entry.status} ${entry.durationMs}ms`
  );
}

export function createLogEntry(
  req: Request,
  status: number,
  startTime: number
): LogEntry {
  const url = new URL(req.url);
  return {
    method: req.method,
    path: url.pathname,
    status,
    durationMs: Math.round(performance.now() - startTime),
    timestamp: new Date().toISOString(),
  };
}
