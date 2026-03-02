package tui

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitStatusPane shows branch, staged/unstaged files, and worktree list.
type GitStatusPane struct {
	branch    string
	staged    []string
	unstaged  []string
	untracked []string
	worktrees []string
	theme     *Theme
	width     int
	height    int
}

// NewGitStatusPane constructs a GitStatusPane.
func NewGitStatusPane(theme *Theme) *GitStatusPane {
	return &GitStatusPane{
		theme: theme,
	}
}

// Refresh runs git commands to populate the pane data.
func (g *GitStatusPane) Refresh() error {
	g.branch = gitCmd("branch", "--show-current")
	g.staged = gitLines("diff", "--cached", "--name-only")
	g.unstaged = gitLines("diff", "--name-only")
	g.untracked = gitLines("ls-files", "--others", "--exclude-standard")
	g.worktrees = gitWorktrees()
	return nil
}

// SetSize updates display dimensions.
func (g *GitStatusPane) SetSize(w, h int) {
	g.width = w
	g.height = h
}

// View renders the git status pane.
func (g *GitStatusPane) View() string {
	var lines []string

	// Branch.
	branchLine := "Branch: " + g.theme.GitBranch.Render(g.branch)
	lines = append(lines, branchLine)
	lines = append(lines, "")

	// Staged files.
	if len(g.staged) > 0 {
		lines = append(lines, g.theme.GitStaged.Render(fmt.Sprintf("Staged (%d):", len(g.staged))))
		for _, f := range g.staged {
			lines = append(lines, "  "+g.theme.GitStaged.Render("M "+f))
		}
		lines = append(lines, "")
	}

	// Unstaged files.
	if len(g.unstaged) > 0 {
		lines = append(lines, g.theme.GitUnstaged.Render(fmt.Sprintf("Unstaged (%d):", len(g.unstaged))))
		for _, f := range g.unstaged {
			lines = append(lines, "  "+g.theme.GitUnstaged.Render("M "+f))
		}
		lines = append(lines, "")
	}

	// Untracked files.
	if len(g.untracked) > 0 {
		lines = append(lines, g.theme.GitUntracked.Render(fmt.Sprintf("Untracked (%d):", len(g.untracked))))
		for _, f := range g.untracked {
			lines = append(lines, "  "+g.theme.GitUntracked.Render("? "+f))
		}
		lines = append(lines, "")
	}

	// Worktrees.
	if len(g.worktrees) > 0 {
		lines = append(lines, fmt.Sprintf("Worktrees (%d):", len(g.worktrees)))
		for _, wt := range g.worktrees {
			lines = append(lines, "  "+wt)
		}
	}

	if len(g.staged) == 0 && len(g.unstaged) == 0 && len(g.untracked) == 0 {
		lines = append(lines, g.theme.Subtitle.Render("  Clean working tree"))
	}

	// Truncate to fit height.
	h := g.height
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}

	return strings.Join(lines, "\n")
}

// gitCmd runs a git command and returns stdout trimmed.
func gitCmd(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitLines runs a git command and returns non-empty lines.
func gitLines(args ...string) []string {
	raw := gitCmd(args...)
	if raw == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// gitWorktrees parses git worktree list output.
func gitWorktrees() []string {
	raw := gitCmd("worktree", "list")
	if raw == "" {
		return nil
	}
	var results []string
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			results = append(results, line)
		}
	}
	return results
}
