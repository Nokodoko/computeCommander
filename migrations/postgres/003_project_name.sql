-- ComputeCommander Postgres schema
-- Migration 003: Add friendly name column to projects

ALTER TABLE projects ADD COLUMN IF NOT EXISTS friendly_name VARCHAR(256) NOT NULL DEFAULT '';
