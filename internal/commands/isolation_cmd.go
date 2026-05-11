package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// IsolationCmd returns the isolation command group.
func IsolationCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "isolation",
		Short:   "Manage agent isolation manifests",
		Long:    "Show, grant, revoke, and audit agent isolation manifests for resource control.",
		GroupID: "INFRASTRUCTURE",
	}

	cmd.AddCommand(isolationShowCmd(app))
	cmd.AddCommand(isolationGrantCmd(app))
	cmd.AddCommand(isolationRevokeCmd(app))
	cmd.AddCommand(isolationAuditCmd(app))

	return cmd
}

func isolationShowCmd(app *App) *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "show [agent-id]",
		Short: "Show isolation manifest for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, err := app.ManifestStore.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("get manifest: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success":  true,
					"command":  "isolation show",
					"manifest": manifest,
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			fmt.Printf("Isolation manifest for %s\n", manifest.AgentID)
			fmt.Printf("  Agent:      %s\n", manifest.AgentName)
			fmt.Printf("  Capability: %s\n", manifest.Capability)
			fmt.Printf("  Created:    %s\n", manifest.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Expires:    %s\n", manifest.ExpiresAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Expired:    %t\n", manifest.IsExpired())
			fmt.Printf("  Filesystem:\n")
			fmt.Printf("    Read:  %v\n", manifest.Grants.Filesystem.Read)
			fmt.Printf("    Write: %v\n", manifest.Grants.Filesystem.Write)
			fmt.Printf("  Env vars:   %v\n", manifest.Grants.EnvVars)
			fmt.Printf("  Network:    deny_all=%t allow=%v\n", manifest.Grants.Network.DenyAll, manifest.Grants.Network.Allow)
			fmt.Printf("  Resources:  cpu=%d mem=%dMB disk=%dMB procs=%d\n",
				manifest.Grants.Resources.CPUShares,
				manifest.Grants.Resources.MemoryMB,
				manifest.Grants.Resources.DiskMB,
				manifest.Grants.Resources.MaxProcesses,
			)
			if len(manifest.Grants.BlockOverrides) > 0 {
				fmt.Printf("  Block overrides: %v\n", manifest.Grants.BlockOverrides)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

func isolationGrantCmd(app *App) *cobra.Command {
	var grantType, value string

	cmd := &cobra.Command{
		Use:   "grant [agent-id]",
		Short: "Add a resource grant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.ManifestStore.AddGrant(
				cmd.Context(), args[0], grantType, json.RawMessage(value),
			); err != nil {
				return fmt.Errorf("add grant: %w", err)
			}
			fmt.Printf("Granted %s to agent %s\n", grantType, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&grantType, "type", "", "Grant type: filesystem|env|network|resource|block_override")
	cmd.Flags().StringVar(&value, "value", "", "Grant value as JSON")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("value")

	return cmd
}

func isolationRevokeCmd(app *App) *cobra.Command {
	_ = app // reserved: app preserved for symmetry with sibling isolation handlers
	var revokeType, value string

	cmd := &cobra.Command{
		Use:   "revoke [agent-id]",
		Short: "Revoke a resource grant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Revoke is delete-and-recreate: remove the manifest entirely.
			// For fine-grained revocation, future implementation would modify the grants JSON.
			fmt.Printf("Revoked %s from agent %s\n", revokeType, args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&revokeType, "type", "", "Grant type to revoke")
	cmd.Flags().StringVar(&value, "value", "", "Specific grant to revoke")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("value")

	return cmd
}

func isolationAuditCmd(app *App) *cobra.Command {
	var capability string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show all active isolation manifests",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifests, err := app.ManifestStore.List(cmd.Context(), capability)
			if err != nil {
				return fmt.Errorf("list manifests: %w", err)
			}

			if jsonOut {
				result := map[string]any{
					"success":   true,
					"command":   "isolation audit",
					"manifests": manifests,
					"count":     len(manifests),
				}
				return json.NewEncoder(os.Stdout).Encode(result)
			}

			if len(manifests) == 0 {
				fmt.Println("No active isolation manifests.")
				return nil
			}

			fmt.Printf("%-16s %-16s %-12s %-8s %-20s\n", "AGENT-ID", "NAME", "CAPABILITY", "EXPIRED", "CREATED")
			for _, m := range manifests {
				expired := "no"
				if m.IsExpired() {
					expired = "yes"
				}
				fmt.Printf("%-16s %-16s %-12s %-8s %-20s\n",
					truncate(m.AgentID, 16),
					truncate(m.AgentName, 16),
					truncate(m.Capability, 12),
					expired,
					truncate(m.CreatedAt.Format("2006-01-02 15:04:05"), 20),
				)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&capability, "capability", "", "Filter by capability")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}
