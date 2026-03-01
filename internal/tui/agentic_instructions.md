# internal/tui/ -- Terminal User Interface (BubbleTea Dashboard)

## Purpose
In-process TUI dashboard for monitoring the ComputeCommander agent swarm. Built on BubbleTea/Lipgloss, it displays agent status tables, mail summaries, merge queue state, and cost tracking in an alt-screen terminal UI with periodic auto-refresh.

## Technology
- Go 1.25
- `github.com/charmbracelet/bubbletea` for the TUI event loop
- `github.com/charmbracelet/lipgloss` for terminal styling
- Depends on: `internal/config`, `internal/mail`, `internal/merge`

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

## Data Types

### viewMode (int enum)
`viewStatus` (0) | `viewMail` (1) | `viewCosts` (2)

### Dashboard (struct)
Fields: agents (*AgentTable), mail (*MailSummary), queue (*MergeQueueView), costs (*CostTracker), interval, theme, mode, width, height, ctx, err

### DashboardOpts (struct)
Fields: Lister (SessionLister), Mail, Queue, Config, Interval

### SessionLister (interface)
Defined in `agent_table.go`: abstracts session listing for testability.

### Theme (struct)
Fields: Title, Header, state-specific styles (Booting, Working, Completed, Stalled, Zombie), merge status styles

## Logging
- No structured logging; errors displayed inline in the TUI view
- Refresh errors accumulated and shown in red at bottom of dashboard

## CRUD Entry Points
- **Read**: All components read from DB via their respective service interfaces (SessionLister, MailStore, MergeQueue)
- **No write operations** -- the TUI is read-only

## Style Guide
- BubbleTea Model pattern: `Init()`, `Update()`, `View()` on `Dashboard`
- Composable sub-views: each component has its own `View()` returning a string
- Lipgloss styles centralized in `Theme` struct
- Key bindings: s=status, m=mail, c=costs, q=quit, j/k=navigate, n=nudge (reserved), i=inspect (reserved)
- Ticker-based refresh via `tea.Tick(interval, ...)`

**Representative snippet (from `dashboard.go`):**
```go
func (d *Dashboard) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return d, tea.Quit
	case "s":
		d.mode = viewStatus
	case "m":
		d.mode = viewMail
	case "c":
		d.mode = viewCosts
	case "j", "down":
		d.agents.CursorDown()
	case "k", "up":
		d.agents.CursorUp()
	}
	return d, nil
}
```
