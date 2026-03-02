<p align="center">
  <img src="assets/banner.jpg" alt="ComputeCommander" width="600">
</p>

<h1 align="center">ComputeCommander</h1>

<p align="center">
  <em>An agentic IDE for orchestrating AI coding swarms</em>
</p>

<p align="center">
  <a href="#installation">Installation</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#ui-layout">UI Layout</a> •
  <a href="#commands">Commands</a> •
  <a href="#keybinds">Keybinds</a>
</p>

---

## Overview

**ComputeCommander** (`cmdr`) transforms terminal-based AI development into a multi-agent IDE experience. Launch agent sessions, manage swarms, and orchestrate across multiple projects—all from a leader-key-driven interface built on [Zellij](https://zellij.dev).

- 🤖 **Embedded agent sessions** — interact with Claude Code, Codex, or other agents directly
- 📁 **File picker navigation** — browse directories, spawn sessions per project
- 📬 **Inter-agent mail** — agents communicate via a built-in message queue
- 🔀 **Merge queue** — track and manage pending PRs across worktrees
- ⌨️ **Leader key system** — `Ctrl+Space` + key for every action

## Installation

```bash
go install github.com/noko/computecommander/cmd/cmdr@latest
```

Or build from source:

```bash
git clone https://github.com/noko/computecommander
cd computecommander
make build
```

## Quick Start

```bash
# Initialize a new project
cmdr init

# Open the IDE (or just run `cmdr` with no args)
cmdr

# Stop all sessions
cmdr stop
```

Running `cmdr` with no arguments opens the full interface.

## UI Layout

```
┌─────────┬─────────────────────────────────────────────┬──────────┐
│         │                                             │          │
│         │           Agent Session                     │          │
│   FP    │           (i.e. Claude Code)                │  Agents  │
│         │                                             │          │
│         │                                             │          │
│         ├───────────┬─────────┬─────────────┬─────────┤          │
│         │  Event    │  Mail   │   Merge     │ Events  │          │
│         │  Log      │         │   Queue     │         │          │
└─────────┴───────────┴─────────┴─────────────┴─────────┴──────────┘
```

| Panel | Description |
|-------|-------------|
| **FP** | File picker — navigate directories, launch sessions |
| **Agent Session** | Primary workspace — embedded agent interaction |
| **Agents** | Agent list and swarm status |
| **Event Log** | System events and logs |
| **Mail** | Inter-agent messages |
| **Merge Queue** | Pending merges and PRs |
| **Events** | Activity feed |

## Commands

| Command | Description |
|---------|-------------|
| `cmdr` | Open the IDE |
| `cmdr init` | Initialize project in current directory |
| `cmdr open` | Open existing project |
| `cmdr stop` | Stop all running sessions |
| `cmdr reset` | Reset project state |
| `cmdr backup` | Backup database |
| `cmdr restore` | Restore from backup |
| `cmdr config edit` | Edit config (hot-reloads) |
| `cmdr export` | Export data |
| `cmdr version` | Show version |

## Keybinds

Leader key: **`Ctrl+Space`**

| Key | Action |
|-----|--------|
| `h` | Help |
| `s` | Shell |
| `e` | Export |
| `v` | Version |
| `u` | Update check |
| `l` | Clear logs |
| `b` | Backup |
| `r` | Restore |
| `t` | Theme |
| `f` | Feedback |
| `?` | Accessibility |

## Requirements

- Go 1.25+
- [Zellij](https://zellij.dev) (for the UI layout)
- SQLite (embedded)

## Architecture

ComputeCommander is structured as a modular Go application with a pluggable runtime system and a dual-database backend.

```
computeCommander/
  cmd/cc/              CLI entry point (Cobra command tree, 11 command groups)
  internal/
    agents/            Agent lifecycle: spawn, stop, guards, overlays, templates
    backup/            Database backup and restore operations
    commands/          CLI command handlers (App DI container)
    config/            Configuration schema, loading, validation, file watcher
    export/            Database export to JSON/CSV
    gateway/           HTTP REST API gateway (/api/v1/)
    keybinds/          Leader-key keybind configuration and action registry
    mail/              Inter-agent mail system with priorities and broadcast
    merge/             FIFO merge queue + 4-tier conflict resolution
    platform/db/       Database abstraction (SQLite + PostgreSQL) with migrations
    tui/               BubbleTea dashboard (status, mail, costs, file picker, sessions)
    watchdog/          3-tier health monitoring daemon
    wezterm/           WezTerm window management (WM_CLASS for tiling WMs)
    worktree/          Git worktree lifecycle management
    zellij/            Zellij pane management + KDL layout generation
  pkg/
    runtimes/          AgentRuntime interface + 5 adapters (claude, gemini, codex, pi, goose)
    integrations/      GitHub, Linear, Webhook integrations
  agents/              YAML agent role definitions
  templates/           Go text/template overlays for agent instructions
  migrations/          Root-level SQL migration mirrors
  scripts/             Shell scripts (agent wrapper for session-switch support)
```

### Key Interfaces

The codebase follows an interface-first design. Core abstractions:

| Interface | Package | Description |
|-----------|---------|-------------|
| `DB` | `internal/platform/db` | Database operations (Exec, Query, QueryRow, Begin, Close) |
| `MailStore` | `internal/mail` | Message send, check, list, reply, purge |
| `MergeQueue` | `internal/merge` | Enqueue, dequeue, peek, list, update status |
| `WorktreeManager` | `internal/worktree` | Create, list, status, remove, clean worktrees |
| `PaneManager` | `internal/zellij` | Create, list, send keys, capture, close panes |
| `WindowManager` | `internal/wezterm` | Spawn terminal windows |
| `AgentRuntime` | `pkg/runtimes` | Pluggable AI agent backends (10 methods) |

### Dependency Injection

The `App` struct in `internal/commands/app.go` is the central DI container. It wires all services together at startup: config, database, spawner, mail store, merge queue, merge executor, worktree manager, pane manager, window manager, watchdog, and gateway. Every CLI command receives an `*App` and accesses services through it.

## Configuration

Configuration is managed through YAML files stored in `.computecommander/config.yaml`. The system supports:

- **Environment variable expansion**: `${VAR}` patterns are replaced with environment variable values
- **Local overlay merging**: A `config.local.yaml` file in the same directory is merged on top of the base config
- **Tilde expansion**: `~/` paths are expanded to the user's home directory
- **Hot reload**: An fsnotify-based file watcher reloads config on changes and notifies registered handlers

### Config Sections

| Section | Key Fields |
|---------|------------|
| `project` | name, root, canonical_branch, quality_gates |
| `database` | driver (`sqlite` or `postgres`), connection settings |
| `agents` | max_concurrent, stagger_delay_ms, max_depth, max_sessions_per_run, max_agents_per_lead, base_dir |
| `zellij` | dashboard_layout path |
| `worktrees` | base_dir |
| `defaults` | runtime, agent_command, model_mappings |
| `nudge` | soft_timeout, hard_timeout, escalation_enabled, loop_detection |
| `watchdog` | tier0_interval_ms, stale_threshold_ms, zombie_threshold_ms |
| `merge` | merge settings |
| `features` | runtime feature flags |
| `logging` | level, format |
| `runtimes` | per-runtime config (claude, gemini, codex, pi, goose) |

### Config CLI

```bash
cmdr config show              # Display current config
cmdr config get <key>         # Get a value by dot-notation key
cmdr config set <key> <value> # Set a value
cmdr config validate          # Validate all sections
cmdr config edit              # Open in editor (hot-reloads)
```

## Database

ComputeCommander supports dual database backends via a unified `DB` interface:

- **SQLite** (default) -- pure-Go via `modernc.org/sqlite`, WAL mode, `busy_timeout=5000`, `foreign_keys=ON`
- **PostgreSQL** -- via `pgx/v5` connection pool, native types (TIMESTAMPTZ, JSONB, TEXT[])

### Schema (10 tables)

| Table | PK | Purpose |
|-------|-----|---------|
| `runs` | id (TEXT) | Orchestration run tracking |
| `sessions` | id (TEXT) | Agent session lifecycle and state |
| `events` | id (AUTO) | Audit trail for tool use, spawns, errors |
| `mail` | id (TEXT) | Inter-agent messages with priority |
| `metrics` | id (AUTO) | Token usage, cost, performance per agent |
| `merge_queue` | branch_name | FIFO merge queue entries |
| `task_groups` | id (TEXT) | Task group metadata |
| `task_group_members` | (group_id, issue_id) | Group-to-issue mapping |
| `checkpoints` | id (AUTO) | Session recovery snapshots |
| `worktrees` | path (TEXT) | Git worktree state tracking |

Migrations are embedded in the binary via `//go:embed` and applied automatically during `cmdr init` and app startup. SQL uses `CREATE TABLE IF NOT EXISTS` for idempotent application.

### Data Management

```bash
cmdr backup            # Backup database file
cmdr restore           # Restore from backup
cmdr export            # Export data as JSON/CSV (--format, --output, --tables)
cmdr clear             # Clear event logs
cmdr reset             # Reset database to empty (confirmation-gated)
```

## Agent Management

Agents are the core unit of work. Each agent runs in an isolated git worktree with capability-based tool guards and a deployed instruction overlay.

### Agent Capabilities

| Capability | Description | Can Spawn | Default Runtime |
|------------|-------------|-----------|-----------------|
| `scout` | Read-only exploration | No | gemini |
| `builder` | Implementation (scoped_write) | No | claude-sonnet-4 |
| `reviewer` | Code review (read_only) | No | claude-sonnet-4 |
| `lead` | Team coordination (full_write) | Up to 5 | claude-sonnet-4 |
| `merger` | Branch merge specialist | No | claude-sonnet-4 |
| `coordinator` | Persistent orchestrator | Leads only | claude-opus-4 |
| `monitor` | Tier 2 continuous patrol | No | claude-haiku-3 |

### Agent Lifecycle

| State | Description |
|-------|-------------|
| `booting` | Agent is starting up |
| `working` | Agent is actively processing |
| `completed` | Agent finished its task |
| `stalled` | Agent has been idle past the soft timeout |
| `zombie` | Agent is unresponsive past the hard timeout |

### Spawn and Stop

```bash
cmdr sling <name> --task <id> --capability builder --runtime claude
cmdr stop <name> [--force] [--reason "..."]
cmdr status [--capability X] [--state Y] [--pane]
cmdr inspect <name>
```

When an agent is spawned, the system: validates the request, creates an isolated git worktree with a branch named `cc/{agent_name}/{task_id}`, renders and deploys a capability-specific instruction overlay, creates a Zellij pane, and registers the session in the database.

### Guard Rules

Each capability has tool and command guards enforced at runtime. For example, `scout` agents can only use read-only tools, `builder` agents cannot spawn sub-agents, and `lead` agents can spawn up to 5 workers. Guards are defined per-capability in YAML files under `agents/`.

### Agent Role Definitions

Agent roles are defined in YAML files in the `agents/` directory. Each file specifies:

```yaml
name: builder
capability: builder
runtime: claude
model: claude-sonnet-4
tools:
  allowed: [Read, Write, Edit, Glob, Grep, Bash]
  blocked: [Spawn]
constraints: [file_scope_enforced, no_spawn, no_git_push]
```

## Runtime Adapters

ComputeCommander supports multiple AI agent backends through a pluggable `AgentRuntime` interface. Each adapter self-registers via Go's `init()` mechanism.

| Runtime | Status | Instruction File | API Key Env |
|---------|--------|-------------------|-------------|
| Claude | Full | `.claude/CLAUDE.md` | `ANTHROPIC_API_KEY` |
| Gemini | Stub | `.gemini/GEMINI.md` | `GOOGLE_API_KEY` |
| Codex | Stub | `AGENTS.md` | `OPENAI_API_KEY` |
| Pi | Stub | `.claude/CLAUDE.md` | `ANTHROPIC_API_KEY` |
| Goose | Stub | `.goose/instructions.md` | `GOOSE_API_KEY` |

The `AgentRuntime` interface provides 10 methods: `ID`, `InstructionPath`, `BuildSpawnCommand`, `BuildPrintCommand`, `DeployConfig`, `DetectReady`, `ParseTranscript`, `BuildEnv`, `RequiresBeaconVerification`, and `Connect`.

The Claude adapter is fully implemented with: `--dangerously-skip-permissions` spawning, `.claude/CLAUDE.md` and `settings.local.json` deployment, pane-based readiness detection, and JSONL transcript parsing for token usage.

## Inter-Agent Mail System

Agents communicate through a priority-ordered message queue persisted in the database.

### Message Types

- **Semantic**: `status`, `question`, `result`, `error`
- **Protocol**: `worker_done`, `merge_ready`, `merged`, `merge_failed`, `escalation`, `health_check`, `dispatch`, `assign`

### Priority Levels

`urgent` (3) > `high` (2) > `normal` (1) > `low` (0) -- messages are ordered by priority DESC, then time ASC.

### Broadcast Addresses

`@all`, `@builders`, `@scouts`, `@reviewers`, `@leads`, `@workers` -- broadcast messages are expanded to individual rows per recipient.

### Mail CLI

```bash
cmdr mail send --from X --to Y --subject "..." --body "..."
cmdr mail check <agent>        # Unread messages, priority-ordered
cmdr mail list [--agent X] [--unread]
cmdr mail read <id>
cmdr mail reply <id>
cmdr mail purge [--agent X] [--before T] [--read-only]
```

## Merge Queue

A FIFO merge queue with 4-tier conflict resolution integrates agent worktree branches into the canonical branch.

### Resolution Tiers

| Tier | Strategy | Description |
|------|----------|-------------|
| 1 | Clean merge | Standard `git merge` -- succeeds if no conflicts |
| 2 | Auto-resolve | `git merge -X theirs` -- automatically resolves in favor of the incoming branch |
| 3 | AI resolve | AI-driven conflict resolution (stub) |
| 4 | Reimagine | Complete reimplementation of conflicting changes (stub) |

The executor runs through tiers sequentially, aborting and retrying with the next tier on failure.

### Merge CLI

```bash
cmdr merge enqueue --branch <branch>
cmdr merge list [--status pending|merging|merged|conflict|failed]
cmdr merge status <branch>
cmdr merge run
```

## Watchdog Health Monitoring

A 3-tier health monitoring daemon continuously checks agent health and intervenes when issues are detected.

### Tier 0 -- Mechanical Checks

- **Process liveness**: Verifies agent PID via `kill -0`
- **Pane existence**: Confirms the Zellij pane still exists
- **Staleness detection**: Identifies agents idle past the configurable threshold

### Tier 1 -- AI Triage (stub)

Pattern classification of issues for intelligent escalation.

### Tier 2 -- Monitor Agents

Dedicated `monitor` capability agents that perform continuous patrol.

### Nudge System

When an agent stalls, the nudger intervenes:

- **Soft nudge**: Sends keystrokes to the agent's Zellij pane to prompt activity
- **Hard nudge**: Closes the pane (kills the process) when the agent is unresponsive past the hard timeout

```bash
cmdr watch                     # Start watchdog daemon
cmdr nudge <agent>             # Send nudge to agent's pane
```

## Session Management

ComputeCommander supports directory-scoped sessions with the ability to create, switch, and stop sessions. The session manager provides thread-safe access via `sync.RWMutex`.

```bash
cmdr session list              # List all sessions
cmdr session switch <id>       # Switch to a session
cmdr session stop [<id>]       # Stop a session
cmdr sessions                  # Interactive Claude session picker (via gum filter)
cmdr fp                        # Open/toggle file picker pane
```

The file picker TUI component provides directory navigation with session markers, allowing you to browse projects and launch sessions directly.

## Zellij Integration

The UI is built on [Zellij](https://zellij.dev) with KDL-based layout generation. The layout includes:

- **File picker pane** (left sidebar)
- **Agent session pane** (center, primary workspace)
- **Agents status pane** (right sidebar)
- **Bottom bar** with event log, mail, merge queue, and events panels

### Key Features

- **KDL layout generation**: Auto-generated dashboard layout at `.computecommander/layouts/cmdr-dashboard.kdl`
- **Agent wrapper script**: Generated bash script at `.computecommander/scripts/cmdr-agent-wrapper.sh` for session-switch support
- **Session management**: Creates new sessions with `--new-session-with-layout`, cleans up stale sessions
- **Floating panes**: Support for help, version, confirm dialogs, and export previews
- **Environment isolation**: Strips `ZELLIJ` env vars when launching to prevent nested session conflicts

### WezTerm Integration

WezTerm is used as the terminal emulator for spawning dashboard windows. It supports:

- Custom `WM_CLASS` for tiling window manager integration (dwm, i3, sway)
- Zellij session attachment within WezTerm windows
- Environment variable isolation to prevent nested agent conflicts

## HTTP API Gateway

An optional REST API gateway exposes ComputeCommander functionality at `/api/v1/`:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/health` | Status, version, uptime |
| `GET` | `/api/v1/status` | Fleet counts by state |
| `GET` | `/api/v1/agents` | List agents (?capability, ?state) |
| `GET` | `/api/v1/agents/{name}` | Get specific agent |
| `POST` | `/api/v1/agents` | Spawn agent (JSON body) |
| `DELETE` | `/api/v1/agents/{name}` | Stop agent |
| `GET` | `/api/v1/mail` | List mail (?agent, ?from, ?unread) |
| `POST` | `/api/v1/mail` | Send mail (JSON body) |
| `GET` | `/api/v1/merge/queue` | List merge queue (?status) |
| `GET` | `/api/v1/costs` | Cost tracking |

Middleware chain: request ID (atomic counter) -> CORS -> request logging. All responses are JSON. HTTP timeouts: read header 10s, write 30s, idle 60s.

## Integrations

### GitHub (stub)

Issue comments, PR status checks, and GitHub Actions workflow dispatch.

### Linear (stub)

Issue syncing and status updates via Linear's GraphQL API.

### Webhooks

Event dispatcher for external integrations. Supports:

- Event-type filtering on subscriptions (`agent.spawned`, `merge.completed`, etc.)
- Concurrent-safe subscription management
- Per-endpoint HTTP POST delivery with 10s timeout
- `ComputeCommander-Webhook/1.0` User-Agent header

## TUI Dashboard

The in-process BubbleTea/Lipgloss terminal dashboard provides real-time monitoring:

### Views

| Key | View | Description |
|-----|------|-------------|
| `1` | Events | System event log |
| `2` | Mail | Unread count + recent message previews |
| `3` | Merge | Pending count + color-coded status entries |
| `4` | Costs | Token usage and estimated cost per agent/model |
| `Tab` | Cycle | Cycle through bottom pane views |

### Navigation

- `j`/`k` -- Navigate agent table (cursor up/down)
- `q` or `Ctrl+C` -- Quit dashboard
- `Ctrl+Space` -- Activate leader key mode

The dashboard auto-refreshes on a configurable tick interval. Agent states are color-coded: booting, working, completed, stalled, zombie.

## Development

### Building

```bash
make build              # Produces ./cmdr binary
```

Version and commit are injected via `-ldflags` at build time.

### Project Initialization

```bash
cmdr init [--name myproject] [--db sqlite|postgres]
```

Creates the `.computecommander/` directory tree with:
- `config.yaml` (configuration)
- `rules/` (guard rules)
- `keybinds/` (keybind configuration)
- `themes/` (UI themes with a default)
- `backups/` (database backup directory)
- `plugins/` (plugin directory)
- `layouts/cmdr-dashboard.kdl` (Zellij KDL layout)
- `scripts/cmdr-agent-wrapper.sh` (agent wrapper script)
- SQLite database with schema applied

### Testing

```bash
go test ./...           # Run all tests
```

Tests use interface mocking and in-memory SQLite databases. Integration tests verify the build, file structure, session manager, and `go vet`.

### Style Guide

- **Go**: PascalCase exports, camelCase internals, `fmt.Errorf("context: %w", err)` error wrapping
- **CLI**: Each command in its own file exporting `XxxCmd(app *App) *cobra.Command`
- **Interfaces first**: DB, MailStore, MergeQueue, WorktreeManager, PaneManager, WindowManager, AgentRuntime
- **Self-registering runtimes**: `init()` + `RegisterRuntime()`
- **SQL**: `?` positional params, `CREATE TABLE IF NOT EXISTS` for idempotence

### Diagnostic Commands

```bash
cmdr doctor             # Health checks (config, db, git, zellij, project dir)
cmdr feature list       # List runtime feature flags
cmdr feature toggle X   # Toggle a feature flag
cmdr version            # Show version + release notes link
cmdr update             # Check for updates
```

## License

MIT

---

<p align="center">
  <sub>Built for humans who orchestrate machines</sub>
</p>
