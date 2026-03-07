-- ComputeCommander Agentic Foundation SQLite schema
-- Migration 002: Agentic foundation tables (traceability, blocks, blueprints, gates, isolation, holdouts)

-- Trace events table: records every action in a causal chain
CREATE TABLE IF NOT EXISTS trace_events (
    id TEXT PRIMARY KEY,                                    -- "trc-" + 16 hex chars
    trace_id TEXT NOT NULL,                                 -- Root trace ID for the causal chain
    parent_id TEXT,                                         -- Parent TraceEvent ID (NULL for root)
    span_id TEXT NOT NULL,                                  -- Unique span within the trace

    -- Actor
    agent_id TEXT NOT NULL,                                 -- Session ID of acting agent
    agent_name TEXT NOT NULL,                               -- Human-readable agent name
    capability TEXT NOT NULL,                               -- scout|builder|reviewer|lead|merger|coordinator|supervisor|monitor

    -- Action
    event_type TEXT NOT NULL
        CHECK (event_type IN ('tool_call', 'agent_spawn', 'agent_stop', 'mail_send',
                              'mail_receive', 'merge_attempt', 'merge_complete',
                              'gate_check', 'block_check', 'blueprint_start',
                              'blueprint_complete', 'holdout_verify', 'context_inject', 'error')),
    tool_name TEXT,                                         -- Bash|Read|Write|Edit|Glob|Grep|Task|NULL
    tool_input_hash TEXT,                                   -- SHA-256 of tool input
    tool_result_code INTEGER,                               -- 0=success, >0=error
    tool_result_summary TEXT,                               -- First 500 chars of result

    -- Safety
    block_rule_id TEXT,                                     -- Block rule ID if one fired
    block_disposition TEXT
        CHECK (block_disposition IN ('allowed', 'blocked', 'overridden', 'warned') OR block_disposition IS NULL),

    -- Context
    blueprint_id TEXT,                                      -- Blueprint being executed
    file_paths TEXT NOT NULL DEFAULT '[]',                  -- JSON array of file paths
    duration_ms INTEGER NOT NULL DEFAULT 0,                 -- Wall-clock duration

    -- Metadata
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    session_id TEXT,                                        -- cmdr session ID
    run_id TEXT                                             -- Orchestration run ID
);

CREATE INDEX IF NOT EXISTS idx_trace_events_trace_id ON trace_events(trace_id);
CREATE INDEX IF NOT EXISTS idx_trace_events_parent_id ON trace_events(parent_id);
CREATE INDEX IF NOT EXISTS idx_trace_events_agent_id ON trace_events(agent_id);
CREATE INDEX IF NOT EXISTS idx_trace_events_agent_name ON trace_events(agent_name);
CREATE INDEX IF NOT EXISTS idx_trace_events_event_type ON trace_events(event_type);
CREATE INDEX IF NOT EXISTS idx_trace_events_created_at ON trace_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_trace_events_run_id ON trace_events(run_id);
CREATE INDEX IF NOT EXISTS idx_trace_events_blueprint_id ON trace_events(blueprint_id);

