package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/mail"
	"github.com/noko/computecommander/internal/merge"
	"github.com/noko/computecommander/internal/platform/db"
)

// DefaultAgentCommand is the fallback agent command when no config or CLI override is set.
const DefaultAgentCommand = "claude --dangerously-skip-permissions --no-chrome --disallowedTools WebSearch WebFetch NotebookEdit"

// DashboardOpts groups the dependencies for constructing a Dashboard.
type DashboardOpts struct {
	Lister             SessionLister
	Mail               mail.MailStore
	Queue              merge.MergeQueue
	Config             *config.Config
	DB                 db.DB         // database handle for staleness reaping (nil-safe)
	Interval           time.Duration
	AgentCmd           string // CLI override for agent command
	ProjectName        string // display name for the active project (title bar)
	ProjectID          string // project ID for filtering
	AgentColorResolver AgentColorResolver // resolves agent names to color hex strings
	JiraLister         JiraLister // optional; nil disables the Jira pane
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
	// Sub-views.
	filePicker   *FilePicker
	agentSession *AgentSession
	agents       *AgentTable
	eventsPane   *EventsPane
	mail         *MailSummary
	queue        *MergeQueueView
	gitStatus    *GitStatusPane
	evals        *EvalsPane
	jira         *JiraPane
	costs        *CostTracker
	palette      *CommandPalette
	lazygit      *LazyGitPane   // PTY: lazygit process
	openbrain    *OpenBrainPane // Data: OpenBrain placeholder

	// State.
	focusedPane    PaneID
	interval       time.Duration
	theme          *Theme
	width          int
	height         int
	ctx            context.Context
	err            error
	agentCmd       string
	projectName    string
	projectID      string
	db             db.DB     // database for staleness reaping (nil-safe)
	dbPath         string    // SQLite DB file path for fsnotify watching
	lastReapTime   time.Time // when we last ran the staleness reaper
	staleThreshold time.Duration
}

// NewDashboard constructs a Dashboard from the provided options.
func NewDashboard(opts DashboardOpts) *Dashboard {
	theme := DefaultTheme()
	interval := opts.Interval
	if interval == 0 {
		interval = 2 * time.Second
	}

	// Resolve agent command: CLI flag > config > default.
	agentCmd := opts.AgentCmd
	if agentCmd == "" && opts.Config != nil && opts.Config.Agents.DefaultCommand != "" {
		agentCmd = opts.Config.Agents.DefaultCommand
	}
	if agentCmd == "" {
		agentCmd = DefaultAgentCommand
	}

	// Determine project root.
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}

	// Resolve project name from opts or config.
	projectName := opts.ProjectName
	if projectName == "" && opts.Config != nil {
		projectName = opts.Config.Project.Name
	}

	fp := NewFilePicker(root, theme)
	if projectName != "" {
		fp.SetProject(projectName, opts.ProjectID)
	}

	// Resolve stale threshold from config, defaulting to 10 minutes.
	staleThreshold := 10 * time.Minute
	if opts.Config != nil && opts.Config.Watchdog.StaleThresholdMs > 0 {
		staleThreshold = time.Duration(opts.Config.Watchdog.StaleThresholdMs) * time.Millisecond
	}

	eventsPane := NewEventsPane(theme)
	if opts.DB != nil {
		eventsPane.SetDB(opts.DB)
	}

	d := &Dashboard{
		filePicker:     fp,
		agentSession:   NewAgentSession(agentCmd, theme),
		agents:         NewAgentTable(opts.Lister, theme),
		eventsPane:     eventsPane,
		mail:           NewMailSummary(opts.Mail, theme),
		queue:          NewMergeQueueView(opts.Queue, theme),
		gitStatus:      NewGitStatusPane(theme),
		evals:          NewEvalsPane(opts.DB, theme),
		jira:           NewJiraPane(opts.JiraLister, theme),
		costs:          NewCostTracker(theme),
		palette:        NewCommandPalette(theme),
		lazygit:        NewLazyGitPane(root, theme),
		openbrain:      NewOpenBrainPane(theme),
		focusedPane:    PaneAgentSession,
		interval:       interval,
		theme:          theme,
		agentCmd:       agentCmd,
		projectName:    projectName,
		projectID:      opts.ProjectID,
		db:             opts.DB,
		dbPath:         dbPathFromConfig(opts.Config),
		staleThreshold: staleThreshold,
	}

	// Wire agent color resolver into all components that support it.
	if opts.AgentColorResolver != nil {
		d.eventsPane.SetColorResolver(opts.AgentColorResolver)
		d.mail.SetColorResolver(opts.AgentColorResolver)
		d.queue.SetColorResolver(opts.AgentColorResolver)
		d.costs.SetColorResolver(opts.AgentColorResolver)
	}

	return d
}

