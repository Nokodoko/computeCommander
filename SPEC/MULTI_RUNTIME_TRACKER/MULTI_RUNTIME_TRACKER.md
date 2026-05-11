# MULTI_RUNTIME_TRACKER

Bring `icarus` and `gemini` agent sessions into cmdr's Agents pane on parity with Claude. Defines the runtime-agnostic tracker contract that every emitter (Claude bridge, icarus T8 emitter, new gemini bridge) must satisfy, plus the cmdr-side schema, palette, pane-rendering, and reaper changes required to display them without collisions.

This is a **feature spec with migration semantics** — the cmdr-side schema is already runtime-agnostic (migration `008` shipped `idx_sessions_runtime` and `heartbeat_at` for exactly this purpose), but the **writer side** and the **liveness-aware reader side** were never finished. This spec finishes both.

> **NON-GOAL.** This spec does NOT replatform `cmdr-bridge.sh` into a Go daemon, does NOT migrate Claude's existing tracking surface, and does NOT change cmdr's spawn pipeline (`internal/agents/spawner.go`). Those are explicit out-of-scope items, listed in §11.

---

## 2. Why

cmdr has the **schema** for multi-runtime tracking but neither the **writers** nor the **runtime-aware reaper**. Six concrete gaps block icarus and gemini from appearing in the Agents pane reliably:

- **Bridge is hard-wired to Claude.** `~/.claude/hooks/cmdr-bridge.sh` writes the literal string `'claude'` into `sessions.runtime` (function `do_start`, INSERT statement). The `do_start` body falls back to `CLAUDE_SESSION_ID` for session resolution. `map_capability()` switches on Claude agent type names (`unix-coder`, `spec-builder`, `queen`). `resolve_model()` resolves only `claude-opus-*` / `claude-sonnet-*` / `claude-haiku-*`. Any non-Claude session that wrote through this bridge would be mislabeled.
- **State files are session-scoped, not runtime-scoped.** `cmdr-bridge.sh` writes `${CMDR_STATE_DIR}/active-${session_id}.txt` (function `do_start`). Two runtimes that happen to issue the same session ID (UUID4 collision is theoretical, but `sess-$(date +%s)-$$` fallback collisions are real on a busy box) would cross-contaminate. The 2026-03-07 SessionEnd cleanup bomb (per-session-state isolation bug) demonstrates this category of failure has materialized once already.
- **icarus T8 emitter IS merged but does not heartbeat.** Per audit `/tmp/icarus-cmdr-integration-audit.md` (2026-05-02), icarus `internal/integration/cmdr/{events.go (648 LOC), evals.go (404 LOC), tags.go (187 LOC)}` is committed on `main` at icarus@`4bf3fb8` (squash) and `f7107fb` (merge), with HEAD at `2383e2c`. The emitters subscribe to 14 lifecycle events plus 4 eval streams via `pkg/ob1client.WriteEntryAsync` (HTTP `POST` to ob-mcp), tagged with the canonical `agent:icarus + host:<host> + session:<sid> + runtime:icarus` triple. cmdr's SSE relay reads by tag. **However, three categorical gaps remain:** (1) `rg heartbeat` returns zero matches across the entire icarus tree — there is no periodic event between lifecycle transitions, so an idle icarus session emits nothing and cmdr's reaper has no liveness signal during long think-time windows; (2) every `WriteEntryAsync` failure is silently dropped at WARN with no retry / DLQ, so a transient ob-mcp outage loses events permanently; (3) emitters are unconditionally on (`agent_factory.go:470-500`) with no env or config flag to disable them — a stale `OB_API_KEY` on a dev box silently drops every event. The polling fallback (T2.5, repurposed) closes the heartbeat gap.
- **Gemini has its own hook system but no analog file.** `gemini hooks --help` only enumerates `migrate` (Claude → Gemini hook config), but the hook *event surface* is live. `~/.gemini/settings.json` exists but has no `hooks` block. No `agents/gemini.yaml` exists in cmdr.
- **Pane rendering is runtime-blind.** `internal/commands/status.go` (function `printAgentsPane`) Printf format does not include a Runtime column or runtime-aware badge. The `--runtime` filter and the `by_runtime` JSON aggregator exist on the read side, but operators looking at the Agents pane cannot tell a Claude row from an icarus row.
- **Reaper is `last_activity`-keyed and lives inline in `status.go`.** `runStatusPane` (`internal/commands/status.go:185-232`) defines a closure `reapStale` (line 215) that reaps stale agents using only `last_activity`. Migration 008 added `heartbeat_at` *specifically* for non-hook-driven runtimes, but no reaper consumes it. Without a runtime-aware reaper, externally-launched icarus/gemini agents either get false-positive reaped or never get reaped. **The reaper is also not extracted into its own package**, which makes runtime-aware logic harder to test and reason about — this spec extracts it as part of T2.4a.

This spec's surface area is small (~7 source files modified, 4 created, 1 migration, 1 YAML, 1 documented contract). The cost of getting the contract wrong is large (cross-runtime cleanup bombs, palette collisions, mislabeled rows). Spec-first is justified.

---

## 3. Design Principles

1. **Runtime is a first-class column, not a string label.** Every writer to `sessions` MUST set `runtime` to a value from `runtimes.AllRuntimeIDs()`. The DB enforces this via a CHECK constraint added by this spec's migration. Free-form `runtime='claude'` defaults are removed.
2. **Bridge emitters are forks, not a fused multi-runtime script.** Each runtime ships its own emitter, in its own file, with its own state-file namespace. Q1 answer: per-runtime forks. Justification in §"Open Questions" / Q1.
3. **State-file naming is runtime-prefixed, period.** `${CMDR_STATE_DIR}/active-${RUNTIME}-${SESSION_ID}.txt` is the only legal format. The 2026-03-07 SessionEnd cleanup bomb taught us that shared state files cross-contaminate. Make collision impossible by construction.
4. **Session IDs are runtime-prefixed.** `sessions.id` is `${RUNTIME}-${SESSION_ID}-${AGENT_NAME}`, truncated to 64 chars. **Truncation policy: prefix wins, suffix loses.** When the composed string exceeds 64 chars, the trailing portion of `agent_name` is truncated (via the existing `db_id="${db_id:0:64}"` pattern in `do_start`). Collision risk is acceptable because the `${SESSION_ID}` portion (typically a 36-char UUID or ULID) is the entropy source; `agent_name` truncation only collides for two agents in the same session whose names differ only in the truncated tail. This eliminates Q5's cross-runtime collision class entirely and makes log triage trivial (`grep '^icarus-' bridge.log`).
5. **Capability mapping is runtime-aware via Go table, not bash case-stmt.** Bash `case` blocks in three places (claude bridge, gemini bridge, anything else) is a maintenance liability. The cmdr-side `internal/agents/capability.go` exposes a table-driven `MapCapability(runtime, agentType)` consumed by emitters via the existing `cmdr` Bash CLI command (`cmdr capability map --runtime <r> --agent-type <a>`). **Unknown agent-types fall back to `builder` with a `warning` field in `--json` output** (§7); they are NOT errors.
6. **Reaper runs TWO disjoint queries, never COALESCE.** (a) `WHERE runtime='claude' AND last_activity < cutoff`, (b) `WHERE runtime != 'claude' AND heartbeat_at IS NOT NULL AND heartbeat_at < cutoff`. NULL `heartbeat_at` is a no-op for non-claude rows (means no heartbeat received yet — too early to reap, not "ages-old"). Cutoff is 10 minutes (matches the `do_session_start` cutoff already in `cmdr-bridge.sh`, function `do_session_start`).
7. **Pane shows runtime via colored single-character badge, not a column.** A full Runtime column eats horizontal space; a one-char badge prefixed to the agent name conveys the same info in 2 chars. Q7 answer: badge. **`NO_COLOR` fallback: when colors are disabled, render `[C]`/`[I]`/`[G]` (3 chars) instead of a single colored char, so runtime info survives piped output and dumb terminals.**
8. **icarus T8 ships first, gemini ships after, palette reservation is shared.** Phased rollout (§16). Both runtimes share the same `AgentPalette` but the 12-slot palette is partitioned by runtime to prevent a Claude agent and an icarus agent from ever being the same color in the same dashboard view.
9. **Reviewer-independence is enforced.** Per CLAUDE.md, every implementation task is authored by one `cmdr_coder` and reviewed by a fresh `cmdr_coder`. Documented in §15 and §"Agent Assignments".
10. **Reaper extraction precedes runtime-aware logic.** T2.4a extracts `reapStale` from `runStatusPane` into `internal/agents/reaper/reaper.go` with no behavioral change; T2.4 then adds runtime-aware logic to the new package. Two tasks because the refactor and the new behavior are independently reviewable.

### Indentation Rules

Not applicable. This is a feature spec, not a byte-exact rebuild; existing files retain their current indentation, new files follow Go default (gofmt) and shell 4-space conventions.

---

## 4. On-Disk Format

```
computeCommander/
  SPEC/MULTI_RUNTIME_TRACKER/
    MULTI_RUNTIME_TRACKER.md          # this file
    REVIEWS/                          # spec-of-spec + per-phase reviews

  agents/
    builder.yaml                      # existing — runtime: claude
    icarus.yaml                       # existing — runtime: icarus
    gemini.yaml                       # NEW — runtime: gemini (T1.2)
    cmdr.yaml                         # existing — runtime: claude

  cmd/cc/main.go                      # MODIFIED — adds `cmdr capability` and `cmdr agent show` subcommand wiring (T1.1, T1.2a)

  internal/agents/
    capability.go                     # NEW — table-driven MapCapability(runtime, agentType) (T1.1)
    capability_test.go                # NEW (T1.1)
    palette.go                        # MODIFIED — partition palette by runtime (T1.3)
    palette_test.go                   # MODIFIED (T1.3)
    types.go                          # existing — Runtime column already present

  internal/agents/reaper/             # NEW PACKAGE
    reaper.go                         # NEW (T2.4a extraction; T2.4 adds runtime-aware logic)
    reaper_test.go                    # NEW (T2.4a + T2.4)

  internal/agents/poller/             # NEW PACKAGE
    icarus_poller.go                  # NEW — heartbeat-supplement, ICARUS_POLLER=1 gated (T2.5)
    icarus_poller_test.go             # NEW (T2.5)

  internal/commands/
    status.go                         # MODIFIED — extract reaper, add runtime badge (T2.4a, T4.1)
    capability.go                     # NEW — `cmdr capability map` CLI subcommand (T1.1)
    agent.go                          # NEW — `cmdr agent show` CLI subcommand (T1.2a)
    agent_test.go                     # NEW (T1.2a)
    status_test.go                    # MODIFIED (T4.1)

  internal/platform/db/migrations/sqlite/
    012_runtime_check.sql             # NEW — CHECK constraint on sessions.runtime (T1.4)
    012_runtime_check_down.sql        # NEW — rollback companion (T1.4)

  pkg/runtimes/gemini/
    gemini.go                         # MODIFIED — flesh out DeployConfig, ParseTranscript (T3.1)
    gemini_test.go                    # NEW (T3.1)

# Out-of-tree (consumed by cmdr but owned elsewhere):
  ~/.claude/hooks/
    cmdr-bridge.sh                    # MODIFIED — runtime-prefixed state files,
                                      #            runtime-prefixed session IDs,
                                      #            shells out to `cmdr capability map` (T1.5)

  ~/.gemini/hooks/
    cmdr-bridge-gemini.sh             # NEW — gemini-side fork (T3.2)
  ~/.gemini/settings.json             # MODIFIED — hooks block (T3.3)

# Owned by icarus repo (referenced by contract only, NOT created by this spec):
  ~/Programs/ai/icarus/internal/integration/cmdr/
    events.go                         # icarus T8 — MERGED at 4bf3fb8 / f7107fb (audit ref)
    evals.go                          # icarus T8 — MERGED
    tags.go                           # icarus T8 — MERGED
```

### `agents/gemini.yaml`

