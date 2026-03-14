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

			if pane {
				if !hasJiraConfig(app) {
					// No Jira config: run BubbleTea pane with nil lister (shows "No Jira issues")
					theme := tui.DefaultTheme()
					p := tui.NewJiraPane(nil, theme)
					m := &jiraPaneModel{
						ctx:  cmd.Context(),
						pane: p,
					}
					prog := tea.NewProgram(m, tea.WithAltScreen())
					_, err := prog.Run()
					return err
				}
				return runJiraPaneLoop(cmd.Context(), app, instance, project)
			}

			if !hasJiraConfig(app) {
				return runJiraFallback(cmd.Context(), app, false, jsonOut)
			}

			noSubTasks, _ := cmd.Flags().GetBool("no-subtasks")
			return listJiraIssues(cmd.Context(), app, instance, project, status, noSubTasks, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (loop + ANSI)")
	cmd.Flags().String("instance", "", "Override active Jira instance")
	cmd.Flags().String("project", "", "Filter by project key")
	cmd.Flags().String("epic", "", "Filter by epic key")
	cmd.Flags().String("status", "", "Filter by Jira status")
	cmd.Flags().Bool("no-subtasks", true, "Exclude sub-tasks from results (default: true)")

	cmd.AddCommand(jiraShowCmd(app))
	cmd.AddCommand(jiraSyncCmd(app))
	cmd.AddCommand(jiraPromptCmd(app))
	cmd.AddCommand(jiraExecuteCmd(app))
	cmd.AddCommand(jiraTransitionCmd(app))
	cmd.AddCommand(jiraInstancesCmd(app))
	cmd.AddCommand(jiraFactoryCmd(app))
	cmd.AddCommand(jiraLogCmd(app))
	cmd.AddCommand(jiraUndoCmd(app))

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

func jiraLogCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List recent prompt executions",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			issueKey, _ := cmd.Flags().GetString("issue")
			batchID, _ := cmd.Flags().GetString("batch")
			limit, _ := cmd.Flags().GetInt("limit")

			var entries []PromptExecLog
			var err error
			if batchID != "" {
				entries, err = queryPromptLogByBatch(cmd.Context(), app.DB, batchID, limit)
			} else {
				entries, err = queryPromptLog(cmd.Context(), app.DB, issueKey, limit)
			}
			if err != nil {
				return jiraError(jsonOut, "jira log", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success": true,
					"command": "jira log",
					"entries": entries,
					"count":   len(entries),
				})
			}

			if len(entries) == 0 {
				fmt.Println("No prompt executions found.")
				return nil
			}

			fmt.Printf("%-6s %-12s %-14s %-8s %-10s %-20s\n", "ID", "ISSUE", "HASH", "STATUS", "COMMENT", "CREATED")
			for _, e := range entries {
				commentID := e.JiraCommentID
				if commentID == "" {
					commentID = "-"
				}
				fmt.Printf("%-6d %-12s %-14s %-8s %-10s %-20s\n",
					e.ID,
					truncate(e.IssueKey, 12),
					truncate(e.PromptHash, 14),
					truncate(e.Status, 8),
					truncate(commentID, 10),
					truncate(e.CreatedAt, 20),
				)
			}
			return nil
		},
	}
	cmd.Flags().String("issue", "", "Filter by issue key")
	cmd.Flags().String("batch", "", "Filter by batch ID")
	cmd.Flags().Int("limit", 50, "Max entries")
	cmd.Flags().Bool("json", false, "JSON output")
	return cmd
}

func jiraUndoCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "undo <log-id>",
		Short: "Undo a prompt execution (delete Jira comment)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			instance, _ := cmd.Flags().GetString("instance")

			var logID int64
			if _, err := fmt.Sscanf(args[0], "%d", &logID); err != nil {
				return jiraError(jsonOut, "jira undo", fmt.Errorf("invalid log ID: %s", args[0]))
			}

			entry, err := getPromptLogByID(cmd.Context(), app.DB, logID)
			if err != nil {
				return jiraError(jsonOut, "jira undo", err)
			}
			if entry.Status == "undone" {
				return jiraError(jsonOut, "jira undo", fmt.Errorf("log entry %d already undone", logID))
			}
			if entry.JiraCommentID == "" {
				return jiraError(jsonOut, "jira undo", fmt.Errorf("no comment ID for log entry %d", logID))
			}

			instName := instance
			if instName == "" {
				instName = entry.InstanceName
			}
			inst, err := resolveInstance(app.Config, instName)
			if err != nil {
				return jiraError(jsonOut, "jira undo", err)
			}

			client := newJiraClient(inst, app.Config)
			if err := client.DeleteComment(cmd.Context(), entry.IssueKey, entry.JiraCommentID); err != nil {
				return jiraError(jsonOut, "jira undo", fmt.Errorf("delete comment failed: %w", err))
			}

			markExecUndone(cmd.Context(), app.DB, logID)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":        true,
					"command":        "jira undo",
					"logId":          logID,
					"issueKey":       entry.IssueKey,
					"commentDeleted": true,
				})
			}

			fmt.Printf("Undone: deleted comment %s from %s (log #%d)\n", entry.JiraCommentID, entry.IssueKey, logID)
			return nil
		},
	}
	cmd.Flags().String("instance", "", "Override Jira instance")
	return cmd
}

// --- Command implementations ---