// Run starts the bubbletea program and blocks until the user quits.
func (d *Dashboard) Run(ctx context.Context) error {
	d.ctx = ctx

	// Write PID file so cmdr-bridge.sh can signal us for instant refresh.
	// Perform an initial refresh before starting.
	if err := d.Refresh(); err != nil {
		d.err = err
	}

	p := tea.NewProgram(d, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Watch the SQLite DB file with fsnotify for instant refresh.
	// When cmdr-bridge.sh writes agent state to the DB, fsnotify fires
	// and we forward a signalRefreshMsg to bubbletea for immediate re-render.
	dbWatcher := d.watchDBForRefresh(p)
	defer func() {
		if dbWatcher != nil {
			dbWatcher.Close()
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	select {
	case err := <-done:
		_ = d.agentSession.Stop()
		_ = d.filePicker.Stop()
		_ = d.lazygit.Stop()
		return err
	case <-ctx.Done():
		p.Quit()
		_ = d.agentSession.Stop()
		_ = d.filePicker.Stop()
		_ = d.lazygit.Stop()
		return ctx.Err()
	}
}

// dbPathFromConfig extracts the SQLite DB path from config, or returns empty string.
func dbPathFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Database.SQLite.Path
}

// watchDBForRefresh sets up an fsnotify watcher on the SQLite DB file.
// Sends signalRefreshMsg to the bubbletea program whenever the DB is modified.
// Returns the watcher (caller must Close) or nil if watching is not possible.
func (d *Dashboard) watchDBForRefresh(p *tea.Program) *fsnotify.Watcher {
	if d.dbPath == "" {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}

	dbDir := filepath.Dir(d.dbPath)
	if err := watcher.Add(dbDir); err != nil {
		watcher.Close()
		return nil
	}

	dbBase := filepath.Base(d.dbPath)
	walBase := dbBase + "-wal"

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				name := filepath.Base(event.Name)
				if name != dbBase && name != walBase {
					continue
				}
				p.Send(signalRefreshMsg{})
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return watcher
}

// Refresh updates all sub-views with the latest data.
// It also periodically reaps stale sessions from the database -- sessions
// whose process has died without the SubagentStop hook updating their state.
func (d *Dashboard) Refresh() error {
	ctx := d.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Run the staleness reaper every 30 seconds to mark ghost sessions as completed.
	// This is the safety net for missed SubagentStop hook calls and mirrors the
	// reaper in internal/commands/status.go's runStatusPane().
	const reapInterval = 30 * time.Second
	if d.db != nil && (d.lastReapTime.IsZero() || time.Since(d.lastReapTime) >= reapInterval) {
		d.reapStaleSessions(ctx)
		d.lastReapTime = time.Now()
	}

	var errs []string

	if err := d.agents.Refresh(ctx); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.eventsPane.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.mail.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.queue.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.filePicker.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.gitStatus.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.evals.Refresh(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := d.jira.Refresh(ctx); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("refresh errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// reapStaleSessions marks working/booting sessions as completed when their
// last_activity exceeds the stale threshold. This prevents ghost entries from
// lingering when the SubagentStop hook fails to update the database (e.g. the
// agent process was killed or the hook crashed).
func (d *Dashboard) reapStaleSessions(ctx context.Context) {
	if d.db == nil {
		return
	}
	cutoff := time.Now().Add(-d.staleThreshold).UTC().Format("2006-01-02T15:04:05Z")
	_ = d.db.Exec(ctx,
		"UPDATE sessions SET state = 'completed', last_activity = $1 WHERE state IN ('working', 'booting') AND last_activity < $2",
		time.Now().UTC().Format("2006-01-02T15:04:05Z"), cutoff,
	)
}

// --- bubbletea.Model implementation ---

// tickMsg triggers a periodic refresh.
type tickMsg time.Time

// ptyOutputMsg signals that a PTY pane has new output ready to render.
// The bubbletea event loop re-renders immediately when this arrives,
// rather than waiting for the next tick.
type ptyOutputMsg struct{}

// signalRefreshMsg is sent when a SIGUSR1 signal is received, triggering
// an immediate data refresh. This allows cmdr-bridge.sh to notify the
// dashboard of agent state changes without waiting for the tick interval.
type signalRefreshMsg struct{}

// Init sets up the initial command, starting the refresh ticker and
// PTY output listeners.
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(d.interval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
		d.waitForPTYOutput(),
	)
}

// waitForPTYOutput returns a tea.Cmd that blocks until either the
// agent session or file picker has new PTY output, then sends a
// ptyOutputMsg to trigger a re-render.
func (d *Dashboard) waitForPTYOutput() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-d.agentSession.Notify():
		case <-d.filePicker.Notify():
		case <-d.lazygit.Notify():
		}
		return ptyOutputMsg{}
	}
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
	case signalRefreshMsg:
		// SIGUSR1 received from cmdr-bridge.sh — instant data refresh.
		d.err = d.Refresh()
		return d, nil
	case ptyOutputMsg:
		// PTY output arrived — re-render and re-subscribe for the next signal.
		return d, d.waitForPTYOutput()
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height
		d.updatePaneSizes()
		return d, nil
	}
	return d, nil
}

