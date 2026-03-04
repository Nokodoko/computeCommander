# ComputeCommander (cmdr) CLI Redesign

Go 1.25 CLI tool redesign: transforms `cmdr` from a subcommand-heavy orchestrator into an agentic IDE with a leader-key-driven zellij UI, unified lifecycle management (init/open/stop/reset), and an embedded agent session pane replacing the agent picker.

Replaces the current `cc` CLI command structure (30+ subcommands across 7 groups) with a streamlined 20-command surface where `cmdr` (no subcommand) opens the full interface, and all secondary operations are available both as CLI subcommands and as leader-key-triggered zellij floating panes.

## Why

The current `cc` CLI works but carries friction that daily use exposed:

- **Dashboard is a separate command.** `cc dashboard` must be explicitly invoked after init; the natural workflow is `init` then immediately work. The init-to-dashboard gap requires two commands where one should suffice.
- **Agent picker pane wastes prime real estate.** The 54%-width agent picker shows a list of agents, but the user actually needs to *interact* with the orchestrator agent directly. Visibility into swarm activity should share space with the bottom pane, not dominate the layout.
- **No lifecycle commands.** There is no `stop`, `reset`, `restart`, or `backup` — the user must manually kill processes and wipe databases. This is error-prone and undiscoverable.
- **No keybind system.** Every operation requires switching to a terminal and typing a full CLI command. Zellij floating panes with a leader key would make the tool feel like an IDE, not a collection of scripts.
- **No in-UI help, version, or update checks.** Users must leave the interface to check versions, read docs, or find available commands. These should be floating-pane overlays triggered by single keystrokes.
- **No export or backup.** Data is locked in SQLite with no CLI path to export, backup, or restore.

The redesign touches ~20 commands on the existing data model and UI layer. This spec covers exactly that surface plus the keybind system and layout restructure.

## Design Principles

1. **`cmdr` with no args opens the interface.** The bare command starts the DB (if not running), launches the zellij dashboard layout, and drops the user into the agent session pane. No subcommand required for the primary workflow.
2. **Leader key for everything.** `Ctrl+Space` is the leader key. Every in-UI action (help, shell, export, version, update, logs clear, backup, restore, theme, feedback, accessibility) is triggered by `Ctrl+Space` followed by a single character. No mode switching, no nested menus.
3. **Confirmation gates for destructive ops.** `stop`, `reset`, `restart`, `backup`, and `restore` require explicit confirmation via a zellij floating pane prompt. Accidental invocation cannot destroy state.
4. **Agent session replaces agent picker.** The main pane (formerly 54%-width agent picker) becomes an embedded orchestrator agent session — turning cmdr into an agentic IDE where the user interacts with the swarm directly while observing events, mail, and merge queue activity in the bottom pane.
5. **CLI and keybind parity.** Every keybind-triggered action has a corresponding `cmdr <subcommand>` for scripting and automation. The CLI is the source of truth; keybinds are shortcuts into it.
6. **Floating panes for transient UI.** Help, version, update, export, feedback, support, and theme selection all render in zellij floating panes that overlay the main interface and dismiss on `Esc` or `q`.
7. **Config hot-reload.** Changes made via `cmdr config edit` (which opens `$EDITOR`) are detected and applied without restarting the DB or UI. A file watcher on `.computecommander/config.yaml` triggers reload.
8. **Plugin-ready architecture.** The command registry and keybind map are data structures (not hardcoded switch statements) so that a future plugin system can register new commands and keybinds at runtime.
9. **Existing infrastructure preserved.** The App struct, DB layer, spawner, mail, merge queue, watchdog, gateway, worktree manager, and all runtime integrations remain untouched. This redesign is a CLI + UI layer change only.
10. **`--agent` and `--sub-agent` flags.** Commands that target agent sessions accept `--agent <name>` to specify which agent to interact with, and `--sub-agent <name>` as a workaround for Claude Code's nested agent focus limitation.
11. **File picker pane for directory navigation and session proliferation.** The UI includes an `fp` (file picker) pane that lets the user navigate to any directory, start a new agent session scoped to that directory, navigate to another directory and start another session there, and freely switch between active sessions. This turns cmdr into a multi-project agentic IDE where the user can work across multiple codebases simultaneously.

## On-Disk Format

```
.computecommander/
  config.yaml              # Project config (YAML, hot-reloaded)
  config.local.yaml        # Local overrides (gitignored)
  local.db                 # SQLite database
  backups/                 # Database backup files
    backup-20260228T140000.db
  layouts/
    cmdr-dashboard.kdl     # Zellij KDL layout (redesigned)
  keybinds.yaml            # Leader key mappings (overridable)
  plugins/                 # Future: plugin directory
  agents/                  # Agent role definitions
  hooks/                   # Guard rules
  specs/                   # Task specifications
  worktrees/               # Agent worktrees
  logs/                    # Application logs
  themes/                  # UI theme files
    default.yaml
```

### cmdr-dashboard.kdl (Redesigned Layout)

The KDL layout defines a 3-column split: FP left sidebar (15%), center section with agent session + bottom bar (77%), and agents right sidebar (8%). The bottom bar sits between the two sidebars and has 4 sections: Event Log, Mail, Merge Queue, Git Status.

```
┌─────────┬──────────────────────────────────────────────────────┬──────┐
│<Project>│                                                      │      │
│         │              Agent Session                            │Agents│
│   FP    │              (i.e. Claude Code)                       │      │
│         │                                                      │      │
│         │                                                      │      │
│         │                                                      │      │
│         │                                                      │      │
│         │                                                      │      │
│         │                                                      │      │
│         ├────────────┬──────────┬──────────────┬───────────────┤      │
│         │  Event     │  Mail    │   Merge      │  Git          │      │
│         │  Log       │          │   Queue      │ Status        │      │
└─────────┴────────────┴──────────┴──────────────┴───────────────┴──────┘
```

