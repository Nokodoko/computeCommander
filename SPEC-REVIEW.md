# SPEC Review

## Verdict: PASS WITH WARNINGS

---

## Findings

### Critical (blocks execution)

1. **Naming conflict: `cmdr stop` (lifecycle) vs existing `stop` (agent termination).** The spec introduces `cmdr stop` as a lifecycle command to "Stop DB + close UI," but the codebase already has `StopCmd` in `internal/commands/stop.go` with `Use: "stop <agent-name>"` and `Args: cobra.ExactArgs(1)`. Cobra will reject two commands with the same `Use` prefix at the same level. The spec's Task Manifest (T4) writes to `internal/commands/lifecycle.go` and proposes a `StopCmd` there, which will collide with the existing exported function name in the same package. The spec must resolve this: either rename the lifecycle stop (e.g., `cmdr halt`, `cmdr shutdown`) or refactor the existing agent stop to a subcommand (e.g., `cmdr agent stop <name>`). Without resolution, T4 cannot be implemented without breaking the existing `stop` command.

2. **File name mismatch in Directory Structure vs actual codebase.** The spec lists `internal/tui/merge_queue_view.go` in the Project Infrastructure section (line ~815) as an "Unchanged" file. The actual file is `internal/tui/merge_view.go`. Agents reading the spec would look for a file that does not exist and may create a duplicate. This also means T9 (Dashboard layout restructure) has an incorrect read-scope entry.

3. **`cmdr logs` spec describes flags (`--follow`, `--lines <n>`) that do not exist on the current `LogsCmd`.** The spec's CLI section (Observability Commands) describes `cmdr logs` with `--follow` and `--lines <n>` flags. The existing `LogsCmd` in `internal/commands/observability.go` only has `--agent`, `--level`, and `--limit`. The spec claims this is an existing/preserved command, but it actually requires enhancement. This is not tracked in the Task Manifest -- no task covers adding `--follow` or `--lines` flags to the `logs` command. An agent implementing T7 (utility commands including `clear`) might assume the logs command is already correct per spec, leaving a gap.

4. **`clear.go` already exists in the codebase but spec treats it as "NEW."** The spec's Directory Structure lists `internal/commands/clear.go` as `# NEW: clear logs command`. However, `internal/commands/clean.go` already exists and is a different command (`CleanCmd`). While `clear.go` vs `clean.go` are different files, the spec also lists `clean.go` as `# Unchanged` right below. This is actually fine at the file level, but the spec must be precise: the Task Manifest (T7) says `clear.go` is in the write-scope for "Utility commands: shell, feedback, support, clear." The spec should acknowledge that `clear` and `clean` are separate commands to avoid agent confusion.

### Warnings (should fix but won't block)

1. **Data model interfaces use TypeScript syntax in a Go project.** The Data Model section (ProcessState, KeybindConfig, BackupRecord, ExportData, ThemeConfig, DirectorySession, PluginManifest) is written in TypeScript interface notation. While this is clearly meant as a schema description language, it could confuse agents implementing Go structs. They will need to mentally translate `?` (optional) to pointer types, `Record<K,V>` to `map[K]V`, union types to custom enums, etc. Recommend adding Go struct definitions alongside or instead of TypeScript.

2. **Execution Order vs Dependency Graph inconsistency.** The "Execution Order" section has 6 phases, while the "Dependency Graph" section has 4 phases + final. Mapping:
   - Execution Order Phase 1 = Dependency Graph Phase 1: T1, T3, T16 (consistent)
   - Execution Order Phase 2 = Dependency Graph Phase 2: mostly consistent, but Execution Order puts T12 (config hot-reload) in Phase 3 (UI Layer), while the Dependency Graph puts T12 in Phase 2. The Dependency Graph is more correct since T12 depends only on T1.
   - Execution Order Phase 3 (UI Layer) includes "Dashboard layout restructure" which is T9, but the Dependency Graph places T9 in Phase 2, not Phase 3.
   - T19 (file picker) and T20 (session commands) appear in Dependency Graph Phase 3, but the Execution Order does not mention them at all -- they are absent from all 6 phases.
   - The Execution Order has a Phase 5 for reviews and Phase 6 for integration, while the Dependency Graph collapses reviews into Phase 4.
   Agents choosing the Dependency Graph will execute in a different order than agents following the Execution Order prose.

