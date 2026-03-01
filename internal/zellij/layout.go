// Package zellij layout provides KDL layout file generation for the cmdr dashboard.
package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LayoutOpts configures the KDL layout generation.
type LayoutOpts struct {
	CmdrBinary    string // path to cmdr binary
	SessionPrefix string // zellij session name prefix
	ProjectDir    string // project root directory
	AgentCommand  string // command to run in the agent session pane
}

// GenerateLayout creates the KDL layout file for the cmdr dashboard.
//
// Each panel is a real zellij pane. The layout produces:
//
//	+------+------------------------------------------+--------+
//	|      |                                          |        |
//	|  fp  |         (borderless, no header)          | Agents |
//	| (10%)|           agent session (80%)            | (10%)  |
//	|      |              (focused)                   |        |
//	+------+------------------------------------------+--------+
//	| Event Log  |    Mail    |  Merge Queue  | Events        |
//	|   (25%)    |   (25%)   |    (25%)      |  (25%)        |
//	+-------------------------------------------------------------+
//
// Top row: 67% height — fp (10%) | agent (80%, borderless) | Agents (10%)
// Bottom row: 33% height — 4 columns spanning full width
//
// Zellij KDL split_direction semantics:
//   - "vertical"   = children arranged left-to-right (columns)
//   - "horizontal" = children arranged top-to-bottom (rows)
// buildAgentPane returns the KDL block for the center agent session pane.
// If agentCmd is empty, returns a plain shell pane with no command block.
func buildAgentPane(agentCmd string) string {
	if agentCmd == "" {
		return "                pane size=\"65%%\" focus=true borderless=true"
	}

	parts := strings.Fields(agentCmd)
	if len(parts) == 0 {
		return "                pane size=\"65%%\" focus=true borderless=true"
	}

	cmd := parts[0]
	if len(parts) == 1 {
		return fmt.Sprintf("                pane size=\"65%%%%\" focus=true borderless=true {\n                    command \"%s\"\n                }", cmd)
	}

	quotedArgs := make([]string, len(parts)-1)
	for i, arg := range parts[1:] {
		quotedArgs[i] = fmt.Sprintf("\"%s\"", arg)
	}
	argsStr := strings.Join(quotedArgs, " ")

	return fmt.Sprintf("                pane size=\"65%%%%\" focus=true borderless=true {\n                    command \"%s\"\n                    args %s\n                }", cmd, argsStr)
}

func GenerateLayout(opts LayoutOpts) string {
	cmdrBin := opts.CmdrBinary
	if cmdrBin == "" {
		cmdrBin = "cmdr"
	}

	projectDir := opts.ProjectDir
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	agentPane := buildAgentPane(opts.AgentCommand)

	return fmt.Sprintf(`layout {
    tab name="[CMDR] Dashboard" {
        pane split_direction="horizontal" {
            pane split_direction="vertical" size="67%%" {
                pane name="fp" size="10%%" {
                    command "fp"
                    args "%s"
                }
%s
                pane name="Agents" size="25%%" {
                    command "%s"
                    args "status" "--pane"
                }
            }
            pane split_direction="vertical" size="33%%" {
                pane name="Event Log" size="25%%" {
                    command "%s"
                    args "feed" "--pane"
                }
                pane name="Mail" size="25%%" {
                    command "%s"
                    args "mail" "list" "--pane"
                }
                pane name="Merge Queue" size="25%%" {
                    command "%s"
                    args "merge" "list" "--pane"
                }
                pane name="Events" size="25%%" {
                    command "%s"
                    args "feed" "--pane"
                }
            }
        }
    }
}
`, projectDir, agentPane, cmdrBin, cmdrBin, cmdrBin, cmdrBin, cmdrBin)
}

// WriteLayout generates and writes the KDL layout file to the given path.
func WriteLayout(path string, opts LayoutOpts) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}

	content := GenerateLayout(opts)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write layout file: %w", err)
	}

	return nil
}

// DefaultLayoutPath returns the default path for the dashboard layout file.
func DefaultLayoutPath() string {
	return filepath.Join(".computecommander", "layouts", "cmdr-dashboard.kdl")
}

// SessionOpts configures a zellij session launch.
type SessionOpts struct {
	SessionName string // zellij session name
	LayoutPath  string // path to the KDL layout file
	WorkDir     string // working directory
}

// LaunchSession opens the cmdr dashboard layout.
//
// When already inside zellij (the normal case — wezterm always runs zellij),
// it creates a new tab in the current session using the layout file.
// No new session or sub-session is spawned.
//
// When outside zellij (rare), it creates a new zellij session with the layout.
func LaunchSession(opts SessionOpts) error {
	zellijBin, err := exec.LookPath("zellij")
	if err != nil {
		return fmt.Errorf("zellij not found in PATH: %w", err)
	}

	if IsInsideZellij() {
		// Already inside zellij — open layout as a new tab in the current session.
		cmd := exec.Command(zellijBin, "action", "new-tab", "--layout", opts.LayoutPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Not inside zellij — create a new session with the layout.
	if opts.SessionName == "" {
		opts.SessionName = "cc-dashboard"
	}

	// Clean up any stale session with this name.
	env := filteredEnv()
	kill := exec.Command(zellijBin, "kill-session", opts.SessionName)
	kill.Env = env
	_ = kill.Run()
	del := exec.Command(zellijBin, "delete-session", opts.SessionName)
	del.Env = env
	_ = del.Run()

	cmd := exec.Command(zellijBin, "--session", opts.SessionName, "--layout", opts.LayoutPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}
	cmd.Env = env

	return cmd.Run()
}

// IsInsideZellij returns true if the process is running inside a zellij session.
func IsInsideZellij() bool {
	return os.Getenv("ZELLIJ") != "" || os.Getenv("ZELLIJ_SESSION_NAME") != ""
}

// ZellijAvailable returns true if the zellij binary is on PATH.
func ZellijAvailable() bool {
	_, err := exec.LookPath("zellij")
	return err == nil
}

// filteredEnv returns the current environment with ZELLIJ-related variables removed.
func filteredEnv() []string {
	skip := map[string]bool{
		"ZELLIJ":              true,
		"ZELLIJ_SESSION_NAME": true,
		"ZELLIJ_PANE_ID":      true,
	}
	var env []string
	for _, e := range os.Environ() {
		key := e
		if idx := len(e); idx > 0 {
			for i, c := range e {
				if c == '=' {
					key = e[:i]
					break
				}
			}
		}
		if !skip[key] {
			env = append(env, e)
		}
	}
	return env
}
