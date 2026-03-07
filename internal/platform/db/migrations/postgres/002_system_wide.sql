-- ComputeCommander System-Wide Postgres schema
-- Migration 002: System-wide schema additions (projects, agent colors, traceability)

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id VARCHAR(32) PRIMARY KEY,
    name VARCHAR(256) NOT NULL,
    path TEXT NOT NULL UNIQUE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    canonical_branch VARCHAR(128) NOT NULL DEFAULT 'main',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    migrated_at TIMESTAMPTZ
);

-- Add project_id to existing tables
ALTER TABLE runs ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS color_index INT NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS color_hex VARCHAR(7) NOT NULL DEFAULT '#808080';
ALTER TABLE events ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE mail ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE metrics ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE merge_queue ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE task_groups ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE checkpoints ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);
ALTER TABLE worktrees ADD COLUMN IF NOT EXISTS project_id VARCHAR(32) REFERENCES projects(id);

-- Agent color assignments (denormalized in sessions, normalized here for history)
CREATE TABLE IF NOT EXISTS agent_colors (
    agent_name VARCHAR(128) NOT NULL,
    run_id VARCHAR(64) NOT NULL REFERENCES runs(id),
    color_index INT NOT NULL,
    color_hex VARCHAR(7) NOT NULL,
    PRIMARY KEY (agent_name, run_id)
);

-- Traceability hooks log
CREATE TABLE IF NOT EXISTS traceability_hooks (
    id BIGSERIAL PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    session_id VARCHAR(64) REFERENCES sessions(id),
    project_id VARCHAR(32) REFERENCES projects(id),
    run_id VARCHAR(64) REFERENCES runs(id),
    hook_type VARCHAR(16) NOT NULL CHECK (hook_type IN ('pre_run', 'post_change', 'post_run')),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    task_description TEXT,
    files_modified JSONB DEFAULT '[]',
    diff_summary TEXT,
    completion_status VARCHAR(16) CHECK (completion_status IN ('success', 'failure', 'partial', 'timeout')),
    quality_status VARCHAR(16) CHECK (quality_status IN ('pass', 'fail', 'skip', 'pending')),
    quality_details JSONB,
    duration_ms BIGINT
);

-- File change tracking
CREATE TABLE IF NOT EXISTS file_changes (
    id BIGSERIAL PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    session_id VARCHAR(64) REFERENCES sessions(id),
    project_id VARCHAR(32) REFERENCES projects(id),
    file_path TEXT NOT NULL,
    change_type VARCHAR(16) NOT NULL CHECK (change_type IN ('create', 'modify', 'delete', 'rename')),
    lines_added INT NOT NULL DEFAULT 0,
    lines_removed INT NOT NULL DEFAULT 0,
    old_path TEXT,
    diff_hash VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- LLM call tracking
CREATE TABLE IF NOT EXISTS llm_calls (
    id BIGSERIAL PRIMARY KEY,
    agent_name VARCHAR(128) NOT NULL,
    session_id VARCHAR(64) REFERENCES sessions(id),
    project_id VARCHAR(32) REFERENCES projects(id),
    runtime VARCHAR(32) NOT NULL,
    model VARCHAR(64) NOT NULL,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd DECIMAL(10, 4),
    latency_ms BIGINT,
    tool_name VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for new tables and columns
CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(path);
CREATE INDEX IF NOT EXISTS idx_projects_active ON projects(active) WHERE active = TRUE;
CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_events_project_id ON events(project_id);
CREATE INDEX IF NOT EXISTS idx_mail_project_id ON mail(project_id);
CREATE INDEX IF NOT EXISTS idx_traceability_hooks_session ON traceability_hooks(session_id);
CREATE INDEX IF NOT EXISTS idx_traceability_hooks_project ON traceability_hooks(project_id);
CREATE INDEX IF NOT EXISTS idx_traceability_hooks_type ON traceability_hooks(hook_type);
CREATE INDEX IF NOT EXISTS idx_file_changes_session ON file_changes(session_id);
CREATE INDEX IF NOT EXISTS idx_file_changes_project ON file_changes(project_id);
CREATE INDEX IF NOT EXISTS idx_file_changes_path ON file_changes(file_path);
CREATE INDEX IF NOT EXISTS idx_llm_calls_session ON llm_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_llm_calls_project ON llm_calls(project_id);
CREATE INDEX IF NOT EXISTS idx_llm_calls_model ON llm_calls(model);
