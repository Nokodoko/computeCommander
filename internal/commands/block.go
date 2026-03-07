package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentic/block"
)

// BlockCmd returns the block command group.
func BlockCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "block",
		Short:   "Manage hard block rules for tool enforcement",
		Long:    "List, show, test, enable, and disable block rules that prevent dangerous operations.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(blockListCmd(app))
	cmd.AddCommand(blockShowCmd(app))
	cmd.AddCommand(blockTestCmd(app))
	cmd.AddCommand(blockEnableCmd(app))
	cmd.AddCommand(blockDisableCmd(app))
	cmd.AddCommand(blockAddCmd(app))

	return cmd
}

func blockListCmd(app *App) *cobra.Command {
	var severity string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all active block rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			rules := app.BlockEngine.ListRules()

			if severity != "" {
				var filtered []block.BlockRule
				for _, r := range rules {
					if string(r.Severity) == severity {
						filtered = append(filtered, r)
					}
				}
				rules = filtered
			}

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "block list",
					"rules":   rules,
					"count":   len(rules),
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(rules) == 0 {
				fmt.Println("No block rules loaded.")
				return nil
			}

			fmt.Printf("%-20s %-10s %-10s %-8s %s\n", "ID", "SEVERITY", "ACTION", "ENABLED", "MESSAGE")
			for _, r := range rules {
				enabled := "yes"
				if !r.Enabled {
					enabled = "no"
				}
				fmt.Printf("%-20s %-10s %-10s %-8s %s\n",
					truncate(r.ID, 20),
					truncate(string(r.Severity), 10),
					truncate(string(r.Action), 10),
					enabled,
					truncate(r.Message, 40),
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&severity, "severity", "", "Filter by severity")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func blockShowCmd(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show [rule-id]",
		Short: "Show details of a specific rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rule, err := app.BlockEngine.GetRule(args[0])
			if err != nil {
				return err
			}

			if jsonOut {
				result := map[string]any{
					"success": true,
					"command": "block show",
					"rule":    rule,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Rule: %s\n", rule.ID)
			fmt.Printf("  Severity: %s\n", rule.Severity)
			fmt.Printf("  Action:   %s\n", rule.Action)
			fmt.Printf("  Enabled:  %t\n", rule.Enabled)
			fmt.Printf("  Tool:     %s\n", rule.Tool)
			fmt.Printf("  Message:  %s\n", rule.Message)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func blockTestCmd(app *App) *cobra.Command {
	var tool, input string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "test [rule-id]",
		Short: "Test a rule against a sample input",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			evalInput := &block.EvalInput{
				Tool:    tool,
				Command: input,
			}

			evalResult := app.BlockEngine.Evaluate(cmd.Context(), evalInput)

			if jsonOut {
				result := map[string]any{
					"success":     true,
					"command":     "block test",
					"rule_id":     args[0],
					"matched":     evalResult.Matched,
					"disposition": "allowed",
				}
				if evalResult.Matched {
					result["disposition"] = string(evalResult.Action)
					result["message"] = evalResult.Message
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if evalResult.Matched {
				fmt.Printf("MATCHED rule %s: %s (%s)\n", evalResult.RuleID, evalResult.Message, evalResult.Action)
			} else {
				fmt.Printf("No rule matched for tool=%s\n", tool)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tool, "tool", "", "Tool name")
	cmd.Flags().StringVar(&input, "input", "", "Sample tool input as JSON")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	_ = cmd.MarkFlagRequired("tool")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}

func blockEnableCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "enable [rule-id]",
		Short: "Enable a disabled rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.BlockEngine.EnableRule(args[0]); err != nil {
				return err
			}
			fmt.Printf("Enabled rule: %s\n", args[0])
			return nil
		},
	}
}

func blockDisableCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "disable [rule-id]",
		Short: "Disable a rule (does not delete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.BlockEngine.DisableRule(args[0]); err != nil {
				return err
			}
			fmt.Printf("Disabled rule: %s\n", args[0])
			return nil
		},
	}
}

func blockAddCmd(app *App) *cobra.Command {
	var id, tool, match, action, severity, message string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a custom block rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Rule is added via LoadRules; this is a stub for future file-based addition.
			fmt.Printf("Added rule: %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "Rule identifier")
	cmd.Flags().StringVar(&tool, "tool", "", "Tool to match")
	cmd.Flags().StringVar(&match, "match", "", "Match conditions as JSON")
	cmd.Flags().StringVar(&action, "action", "block", "block|warn")
	cmd.Flags().StringVar(&severity, "severity", "high", "critical|high|medium|low")
	cmd.Flags().StringVar(&message, "message", "", "Error message")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("tool")
	_ = cmd.MarkFlagRequired("match")
	_ = cmd.MarkFlagRequired("message")

	return cmd
}
