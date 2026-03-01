package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
)

// viewMode identifies which panel is currently displayed in the bottom section.
type viewMode int

const (
	viewEvents viewMode = iota
	viewMail
	viewMerge
	viewCosts
)

// DashboardOpts groups the dependencies for constructing a Dashboard.
type DashboardOpts struct {
	Lister   SessionLister
	Mail     mail.MailStore
	Queue    merge.MergeQueue
	Config   *config.Config
	Interval time.Duration
}

// Dashboard is the top-level TUI component implementing the redesigned layout:
//
//	+----------+--------------------------------------------+----------+
//	|          |                                            |          |
//	|   FP     |           Agent Session                    |  Agents  |
//	|  (15%)   |           (center main)                    |  (15%)   |
//	|          |                                            |          |
//	|          |                                            |          |
//	|          +----------+--------+-----------+------------+          |
//	|          | Event    | Mail   | Merge     | Events     |          |
//	|          | Log      |        | Queue     |            |          |
//	+----------+----------+--------+-----------+------------+----------+
//
// Left sidebar: FP (file picker) spanning full height (~15% width)
// Center: Agent Session (top ~80%) + bottom bar (Event Log | Mail | Merge Queue | Events)
// Right sidebar: Agents list spanning full height (~15% width)
//
// The fp and agent_session panes are in the zellij layout;
// this TUI dashboard handles all panels when running with --tui.
type Dashboard struct {
	agents   *AgentTable
	mail     *MailSummary
	queue    *MergeQueueView
	costs    *CostTracker
	interval time.Duration
	theme    *Theme
	mode     viewMode
	width    int
	height   int
	ctx      context.Context
	err      error

	// leaderActive tracks whether the leader key (Ctrl+Space) has been pressed
	// and we are waiting for the follow-up key.
	leaderActive bool
}

// NewDashboard constructs a Dashboard from the provided options.
func NewDashboard(opts DashboardOpts) *Dashboard {
	theme := DefaultTheme()
	interval := opts.Interval
	if interval == 0 {
		interval = 2 * time.Second
	}
	return &Dashboard{
		agents:   NewAgentTable(opts.Lister, theme),
		mail:     NewMailSummary(opts.Mail, theme),
		queue:    NewMergeQueueView(opts.Queue, theme),
		costs:    NewCostTracker(theme),
		interval: interval,
		theme:    theme,
		mode:     viewEvents,
	}
}

// Run starts the bubbletea program and blocks until the user quits.
func (d *Dashboard) Run(ctx context.Context) error {
	d.ctx = ctx

	// Perform an initial refresh before starting.
	if err := d.Refresh(); err != nil {
		// Log but don't abort; the dashboard can show an error state.
		d.err = err
	}

	p := tea.NewProgram(d, tea.WithAltScreen())

	// Run in a goroutine so we can cancel on context.
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		p.Quit()
		return ctx.Err()
	}
}