func listJiraIssues(ctx context.Context, app *App, instanceName, projectKey, status string, excludeSubTasks bool, jsonOut bool) error {
	inst, err := resolveInstance(app.Config, instanceName)
	if err != nil {
		return err
	}

	engine := newSyncEngine(app, inst)
	issues, err := engine.GetCachedIssuesFiltered(ctx, projectKey, status, excludeSubTasks)
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

	result, err := engine.SyncProjectWithOpts(ctx, jira.SyncOpts{ProjectKey: project})
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

	// For parent-type issues, fetch sub-tasks and use recursive generation.
	var result *jira.PromptResult
	if !isSubTaskType(issue.IssueType) {
		subTasks, _ := engine.GetSubTasks(ctx, issueKey)
		if len(subTasks) > 0 {
			result, err = pg.GenerateRecursive(issue, subTasks, "", "", inst.DefaultProject)
		} else {
			result, err = pg.Generate(issue, "", "", inst.DefaultProject)
		}
	} else {
		result, err = pg.Generate(issue, "", "", inst.DefaultProject)
	}
	if err != nil {
		return jiraError(jsonOut, "jira prompt", err)
	}

	// Post the generated prompt to the Jira ticket as a comment.
	commentBody := fmt.Sprintf("*Generated Prompt (cmdr)*\n\nHash: %s\n\n---\n\n%s",
		result.PromptHash[:12], result.Prompt)
	client := newJiraClient(inst, app.Config)
	if err := client.AddComment(ctx, issueKey, commentBody); err != nil {
		return jiraError(jsonOut, "jira prompt", fmt.Errorf("prompt generated but Jira comment failed: %w", err))
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":    true,
			"command":    "jira prompt",
			"issueKey":   issueKey,
			"promptHash": result.PromptHash,
			"outcomes":   len(result.Outcomes),
			"commented":  true,
		})
	}

	fmt.Printf("Prompt posted to %s (hash: %s, outcomes: %d)\n", issueKey, result.PromptHash[:12], len(result.Outcomes))
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

	// For parent-type issues, fetch sub-tasks and use recursive generation.
	var result *jira.PromptResult
	if !isSubTaskType(issue.IssueType) {
		subTasks, _ := engine.GetSubTasks(ctx, issueKey)
		if len(subTasks) > 0 {
			result, err = pg.GenerateRecursive(issue, subTasks, "", "", inst.DefaultProject)
		} else {
			result, err = pg.Generate(issue, "", "", inst.DefaultProject)
		}
	} else {
		result, err = pg.Generate(issue, "", "", inst.DefaultProject)
	}
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
	pane.SetInstance(inst.Name)
	pane.SetExcludeSubTasks(true)
	if projectKey != "" {
		pane.SetProject(projectKey)
	}

	// Resolve project key once: prefer explicit flag, fall back to instance default.
	if projectKey == "" {
		projectKey = inst.DefaultProject
	}

	m := &jiraPaneModel{
		ctx:        ctx,
		app:        app,
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

type jiraProjectListMsg struct {
	projects []jira.APIProject
	err      error
}

type jiraIssueDetailMsg struct {
	issue *jira.JiraIssue
	err   error
}

// jiraPromptResultMsg carries the outcome of an async prompt generation + comment post.
type jiraPromptResultMsg struct {
	issueKey   string
	promptHash string
	outcomes   int
	err        error
}

// jiraPaneModel is a bubbletea.Model wrapping JiraPane with full keybind support.
type jiraPaneModel struct {
	ctx          context.Context
	app          *App
	pane         *tui.JiraPane
	syncEngine   *jira.SyncEngine
	inst         *config.JiraInstance
	projectKey   string
	statusMsg    string
	statusExpiry time.Time
	lastKey      string
	// Instance picker state.
	showInstancePicker bool
	instanceNames      []string
	instanceCursor     int
	// Project picker state.
	showProjectPicker bool
	projectList       []jira.APIProject
	projectCursor     int
	projectLoading    bool
	// Issue detail overlay state.
	showIssueDetail bool
	detailIssue     *jira.JiraIssue
	// Preview overlay state.
	showPreview bool
	previewKeys []string
	// Execution log overlay state.
	showLogOverlay bool
	logEntries     []PromptExecLog
	logCursor      int
	// Last comment ID for quick undo.
	lastCommentID  string
	lastCommentKey string
	// Preserved dimensions for hot-swap resize.
	lastWidth  int
	lastHeight int
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

// execPromptCmd generates a prompt for an issue and posts it as a Jira comment.
// For parent tasks, it recursively includes sub-task material with orchestrator instructions.
func (m *jiraPaneModel) execPromptCmd(issueKey string) tea.Cmd {
	return func() tea.Msg {
		if m.syncEngine == nil {
			return jiraPromptResultMsg{issueKey: issueKey, err: fmt.Errorf("no sync engine")}
		}
		issue, err := m.syncEngine.GetCachedIssue(m.ctx, issueKey)
		if err != nil {
			return jiraPromptResultMsg{issueKey: issueKey, err: err}
		}
		pg := jira.NewPromptGenerator(m.app.Config.Jira.PromptTemplate)

		// For parent-type issues, fetch sub-tasks and use recursive generation.
		var result *jira.PromptResult
		if !isSubTaskType(issue.IssueType) {
			subTasks, _ := m.syncEngine.GetSubTasks(m.ctx, issueKey)
			if len(subTasks) > 0 {
				result, err = pg.GenerateRecursive(issue, subTasks, "", "", m.projectKey)
			} else {
				result, err = pg.Generate(issue, "", "", m.projectKey)
			}
		} else {
			result, err = pg.Generate(issue, "", "", m.projectKey)
		}
		if err != nil {
			return jiraPromptResultMsg{issueKey: issueKey, err: err}
		}
		commentBody := fmt.Sprintf("*Generated Prompt (cmdr)*\n\nHash: %s\n\n---\n\n%s",
			result.PromptHash[:12], result.Prompt)
		client := newJiraClient(m.inst, m.app.Config)
		if err := client.AddComment(m.ctx, issueKey, commentBody); err != nil {
			return jiraPromptResultMsg{issueKey: issueKey, err: fmt.Errorf("comment failed: %w", err)}
		}
		return jiraPromptResultMsg{
			issueKey:   issueKey,
			promptHash: result.PromptHash[:12],
			outcomes:   len(result.Outcomes),
		}
	}
}

// fetchIssueDetailCmd fetches a single issue from the cache and returns it as jiraIssueDetailMsg.
func (m *jiraPaneModel) fetchIssueDetailCmd(issueKey string) tea.Cmd {
	return func() tea.Msg {
		if m.syncEngine == nil {
			return jiraIssueDetailMsg{err: fmt.Errorf("no sync engine")}
		}
		issue, err := m.syncEngine.GetCachedIssue(m.ctx, issueKey)
		if err != nil {
			return jiraIssueDetailMsg{err: err}
		}
		return jiraIssueDetailMsg{issue: issue}
	}
}

// fetchProjectsCmd fetches the project list from the active Jira instance.
func (m *jiraPaneModel) fetchProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		client := newJiraClient(m.inst, m.app.Config)
		projects, err := client.ListProjects(m.ctx)
		return jiraProjectListMsg{projects: projects, err: err}
	}
}