> The FP pane's zellij frame title is dynamic — it displays the active session's `displayName` (e.g., `<Project>` = "computeCommander"). See [Session Change Protocol](#session-change-protocol) for update mechanics.

```kdl
layout {
    pane size=1 borderless=true {
        plugin location="compact-bar"
    }
    pane split_direction="vertical" {
        pane name="fp" size="15%" {
            // File picker — directory navigation + session launcher
            // User browses directories, selects one to start/switch agent sessions
            // Frame title is dynamically set to the active session's displayName
            // e.g., "computeCommander" — updated via `zellij action rename-pane`
            // on every session switch (see SessionFocusManager)
        }
        pane size="77%" {
            pane name="agent_session" size="80%" focus=true {
                // Embedded orchestrator agent session
                // User interacts with the swarm here
                // Switches to different sessions via fp pane selection
            }
            pane split_direction="vertical" size="20%" {
                pane name="event_log" size="25%" {
                    // Event log — system events and logs
                }
                pane name="mail" size="25%" {
                    // Inter-agent mail / notifications
                }
                pane name="merge_queue" size="25%" {
                    // Merge queue status / pending PRs
                }
                pane name="git_status" size="25%" {
                    command "lazygit"
                    args "-p" "{session_dir}"
                    // lazygit launched in active session's project directory
                    // Restarted with new -p path on session switch
                }
            }
        }
        pane name="agents" size="8%" {
            // Agent roster — right sidebar (cross-project)
            // Aggregates agents from ALL active sessions, color-coded by project
            // See "Agents Pane (Right Sidebar)" under Integration
        }
    }
}
```

### keybinds.yaml

User-overridable keybind mappings. The leader key itself is hardcoded to `Ctrl+Space`; the follow-up keys are configurable.

```yaml
version: 1
leader: "ctrl+space"
bindings:
  "?": help
  "u": update
  "v": version
  "s": shell
  "c": clear
  "e": export
  "r": restart
  "b": backup
  "R": restore
  "f": feedback
  "h": support
  "p": plugins
  "t": theme
  "n": notifications
  "a": analytics
  "i": integrations
  "m": automation
  "A": accessibility
  "d": fp           # Toggle file picker pane / focus directory navigator
  "q": quit        # Kill entire UI process (confirmation-gated)
```

### backup files

Database backups are SQLite `.backup` command outputs, named with ISO 8601 timestamps.

```
backup-20260228T140000.db
backup-20260301T093000.db
```

### themes/default.yaml

```yaml
name: default
colors:
  primary: "#7C3AED"
  secondary: "#10B981"
  error: "#EF4444"
  warning: "#F59E0B"
  info: "#3B82F6"
  muted: "#6B7280"
  background: "#1F2937"
  foreground: "#F9FAFB"
font:
  size: 14
  family: "monospace"
contrast: "normal"  # normal | high
```

## Data Model

### ProcessState

```typescript
interface ProcessState {
  // Database
  dbRunning: boolean;           // true if SQLite file is accessible and not locked
  dbPid?: number;               // PID of process holding DB (if applicable)
  dbPath: string;               // absolute path to .computecommander/local.db

  // UI
  uiRunning: boolean;           // true if zellij session exists
  uiSessionName: string;        // e.g. "cc-dashboard"
  uiPid?: number;               // PID of the zellij session leader

  // Metadata
  version: string;              // current binary version
  uptime?: number;              // seconds since UI launch
  projectName: string;          // from config.yaml
}
```

### KeybindConfig

```typescript
interface KeybindConfig {
  version: number;              // schema version, currently 1
  leader: string;               // "ctrl+space"
  bindings: Record<string, string>;  // key -> action name
}
```

### BackupRecord

```typescript
interface BackupRecord {
  // Identity
  id: string;                   // "bak-{8hex}", e.g. "bak-a1b2c3d4"
  path: string;                 // absolute path to backup file

  // Metadata
  sizeBytes: number;            // file size
  tableCount: number;           // number of tables backed up
  rowCount: number;             // total rows across all tables
  createdAt: string;            // ISO 8601
  restoredAt?: string;          // ISO 8601, set if this backup was restored
}
```

### ExportData

```typescript
interface ExportData {
  // Metadata
  exportedAt: string;           // ISO 8601
  version: string;              // cmdr version that produced this export
  projectName: string;          // from config.yaml

  // Data
  sessions: AgentSession[];     // all sessions
  events: Event[];              // all events
  mail: MailMessage[];          // all mail
  mergeQueue: MergeEntry[];     // all merge queue entries
  metrics: MetricsRow[];        // all metrics
  runs: Run[];                  // all runs
}
```

### ThemeConfig

```typescript
interface ThemeConfig {
  name: string;                 // theme identifier
  colors: {
    primary: string;            // hex color
    secondary: string;
    error: string;
    warning: string;
    info: string;
    muted: string;
    background: string;
    foreground: string;
  };
  font: {
    size: number;               // px
    family: string;             // CSS font-family
  };
  contrast: "normal" | "high";
}
```

### DirectorySession

```typescript
interface DirectorySession {
  // Identity
  id: string;                     // "dsess-{8hex}", e.g. "dsess-a1b2c3d4"
  directory: string;              // Absolute path to project directory
  displayName: string;            // Directory basename or project name

  // Session state
  agentSessionId?: string;        // ID of the agent session running in this directory
  runtime: string;                // Runtime used (claude, gemini, etc.)
  active: boolean;                // Whether this session is currently focused in the agent_session pane

  // Navigation
  parentDirectory?: string;       // For breadcrumb-style navigation
  lastAccessedAt: string;         // ISO 8601, for MRU ordering in fp pane

  // Lifecycle
  createdAt: string;              // ISO 8601
  stoppedAt?: string;             // ISO 8601, set when session is stopped
}
```

### SessionFocusManager

```typescript
interface SessionFocusManager {
  // State
  activeSessionId: string;            // ID of the currently focused DirectorySession
  sessions: DirectorySession[];       // All registered sessions (active and stopped)
  listeners: SessionChangeListener[]; // Panes that react to session changes
  paneIds: PaneIdMap;                 // Zellij pane IDs for coupled panes (captured from $ZELLIJ_PANE_ID at launch)

  // Operations
  switchTo(sessionId: string): void;  // Set active session, notify all listeners, write active-session file
  onSessionChange(listener: SessionChangeListener): void;  // Register a listener
  getActiveSession(): DirectorySession;
  getMRUOrder(): DirectorySession[];  // Most-recently-used ordering for session list
}

// Zellij pane ID mapping for frame title updates and cross-process coordination
interface PaneIdMap {
  fp: string;              // FP pane's $ZELLIJ_PANE_ID
  agentWorkspace: string;  // Agent Workspace pane's $ZELLIJ_PANE_ID
  gitStatus: string;       // Git Status pane's $ZELLIJ_PANE_ID
}

interface SessionChangeListener {
  // Called when the active session changes
  onSessionFocusChanged(newSession: DirectorySession, previousSession: DirectorySession): void;
}
```

The `SessionFocusManager` is the central coordinator for session-scoped panes. When the active session changes (via FP selection or `cmdr session switch`), it broadcasts to all registered listeners so that the FP pane, Agent Workspace pane, and Git Status pane update simultaneously. For in-process panes (FP, Agent Workspace), the broadcast uses Go interface callbacks. For the Git Status pane (lazygit), the `SessionFocusManager` kills the running lazygit process and restarts it with `lazygit -p <new_directory>` to scope it to the new session's project directory. The `active-session.path` file is still written for any future cross-process consumers. Each pane's zellij pane ID is captured at launch from the `$ZELLIJ_PANE_ID` environment variable and stored in `paneIds` so that frame title updates can target the correct pane via `zellij action rename-pane --pane-id <id> "<title>"`.

### PTYManager

```typescript
interface PTYManager {
  // State
  ptys: Map<string, PTYProcess>;      // sessionId -> PTYProcess
  maxConcurrent: number;              // From config.yaml (default: 8)

  // Operations
  spawn(session: DirectorySession): PTYProcess;   // Create PTY for session
  attach(sessionId: string): PTYProcess;           // Get PTY for rendering
  detach(sessionId: string): void;                 // Stop rendering (PTY keeps running)
  kill(sessionId: string): void;                   // Terminate PTY process
  getScrollback(sessionId: string): string[];      // Recent output lines
}

interface PTYProcess {
  // Identity
  sessionId: string;                  // Owning DirectorySession ID
  pid: number;                        // OS process ID
  runtime: string;                    // Runtime command (e.g., "claude")

  // State
  running: boolean;                   // Whether the process is alive
  attached: boolean;                  // Whether output is being rendered to a pane
  scrollbackBuffer: string[];         // Circular buffer of recent output (configurable size)
  scrollbackSize: number;             // Max lines in scrollback (default: 10000)

  // Lifecycle
  createdAt: string;                  // ISO 8601
  lastOutputAt: string;               // ISO 8601, updated on each PTY write
}
```

### PluginManifest (Future)

```typescript
interface PluginManifest {
  name: string;                 // plugin identifier
  version: string;              // semver
  description: string;
  author: string;
  commands: PluginCommand[];    // commands this plugin registers
  keybinds: Record<string, string>;  // key -> command mappings
}

interface PluginCommand {
  name: string;                 // e.g. "my-plugin:analyze"
  description: string;
  handler: string;              // path to handler binary/script
}
```

### ID Generation

- Backups: `bak-{8 random hex}` (e.g., `bak-e7f3a1b2`)
- Checked against existing backup filenames to avoid collision
- Monotonically timestamped filenames ensure chronological ordering

### UI Process Lifecycle

```
stopped ──> starting ──> running ──> stopping ──> stopped
               │                        ^
               └── error ──> stopped    │
                                        │
              running ──> restarting ────┘
```

### Confirmation Gate Flow

```
user_triggers ──> floating_pane("Are you sure?") ──> [y] ──> execute
                                                  ──> [n/Esc] ──> cancel
```

## CLI

Binary name: `cmdr` (replaces `cc` in command usage; binary output unchanged from Makefile).

Every command supports `--json` for structured output. Non-JSON output is human-readable with ANSI colors (respects `NO_COLOR`). Global flags: `--quiet`, `--json`, `--timing`, `--agent <name>`, `--sub-agent <name>`.

### Lifecycle Commands

```
cmdr                                   Open the cmdr interface (start DB if needed)
  --tui                                Force in-process TUI (skip wezterm)

cmdr init                              Initialize project + open cmdr after DB starts
  --name <text>                        Project name (default: directory name)
  --db <driver>                        Database backend: postgres|sqlite (default: sqlite)
  --yes                                Skip interactive prompts

cmdr stop                              Stop DB + close UI (confirmation-gated)
  --force                              Skip confirmation prompt

cmdr reset                             Reset DB to empty + close UI (confirmation-gated)
  --force                              Skip confirmation prompt

cmdr restart                           Restart DB + UI (confirmation-gated)
  --force                              Skip confirmation prompt
```

### Information Commands

```
cmdr help                              Show help in zellij floating pane (or stdout if no UI)
                                       Includes link to documentation

cmdr docs                              Open documentation in default browser

cmdr status                            Show DB status + UI status
  --json                               JSON output

cmdr version                           Show version + link to release notes
  --json                               JSON output

cmdr update                            Check for updates, show current version + release notes link
  --json                               JSON output
```

### Observability Commands

```
cmdr logs                              Show DB logs + UI logs (if running)
  --follow                             Stream logs in real-time (NEW — does not exist in current LogsCmd)
  --lines <n>                          Number of lines (default: 50) (NEW — does not exist in current LogsCmd)
  --agent <name>                       Filter by agent name

cmdr clear                             Clear DB logs + UI logs (if running) (NEW command — distinct from existing `clean.go` which handles resource cleanup like stale worktrees/temp files)
  --force                              Skip confirmation prompt
```

### Configuration Commands

```
cmdr config                            Show current configuration
  show                                 Display full config
  validate                             Validate config
  get <key>                            Get specific value
  set <key> <value>                    Set specific value
  edit                                 Open config in $EDITOR (hot-reload on save)
```

### Data Commands

```
cmdr export                            Export all data from DB as JSON
  --format <fmt>                       json|csv (default: json)
  --output <path>                      Output file (default: stdout)
  --tables <list>                      Comma-separated table list (default: all)

cmdr backup                            Backup DB file (confirmation-gated)
  --output <path>                      Backup destination (default: .computecommander/backups/)

cmdr restore <path>                    Restore DB from backup (confirmation-gated)
  --force                              Skip confirmation prompt
```

### Utility Commands

```
cmdr shell                             Open shell in cmdr interface pane
  --agent <name>                       Open shell in agent's worktree
  --sub-agent <name>                   Nested agent focus (Claude Code limitation workaround)

cmdr feedback                          Open feedback form in default browser

cmdr support                           Open support page in default browser
```

### Settings Commands

```
cmdr theme                             List/set UI theme
  list                                 Show available themes
  set <name>                           Apply theme
  edit                                 Open theme file in $EDITOR

cmdr notifications                     Notification settings
  show                                 Show current settings
  set <key> <value>                    Update setting

cmdr analytics                         Usage analytics dashboard
  --json                               JSON output

cmdr integrations                      Third-party service connections
  list                                 List configured integrations
  add <service>                        Add integration
  remove <service>                     Remove integration

cmdr automation                        Workflow automation builder
  list                                 List automations
  create                               Create new automation
  run <name>                           Execute automation
  delete <name>                        Delete automation
```

### Directory Navigation Commands

```
cmdr fp                                Open/toggle file picker pane
  --path <dir>                         Start browsing from directory (default: cwd)

cmdr session list                      List all active directory sessions
  --json                               JSON output

cmdr session switch <id|path>          Switch agent_session pane to a different directory session
  --create                             Create a new session if none exists for this directory

cmdr session stop <id|path>            Stop a directory session (does not close cmdr)

cmdr git-status                        Show git status for the active session's directory
  --json                               JSON output
                                       Note: the dashboard git_status pane uses lazygit directly
                                       (not this command). This CLI command is for scripting only.
```

### Existing Commands (Preserved)

All existing commands from the current `cc` CLI remain available. They are regrouped under the new command structure but their behavior is unchanged:

```
cmdr sling <name>                      Spawn worker agent
cmdr status                            Fleet status overview (enhanced: now includes UI status)
cmdr coordinator                       Start coordinator agent
cmdr monitor                           Start monitor agent
cmdr mail send|check|list|read|reply|purge   Inter-agent messaging
cmdr nudge <agent>                     Send nudge to agent pane
cmdr merge enqueue|list|status|run     Merge queue operations
cmdr group create|list|status          Task groups
cmdr inspect <session>                 Deep session inspection
cmdr trace|errors|replay|feed|costs|metrics|run   Observability
cmdr worktree list|status|clean|remove Worktree management
cmdr watch                             Start watchdog
cmdr doctor                            Health checks
cmdr clean                             Resource cleanup
cmdr feature list|toggle               Feature flags
```

## JSON Output Format

Success (lifecycle):

```json
{
  "success": true,
  "command": "start",
  "dbRunning": true,
  "uiRunning": true,
  "uiSession": "cc-dashboard",
  "version": "0.1.0"
}
```

Error:

```json
{
  "success": false,
  "command": "stop",
  "error": "DB is not running"
}
```

Status:

```json
{
  "success": true,
  "command": "status",
  "db": {
    "running": true,
    "driver": "sqlite",
    "path": ".computecommander/local.db",
    "tables": 10,
    "totalRows": 1247
  },
  "ui": {
    "running": true,
    "session": "cc-dashboard",
    "uptime": 3600
  },
  "version": "0.1.0",
  "project": "computeCommander"
}
```

Export:

```json
{
  "success": true,
  "command": "export",
  "exportedAt": "2026-02-28T14:00:00Z",
  "tables": ["sessions", "events", "mail", "merge_queue", "metrics", "runs"],
  "totalRows": 1247,
  "sizeBytes": 524288
}
```

Backup:

```json
{
  "success": true,
  "command": "backup",
  "id": "bak-e7f3a1b2",
  "path": ".computecommander/backups/backup-20260228T140000.db",
  "sizeBytes": 1048576
}
```

Version:

```json
{
  "success": true,
  "command": "version",
  "version": "0.1.0",
  "commit": "9d0aadd",
  "releaseNotesUrl": "https://github.com/noko/computecommander/releases/tag/v0.1.0"
}
```

## Concurrency Model

The existing concurrency model (SQLite WAL mode, Go mutex on App struct) is preserved. New concerns:

### Process Lifecycle Locking

```
Lock file:    .computecommander/cmdr.lock
Stale after:  60 seconds
Retry:        100ms polling
Timeout:      10 seconds
```

Implementation:

1. On `cmdr` (open UI): create `.computecommander/cmdr.lock` with PID + timestamp
2. If lock exists and PID is alive: print "cmdr is already running (PID: XXXX)" and exit
3. If lock exists and PID is dead: remove stale lock, proceed
4. On `cmdr stop`: remove lock file after UI and DB shutdown
5. On crash: next `cmdr` invocation detects stale lock via PID check

### Config Hot-Reload

1. `fsnotify` watcher on `.computecommander/config.yaml`
2. On file change: re-parse YAML, validate, apply diff to running `Config` struct
3. Signal dashboard TUI to refresh theme/settings via `tea.Msg`
4. If validation fails: log error, keep current config, show error in status bar

### Cross-Process Session Notification

Panes running as separate OS processes (e.g., lazygit in the git_status pane) cannot receive Go interface callbacks from the `SessionFocusManager`. Cross-process IPC uses a file-based mechanism combined with process restart:

1. **Write**: On session change, `SessionFocusManager` atomically writes the active session's directory path to `.computecommander/active-session.path` (write to temp file, then `os.Rename`)
2. **Watch**: External pane processes watch this file via `fsnotify`
3. **Read**: On `fsnotify.Write` event, the external process reads the new directory path and updates its state
4. **Startup**: On initial launch, the external process reads the current value of `active-session.path` (if it exists) rather than waiting for a change event
5. **Cleanup**: `cmdr stop` removes `active-session.path` alongside `cmdr.lock`

This pattern is consistent with the existing `SIGUSR1`-based file picker refresh (commit `6e8cbe3`) but uses `fsnotify` for more reliable cross-process notification without requiring PID tracking. For the git_status pane (lazygit), the `SessionFocusManager` uses direct process restart (kill + relaunch with new `-p` path) rather than file watching, since lazygit does not natively watch `active-session.path`.

### Atomic Backup/Restore

1. Backup: `sqlite3 .backup` command writes to temp file, rename to final path
2. Restore: copy backup to temp path, validate schema, rename over `local.db`
3. After restore: restart DB connection (close + reopen) and signal UI refresh

## Migration

| Component | Current (`cc` CLI) | Target (`cmdr` redesign) |
|-----------|-------------------|--------------------------|
| Entry point | `cc <subcommand>` required | `cmdr` (bare) opens interface |
| Init flow | `cc init` creates dirs only | `cmdr init` creates dirs + opens interface |
| Dashboard | `cc dashboard` (separate command) | `cmdr` (default action) |
| Stop | No stop command exists | `cmdr stop` (confirmation-gated) |
| Reset | No reset command exists | `cmdr reset` (confirmation-gated) |
| Restart | No restart command exists | `cmdr restart` (confirmation-gated) |
| Agent picker pane | 54% width, top-left | Replaced by agent session pane (80% height) |
| Events/Mail/Merge/Git | Separate panes, mixed layout | 4-section bottom bar (Event Log, Mail, Merge Queue, lazygit) between sidebars |
| Keybinds | Single-key in TUI (`s`, `m`, `c`) | Leader key `Ctrl+Space` + action key |
| Help | `--help` flag only | `cmdr help` + floating pane + `?` keybind |
| Config edit | `cc config edit` | `cmdr config edit` + hot-reload |
| Export | Not available | `cmdr export` + `e` keybind |
| Backup | Not available | `cmdr backup` + `b` keybind |
| Binary name | `cc` (Use field) | `cmdr` (Use field) |
| `--agent` flag | Not available | Global flag on all commands |
| `--sub-agent` flag | Not available | Global flag for nested agent focus |
| FP pane frame title | Static label ("fp") | Dynamic — shows active session's `displayName` via `zellij action rename-pane` |

### Migration Steps

No data migration is required — the SQLite schema is unchanged. The migration is purely CLI + UI:

1. Update `cmd/cc/main.go`: change root command `Use` from `"cc"` to `"cmdr"`, add default `RunE` that opens interface
2. Add new command files in `internal/commands/` for each new subcommand
3. Replace `cmdr-dashboard.kdl` layout file with redesigned 2-region layout
4. Add `keybinds.yaml` generation to `cmdr init`
5. Update `internal/tui/dashboard.go` to implement new pane structure
6. Add leader key handler to TUI event loop
7. Preserve all existing commands — they continue to work as before

## Integration

### Agent Session Pane

The top 80% of the dashboard is now an embedded agent session. When `cmdr` launches:

1. The zellij layout creates the `agent_session` pane
2. cmdr spawns the default orchestrator runtime (from `defaults.runtime` config) in that pane
3. The user interacts with the orchestrator directly — typing prompts, receiving responses
4. All spawned sub-agents appear in the event log pane (bottom) as they are created

### File Picker (fp) Pane

The left 15% of the dashboard is a file picker / directory navigator. Behavior:

1. On launch, the fp pane shows the project root directory tree (cwd where `cmdr` was invoked)
2. User navigates directories using arrow keys / j/k / Enter
3. Selecting a directory and pressing Enter starts a new agent session scoped to that directory (or switches to an existing one if a session already exists for that path)
4. Active sessions are highlighted in the fp pane with a marker (e.g., `*` prefix or colored background)
5. The agent_session pane swaps to show the selected session's agent interaction
6. Multiple sessions can be active simultaneously — the fp pane acts as a session switcher
7. Sessions persist until explicitly stopped (`cmdr session stop`) or until `cmdr stop` terminates everything
8. The fp pane maintains a history of recently accessed directories (MRU ordering) for quick switching
9. Navigation supports going up (`..`) to parent directories and breadcrumb-style path display at the top of the pane
10. The zellij frame title for the FP pane dynamically displays the active session's `displayName` from `DirectorySession`. When the user switches sessions, the frame title is updated via `zellij action rename-pane --pane-id <id> "<displayName>"` to reflect the newly active project name (e.g., "computeCommander"). The pane ID is captured at FP pane launch from the `$ZELLIJ_PANE_ID` environment variable (zellij injects this into every pane's environment) and stored in `SessionFocusManager.paneIds.fp`. This gives the user an at-a-glance indicator of which project is currently focused.

