package commands

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/pkg/runtimes"
)

// generateSessionID creates a unique session ID in the format "{runtime}-{8 hex chars}".
func generateSessionID(runtime string) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%08x", runtime, os.Getpid())
	}
	return fmt.Sprintf("%s-%x", runtime, b)
}

// RegisterCmd returns the "register" command for registering a new agent session.
func RegisterCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "register",
		Short:   "Register a new agent session",
		Long:    "Register a new agent session in the database. Returns the session ID for use with heartbeat and deregister.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name, _ := cmd.Flags().GetString("name")
			runtime, _ := cmd.Flags().GetString("runtime")
			capability, _ := cmd.Flags().GetString("capability")
			taskID, _ := cmd.Flags().GetString("task")
			pid, _ := cmd.Flags().GetInt("pid")
			parent, _ := cmd.Flags().GetString("parent")
			worktreePath, _ := cmd.Flags().GetString("worktree")
			branch, _ := cmd.Flags().GetString("branch")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if runtime == "" {
				return fmt.Errorf("--runtime is required")
			}
			if capability == "" {
				return fmt.Errorf("--capability is required")
			}
			if taskID == "" {
				return fmt.Errorf("--task is required")
			}

			sessionID := generateSessionID(runtime)
			now := time.Now().UTC()

			err := app.DB.Exec(ctx,
				`INSERT INTO sessions (id, agent_name, capability, worktree_path, branch_name,
					task_id, state, pid, parent_agent, depth, run_id,
					started_at, last_activity, escalation_level,
					transcript_path, runtime, heartbeat_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
				sessionID, name, capability, worktreePath, branch,
				taskID, string(agents.StateBooting), pid, parent, 0, "",
				now.Format("2006-01-02T15:04:05Z"), now.Format("2006-01-02T15:04:05Z"), 0,
				"", runtime, now.Format("2006-01-02T15:04:05Z"),
			)
			if err != nil {
				if jsonOut {
					return printJSON(map[string]any{
						"success": false,
						"command": "register",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("register session: %w", err)
			}

			result := map[string]any{
				"success":    true,
				"command":    "register",
				"session_id": sessionID,
				"agent_name": name,
				"runtime":    runtime,
				"state":      string(agents.StateBooting),
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Registered session %s (agent=%s, runtime=%s)\n", sessionID, name, runtime)
			return nil
		},
	}

	cmd.Flags().String("name", "", "Agent name (required)")
	cmd.Flags().String("runtime", "", "Runtime: claude, pi, gemini, codex, goose (required)")
	cmd.Flags().String("capability", "", "Capability: scout, builder, reviewer, lead, etc. (required)")
	cmd.Flags().String("task", "", "Task identifier (required)")
	cmd.Flags().Int("pid", 0, "OS process ID for liveness checks")
	cmd.Flags().String("parent", "", "Parent agent (for subagent tracking)")
	cmd.Flags().String("worktree", "", "Worktree path")
	cmd.Flags().String("branch", "", "Git branch")

	return cmd
}

// DeregisterCmd returns the "deregister" command for marking an agent session as completed.
func DeregisterCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deregister <session-id>",
		Short: "Mark an agent session as completed",
		Long:  "Mark an agent session as completed or zombie in the database.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sessionID := args[0]
			finalState, _ := cmd.Flags().GetString("state")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if finalState == "" {
				finalState = string(agents.StateCompleted)
			}

			// Validate state.
			if finalState != string(agents.StateCompleted) && finalState != string(agents.StateZombie) {
				return fmt.Errorf("--state must be 'completed' or 'zombie'")
			}

			now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
			err := app.DB.Exec(ctx,
				"UPDATE sessions SET state = $1, last_activity = $2 WHERE id = $3",
				finalState, now, sessionID,
			)
			if err != nil {
				if jsonOut {
					return printJSON(map[string]any{
						"success": false,
						"command": "deregister",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("deregister session: %w", err)
			}

			result := map[string]any{
				"success":     true,
				"command":     "deregister",
				"session_id":  sessionID,
				"final_state": finalState,
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Deregistered session %s (state=%s)\n", sessionID, finalState)
			return nil
		},
	}

	cmd.Flags().String("state", "completed", "Final state: completed (default) or zombie")
	cmd.Flags().String("reason", "", "Reason for deregistration")

	return cmd
}

// HeartbeatCmd returns the "heartbeat" command for updating agent heartbeat timestamp.
func HeartbeatCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heartbeat <session-id>",
		Short: "Update agent heartbeat timestamp",
		Long:  "Update the heartbeat_at and last_activity timestamps for an agent session.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sessionID := args[0]
			state, _ := cmd.Flags().GetString("state")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

			if state != "" {
				// Validate state if provided.
				if !agents.ValidSessionState(agents.SessionState(state)) {
					return fmt.Errorf("invalid state: %q", state)
				}
				err := app.DB.Exec(ctx,
					"UPDATE sessions SET heartbeat_at = $1, last_activity = $2, state = $3 WHERE id = $4",
					now, now, state, sessionID,
				)
				if err != nil {
					if jsonOut {
						return printJSON(map[string]any{
							"success": false,
							"command": "heartbeat",
							"error":   err.Error(),
						})
					}
					return fmt.Errorf("heartbeat: %w", err)
				}
			} else {
				err := app.DB.Exec(ctx,
					"UPDATE sessions SET heartbeat_at = $1, last_activity = $2 WHERE id = $3",
					now, now, sessionID,
				)
				if err != nil {
					if jsonOut {
						return printJSON(map[string]any{
							"success": false,
							"command": "heartbeat",
							"error":   err.Error(),
						})
					}
					return fmt.Errorf("heartbeat: %w", err)
				}
			}

			result := map[string]any{
				"success":      true,
				"command":      "heartbeat",
				"session_id":   sessionID,
				"heartbeat_at": now,
			}
			if state != "" {
				result["state"] = state
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Heartbeat updated for session %s\n", sessionID)
			return nil
		},
	}

	cmd.Flags().String("state", "", "Optionally update state (working, stalled)")
	cmd.Flags().Int("tokens-in", 0, "Input tokens consumed since last heartbeat")
	cmd.Flags().Int("tokens-out", 0, "Output tokens consumed since last heartbeat")

	// Suppress unused flag warnings.
	_ = runtimes.RuntimeID("")

	return cmd
}