3. **`cmdr config` is defined in two places.** The config command is currently defined in `cmd/cc/main.go` (as `configCmd()` with subcommands `show`, `validate`, `get`, `set`, `edit`). The spec's CLI section re-lists `cmdr config` under "Configuration Commands" but the spec says "Existing commands preserved." However, the spec also says config needs hot-reload on save (Design Principle 7), which requires modifying `configEditCmd` or adding a watcher. The `config` command is not in the Task Manifest at all -- T12 covers the watcher but not the integration with `config edit`.

4. **Placeholder commands inflate scope without clear value.** The spec explicitly acknowledges in "What It Does NOT Do" that `cmdr automation`, `cmdr notifications`, `cmdr integrations`, and `cmdr analytics` are placeholders with no backend. However, T8 (settings commands) allocates agent time to implement these. The estimated LOC for settings.go is not broken out, but creating 5 placeholder commands with subcommands (each with `list`, `set`, `add`, `remove`, `create`, `run`, `delete` sub-commands for automation alone) is significant busywork that could be deferred entirely.

5. **The `Ctrl+Space` leader key will conflict with zellij's default keybinds.** The spec acknowledges this in the Failure Modes table ("user must unbind `Ctrl+Space` in zellij config"), but this is a significant UX friction point that every user will hit on first run. The spec does not include any `cmdr init` step to generate a zellij config that unbinds or remaps this key. Recommend either: (a) choosing a leader key that doesn't conflict (e.g., `Ctrl+\`), or (b) having `cmdr init` generate a zellij keybind config that clears `Ctrl+Space`.

6. **Leader key implementation approach is underspecified.** The spec says "Add leader key handler to TUI event loop" (T11), but the keybinds are supposed to work *within zellij panes*, not within the bubbletea TUI. The bubbletea TUI runs in one pane; it cannot intercept keystrokes in other panes (agent_session, events, mail, merge_queue). The spec needs to clarify: is the leader key a *zellij-level* keybind (using zellij's native keybind system or a plugin) or a *bubbletea-level* keybind (only works when the TUI pane is focused)? If the former, the implementation is zellij config generation, not Go code. If the latter, it only works in one pane and the spec's UX vision is broken. This architectural ambiguity could cause significant rework.

7. **No task covers updating the KDL layout file shipped with `cmdr init`.** The current `cmdr init` does not generate a KDL layout file at all (checked in `runInit` in `cmd/cc/main.go`). The spec's On-Disk Format shows `layouts/cmdr-dashboard.kdl` under `.computecommander/`. T13 covers "Generate KDL layout file for redesigned dashboard" but its write-scope is `internal/zellij/layout.go` -- a Go file for layout generation, not the actual template or init integration. T15 covers "Update cmdr init to generate keybinds.yaml and open interface after DB start" but does not mention generating the KDL layout. This is a gap: no task ensures the layout KDL file is actually generated during `cmdr init`.

8. **`--agent` flag on `cmdr logs` creates ambiguity with existing `--agent` flag.** The existing `LogsCmd` already accepts `--agent` as a local flag. The spec adds `--agent` as a global persistent flag on the root command. Cobra allows both, but when both a local and persistent flag have the same name, the local flag takes precedence. This is fine, but agents implementing T2 (global flags) may not realize the collision exists and could inadvertently break the existing behavior in `LogsCmd`, `TraceCmd`, and other commands that already define `--agent` locally.

9. **Missing `backups/` and `layouts/` and `themes/` directory creation in `cmdr init`.** The current `runInit` creates: `.computecommander/`, `agents/`, `hooks/`, `specs/`, `worktrees/`, `logs/`. The spec's On-Disk Format also requires `backups/`, `layouts/`, `plugins/`, and `themes/`. T15 covers updating init but only mentions keybinds.yaml generation. The directory creation gap means `cmdr backup` (T6) will fail on first run unless the backup command creates the directory itself.

10. **`truncate` function is defined in two packages.** Both `internal/commands/status.go` and `internal/tui/render.go` define a `func truncate(s string, maxLen int) string` with slightly different implementations (different behavior at maxLen <= 2 vs maxLen < 3). Adding more command files (lifecycle.go, info.go, data.go, etc.) that call `truncate` will work since they are in the same package, but the duplication is a DRY violation that could cause subtle bugs. The spec does not flag this for cleanup.

### Notes (observations, suggestions)

1. **Spec is well-structured and thorough.** The 19-section structure (Why, Design Principles, On-Disk Format, Data Model, CLI, JSON Output, Concurrency, Migration, Integration, What It Does NOT Do, Tech Stack, Project Infrastructure, Estimated Size, UI Layout, Agent Assignments, Execution Order, Failure Modes, Task Manifest, Dependency Graph, Target State, Verification Plan, Success Criteria, Open Questions) covers all angles an agent swarm needs.

2. **Success criteria are machine-verifiable.** The 20 checkboxes in section 19 are all testable with grep, file existence checks, and command invocations. This is excellent for automated verification.

3. **Open questions are all deferrable.** Questions 1-3 (URLs) have reasonable defaults. Question 4 (update check mechanism) has a clear default. Questions 5-8 have suggested answers that unblock implementation. None of these should block execution.

4. **Estimated size (~4,150 LOC) is realistic** for the scope described: 6 new command files, keybind system, floating panes, config watcher, backup/restore, export, layout generation, file picker, and tests. The per-file estimates are within expected ranges for Go.

5. **The file picker (fp) pane is architecturally complex** and underspecified relative to its complexity. It requires: directory tree rendering, keyboard navigation, session lifecycle management, pane content swapping in zellij, and MRU state persistence. At ~450 LOC estimated, this may be the highest-risk component. Consider breaking T19 into sub-tasks.

6. **The spec does not mention whether the `cc` binary symlink or alias should be preserved.** Open Question 5 mentions keeping `cc` as an alias, but the suggested default says "add `cc` as an alias" without specifying how (Cobra `Aliases` on root doesn't work like that; it would need a symlink, shell alias, or a separate binary name). This is a minor UX regression risk if existing users have `cc` in their muscle memory or scripts.

7. **Version bump from 0.1.0 to 0.2.0.** The spec's Makefile section shows `VERSION ?= 0.2.0`, while the current Makefile has `VERSION ?= 0.1.0`. This is intentional and correct -- the redesign warrants a minor version bump. Just noting it for awareness during implementation.

8. **The spec mentions `fsnotify/fsnotify`, `atotto/clipboard`, and `pkg/browser` as new dependencies (Tech Stack table and T16), but none are in the current `go.mod`.** T16 correctly identifies this as a task. Good.

9. **The `monitor` command is listed under "Existing Commands (Preserved)" (`cmdr monitor`), but the `MonitorCmd` function is referenced in `cmd/cc/main.go` yet there is no `monitor.go` file visible in `internal/commands/`.** This may be defined in `coordinator.go` or elsewhere. Agents should verify before assuming they need to create it.

10. **The `merge_queue` reference in the target layout KDL uses `merge_queue` as a pane name, while the existing TUI uses `MergeQueueView` (Go struct) and `merge_view.go` (filename).** The naming is inconsistent across the KDL layout, TUI code, and spec's directory listing. While not blocking, consistent naming would reduce confusion during implementation.
