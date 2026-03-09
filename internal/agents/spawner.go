package agents

import (
	"context"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/internal/wezterm"
	"github.com/noko/computecommander/internal/worktree"
	"github.com/noko/computecommander/internal/zellij"
	"github.com/noko/computecommander/pkg/runtimes"
)

// RuntimeRegistry looks up runtimes by string ID.
type RuntimeRegistry func(id string) (runtimes.AgentRuntime, error)

// IDGenerator produces unique session IDs.
type IDGenerator func() string

// SpawnerOpts configures a Spawner.
type SpawnerOpts struct {
	DB              db.DB
	PaneManager     zellij.PaneManager
	WindowManager   wezterm.WindowManager // nil = spawn panes in current session
	WorktreeManager worktree.WorktreeManager
	GetRuntime      RuntimeRegistry
	GenerateID      IDGenerator
	WorktreeBaseDir string
	MaxDepth        int
	MaxConcurrent   int
	ZellijLayout    string // layout file path for dashboard (legacy, unused when CmdrBinary is set)
	SessionPrefix   string // prefix for zellij session names
	CmdrBinary      string // absolute path to the cmdr binary for dashboard --tui
}

// Spawner manages agent lifecycle: spawn, stop, and list.
type Spawner struct {
	db              db.DB
	panes           zellij.PaneManager
	windows         wezterm.WindowManager
	worktrees       worktree.WorktreeManager
	getRuntime      RuntimeRegistry
	generateID      IDGenerator
	worktreeBaseDir string
	maxDepth        int
	maxConcurrent   int
	zellijLayout    string
	sessionPrefix   string
	cmdrBinary      string
}

// NewSpawner creates a Spawner from the provided options.
func NewSpawner(opts SpawnerOpts) *Spawner {
	genID := opts.GenerateID
	if genID == nil {
		genID = defaultIDGenerator
	}
	maxDepth := opts.MaxDepth
	if maxDepth == 0 {
		maxDepth = 2
	}
	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = 10
	}
	sessionPrefix := opts.SessionPrefix
	if sessionPrefix == "" {
		sessionPrefix = "cc"
	}
	return &Spawner{
		db:              opts.DB,
		panes:           opts.PaneManager,
		windows:         opts.WindowManager,
		worktrees:       opts.WorktreeManager,
		getRuntime:      opts.GetRuntime,
		generateID:      genID,
		worktreeBaseDir: opts.WorktreeBaseDir,
		maxDepth:        maxDepth,
		maxConcurrent:   maxConcurrent,
		zellijLayout:    opts.ZellijLayout,
		sessionPrefix:   sessionPrefix,
		cmdrBinary:      opts.CmdrBinary,
	}
}

// SpawnDashboard creates the cmdr dashboard in a new wezterm window running
// a zellij session with the multi-pane KDL dashboard layout.
func (s *Spawner) SpawnDashboard(ctx context.Context) error {
	if s.windows == nil {
		return fmt.Errorf("WindowManager not configured; set zellij.terminal=wezterm in config")
	}

	sessionName := fmt.Sprintf("%s-dashboard", s.sessionPrefix)

	return s.windows.SpawnWindow(ctx, wezterm.SpawnWindowOpts{
		ZellijSession: sessionName,
		Layout:        s.zellijLayout,
	})
}

