# plugins/ — Out-of-Process Plugins

## Scope

Plugins that run as separate processes (not Go-linked). Currently Rust + future planned.

**Ownership:** `cmdr_coder` (sole code-edit agent — the agent's foundation is `golang-coder` but its scope-lock includes Rust plugin code here, because the focus-watcher protocol is part of the project's contract).

## Subdirectories

| Path | Language | Purpose |
|------|----------|---------|
| `focus-watcher/` | Rust | `/proc`-based pane focus watcher. Required by the dashboard. Built via `cargo build --release`. |
| `focus-tracker/` | (related) | Companion focus tracking. |

## Key Conventions

- **focus-watcher** uses `/proc` parsing to detect pane focus changes. The output protocol feeds `internal/tui/` and `internal/zellij/`.
- **No `unsafe`** without an inline justification comment.
- **Idiomatic Rust:** `?` propagation; `anyhow` or `thiserror` per existing patterns; no panics in production paths.

## Build / Test

```bash
cd /home/n0ko/Programs/ai/computeCommander && make build-focus-watcher
cd /home/n0ko/Programs/ai/computeCommander/plugins/focus-watcher && cargo build --release
cd /home/n0ko/Programs/ai/computeCommander/plugins/focus-watcher && cargo test
cd /home/n0ko/Programs/ai/computeCommander && make build           # full build (focus-watcher + Go cmdr)
```

`make build` (the project-level full build) requires `cargo`; if cargo is missing, the build hard-fails before Go compilation. Install `rustup` / `cargo` before any task that needs the build to succeed.

Edits to `plugins/focus-watcher/` MUST run `cargo build --release --manifest-path plugins/focus-watcher/Cargo.toml` before commit (per cmdr_coder Project Rule #11).

## LSP / AST tools

- **AST:** `tree-sitter-rust` (NOT `tree-sitter-go` — wrong grammar).
- **LSP:** `rust-analyzer`. **OPTIONAL** — if rust-analyzer is not installed on the host, skip the LSP probe with annotation in the activity entry. Do NOT block a task on a missing rust-analyzer; the AST probe alone is acceptable.

## Read First

Before touching any file in this subtree:

1. `/home/n0ko/Programs/ai/computeCommander/CLAUDE.md`
2. `/home/n0ko/Programs/ai/computeCommander/agentic_instructions.md`
3. `/home/n0ko/Programs/ai/computeCommander/SPEC/CMDR_CODER_AGENT/CMDR_CODER_AGENT.md` (§3 principle 11 — focus-watcher must keep building)
4. `plugins/focus-watcher/Cargo.toml` and the crate's existing `src/main.rs` / `src/lib.rs`.
5. `internal/tui/` and `internal/zellij/` — the consumers of the focus-watcher output protocol.

## Gotchas

- **`/proc` parsing is platform-specific.** Linux only. Do not introduce code paths that assume macOS / BSD `/proc` semantics.
- **Output protocol is part of the project contract.** Changing the focus-watcher's stdout/IPC format requires coordinating with `internal/tui/` and `internal/zellij/`. Plan in 2-3 sentences before code (cmdr_coder Project Rule #4).
- **rust-analyzer optionality.** Skip LSP probes gracefully if absent.
- **Cargo is a hard requirement for the project-level build.** Document missing-cargo as an environment precondition, not a code fault.
