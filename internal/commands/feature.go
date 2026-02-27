package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// FeatureCmd returns the "feature" command for feature flag management.
func FeatureCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "feature",
		Short:   "Feature flag management",
		Long:    "View and toggle feature flags for ComputeCommander.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(featureListCmd(app))
	cmd.AddCommand(featureToggleCmd(app))

	return cmd
}

func featureListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all feature flags and their states",
		RunE: func(cmd *cobra.Command, args []string) error {
			features := map[string]bool{
				"distributed":  app.Config.Features.Distributed,
				"remote_agents": app.Config.Features.RemoteAgents,
				"ai_resolve":    app.Config.Merge.AIResolveEnabled,
				"reimagine":     app.Config.Merge.ReimagineEnabled,
				"auto_merge":    app.Config.Merge.AutoMerge,
				"tier0_watchdog": app.Config.Watchdog.Tier0Enabled,
				"tier1_watchdog": app.Config.Watchdog.Tier1Enabled,
				"tier2_watchdog": app.Config.Watchdog.Tier2Enabled,
				"loop_detection": app.Config.Nudge.LoopDetection.Enabled,
			}

			fmt.Printf("%-20s %s\n", "FEATURE", "ENABLED")
			for name, enabled := range features {
				state := "off"
				if enabled {
					state = "on"
				}
				fmt.Printf("%-20s %s\n", name, state)
			}
			return nil
		},
	}
}

func featureToggleCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "toggle <feature>",
		Short: "Toggle a feature flag (runtime only, does not persist to config)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feature := args[0]

			switch feature {
			case "distributed":
				app.Config.Features.Distributed = !app.Config.Features.Distributed
				fmt.Printf("distributed = %v\n", app.Config.Features.Distributed)
			case "remote_agents":
				app.Config.Features.RemoteAgents = !app.Config.Features.RemoteAgents
				fmt.Printf("remote_agents = %v\n", app.Config.Features.RemoteAgents)
			case "ai_resolve":
				app.Config.Merge.AIResolveEnabled = !app.Config.Merge.AIResolveEnabled
				fmt.Printf("ai_resolve = %v\n", app.Config.Merge.AIResolveEnabled)
			case "reimagine":
				app.Config.Merge.ReimagineEnabled = !app.Config.Merge.ReimagineEnabled
				fmt.Printf("reimagine = %v\n", app.Config.Merge.ReimagineEnabled)
			case "auto_merge":
				app.Config.Merge.AutoMerge = !app.Config.Merge.AutoMerge
				fmt.Printf("auto_merge = %v\n", app.Config.Merge.AutoMerge)
			default:
				return fmt.Errorf("unknown feature: %q", feature)
			}

			fmt.Println("(Note: change is runtime-only and not persisted to config file)")
			return nil
		},
	}
}
