package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// PromptLineCmd returns the "prompt" command for the dashboard prompt-line pane.
func PromptLineCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "prompt",
		Short:   "Session info bar for dashboard pane",
		Long:    "Display current session info. In --pane mode, live-updates a single-line prompt bar.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, _ := cmd.Flags().GetBool("pane")
			if pane {
				return runPromptLinePane(cmd.Context(), app)
			}
			return printPromptLineOnce(cmd.Context(), app)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (live-updating single line)")

	return cmd
}

// printPromptLineOnce prints a one-shot prompt line summary.
func printPromptLineOnce(ctx context.Context, app *App) error {
	info := gatherPromptInfo(ctx, app)
	fmt.Println(formatPromptLine(info))
	return nil
}

// promptInfo holds the data displayed in the prompt line.
type promptInfo struct {
	ProjectName  string
	ProjectDir   string
	ActiveAgents int
	SessionName  string
}

// gatherPromptInfo collects info for the prompt line.
func gatherPromptInfo(ctx context.Context, app *App) promptInfo {
	info := promptInfo{}

	// Project name from config.
	if app.Config != nil {
		info.ProjectName = app.Config.Project.Name
	}

	// Project directory.
	info.ProjectDir, _ = os.Getwd()
	if info.ProjectName == "" {
		info.ProjectName = filepath.Base(info.ProjectDir)
	}

	// Active agent count from DB.
	if app.DB != nil {
		row := app.DB.QueryRow(ctx,
			"SELECT COUNT(*) FROM sessions WHERE state NOT IN ('completed', 'zombie')")
		_ = row.Scan(&info.ActiveAgents)
	}

	// Read session from CWD file if available.
	tabHash := os.Getenv("CMDR_TAB_HASH")
	if tabHash != "" {
		cwdFile := fmt.Sprintf("/tmp/cmdr-%d-%s-cwd", os.Getuid(), tabHash)
		if data, err := os.ReadFile(cwdFile); err == nil {
			cwdDir := strings.TrimSpace(string(data))
			if cwdDir != "" {
				info.ProjectDir = cwdDir
				info.ProjectName = filepath.Base(cwdDir)
			}
		}
	}

	return info
}

// formatPromptLine renders the prompt info as a styled single line.
func formatPromptLine(info promptInfo) string {
	var b strings.Builder

	// Project name.
	b.WriteString("\033[1;36m")
	b.WriteString(info.ProjectName)
	b.WriteString("\033[0m")

	// Agent count.
	if info.ActiveAgents > 0 {
		b.WriteString(fmt.Sprintf(" \033[32m%d agents\033[0m", info.ActiveAgents))
	} else {
		b.WriteString(" \033[2mno agents\033[0m")
	}

	// Key hints.
	b.WriteString("  \033[2m[Ctrl+Space S] Sessions  [Ctrl+Space ?] Help\033[0m")

	return b.String()
}

// runPromptLinePane runs the prompt-line pane in a ticker loop.
func runPromptLinePane(ctx context.Context, app *App) error {
	const refreshInterval = 3 * time.Second

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	// Immediate first render.
	renderPromptLine(ctx, app)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renderPromptLine(ctx, app)
		}
	}
}

// renderPromptLine renders one frame of the prompt line (clears and reprints).
func renderPromptLine(ctx context.Context, app *App) {
	info := gatherPromptInfo(ctx, app)
	// Move cursor to beginning, clear line, print.
	fmt.Print("\r\033[K")
	fmt.Print(formatPromptLine(info))
}