// Refresh updates all sub-views with the latest data.
func (d *Dashboard) Refresh() error {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []string

	if err := d.agents.Refresh(ctx); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.mail.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.queue.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("refresh errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// --- bubbletea.Model implementation ---

// tickMsg triggers a periodic refresh.
type tickMsg time.Time

// Init sets up the initial command, starting the refresh ticker.
func (d *Dashboard) Init() tea.Cmd {
	return tea.Tick(d.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages and key events.
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return d.handleKey(msg)
	case tickMsg:
		d.err = d.Refresh()
		return d, tea.Tick(d.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		return d, nil
	}
	return d, nil
}

// handleKey processes keyboard input, including leader key support.
func (d *Dashboard) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// If leader key is active, process the follow-up key.
	if d.leaderActive {
		d.leaderActive = false
		return d.handleLeaderAction(key)
	}

	// Check for leader key activation (Ctrl+Space).
	if key == "ctrl+@" || key == "ctrl+ " {
		d.leaderActive = true
		return d, nil
	}

	// Normal key handling.
	switch key {
	case "q", "ctrl+c":
		return d, tea.Quit
	case "1":
		d.mode = viewEvents
	case "2":
		d.mode = viewMail
	case "3":
		d.mode = viewMerge
	case "4":
		d.mode = viewCosts
	case "j", "down":
		d.agents.CursorDown()
	case "k", "up":
		d.agents.CursorUp()
	case "tab":
		// Cycle through bottom pane views.
		d.mode = (d.mode + 1) % 4
	}
	return d, nil
}

// handleLeaderAction processes a key pressed after the leader key.
func (d *Dashboard) handleLeaderAction(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return d, tea.Quit
	case "?":
		// Help: would open floating pane.
	case "v":
		// Version: would open floating pane.
	case "u":
		// Update: would open floating pane.
	case "s":
		// Shell: would open floating pane.
	case "c":
		// Clear logs: would open confirmation.
	case "e":
		// Export: would open floating pane.
	case "r":
		// Restart: would open confirmation.
	case "b":
		// Backup: would open confirmation.
	case "R":
		// Restore: would open confirmation.
	case "f":
		// Feedback: would open browser.
	case "h":
		// Support: would open browser.
	case "t":
		// Theme: would open picker.
	case "n":
		// Notifications: would open settings.
	case "a":
		// Analytics: would open dashboard.
	case "i":
		// Integrations: would open list.
	case "m":
		// Automation: would open builder.
	case "d":
		// Toggle file picker pane.
	}
	return d, nil
}

// View renders the redesigned dashboard layout.
//
//	+----------+--------------------------------------------+----------+
//	|          |           Agent Session                     |          |
//	|   FP     |           (center, ~80% height)            |  Agents  |
//	|  (15%)   |                                            |  (15%)   |
//	|          +----------+--------+-----------+------------+          |
//	|          | EventLog | Mail   | MergeQ    | Events     |          |
//	+----------+----------+--------+-----------+------------+----------+
func (d *Dashboard) View() string {
	var sections []string

	// Header.
	header := d.theme.Title.Render("ComputeCommander Dashboard")
	sections = append(sections, header)

	// Leader key indicator.
	if d.leaderActive {
		leaderStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true)
		sections = append(sections, leaderStyle.Render("  LEADER KEY ACTIVE -- press action key"))
	}

	// Reserve space for header, status bar, help bar, and error line.
	reservedLines := 3
	if d.err != nil {
		reservedLines++
	}
	if d.leaderActive {
		reservedLines++
	}
	bodyHeight := d.height - reservedLines
	if bodyHeight < 10 {
		bodyHeight = 10
	}

	// Column widths: FP (15%), center (70%), agents (15%).
	fpWidth := d.width * 15 / 100
	if fpWidth < 10 {
		fpWidth = 10
	}
	agentsWidth := d.width * 15 / 100
	if agentsWidth < 10 {
		agentsWidth = 10
	}
	centerWidth := d.width - fpWidth - agentsWidth
	if centerWidth < 20 {
		centerWidth = 20
	}

	// --- Left sidebar: File Picker (full height) ---
	fpStyle := lipgloss.NewStyle().
		Width(fpWidth).
		Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF"))
	fpContent := d.theme.Subtitle.Render("FP") + "\n" + "(file picker)"

	// --- Right sidebar: Agents list (full height) ---
	agentsSidebarStyle := lipgloss.NewStyle().
		Width(agentsWidth).
		Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF"))
	agentsSidebarContent := d.agents.View()

	// --- Center section: Agent Session (top) + Bottom bar ---
	centerTopHeight := bodyHeight * 80 / 100
	if centerTopHeight < 6 {
		centerTopHeight = 6
	}
	centerBottomHeight := bodyHeight - centerTopHeight
	if centerBottomHeight < 3 {
		centerBottomHeight = 3
	}

	// Center top: Agent Session.
	sessionStyle := lipgloss.NewStyle().
		Width(centerWidth).
		Height(centerTopHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B82F6"))
	sessionContent := d.theme.Subtitle.Render("Agent Session") + "\n" + "(agent workspace)"

	// Center bottom: 4-section bottom bar (Event Log | Mail | Merge Queue | Events).
	bottomPaneWidth := centerWidth / 4
	bottomRemainder := centerWidth - (bottomPaneWidth * 4)

	bottomPaneStyle := lipgloss.NewStyle().
		Height(centerBottomHeight).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF"))

	eventLogContent := d.theme.Subtitle.Render("Event Log")
	mailContent := d.theme.Subtitle.Render("Mail") + "\n" + d.mail.View()
	mergeContent := d.theme.Subtitle.Render("Merge Queue") + "\n" + d.queue.View()
	eventsContent := d.theme.Subtitle.Render("Events")

	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top,
		bottomPaneStyle.Width(bottomPaneWidth).Render(eventLogContent),
		bottomPaneStyle.Width(bottomPaneWidth).Render(mailContent),
		bottomPaneStyle.Width(bottomPaneWidth).Render(mergeContent),
		bottomPaneStyle.Width(bottomPaneWidth+bottomRemainder).Render(eventsContent),
	)

	// Compose center column vertically.
	centerColumn := lipgloss.JoinVertical(lipgloss.Left,
		sessionStyle.Render(sessionContent),
		bottomRow,
	)

	// Compose the main body: FP | Center | Agents.
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		fpStyle.Render(fpContent),
		centerColumn,
		agentsSidebarStyle.Render(agentsSidebarContent),
	)
	sections = append(sections, body)

	// Status bar.
	statusBar := renderStatusBar(
		len(d.agents.Sessions()),
		d.mail.UnreadCount(),
		d.queue.PendingCount(),
		d.costs.TotalCost(),
		d.theme,
	)
	sections = append(sections, statusBar)

	// Error display.
	if d.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
		sections = append(sections, errStyle.Render("Error: "+d.err.Error()))
	}

	// Help bar with leader key hint.
	sections = append(sections, renderDashHelpBar(d.theme))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderDashHelpBar renders the updated help bar with leader key info.
func renderDashHelpBar(theme *Theme) string {
	return theme.HelpBar.Render("[Ctrl+Space] leader  [1]events  [2]mail  [3]merge  [4]costs  [Tab]cycle  [q]uit")
}
