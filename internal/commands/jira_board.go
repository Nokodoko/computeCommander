package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/noko/computecommander/internal/jiraboard"
	"github.com/noko/computecommander/pkg/integrations/jira"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// JiraBoardCmd returns the "jira-board" command tree for Jira board generation.
func JiraBoardCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "jira-board",
		Short:   "Generate Jira boards from YAML meta-templates",
		Long:    "Template-driven Jira board generator for Datadog client onboarding. Produces agent-ready tickets with rich descriptions.",
		GroupID: "CORE",
	}

	cmd.AddCommand(jiraBoardGenerateCmd(app))
	cmd.AddCommand(jiraBoardValidateCmd(app))
	cmd.AddCommand(jiraBoardListCmd(app))
	cmd.AddCommand(jiraBoardPreviewCmd(app))
	cmd.AddCommand(jiraBoardDeleteCmd(app))

	return cmd
}

func jiraBoardGenerateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate board from template",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			projectType, _ := cmd.Flags().GetString("project-type")
			intakePath, _ := cmd.Flags().GetString("intake")
			projectKey, _ := cmd.Flags().GetString("project-key")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			outputPath, _ := cmd.Flags().GetString("output")
			instance, _ := cmd.Flags().GetString("instance")

			// Resolve intake file: flag > env var.
			if intakePath == "" {
				intakePath = os.Getenv("INTAKE_FILE")
			}

			if intakePath == "" {
				return fmt.Errorf("--intake flag or $INTAKE_FILE env var required")
			}

			// Output implies dry-run.
			if outputPath != "" {
				dryRun = true
			}

			templateDir := resolveTemplateDir(app)
			engine := jiraboard.NewEngine(templateDir)

			tmpl, err := engine.LoadTemplate(projectType)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			intake, err := jiraboard.LoadIntake(intakePath)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			// Validate intake against template.
			if warnings := jiraboard.ValidateIntakeAgainstTemplate(intake, tmpl); len(warnings) > 0 {
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
				}
			}

			// Override project key if specified.
			if projectKey != "" {
				tmpl.Meta.DefaultProjectKey = projectKey
			} else if intake.Intake.ProjectKey != "" {
				tmpl.Meta.DefaultProjectKey = intake.Intake.ProjectKey
			}

			// Expand template.
			expander := jiraboard.NewExpander(tmpl, intake)
			tickets, err := expander.Expand()
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			// Render descriptions.
			renderer, err := jiraboard.NewRenderer(templateDir)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}
			if err := renderer.RenderTickets(tickets, tmpl); err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			if dryRun {
				if outputPath != "" {
					// Write expanded tickets to file.
					data, err := yaml.Marshal(tickets)
					if err != nil {
						return fmt.Errorf("marshal tickets: %w", err)
					}
					if err := os.WriteFile(outputPath, data, 0o644); err != nil {
						return fmt.Errorf("write output: %w", err)
					}
					if jsonOut {
						return json.NewEncoder(os.Stdout).Encode(map[string]any{
							"success":       true,
							"command":        "jira-board generate",
							"dry_run":        true,
							"output_file":    outputPath,
							"total_tickets":  len(tickets),
						})
					}
					fmt.Printf("Dry run: wrote %d tickets to %s\n", len(tickets), outputPath)
					return nil
				}

				// Print tickets as YAML to stdout.
				data, err := yaml.Marshal(tickets)
				if err != nil {
					return fmt.Errorf("marshal tickets: %w", err)
				}
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success":       true,
						"command":        "jira-board generate",
						"dry_run":        true,
						"total_tickets":  len(tickets),
						"tickets":        tickets,
					})
				}
				fmt.Println(string(data))
				return nil
			}

			// Publish to Jira.
			client, err := resolveBoardJiraClient(app, instance)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			publisher := jiraboard.NewPublisher(client)
			result, err := publisher.Publish(cmd.Context(), tmpl.Meta.DefaultProjectKey, tickets)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board generate", err, intakePath)
			}

			result.BoardName = intake.Intake.ProjectName
			result.ProjectKey = tmpl.Meta.DefaultProjectKey

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":          true,
					"command":          "jira-board generate",
					"project_key":     result.ProjectKey,
					"board_name":      result.BoardName,
					"tickets_created": result.TicketsCreated,
					"tickets_updated": result.TicketsUpdated,
					"tickets_skipped": result.TicketsSkipped,
					"epics":           result.Epics,
					"stories":         result.Stories,
					"tasks":           result.Tasks,
					"tracks":          result.Tracks,
				})
			}

			fmt.Printf("Board generated: %s (%s)\n", result.BoardName, result.ProjectKey)
			fmt.Printf("  Created: %d  Updated: %d  Skipped: %d\n",
				result.TicketsCreated, result.TicketsUpdated, result.TicketsSkipped)
			fmt.Printf("  Epics: %d  Stories: %d  Tasks: %d\n",
				result.Epics, result.Stories, result.Tasks)
			return nil
		},
	}

	cmd.Flags().StringP("project-type", "p", "org-generator", "Template project type")
	cmd.Flags().StringP("intake", "i", "", "YAML intake file for dimension pruning")
	cmd.Flags().String("project-key", "", "Override Jira project key from template")
	cmd.Flags().Bool("dry-run", false, "Preview tickets without Jira API calls")
	cmd.Flags().StringP("output", "o", "", "Write expanded tickets to file (implies --dry-run)")
	cmd.Flags().String("instance", "", "Jira instance name from cmdr config")

	return cmd
}

func jiraBoardValidateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [template-file]",
		Short: "Validate a template file against schema",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			templateDir := filepath.Dir(args[0])
			projectType := filepath.Base(args[0])
			projectType = projectType[:len(projectType)-len(filepath.Ext(projectType))]

			engine := jiraboard.NewEngine(templateDir)
			tmpl, err := engine.LoadTemplate(projectType)
			if err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "jira-board validate",
						"error":   err.Error(),
					})
				}
				return fmt.Errorf("validation failed: %w", err)
			}

			errs := engine.Validate(tmpl)
			if len(errs) > 0 {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "jira-board validate",
						"errors":  errs,
					})
				}
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  %s: %s\n", e.Field, e.Message)
				}
				return fmt.Errorf("validation failed with %d errors", len(errs))
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":      true,
					"command":      "jira-board validate",
					"project_type": tmpl.Meta.ProjectType,
					"version":      tmpl.Meta.Version,
				})
			}

			fmt.Printf("Template valid: %s v%s\n", tmpl.Meta.ProjectType, tmpl.Meta.Version)
			return nil
		},
	}

	cmd.Flags().Bool("strict", false, "Treat warnings as errors")
	return cmd
}

func jiraBoardListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available project types",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			templateDir := resolveTemplateDir(app)
			engine := jiraboard.NewEngine(templateDir)

			templates, err := engine.ListTemplates()
			if err != nil {
				return fmt.Errorf("list templates: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":   true,
					"command":   "jira-board list",
					"templates": templates,
				})
			}

			if len(templates) == 0 {
				fmt.Println("No templates found.")
				return nil
			}

			fmt.Printf("%-20s %-10s %s\n", "PROJECT TYPE", "VERSION", "DESCRIPTION")
			for _, t := range templates {
				fmt.Printf("%-20s %-10s %s\n", t.ProjectType, t.Version, t.Description)
			}
			return nil
		},
	}
}

func jiraBoardPreviewCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview ticket count and structure",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			projectType, _ := cmd.Flags().GetString("project-type")
			intakePath, _ := cmd.Flags().GetString("intake")

			if intakePath == "" {
				intakePath = os.Getenv("INTAKE_FILE")
			}
			if intakePath == "" {
				return fmt.Errorf("--intake flag or $INTAKE_FILE env var required")
			}

			templateDir := resolveTemplateDir(app)
			engine := jiraboard.NewEngine(templateDir)

			tmpl, err := engine.LoadTemplate(projectType)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board preview", err, intakePath)
			}

			intake, err := jiraboard.LoadIntake(intakePath)
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board preview", err, intakePath)
			}

			expander := jiraboard.NewExpander(tmpl, intake)
			preview, err := expander.Preview()
			if err != nil {
				return emitJiraBoardError(jsonOut, "jira-board preview", err, intakePath)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":             true,
					"command":             "jira-board preview",
					"total_tickets":       preview.TotalTickets,
					"by_type":             preview.ByType,
					"by_phase":            preview.ByPhase,
					"dimensions_selected": preview.DimensionsSelected,
				})
			}

			fmt.Printf("Preview: %d total tickets\n", preview.TotalTickets)
			fmt.Printf("  By type:  ")
			for k, v := range preview.ByType {
				fmt.Printf("%s=%d  ", k, v)
			}
			fmt.Println()
			fmt.Printf("  By phase: ")
			for k, v := range preview.ByPhase {
				fmt.Printf("%s=%d  ", k, v)
			}
			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringP("project-type", "p", "org-generator", "Template project type")
	cmd.Flags().StringP("intake", "i", "", "YAML intake file for dimension pruning")

	return cmd
}

func jiraBoardDeleteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [project-key]",
		Short: "Delete all board tickets (destructive)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			confirm, _ := cmd.Flags().GetBool("confirm")
			if !confirm {
				if !confirmAction("Delete ALL cmdr-generated tickets in project " + args[0] + "?") {
					fmt.Println("Cancelled.")
					return nil
				}
			}
			fmt.Println("Delete not yet implemented — use Jira UI or API directly.")
			return nil
		},
	}

	cmd.Flags().Bool("confirm", false, "Skip confirmation prompt")
	return cmd
}

// resolveTemplateDir finds the templates/jira-board directory.
func resolveTemplateDir(app *App) string {
	// Check relative to CWD first.
	if _, err := os.Stat("templates/jira-board"); err == nil {
		return "templates/jira-board"
	}

	// Fallback to relative path.
	return "templates/jira-board"
}

// resolveBoardJiraClient creates a Jira client from config for jira-board commands.
// Reuses the existing resolveInstance helper from jira.go when possible,
// falls back to env vars.
func resolveBoardJiraClient(app *App, instance string) (*jira.Client, error) {
	if hasJiraConfig(app) {
		inst, err := resolveInstance(app.Config, instance)
		if err != nil {
			return nil, err
		}
		return newJiraClient(inst, app.Config), nil
	}

	// Fallback to environment variables.
	baseURL := os.Getenv("JIRA_BASE_URL")
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_API_TOKEN")

	if baseURL != "" && token != "" {
		return jira.NewClient(jira.ClientOpts{
			BaseURL:  baseURL,
			AuthType: "basic",
			Username: email,
			Password: token,
		}), nil
	}

	return nil, fmt.Errorf("no Jira configuration found; set jira config in config.yaml or use JIRA_BASE_URL env var")
}

// emitJiraBoardError outputs a JSON or plain error.
func emitJiraBoardError(jsonOut bool, command string, err error, intakePath string) error {
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success":     false,
			"command":     command,
			"error":       err.Error(),
			"intake_path": intakePath,
		})
		return nil
	}
	return err
}
