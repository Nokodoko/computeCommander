-- Migration 010: Add model and session_name columns to sessions table
-- Model tracks the LLM model used (e.g., "claude-opus-4-6", "claude-sonnet-4-6")
-- Session name tracks the Claude Code session identifier for multi-session management

ALTER TABLE sessions ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN session_name TEXT NOT NULL DEFAULT '';
