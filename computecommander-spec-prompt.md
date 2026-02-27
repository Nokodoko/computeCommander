# ComputeCommander Spec Generation Prompt

You are a technical architect designing **ComputeCommander**, a ground-up rebuild of [Overstory](https://github.com/jayminwest/overstory) — a multi-agent orchestration system for AI coding agents.

## Mission

Generate a comprehensive `spec.md` for ComputeCommander that serves as the authoritative technical specification for implementation.

---

## Context: What is Overstory?

Overstory is a project-agnostic swarm system for Claude Code agent orchestration. It:
- Spawns worker agents in git worktrees via tmux
- Coordinates agents through a custom SQLite mail system
- Merges work back with tiered conflict resolution
- Provides tiered watchdog monitoring (mechanical daemon + AI triage + monitor agent)

Reference the Overstory README and source at `~/Programs/ai/overstory-publish/` for architecture details.

---

## Technology Mapping

| Component | Overstory | ComputeCommander |
|-----------|-----------|------------------|
| Language | TypeScript (Bun) | **Golang** |
| Database | SQLite (bun:sqlite) | **Postgres** (primary) + SQLite (local fallback) |
| TUI Framework | Raw ANSI | **Gum** (charmbracelet/gum) |
| Multiplexer | tmux | **Zellij** |
| Terminal | any | **Wezterm** (spawned in dwm windows) |
| Config Format | JSON | **YAML** |
| Distribution | bun link | **Single binary** |
| Isolation | git worktree | **git worktree** (keep) |

---

## Core Architecture Requirements

### 1. Agent Spawning Model

- Agents spawn in **Zellij panes** within **Wezterm** windows (dwm window manager)
- Each agent gets an isolated **git worktree**
- Visual: users can watch agents work in their panes
- Execution model similar to Overstory's tmux approach, adapted for Zellij

### 2. Modular Agent Support

**Supported Agent Runtimes:**
- Claude Code (`claude`)
- Gemini CLI (`gemini`)
- Codex CLI (`codex`)
- Pi Coding Agent (`pi`)
- Goose (`goose`)

**Agent Definition System:**
- User-defined agent definitions in `.computecommander/agents/`
- Each definition specifies:
  - Agent type/capability (scout, builder, reviewer, lead, merger, etc.)
  - Model/runtime to use (claude, gemini, codex, pi, goose)
  - Tool permissions
  - File scope restrictions
- **Opinionated defaults**: Sub-agents auto-mapped to appropriate model types based on task unless user overrides
- Orchestrator/Supervisor respect user-defined mappings

### 3. Database Architecture

**Postgres (Primary):**
- Mail system (agent-to-agent messaging)
- Session tracking
- Metrics and token instrumentation
- Task groups
- Merge queue
- Designed for distributed operation (colony of swarms)

**SQLite (Fallback):**
- Single-machine operation without Postgres dependency
- Feature parity with Postgres mode

**Remote Agent Support:**
- Mail system supports cross-machine agents
- Built for future distributed orchestration

### 4. Feature Flags & Workflows

- Predefined workflows exposed as feature flags
- Workflows programmable by model agents via `/commands`
- Enable/disable capabilities at project or agent level

---

## Features to Port (from Overstory)

### Must Have (Port Directly)
1. **Mail System** — Agent-to-agent messaging with typed protocol, broadcast addresses (@all, @builders, etc.)
2. **Tiered Watchdog** — Tier 0 mechanical daemon, Tier 1 AI triage, Tier 2 monitor agent
3. **FIFO Merge Queue** — SQLite/Postgres-backed with 4-tier conflict resolution
4. **Task Groups** — Batch coordination with auto-close
5. **Dashboard** — Live TUI (now Gum-powered) for fleet monitoring
6. **Checkpoint/Restore** — Session save/restore for compaction survivability
7. **Token Instrumentation** — Cost tracking and session metrics
8. **Tool Enforcement** — PreToolUse hooks to block unauthorized operations
9. **Agent Hierarchy** — Coordinator → Supervisor → Workers (Scout, Builder, Reviewer, Lead, Merger, Monitor)

### Enhanced Features

**Smart Nudge System:**
- **Soft nudge**: Send message to agent (existing behavior)
- **Hard nudge**: Kill + respawn agent in same worktree
- **Configurable thresholds** with sane defaults
- **Context-aware detection**: 
  - Grab most recent agent context
  - Compare time spent on task vs estimated level of effort
  - Do NOT assume loops — make intelligent determination
  - Escalation path: soft → hard after threshold exceeded

---

## Project Structure

```
computeCommander/
├── cmd/
│   └── computecommander/
│       └── main.go              # CLI entry point
├── internal/
│   ├── config/                  # YAML config loader
│   ├── agents/                  # Agent lifecycle, manifest, overlay
│   ├── mail/                    # Postgres/SQLite mail system
│   ├── worktree/                # Git worktree management
│   ├── zellij/                  # Zellij pane management
│   ├── merge/                   # FIFO queue + conflict resolution
│   ├── watchdog/                # Tiered health monitoring
│   ├── metrics/                 # Token/cost instrumentation
│   ├── tracker/                 # Task tracking
│   ├── tui/                     # Gum-powered dashboard
│   └── commands/                # CLI subcommands
├── pkg/
│   └── runtimes/                # Pluggable agent runtimes
│       ├── claude/
│       ├── gemini/
│       ├── codex/
│       ├── pi/
│       └── goose/
├── agents/                      # Base agent definitions (.md files)
├── templates/                   # Overlay and hook templates
├── go.mod
├── go.sum
└── Makefile
```

### Per-Project Scaffolding (on `init`)

```
.computecommander/
├── config.yaml                  # Project configuration
├── agents/                      # User-defined agent definitions
│   ├── scout.yaml
│   ├── builder.yaml
│   └── ...
└── hooks/                       # Tool enforcement rules
    └── rules.yaml
```

---

## CLI Commands

Port all Overstory commands, adapted for Go/Zellij:

```
computecommander init                    Initialize project
computecommander coordinator start|stop|status
computecommander supervisor start|stop|status
computecommander sling <task-id>         Spawn worker agent
  --capability <type>
  --runtime <claude|gemini|codex|pi|goose>
  --name <name>
computecommander stop <agent>
computecommander status
computecommander dashboard
computecommander nudge <agent>
  --soft | --hard
  --force
computecommander mail send|check|list|read|reply
computecommander group create|status|add|list
computecommander merge --branch|--all|--into|--dry-run
computecommander worktree list|clean
computecommander monitor start|stop|status
computecommander watch                   Watchdog daemon
computecommander doctor                  Health checks
computecommander inspect <agent>
computecommander costs
computecommander metrics
computecommander clean
computecommander config                  Show/edit config
computecommander feature                 Feature flag management
```

---

## Configuration Schema

```yaml
# .computecommander/config.yaml
version: 1

database:
  driver: postgres  # or sqlite
  postgres:
    host: localhost
    port: 5432
    database: computecommander
    user: cc
    password: ${CC_DB_PASSWORD}
  sqlite:
    path: .computecommander/local.db

zellij:
  layout: default
  terminal: wezterm

defaults:
  runtime: claude
  model_mappings:
    scout: gemini      # fast, cheap for exploration
    builder: claude    # best for implementation
    reviewer: claude   # thorough review
    merger: claude     # conflict resolution

nudge:
  soft_timeout: 10m
  hard_timeout: 30m
  escalation_enabled: true
  context_window: 50   # messages to analyze

features:
  distributed: false
  remote_agents: false
  auto_merge: true

agents:
  # Agent-specific overrides
  scout:
    runtime: gemini
    model: gemini-2.5-pro
  builder:
    runtime: claude
    model: claude-sonnet-4
```

---

## Deliverable

Generate a comprehensive `spec.md` that includes:

1. **Executive Summary** — What ComputeCommander is and why it exists
2. **Architecture Overview** — System design with diagrams (mermaid)
3. **Component Specifications** — Detailed design for each subsystem
4. **Data Models** — Database schemas (Postgres + SQLite)
5. **API/CLI Reference** — All commands with flags and examples
6. **Configuration Reference** — Full YAML schema documentation
7. **Agent Runtime Interface** — How to add new agent runtimes
8. **Migration Path** — Notes for Overstory users
9. **Future Roadmap** — Colony of swarms, distributed orchestration

Write the spec to: `~/Programs/ai/computeCommander/spec.md`

Be thorough. This spec will drive implementation.
