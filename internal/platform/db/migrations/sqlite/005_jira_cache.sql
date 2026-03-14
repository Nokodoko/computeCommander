-- Migration 005: Jira cache tables for multi-instance Jira integration.

-- Jira project cache
CREATE TABLE IF NOT EXISTS jira_projects (
    id TEXT PRIMARY KEY,
    instance_name TEXT NOT NULL,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    lead TEXT,
    synced_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(instance_name, key)
);

-- Jira epic cache
CREATE TABLE IF NOT EXISTS jira_epics (
    id TEXT PRIMARY KEY,
    instance_name TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES jira_projects(id),
    key TEXT NOT NULL,
    summary TEXT NOT NULL,
    status TEXT NOT NULL,
    synced_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Jira issue cache
CREATE TABLE IF NOT EXISTS jira_issues (
    id TEXT PRIMARY KEY,
    instance_name TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES jira_projects(id),
    epic_id TEXT REFERENCES jira_epics(id),
    key TEXT NOT NULL,
    summary TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    issue_type TEXT NOT NULL,
    priority TEXT,
    assignee TEXT,
    labels TEXT NOT NULL DEFAULT '[]',
    acceptance_criteria TEXT,
    agent_type TEXT,
    agent_state TEXT,
    session_id TEXT,
    prompt_hash TEXT,
    synced_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(instance_name, key)
);

-- Jira sync state (track last sync per instance)
CREATE TABLE IF NOT EXISTS jira_sync_state (
    instance_name TEXT PRIMARY KEY,
    last_sync_at TEXT NOT NULL,
    last_sync_status TEXT NOT NULL DEFAULT 'success',
    issues_synced INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_jira_issues_project ON jira_issues(project_id);
CREATE INDEX IF NOT EXISTS idx_jira_issues_epic ON jira_issues(epic_id);
CREATE INDEX IF NOT EXISTS idx_jira_issues_status ON jira_issues(status);
CREATE INDEX IF NOT EXISTS idx_jira_issues_instance ON jira_issues(instance_name);
CREATE INDEX IF NOT EXISTS idx_jira_epics_project ON jira_epics(project_id);
