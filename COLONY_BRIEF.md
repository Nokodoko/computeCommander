# ComputeCommander Colony Brief v2
## Parallel Execution Model

**Mission:** Build ComputeCommander — Go-based multi-agent orchestration with Zellij, Postgres, Gum TUI.

**Full Spec:** `~/Programs/ai/computeCommander/spec.md` (100KB)

---

## ⚡ Parallel Execution Protocol

**DO NOT** spawn supervisors sequentially via Task tool.

**USE** the wave-based parallel spawner:

```bash
# 1. Write all team specs for current wave
mkdir -p /tmp/colony-hive/wave-specs/
# Write track-X.json for each team...

# 2. Spawn ALL teams in wave simultaneously  
/home/n0ko/.claude/hooks/colony-wave-spawn.sh /tmp/colony-hive/wave-specs/

# 3. Wait for ALL teams to complete before next wave
# Monitor hive for TEAM_COMPLETE messages

# 4. Clear specs dir, repeat for next wave
rm /tmp/colony-hive/wave-specs/*.json
```

---

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | **Go 1.22+** |
| Database | **Postgres** + **SQLite** fallback |
| TUI | **Gum** |
| Multiplexer | **Zellij** |
| Config | **YAML** |

---

## Wave Execution Plan

### Wave 1: Foundation (2 teams, parallel)
| Track | Name | Owns | Spec Section |
|-------|------|------|--------------|
| A | CLI + Config | `go.mod`, `go.sum`, `cmd/cc/`, `internal/config/`, `Makefile` | §4.1-4.2 |
| B | Database | `internal/platform/db/`, `migrations/` | §4.3 |

**Spawn simultaneously. No dependencies.**

---

### Wave 2: Core Infrastructure (3 teams, parallel)
| Track | Name | Owns | Depends On |
|-------|------|------|------------|
| C | Zellij + Worktree | `internal/zellij/`, `internal/worktree/` | A |
| D | Mail System | `internal/mail/` | B |
| E | Runtime Interface | `pkg/runtimes/` | A |

**Wait for Wave 1 complete. Then spawn C, D, E simultaneously.**

---

### Wave 3: Orchestration (3 teams, parallel)
| Track | Name | Owns | Depends On |
|-------|------|------|------------|
| F | Watchdog | `internal/watchdog/` | B, D |
| G | Merge Queue | `internal/merge/` | B, C |
| H | Agent Overlay | `internal/agents/`, `agents/`, `templates/` | E |

**Wait for Wave 2 complete. Then spawn F, G, H simultaneously.**

---

### Wave 4: Interface (2 teams, parallel)
| Track | Name | Owns | Depends On |
|-------|------|------|------------|
| I | TUI Dashboard | `internal/tui/` | All above |
| J | Integrations | `internal/gateway/`, `pkg/integrations/` | A, B |

**Wait for Wave 3 complete. Then spawn I, J simultaneously.**

---

### Wave 5: Assembly (1 team)
| Track | Name | Owns | Depends On |
|-------|------|------|------------|
| K | Final Assembly | `internal/commands/`, `docs/`, `README.md` | All |

**Wait for Wave 4 complete. Final integration, tests, polish.**

---

## Team Spec Format

Write to `/tmp/colony-hive/wave-specs/track-X.json`:

```json
{
  "team_id": "track-a",
  "name": "CLI + Config",
  "charter": "Build CLI framework with cobra, YAML config parsing, validation",
  "priority": 1,
  "runtime": "claude",
  "model": "claude-sonnet-4",
  "spec_section": "Read spec.md sections 4.1, 4.2, 5.1",
  "file_ownership": ["go.mod", "go.sum", "cmd/cc/", "internal/config/", "Makefile"],
  "interfaces_export": [
    "type Config struct",
    "func LoadConfig(path string) (*Config, error)"
  ],
  "success_criteria": [
    "go build ./cmd/cc/",
    "cc --help prints usage",
    "cc init creates .computecommander/"
  ]
}
```

---

## File Ownership Rules

**CRITICAL:** Each track may ONLY create/modify files in its owned paths.

| Track | Exclusive Ownership |
|-------|---------------------|
| A | `go.mod`, `go.sum`, `cmd/cc/`, `internal/config/`, `Makefile` |
| B | `internal/platform/db/`, `migrations/` |
| C | `internal/zellij/`, `internal/worktree/` |
| D | `internal/mail/` |
| E | `pkg/runtimes/` (all subdirs) |
| F | `internal/watchdog/` |
| G | `internal/merge/` |
| H | `internal/agents/`, `agents/`, `templates/` |
| I | `internal/tui/` |
| J | `internal/gateway/`, `pkg/integrations/` |
| K | `internal/commands/`, `docs/`, `README.md` |

**Imports allowed.** Cross-track file writes = FORBIDDEN.

---

## Interface Contracts

Each track exports interfaces. Dependent tracks import them.

### Track A exports:
```go
// internal/config/config.go
type Config struct { ... }
func LoadConfig(path string) (*Config, error)
func (c *Config) Validate() error
```

### Track B exports:
```go
// internal/platform/db/db.go
type DB interface {
    Exec(ctx context.Context, query string, args ...any) error
    Query(ctx context.Context, query string, args ...any) (*Rows, error)
    Close() error
}
func NewPostgres(cfg PostgresConfig) (DB, error)
func NewSQLite(path string) (DB, error)
```

### Track C exports:
```go
// internal/zellij/zellij.go
func SpawnPane(name, cmd string, opts PaneOpts) (string, error)
func AttachFloating(paneID string, opts AttachOpts) error
func CapturePane(paneID string, lines int) (string, error)

// internal/worktree/worktree.go
func Create(branch, path string) error
func Remove(path string) error
func List() ([]Worktree, error)
```

### Track D exports:
```go
// internal/mail/mail.go
type MailStore interface {
    Send(msg Message) error
    Check(agent string) ([]Message, error)
    MarkRead(id string) error
}
func NewMailStore(db db.DB) MailStore
```

### Track E exports:
```go
// pkg/runtimes/runtime.go
type AgentRuntime interface {
    ID() string
    BuildSpawnCommand(opts SpawnOpts) string
    DeployConfig(worktree string, overlay Overlay) error
    DetectReady(paneContent string) ReadyState
}
func GetRuntime(id string) (AgentRuntime, error)
```

*(Continue pattern for F-K)*

---

## Success Criteria

### Per-Track:
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (>70% coverage)
- [ ] Exported interfaces documented
- [ ] No files outside owned paths modified

### Final (Track K):
- [ ] `cc init` creates project structure
- [ ] `cc sling task-1 --runtime claude` spawns agent
- [ ] `cc status` shows agent state  
- [ ] `cc dashboard` renders TUI
- [ ] `cc mail check` retrieves messages
- [ ] Full integration test suite passes

---

## Reference Files

| Resource | Path |
|----------|------|
| Full Spec | `~/Programs/ai/computeCommander/spec.md` |
| Go Context | `~/programming/context/languages/go_context.md` |
| Overstory Source | `~/Programs/ai/overstory-publish/` |

---

## Queen Checklist

1. ✅ Hive server running
2. ✅ Generate team specs for Wave 1
3. ✅ Spawn Wave 1 with `colony-wave-spawn.sh` (PARALLEL)
4. ⏳ Wait for ALL Wave 1 TEAM_COMPLETE
5. 🔄 Repeat for Waves 2-5
6. ✅ Send WALL summary when all complete

**Total teams:** 11
**Estimated runtime:** 8-16 hours (parallel execution)