// Spawn creates a new agent session following the spawning model (spec 3.1.1):
//  1. Validate the request
//  2. Create a git worktree with a unique branch
//  3. Deploy runtime config (overlay + hooks) into the worktree
//  4. Create a floating Zellij pane for the agent (within existing session)
//  5. Register the session in the database
func (s *Spawner) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResult, error) {
	if err := s.validateSpawnRequest(req); err != nil {
		return nil, fmt.Errorf("spawn validate: %w", err)
	}

	// Resolve the runtime adapter.
	rt, err := s.getRuntime(string(req.Runtime))
	if err != nil {
		return nil, fmt.Errorf("spawn resolve runtime: %w", err)
	}

	// Generate identifiers.
	sessionID := s.generateID()
	branchName := fmt.Sprintf("cc/%s/%s", req.Name, sessionID[:8])
	zellijSession := fmt.Sprintf("%s-%s", s.sessionPrefix, req.Name)

	// 1. Create worktree.
	wt, err := s.worktrees.Create(worktree.CreateOpts{
		Branch:  branchName,
		Agent:   req.Name,
		TaskID:  req.TaskID,
		BaseDir: s.worktreeBaseDir,
	})
	if err != nil {
		return nil, fmt.Errorf("spawn create worktree: %w", err)
	}

	// 2. Build overlay and deploy config.
	overlay, err := BuildOverlay(req.Capability, req.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("spawn build overlay: %w", err)
	}

	hooksDef := &runtimes.HooksDef{
		AgentName:    req.Name,
		Capability:   string(req.Capability),
		WorktreePath: wt.Path,
		FileScope:    req.FileScope,
	}

	if err := rt.DeployConfig(ctx, wt.Path, overlay, hooksDef); err != nil {
		// Clean up worktree on deploy failure.
		_ = s.worktrees.Remove(wt.Path, true)
		return nil, fmt.Errorf("spawn deploy config: %w", err)
	}

	// 3. Build the agent spawn command.
	spawnCmd := rt.BuildSpawnCommand(runtimes.SpawnOpts{
		WorkDir:        wt.Path,
		PermissionMode: "bypass",
	})

	// 4. Create a floating zellij pane for the agent.
	// Agents spawn as floating panes within the dashboard session.
	pane, err := s.panes.CreatePane(zellij.CreatePaneOpts{
		Name:     req.Name,
		WorkDir:  wt.Path,
		Command:  []string{"sh", "-c", spawnCmd},
		Floating: true, // Agents spawn as floating panes
	})
	if err != nil {
		_ = s.worktrees.Remove(wt.Path, true)
		return nil, fmt.Errorf("spawn create pane: %w", err)
	}

	// 5. Assign color from palette (round-robin by spawn index within the run).
	spawnIndex, err := s.countSessionsInRun(ctx, req.Name)
	if err != nil {
		spawnIndex = 0
	}
	agentColor := AssignColor(spawnIndex)

	// 6. Register session in database.
	now := time.Now()
	session := &AgentSession{
		ID:              sessionID,
		AgentName:       req.Name,
		Capability:      req.Capability,
		WorktreePath:    wt.Path,
		BranchName:      branchName,
		TaskID:          req.TaskID,
		ZellijPane:      pane.ID,
		ZellijSession:   zellijSession,
		State:           StateBooting,
		PID:             0,
		ParentAgent:     req.Parent,
		Depth:           req.Depth,
		StartedAt:       now,
		LastActivity:    now,
		EscalationLevel: 0,
		TranscriptPath:  fmt.Sprintf("%s/.transcript", wt.Path),
		Runtime:         req.Runtime,
		ProjectID:       req.ProjectID,
		ColorIndex:      agentColor.Index,
		ColorHex:        agentColor.Hex,
	}

	if err := s.insertSession(ctx, session); err != nil {
		_ = s.panes.ClosePane(pane.ID)
		_ = s.worktrees.Remove(wt.Path, true)
		return nil, fmt.Errorf("spawn insert session: %w", err)
	}

	return &SpawnResult{
		Session:       session,
		WorktreePath:  wt.Path,
		ZellijPane:    pane.ID,
		ZellijSession: zellijSession,
		PID:           session.PID,
	}, nil
}

