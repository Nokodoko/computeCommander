-- Migration 007: Add parent_key column to jira_issues for sub-task relationship tracking.
-- Stores the parent issue key for sub-tasks (non-epic parents only; epic relationships use epic_id).

ALTER TABLE jira_issues ADD COLUMN parent_key TEXT;
CREATE INDEX IF NOT EXISTS idx_jira_issues_parent_key ON jira_issues(parent_key);
