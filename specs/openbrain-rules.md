# OpenBrain Rules Overhaul

Information governance for the OpenBrain pane: what gets written, what gets read, how it renders, and performance constraints.

## Why

OpenBrain currently has three problems:

1. **Noise dominates signal.** The Activity section shows agent register/deregister/heartbeat events. These are operational telemetry, not knowledge. A user scanning OpenBrain sees "registered... completed... registered..." instead of meaningful project context like "decided to use zellij layouts instead of PTY embedding" or "root cause was per-session file cleanup."

2. **Reads are absent.** No agent reads OpenBrain at session start. The entire point of a shared memory pane is that new sessions inherit context from previous sessions. Without reads, every session starts cold — agents re-discover problems that were already solved.

3. **Claude entries lack color coding.** Pi agent entries have color in the pane, but Claude agent entries render in default/dim color. The user cannot visually distinguish which runtime produced a memory entry.

## Design Principles

1. **Knowledge over telemetry.** OpenBrain stores decisions, discoveries, and warnings — not lifecycle noise. "Agent X spawned" is telemetry. "Migration 008 adds heartbeat_at column" is knowledge.
2. **Reads are cheap.** Session start reads must complete in <500ms. Query a single indexed table, return at most 20 rows per project.
3. **Writes are intentional.** Only meaningful information gets written. The bridge hook and agents explicitly decide what constitutes an OpenBrain entry.
4. **Color is identity.** Every entry is tagged with the runtime that produced it (claude, pi, gemini, codex, goose) and rendered with that runtime's assigned color.

## What Gets Written (Write Rules)

### Allowed Entry Types

| Category | Example | Source |
|----------|---------|--------|
| `decision` | "Switched from PTY embedding to zellij KDL layout" | Agent writes via `cmdr openbrain write` |
| `discovery` | "Root cause: shared active-sessions.txt across sessions" | Agent writes via `cmdr openbrain write` |
| `warning` | "Do NOT nest zellij sessions — user's zshrc auto-starts zellij" | Agent writes via `cmdr openbrain write` |
| `solution` | "Fixed NULL column scan by using COALESCE in ListSessions query" | Agent writes via `cmdr openbrain write` |
| `context` | "Project uses bubbletea TUI with zellij layout backend" | Agent writes via `cmdr openbrain write` |
| `memory_change` | MEMORY.md section added/modified/deleted | Existing fsnotify watcher (unchanged) |

### Prohibited Entry Types (Filtered Out)

These are **telemetry**, not knowledge. They belong in the Events/Feed pane, not OpenBrain:

- Agent registered / deregistered / completed
- Heartbeat events
- Tool start / tool end
- Session sweep / cleanup
- Token usage metrics
- Mail notifications (agent spawned, agent completed)

### Write API

New CLI command:

```
cmdr openbrain write \
  --type <decision|discovery|warning|solution|context> \
  --summary "Short one-line summary" \
  --detail "Optional longer explanation" \
  --project <project-name>      # defaults to cwd-based detection (see Project Name Derivation)
  --runtime <claude|pi|gemini>  # defaults to $CMDR_RUNTIME or "claude"
  --tags "tag1,tag2"            # optional, for future filtering
  --ttl <duration>              # optional, sets expires_at (e.g., 7d, 24h)
```

This inserts into a new `openbrain_entries` table (see schema below). Agents call this command when they have meaningful knowledge to share.

#### Deduplication

Writes are deduplicated by `(project_name, entry_type, summary)`. If an identical combination already exists, the write is a no-op (INSERT OR IGNORE in SQLite, ON CONFLICT DO NOTHING in Postgres). This prevents duplicate entries from agent retries or concurrent agents discovering the same thing. The `detail`, `runtime`, and `tags` fields are NOT part of the dedup key — only the core identity triple matters.

#### Error Handling

Write failures (DB locked, disk full, invalid type) return non-zero exit code and print to stderr. Writes MUST NOT block agent execution. Callers that want fire-and-forget semantics should use `cmdr openbrain write ... || true`. The command never exits with a fatal/panic — all errors are caught and reported via stderr + exit code 1.

#### TTL Support

The optional `--ttl <duration>` flag (e.g., `--ttl 7d`, `--ttl 24h`) sets `expires_at` to `now + duration`. If omitted, `expires_at` is NULL (entry never auto-expires). Auto-pruning on read deletes entries where `expires_at < now()`.

