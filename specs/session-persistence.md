# Spec: Session State Persistence and Restore

## Problem

Accidental closure or crash of a terminal running a cmdr session causes loss of work context. Users cannot recover which sessions were active, what directories they were working in, or which runtime was in use.

## Goal

Persist cmdr session state to disk so that sessions can be restored after a crash or accidental terminal closure, but only when the user explicitly requests restoration via a CLI flag.

---

## Architecture

### Current State

- `SessionManager` (`internal/tui/session_manager.go`) holds sessions in-memory via `map[string]*DirectorySession` or queries a DB.
- `DirectorySession` has fields: ID, Directory, DisplayName, ProjectID, AgentSessionID, Runtime, Active, LastAccessedAt, CreatedAt, StoppedAt.
- The root command in `cmd/cc/main.go` writes a `cmdr.lock` file on launch and calls `sharedApp.Close()` in `PersistentPostRun`.
- Sessions are lost when the process exits because the in-memory map is not persisted independently of the DB-backed project data.

### Design

#### 1. State File Location

Path: `<configDir>/session-state.json` where `<configDir>` is `.computecommander/` (per-project) or `~/.computecommander/` (system-wide).

The path is derived from the existing config directory resolution -- no new directory scheme.

#### 2. State File Schema

```json
{
  "version": 1,
  "savedAt": "2026-03-07T07:15:00Z",
  "pid": 12345,
  "activeDir": "/home/user/project",
  "sessions": [
    {
      "id": "dsess-00000001",
      "directory": "/home/user/project",
      "displayName": "project",
      "projectId": "",
      "agentSessionId": "sess-abc123",
      "runtime": "claude",
      "active": true,
      "lastAccessedAt": "2026-03-07T07:10:00Z",
      "createdAt": "2026-03-07T06:00:00Z",
      "stoppedAt": null
    }
  ]
}
```

#### 3. Saving State

State is saved in two scenarios:

**a) Graceful shutdown:** In `App.Close()` (called from `PersistentPostRun` in `cmd/cc/main.go`), call `SessionManager.SaveState(path)` before closing the DB.

**b) Periodic autosave:** A background goroutine in `SessionManager` writes state every 30 seconds while sessions are active. This protects against crashes where `Close()` never runs. The interval is not configurable in v1.

**c) Signal handler:** Register `SIGTERM` and `SIGINT` handlers that call `SaveState` before exit. This is a belt-and-suspenders measure alongside `PersistentPostRun`.

#### 4. Restoring State

Restoration is **opt-in only**, triggered by a CLI flag.

**Flag:** `--restore` on the root `cmdr` command.

**Behavior:**
1. On startup, if `--restore` is passed, read `session-state.json`.
2. Validate the file exists and is parseable.
3. For each session in the file, call `SessionManager.CreateSession(directory, runtime)` to re-populate the in-memory session map.
4. Set the `activeDir` to the previously active directory.
5. Delete the state file after successful restoration to prevent stale restores.
6. If the file is missing or corrupt, print a warning and continue normally.

**No automatic restore.** The user must explicitly pass `--restore`. This prevents confusion from stale state files.

#### 5. Staleness Check

Before restoring, check `savedAt` timestamp. If the state file is older than 24 hours, warn the user and require `--restore --force` to proceed. This prevents restoring very old state that no longer makes sense.

---

## Implementation Plan

### File: `internal/tui/session_state.go` (new)

```go
// SessionState represents the persisted state of all sessions.
type SessionState struct {
    Version   int                `json:"version"`
    SavedAt   time.Time          `json:"savedAt"`
    PID       int                `json:"pid"`
    ActiveDir string             `json:"activeDir"`
    Sessions  []*DirectorySession `json:"sessions"`
}

// SaveState writes the current session state to the given path.
func (sm *SessionManager) SaveState(path string) error

// LoadState reads session state from the given path.
func LoadState(path string) (*SessionState, error)

// RestoreState populates the session manager from a previously saved state.
func (sm *SessionManager) RestoreState(state *SessionState) error

// StartAutosave begins periodic state saving. Returns a stop function.
func (sm *SessionManager) StartAutosave(path string, interval time.Duration) func()
```

### File: `cmd/cc/main.go` (modified)

1. Add `--restore` flag to root command.
2. Add `--force` flag (used with `--restore` to override staleness check).
3. In `appPreRun`, after App initialization, if `--restore` is set:
   - Compute state file path from config dir.
   - Call `LoadState` and validate.
   - Call `SessionManager.RestoreState`.
4. In the root command's `RunE`, after launching, start autosave goroutine.
5. In `PersistentPostRun`, call `SaveState` before `Close()`.

### File: `internal/commands/app.go` (modified)

1. Add `SaveSessionState(configDir string) error` method to `App`.
2. Add `RestoreSessionState(configDir string, force bool) error` method to `App`.

### File: `internal/tui/session_manager.go` (modified)

No structural changes. The `SaveState`/`RestoreState` methods are added via the new `session_state.go` file in the same package.

---

## CLI Interface

```
cmdr                    # Normal launch, saves state on exit
cmdr --restore          # Restore sessions from last saved state, then launch
cmdr --restore --force  # Restore even if state file is >24h old
```

The `--restore` flag is on the root command only. Subcommands do not support it.

---

## Edge Cases

1. **Corrupt state file:** Log warning, continue without restore, do not delete the file.
2. **Missing state file with --restore:** Print "No saved session state found." and continue normally.
3. **Multiple cmdr instances:** Each instance writes its own PID to the state file. On restore, warn if the PID in the file matches a still-running process (check `/proc/<pid>/`).
4. **DB-backed sessions:** `SaveState` saves the in-memory overlay. DB-backed sessions already persist in the DB. The state file captures the `activeDir` and any in-memory-only sessions.
5. **Autosave and concurrent writes:** Use `os.WriteFile` with a temp file + rename pattern to avoid partial writes.

---

## Testing

1. **Unit tests** in `internal/tui/session_state_test.go`:
   - `TestSaveAndLoadState` -- round-trip save/load.
   - `TestRestoreState` -- verify sessions are recreated correctly.
   - `TestStalenessCheck` -- verify 24h staleness warning.
   - `TestCorruptStateFile` -- verify graceful handling.
   - `TestAutosave` -- verify periodic writes.

2. **Integration test** in `internal/commands/commands_test.go`:
   - Test `--restore` flag is registered and accessible.

---

## Non-Goals (v1)

- No automatic restore on startup.
- No UI for browsing past session states.
- No state file rotation or history.
- No encryption of the state file.
- No configurable autosave interval.
