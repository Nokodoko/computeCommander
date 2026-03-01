# internal/agents/templates/ -- Embedded Overlay Template

## Purpose
Contains the Go `text/template` file embedded into the `internal/agents` package binary via `//go:embed`. Used by `BuildOverlay()` to render capability-specific instruction overlays at spawn time.

## Technology
- Go `text/template` syntax
- Embedded via `//go:embed` directive in `internal/agents/overlay.go`

## Contents
| File | Description |
|------|-------------|
| `overlay.tmpl` | Embedded overlay template: renders role, task spec, constraints, allowed/blocked tools, communication protocol |

## Key Functions
N/A -- declarative template, no executable functions.

## Data Types
Same template context as `templates/agent-overlay.tmpl`. See `templates/agentic_instructions.md`.

## Logging
N/A

## CRUD Entry Points
- **Read**: Embedded at compile time and rendered by `internal/agents/overlay.go`

## Style Guide
- Same format as `templates/agent-overlay.tmpl`
- Embedded via `//go:embed templates/overlay.tmpl` in the parent package

**Representative snippet (from `overlay.tmpl`):**
```
## Communication

Use the mail system for all coordination:
- `worker_done` -- signal task completion
- `question` -- ask your parent for clarification
- `escalation` -- escalate a blocker
- `status` -- send progress updates
```
