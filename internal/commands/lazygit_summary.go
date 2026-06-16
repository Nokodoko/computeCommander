package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/agentui/git"
)

// LazygitSummaryCmd returns the "lazygit-summary" subcommand: a
// single-shot text snapshot of the current git working tree, sized to
// embed in a 5-8 line ASCII frame. Does NOT launch interactive lazygit.
//
// Honours the sessionbanner consumer contract (SPEC phase3.md):
//   - exit code 0 on all failure paths
//   - exactly --lines lines
//   - <= --width visible cols
//   - --no-color emits clean ASCII
//   - "git: not a repo" degraded marker on any failure
func LazygitSummaryCmd(app *App) *cobra.Command {
	_ = app // unused — lazygit-summary needs no project state, just a repo path
	var (
		lines   int
		width   int
		noColor bool
		repo    string
	)
	cmd := &cobra.Command{
		Use:     "lazygit-summary",
		Short:   "Emit a fixed-shape, embeddable git status snapshot",
		GroupID: "OBSERVABILITY",
		Long: `Single-shot git working-tree snapshot sized to embed in a ~5-8 line
ASCII frame. Does NOT launch interactive lazygit. Shells out to
'git -C <repo> status --porcelain=v2 -b' and 'git log --oneline'.
Honours NO_COLOR per https://no-color.org. Exits 0 on every failure
path with a single-line degraded marker ("git: not a repo") padded
to --lines so the embedding frame size does not shift between renders.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLazygitSummary(cmd, lines, width, noColor, repo)
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 8, "total output lines including header and trailer")
	cmd.Flags().IntVar(&width, "width", 60, "inner width hint, used for column truncation")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "suppress all ANSI colour codes (also honours $NO_COLOR)")
	cmd.Flags().StringVar(&repo, "repo", "", "explicit path to the git repo (default: $PWD walked up to .git)")
	return cmd
}

func runLazygitSummary(cmd *cobra.Command, lines, width int, noColor bool, repo string) error {
	if !noColor {
		if v := os.Getenv("NO_COLOR"); v != "" {
			noColor = true
		}
	}

	if repo == "" {
		repo = resolveRepoPath()
	}

	out := git.Render(cmd.Context(), git.RenderOpts{
		Lines:    lines,
		Width:    width,
		NoColor:  noColor,
		RepoPath: repo,
		Now:      time.Now(),
	})
	for _, ln := range out {
		fmt.Fprintln(os.Stdout, ln)
	}
	return nil
}

// resolveRepoPath walks up from $PWD looking for a .git directory or
// file (worktree marker). Returns the empty string if no .git is found
// within 32 parent steps, which the renderer translates to the
// "git: not a repo" degraded marker.
func resolveRepoPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for range 32 {
		dotGit := filepath.Join(wd, ".git")
		if _, err := os.Stat(dotGit); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return ""
}