// handleKey processes keyboard input, including leader key support.
func (d *Dashboard) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Command palette takes priority when visible.
	if d.palette.Visible() {
		return d.handlePaletteKey(msg)
	}

	// PTY panes get raw input when focused and running (except control keys).
	if d.focusedPane == PaneAgentSession && d.agentSession.Running() {
		return d.forwardToPTY(msg, func(data []byte) { _ = d.agentSession.WriteInput(data) })
	}
	if d.focusedPane == PaneFilePicker && d.filePicker.Running() {
		return d.forwardToPTY(msg, func(data []byte) { _ = d.filePicker.WriteInput(data) })
	}
	if d.focusedPane == PaneLazyGit && d.lazygit.Running() {
		return d.forwardToPTY(msg, func(data []byte) { _ = d.lazygit.WriteInput(data) })
	}

	switch key {
	case "q", "ctrl+c":
		return d, tea.Quit
	case "ctrl+k":
		d.palette.Open()
		d.palette.SetSize(d.width, d.height)
		return d, nil
	case "tab":
		d.focusedPane = nextPane(d.focusedPane)
	case "shift+tab":
		d.focusedPane = prevPane(d.focusedPane)
	case "1":
		d.focusedPane = PaneFilePicker
	case "2":
		d.focusedPane = PaneAgentSession
	case "3":
		d.focusedPane = PaneAgents
	case "4":
		d.focusedPane = PaneEvents
	case "5":
		d.focusedPane = PaneEvals
	case "6":
		d.focusedPane = PaneMergeQueue
	case "7":
		d.focusedPane = PaneOpenBrain
	case "9":
		d.focusedPane = PaneJira
	case "0":
		d.focusedPane = PaneLazyGit
	default:
		d.handleFocusedPaneKey(msg)
	}
	return d, nil
}

// handlePaletteKey routes keys to the command palette.
func (d *Dashboard) handlePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+k":
		d.palette.Close()
	case "up", "ctrl+p":
		d.palette.CursorUp()
	case "down", "ctrl+n":
		d.palette.CursorDown()
	case "backspace":
		d.palette.Backspace()
	case "enter":
		if sel := d.palette.Selected(); sel != nil {
			d.executePaletteCommand(sel)
		}
		d.palette.Close()
	default:
		// Type character into search.
		if len(msg.String()) == 1 {
			d.palette.TypeChar(rune(msg.String()[0]))
		}
	}
	return d, nil
}

// executePaletteCommand dispatches a palette command.
func (d *Dashboard) executePaletteCommand(cmd *PaletteCommand) {
	switch cmd.Name {
	case "kill":
		_ = d.agentSession.Stop()
	case "background":
		// Detach focus from the agent session pane so it keeps running
		// in the background while the user interacts with other panes.
		if d.focusedPane == PaneAgentSession {
			d.focusedPane = PaneAgents
		}
	case "prompt":
		// Send a newline to the agent to prompt it, useful for nudging
		// an agent that is waiting for input.
		if d.agentSession.Running() {
			_ = d.agentSession.WriteInput([]byte("\n"))
		}
	case "restart":
		_ = d.agentSession.Stop()
		w, h := d.agentSessionDimensions()
		_ = d.agentSession.Start(w, h)
	}
	// Other commands can be wired later.
}

