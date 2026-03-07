-- ComputeCommander System-Wide schema
-- Migration 002: System-wide schema additions (projects, agent colors, traceability)

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    active INTEGER NOT NULL DEFAULT 1,
    canonical_branch TEXT NOT NULL DEFAULT 'main',
    registered_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_accessed_at TEXT NOT NULL DEFAULT (datetime('now')),
    migrated_at TEXT
);

-- Add project_id to existing tables
ALTER TABLE runs ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE sessions ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE sessions ADD COLUMN color_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN color_hex TEXT NOT NULL DEFAULT '#808080';
ALTER TABLE events ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE mail ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE metrics ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE merge_queue ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE task_groups ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE checkpoints ADD COLUMN project_id TEXT REFERENCES projects(id);
ALTER TABLE worktrees ADD COLUMN project_id TEXT REFERENCES projects(id);

-- Agent color assignments (denormalized in sessions, normalized here for history)
CREATE TABLE IF NOT EXISTS agent_colors (
    agent_name TEXT NOT NULL,
    run_id TEXT NOT NULL REFERENCES runs(id),
    color_index INTEGER NOT NULL,
    color_hex TEXT NOT NULL,
    PRIMARY KEY (agent_name, run_id)
);

-- Traceability hooks log
CREATE TABLE IF NOT EXISTS traceability_hooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL,
    session_id TEXT REFERENCES sessions(id),
    project_id TEXT REFERENCES projects(id),
    run_id TEXT REFERENCES runs(id),
    hook_type TEXT NOT NULL CHECK (hook_type IN ('pre_run', 'post_change', 'post_run')),
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    task_description TEXT,
    files_modified TEXT DEFAULT '[]',
    diff_summary TEXT,
    completion_status TEXT CHECK (completion_status IN ('success', 'failure', 'partial', 'timeout')),
    quality_status TEXT CHECK (quality_status IN ('pass', 'fail', 'skip', 'pending')),
    quality_details TEXT,
    duration_ms INTEGER
);

-- File change tracking
CREATE TABLE IF NOT EXISTS file_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL,
    session_id TEXT REFERENCES sessions(id),
    project_id TEXT REFERENCES projects(id),
    file_path TEXT NOT NULL,
    change_type TEXT NOT NULL CHECK (change_type IN ('create', 'modify', 'delete', 'rename')),
    lines_added INTEGER NOT NULL DEFAULT 0,
    lines_removed INTEGER NOT NULL DEFAULT 0,
    old_path TEXT,
    diff_hash TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- LLM call tracking
CREATE TABLE IF NOT EXISTS llm_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL,
    session_id TEXT REFERENCES sessions(id),
    project_id TEXT REFERENCES projects(id),
    runtime TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    estimated_cost_usd REAL,
    latency_ms INTEGER,
    tool_name TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for new tables and columns
CREATE INDEX IF NOT EXISTS idx_projects_path ON projects(path);
CREATE INDEX IF NOT EXISTS idx_projects_active ON projects(active) WHERE active = 1;
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
