package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// DoctorCmd returns the "doctor" command for health checks.
func DoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   "Health checks",
		Long:    "Run diagnostic checks on the ComputeCommander installation and its dependencies.",
		GroupID: "INFRASTRUCTURE",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			checks := runDoctorChecks(app)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(checks)
			}

			allOK := true
			for _, c := range checks {
				status := "OK"
				if !c.OK {
					status = "FAIL"
					allOK = false
				}
				fmt.Printf("  [%4s] %s: %s\n", status, c.Name, c.Detail)
			}

			if allOK {
				fmt.Println("\nAll checks passed.")
			} else {
				fmt.Println("\nSome checks failed. Please review the output above.")
			}
			return nil
		},
	}
}

type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func runDoctorChecks(app *App) []doctorCheck {
	var checks []doctorCheck

	// Config check.
	if app.Config != nil {
		if err := app.Config.Validate(); err != nil {
			checks = append(checks, doctorCheck{"config", false, err.Error()})
		} else {
			checks = append(checks, doctorCheck{"config", true, "valid"})
		}
	} else {
		checks = append(checks, doctorCheck{"config", false, "not loaded"})
	}

	// Database check.
	if app.DB != nil {
		checks = append(checks, doctorCheck{"database", true, fmt.Sprintf("driver=%s", app.DB.Driver())})
	} else {
		checks = append(checks, doctorCheck{"database", false, "not connected"})
	}

	// Git check.
	if _, err := exec.LookPath("git"); err != nil {
		checks = append(checks, doctorCheck{"git", false, "git not found in PATH"})
	} else {
		checks = append(checks, doctorCheck{"git", true, "available"})
	}

	// Zellij check.
	if _, err := exec.LookPath("zellij"); err != nil {
		checks = append(checks, doctorCheck{"zellij", false, "zellij not found in PATH"})
	} else {
		checks = append(checks, doctorCheck{"zellij", true, "available"})
	}

	// fp (file picker) check.
	if _, err := exec.LookPath("fp"); err != nil {
		checks = append(checks, doctorCheck{"fp", false, "fp not found in PATH (required for dashboard file-picker pane; install via 'cargo install fp')"})
	} else {
		checks = append(checks, doctorCheck{"fp", true, "available"})
	}

	// focus-watcher check.
	if _, err := exec.LookPath("focus-watcher"); err != nil {
		checks = append(checks, doctorCheck{"focus-watcher", false, "focus-watcher not found in PATH (build from plugins/focus-watcher/ with 'cargo build --release')"})
	} else {
		checks = append(checks, doctorCheck{"focus-watcher", true, "available"})
	}

	// Project directory check.
	if _, err := os.Stat(".computecommander"); err != nil {
		checks = append(checks, doctorCheck{"project", false, ".computecommander/ not found"})
	} else {
		checks = append(checks, doctorCheck{"project", true, ".computecommander/ exists"})
	}

	// Database health checks (deep).
	if app.DB != nil {
		checks = append(checks, runDBHealthChecks(app)...)
	}

	// Zombie pane process check.
	checks = append(checks, checkZombiePanes()...)

	return checks
}

// runDBHealthChecks performs deep database health checks including
// integrity, WAL size, session counts, and stale data detection.
func runDBHealthChecks(app *App) []doctorCheck {
	var checks []doctorCheck
	ctx := context.Background()

	// SQLite integrity check.
	var integrityResult string
	row := app.DB.QueryRow(ctx, "PRAGMA integrity_check")
	if err := row.Scan(&integrityResult); err != nil {
		checks = append(checks, doctorCheck{"db.integrity", false, fmt.Sprintf("integrity check failed: %v", err)})
	} else if integrityResult != "ok" {
		checks = append(checks, doctorCheck{"db.integrity", false, fmt.Sprintf("integrity issues: %s", integrityResult)})
	} else {
		checks = append(checks, doctorCheck{"db.integrity", true, "ok"})
	}

	// WAL file size check.
	dbPath := ""
	if app.Config != nil {
		dbPath = app.Config.Database.SQLite.Path
	}
	if dbPath != "" {
		walPath := dbPath + "-wal"
		if info, err := os.Stat(walPath); err == nil {
			walSizeMB := float64(info.Size()) / (1024 * 1024)
			if walSizeMB > 1.0 {
				checks = append(checks, doctorCheck{"db.wal_size", false,
					fmt.Sprintf("WAL file is %.1fMB (should be <1MB); run PRAGMA wal_checkpoint(TRUNCATE)", walSizeMB)})
			} else {
				checks = append(checks, doctorCheck{"db.wal_size", true,
					fmt.Sprintf("%.1fMB", walSizeMB)})
			}
		}

		// Try a passive WAL checkpoint to reclaim space.
		_ = app.DB.Exec(ctx, "PRAGMA wal_checkpoint(PASSIVE)")
	}

	// Session count check.
	var totalSessions, workingSessions, completedSessions int
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sessions")
	_ = row.Scan(&totalSessions)
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sessions WHERE state = 'working'")
	_ = row.Scan(&workingSessions)
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM sessions WHERE state = 'completed'")
	_ = row.Scan(&completedSessions)

	detail := fmt.Sprintf("total=%d working=%d completed=%d", totalSessions, workingSessions, completedSessions)
	if totalSessions > 200 {
		checks = append(checks, doctorCheck{"db.sessions", false,
			fmt.Sprintf("%s (excessive; consider pruning old completed sessions)", detail)})
	} else {
		checks = append(checks, doctorCheck{"db.sessions", true, detail})
	}

	// Events table size.
	var eventCount int
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM events")
	_ = row.Scan(&eventCount)
	if eventCount > 1000 {
		checks = append(checks, doctorCheck{"db.events", false,
			fmt.Sprintf("%d events (excessive; consider pruning)", eventCount)})
	} else {
		checks = append(checks, doctorCheck{"db.events", true, fmt.Sprintf("%d events", eventCount)})
	}

	// Mail table size.
	var mailCount int
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM mail")
	_ = row.Scan(&mailCount)
	checks = append(checks, doctorCheck{"db.mail", true, fmt.Sprintf("%d messages", mailCount)})

	// Metrics check.
	var metricsCount int
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM metrics")
	_ = row.Scan(&metricsCount)
	if metricsCount == 0 && completedSessions > 0 {
		checks = append(checks, doctorCheck{"db.metrics", false,
			"no metrics recorded despite completed sessions (token tracking may be broken)"})
	} else {
		checks = append(checks, doctorCheck{"db.metrics", true, fmt.Sprintf("%d records", metricsCount)})
	}

	return checks
}

// checkZombiePanes counts orphaned pane processes (cmdr status/feed/mail/merge --pane)
// that are still running but may not be attached to any visible zellij pane.
func checkZombiePanes() []doctorCheck {
	var checks []doctorCheck

	paneCommands := []string{"status --pane", "feed --pane", "mail list --pane", "merge list --pane", "evals --pane"}

	for _, paneCmd := range paneCommands {
		out, err := exec.Command("pgrep", "-fc", fmt.Sprintf("cmdr %s", paneCmd)).Output()
		if err != nil {
			continue
		}
		count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		name := fmt.Sprintf("zombie.%s", strings.Fields(paneCmd)[0])
		if count > 2 {
			checks = append(checks, doctorCheck{name, false,
				fmt.Sprintf("%d processes running (expected <= 2; kill stale ones)", count)})
		} else if count > 0 {
			checks = append(checks, doctorCheck{name, true,
				fmt.Sprintf("%d process(es)", count)})
		}
	}

	return checks
}
