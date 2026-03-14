-- ComputeCommander SQLite schema
-- Migration 003: Add friendly name column to projects

ALTER TABLE projects ADD COLUMN friendly_name TEXT NOT NULL DEFAULT '';
