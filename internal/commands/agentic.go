// Package commands provides CLI command handler implementations.
// This file registers all agentic foundation subcommands.
package commands

import (
	"github.com/spf13/cobra"
)

// AgenticCmd returns the top-level registration point for agentic foundation
// CLI subcommands. These are registered as direct children of the root command
// (e.g., cmdr block, cmdr blueprint) rather than under a cmdr agentic prefix.
//
// Note: Trace subcommands (list, show, export, prune) are merged into the
// existing TraceCmd via agenticTraceSubcommands() -- not listed here.
func AgenticCmd(app *App) []*cobra.Command {
	return []*cobra.Command{
		BlockCmd(app),
		BlueprintCmd(app),
		GateCmd(app),
		HoldoutCmd(app),
		IsolationCmd(app),
	}
}
