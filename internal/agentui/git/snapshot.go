// Package git provides the read-side helpers used by the
// `cmdr lazygit-summary` subcommand and (later) any consumer that needs an
// embeddable text snapshot of a git working tree.
//
// All commands shell out via `git -C <repoPath> …`. There is intentionally
// no libgit2 dependency: the binary's behavior matches the user's shell,
// the surface stays tiny, and platform / version drift lives entirely in
// the git binary which is already required.
package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Commit is the projection of a git log row used by the renderer.
type Commit struct {
	ShortSha string // first 7 chars by convention
	Subject  string
}

// Snapshot is the renderable shape of a git working tree at one instant.
type Snapshot struct {
	Branch    string
	Staged    int
	Unstaged  int
	Untracked int
	Ahead     int
	Behind    int
	Commits   []Commit
}

// IsClean returns true when no staged, unstaged, or untracked files are
// present. Used by the renderer to color the header green vs yellow.
func (s Snapshot) IsClean() bool {
	return s.Staged == 0 && s.Unstaged == 0 && s.Untracked == 0
}

// LoadSnapshot reads the current git state for repoPath. Returns an error
// when repoPath is not inside a git working tree. The caller (the
// renderer) translates errors into the "git: not a repo" degraded marker;
// no error propagates up to the Cobra command's exit code.
//
// commitsBudget controls how many `git log` rows to fetch. 0 disables the
// log entirely (snapshot-only mode).
func LoadSnapshot(ctx context.Context, repoPath string, commitsBudget int) (Snapshot, error) {
	if repoPath == "" {
		return Snapshot{}, errors.New("repo path empty")
	}

	// `git status --porcelain=v2 -b` gives branch + ahead/behind + counts.
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statusCmd := exec.CommandContext(statusCtx, "git", "-C", repoPath, "status", "--porcelain=v2", "-b", "--untracked-files=normal")
	statusOut, err := statusCmd.Output()
	if err != nil {
		return Snapshot{}, fmt.Errorf("git status: %w", err)
	}

	snap, err := parsePorcelainV2(string(statusOut))
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse porcelain: %w", err)
	}

	if commitsBudget > 0 {
		logCtx, cancelLog := context.WithTimeout(ctx, 2*time.Second)
		defer cancelLog()
		logCmd := exec.CommandContext(logCtx, "git", "-C", repoPath, "log", "--oneline", "-n", strconv.Itoa(commitsBudget))
		logOut, err := logCmd.Output()
		if err == nil {
			snap.Commits = parseOneline(string(logOut))
		}
	}

	return snap, nil
}

// parsePorcelainV2 turns `git status --porcelain=v2 -b` output into a
// Snapshot. The v2 format is line-oriented:
//
//	# branch.head <name-or-detached>
//	# branch.upstream <upstream>
//	# branch.ab +<ahead> -<behind>
//	1 <XY> ...        ← changed tracked file (staged + unstaged in XY)
//	2 <XY> ...        ← renamed tracked file
//	u <XY> ...        ← unmerged
//	? <path>          ← untracked
//	! <path>          ← ignored (we don't request these)
//
// We count staged when X != '.' && X != '?', unstaged when Y != '.' &&
// Y != '?', untracked when leading char is '?'. Unmerged contribute to
// both staged and unstaged for the purposes of the snapshot header
// (matches how lazygit displays them).
func parsePorcelainV2(s string) (Snapshot, error) {
	snap := Snapshot{}
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			snap.Branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
		case strings.HasPrefix(line, "# branch.ab "):
			// "# branch.ab +N -M"
			rest := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(rest)
			for _, p := range parts {
				if strings.HasPrefix(p, "+") {
					n, _ := strconv.Atoi(p[1:])
					snap.Ahead = n
				} else if strings.HasPrefix(p, "-") {
					n, _ := strconv.Atoi(p[1:])
					snap.Behind = n
				}
			}
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			// "1 XY sub <mH> <mI> <mW> <hH> <hI> <path>"
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			xy := fields[1]
			if len(xy) < 2 {
				continue
			}
			x := xy[0]
			y := xy[1]
			if x != '.' && x != '?' {
				snap.Staged++
			}
			if y != '.' && y != '?' {
				snap.Unstaged++
			}
		case strings.HasPrefix(line, "u "):
			// Unmerged — count as both staged and unstaged.
			snap.Staged++
			snap.Unstaged++
		case strings.HasPrefix(line, "? "):
			snap.Untracked++
		}
	}
	return snap, sc.Err()
}

// parseOneline turns `git log --oneline` output into []Commit. Each line
// is "<sha> <subject>"; the first whitespace-delimited token is the sha.
func parseOneline(s string) []Commit {
	var commits []Commit
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp <= 0 {
			commits = append(commits, Commit{ShortSha: line})
			continue
		}
		commits = append(commits, Commit{
			ShortSha: line[:sp],
			Subject:  line[sp+1:],
		})
	}
	return commits
}
