package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ThemeCmd returns the "theme" command for theme management.
func ThemeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "theme",
		Short:   "Theme management",
		GroupID: "SETTINGS",
	}

	cmd.AddCommand(themeListCmd(app))
	cmd.AddCommand(themeSetCmd(app))
	cmd.AddCommand(themeEditCmd(app))

	return cmd
}

func themeListCmd(app *App) *cobra.Command {
	_ = app // reserved: app preserved for symmetry with sibling theme handlers
	return &cobra.Command{
		Use:   "list",
		Short: "List available themes",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			themesDir := filepath.Join(".computecommander", "themes")
			entries, err := os.ReadDir(themesDir)
			if err != nil {
				if os.IsNotExist(err) {
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"themes": []string{"default"},
						})
					}
					fmt.Println("Available themes: default")
					return nil
				}
				return fmt.Errorf("read themes directory: %w", err)
			}

			var themes []string
			themes = append(themes, "default")
			for _, entry := range entries {
				if !entry.IsDir() && filepath.Ext(entry.Name()) == ".yaml" {
					name := entry.Name()[:len(entry.Name())-5]
					if name != "default" {
						themes = append(themes, name)
					}
				}
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"themes": themes,
				})
			}

			fmt.Println("Available themes:")
			for _, t := range themes {
				fmt.Printf("  %s\n", t)
			}
			return nil
		},
	}
}

func themeSetCmd(app *App) *cobra.Command {
	_ = app // reserved: app preserved for symmetry with sibling theme handlers
	return &cobra.Command{
		Use:   "set <name>",
		Short: "Apply theme",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			// Verify theme file exists.
			themePath := filepath.Join(".computecommander", "themes", name+".yaml")
			if name != "default" {
				if _, err := os.Stat(themePath); err != nil {
					return fmt.Errorf("theme %q not found at %s", name, themePath)
				}
			}

			// Update config.
			configPath := filepath.Join(".computecommander", "config.yaml")
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			var m map[string]any
			if err := yaml.Unmarshal(data, &m); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}

			// Set theme in config.
			setConfigKey(m, "theme", name)

			out, err := yaml.Marshal(m)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			if err := os.WriteFile(configPath, out, 0o644); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			fmt.Printf("Theme set to %q\n", name)
			return nil
		},
	}
}

func themeEditCmd(app *App) *cobra.Command {
	_ = app // reserved: app preserved for symmetry with sibling theme handlers
	return &cobra.Command{
		Use:   "edit",
		Short: "Open theme file in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			themePath := filepath.Join(".computecommander", "themes", "default.yaml")
			return openEditor(editor, themePath)
		},
	}
}

// NotificationsCmd returns the "notifications" placeholder command.
func NotificationsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "notifications",
		Short:   "Notification settings",
		GroupID: "SETTINGS",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current notification settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Notifications: enabled (default)")
			fmt.Println("  Desktop notifications: disabled")
			fmt.Println("  Sound: disabled")
			fmt.Println("\nNote: Notification system is a future feature.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update notification setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Setting notifications.%s = %s (will apply when notification system is implemented)\n", args[0], args[1])
			return nil
		},
	})

	return cmd
}

// AnalyticsCmd returns the "analytics" command for usage analytics.
func AnalyticsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "analytics",
		Short:   "Usage analytics dashboard",
		GroupID: "SETTINGS",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			// Query local metrics.
			metrics, err := queryMetrics(cmd.Context(), app)
			if err != nil {
				return fmt.Errorf("query metrics: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":      true,
					"command":      "analytics",
					"totalSessions": len(metrics),
					"metrics":      metrics,
				})
			}

			if len(metrics) == 0 {
				fmt.Println("No analytics data available.")
				return nil
			}

			var totalInput, totalOutput int64
			var totalCost float64
			for _, m := range metrics {
				totalInput += m.InputTokens
				totalOutput += m.OutputTokens
				totalCost += m.EstimatedCost
			}

			fmt.Println("Analytics Summary:")
			fmt.Printf("  Total sessions: %d\n", len(metrics))
			fmt.Printf("  Total input tokens: %d\n", totalInput)
			fmt.Printf("  Total output tokens: %d\n", totalOutput)
			fmt.Printf("  Estimated total cost: $%.4f\n", totalCost)
			return nil
		},
	}
}

// IntegrationsCmd returns the "integrations" placeholder command.
func IntegrationsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "integrations",
		Short:   "Third-party service connections",
		GroupID: "SETTINGS",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured integrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("No integrations configured.")
			fmt.Println("\nNote: Integration connectors are a future feature.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add <service>",
		Short: "Add integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Integration %q: will be available when connectors are implemented.\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "remove <service>",
		Short: "Remove integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Integration %q: not currently configured.\n", args[0])
			return nil
		},
	})

	return cmd
}

// AutomationCmd returns the "automation" placeholder command.
func AutomationCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "automation",
		Short:   "Workflow automation builder",
		GroupID: "SETTINGS",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List automations",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("No automations configured.")
			fmt.Println("\nNote: Workflow automation is a future feature.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create new automation",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Automation builder: will be available in a future release.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "run <name>",
		Short: "Execute automation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("automation %q not found", args[0])
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <name>",
		Short: "Delete automation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("automation %q not found", args[0])
		},
	})

	return cmd
}

// setConfigKey sets a nested key in a map.
func setConfigKey(m map[string]any, key string, value string) {
	m[key] = value
}

// openEditor opens a file in the user's editor.
func openEditor(editor, path string) error {
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
