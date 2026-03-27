# agents.md — Project Agent Registry

Documentation file (`agents.md`) at project root that serves as the authoritative reference for orchestrators and developers working on ComputeCommander. Maps all 8 project-specific Claude agents with their roles, permissions, and usage guidelines. Markdown format, zero dependencies.

## Why

ComputeCommander has 8 specialized Claude agents defined in `.claude/agents/*.md`, each with distinct permissions, owned files, and review scopes. Currently:

- **No single-file reference exists.** Orchestrators must read 8 separate agent definition files to understand who can do what. This slows prompt routing and causes incorrect agent selection.
- **Permission boundaries are implicit.** Read-only vs read-write access is buried in individual agent files. A supervisor cannot quickly determine which agent is safe to assign a write task to.
- **Context-engine integration is undocumented.** The `context-inject.py` hook and supervisor agents have no structured mapping from task type to agent selection.
- **New contributors have no onboarding path.** Understanding the agent roster requires reading ~1,200 lines across 8 files.

`agents.md` is a single ~250-line file that covers exactly this surface: agent registry, permissions matrix, and usage guidelines.

## Design Principles

1. **Single source of derived truth.** `agents.md` is derived from `.claude/agents/*.md` definitions. It is a reference document, not the canonical source. When agent definitions change, `agents.md` must be updated to match.
2. **Machine-parseable tables.** All agent data is in markdown tables so hooks and scripts can parse it with simple regex or markdown parsers.
3. **Permissions are explicit.** Every agent's read and write scope is listed per-directory, not described in prose.
4. **No code, no logic.** This is a pure documentation file. It does not execute, import, or generate anything.
5. **Lives at project root.** Same level as `CLAUDE.md` and `agentic_instructions.md` for maximum visibility to orchestrators.

## On-Disk Format

```
computeCommander/
  agents.md             # Agent registry (this spec's deliverable)
  CLAUDE.md             # Project rules
  agentic_instructions.md  # Architecture context
  .claude/agents/       # Canonical agent definitions (source of truth)
    cmdr-agent.md
    cmdr-bridge.md
    cmdr-coder.md
    cmdr-jira.md
    cmdr-openbrain-agent.md
    cmdr-reviewer.md
    cmdr-security.md
    cmdr-ux-agent.md
```

### agents.md

Single markdown file containing 6 sections:

1. **Header** — purpose statement, relationship to `.claude/agents/`
2. **Agent Registry** — table with name, color, model, purpose, permission mode
3. **Permissions Matrix** — per-agent, per-directory read/write grid
4. **Usage Guidelines** — when to invoke which agent, decision tree
5. **Context-Engine Integration** — how `context-inject.py` and supervisors should route prompts
6. **Worker Output Protocol** — standard response format shared by all agents

Example content structure:

