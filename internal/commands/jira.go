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

	"github.com/spf13/cobra"
)

// JiraCmd returns the "jira" command for task group status in the dashboard pane.
func JiraCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "jira",
		Short:   "Task group status for dashboard pane",
		Long:    "Display task group status. In --pane mode, loops with ANSI-styled output for the dashboard.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, _ := cmd.Flags().GetBool("pane")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if pane {
				return runJiraPane(cmd.Context(), app)
			}

			return printJiraSummary(cmd.Context(), app, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (loop + ANSI)")
	cmd.Flags().String("project", "", "Filter by project ID")

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show task group detail with member agents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			return printJiraDetail(cmd.Context(), app, args[0], jsonOut)
		},
	}
	cmd.AddCommand(showCmd)

	return cmd
}

// jiraTaskEntry represents a task group with member counts.
type jiraTaskEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	MemberCount   int    `json:"memberCount"`
	ActiveMembers int    `json:"activeMembers"`
	CreatedAt     string `json:"createdAt"`
}

// queryJiraTasks reads task groups from the DB with member counts.
func queryJiraTasks(ctx context.Context, app *App) ([]jiraTaskEntry, error) {
	if app.DB == nil {
		return nil, nil
	}

	rows, err := app.DB.Query(ctx, `
		SELECT
			tg.id,
			tg.name,
			tg.status,
			COALESCE(tg.created_at, ''),
			COUNT(tgm.issue_id) AS member_count,
			0 AS active_members
		FROM task_groups tg
		LEFT JOIN task_group_members tgm ON tg.id = tgm.group_id
		GROUP BY tg.id
		ORDER BY tg.created_at DESC
	`)
	if err != nil {
		// Table might not exist — return empty rather than error.
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, fmt.Errorf("query task groups: %w", err)
	}
	defer rows.Close()

	var tasks []jiraTaskEntry
	for rows.Next() {
		var t jiraTaskEntry
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt, &t.MemberCount, &t.ActiveMembers); err != nil {
			return nil, fmt.Errorf("scan task group: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// printJiraSummary prints a one-shot task group summary.
func printJiraSummary(ctx context.Context, app *App, jsonOut bool) error {
	tasks, err := queryJiraTasks(ctx, app)
	if err != nil {
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success": false,
				"command": "jira",
				"error":   err.Error(),
			})
		}
		return err
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": true,
			"command": "jira",
			"tasks":   tasks,
			"count":   len(tasks),
		})
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks.")
		return nil
	}

	fmt.Printf("%-20s %-30s %-10s %-6s %-20s\n", "ID", "NAME", "STATUS", "AGENTS", "CREATED")
	for _, t := range tasks {
		fmt.Printf("%-20s %-30s %-10s %-6d %-20s\n",
			truncate(t.ID, 20),
			truncate(t.Name, 30),
			truncate(t.Status, 10),
			t.MemberCount,
			truncate(t.CreatedAt, 20),
		)
	}
	return nil
}

// printJiraDetail shows a single task group with members.
func printJiraDetail(ctx context.Context, app *App, groupID string, jsonOut bool) error {
	row := app.DB.QueryRow(ctx,
		"SELECT id, name, status, COALESCE(created_at, '') FROM task_groups WHERE id = ?",
		groupID)

	var t jiraTaskEntry
	if err := row.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt); err != nil {
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"success": false,
				"command": "jira",
				"error":   fmt.Sprintf("group %q not found: %v", groupID, err),
			})
		}
		return fmt.Errorf("group %q not found: %w", groupID, err)
	}

	members, err := queryGroupMembers(ctx, app, groupID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	t.MemberCount = len(members)

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": true,
			"command": "jira",
			"task":    t,
			"members": members,
		})
	}

	fmt.Printf("Task: %s\n", t.Name)
	fmt.Printf("  ID:      %s\n", t.ID)
	fmt.Printf("  Status:  %s\n", t.Status)
	fmt.Printf("  Created: %s\n", t.CreatedAt)
	fmt.Printf("  Members: %d\n", t.MemberCount)
	if len(members) > 0 {
		for _, m := range members {
			fmt.Printf("    - %s\n", m)
		}
	}
	return nil
}

// runJiraPane runs the Jira pane in a ticker loop with ANSI-styled output.
func runJiraPane(ctx context.Context, app *App) error {
	const refreshInterval = 5 * time.Second

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	// Immediate first render.
	renderJiraPane(ctx, app)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renderJiraPane(ctx, app)
		}
	}
}

// renderJiraPane renders one frame of the Jira pane.
func renderJiraPane(ctx context.Context, app *App) {
	tasks, err := queryJiraTasks(ctx, app)

	// Get terminal height for truncation.
	maxRows := jiraTermHeight() - 3 // header + border

	// Clear screen and move cursor to top.
	fmt.Print("\033[2J\033[H")

	// Header.
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
		fmt.Printf(" %s \033[1m%-20s\033[0m \033[2m%d agents\033[0m\n",
			statusIcon,
			truncate(t.Name, 20),
			t.MemberCount,
		)
	}
}

// jiraStatusIcon returns an ANSI-colored status indicator.
func jiraStatusIcon(status string) string {
	switch status {
	case "active":
		return "\033[32m●\033[0m" // green dot
	case "completed":
		return "\033[36m✓\033[0m" // cyan check
	case "failed":
		return "\033[31m✗\033[0m" // red x
	default:
		return "\033[33m○\033[0m" // yellow circle (pending)
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
