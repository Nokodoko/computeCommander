-- ComputeCommander PostgreSQL schema
-- Migration 004: Evals table

CREATE TABLE IF NOT EXISTS evals (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    agent_task TEXT NOT NULL DEFAULT '',
    eval_type TEXT NOT NULL DEFAULT 'custom'
        CHECK (eval_type IN (
            'unit_test', 'integration', 'lint', 'build', 'custom',
            'semantic_check', 'structural_check', 'contains_pattern',
            'count_check', 'ast_check', 'negation_check',
            'test_execution', 'type_check', 'diff_validation',
            'output_pattern_match'
        )),
    command TEXT NOT NULL,
    passed BOOLEAN,
    error_detail TEXT,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_evals_project ON evals(project_name);
CREATE INDEX IF NOT EXISTS idx_evals_type ON evals(eval_type);