### Hook Integration

`cmdr-bridge.sh` currently calls `emit_event` and `send_mail` for agent lifecycle. These calls remain unchanged (they feed the Events pane). The bridge does **not** write to `openbrain_entries` — only agents with actual knowledge do, via explicit `cmdr openbrain write` calls.

The existing MEMORY.md watcher in `openbrain.go` continues to detect file-level changes. These are displayed in the Memory section of the pane as before.

### Project Name Derivation

Both `write` and `read` commands accept `--project <name>`. When omitted, the project name is derived automatically using this precedence:

1. Basename of the nearest parent directory containing `.computecommander/` (e.g., `/home/n0ko/Programs/ai/computeCommander/.computecommander/` yields `computeCommander`)
2. If no `.computecommander/` found: basename of the git repository root (from `git rev-parse --show-toplevel`)
3. If no git root: basename of the current working directory

This matches the existing `findLocalDB()` logic in the codebase. The derived name is always the directory basename, never a full path.

## What Gets Read (Read Rules)

### Session Start Read

Every new agent session should read recent OpenBrain entries for the current project. This provides context without requiring the agent to re-explore.

```
cmdr openbrain read \
  --project <project-name>   # defaults to cwd-based detection
  --limit 20                 # max entries to return
  --since 72h                # only entries from last 72 hours
  --types decision,discovery,warning,solution  # filter by type
  --json                     # for programmatic consumption
```

Output (text mode, for injection into agent context):

```
## Recent OpenBrain Entries (computeCommander)

[decision] 2h ago (claude) Switched from PTY embedding to zellij KDL layout
  → Custom VTerm couldn't handle Claude Code's advanced terminal features

[warning] 5h ago (pi) Do NOT nest zellij sessions — user's zshrc auto-starts zellij

[solution] 1d ago (claude) Fixed NULL column scan by using COALESCE in ListSessions
  → Affected ListSessions and findSessionByName queries

[discovery] 2d ago (claude) SessionEnd cleanup used global file, wiped all agents
  → Fix: per-session files active-${CLAUDE_SESSION_ID}.txt
```

Output (JSON mode, for programmatic consumption by hooks):

```json
{
  "success": true,
  "command": "openbrain read",
  "project": "computeCommander",
  "count": 4,
  "entries": [
    {
      "id": 12,
      "type": "decision",
      "summary": "Switched from PTY embedding to zellij KDL layout",
      "detail": "Custom VTerm couldn't handle Claude Code's advanced terminal features",
      "runtime": "claude",
      "agent_name": "unix-coder",
      "tags": "tui,architecture",
      "created_at": "2026-03-18T20:15:00Z",
      "age": "2h"
    }
  ]
}
```

### Hook Integration for Reads

Add a `context-inject.py` integration (or extend existing) that runs `cmdr openbrain read --json --limit 10 --since 72h` at session start and injects the result into agent context. This must:

1. Run only once per session (not per prompt)
2. Complete in <500ms (hard requirement)
3. Fail silently if cmdr is unavailable (graceful degradation)
4. Inject as a clearly delimited block that agents can reference

### Performance Constraints for Reads

| Metric | Target | Rationale |
|--------|--------|-----------|
| Query latency | <100ms | Single indexed SELECT on SQLite |
| Total read time (incl. formatting) | <500ms | Must not delay session start noticeably |
| Max entries returned | 20 | Bounded context injection size |
| Max age filter | 72h default | Stale entries are noise |
| Context size | <4KB | Fits comfortably in agent context window |

## Database Schema

### New Table: `openbrain_entries`

```sql
-- Migration NNN: OpenBrain knowledge entries (use next available number at implementation time)
CREATE TABLE IF NOT EXISTS openbrain_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_name TEXT NOT NULL,
    entry_type TEXT NOT NULL CHECK (entry_type IN ('decision', 'discovery', 'warning', 'solution', 'context')),
    summary TEXT NOT NULL,
    detail TEXT,
    runtime TEXT NOT NULL DEFAULT 'claude',
    agent_name TEXT,
    tags TEXT,                                    -- comma-separated
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT                               -- optional TTL for auto-cleanup
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_openbrain_dedup ON openbrain_entries(project_name, entry_type, summary);
CREATE INDEX IF NOT EXISTS idx_openbrain_project ON openbrain_entries(project_name);
CREATE INDEX IF NOT EXISTS idx_openbrain_project_type ON openbrain_entries(project_name, entry_type);
CREATE INDEX IF NOT EXISTS idx_openbrain_created ON openbrain_entries(created_at);
```

