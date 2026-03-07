package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/noko/computecommander/internal/platform/db"
)

// MigrateCmd returns the "migrate" command for one-time migration from local DBs.
func MigrateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "migrate",
		Short:   "Migrate data from per-project local databases to system-wide DB",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			projectFilter, _ := cmd.Flags().GetString("project")
			return runMigrate(app, dryRun, projectFilter, cmd)
		},
	}
	cmd.Flags().Bool("dry-run", false, "Show what would be migrated without writing")
	cmd.Flags().String("project", "", "Migrate a specific project only")
	return cmd
}

func runMigrate(app *App, dryRun bool, projectFilter string, cmd *cobra.Command) error {
	// Discover local databases
	var projectDirs []string

	if projectFilter != "" {
		absPath, err := filepath.Abs(projectFilter)
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		projectDirs = append(projectDirs, absPath)
	} else {
		// Query registered projects from system DB
		ctx := context.Background()
		rows, err := app.DB.Query(ctx, `SELECT path FROM projects WHERE migrated_at IS NULL`)
		if err != nil {
			return fmt.Errorf("query projects: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return fmt.Errorf("scan project path: %w", err)
			}
			projectDirs = append(projectDirs, path)
		}

		// Also check CWD if not already registered
		wd, _ := os.Getwd()
		localDB := filepath.Join(wd, ".computecommander", "local.db")
		if _, err := os.Stat(localDB); err == nil {
			found := false
			for _, p := range projectDirs {
				if p == wd {
					found = true
					break
				}
			}
			if !found {
				projectDirs = append(projectDirs, wd)
			}
		}
	}

	type migrationResult struct {
		Project              string `json:"project"`
		Path                 string `json:"path"`
		SessionsImported     int    `json:"sessions_imported"`
		EventsImported       int    `json:"events_imported"`
		MailImported         int    `json:"mail_imported"`
		StaleSessZombie      int    `json:"stale_sessions_marked_zombie"`
	}

	var results []migrationResult

	for _, projectPath := range projectDirs {
		localDBPath := filepath.Join(projectPath, ".computecommander", "local.db")
		if _, err := os.Stat(localDBPath); err != nil {
			continue // No local DB, skip
		}

		// Check if already migrated
		home, _ := os.UserHomeDir()
		pathHash := sha256.Sum256([]byte(projectPath))
		migrationMarker := filepath.Join(home, ".computecommander", "migrations", "completed", hex.EncodeToString(pathHash[:])+".migrated")
		if _, err := os.Stat(migrationMarker); err == nil {
			continue // Already migrated
		}

		projectName := filepath.Base(projectPath)
		result := migrationResult{
			Project: projectName,
			Path:    projectPath,
		}

		if dryRun {
			// Count records in local DB
			localDB, err := db.NewSQLite(localDBPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not open %s: %v\n", localDBPath, err)
				continue
			}
			ctx := context.Background()
			rows, _ := localDB.Query(ctx, `SELECT COUNT(*) FROM sessions`)
			if rows != nil && rows.Next() {
				rows.Scan(&result.SessionsImported)
				rows.Close()
			}
			rows, _ = localDB.Query(ctx, `SELECT COUNT(*) FROM events`)
			if rows != nil && rows.Next() {
				rows.Scan(&result.EventsImported)
				rows.Close()
			}
			rows, _ = localDB.Query(ctx, `SELECT COUNT(*) FROM mail`)
			if rows != nil && rows.Next() {
				rows.Scan(&result.MailImported)
				rows.Close()
			}
			localDB.Close()
			results = append(results, result)
			continue
		}

		// Actual migration
		localDB, err := db.NewSQLite(localDBPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open %s: %v\n", localDBPath, err)
			continue
		}

		// Ensure project is registered
		projectID := GenerateProjectID(projectPath)
		now := time.Now().UTC().Format(time.RFC3339)

		ctx := context.Background()
		_ = app.DB.Exec(ctx, `
			INSERT OR IGNORE INTO projects (id, name, path, active, canonical_branch, registered_at, last_accessed_at, migrated_at)
			VALUES (?, ?, ?, 1, 'main', ?, ?, ?)
		`, projectID, projectName, projectPath, now, now, now)

		// Import runs
		rows, err := localDB.Query(ctx, `SELECT id, started_at, completed_at, agent_count, coordinator_session_id, status FROM runs`)
		if err == nil {
			for rows.Next() {
				var id, startedAt, status string
				var completedAt, coordinatorSessionID *string
				var agentCount int
				if err := rows.Scan(&id, &startedAt, &completedAt, &agentCount, &coordinatorSessionID, &status); err != nil {
					continue
				}
				_ = app.DB.Exec(ctx, `
					INSERT OR IGNORE INTO runs (id, started_at, completed_at, agent_count, coordinator_session_id, status, project_id)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`, id, startedAt, completedAt, agentCount, coordinatorSessionID, status, projectID)
			}
			rows.Close()
		}

		// Import sessions (with staleness check)
		rows, err = localDB.Query(ctx, `SELECT id, agent_name, capability, worktree_path, branch_name, task_id, zellij_pane, state, pid, parent_agent, depth, run_id, started_at, last_activity, escalation_level, stalled_since, transcript_path, runtime FROM sessions`)
		if err == nil {
			for rows.Next() {
				var id, agentName, capability, taskID, state, runtime string
				var worktreePath, branchName, zellijPane, parentAgent, stalledSince, transcriptPath, runID *string
				var pid, depth, escalationLevel int
				var startedAt, lastActivity string
				if err := rows.Scan(&id, &agentName, &capability, &worktreePath, &branchName, &taskID, &zellijPane, &state, &pid, &parentAgent, &depth, &runID, &startedAt, &lastActivity, &escalationLevel, &stalledSince, &transcriptPath, &runtime); err != nil {
					continue
				}

				// Staleness check: if working/booting with dead PID, mark zombie
				importState := state
				if (state == "working" || state == "booting") && pid > 0 {
					if !isProcessAlive(pid) {
						importState = "zombie"
						result.StaleSessZombie++
					}
				}

				_ = app.DB.Exec(ctx, `
					INSERT OR IGNORE INTO sessions (id, agent_name, capability, worktree_path, branch_name, task_id, zellij_pane, state, pid, parent_agent, depth, run_id, started_at, last_activity, escalation_level, stalled_since, transcript_path, runtime, project_id, color_index, color_hex)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '#808080')
				`, id, agentName, capability, worktreePath, branchName, taskID, zellijPane, importState, pid, parentAgent, depth, runID, startedAt, lastActivity, escalationLevel, stalledSince, transcriptPath, runtime, projectID)
				result.SessionsImported++
			}
			rows.Close()
		}

		// Import events
		rows, err = localDB.Query(ctx, `SELECT run_id, agent_name, session_id, event_type, tool_name, tool_args, tool_duration_ms, level, data, created_at FROM events`)
		if err == nil {
			for rows.Next() {
				var runID, agentName, eventType, level string
				var sessionID, toolName, toolArgs, data, createdAt *string
				var toolDurationMs *int
				if err := rows.Scan(&runID, &agentName, &sessionID, &eventType, &toolName, &toolArgs, &toolDurationMs, &level, &data, &createdAt); err != nil {
					continue
				}
				_ = app.DB.Exec(ctx, `
					INSERT INTO events (run_id, agent_name, session_id, event_type, tool_name, tool_args, tool_duration_ms, level, data, created_at, project_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, runID, agentName, sessionID, eventType, toolName, toolArgs, toolDurationMs, level, data, createdAt, projectID)
				result.EventsImported++
			}
			rows.Close()
		}

		// Import mail
		rows, err = localDB.Query(ctx, `SELECT id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at FROM mail`)
		if err == nil {
			for rows.Next() {
				var id, fromAgent, toAgent, subject, body, priority, mailType string
				var threadID, payload *string
				var read int
				var createdAt string
				if err := rows.Scan(&id, &fromAgent, &toAgent, &subject, &body, &priority, &mailType, &threadID, &payload, &read, &createdAt); err != nil {
					continue
				}
				_ = app.DB.Exec(ctx, `
					INSERT OR IGNORE INTO mail (id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at, project_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, id, fromAgent, toAgent, subject, body, priority, mailType, threadID, payload, read, createdAt, projectID)
				result.MailImported++
			}
			rows.Close()
		}

		localDB.Close()

		// Mark as migrated in local DB
		localDB2, err := db.NewSQLite(localDBPath)
		if err == nil {
			_ = localDB2.Exec(ctx, `CREATE TABLE IF NOT EXISTS MIGRATED_TO_SYSTEM_DB (migrated_at TEXT)`)
			_ = localDB2.Exec(ctx, `INSERT INTO MIGRATED_TO_SYSTEM_DB VALUES (?)`, now)
			localDB2.Close()
		}

		// Write migration marker
		os.MkdirAll(filepath.Dir(migrationMarker), 0o755)
		os.WriteFile(migrationMarker, []byte(now+"\n"), 0o644)

		// Update project migrated_at
		_ = app.DB.Exec(ctx, `UPDATE projects SET migrated_at = ? WHERE id = ?`, now, projectID)

		results = append(results, result)
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		output := map[string]any{
			"success":        true,
			"command":        "migrate",
			"migrated":       results,
			"total_projects": len(results),
		}
		if dryRun {
			output["dry_run"] = true
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	} else {
		if dryRun {
			fmt.Println("Dry run - no changes written:")
		}
		if len(results) == 0 {
			fmt.Println("No local databases found to migrate.")
			return nil
		}
		for _, r := range results {
			fmt.Printf("  %s (%s)\n", r.Project, r.Path)
			fmt.Printf("    Sessions: %d, Events: %d, Mail: %d", r.SessionsImported, r.EventsImported, r.MailImported)
			if r.StaleSessZombie > 0 {
				fmt.Printf(", Stale->Zombie: %d", r.StaleSessZombie)
			}
			fmt.Println()
		}
	}

	return nil
}

// isProcessAlive is declared in status.go
