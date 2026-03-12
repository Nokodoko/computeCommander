package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noko/computecommander/internal/config"
	"github.com/noko/computecommander/internal/tui"
	"github.com/noko/computecommander/pkg/integrations/jira"
	"github.com/spf13/cobra"
)

// JiraCmd returns the "jira" command tree for Jira integration.
func JiraCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "jira",
		Short:   "Jira integration for task management",
		Long:    "List, sync, execute, and manage Jira issues with agent orchestration.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, _ := cmd.Flags().GetBool("pane")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			project, _ := cmd.Flags().GetString("project")
			status, _ := cmd.Flags().GetString("status")

			if !hasJiraConfig(app) {
				return runJiraFallback(cmd.Context(), app, pane, jsonOut)
			}

			if pane {
				return runJiraPaneLoop(cmd.Context(), app, instance, project)
			}

			return listJiraIssues(cmd.Context(), app, instance, project, status, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (loop + ANSI)")
	cmd.Flags().String("instance", "", "Override active Jira instance")
	cmd.Flags().String("project", "", "Filter by project key")
	cmd.Flags().String("epic", "", "Filter by epic key")
	cmd.Flags().String("status", "", "Filter by Jira status")

	cmd.AddCommand(jiraShowCmd(app))
	cmd.AddCommand(jiraSyncCmd(app))
	cmd.AddCommand(jiraPromptCmd(app))
	cmd.AddCommand(jiraExecuteCmd(app))
	cmd.AddCommand(jiraTransitionCmd(app))
	cmd.AddCommand(jiraInstancesCmd(app))
	cmd.AddCommand(jiraFactoryCmd(app))

	return cmd
}

// hasJiraConfig returns true if the config has at least one Jira instance configured.
func hasJiraConfig(app *App) bool {
	return app.Config != nil && len(app.Config.Jira.Instances) > 0
}

// resolveInstance finds the Jira instance config by name, or returns the first one.
func resolveInstance(cfg *config.Config, name string) (*config.JiraInstance, error) {
	if len(cfg.Jira.Instances) == 0 {
		return nil, fmt.Errorf("no Jira instances configured")
	}
	if name == "" {
		return &cfg.Jira.Instances[0], nil
	}
	for i := range cfg.Jira.Instances {
		if cfg.Jira.Instances[i].Name == name {
			return &cfg.Jira.Instances[i], nil
		}
	}
	return nil, fmt.Errorf("Jira instance %q not found", name)
}

// newJiraClient creates a Jira REST client from an instance config.
func newJiraClient(inst *config.JiraInstance, cfg *config.Config) *jira.Client {
	limiter := jira.NewRateLimiter(
		cfg.Jira.RateLimit.RequestsPerSecond,
		cfg.Jira.RateLimit.Burst,
	)
	return jira.NewClient(jira.ClientOpts{
		BaseURL:     inst.BaseURL,
		AuthType:    inst.Auth.Type,
		Token:       inst.Auth.Token,
		Username:    inst.Auth.Username,
		Password:    inst.Auth.Password,
		RateLimiter: limiter,
	})
}

// newSyncEngine creates a SyncEngine for the given instance.
func newSyncEngine(app *App, inst *config.JiraInstance) *jira.SyncEngine {
	client := newJiraClient(inst, app.Config)
	return jira.NewSyncEngine(client, app.DB, inst.Name)
}

// runJiraFallback falls back to local task_groups when no Jira is configured.
func runJiraFallback(ctx context.Context, app *App, pane, jsonOut bool) error {
	if pane {
		return runJiraLegacyPane(ctx, app)
	}
	return printJiraLegacySummary(ctx, app, jsonOut)
}

// --- Subcommands ---

func jiraShowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <issue-key>",
		Short: "Show issue detail with agent state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			return showJiraIssue(cmd.Context(), app, args[0], instance, jsonOut)
		},
	}
	cmd.Flags().String("instance", "", "Jira instance name")
	cmd.Flags().Bool("raw", false, "Show raw Jira description")
	return cmd
}

func jiraSyncCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Force sync from active Jira instance",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			return syncJira(cmd.Context(), app, instance, jsonOut)
		},
	}
	cmd.Flags().String("instance", "", "Sync specific instance (or 'all')")
	cmd.Flags().Bool("full", false, "Full resync")
	return cmd
}

func jiraPromptCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt <issue-key>",
		Short: "Generate machine-readable prompt from issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return generateJiraPrompt(cmd.Context(), app, args[0], instance, jsonOut, dryRun)
		},
	}
	cmd.Flags().String("instance", "", "Jira instance name")
	cmd.Flags().String("template", "", "Override prompt template")
	cmd.Flags().Bool("dry-run", false, "Print prompt without executing")
	cmd.Flags().Bool("validate", false, "Validate # Outcomes against project objectives")
	return cmd
}

func jiraExecuteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute <issue-key>",
		Short: "Execute issue: generate prompt + spawn agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			mode, _ := cmd.Flags().GetString("mode")
			return executeJiraIssue(cmd.Context(), app, args[0], instance, mode, jsonOut)
		},
	}
	cmd.Flags().String("instance", "", "Jira instance name")
	cmd.Flags().String("mode", "stepped", "Execution mode: full_auto | stepped | scoped")
	cmd.Flags().String("scope", "task", "Scope: project | epic | task")
	cmd.Flags().String("agent", "", "Override agent type")
	cmd.Flags().Bool("review", false, "Run /sr --review --loop on generated prompt")
	return cmd
}

func jiraTransitionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transition <issue-key> <status>",
		Short: "Transition Jira issue status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")
			comment, _ := cmd.Flags().GetString("comment")
			return transitionJiraIssue(cmd.Context(), app, args[0], args[1], instance, comment, jsonOut)
		},
	}
	cmd.Flags().String("instance", "", "Jira instance name")
	cmd.Flags().String("comment", "", "Add comment with transition")
	return cmd
}

func jiraInstancesCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "instances",
		Short: "List configured Jira instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			return listJiraInstances(app, jsonOut)
		},
	}
}

func jiraFactoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "factory",
		Short: "Start dark factory mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			project, _ := cmd.Flags().GetString("project")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if project == "" {
				return fmt.Errorf("--project is required for dark factory mode")
			}
			return startDarkFactory(cmd.Context(), app, project, dryRun, jsonOut)
		},
	}
	cmd.Flags().String("project", "", "Project scope for automation (required)")
	cmd.Flags().String("epic", "", "Narrow to specific epic")
	cmd.Flags().Int("max-concurrent", 0, "Override max concurrent tasks")
	cmd.Flags().Bool("dry-run", false, "Show execution plan without running")
	return cmd
}

// --- Command implementations ---

func listJiraIssues(ctx context.Context, app *App, instanceName, projectKey, status string, jsonOut bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	issues, err := engine.GetCachedIssues(ctx, projectKey, status)
	if err != nil {
		return jiraError(jsonOut, "jira", err)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":  true,
			"command":  "jira",
			"instance": inst.Name,
			"issues":   issues,
			"count":    len(issues),
		})
	}

	if len(issues) == 0 {
		fmt.Println("No issues. Run 'cmdr jira sync' to fetch from Jira.")
		return nil
	}

	fmt.Printf("%-12s %-40s %-14s %-10s %-10s\n", "KEY", "SUMMARY", "STATUS", "TYPE", "AGENT")
	for _, i := range issues {
		agent := i.AgentState
		if agent == "" {
			agent = "-"
		}
		fmt.Printf("%-12s %-40s %-14s %-10s %-10s\n",
			i.Key,
			truncate(i.Summary, 40),
			truncate(i.Status, 14),
			truncate(i.IssueType, 10),
			truncate(agent, 10),
		)
	}
	return nil
}

func showJiraIssue(ctx context.Context, app *App, issueKey, instanceName string, jsonOut bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	issue, err := engine.GetCachedIssue(ctx, issueKey)
	if err != nil {
		return jiraError(jsonOut, "jira show", err)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":  true,
			"command":  "jira show",
			"instance": inst.Name,
			"issue":    issue,
		})
	}

	fmt.Printf("Issue: %s\n", issue.Key)
	fmt.Printf("  Summary:   %s\n", issue.Summary)
	fmt.Printf("  Status:    %s\n", issue.Status)
	fmt.Printf("  Type:      %s\n", issue.IssueType)
	fmt.Printf("  Priority:  %s\n", issue.Priority)
	fmt.Printf("  Assignee:  %s\n", issue.Assignee)
	if len(issue.Labels) > 0 {
		fmt.Printf("  Labels:    %s\n", strings.Join(issue.Labels, ", "))
	}
	if issue.AgentState != "" {
		fmt.Printf("  Agent:     %s (%s)\n", issue.AgentType, issue.AgentState)
	}
	if issue.Description != "" {
		fmt.Printf("\n%s\n", issue.Description)
	}
	return nil
}

