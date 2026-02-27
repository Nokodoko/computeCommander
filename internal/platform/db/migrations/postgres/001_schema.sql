-- ComputeCommander Postgres schema
-- Migration 001: Initial schema

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Runs table
CREATE TABLE IF NOT EXISTS runs (
    id VARCHAR(64) PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    agent_count INT NOT NULL DEFAULT 0,
    coordinator_session_id VARCHAR(64),
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at DESC);

-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(64) PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    capability VARCHAR(32) NOT NULL
        CHECK (capability IN ('scout', 'builder', 'reviewer', 'lead', 'merger', 'coordinator', 'supervisor', 'monitor')),
    worktree_path TEXT,
    branch_name VARCHAR(256),
    task_id VARCHAR(128) NOT NULL,
    zellij_pane VARCHAR(64),
    state VARCHAR(20) NOT NULL DEFAULT 'booting'
        CHECK (state IN ('booting', 'working', 'completed', 'stalled', 'zombie')),
    pid INT,
    parent_agent VARCHAR(128),
    depth INT NOT NULL DEFAULT 0,
    run_id VARCHAR(64) REFERENCES runs(id),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_activity TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    escalation_level INT NOT NULL DEFAULT 0,
    stalled_since TIMESTAMPTZ,
    transcript_path TEXT,
    runtime VARCHAR(32) NOT NULL DEFAULT 'claude'
);

CREATE INDEX IF NOT EXISTS idx_sessions_run_id ON sessions(run_id);
CREATE INDEX IF NOT EXISTS idx_sessions_state ON sessions(state);
CREATE INDEX IF NOT EXISTS idx_sessions_agent_name ON sessions(agent_name);
CREATE INDEX IF NOT EXISTS idx_sessions_capability ON sessions(capability);

-- Events table
CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    run_id VARCHAR(64) REFERENCES runs(id),
    agent_name VARCHAR(128) NOT NULL,
    session_id VARCHAR(64) REFERENCES sessions(id),
    event_type VARCHAR(32) NOT NULL
        CHECK (event_type IN ('tool_start', 'tool_end', 'session_start', 'session_end',
                              'mail_sent', 'mail_received', 'spawn', 'error', 'custom')),
    tool_name VARCHAR(64),
    tool_args JSONB,
    tool_duration_ms INT,
    level VARCHAR(10) NOT NULL DEFAULT 'info'
        CHECK (level IN ('debug', 'info', 'warn', 'error')),
    data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
CREATE INDEX IF NOT EXISTS idx_events_agent_name ON events(agent_name);
CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC);

-- Mail table
CREATE TABLE IF NOT EXISTS mail (
    id VARCHAR(32) PRIMARY KEY,
    from_agent VARCHAR(128) NOT NULL,
    to_agent VARCHAR(128) NOT NULL,
    subject VARCHAR(256) NOT NULL,
    body TEXT NOT NULL,
    priority VARCHAR(10) NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    type VARCHAR(20) NOT NULL
        CHECK (type IN ('status', 'question', 'result', 'error',
                       'worker_done', 'merge_ready', 'merged', 'merge_failed',
                       'escalation', 'health_check', 'dispatch', 'assign')),
    thread_id VARCHAR(32),
    payload JSONB,
    read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mail_to_agent ON mail(to_agent);
CREATE INDEX IF NOT EXISTS idx_mail_created_at ON mail(created_at DESC);

-- Metrics table
CREATE TABLE IF NOT EXISTS metrics (
    id BIGSERIAL PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    task_id VARCHAR(128) NOT NULL,
    capability VARCHAR(32) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    duration_ms BIGINT,
    exit_code INT,
    merge_result VARCHAR(20),
    parent_agent VARCHAR(128),
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd DECIMAL(10, 4),
    model_used VARCHAR(64),
    run_id VARCHAR(64) REFERENCES runs(id)
);

CREATE INDEX IF NOT EXISTS idx_metrics_run_id ON metrics(run_id);
CREATE INDEX IF NOT EXISTS idx_metrics_agent_name ON metrics(agent_name);
CREATE INDEX IF NOT EXISTS idx_metrics_capability ON metrics(capability);

-- Merge queue table
CREATE TABLE IF NOT EXISTS merge_queue (
    branch_name VARCHAR(256) PRIMARY KEY,
    task_id VARCHAR(128) NOT NULL,
    agent_name VARCHAR(128) NOT NULL,
    files_modified TEXT[] NOT NULL DEFAULT '{}',
    enqueued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'merging', 'merged', 'conflict', 'failed')),
    resolved_tier VARCHAR(20)
        CHECK (resolved_tier IN ('clean-merge', 'auto-resolve', 'ai-resolve', 'reimagine'))
);

CREATE INDEX IF NOT EXISTS idx_merge_queue_status ON merge_queue(status);
CREATE INDEX IF NOT EXISTS idx_merge_queue_enqueued_at ON merge_queue(enqueued_at);

-- Task groups
CREATE TABLE IF NOT EXISTS task_groups (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(256) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS task_group_members (
    group_id VARCHAR(32) REFERENCES task_groups(id) ON DELETE CASCADE,
    issue_id VARCHAR(128) NOT NULL,
    PRIMARY KEY (group_id, issue_id)
);

-- Checkpoints for session recovery
CREATE TABLE IF NOT EXISTS checkpoints (
    id BIGSERIAL PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    task_id VARCHAR(128) NOT NULL,
    session_id VARCHAR(64) REFERENCES sessions(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    progress_summary TEXT NOT NULL,
    files_modified TEXT[] NOT NULL DEFAULT '{}',
    current_branch VARCHAR(256) NOT NULL,
    pending_work TEXT NOT NULL,
    mulch_domains TEXT[] NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_agent_name ON checkpoints(agent_name);
CREATE INDEX IF NOT EXISTS idx_checkpoints_created_at ON checkpoints(created_at DESC);

-- Worktrees table
CREATE TABLE IF NOT EXISTS worktrees (
    path TEXT PRIMARY KEY,
    branch VARCHAR(256) NOT NULL,
    agent_name VARCHAR(128) NOT NULL,
    task_id VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    state VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'stale', 'removed'))
);

CREATE INDEX IF NOT EXISTS idx_worktrees_agent_name ON worktrees(agent_name);
CREATE INDEX IF NOT EXISTS idx_worktrees_state ON worktrees(state);
