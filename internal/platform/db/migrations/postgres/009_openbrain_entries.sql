-- Migration 009: OpenBrain knowledge entries
-- Stores meaningful knowledge (decisions, discoveries, warnings, solutions, context)
-- for cross-session context sharing. Replaces agent lifecycle noise in the OpenBrain pane.

CREATE TABLE IF NOT EXISTS openbrain_entries (
    id BIGSERIAL PRIMARY KEY,
    project_name VARCHAR(256) NOT NULL,
    entry_type VARCHAR(32) NOT NULL CHECK (entry_type IN ('decision', 'discovery', 'warning', 'solution', 'context')),
    summary VARCHAR(512) NOT NULL,
    detail TEXT,
    runtime VARCHAR(64) NOT NULL DEFAULT 'claude',
    agent_name VARCHAR(128),
    tags TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_openbrain_dedup ON openbrain_entries(project_name, entry_type, summary);
CREATE INDEX IF NOT EXISTS idx_openbrain_project ON openbrain_entries(project_name);
CREATE INDEX IF NOT EXISTS idx_openbrain_project_type ON openbrain_entries(project_name, entry_type);
CREATE INDEX IF NOT EXISTS idx_openbrain_created ON openbrain_entries(created_at);