func syncJira(ctx context.Context, app *App, instanceName string, jsonOut bool) error {
	if instanceName == "all" {
		var results []jira.SyncResult
		for i := range app.Config.Jira.Instances {
			inst := &app.Config.Jira.Instances[i]
			engine := newSyncEngine(app, inst)
			project := inst.DefaultProject
			if project == "" {
				continue
			}
			result, err := engine.SyncProject(ctx, project)
			if err != nil {
				result.Error = err.Error()
			}
			results = append(results, *result)
		}
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success": true,
				"command": "jira sync",
				"results": results,
			})
		}
		for _, r := range results {
			if r.Error != "" {
				fmt.Printf("  %s: ERROR %s\n", r.Instance, r.Error)
			} else {
				fmt.Printf("  %s: synced %d issues\n", r.Instance, r.IssuesSync)
			}
		}
		return nil
	}

	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	project := inst.DefaultProject
	if project == "" {
		return fmt.Errorf("no default_project configured for instance %q", inst.Name)
	}

	result, err := engine.SyncProject(ctx, project)
	if jsonOut {
		status := "success"
		errMsg := ""
		if err != nil {
			status = "error"
			errMsg = err.Error()
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":  err == nil,
			"command":  "jira sync",
			"instance": inst.Name,
			"status":   status,
			"synced":   result.IssuesSync,
			"error":    errMsg,
		})
	}

	if err != nil {
		return fmt.Errorf("sync %s: %w", inst.Name, err)
	}

	fmt.Printf("Synced %d issues from %s\n", result.IssuesSync, inst.Name)
	return nil
}

func generateJiraPrompt(ctx context.Context, app *App, issueKey, instanceName string, jsonOut, dryRun bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	issue, err := engine.GetCachedIssue(ctx, issueKey)
	if err != nil {
		return jiraError(jsonOut, "jira prompt", err)
	}

	pg := jira.NewPromptGenerator(app.Config.Jira.PromptTemplate)
	result, err := pg.Generate(issue, "", "", inst.DefaultProject)
	if err != nil {
		return jiraError(jsonOut, "jira prompt", err)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":       true,
			"command":       "jira prompt",
			"issueKey":      issueKey,
			"prompt":        result.Prompt,
			"promptHash":    result.PromptHash,
			"outcomesValid": len(result.Outcomes) > 0,
			"outcomes":      result.Outcomes,
		})
	}

	fmt.Print(result.Prompt)
	if dryRun {
		fmt.Printf("\n---\nHash: %s\nOutcomes: %d\n", result.PromptHash, len(result.Outcomes))
	}
	return nil
}

func executeJiraIssue(ctx context.Context, app *App, issueKey, instanceName, mode string, jsonOut bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	issue, err := engine.GetCachedIssue(ctx, issueKey)
	if err != nil {
		return jiraError(jsonOut, "jira execute", err)
	}

	pg := jira.NewPromptGenerator(app.Config.Jira.PromptTemplate)
	result, err := pg.Generate(issue, "", "", inst.DefaultProject)
	if err != nil {
		return jiraError(jsonOut, "jira execute", err)
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":    true,
			"command":    "jira execute",
			"issueKey":   issueKey,
			"mode":       mode,
			"prompt":     result.Prompt,
			"promptHash": result.PromptHash,
			"outcomes":   result.Outcomes,
		})
	}

	fmt.Printf("Generated prompt for %s (hash: %s)\n", issueKey, result.PromptHash[:12])
	fmt.Printf("Mode: %s\n", mode)
	fmt.Printf("Outcomes: %d\n", len(result.Outcomes))

	// In stepped mode, pause for review.
	if mode == "stepped" {
		fmt.Println("\n[Stepped mode] Review the prompt above before agent execution.")
		fmt.Println("Use 'cmdr jira execute " + issueKey + " --mode full_auto' for autonomous execution.")
	}

	return nil
}

