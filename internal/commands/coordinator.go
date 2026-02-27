package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agents"
	"github.com/noko/computecommander/pkg/runtimes"
)

// CoordinatorCmd returns the "coordinator" command for persistent orchestrator lifecycle.
func CoordinatorCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "coordinator",
		Short:   "Persistent orchestrator lifecycle",
		Long:    "Spawn or manage a coordinator agent that orchestrates other agents.",
		GroupID: "COORDINATION",
	}

	cmd.AddCommand(coordinatorStartCmd(app))
	cmd.AddCommand(coordinatorStatusCmd(app))

	return cmd
}

func coordinatorStartCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a coordinator agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task")
			rt, _ := cmd.Flags().GetString("runtime")
			specPath, _ := cmd.Flags().GetString("spec")

			if taskID == "" {
				return fmt.Errorf("--task is required")
			}

			if rt == "" {
				rt = app.Config.Defaults.Runtime
			}

			req := agents.SpawnRequest{
				TaskID:     taskID,
				Capability: agents.CapCoordinator,
				Name:       "coordinator",
				Runtime:    runtimes.RuntimeID(rt),
				SpecPath:   specPath,
			}

			result, err := app.Spawner.Spawn(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("spawn coordinator: %w", err)
			}

			fmt.Printf("Coordinator started\n")
			fmt.Printf("  Session:  %s\n", result.Session.ID)
			fmt.Printf("  Worktree: %s\n", result.WorktreePath)
			return nil
		},
	}

	cmd.Flags().String("task", "", "Task ID (required)")
	cmd.Flags().String("runtime", "", "Runtime to use")
	cmd.Flags().String("spec", "", "Path to spec file")

	return cmd
}

func coordinatorStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show coordinator status",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := app.Spawner.ListSessions(cmd.Context(), agents.ListOpts{
				Capability: agents.CapCoordinator,
			})
			if err != nil {
				return fmt.Errorf("list coordinator sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No coordinator running.")
				return nil
			}

			for _, s := range sessions {
				fmt.Printf("Coordinator: %s (state: %s, task: %s)\n",
					s.AgentName, s.State, s.TaskID)
			}
			return nil
		},
	}
}

// MonitorCmd returns the "monitor" command for Tier 2 monitor agents.
func MonitorCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "monitor",
		Short:   "Tier 2 monitor agent",
		Long:    "Spawn or manage a Tier 2 monitor agent that detects patterns and intervenes proactively.",
		GroupID: "COORDINATION",
	}

	cmd.AddCommand(monitorStartCmd(app))
	cmd.AddCommand(monitorStatusCmd(app))

	return cmd
}

func monitorStartCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a monitor agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, _ := cmd.Flags().GetString("task")
			rt, _ := cmd.Flags().GetString("runtime")

			if taskID == "" {
				return fmt.Errorf("--task is required")
			}

			if rt == "" {
				rt = app.Config.Defaults.Runtime
			}

			req := agents.SpawnRequest{
				TaskID:     taskID,
				Capability: agents.CapMonitor,
				Name:       "monitor",
				Runtime:    runtimes.RuntimeID(rt),
			}

			result, err := app.Spawner.Spawn(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("spawn monitor: %w", err)
			}

			fmt.Printf("Monitor started\n")
			fmt.Printf("  Session:  %s\n", result.Session.ID)
			fmt.Printf("  Worktree: %s\n", result.WorktreePath)
			return nil
		},
	}

	cmd.Flags().String("task", "", "Task ID (required)")
	cmd.Flags().String("runtime", "", "Runtime to use")

	return cmd
}

func monitorStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show monitor status",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := app.Spawner.ListSessions(cmd.Context(), agents.ListOpts{
				Capability: agents.CapMonitor,
			})
			if err != nil {
				return fmt.Errorf("list monitor sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No monitor running.")
				return nil
			}

			for _, s := range sessions {
				fmt.Printf("Monitor: %s (state: %s, task: %s)\n",
					s.AgentName, s.State, s.TaskID)
			}
			return nil
		},
	}
}
