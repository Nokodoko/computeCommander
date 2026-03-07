-- ComputeCommander Agentic Foundation Postgres schema
-- Migration 002: Agentic foundation tables (traceability, blocks, blueprints, gates, isolation, holdouts)

-- Trace events table: records every action in a causal chain
CREATE TABLE IF NOT EXISTS trace_events (
    id VARCHAR(32) PRIMARY KEY,                             -- "trc-" + 16 hex chars
    trace_id VARCHAR(32) NOT NULL,                          -- Root trace ID for the causal chain
    parent_id VARCHAR(32),                                  -- Parent TraceEvent ID (NULL for root)
    span_id VARCHAR(32) NOT NULL,                           -- Unique span within the trace

    -- Actor
    agent_id VARCHAR(64) NOT NULL,                          -- Session ID of acting agent
    agent_name VARCHAR(128) NOT NULL,                       -- Human-readable agent name
    capability VARCHAR(32) NOT NULL,                        -- scout|builder|reviewer|lead|merger|coordinator|supervisor|monitor

    -- Action
    event_type VARCHAR(32) NOT NULL
        CHECK (event_type IN ('tool_call', 'agent_spawn', 'agent_stop', 'mail_send',
                              'mail_receive', 'merge_attempt', 'merge_complete',
                              'gate_check', 'block_check', 'blueprint_start',
                              'blueprint_complete', 'holdout_verify', 'context_inject', 'error')),
    tool_name VARCHAR(64),                                  -- Bash|Read|Write|Edit|Glob|Grep|Task|NULL
    tool_input_hash VARCHAR(64),                            -- SHA-256 of tool input
    tool_result_code INT,                                   -- 0=success, >0=error
    tool_result_summary TEXT,                               -- First 500 chars of result

    -- Safety
    block_rule_id VARCHAR(64),                              -- Block rule ID if one fired
    block_disposition VARCHAR(16)
        CHECK (block_disposition IN ('allowed', 'blocked', 'overridden', 'warned') OR block_disposition IS NULL),

    -- Context
    blueprint_id VARCHAR(32),                               -- Blueprint being executed
    file_paths JSONB NOT NULL DEFAULT '[]',                 -- JSON array of file paths
    duration_ms INT NOT NULL DEFAULT 0,                     -- Wall-clock duration

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id VARCHAR(64),                                 -- cmdr session ID
    run_id VARCHAR(32)                                      -- Orchestration run ID
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
    id VARCHAR(64) PRIMARY KEY,                             -- Rule identifier
    description TEXT NOT NULL,
    tool VARCHAR(64) NOT NULL,                              -- Tool name to match
    match_config JSONB NOT NULL DEFAULT '{}',               -- Match conditions
    action VARCHAR(10) NOT NULL DEFAULT 'block'
        CHECK (action IN ('block', 'warn')),
    message TEXT NOT NULL,
    severity VARCHAR(10) NOT NULL DEFAULT 'high'
        CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    override_type VARCHAR(10) DEFAULT 'none'
        CHECK (override_type IN ('grant', 'none')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source VARCHAR(10) NOT NULL DEFAULT 'default'
        CHECK (source IN ('default', 'custom')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_block_rules_tool ON block_rules(tool);
CREATE INDEX IF NOT EXISTS idx_block_rules_enabled ON block_rules(enabled);
CREATE INDEX IF NOT EXISTS idx_block_rules_severity ON block_rules(severity);

-- Block rate limits: sliding window counters for rate-limited rules
CREATE TABLE IF NOT EXISTS block_rate_limits (
    rule_id VARCHAR(64) NOT NULL REFERENCES block_rules(id) ON DELETE CASCADE,
    agent_id VARCHAR(64) NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 1,
    PRIMARY KEY (rule_id, agent_id, window_start)
);

-- Blueprints table: structured task definitions
CREATE TABLE IF NOT EXISTS blueprints (
    id VARCHAR(32) PRIMARY KEY,                             -- "bp-" + 8 hex chars
    version INT NOT NULL DEFAULT 1,
    name VARCHAR(256) NOT NULL,
    agent VARCHAR(64) NOT NULL,                             -- Target agent type
    capability VARCHAR(32) NOT NULL,                        -- Required capability

    -- Context & IO
    context_grants JSONB NOT NULL DEFAULT '[]',
    inputs JSONB NOT NULL DEFAULT '{}',
    outputs JSONB NOT NULL DEFAULT '{}',
    verify_steps JSONB NOT NULL DEFAULT '[]',
    gates JSONB NOT NULL DEFAULT '[]',
    depends_on JSONB NOT NULL DEFAULT '[]',

    -- Execution config
    retry_limit INT NOT NULL DEFAULT 3,
    timeout VARCHAR(32) NOT NULL DEFAULT '30m',

    -- Execution state
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'passed', 'failed', 'blocked', 'cancelled')),
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blueprints_status ON blueprints(status);
CREATE INDEX IF NOT EXISTS idx_blueprints_agent ON blueprints(agent);
CREATE INDEX IF NOT EXISTS idx_blueprints_capability ON blueprints(capability);

-- Blueprint runs table: execution history for blueprints
CREATE TABLE IF NOT EXISTS blueprint_runs (
    id VARCHAR(32) PRIMARY KEY,                             -- "bpr-" + 8 hex chars
    blueprint_id VARCHAR(32) NOT NULL REFERENCES blueprints(id) ON DELETE CASCADE,
    agent_id VARCHAR(64),                                   -- Session ID of executing agent
    status VARCHAR(20) NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'passed', 'failed', 'cancelled')),
    attempt INT NOT NULL DEFAULT 1,
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    trace_id VARCHAR(32)
);