### Postgres Equivalent

```sql
CREATE TABLE IF NOT EXISTS openbrain_entries (
    id BIGSERIAL PRIMARY KEY,
    project_name VARCHAR(256) NOT NULL,
    entry_type VARCHAR(32) NOT NULL CHECK (entry_type IN ('decision', 'discovery', 'warning', 'solution', 'context')),
    summary VARCHAR(512) NOT NULL,
    detail TEXT,
    runtime VARCHAR(64) NOT NULL DEFAULT 'claude',
    agent_name VARCHAR(128),
    tags TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_openbrain_dedup ON openbrain_entries(project_name, entry_type, summary);
CREATE INDEX IF NOT EXISTS idx_openbrain_project ON openbrain_entries(project_name);
CREATE INDEX IF NOT EXISTS idx_openbrain_project_type ON openbrain_entries(project_name, entry_type);
CREATE INDEX IF NOT EXISTS idx_openbrain_created ON openbrain_entries(created_at);
```

## Color Coding

### Runtime Color Map

Every OpenBrain entry carries a `runtime` field. The pane renderer maps runtime to ANSI color:

| Runtime | Color | ANSI Code | Hex (for TUI) | Rationale |
|---------|-------|-----------|----------------|-----------|
| `claude` | Blue | `\033[34m` | `#5DADE2` | Matches Claude branding |
| `pi` | Magenta | `\033[35m` | `#9B59B6` | Already used for pi in pane |
| `gemini` | Cyan | `\033[36m` | `#4ECDC4` | Google/Gemini association |
| `codex` | Green | `\033[32m` | `#82E0AA` | OpenAI green |
| `goose` | Yellow | `\033[33m` | `#FFB347` | Distinct warm color |
| `unknown` | Dim | `\033[2m` | `#888888` | Fallback |

### Entry Type Indicators

Prefix each entry with a type glyph for quick scanning:

| Type | Glyph | Color |
|------|-------|-------|
| `decision` | `D` | Bold white |
| `discovery` | `?` | Cyan |
| `warning` | `!` | Yellow/bold |
| `solution` | `S` | Green |
| `context` | `~` | Dim |

### Rendered Example

```
 Memory  22:15:03 (3 files)
 modified  memory/MEMORY.md  ## Architecture Decisions
           Switched from PTY embedding to zellij KDL layout

─────────────────────────────────────────────
 Knowledge  (6 entries, last 72h)
 D  2h ago  Switched from PTY embedding to zellij       claude
 !  5h ago  Do NOT nest zellij sessions                    pi
 S  1d ago  Fixed NULL column scan with COALESCE        claude
 ?  2d ago  SessionEnd cleanup used global file         claude
 ~  2d ago  Project uses bubbletea + zellij backend     claude
 D  3d ago  KDL layout with pane_frames for dashboard   claude
```

The runtime name on the right is rendered in the runtime's color. The type glyph uses the type color. The summary uses default foreground.

## Pane Layout Changes

### Current Layout (openbrain.go)

1. **Memory** section — MEMORY.md file change watcher (fsnotify)
2. **Activity** section — agent lifecycle events from `events` table

### New Layout

1. **Memory** section — unchanged (MEMORY.md watcher)
2. **Knowledge** section — replaces Activity; reads from `openbrain_entries` table
3. **Activity** section — moved to bottom, collapsed by default, shows last 3 lifecycle events in dim text

The Activity section is not removed entirely (it has diagnostic value) but is visually de-emphasized. The Knowledge section becomes the primary content below Memory.

## On-Disk Format (Files Changed)

```
computeCommander/
  internal/
    platform/db/
      migrations/
        sqlite/
          NNN_openbrain_entries.sql     # new: openbrain_entries table
        postgres/
          NNN_openbrain_entries.sql     # new: postgres equivalent
    commands/
      openbrain.go                      # modified: add write/read subcommands, knowledge section render
      openbrain_test.go                 # modified: tests for write/read/render
  scripts/
    (none — write/read are Go commands, not shell scripts)
```

## Implementation Tasks

