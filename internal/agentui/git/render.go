package git

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/agentui"
)

// RenderOpts is the contract surface of Render.
type RenderOpts struct {
	Lines   int
	Width   int
	NoColor bool
	// RepoPath is the absolute path to the git repo. When empty, Render
	// returns the degraded marker.
	RepoPath string
	// Now is the trailer timestamp. Tests inject a fixed time; production
	// uses time.Now().
	Now time.Time
}

// Render loads the snapshot at opts.RepoPath and emits exactly opts.Lines
// lines, each <= opts.Width visible cols. On any failure, returns the
// "git: not a repo" degraded marker padded to opts.Lines. NEVER returns
// an error.
//
// The caller is responsible for writing the result to stdout.
func Render(ctx context.Context, opts RenderOpts) []string {
	if opts.Lines <= 0 {
		return nil
	}
	if opts.Width <= 0 {
		return agentui.DegradedMarkerWithReason(agentui.LabelGit, agentui.ReasonNotARepo, opts.Lines)
	}
	if opts.RepoPath == "" {
		return agentui.DegradedMarkerWithReason(agentui.LabelGit, agentui.ReasonNotARepo, opts.Lines)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Commit budget = whatever's left after header(1) + 3 stat lines + sep(1) + trailer(1).
	// Header + trailer always; stat lines included if Lines >= 6.
	headerLines := 1
	statLines := 3
	sepLines := 1
	trailerLines := 1
	if opts.Lines < headerLines+trailerLines {
		statLines = 0
		sepLines = 0
	} else if opts.Lines < headerLines+statLines+sepLines+trailerLines {
		statLines = 0
		sepLines = 0
	}
	commitsBudget := max(opts.Lines-headerLines-statLines-sepLines-trailerLines, 0)

	snap, err := LoadSnapshot(ctx, opts.RepoPath, commitsBudget)
	if err != nil {
		return agentui.DegradedMarkerWithReason(agentui.LabelGit, agentui.ReasonNotARepo, opts.Lines)
	}

	pal := agentui.NewPalette(opts.NoColor)
	bs := agentui.NewBoxStyle(pal)

	out := make([]string, 0, opts.Lines)

	// Header: "Git · <branch> · clean|dirty [· ↑N ↓M]"
	branch := snap.Branch
	if branch == "" {
		branch = "(detached)"
	}
	cleanLabel := pal.Green + "clean" + pal.Reset
	if !snap.IsClean() {
		cleanLabel = pal.Yellow + "dirty" + pal.Reset
	}
	header := pal.Bold + "Git" + pal.Reset +
		pal.Dim + bs.Sep + pal.Reset + branch +
		pal.Dim + bs.Sep + pal.Reset + cleanLabel
	if snap.Ahead > 0 || snap.Behind > 0 {
		ah := pal.Dim + fmt.Sprintf(" up%d down%d", snap.Ahead, snap.Behind) + pal.Reset
		header += ah
	}
	out = append(out, agentui.Truncate(header, opts.Width))

	if statLines > 0 {
		// Three stat lines. Use bold for counts, dim for labels.
		out = append(out, agentui.Truncate("  "+fmt.Sprintf("staged:    %s", colorCount(snap.Staged, pal.Green, pal)), opts.Width))
		out = append(out, agentui.Truncate("  "+fmt.Sprintf("unstaged:  %s", colorCount(snap.Unstaged, pal.Yellow, pal)), opts.Width))
		out = append(out, agentui.Truncate("  "+fmt.Sprintf("untracked: %s", colorCount(snap.Untracked, pal.Red, pal)), opts.Width))
	}

	if sepLines > 0 {
		// Horizontal separator (Unicode in color mode, ASCII in NoColor).
		bar := strings.Repeat(bs.HBar, opts.Width/agentui.VisibleLen(bs.HBar))
		out = append(out, pal.Dim+agentui.Truncate(bar, opts.Width)+pal.Reset)
	}

	// Commit lines.
	commitsShown := 0
	for i := 0; i < commitsBudget && i < len(snap.Commits); i++ {
		c := snap.Commits[i]
		line := "  " + pal.Dim + c.ShortSha + pal.Reset + " " + c.Subject
		out = append(out, agentui.Truncate(line, opts.Width))
		commitsShown++
	}
	// Pad commits area if fewer than budget.
	for commitsShown < commitsBudget {
		out = append(out, "")
		commitsShown++
	}

	// Trailer.
	if trailerLines > 0 && len(out) < opts.Lines {
		trailer := pal.Dim + "updated " + now.Format("15:04:05") + pal.Reset
		out = append(out, agentui.Truncate(trailer, opts.Width))
	}

	return agentui.PadOrTruncate(out, opts.Lines)
}

// colorCount returns "<count> files" or "<count> file" with the count
// colored. count == 0 is dimmed.
func colorCount(n int, colorOnNonZero string, pal agentui.Palette) string {
	if n == 0 {
		return pal.Dim + "0 files" + pal.Reset
	}
	noun := "files"
	if n == 1 {
		noun = "file"
	}
	return colorOnNonZero + pal.Bold + fmt.Sprintf("%d %s", n, noun) + pal.Reset
}
