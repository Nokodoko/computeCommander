# agents/ -- Agent Role Definitions

## Purpose
YAML configuration files defining the role hierarchy for AI coding agents in the ComputeCommander swarm system. Each file specifies a single agent capability with its allowed/blocked tools, runtime, model, and constraints.

## Technology
- YAML configuration files
- No executable code; consumed by `internal/agents` and `internal/config`

## Contents
| File | Description |
|------|-------------|
| `builder.yaml` | Implementation agent (scoped_write, no spawn, claude-sonnet-4) |
| `coordinator.yaml` | Persistent orchestrator (read_only, can spawn leads, claude-opus-4) |
| `lead.yaml` | Team coordination (full_write, can spawn up to 5 workers, claude-sonnet-4) |
| `merger.yaml` | Branch merge specialist (merge_only, no spawn, claude-sonnet-4) |
| `monitor.yaml` | Tier 2 continuous patrol (read_only, no spawn, claude-haiku-3) |
| `reviewer.yaml` | Validation and code review (read_only, no spawn, claude-sonnet-4) |
| `scout.yaml` | Read-only exploration (read_only, no spawn, gemini-2.5-pro) |
| `supervisor.yaml` | DEPRECATED -- replaced by `lead.yaml` |
| `icarus.yaml` | Icarus harness builder (runtime: icarus, effort knob via `ICARUS_EFFORT`, claude-sonnet-4-7) |

## Key Functions
N/A -- declarative YAML, no executable functions.

## Data Types

### Agent Definition Schema (per YAML file)
```yaml
name: string          # agent identifier
capability: string    # role enum (scout|builder|reviewer|lead|merger|coordinator|supervisor|monitor)
description: string   # human-readable purpose
runtime: string       # AI runtime (claude|gemini|codex|pi|goose|icarus)
model: string         # model identifier
tools:
  allowed: [string]   # tools the agent may use
  blocked: [string]   # tools explicitly denied
constraints: [string] # behavioural constraints
spawn:                # (optional) spawning limits
  limit: int
  allowed_capabilities: [string]
git:                  # (optional) allowed/blocked git operations
  allowed: [string]
  blocked: [string]
file_scope:           # (optional) glob patterns for file access
  include: [string]
  exclude: [string]
```

## Logging
N/A

## CRUD Entry Points
- **Create**: Add a new YAML file following the schema above
- **Read**: Parse YAML via `gopkg.in/yaml.v3` in `internal/agents` or `internal/config`
- **Update**: Edit the YAML file directly
- **Delete**: Remove the YAML file

## Style Guide
- File names are lowercase, single-word agent role names
- YAML uses 2-space indentation
- Lists use `- item` syntax
- Boolean-like constraints are expressed as string arrays

**Representative snippet (from `builder.yaml`):**
```yaml
name: builder
capability: builder
description: Implementation and code changes

runtime: claude
model: claude-sonnet-4

tools:
  allowed:
    - Read
    - Write
    - Edit
    - Glob
    - Grep
    - Bash
  blocked:
    - Spawn

constraints:
  - file_scope_enforced
  - no_spawn
  - no_git_push
```
