# Agent Quick-Reference

This file is derived from the canonical agent definitions in `.claude/agents/*.md`.
It serves as a quick-reference for orchestrators and supervisors during task routing,
delegation, and review. When in doubt, the individual agent files are authoritative.

---

## Agent Registry

| Agent | Color | Model | Purpose | Permissions |
|-------|-------|-------|---------|-------------|
| cmdr-agent | green | sonnet | Core agent orchestration: spawn, stop, list, guard rules, overlays, runtime adapters, focus-tracker pane detection | Read-Write |
| cmdr-bridge | yellow | sonnet | Dynamic pane coordination: inotifywait chain, CWD files, wrapper scripts, focus-watcher/tracker | Read-Only |
| cmdr-coder | white | sonnet | Primary implementation agent: features, bugs, tests, refactoring across full codebase | Read-Write |
| cmdr-jira | blue | sonnet | Jira integration: REST client, board engine, prompt generation, sync, dark factory | Read-Only |
| cmdr-openbrain-agent | magenta | sonnet | OpenBrain memory: MEMORY.md tracking, sessions-index.json, session picker/switch | Read-Only |
| cmdr-reviewer | orange | sonnet | Code review: Go and Rust quality, patterns, correctness, test coverage | Read-Only |
| cmdr-security | red | sonnet | Security audit: guard rules, isolation, auth, secrets, process/shell safety | Read-Only |
| cmdr-ux-agent | cyan | sonnet | UX specialist: KDL layouts, BubbleTea dashboard, pane naming, terminal rendering | Read-Only |

---

## Permissions Matrix

### Write Agents

**cmdr-agent (Read-Write, SCOPED):**
- RW: `internal/agents/` (spawner, guards, overlay, types, palette)
- RW: `internal/agentic/` (block, gate, holdout, hooks, trace, blueprint, isolation)
- RW: `internal/commands/sling.go`, `internal/commands/stop.go`, `internal/commands/status.go`, `internal/commands/inspect.go`, `internal/commands/coordinator.go` (ONLY these 5 files, NOT the whole directory)
- RW: `pkg/runtimes/` (runtime interface + all adapters)
- RW: `agents/*.yaml` (agent role definitions)
- RW: `plugins/focus-tracker/` (Rust WASM plugin)
- R: Everything else

**cmdr-coder (Read-Write, BROAD):**
- RW: `cmd/`, `internal/`, `pkg/`, `plugins/`, `scripts/`, `agents/`, `templates/`, `migrations/`
- R: `go.mod`, `go.sum`, `Makefile`, `agentic_instructions.md`, `CLAUDE.md`, `specs/`

### Read-Only Agents

**cmdr-bridge:**
- R: `plugins/focus-watcher/`, `plugins/focus-tracker/`, `internal/zellij/`, `scripts/`, `internal/commands/session_picker.go`

**cmdr-jira:**
- R: `pkg/integrations/jira/`, `internal/jiraboard/`, `internal/darkfactory/`, `internal/commands/jira*.go`, `internal/tui/jira_pane.go`, `internal/config/config.go`

**cmdr-openbrain-agent:**
- R: `internal/commands/openbrain.go`, `internal/commands/session_picker.go`, `internal/commands/session.go`, `internal/tui/session_manager.go`, `internal/tui/session_state.go`, `plugins/focus-tracker/src/main.rs`

**cmdr-reviewer:**
- R: ALL `.go` and `.rs` files, `go.mod`, `Makefile`, `agents/*.yaml`

**cmdr-security:**
- R: ALL project files (full codebase access for security audit)

**cmdr-ux-agent:**
- R: `internal/zellij/layout.go`, `internal/zellij/layout_test.go`, `internal/tui/` (all files), `internal/commands/dashboard.go`, `internal/agents/palette.go`, `internal/agents/color_test.go`

### Directory-by-Directory Grid

