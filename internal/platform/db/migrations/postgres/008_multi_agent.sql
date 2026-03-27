-- Migration 008: Multi-agent tracking support

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_sessions_runtime ON sessions(runtime);
CREATE INDEX IF NOT EXISTS idx_sessions_heartbeat ON sessions(heartbeat_at)
    WHERE state IN ('booting', 'working');
