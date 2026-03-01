# internal/tui/ -- Terminal User Interface (BubbleTea Dashboard)

## Purpose
In-process TUI dashboard for monitoring the ComputeCommander agent swarm. Built on BubbleTea/Lipgloss, it displays agent status tables, mail summaries, merge queue state, and cost tracking in an alt-screen terminal UI with periodic auto-refresh. Also provides file picker, session management, floating pane overlays, and leader-key keybind dispatch.

## Technology
- Go 1.25
- `github.com/charmbracelet/bubbletea` for the TUI event loop
- `github.com/charmbracelet/lipgloss` for terminal styling
- Depends on: `internal/config`, `internal/keybinds`, `internal/mail`, `internal/merge`

## Contents
| File | Description |
|------|-------------|
| `dashboard.go` | `Dashboard` struct (top-level BubbleTea Model), `NewDashboard()`, `Run()`, `Refresh()`, `Init()`/`Update()`/`View()` lifecycle, key handling (s/m/c/q/j/k/n/i) |
| `theme.go` | `Theme` struct with lipgloss styles for title, headers, state colors (booting, working, completed, stalled, zombie), merge status colors, `DefaultTheme()` |
| `render.go` | Table rendering helpers: `renderStatusBar()`, `renderHelpBar()`, `truncate()`, `formatTokens()` |
| `agent_table.go` | `AgentTable` component: cursor navigation (j/k), color-coded state rendering, `SessionLister` interface, `Refresh()` from DB |
| `mail_summary.go` | `MailSummary` component: unread count, recent message previews, `UnreadCount()` |
| `merge_view.go` | `MergeQueueView` component: pending count, color-coded status entries, `PendingCount()` |
| `cost_tracker.go` | `CostTracker` component: aggregates token usage and estimated cost per agent/model, `TotalCost()` |
| `filepicker.go` | `FilePicker` component: directory navigation TUI for zellij fp pane, `FilePickerModel` (standalone BubbleTea Model), `RunFilePicker()`, session markers |
| `floating.go` | `FloatingPane` renderer: floating overlay for help, version, confirm dialogs; `RenderHelpContent()`, `RenderVersionContent()`, `RenderConfirmContent()`, `RenderExportPreview()` |
| `keybinds.go` | `LeaderKeyHandler`: Ctrl+Space leader key state machine, dispatches to keybind config/registry; `IsLeaderKey()` |
| `session_manager.go` | `SessionManager`: directory-scoped session tracking, create/switch/stop/list sessions with thread-safe access |
| `dashboard_test.go` | Tests for dashboard initialization, key handling, view rendering, and refresh cycle |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewDashboard` | `func NewDashboard(opts DashboardOpts) *Dashboard` | `*Dashboard` | Creates dashboard with agent table, mail summary, merge view, cost tracker |
| `Run` | `func (d *Dashboard) Run(ctx context.Context) error` | `error` | Starts BubbleTea program with alt-screen, blocks until quit or context cancel |
| `Refresh` | `func (d *Dashboard) Refresh() error` | `error` | Updates all sub-views with latest data from DB |
| `NewAgentTable` | `func NewAgentTable(lister SessionLister, theme *Theme) *AgentTable` | `*AgentTable` | Creates agent table with cursor navigation |
| `NewMailSummary` | `func NewMailSummary(store mail.MailStore, theme *Theme) *MailSummary` | `*MailSummary` | Creates mail summary panel |
| `NewMergeQueueView` | `func NewMergeQueueView(queue merge.MergeQueue, theme *Theme) *MergeQueueView` | `*MergeQueueView` | Creates merge queue panel |
| `DefaultTheme` | `func DefaultTheme() *Theme` | `*Theme` | Returns default lipgloss color scheme |
| `NewFilePicker` | `func NewFilePicker(rootDir string, theme *Theme) *FilePicker` | `*FilePicker` | Creates file picker with directory navigation, session markers |
| `RunFilePicker` | `func RunFilePicker(startPath string) error` | `error` | Runs file picker as standalone BubbleTea program |
| `NewFloatingPane` | `func NewFloatingPane(title string, theme *Theme) *FloatingPane` | `*FloatingPane` | Creates floating overlay renderer |
| `RenderHelpContent` | `func RenderHelpContent() string` | `string` | Generates leader key help text for floating pane |
| `RenderVersionContent` | `func RenderVersionContent(version string) string` | `string` | Generates version display with release URL |
| `RenderConfirmContent` | `func RenderConfirmContent(action string) string` | `string` | Generates y/n confirmation dialog |
| `NewLeaderKeyHandler` | `func NewLeaderKeyHandler(cfg *keybinds.Config, reg *keybinds.Registry) *LeaderKeyHandler` | `*LeaderKeyHandler` | Creates leader key handler with keybind config and action registry |
| `IsLeaderKey` | `func IsLeaderKey(msg tea.KeyMsg) bool` | `bool` | Checks if key is Ctrl+Space (ctrl+@ or ctrl+ ) |
| `NewSessionManager` | `func NewSessionManager() *SessionManager` | `*SessionManager` | Creates directory-session manager with thread-safe access |

## Data Types

### viewMode (int enum)
`viewEvents` (0) | `viewMail` (1) | `viewMerge` (2) | `viewCosts` (3)

### Dashboard (struct)
Fields: agents (*AgentTable), mail (*MailSummary), queue (*MergeQueueView), costs (*CostTracker), interval, theme, mode, width, height, ctx, err, leaderActive (bool)

### DashboardOpts (struct)
Fields: Lister (SessionLister), Mail, Queue, Config, Interval

### SessionLister (interface)
Defined in `agent_table.go`: abstracts session listing for testability.

### Theme (struct)
Fields: Title, Header, state-specific styles (Booting, Working, Completed, Stalled, Zombie), merge status styles

### FilePicker (struct)
Fields: rootDir, currentDir, entries ([]FilePickerEntry), cursor, theme, width, height, sessions (map[string]string)

### FilePickerEntry (struct)
Fields: Name, Path, IsDir, HasSession (bool), SessionID (string)

### FilePickerModel (struct)
BubbleTea Model wrapping FilePicker for standalone use in zellij fp pane.

### FloatingPane (struct)
Fields: title, content, width, height, theme, visible

### LeaderKeyHandler (struct)
Fields: config (*keybinds.Config), registry (*keybinds.Registry), active (bool)

### DirectorySession (struct)
Fields: ID, Directory, DisplayName, AgentSessionID, Runtime, Active (bool), LastAccessedAt, CreatedAt, StoppedAt (*time.Time)

### SessionManager (struct)
Fields: sessions (map[string]*DirectorySession), activeDir (string), mu (sync.RWMutex), nextID (int)

## Logging
- No structured logging; errors displayed inline in the TUI view
- Refresh errors accumulated and shown in red at bottom of dashboard

## CRUD Entry Points
- **Read**: All components read from DB via their respective service interfaces (SessionLister, MailStore, MergeQueue)
- **Sessions**: `CreateSession()` (create), `ListSessions()` (read), `SwitchSession()` (update), `StopSession()` (delete), `StopAll()` (delete all)
- **File Picker**: `Enter()` (navigate/select), `GoUp()` (navigate), `SetSessionActive()` / `RemoveSession()` (update session markers)
- **Floating Pane**: `Show()` (display), `Hide()` (dismiss)

## Style Guide
- BubbleTea Model pattern: `Init()`, `Update()`, `View()` on `Dashboard` and `FilePickerModel`
- Composable sub-views: each component has its own `View()` returning a string
- Lipgloss styles centralized in `Theme` struct
- Key bindings: 1=events, 2=mail, 3=merge, 4=costs, q=quit, j/k=navigate, Tab=cycle bottom pane
- Leader key (Ctrl+Space) activates leader mode; follow-up keys dispatch to actions (q=quit, ?=help, v=version, s=shell, etc.)
- Ticker-based refresh via `tea.Tick(interval, ...)`
- Thread-safe session management via `sync.RWMutex`

**Representative snippet (from `dashboard.go`):**
```go
func (d *Dashboard) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if d.leaderActive {
		d.leaderActive = false
		return d.handleLeaderAction(key)
	}

	if key == "ctrl+@" || key == "ctrl+ " {
		d.leaderActive = true
		return d, nil
	}

	switch key {
	case "q", "ctrl+c":
		return d, tea.Quit
	case "1":
		d.mode = viewEvents
	case "2":
		d.mode = viewMail
	case "tab":
		d.mode = (d.mode + 1) % 4
	}
	return d, nil
}
```
