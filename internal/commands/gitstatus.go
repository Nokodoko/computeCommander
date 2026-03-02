package commands

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// GitStatusCmd returns the "git-status" command for git repository overview.
func GitStatusCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "git-status",
		Aliases: []string{"gs"},
		Short:   "Git repository status",
		Long:    "Display current branch, staged/unstaged changes, and last commit info.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			if paneMode {
				return runGitStatusPane(cmd)
			}
			printGitStatus()
			return nil
		},
	}

	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runGitStatusPane runs git-status in long-lived pane mode, refreshing every 3 seconds.
func runGitStatusPane(cmd *cobra.Command) error {
	ctx := cmd.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	render := func() {
		clearScreen()
		printGitStatus()
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			render()
		}
	}
}

// printGitStatus prints a compact git status view with ANSI colors.
func printGitStatus() {
	branch := gitBranch()
	staged, modified, untracked := gitStatusFiles()
	lastCommit := gitLastCommit()

	// Branch line with change indicators.
	indicator := ""
	if len(staged) > 0 || len(modified) > 0 || len(untracked) > 0 {
		parts := []string{}
		if len(staged) > 0 {
			parts = append(parts, fmt.Sprintf("\033[32m%d staged\033[0m", len(staged)))
		}
		if len(modified) > 0 {
			parts = append(parts, fmt.Sprintf("\033[31m%d modified\033[0m", len(modified)))
		}
		if len(untracked) > 0 {
			parts = append(parts, fmt.Sprintf("\033[33m%d untracked\033[0m", len(untracked)))
		}
		indicator = " \033[1m●\033[0m " + strings.Join(parts, ", ")
	} else {
		indicator = " \033[32m✓ clean\033[0m"
	}

	fmt.Printf("\033[1m %s\033[0m%s\n", branch, indicator)

	if len(staged) > 0 {
		fmt.Println("\n\033[32mStaged:\033[0m")
		for _, f := range staged {
			fmt.Printf("  \033[32m%s\033[0m\n", f)
		}
	}

	if len(modified) > 0 {
		fmt.Println("\n\033[31mModified:\033[0m")
		for _, f := range modified {
			fmt.Printf("  \033[31m%s\033[0m\n", f)
		}
	}

	if len(untracked) > 0 {
		fmt.Println("\n\033[33mUntracked:\033[0m")
		for _, f := range untracked {
			fmt.Printf("  \033[33m%s\033[0m\n", f)
		}
	}

	if lastCommit != "" {
		fmt.Printf("\n\033[2mLast: %s\033[0m\n", lastCommit)
	}
}

// gitBranch returns the current branch name (or HEAD sha if detached).
func gitBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// gitStatusFiles parses `git status --porcelain` and returns staged, modified, and untracked file lists.
func gitStatusFiles() (staged, modified, untracked []string) {
	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		path := strings.TrimSpace(line[3:])

		x := rune(xy[0]) // index 0 = staged status
		y := rune(xy[1]) // index 1 = worktree status

		// Staged changes: first column is not ' ' or '?'.
		if x != ' ' && x != '?' {
			staged = append(staged, xy[:1]+" "+path)
		}
		// Worktree modifications: second column is 'M', 'D', etc.
		if y == 'M' || y == 'D' {
			modified = append(modified, xy[1:2]+" "+path)
		}
		// Untracked: both columns are '?'.
		if xy == "??" {
			untracked = append(untracked, "?? "+path)
		}
	}
	return
}

// gitLastCommit returns a short summary of the last commit.
func gitLastCommit() string {
	out, err := exec.Command("git", "log", "-1", "--pretty=format:%h %s", "--date=relative").Output()
	if err != nil {
		return ""
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return ""
	}

	// Append relative time.
	timeOut, err := exec.Command("git", "log", "-1", "--pretty=format:%ar").Output()
	if err == nil {
		rel := strings.TrimSpace(string(timeOut))
		if rel != "" {
			msg = fmt.Sprintf("%s (%s)", msg, rel)
		}
	}
	return truncate(msg, 72)
}