// syncCmd spawns an async SyncProject and returns the result as jiraSyncResultMsg.
func (m *jiraPaneModel) syncCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.syncEngine.SyncProjectWithOpts(m.ctx, jira.SyncOpts{ProjectKey: m.projectKey})
		if err != nil {
			return jiraSyncResultMsg{err: err}
		}
		return jiraSyncResultMsg{count: result.IssuesSync}
	}
}

func (m *jiraPaneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.lastWidth = msg.Width
		m.lastHeight = msg.Height
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

	case jiraPromptResultMsg:
		if msg.err != nil {
			return m, m.setStatus(fmt.Sprintf("Prompt error (%s): %v", msg.issueKey, msg.err))
		}
		return m, m.setStatus(fmt.Sprintf("Prompt posted to %s (hash: %s, outcomes: %d)", msg.issueKey, msg.promptHash, msg.outcomes))

	case jiraPromptExecResultMsg:
		if msg.err != nil {
			return m, m.setStatus(fmt.Sprintf("Prompt error (%s): %v", msg.issueKey, msg.err))
		}
		m.lastCommentID = msg.commentID
		m.lastCommentKey = msg.issueKey
		return m, m.setStatus(fmt.Sprintf("Prompt posted to %s (hash: %s, log: #%d)", msg.issueKey, msg.promptHash, msg.logID))

	case jiraBatchExecResultMsg:
		return m, m.setStatus(fmt.Sprintf("Batch %s: %d/%d succeeded, %d failed",
			msg.batchID[:8], msg.succeeded, msg.total, msg.failed))

	case jiraUndoResultMsg:
		if msg.err != nil {
			return m, m.setStatus(fmt.Sprintf("Undo error (%s): %v", msg.issueKey, msg.err))
		}
		return m, m.setStatus(fmt.Sprintf("Undone: deleted comment from %s (log #%d)", msg.issueKey, msg.logID))

	case jiraLogEntriesMsg:
		if msg.err != nil {
			m.showLogOverlay = false
			return m, m.setStatus(fmt.Sprintf("Log error: %v", msg.err))
		}
		m.logEntries = msg.entries
		m.logCursor = 0
		m.showLogOverlay = true
		return m, nil

	case jiraIssueDetailMsg:
		if msg.err != nil {
			return m, m.setStatus(fmt.Sprintf("Detail error: %v", msg.err))
		}
		m.detailIssue = msg.issue
		m.showIssueDetail = true
		return m, nil

	case jiraProjectListMsg:
		m.projectLoading = false
		if msg.err != nil {
			m.showProjectPicker = false
			return m, m.setStatus(fmt.Sprintf("Project fetch error: %v", msg.err))
		}
		m.projectList = msg.projects
		if len(msg.projects) == 0 {
			m.showProjectPicker = false
			return m, m.setStatus("No projects found — check API token and permissions")
		}
		m.projectCursor = 0
		m.showProjectPicker = true
		for i, p := range msg.projects {
			if p.Key == m.projectKey {
				m.projectCursor = i
				break
			}
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		// Preview overlay intercepts all keys when visible.
		if m.showPreview {
			switch key {
			case "enter", "y":
				keys := m.previewKeys
				m.showPreview = false
				m.previewKeys = nil
				return m, tea.Batch(m.setStatus(fmt.Sprintf("Executing batch for %d tickets...", len(keys))), m.execBatchPromptCmd(keys))
			case "esc", "q", "n":
				m.showPreview = false
				m.previewKeys = nil
			}
			return m, nil
		}

		// Log overlay intercepts all keys when visible.
		if m.showLogOverlay {
			switch key {
			case "j", "down":
				if m.logCursor < len(m.logEntries)-1 {
					m.logCursor++
				}
			case "k", "up":
				if m.logCursor > 0 {
					m.logCursor--
				}
			case "esc", "q":
				m.showLogOverlay = false
				m.logEntries = nil
			}
			return m, nil
		}

		// Issue detail overlay intercepts all keys when visible.
		if m.showIssueDetail {
			switch key {
			case "esc", "q", "h":
				m.showIssueDetail = false
				m.detailIssue = nil
			}
			return m, nil
		}

		// Project picker intercepts all keys when visible.
		if m.showProjectPicker {
			switch key {
			case "j", "down":
				if m.projectCursor < len(m.projectList)-1 {
					m.projectCursor++
				}
			case "k", "up":
				if m.projectCursor > 0 {
					m.projectCursor--
				}
			case "enter", "l":
				if len(m.projectList) > 0 {
					selected := m.projectList[m.projectCursor]
					m.projectKey = selected.Key
					m.pane.SetProject(selected.Key)
					m.showProjectPicker = false
					_ = m.pane.Refresh(m.ctx)
					return m, m.setStatus(fmt.Sprintf("Project: %s", selected.Key))
				}
			case "esc", "q", "h":
				m.showProjectPicker = false
				// Reset to instance default when cancelling to avoid stale project key.
				if m.inst != nil {
					m.projectKey = m.inst.DefaultProject
					m.pane.SetProject(m.inst.DefaultProject)
					_ = m.pane.Refresh(m.ctx)
				}
			}
			return m, nil
		}

		// Instance picker intercepts all keys when visible.
		if m.showInstancePicker {
			switch key {
			case "j", "down":
				if m.instanceCursor < len(m.instanceNames)-1 {
					m.instanceCursor++
				}
			case "k", "up":
				if m.instanceCursor > 0 {
					m.instanceCursor--
				}
			case "enter", "l":
				if len(m.instanceNames) > 0 {
					selectedName := m.instanceNames[m.instanceCursor]
					inst, err := resolveInstance(m.app.Config, selectedName)
					if err != nil {
						m.showInstancePicker = false
						return m, m.setStatus(fmt.Sprintf("Error: %v", err))
					}
					engine := newSyncEngine(m.app, inst)
					m.inst = inst
					m.syncEngine = engine
					m.projectKey = inst.DefaultProject
					newPane := tui.NewJiraPane(engine, tui.DefaultTheme())
					newPane.SetInstance(inst.Name)
					newPane.SetProject(inst.DefaultProject)
					newPane.SetSize(m.lastWidth-2, m.lastHeight-4)
					_ = newPane.Refresh(m.ctx)
					m.pane = newPane
					m.showInstancePicker = false
					m.projectLoading = true
					m.showProjectPicker = true
					return m, tea.Batch(m.setStatus(fmt.Sprintf("Switched to %s", inst.Name)), m.fetchProjectsCmd())
				}
			case "esc", "q", "h":
				m.showInstancePicker = false
			}
			return m, nil
		}

		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.pane.CursorDown()
		case "k", "up":
			m.pane.CursorUp()
		case "n", "pgdown", "ctrl+d":
			m.pane.PageDown()
		case "N", "pgup", "ctrl+u":
			m.pane.PageUp()
		case "G":
			m.pane.GoBottom()
		case "g":
			if m.lastKey == "g" {
				m.pane.GoTop()
				m.lastKey = ""
				return m, nil
			}
		case "l", "enter", "right":
			m.pane.Expand()
		case "h", "left":
			m.pane.Collapse()
		case "?":
			m.pane.ToggleHelp()
		case "s":
			if m.syncEngine == nil {
				return m, m.setStatus("No Jira instance configured")
			}
			return m, tea.Batch(m.setStatus("Syncing..."), m.syncCmd())
		case "o":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("No issue selected")
			}
			return m, m.fetchIssueDetailCmd(selected)
		case "e":
			if m.inst == nil || m.syncEngine == nil {
				return m, m.setStatus("No Jira instance configured")
			}
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("No issue selected")
			}
			return m, tea.Batch(m.setStatus(fmt.Sprintf("Generating prompt for %s...", selected)), m.execPromptCmdWithLog(selected, ""))
		case "E":
			if m.inst == nil || m.syncEngine == nil {
				return m, m.setStatus("No Jira instance configured")
			}
			keys := m.selectedOrAllKeys()
			if len(keys) == 0 {
				return m, m.setStatus("No issues to execute")
			}
			// Show preview overlay before batch execution.
			m.previewKeys = keys
			m.showPreview = true
			return m, nil
		case " ":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, nil
			}
			m.pane.ToggleSelect(selected)
			return m, nil
		case "u":
			if m.inst == nil || m.syncEngine == nil {
				return m, m.setStatus("No Jira instance configured")
			}
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("No issue selected")
			}
			return m, tea.Batch(m.setStatus(fmt.Sprintf("Undoing last prompt for %s...", selected)), m.undoPromptCmd(selected))
		case "L":
			if m.app == nil || m.app.DB == nil {
				return m, m.setStatus("No database configured")
			}
			return m, m.fetchLogEntriesCmd()
		case "p":
			if m.app == nil || m.inst == nil {
				return m, m.setStatus("No Jira instance configured")
			}
			m.projectLoading = true
			m.showProjectPicker = true
			return m, m.fetchProjectsCmd()
		case "P":
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
			if m.app == nil || len(m.app.Config.Jira.Instances) == 0 {
				return m, m.setStatus("No Jira instances configured")
			}
			names := make([]string, len(m.app.Config.Jira.Instances))
			for idx, inst := range m.app.Config.Jira.Instances {
				names[idx] = inst.Name
			}
			m.instanceNames = names
			// Set cursor to current instance.
			m.instanceCursor = 0
			if m.inst != nil {
				for idx, n := range names {
					if n == m.inst.Name {
						m.instanceCursor = idx
						break
					}
				}
			}
			m.showInstancePicker = true
			return m, nil
		case "t":
			m.pane.ToggleSubTasks()
			_ = m.pane.Refresh(m.ctx)
			label := "hidden"
			if !m.pane.ExcludeSubTasks() {
				label = "visible"
			}
			return m, m.setStatus(fmt.Sprintf("Sub-tasks: %s", label))
		case "f":
			return m, m.setStatus("Factory: use 'cmdr jira factory --project <key>'")
		case "v":
			selected := m.pane.SelectedKey()
			if selected == "" {
				return m, m.setStatus("Verify: use 'cmdr jira execute <key> --verify'")
			}
			return m, m.setStatus(fmt.Sprintf("Verify: use 'cmdr jira execute %s --verify'", selected))
		}
		// Track last key for multi-key sequences (e.g. gg).
		// Reset to empty for any key other than "g" so partial sequences don't linger.
		if key == "g" {
			m.lastKey = key
		} else {
			m.lastKey = ""
		}
	}
	return m, nil
}

