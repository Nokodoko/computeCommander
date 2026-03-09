package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// wrapperNames are the script names we hunt for stale processes.
var wrapperNames = []string{
	"fp-wrapper.sh",
	"lazygit-wrapper.sh",
	"cmdr-agent-wrapper.sh",
}

// staleProcess holds info about a detected stale wrapper process.
type staleProcess struct {
	pid     int
	cmdline string
	age     time.Duration
	tabHash string
	reason  string
}

// SweepCmd returns the "sweep" command for killing stale wrapper processes.
func SweepCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sweep",
		Short:   "Kill stale wrapper processes",
		Long:    "Find and kill stale fp-wrapper.sh, lazygit-wrapper.sh, and cmdr-agent-wrapper.sh processes left over from crashed dashboard sessions.",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			maxAge, _ := cmd.Flags().GetDuration("max-age")

			stale := findStaleWrappers(maxAge)

			if len(stale) == 0 {
				fmt.Println("No stale wrapper processes found.")
				return nil
			}

			for _, p := range stale {
				ageStr := p.age.Round(time.Second).String()
				if dryRun {
					fmt.Printf("  [dry-run] would kill pid %d (%s, age %s): %s\n", p.pid, p.reason, ageStr, p.cmdline)
				} else {
					if err := syscall.Kill(p.pid, syscall.SIGTERM); err != nil {
						fmt.Fprintf(os.Stderr, "  kill pid %d: %v\n", p.pid, err)
					} else {
						fmt.Printf("  killed pid %d (%s, age %s): %s\n", p.pid, p.reason, ageStr, p.cmdline)
					}
				}
			}

			if dryRun {
				fmt.Printf("\n%d stale process(es) found (dry-run; nothing killed).\n", len(stale))
			} else {
				fmt.Printf("\n%d stale process(es) killed.\n", len(stale))
			}
			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Report stale processes without killing them")
	cmd.Flags().Duration("max-age", 4*time.Hour, "Kill wrappers older than this duration")

	return cmd
}

// findStaleWrappers returns all wrapper processes that are considered stale.
// A process is stale if:
//  1. Its CWD file tab-hash doesn't exist in /tmp (the dashboard that spawned it is gone), OR
//  2. Its age exceeds maxAge.
func findStaleWrappers(maxAge time.Duration) []staleProcess {
	uid := os.Getuid()
	var stale []staleProcess

	// Build a single pgrep pattern that matches any wrapper name, scanning /proc once.
	pattern := strings.Join(wrapperNames, "|")
	out, err := exec.Command("pgrep", "-a", "-f", pattern).Output()
	if err != nil {
		// pgrep exits 1 when no matches — not an error worth reporting.
		return nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cmdline := strings.Join(fields[1:], " ")

		pidStat, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			continue
		}
		age := time.Since(pidStat.ModTime())
		tabHash := extractTabHash(cmdline)

		p := staleProcess{pid: pid, cmdline: cmdline, age: age, tabHash: tabHash}
		if reason, is := isStaleWrapper(p, maxAge, uid); is {
			p.reason = reason
			stale = append(stale, p)
		}
	}

	return stale
}

// extractTabHash pulls the tab hash argument from a wrapper cmdline.
// Wrappers are called as: bash .../fp-wrapper.sh <dir> <hash>
// The hash is passed as the second argument after the script path.
func extractTabHash(cmdline string) string {
	fields := strings.Fields(cmdline)
	for i, f := range fields {
		for _, name := range wrapperNames {
			if strings.HasSuffix(f, name) && i+2 < len(fields) {
				return fields[i+2]
			}
		}
	}
	return ""
}

// isStaleWrapper decides if a process is stale and returns the reason.
func isStaleWrapper(p staleProcess, maxAge time.Duration, uid int) (string, bool) {
	// Age-based staleness.
	if p.age > maxAge {
		return fmt.Sprintf("age>%s", maxAge), true
	}

	// Tab-hash staleness: if the CWD file is gone, the spawning dashboard has exited.
	// The CWD file is /tmp/cmdr-<uid>-<hash>-cwd, created by cmdr-agent-wrapper.
	if p.tabHash != "" {
		cwdFile := fmt.Sprintf("/tmp/cmdr-%d-%s-cwd", uid, p.tabHash)
		if _, err := os.Stat(cwdFile); os.IsNotExist(err) {
			return "tab-gone", true
		}
	}

	return "", false
}
