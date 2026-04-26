# ComputeCommander -- Multi-Agent Orchestration System

## Purpose
ComputeCommander (`cc`) orchestrates swarms of AI coding agents. Each agent works in an isolated git worktree, communicates through a structured mail system, and merges work back with intelligent 4-tier conflict resolution. The system supports multiple AI runtime backends (Claude, Gemini, Codex, Pi, Goose) via a pluggable adapter architecture.

## Technology
- **Go 1.25** -- Core CLI and backend (`cmd/cc/`, `internal/`, `pkg/`)
- **TypeScript/Bun** -- Kubernetes cluster services (`k8s-cluster/`)
- **Cobra** -- CLI framework
- **BubbleTea/Lipgloss** -- Terminal UI dashboard
- **SQLite** (modernc.org/sqlite) / **PostgreSQL** (pgx/v5) -- Dual database support
- **Zellij** -- Terminal multiplexer for agent panes
- **WezTerm** -- Terminal emulator for window management
- **HAProxy** -- Load balancer for K8s deployment
- **Kustomize** -- Kubernetes manifest management

## Project Structure

```
computeCommander/
  cmd/cc/              CLI entry point (Cobra command tree)
  internal/
    agents/            Agent lifecycle: spawn, stop, guards, overlays
    backup/            Database backup and restore operations
    commands/          CLI command handlers (App DI container)
    config/            Configuration schema, loading, validation, file watcher
    export/            Database export to JSON/CSV
    gateway/           HTTP REST API gateway (/api/v1/)
    keybinds/          Leader-key keybind configuration and action registry
    mail/              Inter-agent mail system with priorities
    merge/             FIFO merge queue + 4-tier conflict resolution
    platform/db/       Database abstraction (SQLite + PostgreSQL)
    tui/               BubbleTea dashboard (status, mail, costs, file picker, sessions)
    watchdog/          3-tier health monitoring daemon
    wezterm/           WezTerm window management
    worktree/          Git worktree lifecycle management
    zellij/            Zellij pane management + KDL layout generation
  pkg/
    runtimes/          AgentRuntime interface + 5 adapters
    integrations/      GitHub, Linear, Webhook integrations (stubs)
  agents/              YAML agent role definitions
  templates/           Go text/template overlays
  migrations/          Root-level SQL migration mirrors
  specs/               Specification documents (see specs/index.md)
  scripts/             Shell scripts (agent wrapper for session-switch support)
  k8s-cluster/         Kubernetes infrastructure (API, WS, HAProxy, K8s manifests)
```

## Key Abstractions

### Agent Lifecycle
| Operation | CLI | API | Code Path |
|-----------|-----|-----|-----------|
| Spawn agent | `cc sling <name> --task X --capability Y` | `POST /api/v1/agents` | `internal/commands/sling.go` -> `internal/agents/spawner.go:Spawn()` |
| Stop agent | `cc stop <name>` | `DELETE /api/v1/agents/{name}` | `internal/commands/stop.go` -> `internal/agents/spawner.go:Stop()` |
| List agents | `cc status` | `GET /api/v1/agents` | `internal/commands/status.go` -> `internal/agents/spawner.go:ListSessions()` |
| Inspect agent | `cc inspect <name>` | - | `internal/commands/inspect.go` -> `internal/zellij/pane.go:CapturePaneContent()` |

### Mail System
| Operation | CLI | API | Code Path |
|-----------|-----|-----|-----------|
| Send message | `cc mail send --from X --to Y` | `POST /api/v1/mail` | `internal/commands/mail.go` -> `internal/mail/sql_store.go:Send()` |
| Check inbox | `cc mail check <agent>` | - | `internal/commands/mail.go` -> `internal/mail/sql_store.go:Check()` |
| List messages | `cc mail list` | `GET /api/v1/mail` | `internal/commands/mail.go` -> `internal/mail/sql_store.go:List()` |
| Reply | `cc mail reply <id>` | - | `internal/commands/mail.go` -> `internal/mail/sql_store.go:Reply()` |
| Purge | `cc mail purge` | - | `internal/commands/mail.go` -> `internal/mail/sql_store.go:Purge()` |

