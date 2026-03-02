# templates/ -- Agent Overlay Templates

## Purpose
Contains Go `text/template` files used to generate capability-specific instruction overlays for agents. The overlay is rendered with agent metadata (capability, constraints, tools, task spec) and deployed to the agent's worktree.

## Technology
- Go `text/template` syntax
- Consumed by `internal/agents/overlay.go` via `BuildOverlay()`

## Contents
| File | Description |
|------|-------------|
| `agent-overlay.tmpl` | Main overlay template: renders role, task spec path, constraints list, allowed/blocked tools lists, and communication protocol instructions |

## Key Functions
N/A -- declarative template, no executable functions.

## Data Types

### Template Context (passed by `BuildOverlay()`)
| Field | Type | Description |
|-------|------|-------------|
| `.Capability` | string | Agent role (scout, builder, reviewer, etc.) |
| `.TaskSpec` | string | Path to task specification file |
| `.Constraints` | []string | Behavioral constraints (e.g., "file_scope_enforced", "no_spawn") |
| `.Tools.Allowed` | []string | Tools the agent may use |
| `.Tools.Blocked` | []string | Tools explicitly denied |

## Logging
N/A

## CRUD Entry Points
- **Read**: Template loaded and rendered by `internal/agents/overlay.go`

## Style Guide
- Go `text/template` with `{{ .Field }}` interpolation
- `{{ range }}` loops for list items
- `{{ if }}` guards for optional sections (blocked tools, task spec)
- Markdown output format

**Representative snippet (from `agent-overlay.tmpl`):**
```
# ComputeCommander Agent Overlay

## Role: {{ .Capability }}
{{ if .TaskSpec }}
## Task

Read your task specification at: `{{ .TaskSpec }}`
{{ end }}
## Rules

### Allowed Tools
{{ range .Tools.Allowed }}
- {{ . }}
{{ end }}
```