### Session Scoping and Cross-Pane Coupling

Panes are divided into two scoping categories: **session-scoped** (reflecting the active project session) and **cross-project** (aggregating data from all running sessions).

#### Cross-Project vs Session-Scoped Pane Behavior

| Pane | Scope | Behavior |
|------|-------|----------|
| Agents (right sidebar) | Cross-project | Shows agents from ALL active sessions, color-coded by project |
| Event Log (bottom bar) | Cross-project | Events from all projects, tagged with project name |
| Mail (bottom bar) | Cross-project | Messages from all projects, tagged with project name |
| Merge Queue (bottom bar) | Cross-project | Merge operations from all projects |
| FP (left sidebar) | Session-scoped | Shows directory tree rooted at the active session's directory |
| Agent Workspace (center) | Session-scoped | Displays the active session's agent PTY |
| Git Status (bottom bar) | Session-scoped | lazygit scoped to the active session's directory (restarted on session switch) |

#### Active Session Concept

Exactly one `DirectorySession` has `active: true` at any time. This is the "active session." All session-scoped panes reflect this session.

#### Session Change Protocol

When the active session changes (via FP pane selection or `cmdr session switch`):

1. The `SessionFocusManager` sets the new session as `active: true` and the previous session as `active: false`
2. The `SessionFocusManager` broadcasts the change via two mechanisms:
   - **(a) In-process broadcast**: Sends a `SessionFocusChanged` event to all registered `SessionChangeListener` instances (Go interface callbacks for FP and Agent Workspace panes)
   - **(b) Cross-process broadcast**: Writes the new session's `directory` path to `.computecommander/active-session.path` (atomic write via temp file + rename). This file is the IPC channel for panes running as separate OS processes. For the git_status pane, the `SessionFocusManager` kills the current lazygit process and restarts it with `-p <new_session_directory>`