### Merge System
| Operation | CLI | API | Code Path |
|-----------|-----|-----|-----------|
| Enqueue merge | `cc merge enqueue --branch X` | - | `internal/commands/merge.go` -> `internal/merge/queue.go:Enqueue()` |
| Execute merge | `cc merge run` | - | `internal/commands/merge.go` -> `internal/merge/executor.go:Execute()` |
| Queue status | `cc merge list` | `GET /api/v1/merge/queue` | `internal/commands/merge.go` -> `internal/merge/queue.go:List()` |

### Configuration
| Operation | CLI | Code Path |
|-----------|-----|-----------|
| Initialize project | `cc init` | `cmd/cc/main.go:runInit()` |
| Show config | `cc config show` | `cmd/cc/main.go:configShowCmd()` -> `internal/config/config.go:LoadConfig()` |
| Set value | `cc config set <key> <value>` | `cmd/cc/main.go:configSetCmd()` |
| Validate | `cc config validate` | `internal/config/config.go:Validate()` |

### Monitoring
| Operation | CLI | Code Path |
|-----------|-----|-----------|
| Start watchdog | `cc watch` | `internal/commands/watch.go` -> `internal/watchdog/watchdog.go:Run()` |
| Send nudge | `cc nudge <agent>` | `internal/commands/nudge.go` -> `internal/watchdog/nudge.go:Nudge()` |
| Launch dashboard | `cc dashboard` | `internal/commands/dashboard.go` -> `internal/tui/dashboard.go:Run()` |
| View costs | `cc costs` | `internal/commands/observability.go` |

### Runtime Adapters
| Runtime | Status | Instruction File | API Key Env |
|---------|--------|-------------------|-------------|
| Claude | Full | `.claude/CLAUDE.md` | `ANTHROPIC_API_KEY` |
| Gemini | Stub | `.gemini/GEMINI.md` | `GOOGLE_API_KEY` |
| Codex | Stub | `AGENTS.md` | `OPENAI_API_KEY` |
| Pi | Stub | `.claude/CLAUDE.md` | `ANTHROPIC_API_KEY` |
| Goose | Stub | `.goose/instructions.md` | `GOOSE_API_KEY` |

### Agent Capabilities
| Capability | Description | Can Spawn | Runtime |
|------------|-------------|-----------|---------|
| `scout` | Read-only exploration | No | gemini |
| `builder` | Implementation (scoped_write) | No | claude-sonnet-4 |
| `reviewer` | Code review (read_only) | No | claude-sonnet-4 |
| `lead` | Team coordination (full_write) | Up to 5 | claude-sonnet-4 |
| `merger` | Branch merge specialist | No | claude-sonnet-4 |
| `coordinator` | Persistent orchestrator | Leads only | claude-opus-4 |
| `monitor` | Tier 2 continuous patrol | No | claude-haiku-3 |

## Database Schema (10 tables)
`runs`, `sessions`, `events`, `mail`, `metrics`, `merge_queue`, `task_groups`, `task_group_members`, `checkpoints`, `worktrees`

See `internal/platform/db/agentic_instructions.md` for full schema.

## Build & Run
```bash
make build          # Produces ./cmdr binary
./cmdr init         # Initialize project (.computecommander/)
./cmdr status       # Fleet overview
./cmdr dashboard    # Launch TUI
```

## Specs

All specification files live in `specs/`. Never create spec files in the repo root.

- Name specs with a descriptive identifier: `<topic>.md` (e.g., `session-naming.md`, `dashboard-v2.md`)
- Reviews, feedback, and validation artifacts go in `specs/reviews/`, prefixed with the spec they reviewed: `reviews/<topic>-review.md`, `reviews/<topic>-validation-errors.md`
- Update `specs/index.md` when adding or removing specs