### T1: Database Migration (NNN_openbrain_entries.sql)
- Create `openbrain_entries` table in both SQLite and Postgres migrations
- Add UNIQUE index on `(project_name, entry_type, summary)` for dedup
- Add indexes on project_name, entry_type, created_at
- Use next available migration number (currently 008 is latest)
- Estimated: ~30 lines SQL

### T2: `cmdr openbrain write` Subcommand
- Add `write` subcommand to OpenBrainCmd
- Validate entry_type against allowed values
- Detect project_name from cwd (see Project Name Derivation section)
- INSERT OR IGNORE into `openbrain_entries` (dedup by project_name + entry_type + summary)
- Support `--ttl <duration>` flag to set `expires_at`
- Pane refresh is automatic via existing fsnotify on `local.db` — no additional signaling needed
- Error handling: non-zero exit + stderr on failure, never panic
- Estimated: ~100 lines Go

### T3: `cmdr openbrain read` Subcommand
- Add `read` subcommand to OpenBrainCmd
- Query `openbrain_entries` with project, limit, since, types filters
- Output in text mode (human-readable for context injection) and JSON mode
- Performance: query must complete in <100ms
- Estimated: ~100 lines Go

### T4: Knowledge Section Renderer
- Replace `renderAgentActivity` with `renderKnowledgeSection` in pane mode
- Query `openbrain_entries` instead of `events WHERE event_type LIKE 'agent.%'`
- Apply runtime color map and type glyph indicators
- Keep Activity as collapsed/dim section at bottom (last 3 events)
- Estimated: ~60 lines Go

### T5: Color Coding for Runtime Entries
- Add `runtimeColor()` function mapping runtime string to ANSI code
- Add `entryTypeGlyph()` function mapping entry_type to glyph + color
- Apply in both pane render and text output
- Estimated: ~30 lines Go

### T6: Session Start Context Injection
- Extend `context-inject.py` (or create new hook) to call `cmdr openbrain read`
- Run once per session (guard with env var or file flag)
- Inject result block into agent context
- Fail silently if cmdr not available
- Must complete in <500ms total
- Estimated: ~40 lines Python/shell

### T7: Tests
- Test write command inserts correctly
- Test write with invalid entry_type is rejected (non-zero exit)
- Test write dedup: second identical write is a no-op (no duplicate rows)
- Test write with `--ttl 24h` sets `expires_at` correctly
- Test read command filters by project, type, since
- Test read with no entries returns empty result (not error)
- Test read `--since` boundary condition (entry exactly at boundary is included)
- Test read auto-prunes expired entries (where `expires_at < now()`)
- Test render output includes color codes and glyphs
- Test performance: read of 100 entries completes in <100ms
- Test project name derivation from cwd
- Estimated: ~180 lines Go

### T8: Cleanup Pruning
- Add `cmdr openbrain prune --older-than 7d` command
- Also auto-prune entries where `expires_at < now()` on every read
- Prevents unbounded table growth
- Estimated: ~30 lines Go

## Agent Guidelines (for CLAUDE.md / SOUL.md)

Add these rules to agent instructions so they know when and what to write:

```markdown
## OpenBrain Write Rules

Write to OpenBrain when you:
- Make an architectural decision (type: decision)
- Discover the root cause of a bug (type: discovery)
- Find a constraint or gotcha future agents should know (type: warning)
- Solve a problem in a non-obvious way (type: solution)
- Establish important context about the project (type: context)

Do NOT write to OpenBrain for:
- Routine file edits
- Test results (these go to evals)
- Agent lifecycle events (these go to events table)
- Token usage or cost information (these go to metrics)

Format: one-line summary (max 80 chars) + optional detail (max 256 chars).
Command: cmdr openbrain write --type <type> --summary "..." [--detail "..."]
```

## Backwards Compatibility

- MEMORY.md watcher continues unchanged
- `events` table continues receiving lifecycle events from `cmdr-bridge.sh`
- `cmdr openbrain` without subcommand behaves as before (summary mode)
- `cmdr openbrain --pane` adds Knowledge section but retains Memory section
- Agent Activity section demoted to bottom, not removed

## Success Criteria

1. New sessions start with relevant project context from OpenBrain (verifiable by checking agent context contains OpenBrain block)
2. OpenBrain pane shows knowledge entries, not just register/deregister noise
3. Claude entries render with blue color coding, pi with magenta (visual verification)
4. `cmdr openbrain read` completes in <500ms for a table with 1000 entries
5. No increase in session start time beyond 500ms attributable to OpenBrain reads
