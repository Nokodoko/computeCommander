# Colony Prompt (Parallel Execution)

## Modified Invocation

```bash
cd ~/Programs/ai/computeCommander

/colony "Build ComputeCommander using COLONY_BRIEF.md. Execute waves in PARALLEL within each wave. Use the parallel-spawn protocol: write all team specs first, then invoke colony-wave-spawn.sh to launch supervisors simultaneously. Full spec at spec.md. Go patterns at ~/programming/context/languages/go_context.md."
```

---

## Modified Queen Instructions (Add to queen-agent.md or override in prompt)

### Parallel Spawn Protocol

**DO NOT use sequential Task tool spawning.** Instead, use the wave-based parallel spawner:

#### Step 1: Write Team Specs

For each team in the current wave, write a spec file:

```bash
mkdir -p /tmp/colony-hive/wave-specs

cat > /tmp/colony-hive/wave-specs/track-a.json << 'EOF'
{
  "team_id": "track-a",
  "name": "Foundation",
  "charter": "Build CLI framework, config system, database layer",
  "priority": 1,
  "runtime": "claude",
  "model": "claude-sonnet-4",
  "spec_file": "~/Programs/ai/computeCommander/specs/01-foundation.md",
  "file_ownership": ["go.mod", "go.sum", "cmd/", "internal/config/", "internal/platform/db/"]
}
EOF
```

Write ALL team specs for the wave before spawning any.

#### Step 2: Parallel Wave Spawn

Invoke the wave spawner to launch all teams simultaneously:

```bash
/home/n0ko/.claude/hooks/colony-wave-spawn.sh /tmp/colony-hive/wave-specs/
```

This script:
1. Reads all JSON specs in the directory
2. Sends TEAM_CREATE to hive for each
3. Spawns supervisor processes in PARALLEL (background with &)
4. Waits for all spawns to initialize
5. Returns when all supervisors are registered with hive

#### Step 3: Wait for Wave Completion

Monitor hive for TEAM_COMPLETE messages. Only proceed to next wave when ALL teams in current wave report complete:

```bash
# Poll for completion
while true; do
  COMPLETED=$(/home/n0ko/.claude/hooks/colony-hive-send.sh query-complete "wave-1")
  if [ "$COMPLETED" = "all" ]; then
    break
  fi
  sleep 30
done
```

#### Step 4: Next Wave

Clear wave specs directory, write next wave specs, repeat.

---

## Wave Definitions for ComputeCommander

### Wave 1: Foundation (Parallel: 2 teams)
```
Track A: CLI + Config
Track B: Database Layer
```
No dependencies. Spawn simultaneously.

### Wave 2: Core Infrastructure (Parallel: 3 teams)  
```
Track C: Zellij + Worktree (depends: A)
Track D: Mail System (depends: B)
Track E: Runtime Interface (depends: A)
```
Wait for Wave 1. Then spawn C, D, E simultaneously.

### Wave 3: Orchestration (Parallel: 3 teams)
```
Track F: Watchdog (depends: B, D)
Track G: Merge Queue (depends: B, C)
Track H: Agent Overlay (depends: E)
```
Wait for Wave 2. Then spawn F, G, H simultaneously.

### Wave 4: Interface (Parallel: 2 teams)
```
Track I: TUI Dashboard (depends: all above)
Track J: Integrations (depends: A, B)
```
Wait for Wave 3. Spawn I, J simultaneously.

### Wave 5: Final Assembly (Sequential: 1 team)
```
Track K: Integration + Testing
```
Wait for Wave 4. Final merge and integration tests.

---

## colony-wave-spawn.sh (New Script)

Create this script at `~/.claude/hooks/colony-wave-spawn.sh`:

```bash
#!/usr/bin/env bash
# colony-wave-spawn.sh - Spawn all supervisors in a wave SIMULTANEOUSLY
#
# Usage: colony-wave-spawn.sh <specs-dir>
#
# Reads all *.json files in specs-dir, spawns a supervisor for each in parallel.

set -uo pipefail

SPECS_DIR="${1:?Usage: colony-wave-spawn.sh <specs-dir>}"
HIVE_SOCK="/tmp/colony-hive.sock"
PIDS=()

log() {
    echo "[wave-spawn] $(date +%H:%M:%S) $*"
}

spawn_supervisor() {
    local spec_file="$1"
    local team_id name charter runtime model spec_file_path ownership
    
    team_id=$(jq -r '.team_id' "$spec_file")
    name=$(jq -r '.name' "$spec_file")
    charter=$(jq -r '.charter' "$spec_file")
    runtime=$(jq -r '.runtime // "claude"' "$spec_file")
    model=$(jq -r '.model // "claude-sonnet-4"' "$spec_file")
    spec_file_path=$(jq -r '.spec_file // ""' "$spec_file")
    ownership=$(jq -r '.file_ownership | join(", ")' "$spec_file")
    
    log "Spawning supervisor for $team_id ($name)"
    
    # Send TEAM_CREATE to hive
    /home/n0ko/.claude/hooks/colony-hive-send.sh team-create "$team_id" "$name" "$charter" 1
    
    # Write pending team for supervisor hook
    echo "{\"team_id\":\"$team_id\"}" > /tmp/colony-hive/pending-team-${team_id}.json
    
    # Spawn supervisor in background via zellij
    zellij run --name "sup-${team_id}" --floating -- \
        claude --dangerously-skip-permissions -p "
You are supervisor for team '$name' (ID: $team_id).

Charter: $charter

File Ownership (ONLY modify these paths): $ownership

Spec file: $spec_file_path

1. Register with hive as supervisor
2. Read the spec file for detailed requirements
3. Spawn builder workers as needed (use Task tool)
4. Monitor progress, report STATUS_REPORT every 5 minutes
5. When complete, send TEAM_COMPLETE to hive

Hive socket: $HIVE_SOCK
" &
    
    PIDS+=($!)
}

# Main
log "Starting wave spawn from $SPECS_DIR"

# Spawn all supervisors in parallel
for spec_file in "$SPECS_DIR"/*.json; do
    [ -f "$spec_file" ] || continue
    spawn_supervisor "$spec_file" &
done

# Wait for all spawn processes to start
wait

log "All supervisors launched. PIDs: ${PIDS[*]}"

# Give supervisors time to register with hive
sleep 5

log "Wave spawn complete"
```

---

## File Ownership Table

| Track | Owns |
|-------|------|
| A (CLI+Config) | `go.mod`, `go.sum`, `cmd/cc/`, `internal/config/`, `Makefile` |
| B (Database) | `internal/platform/db/`, `migrations/` |
| C (Zellij) | `internal/zellij/`, `internal/worktree/` |
| D (Mail) | `internal/mail/` |
| E (Runtimes) | `pkg/runtimes/` |
| F (Watchdog) | `internal/watchdog/` |
| G (Merge) | `internal/merge/` |
| H (Overlay) | `internal/agents/`, `agents/`, `templates/` |
| I (TUI) | `internal/tui/` |
| J (Integrations) | `internal/gateway/`, `pkg/integrations/` |
| K (Assembly) | `internal/commands/`, `docs/`, `README.md` |

**Rule:** Each track may ONLY create/modify files in its owned paths. Imports across boundaries are fine, but file writes are exclusive.

---

## Dependency Graph (Visual)

```
Wave 1:    [A: CLI+Cfg]     [B: Database]
               │                  │
               ▼                  ▼
Wave 2:    [C: Zellij]  [D: Mail]  [E: Runtimes]
               │            │           │
               └─────┬──────┴───────────┘
                     ▼
Wave 3:    [F: Watchdog]  [G: Merge]  [H: Overlay]
               │              │            │
               └──────────────┴────────────┘
                              │
                              ▼
Wave 4:           [I: TUI]        [J: Integrations]
                      │                  │
                      └────────┬─────────┘
                               ▼
Wave 5:                   [K: Assembly]
```

---