## Style Guide
- **Go**: PascalCase exports, camelCase internals, `fmt.Errorf("context: %w", err)` error wrapping
- **CLI**: Each command in its own file exporting `XxxCmd(app *App) *cobra.Command`
- **Interfaces first**: DB, MailStore, MergeQueue, WorktreeManager, PaneManager, WindowManager, AgentRuntime
- **Self-registering runtimes**: `init()` + `RegisterRuntime()`
- **SQL**: `?` positional params, `CREATE TABLE IF NOT EXISTS` for idempotence
- **TypeScript**: Functional exports (`createServer()`), Bun-native APIs, `import.meta.main` guards

---

## cmdr_coder scope anchor

This repository is governed by the `cmdr_coder` agent (definition: `/home/n0ko/.claude/agents/cmdr_coder.md`).

**All Go and Rust code authorship, modification, refactoring, and code review for `/home/n0ko/Programs/ai/computeCommander/` MUST be delegated to `cmdr_coder`.** This SUPERSEDES any earlier rule that named `golang-coder` or `unix-coder`. See the three pinned rules at the top of `CLAUDE.md` (PINNED ORCHESTRATION RULE, REVIEWER INDEPENDENCE RULE, SPEC LAYOUT RULE).

### Scope boundary

| Path | Owner | Notes |
|------|-------|-------|
| `cmd/` | `cmdr_coder` | See `cmd/agentic_instructions.md`. Cobra command tree (one command per file). |
| `internal/` | `cmdr_coder` | See `internal/agentic_instructions.md`. 23+ internal packages; 7 load-bearing interfaces. |
| `pkg/` | `cmdr_coder` | See `pkg/agentic_instructions.md`. LOCKED at 2 packages (`runtimes`, `integrations`). |
| `plugins/` | `cmdr_coder` | See `plugins/agentic_instructions.md`. Rust focus-watcher; cargo required for `make build`. |
| `bridge/` | `cmdr_coder` | (planned per legacy `specs/go-typescript-bridge.md`) |
| `agents/`, `templates/`, `migrations/`, `scripts/` | `cmdr_coder` | Project-internal config / overlays. |
| `k8s-cluster/` | NOT `cmdr_coder` | TypeScript/Bun. Read-only for `cmdr_coder`. Route TS edits to a TypeScript-capable agent. |
| `SPEC/`, `specs/` | `spec-builder` / `spec-reviewer` | Spec authorship and review. `cmdr_coder` may READ but never write. |
| `.computecommander/scripts/tg-viz.html` | NOT `cmdr_coder` | Pre-existing WIP on `pi` branch — separate workstream; do NOT stage. |

### Spec of record

Authoritative spec for the agent: `SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md`.

Reviews:
- `SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_REVIEW.md` — spec review
- `SPEC/CMDR_CODER_AGENT/REVIEWS/CMDR_CODER_AGENT_FILE_REVIEW.md` — review of the agent file at `~/.claude/agents/cmdr_coder.md`

### Build / Test / Vet (the gates `cmdr_coder` honors)

```bash
cd /home/n0ko/Programs/ai/computeCommander && make build      # requires cargo (Rust focus-watcher)
cd /home/n0ko/Programs/ai/computeCommander && go test ./...
cd /home/n0ko/Programs/ai/computeCommander && make vet
cd /home/n0ko/Programs/ai/computeCommander && make lint       # best-effort, NOT a gate
```

### ob1 / ob2 project tag

`cmdr_coder` writes activity entries tagged `project=computeCommander` (key shape: `computeCommander/activity/<YYYY-MM-DD>/<task_id>.md`). The host CLI today is `ob2` at `/home/n0ko/.local/bin/ob2`; legacy spec text references "ob1" for historical naming. Project-scoping flag: `-project <path>` (single dash). Default project detection is via `.git`, so within this repo no flag is strictly required.

### Read-first cascade

1. `CLAUDE.md` (repo-root)
2. This file (`agentic_instructions.md`)
3. The local `agentic_instructions.md` for the subdirectory you'll touch
4. `SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` (§3 Design Principles, §10 Integration / Orchestrator-Injected Context)
5. The interface or symbol definition for any load-bearing type you'll modify (`AgentRuntime`, `DB`, `MailStore`, `MergeQueue`, `WorktreeManager`, `PaneManager`, `WindowManager`).
