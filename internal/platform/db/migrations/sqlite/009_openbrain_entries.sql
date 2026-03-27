-- Migration 009: OpenBrain knowledge entries
-- Stores meaningful knowledge (decisions, discoveries, warnings, solutions, context)
-- for cross-session context sharing. Replaces agent lifecycle noise in the OpenBrain pane.

CREATE TABLE IF NOT EXISTS openbrain_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_name TEXT NOT NULL,
    entry_type TEXT NOT NULL CHECK (entry_type IN ('decision', 'discovery', 'warning', 'solution', 'context')),
    summary TEXT NOT NULL,
    detail TEXT,
    runtime TEXT NOT NULL DEFAULT 'claude',
    agent_name TEXT,
    tags TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_openbrain_dedup ON openbrain_entries(project_name, entry_type, summary);
CREATE INDEX IF NOT EXISTS idx_openbrain_project ON openbrain_entries(project_name);
CREATE INDEX IF NOT EXISTS idx_openbrain_project_type ON openbrain_entries(project_name, entry_type);
CREATE INDEX IF NOT EXISTS idx_openbrain_created ON openbrain_entries(created_at);
