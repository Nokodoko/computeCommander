package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// ProjectCmd returns the "project" command group with add/remove/list/switch subcommands.
func ProjectCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage registered projects",
		GroupID: "CORE",
	}

	cmd.AddCommand(projectAddCmd(app))
	cmd.AddCommand(projectRemoveCmd(app))
	cmd.AddCommand(projectListCmd(app))
	cmd.AddCommand(projectSwitchCmd(app))

	return cmd
}

// GenerateProjectID generates a deterministic project ID from an absolute path.
// Format: "proj-" + first 8 chars of SHA-256 hex of the path.
func GenerateProjectID(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("proj-%x", h[:4])
}

func projectAddCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register a project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			// Verify directory exists
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat path: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", path)
			}

			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				name = filepath.Base(path)
			}

			projectID := GenerateProjectID(path)
			now := time.Now().UTC().Format(time.RFC3339)

			ctx := context.Background()
			err = app.DB.Exec(ctx, `
				INSERT INTO projects (id, name, path, active, canonical_branch, registered_at, last_accessed_at)
				VALUES (?, ?, ?, 1, 'main', ?, ?)
			`, projectID, name, path, now, now)
			if err != nil {
				return fmt.Errorf("register project: %w (project may already be registered)", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"success": true,
					"command": "project add",
					"project": map[string]any{
						"id":            projectID,
						"name":          name,
						"path":          path,
						"active":        true,
						"registered_at": now,
					},
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("Registered project %q (%s) as %s\n", name, path, projectID)
			}

			return nil
		},
	}
	cmd.Flags().String("name", "", "Override display name (default: basename of path)")
	return cmd
}

func projectRemoveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <path|id>",
		Short: "Unregister a project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			identifier := args[0]

			if !force {
				fmt.Printf("Remove project %q? This will not delete any files. [y/N] ", identifier)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			ctx := context.Background()

			// Try by path first (resolve to absolute), then by ID
			absPath, _ := filepath.Abs(identifier)
			err := app.DB.Exec(ctx, `DELETE FROM projects WHERE path = ? OR id = ?`, absPath, identifier)
			if err != nil {
				return fmt.Errorf("remove project: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"success": true,
					"command": "project remove",
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				fmt.Printf("Removed project %q\n", identifier)
			}

			return nil
		},
	}
	cmd.Flags().Bool("force", false, "Skip confirmation")
	return cmd
}

func projectListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			activeOnly, _ := cmd.Flags().GetBool("active")

			ctx := context.Background()
			query := `SELECT id, name, path, active, canonical_branch, registered_at, last_accessed_at, migrated_at FROM projects`
			if activeOnly {
				query += ` WHERE active = 1`
			}
			query += ` ORDER BY last_accessed_at DESC`

			rows, err := app.DB.Query(ctx, query)
			if err != nil {
				return fmt.Errorf("query projects: %w", err)
			}
			defer rows.Close()

			type projectRow struct {
				ID              string  `json:"id"`
				Name            string  `json:"name"`
				Path            string  `json:"path"`
				Active          bool    `json:"active"`
				CanonicalBranch string  `json:"canonical_branch"`
				RegisteredAt    string  `json:"registered_at"`
				LastAccessedAt  string  `json:"last_accessed_at"`
				MigratedAt      *string `json:"migrated_at,omitempty"`
			}

			var projects []projectRow
			for rows.Next() {
				var p projectRow
				var active int
				var migratedAt *string
				if err := rows.Scan(&p.ID, &p.Name, &p.Path, &active, &p.CanonicalBranch, &p.RegisteredAt, &p.LastAccessedAt, &migratedAt); err != nil {
					return fmt.Errorf("scan project: %w", err)
				}
				p.Active = active != 0
				p.MigratedAt = migratedAt
				projects = append(projects, p)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"success":  true,
					"command":  "project list",
					"projects": projects,
					"count":    len(projects),
				}
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
			} else {
				if len(projects) == 0 {
					fmt.Println("No registered projects.")
					return nil
				}
				for _, p := range projects {
					activeStr := ""
					if p.Active {
						activeStr = " [active]"
					}
					fmt.Printf("%-12s %-30s %s%s\n", p.ID, p.Name, p.Path, activeStr)
				}
			}

			return nil
		},
	}
	cmd.Flags().Bool("active", false, "Show only projects with active sessions")
	return cmd
}

func projectSwitchCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "switch <path|id>",
		Short: "Switch active project context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identifier := args[0]
			ctx := context.Background()

			// Resolve identifier to a project
			absPath, _ := filepath.Abs(identifier)
			now := time.Now().UTC().Format(time.RFC3339)

			err := app.DB.Exec(ctx, `
				UPDATE projects SET last_accessed_at = ? WHERE path = ? OR id = ?
			`, now, absPath, identifier)
			if err != nil {
				return fmt.Errorf("switch project: %w", err)
			}

			// Write session-switch.json for agent wrapper / FP sync
			home, _ := os.UserHomeDir()
			switchFile := filepath.Join(home, ".computecommander", "session-switch.json")
			switchData := map[string]string{
				"project_path": absPath,
				"switched_at":  now,
			}
			data, _ := json.MarshalIndent(switchData, "", "  ")
			_ = os.WriteFile(switchFile, data, 0o644)

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"success": true,
					"command": "project switch",
					"project": map[string]any{
						"path": absPath,
					},
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			} else {
				fmt.Printf("Switched to project at %s\n", absPath)
			}

			return nil
		},
	}
}
