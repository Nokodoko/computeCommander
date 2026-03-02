package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/backup"
	"github.com/noko/computecommander/internal/export"
)

// ExportCmd returns the "export" command for exporting data from the database.
func ExportCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "export",
		Short:   "Export all data from DB as JSON",
		Long:    "Export sessions, events, mail, merge queue, metrics, and runs as JSON or CSV.",
		GroupID: "DATA",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			output, _ := cmd.Flags().GetString("output")
			tablesStr, _ := cmd.Flags().GetString("tables")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			var tables []string
			if tablesStr != "" {
				tables = strings.Split(tablesStr, ",")
			}

			var writer *os.File
			if output != "" && output != "-" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				defer f.Close()
				writer = f
			} else {
				writer = os.Stdout
			}

			result, err := export.Export(cmd.Context(), app.DB, export.ExportOpts{
				Format:  format,
				Tables:  tables,
				Writer:  writer,
				Version: app.Version,
				Project: app.Config.Project.Name,
			})
			if err != nil {
				return fmt.Errorf("export: %w", err)
			}

			if jsonOut && output != "" {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"command":    "export",
					"exportedAt": result.ExportedAt,
					"tables":     result.Tables,
					"totalRows":  result.TotalRows,
				})
			}

			if output != "" {
				fmt.Fprintf(os.Stderr, "Exported %d rows to %s\n", result.TotalRows, output)
			}
			return nil
		},
	}

	cmd.Flags().String("format", "json", "Output format: json|csv")
	cmd.Flags().String("output", "", "Output file (default: stdout)")
	cmd.Flags().String("tables", "", "Comma-separated table list (default: all)")

	return cmd
}

// BackupCmd returns the "backup" command for backing up the database.
func BackupCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "backup",
		Short:   "Backup DB file (confirmation-gated)",
		Long:    "Create a backup of the SQLite database file.",
		GroupID: "DATA",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction("Create a database backup?") {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "backup",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			dbPath := app.Config.Database.SQLite.Path
			if dbPath == "" {
				dbPath = ".computecommander/local.db"
			}

			result, err := backup.Backup(dbPath, output)
			if err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "backup",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("backup: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":   true,
					"command":   "backup",
					"id":        result.ID,
					"path":      result.Path,
					"sizeBytes": result.SizeBytes,
				})
			}

			fmt.Printf("Backup created: %s (%d bytes)\n", result.Path, result.SizeBytes)
			return nil
		},
	}

	cmd.Flags().String("output", "", "Backup destination directory (default: .computecommander/backups/)")
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}

// RestoreCmd returns the "restore" command for restoring from a backup.
func RestoreCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restore <path>",
		Short:   "Restore DB from backup (confirmation-gated)",
		Long:    "Restore the database from a backup file. The current database will be overwritten.",
		GroupID: "DATA",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backupPath := args[0]
			force, _ := cmd.Flags().GetBool("force")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if !force {
				if !confirmAction(fmt.Sprintf("Restore database from %s? This will overwrite the current database.", backupPath)) {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success": false,
							"command": "restore",
							"error":   "cancelled by user",
						})
					}
					fmt.Println("Cancelled.")
					return nil
				}
			}

			dbPath := app.Config.Database.SQLite.Path
			if dbPath == "" {
				dbPath = ".computecommander/local.db"
			}

			// Close current DB connection before restore.
			if app.DB != nil {
				_ = app.DB.Close()
			}

			result, err := backup.Restore(backupPath, dbPath)
			if err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "restore",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("restore: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":    true,
					"command":    "restore",
					"backupPath": result.BackupPath,
					"restoredAt": result.RestoredAt,
				})
			}

			fmt.Printf("Database restored from %s\n", result.BackupPath)
			fmt.Println("Please restart cmdr to use the restored database.")
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}
