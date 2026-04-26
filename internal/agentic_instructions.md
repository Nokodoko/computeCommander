# internal/ — Project-Internal Packages

## Scope

All non-public Go packages. Cannot be imported from outside this repo.

**Ownership:** `cmdr_coder` (sole code-edit agent for this subtree).

## Key Abstractions (interface-first)

Each interface has 2+ concrete implementations. These are the load-bearing types of the system; refactoring any of them requires AST + gopls grounding before edit (per cmdr_coder Project Rule #3).

| Interface | Defined In | Implementations |
|-----------|-----------|-----------------|
| `AgentRuntime` | `pkg/runtimes/runtime.go` (public) | 5 adapters in `pkg/runtimes/{claude,gemini,codex,pi,goose}/`, self-registering via `init()` + `RegisterRuntime()`. |
| `DB` | `internal/platform/db/` | SQLite (`modernc.org/sqlite`) + PostgreSQL (`pgx/v5`). Same migrations live in `internal/platform/db/migrations/` and root `migrations/` mirror. |
| `MailStore` | `internal/mail/` | SQL-backed store (`sql_store.go`). Inter-agent mail with priorities. |
| `MergeQueue` | `internal/merge/` | FIFO queue + 4-tier conflict resolution. |
| `WorktreeManager` | `internal/worktree/` | git worktree lifecycle. |
| `PaneManager` | `internal/zellij/` | Zellij pane mgmt + KDL layout generation. |
| `WindowManager` | `internal/wezterm/` | WezTerm window orchestration. |

## Subdirectories

| Path | Purpose |
|------|---------|
| `agentic/` | Agent role definitions / overlays |
| `agents/` | Agent lifecycle: spawn, stop, guards, overlays |
| `backup/` | DB backup / restore |
| `commands/` | CLI command handlers (App DI container) |
| `config/` | Config schema, loading, validation, fsnotify file watcher |
| `darkfactory/` | Darkfactory pipeline integration |
| `export/` | DB export to JSON/CSV |
| `gateway/` | HTTP REST API gateway (`/api/v1/`) |
| `jiraboard/` | Jira board generator |
| `keybinds/` | Leader-key keybind config + action registry |
| `linkedin/` | LinkedIn post generator pipeline |
| `mail/` | Inter-agent mail system |
| `merge/` | FIFO merge queue + 4-tier conflict resolution |
| `platform/db/` | DB abstraction (SQLite + PostgreSQL) |
| `sse/` | Server-Sent Events relay |
| `trustgraph/` | Trustgraph viz / event source |
| `tui/` | BubbleTea dashboard (status, mail, costs, file picker, sessions) |
| `watchdog/` | 3-tier health monitoring daemon |
| `wezterm/` | WezTerm window management |
| `worktree/` | git worktree lifecycle |
| `zellij/` | Zellij pane management + KDL layout generation |

## Build / Test

```bash
cd /home/n0ko/Programs/ai/computeCommander && make build       # full build (includes Rust focus-watcher)
cd /home/n0ko/Programs/ai/computeCommander && go test ./internal/...
cd /home/n0ko/Programs/ai/computeCommander && go test ./internal/... -race   # explicit race probe
cd /home/n0ko/Programs/ai/computeCommander && make vet
cd /home/n0ko/Programs/ai/computeCommander && make lint        # best-effort, NOT a gate
```

Edits to `internal/agents`, `internal/platform/db`, or `internal/zellij` MUST run `make build && go test ./... && make vet` before commit (per cmdr_coder Project Rule #11).

## Read First

Before touching any file in this subtree:

1. `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md`
2. `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md`
3. `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` (especially §3 and §10)
4. The interface definition file for any interface you'll touch (e.g., `internal/platform/db/db.go` for `DB`).
5. Existing implementations in sibling packages for patterns and conventions.

## Gotchas

- **Dual-DB invariant:** migrations in `internal/platform/db/migrations/` AND root `migrations/` MUST stay in sync.
- **fsnotify + SIGUSR1:** `internal/config/` uses fsnotify to trigger SIGUSR1 → file picker refresh in `internal/tui/`. Don't break the signal chain.
- **MailStore SQL params:** `?` positional only; `CREATE TABLE IF NOT EXISTS` for idempotence.
- **Speculative interfaces are forbidden** (cmdr_coder Project Rule #9): introduce an interface only when 2+ concrete implementations exist.
- **No mocks in production paths** (cmdr_coder Project Rule #7): mocks live in `_test.go` or behind `//go:build test`.