3. Each session-scoped pane reacts:
   - **FP pane** (in-process): Updates its root directory to the new session's `directory` path and renames its zellij frame title to the new session's `displayName` via `zellij action rename-pane --pane-id <id>` (pane ID from `SessionFocusManager.paneIds.fp`, originally captured from `$ZELLIJ_PANE_ID` at pane launch)
   - **Agent Workspace pane** (in-process): Re-attaches to the new session's PTY (via `PTYManager.attach()`)
   - **Git Status pane** (managed process): The `SessionFocusManager` kills the running lazygit process in the git_status pane and restarts it with `lazygit -p <new_directory>` scoped to the new session's project directory
4. Cross-project panes (Agents, Event Log, Mail, Merge Queue) are **not affected** by session changes — they always show aggregated data

#### Session Fallback Behavior

When the active session is stopped (`cmdr session stop`):
- The next most-recently-used (MRU) session becomes active
- If no other sessions exist, the FP pane returns to the project root and the Agent Workspace shows an empty state with instructions to start a new session
- The `SessionFocusManager.getMRUOrder()` determines fallback priority based on `lastAccessedAt` timestamps

### Agents Pane (Right Sidebar)

The right 8% of the dashboard is the agent roster. This pane operates in **cross-project** mode:

1. Aggregates agents from ALL running `DirectorySession` instances, not just the active session
2. Each agent row displays: agent name, status (running/idle/stopped), runtime duration, and parent project name
3. Agents are color-coded by their parent project session (each `DirectorySession` is assigned a unique color from the theme palette)
4. Selecting an agent in this pane shows agent details (expanded row or floating pane) but does **NOT** switch the session focus — the FP, Agent Workspace, and Git Status panes remain on the current active session
5. To switch to an agent's parent project, the user must use the FP pane or `cmdr session switch`
6. The pane auto-refreshes as agents are spawned or terminated across any session

### PTY Swapping Mechanics

Each `DirectorySession` owns a PTY process managed by the `PTYManager`. The PTY lifecycle:

1. **Spawn**: When a session is created (via FP selection or `cmdr session switch --create`), the `PTYManager` spawns a new PTY running the configured runtime (e.g., `claude --dangerously-skip-permissions`). The PTY runs in the session's `directory`.
2. **Background execution**: When the user switches away from a session, its PTY continues running in the background. It is **not** suspended or killed. Output continues to be captured into the scrollback buffer.
3. **Re-attachment**: When the user switches back to a session, the Agent Workspace pane re-attaches to that session's PTY via `PTYManager.attach()`. The pane renders the scrollback buffer first (so the user sees recent activity), then streams live output.
4. **Scrollback**: Each PTY maintains a circular scrollback buffer (default: 10,000 lines, configurable via `config.yaml` key `pty.scrollback_lines`). This ensures the user sees recent activity when switching back to a session.
5. **Kill**: When a session is stopped (`cmdr session stop`), the PTY process is killed via `SIGTERM` (with `SIGKILL` fallback after 5 seconds). The scrollback buffer is discarded.
6. **Max concurrent**: The maximum number of concurrent PTY processes is configurable (default: 8) via `config.yaml` key `pty.max_concurrent`. When the limit is reached, starting a new session returns an error: "Maximum concurrent sessions (8) reached. Stop an existing session first."
7. **Crash recovery**: If a PTY process dies unexpectedly (non-zero exit), the session remains in the session list with a "crashed" status indicator. The user can restart it by selecting the session in the FP pane and pressing Enter.

### Zellij Floating Panes

Transient information (help, version, update, export preview, confirmation prompts) renders in zellij floating panes:

