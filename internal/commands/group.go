package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// GroupCmd returns the "group" command for task group management.
func GroupCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "group",
		Short:   "Task group batch tracking",
		Long:    "Create, list, and manage task groups for batch agent coordination.",
		GroupID: "GROUPS",
	}

	cmd.AddCommand(groupCreateCmd(app))
	cmd.AddCommand(groupListCmd(app))
	cmd.AddCommand(groupStatusCmd(app))

	return cmd
}

func groupCreateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new task group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			groupID := fmt.Sprintf("grp-%d", timeNow())

			err := app.DB.Exec(cmd.Context(),
				"INSERT INTO task_groups (id, name) VALUES (?, ?)",
				groupID, name)
			if err != nil {
				return fmt.Errorf("create group: %w", err)
			}

			fmt.Printf("Created group %q (id: %s)\n", name, groupID)
			return nil
		},
	}

	return cmd
}

func groupListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List task groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			groups, err := queryGroups(cmd.Context(), app)
			if err != nil {
				return fmt.Errorf("list groups: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(groups)
			}

			if len(groups) == 0 {
				fmt.Println("No task groups.")
				return nil
			}

			fmt.Printf("%-20s %-20s %-10s %-20s\n", "ID", "NAME", "STATUS", "CREATED")
			for _, g := range groups {
				fmt.Printf("%-20s %-20s %-10s %-20s\n",
					truncate(g.ID, 20),
					truncate(g.Name, 20),
					truncate(g.Status, 10),
					truncate(g.CreatedAt, 20),
				)
			}
			return nil
		},
	}
}

func groupStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status <group-id>",
		Short: "Show group status and members",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := args[0]

			row := app.DB.QueryRow(cmd.Context(),
				"SELECT id, name, status, created_at FROM task_groups WHERE id = ?",
				groupID)

			var g groupRow
			if err := row.Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt); err != nil {
				return fmt.Errorf("group %q not found: %w", groupID, err)
			}

			fmt.Printf("Group: %s\n", g.Name)
			fmt.Printf("  ID:      %s\n", g.ID)
			fmt.Printf("  Status:  %s\n", g.Status)
			fmt.Printf("  Created: %s\n", g.CreatedAt)

			// List members.
			members, err := queryGroupMembers(cmd.Context(), app, groupID)
			if err != nil {
				return fmt.Errorf("list members: %w", err)
			}

			if len(members) > 0 {
				fmt.Println("  Members:")
				for _, m := range members {
					fmt.Printf("    - %s\n", m)
				}
			}
			return nil
		},
	}
}

type groupRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

func queryGroups(ctx context.Context, app *App) ([]groupRow, error) {
	rows, err := app.DB.Query(ctx, "SELECT id, name, status, created_at FROM task_groups ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []groupRow
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Status, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func queryGroupMembers(ctx context.Context, app *App, groupID string) ([]string, error) {
	rows, err := app.DB.Query(ctx, "SELECT issue_id FROM task_group_members WHERE group_id = ?", groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// timeNow returns current unix nanosecond timestamp.
func timeNow() int64 {
	return time.Now().UnixNano()
}