func (m *jiraPaneModel) View() string {
	if m.showPreview {
		return m.viewPreviewOverlay()
	}
	if m.showLogOverlay {
		return m.viewLogOverlay()
	}
	if m.showIssueDetail && m.detailIssue != nil {
		return m.viewIssueDetail()
	}
	if m.showProjectPicker {
		return m.viewProjectPicker()
	}
	if m.showInstancePicker {
		return m.viewInstancePicker()
	}
	paneView := m.pane.View()
	if m.statusMsg == "" {
		return paneView
	}
	return paneView + "\n" + m.statusMsg
}

func (m *jiraPaneModel) viewInstancePicker() string {
	theme := tui.DefaultTheme()
	sep := strings.Repeat("\u2500", 20)
	var lines []string
	lines = append(lines, theme.Title.Render("Switch Jira Instance"))
	lines = append(lines, sep)
	for i, name := range m.instanceNames {
		cursor := "  "
		if i == m.instanceCursor {
			cursor = "> "
		}
		lines = append(lines, cursor+name)
	}
	lines = append(lines, sep)
	lines = append(lines, theme.HelpBar.Render("  j/k:nav  enter:select  esc:cancel"))
	return strings.Join(lines, "\n")
}

func (m *jiraPaneModel) viewProjectPicker() string {
	theme := tui.DefaultTheme()
	var lines []string
	lines = append(lines, theme.Title.Render("Select Project"))
	lines = append(lines, strings.Repeat("\u2500", 60))

	if m.projectLoading {
		lines = append(lines, "  Loading projects...")
	} else if len(m.projectList) == 0 {
		lines = append(lines, "  No projects found")
	} else {
		lines = append(lines, fmt.Sprintf("  %-8s %-28s %-12s %s", "KEY", "NAME", "TYPE", "LEAD"))
		for i, proj := range m.projectList {
			cursor := "  "
			if i == m.projectCursor {
				cursor = "> "
			}
			lead := "-"
			if proj.Lead != nil {
				lead = truncate(proj.Lead.DisplayName, 16)
			}
			ptype := proj.ProjectTypeKey
			if ptype == "" {
				ptype = "-"
			}
			lines = append(lines, fmt.Sprintf("%s%-8s %-28s %-12s %s",
				cursor,
				truncate(proj.Key, 8),
				truncate(proj.Name, 28),
				truncate(ptype, 12),
				lead,
			))
		}
	}

	lines = append(lines, strings.Repeat("\u2500", 60))
	lines = append(lines, theme.HelpBar.Render("  j/k:nav  enter:select  esc:cancel"))
	return strings.Join(lines, "\n")
}