| Trigger | Floating Pane Content |
|---------|----------------------|
| `Ctrl+Space` + `?` | Help text + documentation link |
| `Ctrl+Space` + `v` | Version + release notes link |
| `Ctrl+Space` + `u` | Update check result + release notes link |
| `Ctrl+Space` + `e` | Export preview + copy-to-clipboard button |
| `Ctrl+Space` + `s` | Shell (in project root or agent worktree) |
| `Ctrl+Space` + `c` | Confirmation: "Clear all logs?" |
| `Ctrl+Space` + `r` | Confirmation: "Restart DB + UI?" |
| `Ctrl+Space` + `b` | Confirmation: "Backup database?" |
| `Ctrl+Space` + `R` | Confirmation: "Restore from backup?" |
| `Ctrl+Space` + `f` | Opens feedback URL in browser |
| `Ctrl+Space` + `h` | Opens support URL in browser |
| `Ctrl+Space` + `t` | Theme picker |
| `Ctrl+Space` + `n` | Notification settings |
| `Ctrl+Space` + `a` | Analytics dashboard |
| `Ctrl+Space` + `i` | Integrations list |
| `Ctrl+Space` + `m` | Automation builder |
| `Ctrl+Space` + `A` | Accessibility settings (font size, contrast, keyboard nav) |
| `Ctrl+Space` + `p` | Plugin manager |
| `Ctrl+Space` + `d` | Toggle/focus file picker (fp) pane |
| `Ctrl+Space` + `q` | Confirmation: "Kill entire UI process?" |

### CLI-to-Keybind Mapping

```bash
# Lifecycle
cmdr                           # Opens interface (equivalent to launching zellij session)
cmdr stop                      # Ctrl+Space -> q (with confirmation)
cmdr restart                   # Ctrl+Space -> r (with confirmation)

# Information (floating panes in UI)
cmdr help                      # Ctrl+Space -> ?
cmdr version                   # Ctrl+Space -> v
cmdr update                    # Ctrl+Space -> u

# Operations (floating panes in UI)
cmdr shell                     # Ctrl+Space -> s
cmdr clear                     # Ctrl+Space -> c
cmdr export                    # Ctrl+Space -> e
cmdr backup                    # Ctrl+Space -> b
cmdr restore                   # Ctrl+Space -> R (capital)
cmdr fp                        # Ctrl+Space -> d (toggle file picker pane)
cmdr session list              # (no keybind -- CLI only)
cmdr session switch <path>     # (triggered via fp pane Enter key)
lazygit -p <dir>               # (runs in bottom bar Git Status pane, no keybind — managed by SessionFocusManager)
cmdr feedback                  # Ctrl+Space -> f
cmdr support                   # Ctrl+Space -> h
cmdr theme                     # Ctrl+Space -> t
cmdr notifications             # Ctrl+Space -> n
cmdr analytics                 # Ctrl+Space -> a
cmdr integrations              # Ctrl+Space -> i
cmdr automation                # Ctrl+Space -> m
```

### Agent-Facing Commands

```bash
# Spawning agents (existing, unchanged)
cmdr sling my-builder --task TASK-001 --capability builder --runtime claude

# Agent-scoped shell access (new)
cmdr shell --agent my-builder           # Opens shell in agent's worktree
cmdr shell --sub-agent nested-builder   # Focus nested agent (Claude Code workaround)

# Status with agent filter (enhanced)
cmdr status --agent my-builder --json
```

### Hooks Integration

> **Note:** This section documents existing agentic infrastructure behavior for context. No changes are required by this spec. Authoritative source: AGENTIC-SPEC.md.

```json
{
  "hooks": {
    "PostToolUse(Write)": [
      {
        "command": "sync-agents-dict.sh",
        "description": "Sync agent dictionary after file writes"
      }
    ],
    "SubagentStart": [
      {
        "command": "agent-counter.sh start",
        "description": "Track agent spawn count"
      }
    ],
    "SubagentStop": [
      {
        "command": "agent-counter.sh stop",
        "description": "Track agent termination"
      }
    ]
  }
}
```

## What It Does NOT Do

Explicitly out of scope (keep it minimal):

- **No new database schema.** The SQLite schema (runs, sessions, events, mail, metrics, merge_queue, task_groups, task_group_members, checkpoints, worktrees) is unchanged. Backup metadata is filesystem-based (directory listing), not a new table.
- **No new runtimes.** The pluggable runtime system (claude, gemini, codex, pi, goose) is unchanged. The agent session pane uses the existing runtime spawning infrastructure.
- **No remote/distributed mode.** The `features.distributed` and `features.remote_agents` flags remain false. This redesign is local-only.
- **No custom merge drivers.** Merge queue operations are unchanged.
- **No new agent capabilities.** The 8 capabilities (scout, builder, reviewer, lead, merger, coordinator, supervisor, monitor) are unchanged. (Reference only -- see AGENTIC-SPEC.md for authoritative agent capability definitions.)
- **No plugin implementation.** The plugin system is a *consideration* — the architecture is plugin-ready (data-driven command/keybind registry) but no plugin loading, sandboxing, or marketplace is implemented in this phase.
- **No automation engine.** `cmdr automation` is a consideration placeholder — the command exists but the workflow builder is a future phase.
- **No analytics backend.** `cmdr analytics` shows local metrics from the existing `metrics` table. No telemetry, no external analytics service.
- **No notification system.** `cmdr notifications` is a settings placeholder. No push notifications, no webhook integrations in this phase.
- **No integration connectors.** `cmdr integrations` is a placeholder. No Slack, GitHub, Jira connectors in this phase.

## Tech Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| Runtime | Go 1.25 | Existing codebase, single binary distribution |
| CLI Framework | Cobra | Already in use, proven command tree management |
| Database | SQLite (modernc.org/sqlite) | Existing, no schema changes needed |
| Terminal Multiplexer | Zellij | Already in use, floating pane support for overlays |
| Terminal Emulator | Wezterm | Already in use, spawns new windows for dashboard |
| TUI Framework | Bubbletea + Lipgloss | Already in use for dashboard views |
| Configuration | YAML (gopkg.in/yaml.v3) | Already in use, hot-reload via fsnotify |
| File Watching | fsnotify/fsnotify | Go-native, for config hot-reload |
| Clipboard | atotto/clipboard | Go-native, for export copy-to-clipboard |
| Browser Open | pkg/browser | Go-native, for docs/feedback/support URLs |
| Git UI | lazygit | Proven terminal git UI, eliminates need for custom git status rendering |
| Testing | go test (stdlib) | Existing test infrastructure |
| Linting | golangci-lint | Existing CI pipeline |
| Build | Makefile + ldflags | Existing build system |
| Distribution | Single binary via `go build` | Existing pattern |

## Project Infrastructure

### Directory Structure

```
computeCommander/
  cmd/cc/
    main.go                           # Entry point (updated: root cmd opens UI)
  internal/
    commands/
      app.go                          # App struct (unchanged)
      dashboard.go                    # Dashboard cmd (updated: default action)
      lifecycle.go                    # NEW: stop, reset, restart commands
      info.go                         # NEW: help, docs, version, update commands
      data.go                         # NEW: export, backup, restore commands
      utility.go                      # NEW: shell, feedback, support commands
      settings.go                     # NEW: theme, notifications, analytics, integrations, automation
      clear.go                        # NEW: clear logs command
      gitstatus.go                    # NEW: git-status pane command
      session.go                      # NEW: session list/switch/stop commands
      sling.go                        # Unchanged
      stop.go                         # Unchanged (agent stop, not cmdr stop)
      status.go                       # Enhanced: includes UI status
      coordinator.go                  # Unchanged
      mail.go                         # Unchanged
      nudge.go                        # Unchanged
      merge.go                        # Unchanged
      group.go                        # Unchanged
      inspect.go                      # Unchanged
      observability.go                # Unchanged
      watch.go                        # Unchanged
      worktree.go                     # Unchanged
      clean.go                        # Unchanged
      doctor.go                       # Unchanged
      feature.go                      # Unchanged
    tui/
      dashboard.go                    # Updated: new pane layout
      keybinds.go                     # NEW: leader key handler + action dispatch
      floating.go                     # NEW: floating pane renderer (help, version, confirm)
      filepicker.go                   # NEW: file picker / directory navigator
      session_manager.go              # NEW: session switching logic
      session_focus.go                # NEW: SessionFocusManager (cross-pane session coupling)
      pty_manager.go                  # NEW: PTYManager (concurrent PTY lifecycle)
      theme.go                        # Enhanced: theme loading + switching
      agent_table.go                  # Unchanged (used in bottom pane now)
      mail_summary.go                 # Unchanged (used in bottom pane now)
      merge_view.go             # Unchanged (used in bottom pane now)
      cost_tracker.go                 # Unchanged
    config/
      config.go                       # Enhanced: hot-reload support
      watcher.go                      # NEW: fsnotify config watcher
    zellij/
      pane.go                         # Enhanced: floating pane helpers
      layout.go                       # NEW: KDL layout generation
    keybinds/
      keybinds.go                     # NEW: keybind config loading + matching
      registry.go                     # NEW: action registry (command -> handler map)
    backup/
      backup.go                       # NEW: SQLite backup + restore logic
    export/
      export.go                       # NEW: data export (JSON/CSV)
    platform/db/                      # Unchanged
    agents/                           # Unchanged
    mail/                             # Unchanged
    merge/                            # Unchanged
    watchdog/                         # Unchanged
    gateway/                          # Unchanged
    wezterm/                          # Unchanged
    worktree/                         # Unchanged
  pkg/runtimes/                       # Unchanged
  agents/                             # Unchanged (YAML role definitions)
  migrations/                         # Unchanged
  templates/                          # Unchanged
  Makefile                            # Unchanged
  go.mod                              # Updated: new deps (fsnotify, clipboard, browser)
  go.sum                              # Updated
```

