# ComputeCommander Colony Launch

## Pre-Flight Checklist

```bash
# 1. Verify you're on monty
hostname  # should be monty

# 2. Navigate to project
cd ~/Programs/ai/computeCommander

# 3. Verify files exist
ls -la COLONY_BRIEF.md spec.md colony-parallel.md

# 4. Verify wave spawner is executable
ls -la ~/.claude/hooks/colony-wave-spawn.sh

# 5. Verify Go context is accessible
head -20 ~/programming/context/languages/go_context.md

# 6. Initialize git repo (if not already)
git init 2>/dev/null || true
git add -A && git commit -m "Initial: specs and colony brief" 2>/dev/null || true

# 7. Ensure jq is installed (needed by wave spawner)
which jq || sudo pacman -S jq

# 8. Ensure zellij is running or will auto-start
which zellij
```

---

## Launch Command

From within `~/Programs/ai/computeCommander`, run:

```bash
/colony "Build ComputeCommander from COLONY_BRIEF.md.

PROJECT: ~/Programs/ai/computeCommander
SPEC: spec.md (100KB full specification)
GO PATTERNS: ~/programming/context/languages/go_context.md

EXECUTION PROTOCOL:
Use PARALLEL wave spawning. DO NOT use sequential Task tool for supervisors.

For each wave:
1. Create /tmp/colony-hive/wave-specs/ directory
2. Write team spec JSON files (see COLONY_BRIEF.md for format)
3. Call: /home/n0ko/.claude/hooks/colony-wave-spawn.sh /tmp/colony-hive/wave-specs/
4. Monitor hive for TEAM_COMPLETE messages from ALL teams in wave
5. Only proceed to next wave when current wave is 100% complete
6. Clear wave-specs directory before writing next wave

WAVE PLAN:
- Wave 1: Track A (CLI+Config), Track B (Database) — 2 parallel
- Wave 2: Track C (Zellij), Track D (Mail), Track E (Runtimes) — 3 parallel
- Wave 3: Track F (Watchdog), Track G (Merge), Track H (Overlay) — 3 parallel
- Wave 4: Track I (TUI), Track J (Integrations) — 2 parallel
- Wave 5: Track K (Assembly) — 1 final

FILE OWNERSHIP: Each track has EXCLUSIVE write access to its paths. See COLONY_BRIEF.md.

SUCCESS: Binary 'cc' that passes all integration tests.

Begin orchestration now."
```

---

## Monitor Progress

### Hive Dashboard
The colony dashboard should auto-launch. If not:
```bash
~/.claude/hooks/colony-overstory.sh start
```

### Check Team Status
```bash
# List active supervisors
zellij action list-clients

# Check hive messages
cat /tmp/colony-hive/*.log | tail -50

# View specific supervisor pane
zellij action focus-tab --name "sup-track-a"
```

### Manual Wave Status
```bash
# Count completed teams
ls /tmp/colony-hive/complete-*.json 2>/dev/null | wc -l

# View completion summaries
cat /tmp/colony-hive/complete-*.json | jq -s '.'
```

---

## Intervention Commands

### Nudge a Stalled Supervisor
```bash
# Soft nudge (send message)
~/.claude/hooks/colony-hive-send.sh wall "Track A: status check requested" warning

# Hard nudge (view pane, send input)
zellij action focus-tab --name "sup-track-a"
```

### Kill a Stuck Team
```bash
# Close supervisor pane
zellij action close-tab --name "sup-track-a"

# Respawn manually
~/.claude/hooks/colony-wave-spawn.sh /tmp/colony-hive/wave-specs/track-a.json
```

### Emergency Stop
```bash
# Kill all supervisors
zellij action close-all-tabs

# Stop hive
~/.claude/hooks/colony-hive-spawn.sh stop-sync

# Stop dashboard
~/.claude/hooks/colony-overstory.sh stop
```

---

## Post-Build Verification

After colony completes:

```bash
cd ~/Programs/ai/computeCommander

# Build
go build -o cc ./cmd/cc/

# Verify binary
./cc --help
./cc --version

# Run tests
go test ./... -v

# Test core functionality
./cc init --help
./cc sling --help
./cc status --help
./cc dashboard --help

# Integration test
mkdir /tmp/cc-test && cd /tmp/cc-test
~/Programs/ai/computeCommander/cc init
cat .computecommander/config.yaml
```

---

## Estimated Timeline

| Wave | Teams | Est. Duration |
|------|-------|---------------|
| 1 | 2 | 2-3 hours |
| 2 | 3 | 3-4 hours |
| 3 | 3 | 3-4 hours |
| 4 | 2 | 2-3 hours |
| 5 | 1 | 1-2 hours |

**Total: 11-16 hours** (parallel execution)

---

## Troubleshooting

### "Hive socket not found"
```bash
~/.claude/hooks/colony-hive-spawn.sh start-sync
```

### "zellij: command not found"
```bash
cargo install zellij
# or
sudo pacman -S zellij
```

### "Queen not spawning teams"
Check queen is using wave spawner, not Task tool. Review queen output in its pane.

### "Build fails with import errors"
A dependent track may not have exported its interfaces yet. Check wave dependencies — don't skip ahead.

### "Merge conflicts"
File ownership violation. Check which track modified files outside its boundary. Revert and reassign.

---

## Files Reference

| File | Purpose |
|------|---------|
| `COLONY_BRIEF.md` | Executive summary, wave plan, ownership table |
| `spec.md` | Full 100KB technical specification |
| `colony-parallel.md` | Parallel execution protocol details |
| `launch.md` | This file — launch instructions |
| `~/.claude/hooks/colony-wave-spawn.sh` | Parallel supervisor spawner |

---

🚀 **Ready to launch. Good luck, commander.**
