package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/pkg/runtimes"
)

// SlingCmd returns the "sling" command for spawning a worker agent.
func SlingCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sling <name>",
		Short:   "Spawn worker agent",
		Long:    "Spawn a new agent in an isolated worktree. Requires a task ID and agent name.",
		GroupID: "CORE",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			taskID, _ := cmd.Flags().GetString("task")
			capability, _ := cmd.Flags().GetString("capability")
			rt, _ := cmd.Flags().GetString("runtime")
			parent, _ := cmd.Flags().GetString("parent")
			depth, _ := cmd.Flags().GetInt("depth")
			specPath, _ := cmd.Flags().GetString("spec")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if taskID == "" {
				return fmt.Errorf("--task is required")
			}

			if rt == "" {
				rt = app.Config.Defaults.Runtime
			}

			req := agents.SpawnRequest{
				TaskID:     taskID,
				Capability: agents.Capability(capability),
				Name:       name,
				Runtime:    runtimes.RuntimeID(rt),
				Parent:     parent,
				Depth:      depth,
				SpecPath:   specPath,
			}

			result, err := app.Spawner.Spawn(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("spawn agent: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Spawned agent %q\n", name)
			fmt.Printf("  Session:  %s\n", result.Session.ID)
			fmt.Printf("  Worktree: %s\n", result.WorktreePath)
			fmt.Printf("  Pane:     %s\n", result.ZellijPane)
			return nil
		},
	}

	cmd.Flags().String("task", "", "Task ID (required)")
	cmd.Flags().String("capability", "builder", "Agent capability (scout, builder, reviewer, lead, merger, coordinator, supervisor, monitor)")
	cmd.Flags().String("runtime", "", "Runtime to use (default: from config)")
	cmd.Flags().String("parent", "", "Parent agent name")
	cmd.Flags().Int("depth", 0, "Agent depth in hierarchy")
	cmd.Flags().String("spec", "", "Path to task spec file")

	return cmd
}