// handleFocusedPaneKey routes keys to the focused pane's internal navigation.
func (d *Dashboard) handleFocusedPaneKey(msg tea.KeyMsg) {
	key := msg.String()
	switch d.focusedPane {
	case PaneFilePicker:
		if key == "enter" && !d.filePicker.Running() {
			w, h := d.filePickerDimensions()
			if err := d.filePicker.Start(w, h); err != nil {
				d.filePicker.SetLastError(err.Error())
			}
		} else if d.filePicker.Running() {
			// Forward keystroke to fp process.
			_ = d.filePicker.WriteInput([]byte(keyToBytes(msg)))
		}
	case PaneAgentSession:
		if key == "enter" && !d.agentSession.Running() {
			w, h := d.agentSessionDimensions()
			if err := d.agentSession.Start(w, h); err != nil {
				d.err = fmt.Errorf("agent start: %w", err)
			}
		}
	case PaneAgents:
		switch key {
		case "j", "down":
			d.agents.CursorDown()
		case "k", "up":
			d.agents.CursorUp()
		}
	case PaneEvents:
		switch key {
		case "j", "down":
			d.eventsPane.ScrollDown()
		case "k", "up":
			d.eventsPane.ScrollUp()
		}
	case PaneEvals:
		switch key {
		case "j", "down":
			d.evals.ScrollDown()
		case "k", "up":
			d.evals.ScrollUp()
		case "r":
			_ = d.evals.RunAll()
		}
	case PaneJira:
		switch key {
		case "j", "down":
			d.jira.CursorDown()
		case "k", "up":
			d.jira.CursorUp()
		case "l", "enter", "right":
			d.jira.Expand()
		case "h", "left":
			d.jira.Collapse()
		case "?":
			d.jira.ToggleHelp()
		}
	case PaneLazyGit:
		if key == "enter" && !d.lazygit.Running() {
			w, h := d.lazyGitDimensions()
			if err := d.lazygit.Start(w, h); err != nil {
				d.err = fmt.Errorf("lazygit start: %w", err)
			}
		}
	}
}

// updatePaneSizes recalculates pane dimensions after a window resize.
// Inner dimensions subtract 2 for the border and 1 for the title row
// that RenderPane() reserves, giving height-3 usable content rows.
func (d *Dashboard) updatePaneSizes() {
	topH, bottomH, fpW, asW, agW, bottomPaneW := d.calculateLayout()

	// Top row.
	d.filePicker.SetSize(fpW-2, topH-3)
	d.agentSession.SetSize(asW-2, topH-3)
	// agents pane uses CompactView which gets width/height at render time.
	_ = agW

	// Bottom row -- ALL panes must receive SetSize.
	d.eventsPane.SetSize(bottomPaneW-2, bottomH-3)
	d.evals.SetSize(bottomPaneW-2, bottomH-3)
	d.queue.SetSize(bottomPaneW-2, bottomH-3)
	d.openbrain.SetSize(bottomPaneW-2, bottomH-3)
	d.lazygit.SetSize(bottomPaneW-2, bottomH-3)

	// Overlays.
	d.palette.SetSize(d.width, d.height)
}

// calculateLayout returns the dimensions for each section of the grid.
// Returns: topRowHeight, bottomRowHeight, filePickerWidth, agentSessionWidth, agentsWidth, bottomPaneWidth
func (d *Dashboard) calculateLayout() (int, int, int, int, int, int) {
	w := d.width
	h := d.height
	if w < 40 {
		w = 80
	}
	if h < 10 {
		h = 24
	}

	// Reserve 2 lines for status/help bars.
	usableH := h - 2

	topH := usableH * 70 / 100
	bottomH := usableH - topH
	if topH < 5 {
		topH = 5
	}
	if bottomH < 3 {
		bottomH = 3
	}

	fpW := w * 15 / 100
	agW := w * 20 / 100
	asW := w - fpW - agW
	if fpW < 20 {
		fpW = 20
	}
	if agW < 20 {
		agW = 20
	}
	if asW < 20 {
		asW = 20
	}

	bottomPaneW := w / 5

	return topH, bottomH, fpW, asW, agW, bottomPaneW
}