### Version Management

Version lives in two locations (verified in sync by CI):

- `cmd/cc/main.go` — `var version = "X.Y.Z"` (injected via ldflags at build)
- `Makefile` — `VERSION ?= X.Y.Z`

Bump via: edit `Makefile` VERSION, `make build` injects via ldflags.

### CHANGELOG.md

[Keep a Changelog](https://keepachangelog.com/) format:

```markdown
# Changelog

## [Unreleased]

### Added
- `cmdr` (bare command) opens the full interface
- `cmdr stop`, `cmdr reset`, `cmdr restart` lifecycle commands
- `cmdr help` with zellij floating pane display
- `cmdr docs`, `cmdr feedback`, `cmdr support` browser-open commands
- `cmdr export`, `cmdr backup`, `cmdr restore` data commands
- `cmdr shell` with `--agent` and `--sub-agent` flags
- `cmdr clear` log clearing
- `cmdr version`, `cmdr update` with release notes links
- `cmdr theme`, `cmdr notifications`, `cmdr analytics` (placeholder commands)
- `cmdr integrations`, `cmdr automation` (placeholder commands)
- Leader key system: `Ctrl+Space` + action key for all in-UI operations
- Redesigned dashboard layout: FP sidebar (15%) + agent session (top 80%) + event log/mail/merge/lazygit (bottom 20%) + agents sidebar (8%)
- Config hot-reload via fsnotify
- Database backup and restore
- Data export (JSON/CSV)
- Keybind configuration via keybinds.yaml
- `--agent` and `--sub-agent` global flags

### Changed
- `cmdr init` now opens interface after initialization
- Root command `Use` changed from `cc` to `cmdr`
- Dashboard pane layout restructured (agent picker replaced by agent session)
- Status command enhanced to include UI process status
```

### CI Workflow

```yaml
name: CI
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]
jobs:
  ci:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go vet ./...
      - run: go test ./...
      - name: Lint
        run: |
          command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"
      - name: Build
        run: make build
```

### Scripts (Makefile)

```makefile
VERSION ?= 0.2.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

.PHONY: build test lint vet clean

build:
	go build $(LDFLAGS) -o cmdr ./cmd/cc/

test:
	go test ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

vet:
	go vet ./...

clean:
	rm -f cmdr
```

## Estimated Size

| Area | Files | LOC |
|------|-------|-----|
| New commands (lifecycle, info, data, utility, settings, clear) | 6 | ~800 |
| Keybind system (keybinds pkg + TUI handler) | 3 | ~400 |
| Floating pane renderer | 1 | ~300 |
| Config hot-reload (watcher) | 1 | ~120 |
| Backup/restore | 1 | ~200 |
| Export | 1 | ~250 |
| Zellij layout generation | 1 | ~100 |
| TUI dashboard update (layout restructure) | 1 | ~200 |
| Entry point update (main.go) | 1 | ~150 |
| Tests for new commands | 6 | ~600 |
| Tests for keybinds, backup, export | 3 | ~300 |
| Config/theme files (YAML) | 3 | ~80 |
| File picker pane (fp) + session manager | 2 | ~800 |
| Session commands (list, switch, stop) | 1 | ~200 |
| SessionFocusManager | 1 | ~300 |
| PTYManager | 1 | ~400 |
| Git status pane (lazygit integration) | 1 | ~150 |
| Logs enhancement (--follow, --lines flags) | 0 | ~50 |
| **Total new/modified** | **35** | **~5,350** |

## UI Layout Comparison

### Current Layout (5 panes)

```
+-------------------------------+-------------+-------------+
|                               |             |             |
|       agent_picker            |    mail     | merge_queue |
|         (54%)                 |   (22%)     |   (22%)     |
|                               |             |             |
|                               |             |             |
+-------------------------------+-----+-------+------+------+
|              event_log (80%)        | cmdr_feed    |
|                                     |   (20%)      |
+-------------------------------------+--------------+
```

### Target Layout (7 panels)

```
┌─────────┬──────────────────────────────────────────────────────┬──────┐
│         │                                                      │      │
│         │              Agent Session                            │Agents│
│   FP    │              (i.e. Claude Code)                       │      │
│  (15%)  │                (77% width)                            │ (8%) │
│         │                                                      │      │
│ Dir nav │  Embedded orchestrator -- user interacts              │Agent │
│ + sess  │  with swarm here. Switches between sessions          │list /│
│ launch  │  via fp pane directory selection.                     │ mgmt │
│         │                                                      │      │
│         ├────────────┬──────────┬──────────────┬───────────────┤      │
│         │  Event     │  Mail    │   Merge      │  Git          │      │
│         │  Log       │          │   Queue      │ Status        │      │
└─────────┴────────────┴──────────┴──────────────┴───────────────┴──────┘
```

Panels:
| Panel         | Position      | Scope          | Description                                          |
|---------------|---------------|----------------|------------------------------------------------------|
| FP            | Left sidebar  | Session-scoped | File picker / navigation (full height)               |
| Agent Session | Center main   | Session-scoped | Primary workspace (Claude Code, etc.)                |
| Agents        | Right sidebar | Cross-project  | Agent roster from ALL active sessions (full height)  |
| Event Log     | Bottom bar    | Cross-project  | System events and logs from all projects             |
| Mail          | Bottom bar    | Cross-project  | Messages / notifications from all projects           |
| Merge Queue   | Bottom bar    | Cross-project  | Pending merges / PRs from all projects               |
| Git Status    | Bottom bar    | Session-scoped | lazygit — full interactive git UI scoped to active project session |

### Floating Pane Overlay (triggered by leader key)

```
┌─────────┬─────────────────────────────────────────────┬──────────┐
│         │                                             │          │
│         │     agent_session (dimmed)                   │          │
│   FP    │  ┌────────────────────────────────────────┐  │  Agents  │
│         │  │                                        │  │          │
│         │  │     Floating Pane (60% x 60%)          │  │          │
│         │  │     Content: help / version / export   │  │          │
│         │  │     [Esc to close]                     │  │          │
│         │  │                                        │  │          │
│         │  └────────────────────────────────────────┘  │          │
│         │                                             │          │
│         ├───────────┬─────────┬─────────────┬─────────┤          │
│         │ Event Log │  Mail   │ Merge Queue │Git Stat │          │
└─────────┴───────────┴─────────┴─────────────┴─────────┴──────────┘
```

## Agent Assignments

| Task | Agent | Rationale |
|------|-------|-----------|
| Entry point redesign (main.go root command) | `unix-coder` | Single file, well-scoped CLI wiring |
| Lifecycle commands (stop, reset, restart) | `unix-coder` | Process management, confirmation gates |
| Information commands (help, docs, version, update) | `unix-coder` | Browser open, floating pane rendering |
| Data commands (export, backup, restore) | `unix-coder` | SQLite backup API, JSON serialization |
| Utility commands (shell, feedback, support, clear) | `unix-coder` | Zellij pane spawning, browser open |
| Settings commands (theme, notifications, analytics, integrations, automation) | `unix-coder` | Placeholder commands with config read/write |
| Keybind system | `unix-coder` | YAML loading, TUI event dispatch |
| Floating pane renderer | `unix-coder` | Bubbletea component, zellij floating pane API |
| Dashboard layout restructure | `unix-coder` | KDL layout + TUI view composition update |
| Config hot-reload | `unix-coder` | fsnotify watcher, config diff/apply |
| Status command enhancement | `unix-coder` | Process detection, JSON output update |
| Global flags (--agent, --sub-agent) | `unix-coder` | Cobra persistent flags, propagation |
| File picker (fp) pane + session manager | `unix-coder` | TUI component, directory tree traversal, session state |
| Session commands (list, switch, stop) | `unix-coder` | CLI commands for multi-directory agent sessions |
| Git status pane (lazygit integration) | `unix-coder` | lazygit process lifecycle, session-scoped restart |
| SessionFocusManager | `unix-coder` | Cross-pane session coupling, MRU fallback |
| PTYManager | `unix-coder` | Concurrent PTY lifecycle, scrollback buffering |
| Architecture review | `code-review` | DRY analysis, command registry pattern validation |
| Security review | `security-review` | Confirmation gates, backup file permissions, clipboard handling |
| Integration testing | `unix-coder` | End-to-end lifecycle + session-scoping tests |

## Execution Order

Aligned with the Dependency Graph (Section 16). Four phases plus a final integration phase.

```
Phase 1: Foundation [parallel]
  T1  -- Entry point redesign: main.go root command (agent: unix-coder)
  T3  -- Keybind config loading (agent: unix-coder)
  T16 -- Add go.mod dependencies (agent: unix-coder)

Phase 2: Core Commands + UI Layer [blocked by Phase 1, parallel within]
  T2  -- Global flags: --agent, --sub-agent (agent: unix-coder)
  T4  -- Lifecycle commands: stop, reset, restart (agent: unix-coder)
  T5  -- Information commands: help, docs, version, update (agent: unix-coder)
  T6  -- Data commands: export, backup, restore (agent: unix-coder)
  T7  -- Utility commands: shell, feedback, support, clear (agent: unix-coder)
  T8  -- Settings commands: theme, notifications, analytics, etc. (agent: unix-coder)
  T9  -- Dashboard layout restructure (agent: unix-coder)
  T12 -- Config hot-reload watcher (agent: unix-coder)
  T21 -- Logs enhancement: --follow, --lines flags (agent: unix-coder)

Phase 3: Advanced UI + Session Management [blocked by Phase 2, parallel within]
  T10 -- Floating pane renderer (agent: unix-coder)
  T11 -- Leader key handler in TUI (agent: unix-coder)
  T13 -- KDL layout generation (agent: unix-coder)
  T14 -- Status command enhancement (agent: unix-coder)
  T15 -- Update cmdr init (agent: unix-coder)
  T19 -- File picker (fp) pane (agent: unix-coder)
  T20 -- Session commands: list, switch, stop (agent: unix-coder)
  T23 -- Git status pane: lazygit integration (agent: unix-coder)
  T24 -- SessionFocusManager (agent: unix-coder)
  T25 -- PTYManager (agent: unix-coder)

Phase 4: Review [blocked by Phase 3, parallel within]
  T17 -- Architecture review (agent: code-review)
  T18 -- Security review (agent: security-review)

Final: Integration [blocked by Phase 4]
  T22 -- Integration testing incl. session-scoping tests (agent: unix-coder)
```

Recommended directive: `/pai` -- plan-then-implement pipeline. The work is sequential (foundation -> commands+UI -> advanced UI+sessions -> review -> integration) with parallelism within each phase.

## Failure Modes

| Failure | Detection | Recovery |
|---------|-----------|----------|
| DB not found on `cmdr` | `os.Stat()` on SQLite path returns error | Print "Run `cmdr init` first" and exit 1 |
| Zellij not installed | `exec.LookPath("zellij")` returns error | Fall back to in-process TUI (existing behavior) |
| Wezterm not installed | `exec.LookPath("wezterm")` returns error | Fall back to zellij in current terminal |
| cmdr already running | Lock file exists with live PID | Print "cmdr is already running (PID: XXXX)" and exit 0 |
| Config hot-reload parse failure | YAML unmarshal returns error | Keep current config, log error, show in status bar |
| Backup fails (disk full) | `sqlite3 .backup` returns error | Report error, do not modify anything |
| Restore fails (corrupt backup) | Schema validation after copy fails | Roll back: keep original DB, remove temp file |
| Leader key conflict with zellij | `Ctrl+Space` already bound in zellij config | Document: user must unbind `Ctrl+Space` in zellij config |
| Floating pane spawn fails | `zellij action new-pane --floating` returns error | Fall back to inline output in current pane |
| $EDITOR not set | `os.Getenv("EDITOR")` returns empty | Default to `vi` (existing behavior) |
| Browser open fails | `pkg/browser.Open()` returns error | Print URL to stdout, let user copy manually |
| lazygit not installed | `exec.LookPath("lazygit")` returns error | Fall back to simple `git status` output in the git_status pane |
| Agent not found for --agent flag | DB query returns no rows | Print "Agent 'X' not found. Run `cmdr status` to see active agents" |

## 15. Task Manifest

| ID | Agent | Description | File Scope (read) | File Scope (write) | Depends On | Verify Command |
|----|-------|-------------|--------------------|--------------------|------------|----------------|
| T1 | unix-coder | Redesign root command: bare `cmdr` opens interface, change Use to "cmdr" | cmd/cc/main.go | cmd/cc/main.go | -- | `go build ./cmd/cc/ && go vet ./cmd/cc/` |
| T2 | unix-coder | Add global --agent and --sub-agent persistent flags | cmd/cc/main.go | cmd/cc/main.go | T1 | `go build ./cmd/cc/ && go vet ./cmd/cc/` |
| T3 | unix-coder | Create keybind config loader (keybinds.yaml parsing) | -- | internal/keybinds/keybinds.go, internal/keybinds/registry.go | -- | `go test ./internal/keybinds/...` |
| T4 | unix-coder | Implement lifecycle commands: stop (as `ShutdownCmd` to avoid collision with existing agent `StopCmd` in stop.go), reset, restart. Note: the Cobra command name is `stop` but the Go function is `ShutdownCmd`. | internal/commands/app.go, internal/commands/stop.go, internal/zellij/pane.go | internal/commands/lifecycle.go | T1 | `go test ./internal/commands/... -run TestLifecycle` |
| T5 | unix-coder | Implement info commands: help, docs, version, update | cmd/cc/main.go, internal/commands/app.go | internal/commands/info.go | T1 | `go test ./internal/commands/... -run TestInfo` |
| T6 | unix-coder | Implement data commands: export, backup, restore | internal/platform/db/db.go | internal/commands/data.go, internal/backup/backup.go, internal/export/export.go | T1 | `go test ./internal/backup/... && go test ./internal/export/... && go test ./internal/commands/... -run TestData` |
| T7 | unix-coder | Implement utility commands: shell, feedback, support, clear | internal/zellij/pane.go | internal/commands/utility.go, internal/commands/clear.go | T1 | `go test ./internal/commands/... -run TestUtility` |
| T8 | unix-coder | Implement settings commands: theme, notifications, analytics, integrations, automation | internal/config/config.go | internal/commands/settings.go | T1 | `go test ./internal/commands/... -run TestSettings` |
| T9 | unix-coder | Restructure dashboard layout: agent session (top) + event log/mail/merge/git-status (bottom), fp sidebar (left), agents sidebar (right). See Panels table in UI Layout Comparison. | internal/tui/dashboard.go, internal/tui/agent_table.go, internal/tui/mail_summary.go, internal/tui/merge_view.go | internal/tui/dashboard.go | T1 | `go test ./internal/tui/...` |
| T10 | unix-coder | Implement floating pane renderer for help, version, confirm dialogs | internal/zellij/pane.go | internal/tui/floating.go | T9 | `go test ./internal/tui/... -run TestFloating` |
| T11 | unix-coder | Implement leader key handler in TUI event loop | internal/tui/dashboard.go, internal/keybinds/keybinds.go | internal/tui/keybinds.go | T3, T9 | `go test ./internal/tui/... -run TestKeybind` |
| T12 | unix-coder | Implement config hot-reload via fsnotify | internal/config/config.go | internal/config/watcher.go | T1 | `go test ./internal/config/... -run TestWatcher` |
| T13 | unix-coder | Generate KDL layout file for redesigned dashboard | internal/agents/spawner.go | internal/zellij/layout.go | T9 | `go test ./internal/zellij/... -run TestLayout` |
| T14 | unix-coder | Enhance status command: include UI process detection | internal/commands/status.go | internal/commands/status.go | T4 | `go test ./internal/commands/... -run TestStatus` |
| T15 | unix-coder | Update cmdr init to generate keybinds.yaml and open interface after DB start | cmd/cc/main.go | cmd/cc/main.go | T3, T4 | `go test ./cmd/cc/... -run TestInit` |
| T16 | unix-coder | Add go.mod dependencies: fsnotify, clipboard, browser | go.mod | go.mod, go.sum | -- | `go mod tidy && go build ./...` |
| T17 | code-review | Architecture review: command registry pattern, DRY, separation of concerns | internal/commands/*.go, internal/tui/*.go, internal/keybinds/*.go | -- | T4, T5, T6, T7, T8, T9, T10, T11 | `echo "review complete"` |
| T18 | security-review | Security review: confirmation gates, backup permissions, clipboard | internal/commands/lifecycle.go, internal/commands/data.go, internal/backup/backup.go | -- | T4, T6 | `echo "review complete"` |
| T19 | unix-coder | Implement fp (file picker) pane: directory navigator + session launcher TUI component | internal/tui/dashboard.go | internal/tui/filepicker.go, internal/tui/session_manager.go | T9 | `go test ./internal/tui/... -run TestFilePicker` |
| T20 | unix-coder | Implement directory session management: session list, switch, stop commands | internal/commands/app.go, internal/agents/spawner.go | internal/commands/session.go | T1, T19 | `go test ./internal/commands/... -run TestSession` |
| T21 | unix-coder | Enhance logs command: add --follow and --lines flags to existing LogsCmd | internal/commands/observability.go | internal/commands/observability.go | T1 | `go test ./internal/commands/... -run TestLogs` |
| T22 | unix-coder | Integration tests: full lifecycle (init -> open -> fp navigate -> session start -> switch -> stop -> restart). Must include session-scoping tests: verify FP, Git Status, and Agent Workspace all update when session changes. | internal/commands/*.go | internal/commands/integration_test.go | T17, T18, T20, T21, T23, T24, T25 | `go test ./internal/commands/... -run TestIntegration` |
| T23 | unix-coder | Configure lazygit integration in git_status pane with session-scoped directory switching. `SessionFocusManager` kills and restarts lazygit with `-p <new_directory>` on session change. Requires `exec.LookPath("lazygit")` check with fallback to simple `git status` output. | internal/commands/app.go, internal/tui/session_focus.go | internal/commands/gitstatus.go | T1, T24 | `go test ./internal/commands/... -run TestGitStatus` |
| T24 | unix-coder | Implement `SessionFocusManager`: central coordinator for session-scoped panes. Manages active session state, broadcasts `SessionFocusChanged` events to registered listeners (FP, Agent Workspace, Git Status). Includes MRU fallback logic. | internal/tui/dashboard.go, internal/tui/session_manager.go | internal/tui/session_focus.go | T19, T20 | `go test ./internal/tui/... -run TestSessionFocus` |
| T25 | unix-coder | Implement `PTYManager`: manages concurrent PTY processes for multi-session agent workspace. Handles spawn, attach, detach, kill, scrollback buffering, and max-concurrent enforcement. | internal/tui/agent_session.go, internal/config/config.go | internal/tui/pty_manager.go | T9, T19 | `go test ./internal/tui/... -run TestPTYManager` |

## 16. Dependency Graph

```
Phase 1 (parallel): [T1, T3, T16]
Phase 2 (after Phase 1): [T2, T4, T5, T6, T7, T8, T9, T12, T21]
Phase 3 (after Phase 2): [T10, T11, T13, T14, T15, T19, T20, T23, T24, T25]
Phase 4 (after Phase 3): [T17, T18]
Final: [T22] -- integration test (depends on T17, T18, T20, T21, T23, T24, T25)
```

## 17. Target State

Files created:
- `internal/commands/lifecycle.go`
- `internal/commands/info.go`
- `internal/commands/data.go`
- `internal/commands/utility.go`
- `internal/commands/clear.go`
- `internal/commands/settings.go`
- `internal/commands/session.go`
- `internal/commands/gitstatus.go`
- `internal/commands/integration_test.go`
- `internal/tui/floating.go`
- `internal/tui/keybinds.go`
- `internal/tui/filepicker.go`
- `internal/tui/session_manager.go`
- `internal/tui/session_focus.go`
- `internal/tui/pty_manager.go`
- `internal/keybinds/keybinds.go`
- `internal/keybinds/registry.go`
- `internal/config/watcher.go`
- `internal/zellij/layout.go`
- `internal/backup/backup.go`
- `internal/export/export.go`

Files modified:
- `cmd/cc/main.go`
- `internal/tui/dashboard.go`
- `internal/commands/status.go`
- `internal/commands/observability.go`
- `go.mod`
- `go.sum`

Files deleted: none

## 18. Verification Plan

Per-task checks: (from Task Manifest verify column -- see T1 through T25)

Integration check:
```bash
make build && \
  ./cmdr version --json && \
  go test ./... && \
  go vet ./...
```

Rollback:
```bash
git stash
```

## 19. Success Criteria (Machine-Verifiable)

- [ ] `go build ./cmd/cc/` exits 0 and produces `cmdr` binary
- [ ] `go test ./...` exits 0 with all tests passing
- [ ] `go vet ./...` exits 0 with no warnings
- [ ] `./cmdr version --json` outputs valid JSON with `success: true`
- [ ] `./cmdr help` exits 0 and output contains "stop", "reset", "restart", "export", "backup", "restore", "shell"
- [ ] `./cmdr status --json` outputs valid JSON with `db` and `ui` fields
- [ ] File `internal/commands/lifecycle.go` exists and contains `ShutdownCmd` (lifecycle stop — distinct from existing agent `StopCmd` in `stop.go`), `ResetCmd`, `RestartCmd` functions
- [ ] File `internal/commands/data.go` exists and contains `ExportCmd`, `BackupCmd`, `RestoreCmd` functions
- [ ] File `internal/commands/info.go` exists and contains `HelpCmd`, `DocsCmd`, `VersionCmd`, `UpdateCmd` functions
- [ ] File `internal/tui/keybinds.go` exists and contains leader key handler
- [ ] File `internal/tui/floating.go` exists and contains floating pane renderer
- [ ] File `internal/keybinds/keybinds.go` exists and parses `keybinds.yaml`
- [ ] File `internal/config/watcher.go` exists and implements fsnotify watcher
- [ ] File `internal/backup/backup.go` exists and implements SQLite backup/restore
- [ ] File `internal/export/export.go` exists and implements JSON/CSV export
- [ ] Root command `Use` field is `"cmdr"` (verified by grep in `cmd/cc/main.go`)
- [ ] Root command has `RunE` that opens the interface (verified by grep in `cmd/cc/main.go`)
- [ ] `--agent` and `--sub-agent` flags exist on root command (verified by `./cmdr --help`)
- [ ] File `internal/tui/filepicker.go` exists and contains directory navigation TUI component
- [ ] File `internal/tui/session_manager.go` exists and contains session switching logic
- [ ] File `internal/tui/session_focus.go` exists and contains `SessionFocusManager` with `switchTo()`, `onSessionChange()`, and `getMRUOrder()` methods
- [ ] File `internal/tui/pty_manager.go` exists and contains `PTYManager` with `spawn()`, `attach()`, `detach()`, `kill()` methods
- [ ] File `internal/commands/gitstatus.go` exists and contains `GitStatusCmd` with lazygit integration (process restart on session switch, fallback when lazygit not installed)
- [ ] File `internal/commands/session.go` exists and contains `SessionListCmd`, `SessionSwitchCmd`, `SessionStopCmd` functions
- [ ] `./cmdr help` output contains "session", "fp", and "git-status" commands
- [ ] Session switching updates FP, Agent Workspace, and Git Status (lazygit restart) panes simultaneously (integration test)
- [ ] `go mod tidy` exits 0 (no missing or unused dependencies)

## Open Questions

| # | Question | Impact | Suggested Default |
|---|----------|--------|-------------------|
| 1 | What URL should `cmdr docs` open? | Blocks `DocsCmd` implementation | `https://github.com/noko/computecommander/wiki` |
| 2 | What URL should `cmdr feedback` open? | Blocks `FeedbackCmd` implementation | `https://github.com/noko/computecommander/issues/new` |
| 3 | What URL should `cmdr support` open? | Blocks `SupportCmd` implementation | `https://github.com/noko/computecommander/discussions` |
| 4 | How should `cmdr update` check for new versions? | Blocks `UpdateCmd` implementation | GitHub Releases API: `GET /repos/noko/computecommander/releases/latest` |
| 5 | Should the binary name change from `cc` to `cmdr` in the Makefile and root command, or should both be supported? | Affects `cmd/cc/main.go` `Use` field and Makefile output | Change `Use` to `"cmdr"`, keep Makefile output as `cmdr` (already is), add `cc` as an alias |
| 6 | Should `Ctrl+Space` be configurable as the leader key, or hardcoded? | Affects keybind system complexity | Hardcoded in TUI code, documented in keybinds.yaml as reference only |
| 7 | Which orchestrator runtime should the agent session pane spawn by default? | Affects `cmdr` default behavior | Use `defaults.runtime` from config (currently `claude`) |
| 8 | Should `cmdr shell --agent` open a shell in the agent's worktree, or in the agent's zellij pane? | Affects shell command implementation | New zellij pane with `cwd` set to agent's worktree path |