func (m *jiraPaneModel) viewPreviewOverlay() string {
	theme := tui.DefaultTheme()
	var lines []string
	lines = append(lines, theme.Title.Render("Batch Prompt Execution Preview"))
	lines = append(lines, strings.Repeat("─", 60))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %d ticket(s) will be prompted:", len(m.previewKeys)))
	lines = append(lines, "")
	for i, key := range m.previewKeys {
		if i >= 20 {
			lines = append(lines, fmt.Sprintf("    ... and %d more", len(m.previewKeys)-20))
			break
		}
		lines = append(lines, fmt.Sprintf("    • %s", key))
	}
	lines = append(lines, "")
	lines = append(lines, strings.Repeat("─", 60))
	lines = append(lines, theme.HelpBar.Render("  enter/y:confirm  esc/n:cancel"))
	return strings.Join(lines, "\n")
}

func (m *jiraPaneModel) viewLogOverlay() string {
	theme := tui.DefaultTheme()
	var lines []string
	lines = append(lines, theme.Title.Render("Prompt Execution Log"))
	lines = append(lines, strings.Repeat("─", 70))

	if len(m.logEntries) == 0 {
		lines = append(lines, "  No log entries found.")
	} else {
		lines = append(lines, fmt.Sprintf("  %-5s %-12s %-14s %-8s %-20s", "ID", "ISSUE", "HASH", "STATUS", "CREATED"))
		for i, e := range m.logEntries {
			cursor := "  "
			if i == m.logCursor {
				cursor = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%-5d %-12s %-14s %-8s %-20s",
				cursor,
				e.ID,
				truncate(e.IssueKey, 12),
				truncate(e.PromptHash, 14),
				truncate(e.Status, 8),
				truncate(e.CreatedAt, 20),
			))
		}
	}

	lines = append(lines, strings.Repeat("─", 70))
	lines = append(lines, theme.HelpBar.Render("  j/k:nav  esc/q:close"))
	return strings.Join(lines, "\n")
}