```markdown
# ComputeCommander Agent Registry

Authoritative quick-reference for all 8 project agents. Derived from
`.claude/agents/*.md` — those files remain the canonical definitions.

## Agent Registry

| Agent | Color | Model | Purpose | Permissions |
|-------|-------|-------|---------|-------------|
| cmdr-agent | green | sonnet | Agent orchestration, spawn/stop, guards, overlays, runtimes, focus-tracker | Read-Write |
| cmdr-bridge | yellow | sonnet | Dynamic pane coordination, inotifywait chain, CWD files, wrappers | Read-Only |
| cmdr-coder | white | sonnet | Primary implementation — features, bugs, tests, refactoring | Read-Write |
| cmdr-jira | blue | sonnet | Jira integration — REST client, board engine, prompts, dark factory | Read-Only |
| cmdr-openbrain-agent | magenta | sonnet | OpenBrain memory — MEMORY.md tracking, session picker, session switch | Read-Only |
| cmdr-reviewer | orange | sonnet | Code review — Go/Rust quality, patterns, correctness | Read-Only |
| cmdr-security | red | sonnet | Security audit — guards, isolation, auth, secrets, shell injection | Read-Only |
| cmdr-ux-agent | cyan | sonnet | UX specialist — KDL layouts, dashboard UI, pane naming, rendering | Read-Only |

## Permissions Matrix

| Directory | cmdr-agent | cmdr-bridge | cmdr-coder | cmdr-jira | cmdr-openbrain | cmdr-reviewer | cmdr-security | cmdr-ux |
|-----------|-----------|-------------|------------|-----------|----------------|---------------|---------------|---------|
| cmd/ | - | - | RW | - | - | R | R | - |
| internal/agents/ | RW | - | RW | - | - | R | R | - |
| internal/agentic/ | RW | - | RW | - | - | R | R | - |
| internal/commands/ | RW | R | RW | R | R | R | R | R |
| internal/config/ | - | - | RW | R | - | R | R | - |
| ...continues for all directories...

## Usage Guidelines

### Decision Tree

1. **New feature implementation** -> cmdr-coder
2. **Agent lifecycle changes** -> cmdr-agent
3. **Code quality review** -> cmdr-reviewer
4. **Security audit** -> cmdr-security
...

## Context-Engine Integration

### Prompt-to-Agent Routing

| Prompt Keywords | Primary Agent | Fallback |
|----------------|---------------|----------|
| spawn, stop, agent, guard, overlay, runtime | cmdr-agent | cmdr-coder |
| layout, pane, dashboard, KDL, UI, render | cmdr-ux-agent | cmdr-coder |
...

## Worker Output Protocol

All agents use the same output format:
- `Done.`
- `Done. Output: <path>`
- `Blocked: <reason>`
- `Error: <desc>`
```

## Data Model

Not applicable — this is a documentation file with no structured data entities or storage.

## CLI

Not applicable — this file is not executable and exposes no CLI interface.

## JSON Output Format

Not applicable — this is a markdown documentation file with no JSON API or structured output.

## Concurrency Model

Not applicable — single markdown file with no concurrent access concerns beyond normal git merge semantics.

## Migration

Not applicable — no predecessor system. This is a new documentation file.

## Integration

### Supervisor Agents

Supervisors read `agents.md` at the start of orchestration to determine which agent to assign each task to. The file is referenced alongside `CLAUDE.md` and `agentic_instructions.md` in the project root.

| Supervisor Action | agents.md Section |
|-------------------|-------------------|
| Select agent for task | Agent Registry + Usage Guidelines |
| Verify write permissions | Permissions Matrix |
| Route by prompt keywords | Context-Engine Integration |
| Validate worker output | Worker Output Protocol |

### context-inject.py Hook

The `context-inject.py` hook (fired on `UserPromptSubmit`) can reference the Prompt-to-Agent Routing table to suggest or auto-select agents based on prompt content.

```bash
# Supervisor workflow
# 1. Read agents.md to understand the roster
# 2. Parse task description for keywords
# 3. Match against Prompt-to-Agent Routing table
# 4. Verify selected agent has write access to target files (Permissions Matrix)
# 5. Spawn worker with selected agent
```

### Hooks Integration

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "command": "context-inject.py",
        "description": "Injects agent routing context from agents.md into supervisor prompts"
      }
    ]
  }
}
```

## What It Does NOT Do

Explicitly out of scope:

- **Replace canonical definitions.** `.claude/agents/*.md` files remain the source of truth. `agents.md` is a derived reference — not a replacement.
- **Auto-generate from agent files.** No build step or script generates `agents.md`. It is manually maintained. A future task could add a generation script, but that is not in scope here.
- **Define new agents.** This file documents existing agents. Creating new agent definitions belongs in `.claude/agents/`.
- **Enforce permissions at runtime.** The permissions matrix is informational. Actual enforcement is done by Claude's agent system and the guard rules in `internal/agents/guards.go`.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Format | Markdown | Standard for project documentation, parseable by hooks |
| Storage | Git-tracked file at project root | Maximum visibility, versioned with codebase |
| Dependencies | None | Pure documentation file |
| Validation | Manual review | Verified against `.claude/agents/*.md` by code-review agent |

## Project Infrastructure

### Directory Structure

```
computeCommander/
  agents.md                     # NEW: agent registry (this spec)
  CLAUDE.md                     # project rules
  agentic_instructions.md       # architecture context
  .claude/agents/               # canonical agent definitions
  specs/
    agents-md.md                # this spec
    index.md                    # spec index (updated)
```

### Version Management

`agents.md` is versioned via git alongside the rest of the project. No separate version number.

### CHANGELOG.md

Not applicable — no changelog maintained for this project currently.

### CI Workflow

Not applicable — no CI pipeline runs against documentation files.

### Scripts

Not applicable — no build or generation scripts for this file.

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| agents.md | 1 | ~250 |
| specs/index.md update | 1 | ~2 (added lines) |
| **Total** | **2** | **~252** |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Create `agents.md` at project root with full agent registry, permissions matrix, usage guidelines, context-engine routing table, and worker output protocol. Content must be derived from all 8 `.claude/agents/*.md` files. | `.claude/agents/cmdr-agent.md`, `.claude/agents/cmdr-bridge.md`, `.claude/agents/cmdr-coder.md`, `.claude/agents/cmdr-jira.md`, `.claude/agents/cmdr-openbrain-agent.md`, `.claude/agents/cmdr-reviewer.md`, `.claude/agents/cmdr-security.md`, `.claude/agents/cmdr-ux-agent.md`, `agentic_instructions.md` | `agents.md` | -- | `test -f agents.md && wc -l agents.md \| awk '{exit ($1 < 100)}'` |
| T2 | code-review | Review `agents.md` for completeness and accuracy against actual agent definitions in `.claude/agents/`. Verify every agent appears, permissions match source files, and no information is missing or contradictory. | `agents.md`, `.claude/agents/*.md` | -- | T1 | `echo "Review complete"` |
| T3 | unix-coder | Git workflow: stash feat branch changes, checkout main, commit `agents.md`, push, create PR via `gh pr create`. Update `specs/index.md` with this spec entry. | `agents.md`, `specs/index.md` | `specs/index.md` | T2 | `git log --oneline -1 \| grep -q 'agents.md' && gh pr list --head main --json number -q '.[0].number' 2>/dev/null` |

## 16. Dependency Graph

```
Phase 1: [T1]
  T1: Create agents.md from agent definitions

Phase 2 (after Phase 1): [T2]
  T2: Review agents.md for accuracy

Phase 3 (after Phase 2): [T3]
  T3: Git workflow — commit, push, PR
```

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `agents.md` | ~250 | No |

Files modified:

| File Path | Change |
|-----------|--------|
| `specs/index.md` | Add entry for `agents-md.md` spec |

Files deleted: None

## 18. Verification Plan

**Per-task checks:**
- T1: `test -f agents.md && wc -l agents.md | awk '{exit ($1 < 100)}'`
- T2: Manual review output (code-review agent produces findings)
- T3: `git log --oneline -1 | grep -q 'agents.md'`

**Integration check:**

```bash
# Verify agents.md contains all 8 agents
for agent in cmdr-agent cmdr-bridge cmdr-coder cmdr-jira cmdr-openbrain-agent cmdr-reviewer cmdr-security cmdr-ux-agent; do
  grep -q "$agent" agents.md || { echo "MISSING: $agent"; exit 1; }
done

# Verify agents.md contains key sections
for section in "Agent Registry" "Permissions Matrix" "Usage Guidelines" "Context-Engine Integration" "Worker Output Protocol"; do
  grep -q "$section" agents.md || { echo "MISSING SECTION: $section"; exit 1; }
done
```

**Rollback:** `git checkout main -- agents.md` or `git revert HEAD` if already committed.

## 19. Success Criteria (Machine-Verifiable)

- [ ] `test -f agents.md` exits 0 — file exists at project root
- [ ] `wc -l agents.md | awk '{exit ($1 < 100)}'` exits 0 — file has substantial content (>100 lines)
- [ ] `for agent in cmdr-agent cmdr-bridge cmdr-coder cmdr-jira cmdr-openbrain-agent cmdr-reviewer cmdr-security cmdr-ux-agent; do grep -q "$agent" agents.md || exit 1; done` exits 0 — all 8 agents present
- [ ] `grep -q "Permissions Matrix" agents.md` exits 0 — permissions matrix section exists
- [ ] `grep -q "Read-Write" agents.md && grep -q "Read-Only" agents.md` exits 0 — permission modes documented
- [ ] `grep -c "|" agents.md | awk '{exit ($1 < 20)}'` exits 0 — contains substantial table content
- [ ] `grep -q "Worker Output Protocol" agents.md` exits 0 — worker protocol documented
- [ ] `grep -q "agents-md" specs/index.md` exits 0 — spec index updated

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Create agents.md | `unix-coder` | File creation and content authoring from source material |
| Review accuracy | `code-review` | Cross-reference verification against canonical agent definitions |
| Git workflow + PR | `unix-coder` | Git operations, branch management, gh CLI usage |

## Execution Order

```
Phase 1: Content Creation
  +-- T1: Create agents.md (agent: unix-coder)

Phase 2: Quality Gate [blocked by Phase 1]
  +-- T2: Review agents.md (agent: code-review)

Phase 3: Ship [blocked by Phase 2]
  +-- T3: Git stash, checkout main, commit, push, PR (agent: unix-coder)
```

Recommended directive: `/pai` — sequential plan-then-implement pipeline, three tasks with strict ordering.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| agents.md missing an agent | Integration check grep loop fails | Re-read `.claude/agents/` and add missing entry |
| Permissions matrix contradicts source file | code-review agent flags discrepancy | Update matrix to match `.claude/agents/*.md` |
| Git stash fails (no changes to stash) | `git stash` exits non-zero | Skip stash if working tree is clean; proceed to checkout |
| PR creation fails (not on correct branch) | `gh pr create` exits non-zero | Verify branch with `git branch --show-current` before creating PR |
| specs/index.md has merge conflict | `git commit` fails | Resolve conflict manually, re-commit |

## Git Workflow Detail

T3 must execute these steps in order:

```bash
# 1. Stash current feature branch changes
git stash push -m "feat/crush-ui-refactor WIP"

# 2. Switch to main
git checkout main
git pull origin main

# 3. Create agents.md (already created by T1, so copy from stash or recreate)
# NOTE: T1 creates agents.md on whatever branch is current.
# T3 must ensure the file lands on main.

# 4. Update specs/index.md
# Add: | [agents-md.md](agents-md.md) | Agent registry — 8-agent roster, permissions matrix, usage guidelines |

# 5. Commit
git add agents.md specs/index.md specs/agents-md.md
git commit -m "Add agents.md — project agent registry with permissions matrix"

# 6. Push and create PR
git push origin main
# OR: create a short-lived branch, push, PR into main
git checkout -b docs/agents-md
git push -u origin docs/agents-md
gh pr create --title "Add agents.md agent registry" --body "..."

# 7. Restore feature branch
git checkout feat/crush-ui-refactor
git stash pop
```

**Important:** The preferred approach is to create a `docs/agents-md` branch from main, commit there, push, and create a PR into main. This avoids pushing directly to main and allows review.

## Content Specification for agents.md

The unix-coder agent MUST include the following content, derived from reading all 8 `.claude/agents/*.md` files:

### Section 1: Header

```markdown
# ComputeCommander Agent Registry

Authoritative quick-reference for all 8 project-specific Claude agents defined
in `.claude/agents/`. This file is a derived reference — the individual agent
files remain the canonical source of truth.

Last synced: 2026-03-17
```

### Section 2: Agent Registry Table

All 8 agents with: name, color, model, one-line purpose, permission mode (Read-Write or Read-Only).

### Section 3: Permissions Matrix

A grid table with directories as rows and agents as columns. Cell values: `RW` (read-write), `R` (read-only), `-` (no access). Directories to include:

- `cmd/cc/`
- `internal/agents/`
- `internal/agentic/`
- `internal/commands/`
- `internal/config/`
- `internal/gateway/`
- `internal/jiraboard/`
- `internal/darkfactory/`
- `internal/mail/`
- `internal/merge/`
- `internal/platform/db/`
- `internal/tui/`
- `internal/watchdog/`
- `internal/wezterm/`
- `internal/worktree/`
- `internal/zellij/`
- `pkg/runtimes/`
- `pkg/integrations/`
- `plugins/focus-tracker/`
- `plugins/focus-watcher/`
- `agents/`
- `templates/`
- `migrations/`
- `scripts/`
- `specs/`
- `go.mod`, `go.sum`, `Makefile`

### Section 4: Usage Guidelines

Decision tree mapping task types to recommended agents:

| Task Type | Primary Agent | When to Use |
|-----------|---------------|-------------|
| New feature implementation | cmdr-coder | Any code creation or modification |
| Agent lifecycle changes | cmdr-agent | Spawn, stop, guards, overlays, runtime adapters |
| Code quality review | cmdr-reviewer | Post-implementation review, pre-merge check |
| Security audit | cmdr-security | Auth changes, shell injection risk, guard rule modifications |
| UX/layout review | cmdr-ux-agent | Dashboard changes, KDL layout, pane naming |
| Jira integration review | cmdr-jira | Jira client, board engine, dark factory changes |
| Bridge/pane coordination review | cmdr-bridge | Wrapper scripts, CWD files, inotifywait chain |
| Memory/session review | cmdr-openbrain-agent | MEMORY.md, session picker, session switch |

### Section 5: Context-Engine Integration

Prompt keyword to agent routing table for use by `context-inject.py` and supervisor agents:

| Keywords in Prompt | Route To |
|-------------------|----------|
| spawn, stop, agent, guard, overlay, runtime, capability | cmdr-agent |
| layout, pane, KDL, dashboard, UI, render, theme, color | cmdr-ux-agent |
| jira, board, sprint, issue, dark factory, sync | cmdr-jira |
| wrapper, inotifywait, CWD, focus-watcher, focus-tracker, bridge | cmdr-bridge |
| memory, MEMORY.md, session, openbrain, session-switch | cmdr-openbrain-agent |
| review, quality, pattern, correctness, test coverage | cmdr-reviewer |
| security, audit, guard, injection, isolation, auth, secret | cmdr-security |
| implement, build, fix, refactor, test, feature, bug | cmdr-coder |

### Section 6: Worker Output Protocol

Standard output format used by all 8 agents (verbatim from agent definitions).