func transitionJiraIssue(ctx context.Context, app *App, issueKey, targetStatus, instanceName, comment string, jsonOut bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	client := newJiraClient(inst, app.Config)

	// Find the transition ID for the target status.
	transitions, err := client.GetTransitions(ctx, issueKey)
	if err != nil {
		return jiraError(jsonOut, "jira transition", err)
	}

	var transitionID string
	for _, t := range transitions {
		if strings.EqualFold(t.To.Name, targetStatus) || strings.EqualFold(t.Name, targetStatus) {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return jiraError(jsonOut, "jira transition",
			fmt.Errorf("no transition to status %q found for %s", targetStatus, issueKey))
	}

	if err := client.TransitionIssue(ctx, issueKey, transitionID); err != nil {
		return jiraError(jsonOut, "jira transition", err)
	}

	if comment != "" {
		if err := client.AddComment(ctx, issueKey, comment); err != nil {
			// Non-fatal: transition succeeded, comment failed.
			fmt.Fprintf(os.Stderr, "Warning: transition succeeded but comment failed: %v\n", err)
		}
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":  true,
			"command":  "jira transition",
			"issueKey": issueKey,
			"status":   targetStatus,
		})
	}

	fmt.Printf("Transitioned %s to %s\n", issueKey, targetStatus)
	return nil
}

func listJiraInstances(app *App, jsonOut bool) error {
	instances := app.Config.Jira.Instances
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":   true,
			"command":   "jira instances",
			"instances": instances,
			"count":     len(instances),
		})
	}

	if len(instances) == 0 {
		fmt.Println("No Jira instances configured.")
		return nil
	}

	fmt.Printf("%-20s %-40s %-10s %-10s\n", "NAME", "URL", "AUTH", "PROJECT")
	for _, inst := range instances {
		fmt.Printf("%-20s %-40s %-10s %-10s\n",
			inst.Name,
			truncate(inst.BaseURL, 40),
			inst.Auth.Type,
			inst.DefaultProject,
		)
	}
	return nil
}

func startDarkFactory(ctx context.Context, app *App, project string, dryRun, jsonOut bool) error {
	if !app.Config.Jira.DarkFactory.Enabled && !dryRun {
		return fmt.Errorf("dark factory mode is disabled in config; set jira.dark_factory.enabled: true")
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": true,
			"command": "jira factory",
			"mode":    app.Config.Jira.DarkFactory.ExecutionMode,
			"project": project,
			"dryRun":  dryRun,
		})
	}

	if dryRun {
		fmt.Printf("Dark factory dry run for project %s\n", project)
		fmt.Printf("  Mode: %s\n", app.Config.Jira.DarkFactory.ExecutionMode)
		fmt.Printf("  Max concurrent: %d\n", app.Config.Jira.DarkFactory.MaxConcurrentTasks)
		fmt.Printf("  UAT timeout: %s\n", app.Config.Jira.DarkFactory.UATTimeout)
		return nil
	}

	fmt.Printf("Starting dark factory for project %s (mode: %s)\n",
		project, app.Config.Jira.DarkFactory.ExecutionMode)
	return nil
}

// --- Legacy fallback (no Jira configured) ---

// jiraLegacyEntry represents a task group from the old local-only system.
type jiraLegacyEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	MemberCount   int    `json:"memberCount"`
	ActiveMembers int    `json:"activeMembers"`
	CreatedAt     string `json:"createdAt"`
}

func queryJiraLegacyTasks(ctx context.Context, app *App) ([]jiraLegacyEntry, error) {
	if app.DB == nil {
		return nil, nil
	}
	rows, err := app.DB.Query(ctx, `
		SELECT tg.id, tg.name, tg.status, COALESCE(tg.created_at, ''),
			COUNT(tgm.issue_id) AS member_count, 0 AS active_members
		FROM task_groups tg
		LEFT JOIN task_group_members tgm ON tg.id = tgm.group_id
		GROUP BY tg.id ORDER BY tg.created_at DESC`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, fmt.Errorf("query task groups: %w", err)
	}
	defer rows.Close()

	var tasks []jiraLegacyEntry
	for rows.Next() {
		var t jiraLegacyEntry
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.MemberCount, &t.ActiveMembers); err != nil {
			return nil, fmt.Errorf("scan task group: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func printJiraLegacySummary(ctx context.Context, app *App, jsonOut bool) error {
	tasks, err := queryJiraLegacyTasks(ctx, app)
	if err != nil {
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success": false, "command": "jira", "error": err.Error(),
			})
		}
		return err
	}
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": true, "command": "jira", "tasks": tasks, "count": len(tasks),
		})
	}
	if len(tasks) == 0 {
		fmt.Println("No tasks.")
		return nil
	}
	fmt.Printf("%-20s %-30s %-10s %-6s %-20s\n", "ID", "NAME", "STATUS", "AGENTS", "CREATED")
	for _, t := range tasks {
		fmt.Printf("%-20s %-30s %-10s %-6d %-20s\n",
			truncate(t.ID, 20), truncate(t.Name, 30), truncate(t.Status, 10), t.MemberCount, truncate(t.CreatedAt, 20))
	}
	return nil
}