YAML mirroring `agents/icarus.yaml` shape. Non-trivial fields: `model: gemini-2.5-pro` (Gemini's flagship as of 2026-05); `tools.allowed` is the Gemini CLI tool catalog, NOT Claude's. Verified against `gemini --help` tool listing.

```yaml
name: gemini
capability: builder
description: Google Gemini CLI agent with native tool use and web grounding.

runtime: gemini
model: gemini-2.5-pro

tools:
  allowed:
    - Read
    - Write
    - Edit
    - Glob
    - Grep
    - Bash
    - WebFetch
  blocked:
    - Spawn

constraints:
  - file_scope_enforced
  - no_spawn
  - no_git_push

git:
  allowed:
    - add
    - commit
    - status
    - diff
    - log
  blocked:
    - push
    - force-push
    - reset --hard

file_scope:
  include:
    - "**/*.go"
    - "**/*.md"
    - "**/*.yaml"
  exclude:
    - "**/vendor/**"
    - "**/node_modules/**"
    - ".git/**"
```

### Tracker Protocol Contract (the new wire format)

Every emitter (Claude, icarus T8, gemini) writes the following on agent lifecycle events. This is the **single source of truth** for what cmdr expects.

#### Spawn — INSERT into `sessions`

```sql
INSERT OR REPLACE INTO sessions (
    id,                  -- "{runtime}-{session_id}-{agent_name}", max 64 chars (truncated tail)
    agent_name,          -- short name, max 40 chars
    capability,          -- one of: supervisor, scout, builder, reviewer, lead,
                         --         merger, coordinator, monitor
    worktree_path,       -- absolute path or '' if N/A
    branch_name,         -- git branch or ''
    task_id,             -- runtime-specific task identifier or '{runtime}-task'
    zellij_pane,         -- '' if not embedded
    state,               -- 'booting' on register, 'working' once ready
    pid,                 -- emitter process pid; 0 if unknown
    parent_agent,        -- '' for top-level
    depth,               -- 0 for top-level
    run_id,              -- '' or runtime-specific run grouping
    started_at,          -- ISO 8601 UTC
    last_activity,       -- ISO 8601 UTC, equals started_at on spawn
    escalation_level,    -- 0
    stalled_since,       -- NULL
    transcript_path,     -- absolute path to runtime's transcript or ''
    runtime,             -- one of: claude, gemini, codex, pi, goose, icarus
    color_index,         -- 0..11; allocated via cmdr palette (see §3 rule 8)
    color_hex,           -- "#RRGGBB" matching color_index
    model,               -- runtime-specific full model ID
    session_name,        -- short display name, max 60 chars
    heartbeat_at         -- ISO 8601 UTC, equals started_at on spawn (REQUIRED for non-Claude)
) VALUES ( ... );
```

**Truncation note (Q5 caveat).** The `id` field is capped at 64 chars. With `claude-${UUID(36)}-${agent_name(40)}` = 83 chars worst case, the bridge applies `db_id="${db_id:0:64}"` (existing pattern in `do_start`), which truncates from the right (agent_name suffix). Collision risk: two agents with names differing only in the truncated tail collide; in practice agent_names are short (<20 chars) and this is acceptable.

#### Heartbeat — UPDATE `sessions.heartbeat_at` every 30s while `state='working'`

```sql
UPDATE sessions
SET heartbeat_at = '{now_iso}',
    last_activity = COALESCE(NULLIF(last_activity, ''), '{now_iso}')
WHERE id = '{runtime}-{session_id}-{agent_name}';
```

Cadence: 30 seconds for icarus/gemini. **Configurable via `CMDR_HEARTBEAT_INTERVAL` env var (default `30`, integer seconds).** Claude is exempt (its tool-driven `last_activity` updates are dense enough). Reaper cutoff: 10 minutes since last heartbeat (per §3 rule 6, two disjoint queries).

#### Activity — UPDATE `sessions.last_activity` on every tool call (matches Claude bridge `emit_event` callsite in `do_post_tool`)

```sql
UPDATE sessions
SET last_activity = '{now_iso}',
    heartbeat_at  = '{now_iso}'
WHERE id = '{runtime}-{session_id}-{agent_name}';
```

#### Completion — UPDATE state + write `metrics` row (matches Claude bridge function `do_stop`)

```sql
UPDATE sessions
SET state = 'completed', last_activity = '{now_iso}'
WHERE id = '{runtime}-{session_id}-{agent_name}';

INSERT INTO metrics
    (agent_name, task_id, capability, started_at, completed_at,
     input_tokens, output_tokens, model_used)
VALUES (...);
```

#### Events emission (matches `emit_event` helper in `cmdr-bridge.sh`)

Required event types per emitter:
- `spawn` (level=info) at registration
- `tool_end` (level=info) on each tool completion
- `session_end` (level=info) on completion
- `heartbeat` (level=debug) optional, at heartbeat cadence

#### Mail emission (matches `send_mail` helper in `cmdr-bridge.sh`)

Required mail on lifecycle:
- `from='{runtime}-bridge'`, `to='supervisor'`, `subject='Agent spawned: {agent_name}'`, `priority='normal'` on spawn
- `from='{runtime}-bridge'`, `to='supervisor'`, `subject='Agent completed: {agent_name}'`, `priority='low'` on completion

#### State file naming (collision-proof)

```
${CMDR_STATE_DIR}/active-{runtime}-{session_id}.txt          # active sessions
${CMDR_STATE_DIR}/agent-id-{runtime}-{agent_name}.map        # ID resolution sidecar
```

`{runtime}` is one of `claude|gemini|icarus`. Adding new runtimes adds new files; never reuse an existing namespace.

#### Pane signaling (matches `signal_panes` helper in `cmdr-bridge.sh`)

```bash
# Send SIGUSR1 to status-pane.pid and dashboard.pid for instant refresh.
for pidfile in "$state_dir/status-pane.pid" "$state_dir/dashboard.pid"; do
    [ -f "$pidfile" ] && kill -USR1 "$(cat "$pidfile")" 2>/dev/null || true
done
```

This is unchanged. Every emitter must signal both pidfiles after every DB write that affects display.

---

## 5. Data Model

### Sessions row (after this spec's migration)

```typescript
interface AgentSession {
  // Identity (runtime-prefixed)
  id: string;              // "{runtime}-{session_id}-{agent_name}", max 64 (truncated tail)
  agent_name: string;      // max 40
  capability: string;      // enum: supervisor|scout|builder|reviewer|lead|merger|coordinator|monitor
  runtime: RuntimeID;      // CHECK enum (NEW): claude|gemini|codex|pi|goose|icarus

  // Lifecycle
  state: SessionState;     // booting|working|completed|stalled|zombie
  pid: number;
  started_at: string;      // ISO 8601 UTC
  last_activity: string;   // ISO 8601 UTC (Claude-driven via tool hooks)
  heartbeat_at: string?;   // ISO 8601 UTC (icarus/gemini-driven, every 30s; NULL for fresh non-Claude rows until first heartbeat fires)
  stalled_since: string?;
  escalation_level: number;

  // Hierarchy
  parent_agent: string;    // '' for top-level
  depth: number;
  run_id: string;
  task_id: string;

  // Display
  color_index: number;     // 0..11
  color_hex: string;       // "#RRGGBB"
  model: string;
  session_name: string;    // max 60
  worktree_path: string;
  branch_name: string;
  zellij_pane: string;
  transcript_path: string;
}
```

### Lifecycle (unchanged across runtimes)

```
booting --> working --> completed
   ^           |           ^
   |           v           |
   |       stalled --------+
   |           |
   |           v
   +------- zombie  (process gone, DB row stuck)
```

Reaper transitions: `working/stalled --> completed` on staleness. Cutoff per runtime, computed via TWO disjoint SQL queries (see §3 rule 6):

| Runtime | Staleness Signal | Cutoff | Reaper Query |
|---------|------------------|--------|--------------|
| claude  | `last_activity`  | 10 min | `WHERE runtime='claude' AND last_activity < cutoff` |
| icarus  | `heartbeat_at`   | 10 min | `WHERE runtime != 'claude' AND heartbeat_at IS NOT NULL AND heartbeat_at < cutoff` |
| gemini  | `heartbeat_at`   | 10 min | (same as icarus) |
| codex   | `heartbeat_at`   | 10 min | (same as icarus) |
| pi      | `heartbeat_at`   | 10 min | (same as icarus) |
| goose   | `heartbeat_at`   | 10 min | (same as icarus) |

NULL `heartbeat_at` for non-claude rows is a no-op (means no heartbeat received yet — fresh row, not stale). The reaper MUST NOT use `COALESCE(heartbeat_at, last_activity)` because that would re-introduce false-positive reaping for old Claude rows whose heartbeat_at is permanently NULL.

### Capability mapping (table-driven, runtime-aware)

`internal/agents/capability.go` exports:

```go
// MapCapability returns the cmdr capability for a runtime-specific agent type.
// Used by all emitters via `cmdr capability map --runtime <r> --agent-type <a>`.
// Unknown agent_types fall back to CapabilityBuilder; the CLI surfaces this as
// a non-fatal `warning` field in --json output (§7) but exit code is 0.
func MapCapability(runtime runtimes.RuntimeID, agentType string) (Capability, bool) {
    if m, ok := capabilityMap[runtime]; ok {
        if c, ok := m[strings.ToLower(agentType)]; ok {
            return c, true // exact match
        }
    }
    return CapabilityBuilder, false // fallback
}

var capabilityMap = map[runtimes.RuntimeID]map[string]Capability{
    runtimes.RuntimeClaude: {
        "supervisor": CapabilitySupervisor, "general-purpose": CapabilitySupervisor,
        "explore": CapabilityScout, "scout": CapabilityScout,
        "spec-builder": CapabilityBuilder, "unix-coder": CapabilityBuilder,
        "cmdr_coder": CapabilityBuilder, "icarus_coder": CapabilityBuilder,
        "spec-reviewer": CapabilityReviewer, "code-review": CapabilityReviewer,
        "tech-lead": CapabilityLead,
        "merge-manager": CapabilityMerger,
        "queen": CapabilityCoordinator, "janitor": CapabilityCoordinator,
        "datadog-observability-sme": CapabilityMonitor,
    },
    runtimes.RuntimeIcarus: {
        // icarus agent types per ~/Programs/ai/icarus/internal/agents/
        "default": CapabilityBuilder,
        "reasoning": CapabilityBuilder,
        "research": CapabilityScout,
        "verifier": CapabilityReviewer,
    },
    runtimes.RuntimeGemini: {
        // Gemini agent types are user-defined; default to builder.
        "default": CapabilityBuilder,
    },
}
```

### Palette partitioning

`internal/agents/palette.go` adds a runtime-aware allocator. The 12-slot palette is preserved but the **assignment policy** changes:

- Slots 0-3 (`Coral`, `Teal`, `Amber`, `Violet`): claude
- Slots 4-7 (`Sky`, `Lime`, `Rose`, `Indigo`): icarus
- Slots 8-11 (`Peach`, `Mint`, `Salmon`, `Lavender`): gemini

This is **partition-by-runtime** so a Claude agent and an icarus agent in the same dashboard view are guaranteed visually distinct. Within a runtime, agents round-robin through the partition (4 slots per runtime, wraps at the 5th concurrent agent).

```go
// AssignColorForRuntime returns a palette color partitioned by runtime.
func AssignColorForRuntime(runtime runtimes.RuntimeID, spawnIndex int) AgentColor {
    base := runtimePartitionBase(runtime) // 0, 4, or 8
    return AgentPalette[base+(spawnIndex%4)]
}

func runtimePartitionBase(r runtimes.RuntimeID) int {
    switch r {
    case runtimes.RuntimeIcarus: return 4
    case runtimes.RuntimeGemini: return 8
    default: return 0 // claude + everything else
    }
}
```

Existing per-agent-type color overrides (`map_agent_color` helper in `cmdr-bridge.sh`) continue to apply for Claude only — they are documented Claude-internal conventions.

---

## 6. CLI

Binary: `cmdr` (existing; this spec adds two subcommands and one badge behavior).

### Capability mapping (NEW, T1.1)

```
cmdr capability map                            Resolve runtime+agent-type to cmdr capability
  --runtime <id>          (required)           One of: claude, gemini, codex, pi, goose, icarus
  --agent-type <name>     (required)           Runtime-specific agent type
  --json                                       Structured output

# Example:
cmdr capability map --runtime icarus --agent-type reasoning
# Output: builder

cmdr capability map --runtime icarus --agent-type unknown-type
# Output: builder         (fallback, exit 0)

cmdr capability map --runtime icarus --agent-type reasoning --json
# Output: {"success": true, "command": "capability map", "capability": "builder",
#          "runtime": "icarus", "agent_type": "reasoning"}
```

This subcommand is what bridges shell out to. It replaces the `case` statement in `map_capability` (function in `cmdr-bridge.sh`) with a single source of truth.

### Agent inspection (NEW, T1.2a)

```
cmdr agent show <name>                         Print agent definition resolved from agents/<name>.yaml
  --json                                       Structured output
  # Reads agents/<name>.yaml from $PWD or $CMDR_PROJECT_ROOT, yaml-unmarshals, prints fields.
  # Exit 0 on found, 1 on not found, 2 on parse error.

# Example:
cmdr agent show gemini
# name: gemini
# runtime: gemini
# capability: builder
# model: gemini-2.5-pro
# ...

cmdr agent show gemini --json
# Output: {"success": true, "command": "agent show", "name": "gemini",
#          "runtime": "gemini", "capability": "builder", "model": "gemini-2.5-pro", ...}

cmdr agent show nonexistent --json
# Output: {"success": false, "command": "agent show", "error": "agents/nonexistent.yaml not found"}
# Exit code: 1
```

This subcommand is a small primitive (~80 LOC + test) that several verify commands and success criteria depend on. T1.2a writes the cobra wiring + handler; T1.2 (gemini.yaml) depends on it for verification.

### Status pane runtime filter (existing, unchanged)

```
cmdr status                                    Show fleet status
  --runtime <id>                               Filter to one runtime
  --pane                                       Pane mode (long-running)
  --json                                       Structured output
```

### Status pane runtime badge (NEW behavior, no flag, T4.1)

`cmdr status --pane` (and `cmdr status` non-pane) prefixes each row with a single runtime-colored character badge:

```
  C  unix-coder       builder    ● working    1m23s   sonnet-4-6     a1b2c3d4   T-1234
  I  reasoning        builder    ● working    2m05s   sonnet-4-7     i2d4e6f8   icarus-001
  G  default          builder    ● working      45s   gemini-2.5-pro 3a5b7c9d   gem-007
```

When `NO_COLOR` env is set or stdout is not a TTY, the badge falls back to bracket form:

```
  [C]  unix-coder       builder    working    1m23s   sonnet-4-6     a1b2c3d4   T-1234
  [I]  reasoning        builder    working    2m05s   sonnet-4-7     i2d4e6f8   icarus-001
  [G]  default          builder    working      45s   gemini-2.5-pro 3a5b7c9d   gem-007
```

| Runtime | Badge (color) | Badge (NO_COLOR) | Color hex (palette base) |
|---------|--------------|------------------|--------------------------|
| claude  | `C`          | `[C]`            | `#FF6B6B` (Coral)   |
| icarus  | `I`          | `[I]`            | `#5DADE2` (Sky)     |
| gemini  | `G`          | `[G]`            | `#FFDAB9` (Peach)   |
| codex   | `X`          | `[X]`            | `#FFB347` (Amber)   |
| pi      | `P`          | `[P]`            | `#9B59B6` (Violet)  |
| goose   | `O`          | `[O]`            | `#82E0AA` (Lime)    |

The badge is rendered via existing `colorizeAgent` machinery; the format string in `printAgentsPane` gains a `%s ` prefix for the badge, with the `NO_COLOR`-aware renderer.

---

## 7. JSON Output Format

`cmdr capability map --json`:

Success (exact match):
```json
{
  "success": true,
  "command": "capability map",
  "runtime": "icarus",
  "agent_type": "reasoning",
  "capability": "builder"
}
```

Success (fallback for unknown agent_type):
```json
{
  "success": true,
  "command": "capability map",
  "runtime": "icarus",
  "agent_type": "unknown-type",
  "capability": "builder",
  "warning": "agent_type not found in runtime mapping; defaulted to builder"
}
```

Error (unknown runtime — runtime is hard-validated against `runtimes.AllRuntimeIDs()`):
```json
{
  "success": false,
  "command": "capability map",
  "error": "unknown runtime: \"foobar\""
}
```

`cmdr agent show --json`:

Success:
```json
{
  "success": true,
  "command": "agent show",
  "name": "gemini",
  "runtime": "gemini",
  "capability": "builder",
  "model": "gemini-2.5-pro",
  "tools": {
    "allowed": ["Read", "Write", "Edit", "Glob", "Grep", "Bash", "WebFetch"],
    "blocked": ["Spawn"]
  }
}
```

Error (file not found):
```json
{
  "success": false,
  "command": "agent show",
  "error": "agents/nonexistent.yaml not found"
}
```

`cmdr status --json` (extended; existing fields unchanged, `by_runtime` already populated by `runStatusPane`):

```json
{
  "success": true,
  "command": "status",
  "sessions": [...],
  "by_runtime": {"claude": 3, "icarus": 2, "gemini": 1}
}
```

---

## 8. Concurrency Model

**Strategy: per-runtime advisory locks + atomic SQL with `INSERT OR REPLACE` + per-runtime state-file namespace.**

```
DB lock file:    .computecommander/cmdr.lock      (existing, unchanged)
Bridge lock:     ${CMDR_STATE_DIR}/.bridge-lock-{runtime}    (NEW: per-runtime)
Stale after:     2s flock timeout (matches existing bridge constant)
SQLite timeout:  5000ms (matches existing -cmd ".timeout 5000")
Heartbeat:       30s cadence per emitter (configurable: CMDR_HEARTBEAT_INTERVAL)
```

Implementation:

1. Each emitter acquires `${CMDR_STATE_DIR}/.bridge-lock-{runtime}` (NOT the shared `.bridge-lock`) via `flock`. This prevents two icarus agents from racing each other but does NOT block a concurrent Claude agent — they hold separate locks.
2. SQLite `BEGIN IMMEDIATE` is implicit via `INSERT OR REPLACE`; SQLite WAL mode handles concurrent writers across processes.
3. State file writes are append-only (`>>` for `active-...txt`) and overwrite-only (`>` for `agent-id-*.map`). No read-modify-write on state files except the `sed -i` removal in `do_stop`, which is line-keyed and idempotent.

### Atomic Writes

State files use the existing pattern (no change): direct write, no temp+rename. Race window is bounded by the per-runtime flock.

DB writes use SQLite's atomic transaction guarantees. `INSERT OR REPLACE` is the existing idiom — preserved.

### Conflict Resolution

- **Same agent registered twice**: `INSERT OR REPLACE` keyed on `id` (which is `{runtime}-{session_id}-{agent_name}`); second insert wins. This is existing Claude behavior, preserved.
- **Concurrent stop signals across runtimes**: each runtime's emitter only ever writes rows where `id LIKE '{runtime}-%'`. The `do_stop` sweep MUST be modified to add `runtime = '{runtime}'` to its WHERE clause. This is an explicit task (T2.3, §15).
- **Reaper writes vs emitter writes**: reaper updates only `state` to `completed` and only when staleness exceeds cutoff. Emitter heartbeat writes `last_activity` + `heartbeat_at`. These are disjoint columns; last-write-wins on `state` is acceptable because `completed` is terminal.
- **Three concurrent runtimes (claude + icarus + gemini)**: SQLite WAL + 5000ms busy_timeout handles N=3 concurrent writers comfortably on consumer hardware. If contention exceeds 5s, the existing bridge silently drops the row (logged as WARN). Multi-runtime regression test T4.2 must run for ≥30s with all three runtimes spawning simultaneously to flush this out.
- **Poller vs T8 emitter writing same icarus row**: the poller (T2.5) skips rows where `heartbeat_at` is set within the last 60s (means T8 owns it via SSE relay). Collision impossible by construction; no conflict-resolution arbitration needed.

---

## 9. Migration

| Component | Current | Target |
|-----------|---------|--------|
| `sessions.runtime` column | `TEXT NOT NULL DEFAULT 'claude'`, no constraint | Same + `CHECK (runtime IN ('claude','gemini','codex','pi','goose','icarus'))` via migration `012` |
| `cmdr-bridge.sh` runtime column | Hard-coded `'claude'` (function `do_start`, INSERT) | Reads `${CMDR_RUNTIME:-claude}` env, defaults to claude |
| `cmdr-bridge.sh` session ID prefix | None (function `do_start`: `db_id="${session_id}-${agent_name}"`) | `db_id="${CMDR_RUNTIME}-${session_id}-${agent_name}"` (still truncated to 64 chars) |
| State file naming | `active-${session_id}.txt`, `agent-id-${agent_name}.map` | `active-${CMDR_RUNTIME}-${session_id}.txt`, `agent-id-${CMDR_RUNTIME}-${agent_name}.map` |
| `map_capability` (function in bridge) | Inline bash `case` | Shells out to `cmdr capability map --runtime "$CMDR_RUNTIME" --agent-type "$1" 2>/dev/null \|\| echo "builder"` |
| `resolve_model` (function in bridge) | Inline, claude-only | Inline, claude-only (unchanged — gemini/icarus emitters carry their own resolvers) |
| Reaper location | Inline closure `reapStale` in `runStatusPane` (`internal/commands/status.go:215`) | Extracted into `internal/agents/reaper/reaper.go` (T2.4a), then runtime-aware (T2.4) |
| Reaper logic | `last_activity`-keyed for all runtimes | TWO disjoint queries: claude→last_activity, non-claude→heartbeat_at (NULL-safe) |
| Pane format string (`printAgentsPane`) | No runtime indicator | Runtime badge (one char colored, `[X]` in NO_COLOR) prefixed |
| `cmdr-bridge.sh` `do_session_start` sweep | Sweeps ALL `working` rows older than cutoff | Sweeps only rows WHERE `runtime='claude'` AND `last_activity < cutoff`. Other runtimes use heartbeat-based reaping in cmdr's Go reaper. |
| `cmdr-bridge.sh` `do_cleanup` per-session file | `active-${session_id}.txt` | `active-claude-${session_id}.txt` |

### Cmdr-bridge.sh change locus (function-keyed, NOT line-keyed)

To keep the spec robust against bridge drift, T1.5 changes are described by function name. Workers MUST `grep -n` for the function declaration and apply changes within its body.

| Function | Change |
|----------|--------|
| Top of file (preamble) | Add `CMDR_RUNTIME="${CMDR_RUNTIME:-claude}"` |
| Top of file (preamble) | Change `LOCK_FILE` to `${CMDR_STATE_DIR}/.bridge-lock-${CMDR_RUNTIME}` |
| `map_capability()` body | Replace `case` block with `cmdr capability map --runtime "$CMDR_RUNTIME" --agent-type "$1" 2>/dev/null \|\| echo "builder"` |
| `do_start()`: db_id assignment | Replace literal `"${session_id}-${agent_name}"` with `"${CMDR_RUNTIME}-${session_id}-${agent_name}"`; preserve existing `db_id="${db_id:0:64}"` truncation immediately after |
| `do_start()`: INSERT VALUES | Replace literal `'claude'` with `'${CMDR_RUNTIME}'` (two occurrences in INSERT body) |
| `do_start()`: state file write | `echo "$db_id" >> "${CMDR_STATE_DIR}/active-${CMDR_RUNTIME}-${session_id}.txt"` |
| `do_start()`: sidecar write | `echo "$db_id" > "${CMDR_STATE_DIR}/agent-id-${CMDR_RUNTIME}-${agent_name}.map"` |
| `do_stop()`: sidecar reads | All sidecar reads use `agent-id-${CMDR_RUNTIME}-${agent_name}.map` |
| `do_stop()`: WHERE clauses | All `WHERE` clauses gain `AND id LIKE '${CMDR_RUNTIME}-%'` for fallback queries (preserve existing fallbacks for backwards compat with pre-migration row IDs) |
| `do_stop()`: sed-deletion | Targets `active-${CMDR_RUNTIME}-${session_id}.txt` |
| `do_session_start()` sweep | UPDATE WHERE clause gains `AND runtime='${CMDR_RUNTIME}'` |
| `do_cleanup()` | Targets `active-${CMDR_RUNTIME}-${session_id}.txt` |

The reviewer (T1.5R) MUST `grep -nE 'literal\s*claude|active-\$\{session_id|agent-id-\$\{agent_name'` over the post-T1.5 bridge and confirm zero matches outside of the `CMDR_RUNTIME=...:-claude` default and any deliberately-preserved backwards-compat fallback strings.

### One-time backfill (no script needed)

Existing rows in `sessions` already have `runtime='claude'` (the default). No row migration is required because:
1. The migration `012` CHECK constraint accepts `claude` as a valid value (it's the first member of the IN clause).
2. Existing rows have `id` like `${session_id}-${agent_name}` (no `claude-` prefix). New Claude inserts will have `id='claude-${session_id}-${agent_name}'`. **This means `id` cardinality changes for Claude rows going forward, but old rows retain their old IDs.** The `do_stop` resolver (function in bridge) already tries multiple fallback strategies — we add one more (runtime-prefixed sweep) WITHOUT removing the existing fallbacks, preserving backwards compatibility for in-flight pre-migration sessions during the rollout window.
3. After 24h, running `do_session_start` will reap any pre-migration rows still stuck in `working`. No manual cleanup required.

### Migration 012 SQLite table-rebuild fidelity

Migration 012 uses the SQLite table-rebuild pattern (CHECK can't be added via `ALTER TABLE`). The `sessions_new` skeleton MUST enumerate every column from the live schema (post-`011_linkedin.sql`), NOT a "... all existing columns ..." placeholder. T1.4 author MUST:

1. Run `sqlite3 /tmp/probe.db < 0*.sql` over **all migrations 001..011** (in order).
2. Run `PRAGMA table_info(sessions);` against `/tmp/probe.db`.
3. Copy the column list verbatim into `012_runtime_check.sql` `CREATE TABLE sessions_new`.
4. Run `INSERT INTO sessions_new SELECT * FROM sessions;` (column-positional copy).
5. Verify post-migration `PRAGMA table_info(sessions);` matches pre-migration column-for-column except for the `runtime` CHECK clause.

T1.4 verify command MUST apply ALL prior migrations (`for f in /path/to/migrations/0*.sql; do sqlite3 ... < "$f"; done`), not just a hand-picked subset.

### Cross-runtime ID rollout safety

| Scenario | Behavior |
|----------|----------|
| Pre-migration Claude row (id=`abc-agent1`) still working when bridge upgrades | `do_stop` existing fallback (`SELECT id FROM sessions WHERE id LIKE '${session_id}-%'`) still resolves it. |
| Post-migration Claude row (id=`claude-abc-agent1`) | New fallback: `SELECT id FROM sessions WHERE id LIKE 'claude-${session_id}-%'`. |
| icarus row written by T8 emitter (via ob-mcp SSE relay, id=`icarus-sess-1234-reasoning`) | Resolved only by cmdr's SSE relay consumer; never touched by claude bridge (its WHERE clauses gain `AND id LIKE 'claude-%'`). |
| gemini row written by gemini bridge | Same as icarus, namespace `gemini-`. |

---

## 10. Integration

### Claude bridge (`~/.claude/hooks/cmdr-bridge.sh`)

Modified to be runtime-aware (defaults to claude, but reads `CMDR_RUNTIME`). Specific changes are enumerated by function name in §9 "Cmdr-bridge.sh change locus" — workers grep for the function, not line numbers.

### Gemini bridge (`~/.gemini/hooks/cmdr-bridge-gemini.sh`, NEW)

Forked from the modified Claude bridge with:

- `CMDR_RUNTIME=gemini` hard-coded (no env override).
- `parse_payload` adapted to Gemini's hook payload shape. Gemini's hook event surface (per Q2 investigation, confirmed via `gemini hooks --help`): `PreToolUse`, `PostToolUse`, `SessionStart`, `SessionEnd`. **No `SubagentStart`/`SubagentStop`** — Gemini does not have a subagent concept analog to Claude's `Task` tool. Each `gemini` invocation is its own session.
- Spawn registration fires on `SessionStart` (not SubagentStart).
- Completion fires on `SessionEnd` (not SubagentStop).
- Heartbeat goroutine (background process, see "Heartbeat daemon" below) maintains `heartbeat_at`. **Cadence read from `CMDR_HEARTBEAT_INTERVAL` env (default 30s).**
- Model resolution uses `GEMINI_MODEL` env var, fallback to `gemini-2.5-pro`.

#### Gemini hooks settings.json

`~/.gemini/settings.json` gains:

```json
{
  "hooks": {
    "SessionStart": [
      { "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh session-start" }
    ],
    "PostToolUse": [
      { "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh post-tool" }
    ],
    "SessionEnd": [
      { "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh stop" }
    ]
  }
}
```

#### Heartbeat daemon (gemini)

Because Gemini lacks a continuous activity hook (no SubagentStart, no in-progress signals), the gemini bridge spawns a configurable-interval heartbeat goroutine on `SessionStart`. Implementation: a detached `setsid bash` loop that updates `heartbeat_at` until the session row's state becomes `completed`.

```bash
# Spawned by do_start in gemini bridge
heartbeat_loop() {
    local interval="${CMDR_HEARTBEAT_INTERVAL:-30}"
    local max_iterations=$((86400 / interval))  # 24h escape hatch
    local i=0
    while [ "$i" -lt "$max_iterations" ]; do
        sleep "$interval"
        local state
        state=$(sqlite3 "$cmdr_db" "SELECT state FROM sessions WHERE id='$db_id' LIMIT 1;" 2>/dev/null || echo "")
        if [ "$state" != "working" ] && [ "$state" != "booting" ]; then
            exit 0
        fi
        local now
        now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
        sqlite3 "$cmdr_db" "UPDATE sessions SET heartbeat_at='$now' WHERE id='$db_id';" 2>/dev/null
        i=$((i+1))
    done
}
setsid bash -c "$(declare -f heartbeat_loop); heartbeat_loop" </dev/null >/dev/null 2>&1 &
disown
```

The 24h escape hatch prevents goroutine leaks if the session row is deleted out from under the loop or the state column is corrupted.

### icarus T8 contract (cmdr-side defines, icarus-side IMPLEMENTS — MERGED)

The icarus repo's T8 (`internal/integration/cmdr/{events.go, evals.go, tags.go}`) is **MERGED on icarus@`main`** at commits `4bf3fb8` (squash) and `f7107fb` (merge), with HEAD at `2383e2c` (audit ref: `/tmp/icarus-cmdr-integration-audit.md`, 2026-05-02). The emitters write to ob-mcp via HTTP (`pkg/ob1client.WriteEntryAsync`); cmdr's SSE relay reads by tag. **The polling fallback (T2.5) is NOT a substitute for T8 — it is a defensive heartbeat-supplement that closes a categorical gap T8 by design does not address (see §2 bullet 3 for the three gaps).**

T8's contract obligations (excerpt — see audit §2 for full list):

1. On `EventAgentSpawn`: emit ob-mcp entry tagged `agent:icarus + event_type:agent.spawn + session:<sid> + runtime:icarus + host:<host>`. cmdr's SSE relay performs the INSERT into `sessions` per §4 contract; emits `events` row with `event_type='spawn'`; sends mail.
2. On `EventAgentStop`: emit ob-mcp entry tagged `event_type:agent.stop`. cmdr's SSE relay UPDATEs `sessions` SET `state='completed'`; INSERTs `metrics`; emits `events` row with `event_type='session_end'`; sends mail.
3. **Heartbeat: NOT IMPLEMENTED on icarus side.** Audit confirmed `rg heartbeat` returns zero matches across the icarus tree. Lifecycle transitions are the only liveness signal. **This is the gap T2.5 closes.**
4. State file: `${CMDR_STATE_DIR}/active-icarus-${session_id}.txt` (written by cmdr's SSE relay consumer, not by icarus directly).
5. Sidecar: `${CMDR_STATE_DIR}/agent-id-icarus-${agent_name}.map` (same — written by cmdr-side relay consumer).

### Polling fallback (T2.5) — defensive heartbeat-supplement

`internal/agents/poller/icarus_poller.go` is a feature-gated (`ICARUS_POLLER=1`, **default OFF**) goroutine that watches `${ICARUS_HOME:-$HOME/.icarus}/sessions/*.jsonl` mtimes. It DOES NOT replace T8's INSERT path; it ONLY synthesizes heartbeat events on the cmdr side when the JSONL has activity but no recent ob-mcp event arrived.

Behavior:

1. Every `${ICARUS_POLLER_INTERVAL:-30}` seconds, scan `${ICARUS_HOME}/sessions/*.jsonl` for files modified within the last `2 * interval` seconds.
2. For each such file, parse the trailing line for `session_id` (icarus ULID, present in every JSONL line).
3. Look up `sessions WHERE runtime='icarus' AND id LIKE 'icarus-${session_id}-%' AND state IN ('booting','working')`.
4. **Skip rows where `heartbeat_at` is within the last 60s** (means T8 owns the row via ob-mcp SSE; do not double-write).
5. Otherwise, `UPDATE sessions SET heartbeat_at='${now_iso}'` for the matching rows.
6. Signal panes (`SIGUSR1` to `status-pane.pid` and `dashboard.pid`).

**Default-off rationale:** the poller is deployed cold. Operators flip `ICARUS_POLLER=1` only when the reaper produces stale-icarus-session false positives in production logs. When T8 grows a real heartbeat in a future spec, the poller is removed via env-var unset (no code change required).

**Configuration knobs:**

| Env var | Default | Purpose |
|---------|---------|---------|
| `ICARUS_POLLER` | `0` | Master kill-switch; `1` enables the poller goroutine |
| `ICARUS_POLLER_INTERVAL` | `30` | Poll interval in seconds |
| `ICARUS_HOME` | `$HOME/.icarus` | Root directory containing `sessions/*.jsonl` |
| `CMDR_HEARTBEAT_INTERVAL` | `30` | Used by the poller to compute the "T8 owns the row" cutoff (`2 * interval`) |

### Hook integration matrix

| Hook event | Claude | Gemini | Icarus (T8 + poller) |
|------------|--------|--------|----------------------|
| Spawn signal | SubagentStart | SessionStart | `EventAgentSpawn` (TypedBus → ob-mcp SSE) |
| Activity signal | PostToolUse(Task) | PostToolUse | `EventPostToolCall` / `EventProviderRequest` |
| Completion signal | SubagentStop | SessionEnd | `EventAgentStop` |
| Sweep trigger | SessionStart | SessionStart | icarus boot hook (T8 unsubscribe) |
| Cleanup trigger | SessionEnd | SessionEnd | icarus shutdown hook |
| Heartbeat strategy | None (tool-driven) | 30s setsid bash loop (`CMDR_HEARTBEAT_INTERVAL`) | T8: NONE. Cmdr-side poller: 30s JSONL mtime watch (`ICARUS_POLLER_INTERVAL`), default OFF |

### Hooks Integration

```json
{
  "hooks": {
    "SessionStart": [
      {
        "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh session-start",
        "description": "Register gemini agent in cmdr sessions table"
      }
    ],
    "PostToolUse": [
      {
        "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh post-tool",
        "description": "Bump last_activity and heartbeat_at on tool completion"
      }
    ],
    "SessionEnd": [
      {
        "command": "$HOME/.gemini/hooks/cmdr-bridge-gemini.sh stop",
        "description": "Mark session completed; write metrics row"
      }
    ]
  }
}
```

---

## 11. What It Does NOT Do

Explicitly out of scope (keep it minimal):

- **Does NOT replatform `cmdr-bridge.sh` into Go.** The Go-native bridge (`cmd/hook-bridge/main.go`) is a separate effort — it currently serves intent-verify and `cmdr-bridge` (different namespace, the Go-bridge `cmdr` handler in `bridge/cmdr/`). Migrating the shell bridge to that pipeline is out of scope; this spec keeps the shell bridge as the source of truth and the Go bridge as a separate consumer.
- **Does NOT change cmdr's spawn pipeline.** `internal/agents/spawner.go` continues to use `runtimes.GetRuntime(id).BuildSpawnCommand()`. This spec only changes the *tracker* (post-spawn sessions table writers) and the *reaper* (post-spawn liveness reader), not the spawner.
- **Does NOT alter Claude's existing palette behavior beyond partition reservation.** Claude's per-agent-type color overrides (`map_agent_color` helper in cmdr-bridge.sh) are preserved. The new partition allocator (`AssignColorForRuntime`) only kicks in for unknown agent types and for non-Claude runtimes.
- **Does NOT add icarus T8 to this spec's task manifest.** T8 ships from the icarus repo and is **already merged at icarus@`4bf3fb8` / `f7107fb`** (audit ref: `/tmp/icarus-cmdr-integration-audit.md`). This spec defines the contract T8 satisfies (§4, §10) and ships a defensive heartbeat-supplement poller (T2.5) that closes the categorical gap T8 by design does not address.
- **Does NOT add `gemini.yaml` agent types beyond `default`.** Gemini's agent-type taxonomy is user-defined; this spec ships a single `default` mapping. Adding richer Gemini agent types is a follow-up spec.
- **Does NOT migrate the legacy `specs/multi-agent-tracking.md`.** Per CLAUDE.md SPEC LAYOUT RULE (which supersedes the older `# Project Rules > Specs` lowercase guidance), the legacy `specs/` directory is FROZEN. This spec supersedes that document for ongoing work.
- **Does NOT add a `runtime` column to the Agents pane.** Per Q7 decision: badge, not column.
- **Does NOT change the SQLite WAL mode or DB lock semantics.**
- **Does NOT add retry / DLQ to icarus T8's `WriteEntryAsync` path.** That fix lives in the icarus repo. This spec only addresses the heartbeat gap on the cmdr side.
- **Does NOT add a kill-switch on the icarus side.** Disabling icarus emitters when ob-mcp is unreachable is a follow-up that requires icarus-side env-gating.

---

## 12. Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime (Go side) | Go 1.22+ | Existing cmdr toolchain; per CLAUDE.md "Default language is Go" |
| Bridge scripts | bash 5+ | Existing cmdr-bridge.sh is bash; symmetry with Claude bridge for review burden |
| Runtime registration | `runtimes.RegisterRuntime` registry | Pattern already shipped in T7 (commit `03422b8`); zero new infra |
| Storage | SQLite (WAL, `INSERT OR REPLACE`, advisory flock) | Existing; no replacement |
| IPC (state files) | `${CMDR_STATE_DIR}/active-{runtime}-{session_id}.txt` | Existing pattern, runtime-prefixed |
| Pane refresh | SIGUSR1 to status-pane.pid + dashboard.pid | Existing pattern (`signal_panes` helper) |
| Heartbeat (gemini) | `setsid bash` background loop, `CMDR_HEARTBEAT_INTERVAL` (default 30s) | Matches existing shell idioms; Gemini's hook surface lacks an in-process daemon channel |
| Heartbeat (icarus) | T8: NONE. Cmdr poller fallback: Go goroutine watching JSONL mtimes | T8 by design does not heartbeat (audit ref); poller fills the gap |
| Capability mapping | Go table-driven (`internal/agents/capability.go`) | Replaces three bash case-statements with one source of truth |
| Tests (Go) | `go test ./...` with table tests | Existing pattern |
| Tests (bash) | `bats` (already in cmdr testing toolchain for cmdr-bridge.sh per agentic_instructions) | Existing |
| Distribution | Same as cmdr (`go build ./cmd/cc`) | Unchanged |

---

## 13. Project Infrastructure

### Directory Structure (changes only — full tree in §4)

```
SPEC/MULTI_RUNTIME_TRACKER/
  MULTI_RUNTIME_TRACKER.md                        # this file
  REVIEWS/
    MULTI_RUNTIME_TRACKER_REVIEW.md               # spec-of-spec review (downstream)
    MULTI_RUNTIME_TRACKER_REBUILD_NOTES.md        # iterative rebuild changelog (this iteration)
    phase1_T1.1_review.md                         # T1.1 file review
    phase1_T1.2a_review.md                        # T1.2a file review (NEW reviewer-independence checkpoint)
    phase1_T1.4_review.md                         # T1.4 file review (NEW reviewer-independence checkpoint)
    phase1_T1.5_review.md                         # T1.5 file review
    phase2_T2.4_review.md                         # T2.4 file review (reaper extraction + runtime-aware)
    phase2_T2.5_review.md                         # T2.5 file review
    phase3_review.md                              # phase 3 file review
    phase4_review.md                              # phase 4 file review

agents/
  gemini.yaml                                     # NEW (T1.2)

internal/agents/
  capability.go                                   # NEW (T1.1)
  capability_test.go                              # NEW (T1.1)
  palette.go                                      # MODIFIED (T1.3)
  palette_test.go                                 # MODIFIED (T1.3)

internal/agents/reaper/                           # NEW PACKAGE
  reaper.go                                       # NEW (T2.4a, then runtime-aware in T2.4)
  reaper_test.go                                  # NEW (T2.4a + T2.4)

internal/agents/poller/                           # NEW PACKAGE
  icarus_poller.go                                # NEW (T2.5)
  icarus_poller_test.go                           # NEW (T2.5)

internal/commands/
  status.go                                       # MODIFIED (T2.4a extraction, T4.1 badge)
  capability.go                                   # NEW (T1.1, CLI subcommand wiring)
  agent.go                                        # NEW (T1.2a, `cmdr agent show` cobra)
  agent_test.go                                   # NEW (T1.2a)
  status_test.go                                  # MODIFIED (T4.1)

internal/platform/db/migrations/sqlite/
  012_runtime_check.sql                           # NEW (T1.4)
  012_runtime_check_down.sql                      # NEW (T1.4 rollback companion)

pkg/runtimes/gemini/
  gemini.go                                       # MODIFIED (T3.1, flesh out stubs)
  gemini_test.go                                  # NEW (T3.1)
```

### Version Management

Existing — `cmd/cc/version.go` constant + git short SHA. No bump policy change.

### CHANGELOG.md

Existing — Keep a Changelog format. Each phase contributes an entry under `### Added` or `### Changed`.

### CI Workflow

Existing (`go test ./...`). No new workflow file. Phase 1 adds `agents/gemini.yaml` schema validation to the existing agent-yaml lint step.

### Scripts (Makefile or task runner — existing only)

| Script | Behavior |
|--------|----------|
| `make test` | `go test ./...` (existing) |
| `make build` | `go build ./cmd/cc` (existing) |
| `make install` | Installs `cmdr` to `~/.local/bin/cmdr` AND `~/go/bin/cmdr` per `feedback_claude_hooks_install.md` (existing) |
| `bats test/cmdr-bridge.bats` | Bash bridge regression tests (existing — extended in T2.1, T2.2, T2.3) |

---

## 14. Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| `internal/agents/capability.go` + test | 2 | ~180 |
| `internal/agents/palette.go` + test (modified) | 2 | +60 |
| `internal/commands/capability.go` (CLI) | 1 | ~80 |
| `internal/commands/agent.go` (CLI) + test | 2 | ~140 |
| `internal/commands/status.go` (modified, badge + reaper extraction) | 1 | +30 / -50 |
| `internal/agents/reaper/reaper.go` (extracted, runtime-aware) + test | 2 | ~220 |
| `internal/agents/poller/icarus_poller.go` + test | 2 | ~220 |
| `internal/platform/db/migrations/sqlite/012_runtime_check.sql` + down | 2 | ~30 |
| `agents/gemini.yaml` | 1 | ~45 |
| `pkg/runtimes/gemini/gemini.go` + test (modified) | 2 | +120 |
| `~/.claude/hooks/cmdr-bridge.sh` (modified) | 1 | +30 / -20 |
| `~/.gemini/hooks/cmdr-bridge-gemini.sh` (new) | 1 | ~280 |
| `test/cmdr-bridge.bats` (extended) | 1 | +120 |
| `test/multi_runtime/smoke.bats` | 1 | ~120 |
| **Total** | **21** | **~1675** |

---

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|-------------------|--------------------|------------|----------------|
| T1.1 | cmdr_coder | Implement `MapCapability` Go table + `cmdr capability map` CLI subcommand (with `warning` field for unknown agent_types) | `internal/agents/types.go`, `pkg/runtimes/runtime.go`, `internal/commands/status.go` (read only for cobra patterns) | `internal/agents/capability.go`, `internal/agents/capability_test.go`, `internal/commands/capability.go`, `cmd/cc/main.go` | — | `go test ./internal/agents/... ./internal/commands/...` exits 0 AND `cmdr capability map --runtime icarus --agent-type reasoning` outputs `builder` AND `cmdr capability map --runtime icarus --agent-type unknown-type --json \| jq -e '.warning'` exits 0 |
| T1.1R | cmdr_coder (FRESH) | Review T1.1 diff for clean-room correctness; verify unknown-type fallback emits `warning` in JSON | T1.1 outputs only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.1_review.md` | T1.1 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.1_review.md` |
| T1.2a | cmdr_coder | Implement `cmdr agent show <name>` cobra subcommand: read `agents/<name>.yaml`, yaml-unmarshal, print human or JSON; exit 0/1/2 | `internal/commands/status.go` (cobra patterns), `agents/icarus.yaml` (schema reference) | `internal/commands/agent.go`, `internal/commands/agent_test.go`, `cmd/cc/main.go` | T1.1 (cobra root reuse) | `go test ./internal/commands/... -run TestAgentShow` exits 0 AND `cmdr agent show icarus --json \| jq -r .runtime` outputs `icarus` |
| T1.2aR | cmdr_coder (FRESH) | Review T1.2a for cobra wiring and exit-code correctness | T1.2a outputs only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.2a_review.md` | T1.2a | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.2a_review.md` |
| T1.2 | cmdr_coder | Add `agents/gemini.yaml` per §4 schema | `agents/icarus.yaml`, `agents/builder.yaml` | `agents/gemini.yaml` | T1.2a | `cmdr agent show gemini --json \| jq -e '.success == true and .runtime == "gemini"'` exits 0 AND `yamllint agents/gemini.yaml` exits 0 |
| T1.3 | cmdr_coder | Partition palette by runtime; add `AssignColorForRuntime` and `runtimePartitionBase` | `internal/agents/palette.go`, `internal/agents/types.go` | `internal/agents/palette.go`, `internal/agents/palette_test.go` | — | `go test ./internal/agents/... -run TestPalette` exits 0 AND test asserts `runtimePartitionBase(RuntimeClaude)==0`, `RuntimeIcarus==4`, `RuntimeGemini==8` |
| T1.4 | cmdr_coder | Add migration `012_runtime_check.sql` (and `012_runtime_check_down.sql`) with CHECK constraint on `sessions.runtime`; full-column-list table-rebuild | ALL `internal/platform/db/migrations/sqlite/0*.sql` (must read in order to enumerate live schema) | `internal/platform/db/migrations/sqlite/012_runtime_check.sql`, `internal/platform/db/migrations/sqlite/012_runtime_check_down.sql` | — | Apply ALL migrations 001..011 then 012 to `/tmp/probe.db`; `sqlite3 /tmp/probe.db "PRAGMA table_info(sessions);" \| wc -l` matches pre-012 count; `sqlite3 /tmp/probe.db "INSERT INTO sessions (id, agent_name, capability, state, runtime, started_at, last_activity) VALUES ('x','x','builder','working','invalid_runtime',datetime('now'),datetime('now'));" 2>&1 \| grep -q 'CHECK constraint failed'` |
| T1.4R | cmdr_coder (FRESH) | Review T1.4 for column-fidelity vs live schema; verify column count and types unchanged | T1.4 output only, plus probe of fresh DB | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.4_review.md` | T1.4 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.4_review.md` |
| T1.5 | cmdr_coder | Modify Claude `cmdr-bridge.sh` for runtime-prefixed IDs/state files; replace inline `map_capability` with shell-out. Changes are function-keyed per §9; preserve existing `db_id="${db_id:0:64}"` truncation | `~/.claude/hooks/cmdr-bridge.sh` | `~/.claude/hooks/cmdr-bridge.sh` | T1.1, T1.4 | `bats test/cmdr-bridge.bats` exits 0 AND `grep -qE '^CMDR_RUNTIME=' ~/.claude/hooks/cmdr-bridge.sh` AND `grep -qE 'active-\$\{CMDR_RUNTIME\}-' ~/.claude/hooks/cmdr-bridge.sh` AND `grep -qE 'agent-id-\$\{CMDR_RUNTIME\}-' ~/.claude/hooks/cmdr-bridge.sh` AND `grep -qE "AND id LIKE .\\\$\\{CMDR_RUNTIME\\}-" ~/.claude/hooks/cmdr-bridge.sh` |
| T1.5R | cmdr_coder (FRESH) | Review T1.5 diff for collision-proof state-file naming + WHERE-clause runtime scoping; grep for residual literal `'claude'` outside the `:-claude` default and any documented backwards-compat fallback | T1.5 output only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.5_review.md` | T1.5 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.5_review.md` |
| T2.1 | cmdr_coder | Cross-runtime cleanup isolation 2x2 regression test (bats): (claude-killed → icarus-survives) AND (icarus-killed → claude-survives) | `~/.claude/hooks/cmdr-bridge.sh`, `test/cmdr-bridge.bats` | `test/cmdr-bridge.bats` | T1.5 | `bats test/cmdr-bridge.bats -f 'cross.runtime.cleanup'` exits 0 |
| T2.2 | cmdr_coder | Verify `do_session_start` and `do_cleanup` are runtime-scoped (no claude session can affect icarus/gemini rows) | `~/.claude/hooks/cmdr-bridge.sh` | `~/.claude/hooks/cmdr-bridge.sh`, `test/cmdr-bridge.bats` | T1.5 | `bats test/cmdr-bridge.bats -f 'session.start.scoping\|cleanup.scoping'` exits 0 |
| T2.3 | cmdr_coder | Add `runtime` filter to all WHERE clauses in `do_stop` resolver (preserve existing fallbacks for backwards compat) | `~/.claude/hooks/cmdr-bridge.sh` | `~/.claude/hooks/cmdr-bridge.sh` | T1.5 | `bats test/cmdr-bridge.bats -f 'do_stop.runtime'` exits 0 |
| T2.4a | cmdr_coder | EXTRACT reaper from `runStatusPane` closure into `internal/agents/reaper/reaper.go` package with NO behavioral change; rewire `runStatusPane` to call new package; add unit tests for the existing `last_activity`-only logic | `internal/commands/status.go:185-232` | `internal/agents/reaper/reaper.go`, `internal/agents/reaper/reaper_test.go`, `internal/commands/status.go` | — | `go test ./internal/agents/reaper/...` exits 0 AND `go test ./internal/commands/...` exits 0 AND `git diff --stat internal/commands/status.go \| grep -q '\-'` (verifies status.go shrank) |
| T2.4 | cmdr_coder | Add runtime-aware logic to extracted reaper: TWO disjoint queries per §3 rule 6 (NOT COALESCE); preserve existing claude behavior | `internal/agents/reaper/reaper.go` (post-T2.4a) | `internal/agents/reaper/reaper.go`, `internal/agents/reaper/reaper_test.go` | T2.4a, T1.4 | `go test ./internal/agents/reaper/... -run 'TestReaper.*Runtime'` exits 0; integration test: seed claude row with last_activity=15min-ago AND heartbeat_at=NULL → reaped; seed icarus row with heartbeat_at=15min-ago → reaped; seed icarus row with heartbeat_at=NULL → NOT reaped (too early) |
| T2.4R | cmdr_coder (FRESH) | Review T2.4a + T2.4 combined diff: extraction is behavior-preserving; runtime-aware logic uses two disjoint queries (no COALESCE) | T2.4a + T2.4 outputs only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.4_review.md` | T2.4 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.4_review.md` |
| T2.5 | cmdr_coder | Implement icarus polling fallback as defensive heartbeat-supplement (`ICARUS_POLLER=1` env-gated, default OFF). Skips rows with recent `heartbeat_at` (T8 owns); writes only synthesized heartbeats. NOT a substitute for T8 INSERT path | `pkg/runtimes/icarus/icarus.go`, `~/Programs/ai/icarus/internal/integration/cmdr/events.go` (read-only reference for tag scheme) | `internal/agents/poller/icarus_poller.go`, `internal/agents/poller/icarus_poller_test.go` | T1.4 | `go test ./internal/agents/poller/...` exits 0 AND end-to-end test: seed icarus row with `heartbeat_at` 5 minutes old, write fresh `~/.icarus-test/sessions/sess-X.jsonl` mtime, run poller with `ICARUS_POLLER=1`, verify `heartbeat_at` updated; AND: seed icarus row with `heartbeat_at` 30s old, run poller, verify `heartbeat_at` UNCHANGED (T8 owns) |
| T2.5R | cmdr_coder (FRESH) | Review T2.5 polling logic for race conditions, T8-collision-avoidance (60s skip window), SIGUSR1 signaling, and that the poller does NOT INSERT rows (heartbeat-only) | T2.5 output only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.5_review.md` | T2.5 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.5_review.md` |
| T3.1 | cmdr_coder | Flesh out gemini runtime adapter (`DeployConfig`, `ParseTranscript`, `BuildEnv`) | `pkg/runtimes/gemini/gemini.go`, `pkg/runtimes/icarus/icarus.go` (reference impl) | `pkg/runtimes/gemini/gemini.go`, `pkg/runtimes/gemini/gemini_test.go` | T1.2 | `go test ./pkg/runtimes/gemini/...` exits 0 |
| T3.2 | cmdr_coder | Author `~/.gemini/hooks/cmdr-bridge-gemini.sh` (fork from claude bridge with hard-coded `CMDR_RUNTIME=gemini`, `CMDR_HEARTBEAT_INTERVAL`-aware heartbeat loop with 24h escape hatch) | `~/.claude/hooks/cmdr-bridge.sh` (modified by T1.5) | `~/.gemini/hooks/cmdr-bridge-gemini.sh` | T1.5, T2.4 | `bash -n ~/.gemini/hooks/cmdr-bridge-gemini.sh` exits 0 AND end-to-end: simulate gemini SessionStart, verify `sqlite3 ~/.computecommander/local.db "SELECT runtime FROM sessions WHERE id LIKE 'gemini-%' LIMIT 1;"` returns `gemini` |
| T3.3 | cmdr_coder | Patch `~/.gemini/settings.json` with hooks block (SessionStart + PostToolUse + SessionEnd). Use `jq` for the patch (read+merge+write), not `sed`; backup before patching | `~/.gemini/settings.json` | `~/.gemini/settings.json` | T3.2 | `jq -e '.hooks.SessionStart and .hooks.PostToolUse and .hooks.SessionEnd' ~/.gemini/settings.json` exits 0 |
| T3.3R | cmdr_coder (FRESH) | Review T3.2+T3.3 for parity with claude bridge contract; verify all three hook events present | T3.2, T3.3 outputs only | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase3_review.md` | T3.2, T3.3 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase3_review.md` |
| T4.1 | cmdr_coder | Add runtime badge prefix to `printAgentsPane` with `NO_COLOR`-aware fallback (`[C]`/`[I]`/`[G]`) | `internal/commands/status.go`, `internal/agents/palette.go` | `internal/commands/status.go`, `internal/commands/status_test.go` | T1.3 | `go test ./internal/commands/... -run TestPaneBadge` exits 0; `NO_COLOR=1 cmdr status --pane 2>&1 \| timeout 3s grep -qE '^\s*\[[CIG]\]\s'` returns 0 OR exit 124 (timeout); `cmdr status --pane 2>&1 \| timeout 3s grep -qE '^\s*[CIG]\s'` returns 0 OR exit 124 |
| T4.2 | cmdr_coder | Multi-runtime smoke test: spawn 1 claude + 1 icarus (poller seed) + 1 gemini, verify all 3 visible in `cmdr status --pane`, no palette collision, no cross-runtime cleanup; run for ≥30s to flush WAL contention | full test fixture under `test/multi_runtime/` | `test/multi_runtime/smoke.bats`, `test/multi_runtime/seed.go` | T4.1, T2.1, T3.2 | `bats test/multi_runtime/smoke.bats` exits 0 |
| T4.2R | cmdr_coder (FRESH) | Final integration review across all phases; verify reviewer-independence checkpoints all present (count >= 7) | All phase 1-4 outputs | `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase4_review.md` | T4.2 | `test -f SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase4_review.md` AND `find SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/ -name '*review*.md' \| wc -l \| awk '{print ($1 >= 7)}' \| grep -q 1` |

---

## 16. Dependency Graph

```
Phase 1a (parallel preconditions): [T1.1, T1.3, T1.4]
  T1.1: capability map (Go + CLI)
  T1.3: palette partition
  T1.4: migration 012 (CHECK constraint)
  (T1.1R, T1.4R fire as soon as their respective parents complete)

Phase 1b (after T1.1): [T1.2a, T1.5]
  T1.2a: cmdr agent show (depends on T1.1 cobra root)
  T1.5: cmdr-bridge.sh runtime-prefixed (depends on T1.1 + T1.4)
  (T1.2aR, T1.5R fire on completion of their parents)

Phase 1c (after T1.2a): [T1.2]
  T1.2: agents/gemini.yaml (depends on T1.2a so verify can run)

Note: T1.2 and T1.3 do NOT bottleneck T1.5. T1.5 dispatches the moment T1.1 + T1.4 are green; T1.2 and T1.3 may still be in flight.

Phase 2 (parallel, after T1.5): [T2.1, T2.2, T2.3, T2.4a, T2.5]
  T2.1: cross-runtime cleanup 2x2 regression test
  T2.2: do_session_start / do_cleanup runtime scoping
  T2.3: do_stop WHERE-clause runtime filter
  T2.4a: reaper extraction (no behavioral change)
  T2.5: icarus polling fallback (defensive heartbeat-supplement)
  (T2.5R fires on T2.5 completion)

Phase 2 finalization (after T2.4a + T1.4): [T2.4]
  T2.4: runtime-aware reaper (two disjoint queries)
  (T2.4R fires on T2.4 completion)

Phase 3 (parallel, after Phase 2): [T3.1, T3.2, T3.3]
  T3.1: gemini runtime adapter flesh-out (depends on T1.2)
  T3.2: gemini bridge fork (depends on T1.5 + T2.4)
  T3.3: ~/.gemini/settings.json patch (depends on T3.2)
  (T3.3R fires on T3.2 + T3.3 completion)

Phase 4 (after Phase 3): [T4.1, T4.2]
  T4.1: pane runtime badge with NO_COLOR fallback (depends on T1.3)
  T4.2: multi-runtime smoke test (depends on T4.1 + T2.1 + T3.2)
  (T4.2R fires on T4.2 completion)

Final: T4.2R — integration review gates merge
```

Acyclic. T1.5 is the bridge bottleneck (depends on T1.1 + T1.4). T2.4 depends on T2.4a (extraction must precede behavior change). Phase 1 internal parallelism: T1.1, T1.3, T1.4 are all independent.

---

## 17. Target State

Files created:

| File Path | Lines | Executable |
|-----------|-------|------------|
| `internal/agents/capability.go` | ~120 | No |
| `internal/agents/capability_test.go` | ~80 | No |
| `internal/commands/capability.go` | ~80 | No |
| `internal/commands/agent.go` | ~80 | No |
| `internal/commands/agent_test.go` | ~60 | No |
| `internal/agents/reaper/reaper.go` | ~140 | No |
| `internal/agents/reaper/reaper_test.go` | ~80 | No |
| `internal/agents/poller/icarus_poller.go` | ~150 | No |
| `internal/agents/poller/icarus_poller_test.go` | ~80 | No |
| `internal/platform/db/migrations/sqlite/012_runtime_check.sql` | ~60 | No |
| `internal/platform/db/migrations/sqlite/012_runtime_check_down.sql` | ~30 | No |
| `agents/gemini.yaml` | ~45 | No |
| `pkg/runtimes/gemini/gemini_test.go` | ~120 | No |
| `~/.gemini/hooks/cmdr-bridge-gemini.sh` | ~280 | Yes (chmod +x) |
| `test/multi_runtime/smoke.bats` | ~120 | No |
| `test/multi_runtime/seed.go` | ~80 | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.1_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.2a_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.4_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase1_T1.5_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.4_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase2_T2.5_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase3_review.md` | review | No |
| `SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/phase4_review.md` | review | No |

Files modified:

- `internal/agents/palette.go` (add `AssignColorForRuntime`, `runtimePartitionBase`)
- `internal/agents/palette_test.go`
- `internal/commands/status.go` (T2.4a: extract `reapStale`, rewire `runStatusPane` to call package; T4.1: badge prefix)
- `internal/commands/status_test.go`
- `cmd/cc/main.go` (register `capability` and `agent` cobra subcommands)
- `pkg/runtimes/gemini/gemini.go` (flesh out stubs)
- `~/.claude/hooks/cmdr-bridge.sh` (runtime-prefixed)
- `~/.gemini/settings.json` (hooks block)
- `test/cmdr-bridge.bats` (extended with cross-runtime scenarios)

Files deleted: None.

---

## 18. Verification Plan

**Per-task checks:** (from Task Manifest Verify Command column)

- T1.1: `go test ./internal/agents/... ./internal/commands/...` exits 0; `cmdr capability map --runtime icarus --agent-type reasoning` outputs `builder`; unknown agent_type emits `warning` field
- T1.2a: `go test ./internal/commands/... -run TestAgentShow` exits 0; `cmdr agent show icarus --json | jq -r .runtime` outputs `icarus`
- T1.2: `cmdr agent show gemini --json | jq -e '.success == true and .runtime == "gemini"'` exits 0; `yamllint agents/gemini.yaml` exits 0
- T1.3: `go test ./internal/agents/... -run TestPalette` exits 0; partition bases asserted
- T1.4: full migration chain 001..012 applies; `PRAGMA table_info(sessions)` column count unchanged; CHECK rejects `'invalid_runtime'`
- T1.5: `bats test/cmdr-bridge.bats` exits 0; grep checks for `CMDR_RUNTIME`, runtime-prefixed state files, runtime-scoped WHERE clauses
- T2.1: `bats test/cmdr-bridge.bats -f 'cross.runtime.cleanup'` exits 0 (2x2 matrix)
- T2.2: `bats test/cmdr-bridge.bats -f 'session.start.scoping|cleanup.scoping'` exits 0
- T2.3: `bats test/cmdr-bridge.bats -f 'do_stop.runtime'` exits 0
- T2.4a: `go test ./internal/agents/reaper/...` exits 0; `go test ./internal/commands/...` exits 0; status.go LOC shrunk
- T2.4: runtime-aware reaper integration test — claude reaped via last_activity, icarus reaped via heartbeat_at, NULL heartbeat_at on icarus row is NOT reaped
- T2.5: poller integration test — stale icarus heartbeat (5min) gets refreshed; recent heartbeat (<60s, T8 owns) is NOT touched
- T3.1: `go test ./pkg/runtimes/gemini/...` exits 0
- T3.2: `bash -n ~/.gemini/hooks/cmdr-bridge-gemini.sh` exits 0; end-to-end gemini SessionStart simulation
- T3.3: `jq -e '.hooks.SessionStart and .hooks.PostToolUse and .hooks.SessionEnd' ~/.gemini/settings.json` exits 0
- T4.1: `go test ./internal/commands/... -run TestPaneBadge` exits 0; pane shows colored badges and `[X]` fallback under NO_COLOR
- T4.2: `bats test/multi_runtime/smoke.bats` exits 0

**Integration check:**
```bash
# Multi-runtime smoke test (T4.2 expanded)
go test ./... && \
bats test/cmdr-bridge.bats test/multi_runtime/smoke.bats && \
timeout 5s cmdr status --pane | grep -E '^\s*[CIG]\s' | wc -l | grep -q '^[3-9]$'
# Expected: 3+ rows with C/I/G badges visible after 5s of running 1 claude + 1 icarus + 1 gemini
```

**Rollback:**
```bash
# Phase 1: git revert <T1.5 commit> && sqlite3 .computecommander/local.db < internal/platform/db/migrations/sqlite/012_runtime_check_down.sql
# Phase 2-4: git revert per-task commits in reverse dependency order
# State files: rm -rf /tmp/cmdr-state/active-{icarus,gemini}-* /tmp/cmdr-state/agent-id-{icarus,gemini}-*
# Bridge: cp ~/.claude/hooks/cmdr-bridge.sh.bak ~/.claude/hooks/cmdr-bridge.sh
# Gemini: rm ~/.gemini/hooks/cmdr-bridge-gemini.sh && jq 'del(.hooks)' ~/.gemini/settings.json > /tmp/sj && mv /tmp/sj ~/.gemini/settings.json
```

### Functional Smoke Tests

#### TUI Smoke Tests

**Status pane launches with runtime badges:**

```bash
timeout 3s cmdr status --pane 2>&1 | head -20
test $? -eq 124 -o $? -eq 0
```

**Pane output contains runtime badge characters when sessions exist (color mode):**

```bash
# Pre-seed a fake row per runtime
sqlite3 .computecommander/local.db <<SQL
INSERT INTO sessions (id, agent_name, capability, state, runtime, started_at, last_activity, color_index, color_hex)
VALUES ('claude-test-1','t1','builder','working','claude',datetime('now'),datetime('now'),0,'#FF6B6B'),
       ('icarus-test-2','t2','builder','working','icarus',datetime('now'),datetime('now'),4,'#5DADE2'),
       ('gemini-test-3','t3','builder','working','gemini',datetime('now'),datetime('now'),8,'#FFDAB9');
SQL
timeout 3s cmdr status --pane 2>&1 | grep -qE '^\s*[CIG]\s'
```

**Pane output contains bracket-form badge under NO_COLOR:**

```bash
NO_COLOR=1 timeout 3s cmdr status --pane 2>&1 | grep -qE '^\s*\[[CIG]\]\s'
```

#### Keybind Coverage

**Existing pane keybinds unchanged:** `/` (filter), `Enter` (refresh) — no new keybinds added by this spec, so no coverage check needed beyond existing `status_test.go`.

#### Binary Install Verification

**Built binary matches installed:**

```bash
go build -o cmdr ./cmd/cc
./cmdr --version | grep -q "$(git rev-parse --short HEAD)"

INSTALLED=$(cmdr --version 2>&1)
BUILT=$(./cmdr --version 2>&1)
test "$INSTALLED" = "$BUILT" || { echo "STALE INSTALL: installed=$INSTALLED built=$BUILT"; exit 1; }
```

Per `feedback_claude_hooks_install.md`, install to BOTH `~/.local/bin/cmdr` and `~/go/bin/cmdr`.

#### Layout/Config Validation

**`~/.gemini/settings.json` parses as valid JSON and contains all three hook events:**

```bash
jq -e '.hooks.SessionStart and .hooks.PostToolUse and .hooks.SessionEnd' ~/.gemini/settings.json
```

**`agents/gemini.yaml` parses as valid YAML and conforms to agent schema:**

```bash
yamllint agents/gemini.yaml && cmdr agent show gemini --json | jq -e '.success == true'
```

**Migration 012 applies cleanly to a fresh DB (FULL chain, all migrations in order):**

```bash
rm -f /tmp/test-cmdr.db
for f in internal/platform/db/migrations/sqlite/0*.sql; do
    sqlite3 /tmp/test-cmdr.db < "$f" || { echo "FAIL: $f"; exit 1; }
done
# Pre/post column count must match
PRE_COUNT=$(sqlite3 /tmp/test-cmdr.db "PRAGMA table_info(sessions);" | wc -l)
sqlite3 /tmp/test-cmdr.db < internal/platform/db/migrations/sqlite/012_runtime_check.sql
POST_COUNT=$(sqlite3 /tmp/test-cmdr.db "PRAGMA table_info(sessions);" | wc -l)
test "$PRE_COUNT" = "$POST_COUNT" || { echo "Column count drift: pre=$PRE_COUNT post=$POST_COUNT"; exit 1; }
# Reject unknown runtime
sqlite3 /tmp/test-cmdr.db "INSERT INTO sessions (id, agent_name, capability, state, runtime, started_at, last_activity) VALUES ('x','x','builder','working','invalid_runtime',datetime('now'),datetime('now'));" 2>&1 | grep -q 'CHECK constraint failed'
```

#### Integration Smoke (Dashboard)

**Dashboard launches without crash:**

```bash
timeout 5s cmdr dashboard --kdl 2>&1 | tail -5
test $? -eq 124 -o $? -eq 0
```

---

## 19. Success Criteria (Machine-Verifiable)

- [ ] `go test ./...` exits 0
- [ ] `bats test/cmdr-bridge.bats` exits 0
- [ ] `bats test/multi_runtime/smoke.bats` exits 0
- [ ] `cmdr capability map --runtime icarus --agent-type reasoning` outputs `builder`
- [ ] `cmdr capability map --runtime gemini --agent-type default --json | jq -r .capability` outputs `builder`
- [ ] `cmdr capability map --runtime claude --agent-type unix-coder` outputs `builder`
- [ ] `cmdr capability map --runtime claude --agent-type spec-reviewer` outputs `reviewer`
- [ ] `cmdr capability map --runtime icarus --agent-type unknown-type --json | jq -e '.warning'` exits 0 (unknown agent_type emits warning, not error)
- [ ] `cmdr capability map --runtime foobar --agent-type x --json | jq -e '.success == false'` exits 0 (unknown runtime is hard error)
- [ ] `cmdr agent show gemini --json | jq -r .runtime` outputs `gemini`
- [ ] `cmdr agent show icarus --json | jq -r .runtime` outputs `icarus`
- [ ] `cmdr agent show nonexistent; test $? -eq 1` (missing agent file → exit 1)
- [ ] `for f in internal/platform/db/migrations/sqlite/0*.sql; do sqlite3 /tmp/check.db < "$f"; done && sqlite3 /tmp/check.db "INSERT INTO sessions (id, agent_name, capability, state, runtime, started_at, last_activity) VALUES ('x','x','builder','working','invalid',datetime('now'),datetime('now'));" 2>&1 | grep -q 'CHECK constraint failed'`
- [ ] `bash -n ~/.gemini/hooks/cmdr-bridge-gemini.sh` exits 0
- [ ] `jq -e '.hooks.SessionStart and .hooks.PostToolUse and .hooks.SessionEnd' ~/.gemini/settings.json` exits 0
- [ ] `test -x ~/.gemini/hooks/cmdr-bridge-gemini.sh`
- [ ] `grep -qE '^CMDR_RUNTIME=' ~/.claude/hooks/cmdr-bridge.sh`
- [ ] `grep -qE 'active-\$\{CMDR_RUNTIME\}-' ~/.claude/hooks/cmdr-bridge.sh` (state-file naming uses runtime prefix)
- [ ] `grep -qE 'agent-id-\$\{CMDR_RUNTIME\}-' ~/.claude/hooks/cmdr-bridge.sh` (sidecar-file naming uses runtime prefix)
- [ ] `grep -qE "AND id LIKE .\\\$\\{CMDR_RUNTIME\\}-" ~/.claude/hooks/cmdr-bridge.sh` (do_stop WHERE clauses runtime-scoped)
- [ ] `grep -qE 'cmdr capability map' ~/.claude/hooks/cmdr-bridge.sh` (map_capability shells out to Go)
- [ ] After T4.1: `timeout 5s cmdr status --pane 2>&1 | grep -qE '^\s*[CIG]\s'` (colored badges visible)
- [ ] After T4.1: `NO_COLOR=1 timeout 5s cmdr status --pane 2>&1 | grep -qE '^\s*\[[CIG]\]\s'` (bracket fallback under NO_COLOR)
- [ ] After T4.2: `timeout 30s cmdr status --pane 2>&1 | grep -qE '^\s*[CIG]\s' | wc -l | awk '{print ($1 >= 3)}' | grep -q 1` (badges for 3 runtimes after 30s WAL-contention smoke)
- [ ] After T2.1: bats regression test simulates 2 concurrent sessions in 2x2 matrix: (kill claude → icarus survives) AND (kill icarus → claude survives)
- [ ] After T2.4: integration test marks an icarus row as last-heartbeat=15min-ago, runs reaper, verifies row transitions to `completed`; AND verifies a claude row with last_activity=15min-ago but `heartbeat_at IS NULL` IS reaped (claude uses last_activity); AND verifies an icarus row with `heartbeat_at IS NULL` is NOT reaped (no heartbeat yet, too early)
- [ ] After T2.5: writing a fake `~/.icarus-test/sessions/sess-poll-1.jsonl` with stale (`>5min`) heartbeat_at on the corresponding sessions row triggers a `heartbeat_at` UPDATE within one poll cycle; AND a row with recent (`<60s`) heartbeat_at is left untouched (T8 owns it)
- [ ] `find_cmdr_db` continues to resolve the project DB from `~/.gemini/hooks/cmdr-bridge-gemini.sh` (uses same `CMDR_PROJECT` env or git rev-parse logic as claude bridge)
- [ ] No file under `pkg/runtimes/` writes the literal string `'claude'` outside `pkg/runtimes/claude/`: `! grep -rE "['\"]claude['\"]" pkg/runtimes/ --exclude-dir=claude | grep -v '_test.go'`
- [ ] `find SPEC/MULTI_RUNTIME_TRACKER/REVIEWS/ -name '*review*.md' | wc -l | awk '{print ($1 >= 7)}' | grep -q 1` (at least 7 reviewer-independence checkpoints: T1.1R, T1.2aR, T1.4R, T1.5R, T2.4R, T2.5R, T3.3R, T4.2R = 8 expected)

---

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| All Go authorship (T1.1, T1.2a, T1.3, T1.4, T2.4a, T2.4, T2.5, T3.1, T4.1, T4.2) | `cmdr_coder` | Per CLAUDE.md PINNED ORCHESTRATION RULE |
| All shell authorship (T1.5, T3.2) | `cmdr_coder` | Project rule extends cmdr_coder scope to all in-tree code, including bash bridges |
| Gemini settings.json patch (T3.3) | `cmdr_coder` | JSON config is in-scope per cmdr_coder's owned-files spec |
| YAML authorship (T1.2) | `cmdr_coder` | Same routing |
| Bats test authorship (T2.1, T2.2, T2.3, T4.2) | `cmdr_coder` | Same routing |
| All reviews (T1.1R, T1.2aR, T1.4R, T1.5R, T2.4R, T2.5R, T3.3R, T4.2R) | `cmdr_coder` (FRESH instance per task) | Per CLAUDE.md REVIEWER INDEPENDENCE RULE — distinct Task invocation, no shared context with author |

---

## Execution Order

```
Phase 1 (Foundation)
  +-- T1.1 capability.go + CLI    (cmdr_coder)
  +-- T1.3 palette partition      (cmdr_coder)         [parallel]
  +-- T1.4 migration 012          (cmdr_coder)         [parallel]
  +-- T1.1R review                (FRESH cmdr_coder)
  +-- T1.4R review                (FRESH cmdr_coder)
  +-- T1.2a cmdr agent show       (cmdr_coder)         [needs T1.1]
  +-- T1.2aR review               (FRESH cmdr_coder)
  +-- T1.2 agents/gemini.yaml     (cmdr_coder)         [needs T1.2a]
  +-- T1.5 cmdr-bridge.sh runtime (cmdr_coder)         [needs T1.1+T1.4]
  +-- T1.5R review                (FRESH cmdr_coder)

Phase 2 (Bridge hardening + reaper) [blocked by T1.5]
  +-- T2.1 cross-runtime cleanup 2x2 test (cmdr_coder)
  +-- T2.2 session-start scoping       (cmdr_coder)    [parallel]
  +-- T2.3 do_stop WHERE filter        (cmdr_coder)    [parallel]
  +-- T2.4a reaper extraction          (cmdr_coder)    [parallel]
  +-- T2.5 icarus polling fallback     (cmdr_coder)    [parallel, needs T1.4]
  +-- T2.5R review                     (FRESH cmdr_coder)
  +-- T2.4 runtime-aware reaper        (cmdr_coder)    [needs T2.4a + T1.4]
  +-- T2.4R review                     (FRESH cmdr_coder)

Phase 3 (Gemini emitter) [blocked by Phase 2]
  +-- T3.1 gemini adapter flesh-out    (cmdr_coder)
  +-- T3.2 gemini bridge fork          (cmdr_coder)    [needs T2.4]
  +-- T3.3 ~/.gemini/settings.json     (cmdr_coder)    [needs T3.2]
  +-- T3.3R review                     (FRESH cmdr_coder)

Phase 4 (UI polish + integration) [blocked by Phase 3]
  +-- T4.1 pane runtime badge + NO_COLOR (cmdr_coder)
  +-- T4.2 multi-runtime smoke test    (cmdr_coder)
  +-- T4.2R review                     (FRESH cmdr_coder)
```

Recommended directive: `/sr` — spec → spec-review → execute. Per CLAUDE.md, the spec MUST clear `spec-reviewer` before T1.1 dispatch. After review pass, `/colony` is the right execution shape (4 phases, parallel intra-phase, sequential inter-phase, 8 reviewer-independence checkpoints).

---

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Migration 012 CHECK rejects existing row | `pragma integrity_check` after migration | DB has no rows with non-enum runtimes by construction (default is `claude`); but if it does, pre-migration `UPDATE sessions SET runtime='claude' WHERE runtime NOT IN (...)` runs first |
| Migration 012 column-list drift (live schema has columns the table-rebuild doesn't enumerate) | T1.4R review catches column count mismatch via `PRAGMA table_info` pre/post comparison | T1.4 author re-derives column list from `for f in 0*.sql; do sqlite3 ... < $f; done` and updates `CREATE TABLE sessions_new` |
| State file collision between two runtimes | Bats test T2.1 fails | Verify naming: `active-{runtime}-{session_id}.txt`; never `active-{session_id}.txt` |
| Cross-runtime cleanup bomb (re-occurrence of 2026-03-07) | T2.1 / T2.2 regression test fails; observed in production as agent rows from one runtime disappearing when another runtime's session ends | Roll back T1.5 + T2.2; restore from `~/.claude/hooks/cmdr-bridge.sh.bak` |
| Palette collision (claude + icarus same color) | T1.3 unit test fails; visual inspection of `cmdr status --pane` | Verify `runtimePartitionBase()` returns disjoint bases; check for off-by-one in `spawnIndex%4` |
| icarus T8 emitters drop events silently (network outage, stale OB_API_KEY) | Audit ref `/tmp/icarus-cmdr-integration-audit.md` §6 — every `WriteEntryAsync` failure logs WARN, no retry, no DLQ | Cmdr-side: enable `ICARUS_POLLER=1` so heartbeat-supplement keeps liveness signal alive even when ob-mcp is down. Long-term: icarus-side env-gating + DLQ (separate spec) |
| Cmdr poller (T2.5) double-writes a row T8 already updated | Detected by T2.5R review; in production, `heartbeat_at` shows 30s churn from two writers | Poller skip-window must be `≥ 2 * CMDR_HEARTBEAT_INTERVAL`; default 60s. If T8 raises its cadence, raise the poller skip-window in lockstep |
| Gemini hook surface differs from `migrate`-implied shape | T3.2 bats test fails | Re-enumerate hook events from `gemini hooks --help` (live, not docs); patch `~/.gemini/hooks/cmdr-bridge-gemini.sh` event names |
| Heartbeat goroutine leak (gemini) | After many gemini sessions, `pgrep -f cmdr-bridge-gemini` shows >N stale processes | Heartbeat loop's `state != working` exit clause failing; 24h escape hatch in heartbeat_loop bounds the leak window |
| `cmdr capability map` not on PATH at bridge runtime | Bridge falls back to `echo "builder"` (the `\|\| echo` fallback in T1.5) | Document in install steps that `cmdr` must be on PATH for bridge to use Go-side capability map; bash fallback ensures correctness even when `cmdr` is missing |
| Reaper false-positive reaps active claude agent because heartbeat_at is null | Old claude rows have `heartbeat_at=NULL`; reaper checks `heartbeat_at < cutoff` and reaps | Reaper logic uses TWO disjoint queries (§3 rule 6, NOT COALESCE): claude→last_activity only, non-claude→heartbeat_at NOT NULL only |
| Reaper false-positive reaps fresh icarus agent because heartbeat_at is null | Brand new icarus row from SSE relay INSERT has heartbeat_at=NULL until first heartbeat fires | Non-claude reaper query has `heartbeat_at IS NOT NULL`; NULL is a no-op. Verified by T2.4 integration test |
| `~/.gemini/settings.json` patch corrupts existing JSON | T3.3 jq check fails | T3.3 uses `jq` for the patch (read+merge+write), not `sed`; backup file before patching |
| 64-char `id` truncation collides two agents in same session whose names differ only in tail | Manual observation; collision rate scales with agent_name length | Acceptable in practice (agent_names typically <20 chars); follow-up spec could lengthen `id` column to 128 chars if real-world collisions appear |
| WAL contention with 3 concurrent runtimes exceeds 5000ms busy_timeout | Bridge logs `WARN Failed to insert`; row dropped silently | T4.2 multi-runtime smoke test runs ≥30s with all three runtimes; if contention observed, raise `busy_timeout` to 10000ms in bridge SQLite invocations |

---

## Open Questions

All seven open questions from the original investigation are answered in the body of this spec. Summary table for reviewer convenience:

| # | Question | Decision (in this spec) | Justification (section) |
|---|----------|-------------------------|------------------------|
| 1 | Single multi-runtime bridge vs per-runtime forks vs Go daemon? | **Per-runtime bash forks.** | §3 rule 2, §10 (gemini bridge fork). Forks keep the failure blast radius small (one bridge bug doesn't break others), match Claude's existing shape, and avoid the 1-2 quarter Go-daemon migration cost. The Go bridge (`cmd/hook-bridge/`) is a separate concern (Anthropic-shaped JSON request/response, not stateful tracker). |
| 2 | Gemini hook event surface (SubagentStart analog?) | **No SubagentStart analog. Use SessionStart/PostToolUse/SessionEnd.** Heartbeat goroutine fills the activity gap. | §10 ("Heartbeat daemon"), §4 (Tracker Protocol Heartbeat section). Verified via `gemini hooks --help` — only `migrate` is exposed but the underlying event surface is Claude-shaped sans subagents. |
| 3 | Block on icarus T8 vs ship polling fallback? | **icarus T8 is MERGED at icarus@`4bf3fb8`/`f7107fb`** (audit ref `/tmp/icarus-cmdr-integration-audit.md`). Ship the poller (T2.5) as a defensive heartbeat-supplement, env-gated `ICARUS_POLLER=1`, default OFF. | §10 ("icarus T8 contract", "Polling fallback"), §15 T2.5. The poller is NOT redundant with T8 — it closes a categorical liveness gap T8 by design does not address (no heartbeat across icarus tree, audit §4). |
| 4 | Use `heartbeat_at` for non-Claude staleness reaping? | **Yes, via TWO disjoint reaper queries (NOT COALESCE).** | §3 rule 6, §5 (Lifecycle table), §15 T2.4. Migration 008 added the column for exactly this. |
| 5 | Session-id collision avoidance across runtimes? | **Runtime-prefixed `sessions.id` (`{runtime}-{session_id}-{agent_name}`); 64-char cap with prefix-wins suffix-loses truncation.** | §3 rule 4, §4 (contract), §9 (migration). Eliminates cross-runtime collision class entirely; preserves Claude backwards compat via fallback resolvers. Truncation policy explicit. |
| 6 | `agents/gemini.yaml` — Gemini-specific tool catalog? | **Yes — adds `WebFetch` to allowed; otherwise mirror icarus shape.** | §4 (gemini.yaml block). Gemini's web grounding is its differentiating capability. |
| 7 | Pane runtime indicator — column or badge? | **Single-character runtime-colored badge prefixed to agent name; `[X]` bracket fallback under NO_COLOR.** | §3 rule 7, §6 (CLI section), §15 T4.1. Saves horizontal space; conveys runtime in 2 chars; aligns with existing ANSI color scheme; survives piped output via NO_COLOR fallback. |

No questions remain unresolved.

---

## Domain-Specific Reference

### SQLite CHECK constraint syntax (for migration 012)

```sql
-- Migration 012: CHECK constraint on sessions.runtime
-- SQLite does not support ALTER TABLE ADD CONSTRAINT; use the table-rebuild pattern.
-- IMPORTANT: The `sessions_new` skeleton MUST enumerate every column from the live
-- schema (post-011_linkedin.sql). T1.4 author derives the list from
-- `for f in 0*.sql; do sqlite3 /tmp/probe.db < "$f"; done` and `PRAGMA table_info(sessions);`.
-- DO NOT use a "... all existing columns ..." placeholder.
PRAGMA foreign_keys = OFF;
BEGIN TRANSACTION;

CREATE TABLE sessions_new (
    -- Enumerate every column from the post-011 live schema in the same order.
    -- Example (illustrative; T1.4 must verify against the actual schema):
    id              TEXT PRIMARY KEY,
    agent_name      TEXT NOT NULL,
    capability      TEXT NOT NULL,
    worktree_path   TEXT,
    branch_name     TEXT,
    task_id         TEXT,
    zellij_pane     TEXT,
    state           TEXT NOT NULL,
    pid             INTEGER,
    parent_agent    TEXT,
    depth           INTEGER,
    run_id          TEXT,
    started_at      TEXT NOT NULL,
    last_activity   TEXT,
    escalation_level INTEGER,
    stalled_since   TEXT,
    transcript_path TEXT,
    runtime         TEXT NOT NULL DEFAULT 'claude'
        CHECK (runtime IN ('claude','gemini','codex','pi','goose','icarus')),
    color_index     INTEGER,
    color_hex       TEXT,
    model           TEXT,
    session_name    TEXT,
    heartbeat_at    TEXT
    -- ... any additional columns from intervening migrations ...
);

INSERT INTO sessions_new SELECT * FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;

-- Recreate indexes from migration 008 + others
CREATE INDEX IF NOT EXISTS idx_sessions_runtime ON sessions(runtime);
CREATE INDEX IF NOT EXISTS idx_sessions_heartbeat ON sessions(heartbeat_at)
    WHERE state IN ('booting', 'working');
-- ... any other indexes from prior migrations ...

COMMIT;
PRAGMA foreign_keys = ON;
```

### Bridge protocol event matrix (cross-reference for emitter authors)

```
Lifecycle event       | Claude          | Gemini           | Icarus (T8 merged + cmdr-side poller fallback)
----------------------|-----------------|------------------|------------------------------------------------
Spawn                 | SubagentStart   | SessionStart     | EventAgentSpawn (TypedBus → ob-mcp SSE)
Activity              | PostToolUse(Task)| PostToolUse     | EventPostToolCall / EventProviderRequest
Heartbeat             | (implicit)      | 30s setsid loop  | T8: NONE. Poller (default OFF): 30s JSONL mtime watch
Completion            | SubagentStop    | SessionEnd       | EventAgentStop
Sweep stale           | SessionStart    | SessionStart     | (icarus boot — T8 unsubscribes)
Cleanup               | SessionEnd      | SessionEnd       | (icarus shutdown)
```

### Capability enum (canonical, used by `MapCapability`)

| Capability | Description |
|------------|-------------|
| `supervisor` | Orchestration; never writes code directly |
| `scout` | Read-only exploration / investigation |
| `builder` | Code authorship + modification (default fallback) |
| `reviewer` | Code review / spec review / validation |
| `lead` | Architecture / tech-lead decisions |
| `merger` | Git merge coordination |
| `coordinator` | Multi-agent coordination (queens, janitors) |
| `monitor` | Observability / metrics / monitoring |

### icarus T8 audit cross-reference

For T2.5 authors and T2.5R reviewers, the authoritative reference for icarus T8 behavior is `/tmp/icarus-cmdr-integration-audit.md` (2026-05-02). Key findings:

- **Files merged on icarus@`main`**: `internal/integration/cmdr/{events.go (648 LOC), evals.go (404 LOC), tags.go (187 LOC)}` plus three test files.
- **Substantive commits**: `4bf3fb8` (squash) and `f7107fb` (merge). HEAD: `2383e2c`.
- **14 lifecycle events** subscribed via `RegisterCmdrEmitters`; **4 eval streams** via `RegisterCmdrEvalEmitters`.
- **Tag scheme**: `agent:icarus + event + event_type:<type> + host:<host> + runtime:icarus + session:<sid>` plus extras.
- **Emit path**: `pkg/ob1client.WriteEntryAsync` (HTTP POST to ob-mcp). NOT cmdr-bridge.sh, NOT SQLite direct.
- **Wired unconditionally** at `internal/cli/agent_factory.go:470-500`. No env gate, no kill-switch.
- **NO HEARTBEAT** — `rg heartbeat` returns zero matches across the entire icarus tree.
- **No retry / DLQ** — `WriteEntryAsync` failures log WARN and drop silently.
- **2s HTTP timeout** with detached parent context.

The poller (T2.5) closes the heartbeat gap — T2.5 does NOT INSERT rows (T8 + cmdr's SSE relay own that path); T2.5 only synthesizes `heartbeat_at` updates when JSONL has activity but no recent ob-mcp event arrived.
