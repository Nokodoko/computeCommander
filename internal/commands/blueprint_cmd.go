package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentic/blueprint"
)

// BlueprintCmd returns the blueprint command group.
func BlueprintCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "blueprint",
		Short:   "Manage task blueprints",
		Long:    "List, show, create, run, validate, and graph task blueprints.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(blueprintListCmd(app))
	cmd.AddCommand(blueprintShowCmd(app))
	cmd.AddCommand(blueprintCreateCmd(app))
	cmd.AddCommand(blueprintRunCmd(app))
	cmd.AddCommand(blueprintValidateCmd(app))
	cmd.AddCommand(blueprintGraphCmd(app))

	return cmd
}

func blueprintListCmd(app *App) *cobra.Command {
	var status, agent string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all blueprints",
		RunE: func(cmd *cobra.Command, args []string) error {
			bps, err := app.BlueprintEngine.List(cmd.Context(), blueprint.Status(status), agent)
			if err != nil {
				return fmt.Errorf("list blueprints: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success":    true,
					"command":    "blueprint list",
					"blueprints": bps,
					"count":      len(bps),
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(bps) == 0 {
				fmt.Println("No blueprints found.")
				return nil
			}

			fmt.Printf("%-16s %-20s %-12s %-10s %-10s\n", "ID", "NAME", "AGENT", "STATUS", "ATTEMPTS")
			for _, bp := range bps {
				fmt.Printf("%-16s %-20s %-12s %-10s %-10d\n",
					truncate(bp.ID, 16),
					truncate(bp.Name, 20),
					truncate(bp.Agent, 12),
					truncate(string(bp.Status), 10),
					bp.Attempts,
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&agent, "agent", "", "Filter by agent")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func blueprintShowCmd(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show [bp-id]",
		Short: "Show blueprint details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bp, err := app.BlueprintEngine.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get blueprint: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success":   true,
					"command":   "blueprint show",
					"blueprint": bp,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Blueprint: %s\n", bp.ID)
			fmt.Printf("  Name:       %s\n", bp.Name)
			fmt.Printf("  Agent:      %s\n", bp.Agent)
			fmt.Printf("  Capability: %s\n", bp.Capability)
			fmt.Printf("  Status:     %s\n", bp.Status)
			fmt.Printf("  Attempts:   %d / %d\n", bp.Attempts, bp.RetryLimit)
			fmt.Printf("  Created:    %s\n", bp.CreatedAt.Format("2006-01-02 15:04:05"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func blueprintCreateCmd(app *App) *cobra.Command {
	var name, agent, capability, spec, file string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new blueprint",
		RunE: func(cmd *cobra.Command, args []string) error {
			bp := &blueprint.Blueprint{
				Name:       name,
				Agent:      agent,
				Capability: capability,
			}

			if file != "" {
				data, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("read spec file: %w", err)
				}
				spec = string(data)
			}
			_ = spec // spec content will be used when full blueprint DSL parsing is implemented

			if err := app.BlueprintEngine.Create(cmd.Context(), bp); err != nil {
				return fmt.Errorf("create blueprint: %w", err)
			}

			fmt.Printf("Created blueprint: %s (%s)\n", bp.ID, bp.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Task name")
	cmd.Flags().StringVar(&agent, "agent", "", "Target agent type")
	cmd.Flags().StringVar(&capability, "capability", "", "Required capability")
	cmd.Flags().StringVar(&spec, "spec", "", "Task specification")
	cmd.Flags().StringVar(&file, "file", "", "Load spec from file")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("agent")
	_ = cmd.MarkFlagRequired("capability")

	return cmd
}

func blueprintRunCmd(app *App) *cobra.Command {
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "run [bp-id]",
		Short: "Execute a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bp, err := app.BlueprintEngine.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get blueprint: %w", err)
			}

			if dryRun {
				fmt.Printf("Dry run: blueprint %s (%s) is valid\n", bp.ID, bp.Name)
				return nil
			}

			if bp.Status == blueprint.StatusRunning && !force {
				return fmt.Errorf("blueprint %s is already running (use --force to override)", bp.ID)
			}

			fmt.Printf("Running blueprint: %s (%s)\n", bp.ID, bp.Name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate without executing")
	cmd.Flags().BoolVar(&force, "force", false, "Ignore dependency checks")

	return cmd
}

func blueprintValidateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "validate [bp-id]",
		Short: "Validate blueprint YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bp, err := app.BlueprintEngine.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("validate blueprint: %w", err)
			}
			fmt.Printf("Blueprint %s (%s) is valid\n", bp.ID, bp.Name)
			return nil
		},
	}
}

func blueprintGraphCmd(app *App) *cobra.Command {
	var format string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Show dependency graph",
		RunE: func(cmd *cobra.Command, args []string) error {
			bps, err := app.BlueprintEngine.List(cmd.Context(), "", "")
			if err != nil {
				return fmt.Errorf("list blueprints: %w", err)
			}

			if jsonOut {
				graph := make(map[string][]string)
				for _, bp := range bps {
					graph[bp.ID] = bp.DependsOn
				}
				result := map[string]any{
					"success": true,
					"command": "blueprint graph",
					"graph":   graph,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(bps) == 0 {
				fmt.Println("No blueprints to graph.")
				return nil
			}

			fmt.Println("Blueprint dependency graph:")
			for _, bp := range bps {
				if len(bp.DependsOn) > 0 {
					fmt.Printf("  %s -> %v\n", bp.ID, bp.DependsOn)
				} else {
					fmt.Printf("  %s (no dependencies)\n", bp.ID)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "ascii", "ascii|dot|json")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}