// Stop terminates an agent session by name.
func (s *Spawner) Stop(ctx context.Context, agentName string, opts StopOpts) error {
	session, err := s.findSessionByName(ctx, agentName)
	if err != nil {
		return fmt.Errorf("stop find session: %w", err)
	}

	// Close the Zellij pane.
	if session.ZellijPane != "" {
		if err := s.panes.ClosePane(session.ZellijPane); err != nil {
			// Log but continue cleanup.
			_ = err
		}
	}

	// Update session state.
	newState := StateCompleted
	if opts.Force {
		newState = StateZombie
	}

	if err := s.updateSessionState(ctx, session.ID, newState); err != nil {
		return fmt.Errorf("stop update state: %w", err)
	}

	return nil
}

// ListSessions returns agent sessions filtered by the provided options.
// Token usage is aggregated from the metrics table via LEFT JOIN.
// The query gracefully handles databases that haven't been migrated to v2
// (missing color_index, color_hex, project_id columns) by falling back to
// a base query without those columns.
func (s *Spawner) ListSessions(ctx context.Context, opts ListOpts) ([]*AgentSession, error) {
	// Detect whether the v2 columns exist. We cache this per-call since
	// migrations could run between calls, but column detection is cheap.
	hasV2 := s.hasV2Columns(ctx)

	var selectCols string
	if hasV2 {
		selectCols = `s.id, s.agent_name, s.capability, s.worktree_path, s.branch_name,
		s.task_id, s.zellij_pane, s.state, s.pid, s.parent_agent, s.depth, s.run_id,
		s.started_at, s.last_activity, s.escalation_level, s.stalled_since,
		s.transcript_path, s.runtime,
		COALESCE(m.total_in, 0), COALESCE(m.total_out, 0),
		COALESCE(s.color_index, 0), COALESCE(s.color_hex, ''), COALESCE(s.project_id, '')`
	} else {
		selectCols = `s.id, s.agent_name, s.capability, s.worktree_path, s.branch_name,
		s.task_id, s.zellij_pane, s.state, s.pid, s.parent_agent, s.depth, s.run_id,
		s.started_at, s.last_activity, s.escalation_level, s.stalled_since,
		s.transcript_path, s.runtime,
		COALESCE(m.total_in, 0), COALESCE(m.total_out, 0)`
	}

	query := fmt.Sprintf(`SELECT %s
	FROM sessions s
	LEFT JOIN (
		SELECT agent_name, SUM(input_tokens) AS total_in, SUM(output_tokens) AS total_out
		FROM metrics GROUP BY agent_name
	) m ON s.agent_name = m.agent_name
	WHERE 1=1`, selectCols)

	var args []any
	argIdx := 1

	if opts.RunID != "" {
		query += fmt.Sprintf(" AND s.run_id = $%d", argIdx)
		args = append(args, opts.RunID)
		argIdx++
	}
	if opts.Capability != "" {
		query += fmt.Sprintf(" AND s.capability = $%d", argIdx)
		args = append(args, string(opts.Capability))
		argIdx++
	}
	if opts.State != "" {
		query += fmt.Sprintf(" AND s.state = $%d", argIdx)
		args = append(args, string(opts.State))
		argIdx++
	}
	if opts.Parent != "" {
		query += fmt.Sprintf(" AND s.parent_agent = $%d", argIdx)
		args = append(args, opts.Parent)
		argIdx++
	}
	if opts.ProjectID != "" && hasV2 {
		query += fmt.Sprintf(" AND s.project_id = $%d", argIdx)
		args = append(args, opts.ProjectID)
		argIdx++
	}

	query += " ORDER BY s.started_at DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions query: %w", err)
	}
	defer rows.Close()

	var sessions []*AgentSession
	for rows.Next() {
		sess := &AgentSession{}
		var scanErr error
		if hasV2 {
			scanErr = rows.Scan(
				&sess.ID, &sess.AgentName, &sess.Capability,
				&sess.WorktreePath, &sess.BranchName, &sess.TaskID,
				&sess.ZellijPane, &sess.State, &sess.PID,
				&sess.ParentAgent, &sess.Depth, &sess.RunID,
				&sess.StartedAt, &sess.LastActivity, &sess.EscalationLevel,
				&sess.StalledSince, &sess.TranscriptPath, &sess.Runtime,
				&sess.InputTokens, &sess.OutputTokens,
				&sess.ColorIndex, &sess.ColorHex, &sess.ProjectID,
			)
		} else {
			scanErr = rows.Scan(
				&sess.ID, &sess.AgentName, &sess.Capability,
				&sess.WorktreePath, &sess.BranchName, &sess.TaskID,
				&sess.ZellijPane, &sess.State, &sess.PID,
				&sess.ParentAgent, &sess.Depth, &sess.RunID,
				&sess.StartedAt, &sess.LastActivity, &sess.EscalationLevel,
				&sess.StalledSince, &sess.TranscriptPath, &sess.Runtime,
				&sess.InputTokens, &sess.OutputTokens,
			)
			// Set defaults for missing v2 fields.
			sess.ColorIndex = 0
			sess.ColorHex = ""
			sess.ProjectID = ""
		}
		if scanErr != nil {
			return nil, fmt.Errorf("list sessions scan: %w", scanErr)
		}
		sessions = append(sessions, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions rows: %w", err)
	}

	return sessions, nil
}