func runJiraLegacyPane(ctx context.Context, app *App) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	renderJiraLegacyPane(ctx, app)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renderJiraLegacyPane(ctx, app)
		}
	}
}

func renderJiraLegacyPane(ctx context.Context, app *App) {
	tasks, err := queryJiraLegacyTasks(ctx, app)
	maxRows := jiraTermHeight() - 3
	fmt.Print("\033[2J\033[H")
	fmt.Print("\033[1;36m Tasks \033[0m")
	fmt.Printf(" \033[2m%s\033[0m\n", time.Now().Format("15:04:05"))
	if err != nil {
		fmt.Printf("\033[31m Error: %v\033[0m\n", err)
		return
	}
	if len(tasks) == 0 {
		fmt.Print("\033[2m No tasks\033[0m\n")
		return
	}
	for i, t := range tasks {
		if i >= maxRows {
			fmt.Printf("\033[2m  ... +%d more\033[0m\n", len(tasks)-maxRows)
			break
		}
		statusIcon := jiraStatusIcon(t.Status)
		fmt.Printf(" %s \033[1m%-20s\033[0m \033[2m%d agents\033[0m\n", statusIcon, truncate(t.Name, 20), t.MemberCount)
	}
}

// --- Jira pane mode (with Jira API) ---

// runJiraPaneLoop launches a standalone BubbleTea program rendering the JiraPane.
// This is invoked by `cmdr jira --pane` and is intended to run inside a zellij
// pane as a self-contained TUI. Navigation mirrors the dashboard JiraPane keys.
func runJiraPaneLoop(ctx context.Context, app *App, instanceName, projectKey string) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}
	engine := newSyncEngine(app, inst)

	theme := tui.DefaultTheme()
	pane := tui.NewJiraPane(engine, theme)
	if projectKey != "" {
		pane.SetProject(projectKey)
	}

	// Resolve project key once: prefer explicit flag, fall back to instance default.
	if projectKey == "" {
		projectKey = inst.DefaultProject
	}

	m := &jiraPaneModel{
		ctx:        ctx,
		pane:       pane,
		syncEngine: engine,
		inst:       inst,
		projectKey: projectKey,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, runErr := p.Run()
	return runErr
}

// jiraSyncResultMsg carries the outcome of an async SyncProject call.
type jiraSyncResultMsg struct {
	count int
	err   error
}

// jiraPaneTickMsg drives periodic refresh.
type jiraPaneTickMsg time.Time

// jiraPaneStatusClearMsg signals that the ephemeral status line should be cleared.
type jiraPaneStatusClearMsg struct{}

// jiraPaneModel is a bubbletea.Model wrapping JiraPane with full keybind support.
type jiraPaneModel struct {
	ctx          context.Context
	pane         *tui.JiraPane
	syncEngine   *jira.SyncEngine
	inst         *config.JiraInstance
	projectKey   string
	statusMsg    string
	statusExpiry time.Time
}

func (m *jiraPaneModel) Init() tea.Cmd {
	_ = m.pane.Refresh(m.ctx)
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return jiraPaneTickMsg(t) })
}

// setStatus stores an ephemeral status message and returns a Cmd to clear it after 5s.
func (m *jiraPaneModel) setStatus(msg string) tea.Cmd {
	m.statusMsg = msg
	m.statusExpiry = time.Now().Add(5 * time.Second)
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return jiraPaneStatusClearMsg{} })
}

// syncCmd spawns an async SyncProject and returns the result as jiraSyncResultMsg.
func (m *jiraPaneModel) syncCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.syncEngine.SyncProject(m.ctx, m.projectKey)
		if err != nil {
			return jiraSyncResultMsg{err: err}
		}
		return jiraSyncResultMsg{count: result.IssuesSync}
	}
}

