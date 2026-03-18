-- Migration 008: Multi-agent tracking support

-- Add heartbeat_at for non-Claude runtimes that lack hook-based activity updates
ALTER TABLE sessions ADD COLUMN heartbeat_at TEXT;

-- Index for runtime-filtered queries (e.g., "show all pi agents")
CREATE INDEX IF NOT EXISTS idx_sessions_runtime ON sessions(runtime);

-- Index for heartbeat staleness queries
CREATE INDEX IF NOT EXISTS idx_sessions_heartbeat ON sessions(heartbeat_at)
    WHERE state IN ('booting', 'working');