CREATE INDEX IF NOT EXISTS idx_blueprint_runs_blueprint_id ON blueprint_runs(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_blueprint_runs_status ON blueprint_runs(status);

-- Quality gate results table
CREATE TABLE IF NOT EXISTS gate_results (
    id VARCHAR(32) PRIMARY KEY,                             -- "gate-" + 8 hex chars
    blueprint_id VARCHAR(32) NOT NULL,
    agent_id VARCHAR(64) NOT NULL,
    gate_name VARCHAR(20) NOT NULL
        CHECK (gate_name IN ('lint', 'typecheck', 'test', 'security', 'format')),
    passed BOOLEAN NOT NULL DEFAULT FALSE,
    command TEXT NOT NULL,
    exit_code INT NOT NULL DEFAULT 0,
    stdout_excerpt TEXT NOT NULL DEFAULT '',
    stderr_excerpt TEXT NOT NULL DEFAULT '',
    duration_ms INT NOT NULL DEFAULT 0,
    attempt INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    trace_id VARCHAR(32)
);

CREATE INDEX IF NOT EXISTS idx_gate_results_blueprint_id ON gate_results(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_gate_results_gate_name ON gate_results(gate_name);
CREATE INDEX IF NOT EXISTS idx_gate_results_agent_id ON gate_results(agent_id);

-- Isolation manifests table
CREATE TABLE IF NOT EXISTS isolation_manifests (
    agent_id VARCHAR(64) PRIMARY KEY,                       -- Session ID
    agent_name VARCHAR(128) NOT NULL,
    capability VARCHAR(32) NOT NULL,
    grants JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_isolation_manifests_capability ON isolation_manifests(capability);
CREATE INDEX IF NOT EXISTS idx_isolation_manifests_expires_at ON isolation_manifests(expires_at);

-- Holdout specs table (metadata only; encrypted content on disk)
CREATE TABLE IF NOT EXISTS holdout_specs (
    id VARCHAR(32) PRIMARY KEY,                             -- "hold-" + 8 hex chars
    blueprint_id VARCHAR(32) NOT NULL,
    encrypted BOOLEAN NOT NULL DEFAULT TRUE,
    file_path TEXT NOT NULL,
    test_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_holdout_specs_blueprint_id ON holdout_specs(blueprint_id);

-- Holdout results table
CREATE TABLE IF NOT EXISTS holdout_results (
    id VARCHAR(32) PRIMARY KEY,                             -- "hr-" + 8 hex chars
    holdout_id VARCHAR(32) NOT NULL REFERENCES holdout_specs(id) ON DELETE CASCADE,
    blueprint_id VARCHAR(32) NOT NULL,
    agent_id VARCHAR(64) NOT NULL,
    score DECIMAL(5, 4) NOT NULL DEFAULT 0.0,
    tests_passed INT NOT NULL DEFAULT 0,
    tests_total INT NOT NULL DEFAULT 0,
    behavioral_drift BOOLEAN NOT NULL DEFAULT FALSE,
    details JSONB NOT NULL DEFAULT '[]',
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    trace_id VARCHAR(32)
);

CREATE INDEX IF NOT EXISTS idx_holdout_results_holdout_id ON holdout_results(holdout_id);
CREATE INDEX IF NOT EXISTS idx_holdout_results_blueprint_id ON holdout_results(blueprint_id);
CREATE INDEX IF NOT EXISTS idx_holdout_results_agent_id ON holdout_results(agent_id);

-- Behavioral baselines table
CREATE TABLE IF NOT EXISTS behavioral_baselines (
    id VARCHAR(32) PRIMARY KEY,                             -- "base-" + 8 hex chars
    blueprint_id VARCHAR(32) NOT NULL,
    agent VARCHAR(64) NOT NULL,
    capability VARCHAR(32) NOT NULL,
    metrics JSONB NOT NULL DEFAULT '{}',
    tolerance DECIMAL(3, 2) NOT NULL DEFAULT 0.30,
    drift_threshold DECIMAL(3, 2) NOT NULL DEFAULT 0.70,
    sample_count INT NOT NULL DEFAULT 0,
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_behavioral_baselines_blueprint_id ON behavioral_baselines(blueprint_id);