func (m *jiraPaneModel) viewIssueDetail() string {
	theme := tui.DefaultTheme()
	issue := m.detailIssue
	var lines []string

	lines = append(lines, theme.Title.Render(fmt.Sprintf("  %s", issue.Key)))
	lines = append(lines, strings.Repeat("─", 60))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Summary:    %s", issue.Summary))
	lines = append(lines, fmt.Sprintf("  Status:     %s", issue.Status))
	lines = append(lines, fmt.Sprintf("  Type:       %s", issue.IssueType))
	lines = append(lines, fmt.Sprintf("  Priority:   %s", issue.Priority))
	assignee := issue.Assignee
	if assignee == "" {
		assignee = "Unassigned"
	}
	lines = append(lines, fmt.Sprintf("  Assignee:   %s", assignee))
	if len(issue.Labels) > 0 {
		lines = append(lines, fmt.Sprintf("  Labels:     %s", strings.Join(issue.Labels, ", ")))
	}
	if issue.AgentState != "" {
		lines = append(lines, fmt.Sprintf("  Agent:      %s (%s)", issue.AgentType, issue.AgentState))
	}

	if issue.Description != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Repeat("─", 60))
		lines = append(lines, "")
		// Word-wrap description to ~56 chars with 2-char indent.
		for _, descLine := range strings.Split(issue.Description, "\n") {
			if len(descLine) > 56 {
				for len(descLine) > 56 {
					lines = append(lines, "  "+descLine[:56])
					descLine = descLine[56:]
				}
				if descLine != "" {
					lines = append(lines, "  "+descLine)
				}
			} else {
				lines = append(lines, "  "+descLine)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, strings.Repeat("─", 60))
	lines = append(lines, theme.HelpBar.Render("  esc/q:close"))
	return strings.Join(lines, "\n")
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