-- Block rules table: persisted block rule state (enabled/disabled, rate counters)
CREATE TABLE IF NOT EXISTS block_rules (
    id TEXT PRIMARY KEY,                                    -- Rule identifier (e.g., "no-force-push")
    description TEXT NOT NULL,
    tool TEXT NOT NULL,                                     -- Tool name to match
    match_config TEXT NOT NULL DEFAULT '{}',                -- JSON: match conditions
    action TEXT NOT NULL DEFAULT 'block'
        CHECK (action IN ('block', 'warn')),
    message TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'high'
        CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    override_type TEXT DEFAULT 'none'
        CHECK (override_type IN ('grant', 'none')),
    enabled INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT 'default'                  -- 'default' or 'custom'
        CHECK (source IN ('default', 'custom')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_block_rules_tool ON block_rules(tool);
CREATE INDEX IF NOT EXISTS idx_block_rules_enabled ON block_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_block_rules_severity ON block_rules(severity);

-- Block rate limits: sliding window counters for rate-limited rules
CREATE TABLE IF NOT EXISTS block_rate_limits (
    rule_id TEXT NOT NULL REFERENCES block_rules(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    window_start TEXT NOT NULL,                             -- ISO 8601 window start
    count INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (rule_id, agent_id, window_start)
);

-- Blueprints table: structured task definitions
CREATE TABLE IF NOT EXISTS blueprints (
    id TEXT PRIMARY KEY,                                    -- "bp-" + 8 hex chars
    version INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL,
    agent TEXT NOT NULL,                                    -- Target agent type
    capability TEXT NOT NULL,                               -- Required capability

    -- Context & IO
    context_grants TEXT NOT NULL DEFAULT '[]',              -- JSON array of ContextGrant
    inputs TEXT NOT NULL DEFAULT '{}',                      -- JSON: BlueprintInputs
    outputs TEXT NOT NULL DEFAULT '{}',                     -- JSON: BlueprintOutputs
    verify_steps TEXT NOT NULL DEFAULT '[]',                -- JSON array of VerifyStep
    gates TEXT NOT NULL DEFAULT '[]',                       -- JSON array of gate names
    depends_on TEXT NOT NULL DEFAULT '[]',                  -- JSON array of blueprint IDs

    -- Execution config
    retry_limit INTEGER NOT NULL DEFAULT 3,
    timeout TEXT NOT NULL DEFAULT '30m',

    -- Execution state
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'passed', 'failed', 'blocked', 'cancelled')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,

    -- Timestamps
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_blueprints_status ON blueprints(status);
CREATE INDEX IF NOT EXISTS idx_blueprints_agent ON blueprints(agent);
CREATE INDEX IF NOT EXISTS idx_blueprints_capability ON blueprints(capability);

-- Blueprint runs table: execution history for blueprints
CREATE TABLE IF NOT EXISTS blueprint_runs (
    id TEXT PRIMARY KEY,                                    -- "bpr-" + 8 hex chars
    blueprint_id TEXT NOT NULL REFERENCES blueprints(id) ON DELETE CASCADE,
    agent_id TEXT,                                          -- Session ID of executing agent
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'passed', 'failed', 'cancelled')),
    attempt INTEGER NOT NULL DEFAULT 1,
    error TEXT,
    started_at TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT,
    trace_id TEXT                                           -- Links to trace chain
);

CREATE INDEX IF NOT EXISTS idx_blueprint_runs_blueprint_id ON blueprint_runs(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_blueprint_runs_status ON blueprint_runs(status);

-- Quality gate results table
CREATE TABLE IF NOT EXISTS gate_results (
    id TEXT PRIMARY KEY,                                    -- "gate-" + 8 hex chars
    blueprint_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    gate_name TEXT NOT NULL
        CHECK (gate_name IN ('lint', 'typecheck', 'test', 'security', 'format')),
    passed INTEGER NOT NULL DEFAULT 0,                      -- 0=failed, 1=passed
    command TEXT NOT NULL,
    exit_code INTEGER NOT NULL DEFAULT 0,
    stdout_excerpt TEXT NOT NULL DEFAULT '',                 -- First 2000 chars
    stderr_excerpt TEXT NOT NULL DEFAULT '',                 -- First 2000 chars
    duration_ms INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    trace_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_gate_results_blueprint_id ON gate_results(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_gate_results_gate_name ON gate_results(gate_name);
CREATE INDEX IF NOT EXISTS idx_gate_results_agent_id ON gate_results(agent_id);

-- Isolation manifests table
CREATE TABLE IF NOT EXISTS isolation_manifests (
    agent_id TEXT PRIMARY KEY,                              -- Session ID
    agent_name TEXT NOT NULL,
    capability TEXT NOT NULL,
    grants TEXT NOT NULL DEFAULT '{}',                      -- JSON: ResourceGrants
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_isolation_manifests_capability ON isolation_manifests(capability);
CREATE INDEX IF NOT EXISTS idx_isolation_manifests_expires_at ON isolation_manifests(expires_at);

-- Holdout specs table (metadata only; encrypted content on disk)
CREATE TABLE IF NOT EXISTS holdout_specs (
    id TEXT PRIMARY KEY,                                    -- "hold-" + 8 hex chars
    blueprint_id TEXT NOT NULL,
    encrypted INTEGER NOT NULL DEFAULT 1,                   -- Always 1
    file_path TEXT NOT NULL,                                -- Path to encrypted file on disk
    test_count INTEGER NOT NULL DEFAULT 0,                  -- Number of tests (metadata only)
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_holdout_specs_blueprint_id ON holdout_specs(blueprint_id);

-- Holdout results table
CREATE TABLE IF NOT EXISTS holdout_results (
    id TEXT PRIMARY KEY,                                    -- "hr-" + 8 hex chars
    holdout_id TEXT NOT NULL REFERENCES holdout_specs(id) ON DELETE CASCADE,
    blueprint_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    score REAL NOT NULL DEFAULT 0.0,                        -- 0.0-1.0 weighted score
    tests_passed INTEGER NOT NULL DEFAULT 0,
    tests_total INTEGER NOT NULL DEFAULT 0,
    behavioral_drift INTEGER NOT NULL DEFAULT 0,            -- 0=no drift, 1=drift detected
    details TEXT NOT NULL DEFAULT '[]',                      -- JSON array of HoldoutTestResult
    verified_at TEXT NOT NULL DEFAULT (datetime('now')),
    trace_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_holdout_results_holdout_id ON holdout_results(holdout_id);
CREATE INDEX IF NOT EXISTS idx_holdout_results_blueprint_id ON holdout_results(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_holdout_results_agent_id ON holdout_results(agent_id);

-- Behavioral baselines table
CREATE TABLE IF NOT EXISTS behavioral_baselines (
    id TEXT PRIMARY KEY,                                    -- "base-" + 8 hex chars
    blueprint_id TEXT NOT NULL,
    agent TEXT NOT NULL,
    capability TEXT NOT NULL,
    metrics TEXT NOT NULL DEFAULT '{}',                      -- JSON: baseline metrics
    tolerance REAL NOT NULL DEFAULT 0.3,                    -- 0.0-1.0 deviation tolerance
    drift_threshold REAL NOT NULL DEFAULT 0.7,              -- Score below which drift fires
    sample_count INTEGER NOT NULL DEFAULT 0,
    last_updated TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_behavioral_baselines_blueprint_id ON behavioral_baselines(blueprint_id);
