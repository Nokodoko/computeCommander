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

// viewMode identifies which panel is currently displayed.
type viewMode int

const (
	viewStatus viewMode = iota
	viewMail
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

// Dashboard is the top-level TUI component that composes all sub-views
// and drives the bubbletea event loop.
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
		mode:     viewStatus,
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

// handleKey processes keyboard input.
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
	case "n":
		// Nudge: reserved for future implementation.
	case "i":
		// Inspect: reserved for future implementation.
	}
	return d, nil
}

// View renders the full dashboard layout.
func (d *Dashboard) View() string {
	var sections []string

	// Header.
	header := d.theme.Title.Render("ComputeCommander Dashboard")
	sections = append(sections, header)

	// Main content based on current mode.
	switch d.mode {
	case viewStatus:
		sections = append(sections, d.agents.View())
	case viewMail:
		sections = append(sections, d.mail.View())
	case viewCosts:
		sections = append(sections, d.costs.View())
	}

	// Merge queue compact view (always visible in status mode).
	if d.mode == viewStatus {
		sections = append(sections, d.queue.View())
	}

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

	// Help bar.
	sections = append(sections, renderHelpBar(d.theme))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
