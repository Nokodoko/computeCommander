# SPEC: Wire Agent Color Resolver into Events Log and Bridge

**Date:** 2026-03-08
**Status:** Approved

## Problem

The agent color resolver infrastructure exists and is wired into the agent table, mail summary, merge view, cost tracker, and CLI status/mail/merge panes. However, three components are missing color support:

1. **Events log (TUI):** `internal/tui/events_pane.go` renders agent source names as plain text
2. **Events log (CLI):** `internal/commands/observability.go` uses `eventAgentColor()` which maps by event type keyword, not by agent identity
3. **Bridge:** `~/.claude/hooks/cmdr-bridge.sh` inserts sessions with default `color_index=0` and `color_hex='#808080'` instead of assigning palette colors

## Track D: System-Wide Agent Database

**Status: ALREADY EXISTS -- NO WORK NEEDED**

- System-wide DB: `~/.computecommander/cc.db`
- Config: `internal/config/config.go` line 244, `SystemDBPath()` at line 558
- Migration: `migrations/sqlite/002_system_wide.sql` defines projects table and project_id FKs
- CLI: `cmdr migrate` copies local per-project DBs to system-wide DB
- App: `NewAppSystemWide()` in `internal/commands/app.go` line 63

## Track A: Events Pane Color Resolver (TUI)

**File:** `internal/tui/events_pane.go`

**Pattern:** Identical to `MailSummary`, `MergeQueueView`, and `CostTracker`.

### Changes

1. Add `colorResolver AgentColorResolver` field to `EventsPane` struct (line 21)
2. Add `SetColorResolver(resolver AgentColorResolver)` method (after `SetDB`)
3. In `View()` (line 183), after building the `source` string, color it using the resolver:
   ```go
   if ep.colorResolver != nil {
       if hex := ep.colorResolver(e.Source); hex != "" {
           source = AgentColorStyle(hex).Render(source)
       }
   }
   ```

**File:** `internal/tui/dashboard.go`

4. Add `colorResolver AgentColorResolver` field to `DashboardOpts` struct
5. In `NewDashboard()`, after constructing all components, call `SetColorResolver` on:
   - `eventsPane`
   - `mail` (already has the method, just not wired)
   - `queue` (already has the method, just not wired)
   - `costs` (already has the method, just not wired)
6. The caller (`internal/commands/dashboard.go` or `cmd/cc/main.go`) must pass `spawner.BuildColorResolver(ctx)` into DashboardOpts

**Caller wiring:** Find where `NewDashboard` is called and add the resolver to opts.

## Track B: Events Log Color Resolver (CLI)

**File:** `internal/commands/observability.go`

### Changes

1. In `printEventPane()` (line 239): Replace `eventAgentColor(e.EventType)` with proper color resolution:
   - Add `colorResolver func(string) string` parameter to `printEventPane`
   - Use `colorizeAgent(truncate(e.Agent, 16), colorResolver(e.Agent))` (same pattern as status.go line 400)
2. In `runFeedPane()` (line 172): Build color resolver at start of function:
   ```go
   colorResolver := app.Spawner.BuildColorResolver(ctx)
   ```
   In the render closure, use `colorizeAgent(truncate(e.Agent, 12), colorResolver(e.Agent))` for agent names
3. Update all callers of `printEventPane` to pass the resolver
4. Remove `eventAgentColor()` function (lines 274-286) -- dead code after replacement

## Track C: Bridge Color Assignment

**File:** `~/.claude/hooks/cmdr-bridge.sh`

### Changes

1. Add palette hex array at the top of the file (after the logging section):
   ```bash
   PALETTE_HEXES=(
       "#FF6B6B" "#4ECDC4" "#45B7D1" "#96CEB4" "#FFEAA7" "#DDA0DD"
       "#98D8C8" "#F7DC6F" "#BB8FCE" "#85C1E9" "#F0B27A" "#82E0AA"
   )
   ```

2. Add `assign_color()` function:
   ```bash
   assign_color() {
       local db="$1"
       local count
       count=$(sqlite3 "$db" "SELECT COUNT(*) FROM sessions WHERE state = 'working';" 2>/dev/null || echo "0")
       local idx=$(( count % 12 ))
       echo "$idx ${PALETTE_HEXES[$idx]}"
   }
   ```

3. In `do_start()`, before the INSERT, call `assign_color`:
   ```bash
   local color_info color_index color_hex
   color_info=$(assign_color "$cmdr_db")
   color_index=$(echo "$color_info" | cut -d' ' -f1)
   color_hex=$(echo "$color_info" | cut -d' ' -f2)
   ```

4. Update the INSERT statement to include `color_index` and `color_hex` columns:
   - Add to column list: `, color_index, color_hex`
   - Add to VALUES: `, ${color_index}, '${color_hex}'`

5. After the INSERT, also write to `agent_colors` table (same as Go spawner):
   ```bash
   sqlite3 "$cmdr_db" \
       "INSERT OR IGNORE INTO agent_colors (agent_name, run_id, color_index, color_hex) VALUES ('${agent_name//\'/\'\'}', '', ${color_index}, '${color_hex}');" \
       2>/dev/null || true
   ```

## Implementation Order

All three tracks are independent and can run in parallel.

## Verification

- `go build ./cmd/cc/` must pass after Track A and B changes
- Track C is a shell script change -- verify syntax with `bash -n cmdr-bridge.sh`

## Files Modified

| Track | File | Type |
|-------|------|------|
| A | `internal/tui/events_pane.go` | Add colorResolver field + method + View() coloring |
| A | `internal/tui/dashboard.go` | Add colorResolver to DashboardOpts, wire SetColorResolver |
| A | Caller of NewDashboard | Pass spawner.BuildColorResolver |
| B | `internal/commands/observability.go` | Replace eventAgentColor with proper resolver |
| C | `~/.claude/hooks/cmdr-bridge.sh` | Add palette array, assign_color(), update INSERT |