// agentSessionDimensions returns the inner dimensions of the agent session pane.
// Subtract 2 for left/right borders from width, and 3 for top/bottom borders
// plus the title row from height. This matches what RenderPane() provides as
// usable content area.
func (d *Dashboard) agentSessionDimensions() (int, int) {
	topH, _, _, asW, _, _ := d.calculateLayout()
	return asW - 2, topH - 3
}

// filePickerDimensions returns the inner dimensions of the file picker pane.
// Subtract 2 for left/right borders from width, and 3 for top/bottom borders
// plus the title row from height. This matches what RenderPane() provides as
// usable content area.
func (d *Dashboard) filePickerDimensions() (int, int) {
	topH, _, fpW, _, _, _ := d.calculateLayout()
	return fpW - 2, topH - 3
}

// lazyGitDimensions returns the inner dimensions of the lazygit pane.
func (d *Dashboard) lazyGitDimensions() (int, int) {
	_, bottomH, _, _, _, bottomPaneW := d.calculateLayout()
	return bottomPaneW - 2, bottomH - 3
}

// forwardToPTY handles keyboard input for a focused PTY pane. Only a small
// set of control-key chords are intercepted for dashboard navigation;
// everything else — including digits, printable characters, and arrow keys
// — is forwarded verbatim to the PTY via writeFn.
func (d *Dashboard) forwardToPTY(msg tea.KeyMsg, writeFn func([]byte)) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+k":
		d.palette.Open()
		d.palette.SetSize(d.width, d.height)
		return d, nil
	case "ctrl+c":
		return d, tea.Quit
	default:
		writeFn([]byte(keyToBytes(msg)))
		return d, nil
	}
}

// View renders the full dashboard layout.
// When PaneJira is focused it renders a full-screen Jira overlay instead of
// the normal grid so the issue tree has maximum space.
func (d *Dashboard) View() string {
	if d.focusedPane == PaneJira {
		return d.viewJira()
	}
	topH, bottomH, fpW, asW, agW, bottomPaneW := d.calculateLayout()

	// --- Top Row ---
	fpContent := d.filePicker.View()
	fpMeta := paneMetaByID(PaneFilePicker)
	filePicker := RenderPane(fpContent, fpMeta, d.focusedPane == PaneFilePicker, fpW, topH, d.theme)

	asContent := d.agentSession.View()
	asMeta := paneMetaByID(PaneAgentSession)
	agentSession := RenderPane(asContent, asMeta, d.focusedPane == PaneAgentSession, asW, topH, d.theme)

	agContent := d.agents.CompactView(agW-2, topH-3)
	agMeta := paneMetaByID(PaneAgents)
	agentsPane := RenderPane(agContent, agMeta, d.focusedPane == PaneAgents, agW, topH, d.theme)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, filePicker, agentSession, agentsPane)

	// --- Bottom Row ---
	// Events | Evals | MergeQueue | OpenBrain | LazyGit
	evContent := d.eventsPane.View()
	evMeta := paneMetaByID(PaneEvents)
	eventsPane := RenderPane(evContent, evMeta, d.focusedPane == PaneEvents, bottomPaneW, bottomH, d.theme)

	evalsContent := d.evals.View()
	evalsMeta := paneMetaByID(PaneEvals)
	evalsPane := RenderPane(evalsContent, evalsMeta, d.focusedPane == PaneEvals, bottomPaneW, bottomH, d.theme)

	mqContent := d.queue.View()
	mqMeta := paneMetaByID(PaneMergeQueue)
	mergePane := RenderPane(mqContent, mqMeta, d.focusedPane == PaneMergeQueue, bottomPaneW, bottomH, d.theme)

	obContent := d.openbrain.View()
	obMeta := paneMetaByID(PaneOpenBrain)
	openbrainPane := RenderPane(obContent, obMeta, d.focusedPane == PaneOpenBrain, bottomPaneW, bottomH, d.theme)

	// LazyGit pane gets remaining width (last pane in row).
	lastPaneW := d.width - (bottomPaneW * 4)
	if lastPaneW < 10 {
		lastPaneW = bottomPaneW
	}
	lgContent := d.lazygit.View()
	lgMeta := paneMetaByID(PaneLazyGit)
	lazygitPane := RenderPane(lgContent, lgMeta, d.focusedPane == PaneLazyGit, lastPaneW, bottomH, d.theme)

	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, eventsPane, evalsPane, mergePane, openbrainPane, lazygitPane)

	// --- Status and help bars ---
	statusBar := renderStatusBarWithProject(
		d.projectName,
		len(d.agents.Sessions()),
		d.mail.UnreadCount(),
		d.queue.PendingCount(),
		d.costs.TotalCost(),
		d.theme,
	)

	// --- Compose full layout ---
	var sections []string
	sections = append(sections, topRow)
	sections = append(sections, bottomRow)
	sections = append(sections, statusBar)

	// Error display.
	if d.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
		sections = append(sections, errStyle.Render("Error: "+d.err.Error()))
	}

	sections = append(sections, renderHelpBar(d.theme))

	layout := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Overlay command palette if visible.
	if d.palette.Visible() {
		paletteView := d.palette.View()
		layout = d.overlayPalette(layout, paletteView)
	}

	return layout
}

