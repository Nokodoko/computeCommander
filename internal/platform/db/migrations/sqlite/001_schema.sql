-- ComputeCommander SQLite schema
-- Migration 001: Initial schema

-- Runs table
CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT,
    agent_count INTEGER NOT NULL DEFAULT 0,
    coordinator_session_id TEXT,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed'))
);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    agent_name TEXT NOT NULL,
    capability TEXT NOT NULL
        CHECK (capability IN ('scout', 'builder', 'reviewer', 'lead', 'merger', 'coordinator', 'supervisor', 'monitor')),
    worktree_path TEXT,
    branch_name TEXT,
    task_id TEXT NOT NULL,
    zellij_pane TEXT,
    state TEXT NOT NULL DEFAULT 'booting'
        CHECK (state IN ('booting', 'working', 'completed', 'stalled', 'zombie')),
    pid INTEGER,
    parent_agent TEXT,
    depth INTEGER NOT NULL DEFAULT 0,
    run_id TEXT REFERENCES runs(id),
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_activity TEXT NOT NULL DEFAULT (datetime('now')),
    escalation_level INTEGER NOT NULL DEFAULT 0,
    stalled_since TEXT,
    transcript_path TEXT,
    runtime TEXT NOT NULL DEFAULT 'claude'
);

-- Events table
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT REFERENCES runs(id),
    agent_name TEXT NOT NULL,
    session_id TEXT REFERENCES sessions(id),
    event_type TEXT NOT NULL,
    tool_name TEXT,
    tool_args TEXT,
    tool_duration_ms INTEGER,
    level TEXT NOT NULL DEFAULT 'info',
    data TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Mail table
CREATE TABLE IF NOT EXISTS mail (
    id TEXT PRIMARY KEY,
    from_agent TEXT NOT NULL,
    to_agent TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'normal',
    type TEXT NOT NULL,
    thread_id TEXT,
    payload TEXT,
    read INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Metrics table
CREATE TABLE IF NOT EXISTS metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL,
    task_id TEXT NOT NULL,
    capability TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    duration_ms INTEGER,
    exit_code INTEGER,
    merge_result TEXT,
    parent_agent TEXT,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    estimated_cost_usd REAL,
    model_used TEXT,
    run_id TEXT REFERENCES runs(id)
);

-- Merge queue
CREATE TABLE IF NOT EXISTS merge_queue (
    branch_name TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    files_modified TEXT NOT NULL DEFAULT '[]',
    enqueued_at TEXT NOT NULL DEFAULT (datetime('now')),
    status TEXT NOT NULL DEFAULT 'pending',
    resolved_tier TEXT
);

-- Task groups
CREATE TABLE IF NOT EXISTS task_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS task_group_members (
    group_id TEXT NOT NULL REFERENCES task_groups(id) ON DELETE CASCADE,
    issue_id TEXT NOT NULL,
    PRIMARY KEY (group_id, issue_id)
);

-- Checkpoints
CREATE TABLE IF NOT EXISTS checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL,
    task_id TEXT NOT NULL,
    session_id TEXT REFERENCES sessions(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    progress_summary TEXT NOT NULL,
    files_modified TEXT NOT NULL DEFAULT '[]',
    current_branch TEXT NOT NULL,
    pending_work TEXT NOT NULL,
    mulch_domains TEXT NOT NULL DEFAULT '[]'
);

-- Worktrees
CREATE TABLE IF NOT EXISTS worktrees (
    path TEXT PRIMARY KEY,
    branch TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    task_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    state TEXT NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'stale', 'removed'))
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_sessions_run_id ON sessions(run_id);
CREATE INDEX IF NOT EXISTS idx_sessions_state ON sessions(state);
CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
CREATE INDEX IF NOT EXISTS idx_events_agent_name ON events(agent_name);
CREATE INDEX IF NOT EXISTS idx_mail_to_agent ON mail(to_agent);
CREATE INDEX IF NOT EXISTS idx_mail_read ON mail(read) WHERE read = 0;
CREATE INDEX IF NOT EXISTS idx_metrics_run_id ON metrics(run_id);
CREATE INDEX IF NOT EXISTS idx_merge_queue_status ON merge_queue(status);
CREATE INDEX IF NOT EXISTS idx_checkpoints_agent_name ON checkpoints(agent_name);
CREATE INDEX IF NOT EXISTS idx_checkpoints_created_at ON checkpoints(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_worktrees_agent_name ON worktrees(agent_name);
CREATE INDEX IF NOT EXISTS idx_worktrees_state ON worktrees(state);