// hasV2Columns checks whether the sessions table has the v2 schema columns
// (color_index, color_hex, project_id) added by migration 002_system_wide.
// Returns false for pre-migration databases so queries can adapt gracefully.
func (s *Spawner) hasV2Columns(ctx context.Context) bool {
	// Use a lightweight probe: try to select the v2 columns from a LIMIT 0 query.
	// If the columns don't exist, the query will fail.
	rows, err := s.db.Query(ctx, "SELECT color_index FROM sessions LIMIT 0")
	if err != nil {
		return false
	}
	rows.Close()
	return true
}

// validateSpawnRequest checks the request fields.
func (s *Spawner) validateSpawnRequest(req SpawnRequest) error {
	if req.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if !ValidCapability(req.Capability) {
		return fmt.Errorf("invalid capability: %q", req.Capability)
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Runtime == "" {
		return fmt.Errorf("runtime is required")
	}
	if req.Depth > s.maxDepth {
		return fmt.Errorf("depth %d exceeds max depth %d", req.Depth, s.maxDepth)
	}
	return nil
}

// insertSession writes a new session record.
func (s *Spawner) insertSession(ctx context.Context, sess *AgentSession) error {
	err := s.db.Exec(ctx,
		`INSERT INTO sessions (id, agent_name, capability, worktree_path, branch_name,
			task_id, zellij_pane, state, pid, parent_agent, depth, run_id,
			started_at, last_activity, escalation_level, stalled_since,
			transcript_path, runtime, project_id, color_index, color_hex)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		sess.ID, sess.AgentName, string(sess.Capability), sess.WorktreePath,
		sess.BranchName, sess.TaskID, sess.ZellijPane, string(sess.State),
		sess.PID, sess.ParentAgent, sess.Depth, sess.RunID,
		sess.StartedAt, sess.LastActivity, sess.EscalationLevel,
		sess.StalledSince, sess.TranscriptPath, string(sess.Runtime),
		sess.ProjectID, sess.ColorIndex, sess.ColorHex,
	)
	if err != nil {
		return err
	}

	// Also write to agent_colors table for color history.
	if sess.RunID != "" {
		_ = s.db.Exec(ctx,
			`INSERT OR IGNORE INTO agent_colors (agent_name, run_id, color_index, color_hex) VALUES ($1, $2, $3, $4)`,
			sess.AgentName, sess.RunID, sess.ColorIndex, sess.ColorHex,
		)
	}
	return nil
}

// countSessionsInRun returns the next color index for a new agent.
// Finds the lowest palette index not currently in use by any active session,
// so parallel agents always get distinct colors regardless of completion order.
func (s *Spawner) countSessionsInRun(ctx context.Context, _ string) (int, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT color_index FROM sessions WHERE state NOT IN ('completed', 'zombie')`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	used := make(map[int]bool)
	for rows.Next() {
		var idx int
		if scanErr := rows.Scan(&idx); scanErr == nil {
			used[idx] = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// Find the lowest unused palette slot; wrap after one full cycle.
	for i := 0; i < PaletteSize; i++ {
		if !used[i] {
			return i, nil
		}
	}
	// All palette slots occupied — pick next index past the highest used.
	return len(used) % PaletteSize, nil
}

// findSessionByName locates a session by agent name.
func (s *Spawner) findSessionByName(ctx context.Context, name string) (*AgentSession, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, agent_name, capability, worktree_path, branch_name, task_id,
			zellij_pane, state, pid, parent_agent, depth, run_id,
			started_at, last_activity, escalation_level, stalled_since,
			transcript_path, runtime
		FROM sessions WHERE agent_name = $1 AND state NOT IN ('completed', 'zombie')
		ORDER BY started_at DESC LIMIT 1`, name)

	sess := &AgentSession{}
	if err := row.Scan(
		&sess.ID, &sess.AgentName, &sess.Capability,
		&sess.WorktreePath, &sess.BranchName, &sess.TaskID,
		&sess.ZellijPane, &sess.State, &sess.PID,
		&sess.ParentAgent, &sess.Depth, &sess.RunID,
		&sess.StartedAt, &sess.LastActivity, &sess.EscalationLevel,
		&sess.StalledSince, &sess.TranscriptPath, &sess.Runtime,
	); err != nil {
		return nil, fmt.Errorf("session not found for agent %q: %w", name, err)
	}
	return sess, nil
}

// updateSessionState sets the state for a session.
func (s *Spawner) updateSessionState(ctx context.Context, sessionID string, state SessionState) error {
	return s.db.Exec(ctx,
		"UPDATE sessions SET state = $1, last_activity = $2 WHERE id = $3",
		string(state), time.Now(), sessionID,
	)
}

// LookupAgentColor returns the color hex assigned to the given agent name.
// It first checks active sessions, then falls back to the agent_colors history table.
// Returns empty string if no color is found.
func (s *Spawner) LookupAgentColor(ctx context.Context, agentName string) string {
	// Check active sessions first.
	var hex string
	row := s.db.QueryRow(ctx,
		`SELECT color_hex FROM sessions WHERE agent_name = $1 AND color_hex != '' ORDER BY started_at DESC LIMIT 1`,
		agentName)
	if err := row.Scan(&hex); err == nil && hex != "" {
		return hex
	}

	// Fall back to agent_colors history.
	row = s.db.QueryRow(ctx,
		`SELECT color_hex FROM agent_colors WHERE agent_name = $1 ORDER BY rowid DESC LIMIT 1`,
		agentName)
	if err := row.Scan(&hex); err == nil {
		return hex
	}

	return ""
}

// BuildColorResolver returns an AgentColorResolver function that maps agent names to color hex strings.
// This is suitable for passing to TUI components and CLI renderers.
func (s *Spawner) BuildColorResolver(ctx context.Context) func(string) string {
	// Build a cache from all known sessions to avoid per-name queries.
	cache := make(map[string]string)
	rows, err := s.db.Query(ctx,
		`SELECT agent_name, color_hex FROM sessions WHERE color_hex != '' ORDER BY started_at ASC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, hex string
			if err := rows.Scan(&name, &hex); err == nil {
				cache[name] = hex
			}
		}
	}

	// Supplement with agent_colors history for agents not in active sessions.
	rows2, err := s.db.Query(ctx,
		`SELECT agent_name, color_hex FROM agent_colors`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var name, hex string
			if err := rows2.Scan(&name, &hex); err == nil {
				if _, ok := cache[name]; !ok {
					cache[name] = hex
				}
			}
		}
	}

	return func(agentName string) string {
		return cache[agentName]
	}
}

// defaultIDGenerator produces a timestamp-based unique ID.
func defaultIDGenerator() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}