// viewJira renders a full-screen Jira pane (active when key 9 is pressed).
func (d *Dashboard) viewJira() string {
	w := d.width
	h := d.height
	if w < 40 {
		w = 80
	}
	if h < 10 {
		h = 24
	}

	// Reserve 2 lines for status/help bars.
	innerH := h - 2
	d.jira.SetSize(w-2, innerH-3)

	jiraMeta := paneMetaByID(PaneJira)
	content := d.jira.View()
	jiraPane := RenderPane(content, jiraMeta, true, w, innerH, d.theme)

	statusBar := renderStatusBarWithProject(
		d.projectName,
		len(d.agents.Sessions()),
		d.mail.UnreadCount(),
		d.queue.PendingCount(),
		d.costs.TotalCost(),
		d.theme,
	)

	helpBar := d.theme.HelpBar.Render("9=jira  j/k:nav  l/h:expand  ?:help  Tab:back  q:quit")

	return lipgloss.JoinVertical(lipgloss.Left, jiraPane, statusBar, helpBar)
}

// overlayPalette centers the command palette over the dashboard.
func (d *Dashboard) overlayPalette(background, palette string) string {
	bgLines := strings.Split(background, "\n")
	palLines := strings.Split(palette, "\n")

	// Center vertically.
	startY := (len(bgLines) - len(palLines)) / 2
	if startY < 0 {
		startY = 0
	}

	// Center horizontally.
	palMaxW := 0
	for _, l := range palLines {
		if len(l) > palMaxW {
			palMaxW = len(l)
		}
	}
	startX := (d.width - palMaxW) / 2
	if startX < 0 {
		startX = 0
	}

	// Overlay palette lines onto background.
	for i, pl := range palLines {
		y := startY + i
		if y >= len(bgLines) {
			break
		}
		line := bgLines[y]
		// Pad line to at least startX + len(pl).
		for len(line) < startX+len(pl) {
			line += " "
		}
		bgLines[y] = line[:startX] + pl + line[startX+len(pl):]
	}

	return strings.Join(bgLines, "\n")
}

// keyToBytes converts a bubbletea key message to raw bytes for PTY forwarding.
func keyToBytes(msg tea.KeyMsg) string {
	switch msg.Type {
	case tea.KeyEnter:
		return "\r"
	case tea.KeyBackspace:
		return "\x7f"
	case tea.KeyEscape:
		return "\x1b"
	case tea.KeyUp:
		return "\x1b[A"
	case tea.KeyDown:
		return "\x1b[B"
	case tea.KeyRight:
		return "\x1b[C"
	case tea.KeyLeft:
		return "\x1b[D"
	case tea.KeySpace:
		return " "
	case tea.KeyTab:
		return "\t"
	default:
		// For regular characters, use the string representation.
		s := msg.String()
		if len(s) == 1 {
			return s
		}
		// Control characters.
		if strings.HasPrefix(s, "ctrl+") && len(s) == 6 {
			ch := s[5]
			return string(rune(ch - 'a' + 1))
		}
		return s
	}
}