func (m *jiraPaneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve 1 line for status bar at bottom.
		m.pane.SetSize(msg.Width-2, msg.Height-4)

	case jiraPaneTickMsg:
		_ = m.pane.Refresh(m.ctx)
		return m, tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return jiraPaneTickMsg(t) })

	case jiraSyncResultMsg:
		if msg.err != nil {
			return m, m.setStatus(fmt.Sprintf("Sync error: %v", msg.err))
		}
		_ = m.pane.Refresh(m.ctx)
		return m, m.setStatus(fmt.Sprintf("Synced %d issues", msg.count))

	case jiraPaneStatusClearMsg:
		if time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.pane.CursorDown()
		case "k", "up":
			m.pane.CursorUp()
		case "l", "enter", "right":
			m.pane.Expand()
		case "h", "left":
			m.pane.Collapse()
		case "?":
			m.pane.ToggleHelp()
		case "s":
			return m, tea.Batch(m.setStatus("Syncing..."), m.syncCmd())
		case "e":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("Prompt: use 'cmdr jira prompt <key>'")
			}
			return m, m.setStatus(fmt.Sprintf("Prompt: use 'cmdr jira prompt %s'", selected))
		case "p":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("Preview: use 'cmdr jira prompt --dry-run <key>'")
			}
			return m, m.setStatus(fmt.Sprintf("Preview: use 'cmdr jira prompt --dry-run %s'", selected))
		case "x":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("Execute: use 'cmdr jira execute <key>'")
			}
			return m, m.setStatus(fmt.Sprintf("Execute: use 'cmdr jira execute %s'", selected))
		case "i":
			return m, m.setStatus(fmt.Sprintf("Instance: %s (switching requires restart)", m.inst.Name))
		case "f":
			return m, m.setStatus("Factory: use 'cmdr jira factory --project <key>'")
		case "v":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("Verify: use 'cmdr jira execute <key> --verify'")
			}
			return m, m.setStatus(fmt.Sprintf("Verify: use 'cmdr jira execute %s --verify'", selected))
		}
	}
	return m, nil
}

func (m *jiraPaneModel) View() string {
	paneView := m.pane.View()
	if m.statusMsg == "" {
		return paneView
	}
	return paneView + "\n" + m.statusMsg
}

// renderJiraPane is kept for the legacy ANSI loop path (runJiraLegacyPane).
func renderJiraPane(ctx context.Context, app *App, instanceName, projectKey string) {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		fmt.Print("\033[2J\033[H")
		fmt.Printf("\033[31m%v\033[0m\n", err)
		return
	}

	engine := newSyncEngine(app, inst)
	issues, _ := engine.GetCachedIssues(ctx, projectKey, "")

	maxRows := jiraTermHeight() - 3
	fmt.Print("\033[2J\033[H")
	fmt.Printf("\033[1;36m Jira: %s \033[0m", inst.Name)
	fmt.Printf(" \033[2m%s\033[0m\n", time.Now().Format("15:04:05"))

	if len(issues) == 0 {
		fmt.Print("\033[2m No issues\033[0m\n")
		return
	}

	for i, issue := range issues {
		if i >= maxRows {
			fmt.Printf("\033[2m  ... +%d more\033[0m\n", len(issues)-maxRows)
			break
		}
		statusIcon := jiraStatusIcon(issue.Status)
		agent := ""
		if issue.AgentState != "" {
			agent = fmt.Sprintf(" \033[33m[%s]\033[0m", issue.AgentState)
		}
		fmt.Printf(" %s \033[1m%-10s\033[0m %-30s%s\n",
			statusIcon, issue.Key, truncate(issue.Summary, 30), agent)
	}
}

// jiraStatusIcon returns an ANSI-colored status indicator.
func jiraStatusIcon(status string) string {
	switch strings.ToLower(status) {
	case "active", "in progress":
		return "\033[32m●\033[0m"
	case "completed", "done":
		return "\033[36m✓\033[0m"
	case "failed", "blocked":
		return "\033[31m✗\033[0m"
	case "to do":
		return "\033[33m○\033[0m"
	default:
		return "\033[33m○\033[0m"
	}
}

// jiraTermHeight returns the terminal height, defaulting to 20.
func jiraTermHeight() int {
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{}
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if err != 0 || ws.Row == 0 {
		return 20
	}
	return int(ws.Row)
}

// jiraError encodes an error as JSON or returns it.
func jiraError(jsonOut bool, command string, err error) error {
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": false,
			"command": command,
			"error":   err.Error(),
		})
	}
	return err
}
