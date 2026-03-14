-- Migration 006: Jira prompt execution log

CREATE TABLE IF NOT EXISTS jira_prompt_log (
    id BIGSERIAL PRIMARY KEY,
    instance_name TEXT NOT NULL,
    issue_key TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    jira_comment_id TEXT,
    status TEXT NOT NULL DEFAULT 'success'
        CHECK (status IN ('success', 'failed', 'undone')),
    error_message TEXT,
    batch_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jira_prompt_log_issue ON jira_prompt_log(issue_key);
CREATE INDEX IF NOT EXISTS idx_jira_prompt_log_batch ON jira_prompt_log(batch_id);
CREATE INDEX IF NOT EXISTS idx_jira_prompt_log_created ON jira_prompt_log(created_at DESC);