| Directory | cmdr-agent | cmdr-bridge | cmdr-coder | cmdr-jira | cmdr-openbrain | cmdr-reviewer | cmdr-security | cmdr-ux |
|-----------|-----------|-------------|------------|-----------|----------------|---------------|---------------|---------|
| `cmd/` | R | - | RW | - | - | R | R | - |
| `internal/agents/` | RW | - | RW | - | - | R | R | R* |
| `internal/agentic/` | RW | - | RW | - | - | R | R | - |
| `internal/commands/` | RW* | - | RW | R* | R* | R | R | R* |
| `internal/config/` | R | - | RW | R | - | R | R | - |
| `internal/darkfactory/` | R | - | RW | R | - | R | R | - |
| `internal/jiraboard/` | R | - | RW | R | - | R | R | - |
| `internal/tui/` | R | - | RW | R* | R* | R | R | R |
| `internal/zellij/` | R | R | RW | - | - | R | R | R* |
| `pkg/runtimes/` | RW | - | RW | - | - | R | R | - |
| `pkg/integrations/jira/` | R | - | RW | R | - | R | R | - |
| `plugins/focus-tracker/` | RW | R | RW | - | R* | R | R | - |
| `plugins/focus-watcher/` | R | R | RW | - | - | R | R | - |
| `scripts/` | R | R | RW | - | - | R | R | - |
| `agents/` | RW | - | RW | - | - | R | R | - |
| `templates/` | R | - | RW | - | - | R | R | - |
| `migrations/` | R | - | RW | - | - | R | R | - |

`*` = partial (specific files only, not full directory). See per-agent lists above for exact scopes.

---

## Usage Guidelines

### Decision Tree

```
What kind of work?
│
├─ New feature / bug fix / refactoring / tests
│  └─► cmdr-coder
│
├─ Agent lifecycle (spawn/stop/guard rules/overlays/runtimes)
│  └─► cmdr-agent
│
├─ Code quality review
│  └─► cmdr-reviewer
│
├─ Security audit
│  └─► cmdr-security
│
├─ Jira integration work
│  ├─ Review/design ─► cmdr-jira
│  └─ Implementation ─► cmdr-coder
│
├─ OpenBrain / session management
│  ├─ Review/design ─► cmdr-openbrain-agent
│  └─ Implementation ─► cmdr-coder
│
├─ Dashboard UI / layout / UX
│  ├─ Review/design ─► cmdr-ux-agent
│  └─ Implementation ─► cmdr-coder
│
└─ Pane coordination / wrapper scripts
   ├─ Review/design ─► cmdr-bridge
   └─ Implementation ─► cmdr-coder
```

**Key rule:** Read-only agents REVIEW and PROPOSE. Only cmdr-coder and cmdr-agent IMPLEMENT.

---

## Context-Engine Integration

Keyword-to-agent routing for automatic delegation:

| Keywords | Primary Agent | Fallback |
|----------|---------------|----------|
| spawn, stop, agent, guard, overlay, runtime, capability | cmdr-agent | cmdr-coder |
| pane, wrapper, inotifywait, CWD, focus-watcher, tab hash | cmdr-bridge | cmdr-coder |
| feature, implement, fix, refactor, test, build | cmdr-coder | -- |
| jira, board, sprint, issue, dark factory, prompt | cmdr-jira | cmdr-coder |
| openbrain, memory, session, MEMORY.md, session-switch | cmdr-openbrain-agent | cmdr-coder |
| review, quality, pattern, lint, vet | cmdr-reviewer | -- |
| security, auth, guard, injection, secrets, isolation | cmdr-security | -- |
| layout, KDL, dashboard, UI, pane, render, theme | cmdr-ux-agent | cmdr-coder |

---

## Worker Output Protocol

All agents use the standard worker output format:

```
Done.
Done. Output: <path>
Blocked: <reason>
Error: <desc>
```

No other output. Chatty workers = scoping bug.
