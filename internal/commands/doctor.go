package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/platform/db"
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

	// Database health checks (deep) on the active DB.
	if app.DB != nil {
		checks = append(checks, runDBHealthChecks(app)...)
	}

	// Multi-database cross-checks: compare project-local, system-wide, and legacy DBs.
	checks = append(checks, checkMultiDB(app)...)

	// Bridge hook connectivity check.
	checks = append(checks, checkBridgeHook(app)...)

	// Zombie pane process check.
	checks = append(checks, checkZombiePanes()...)

	// Pane process binary freshness check.
	checks = append(checks, checkPaneBinaryFreshness()...)

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

	// WAL journal mode check.
	var journalMode string
	row = app.DB.QueryRow(ctx, "PRAGMA journal_mode")
	if err := row.Scan(&journalMode); err == nil && journalMode != "wal" {
		checks = append(checks, doctorCheck{"db.journal_mode", false,
			fmt.Sprintf("journal_mode=%s (should be wal; run PRAGMA journal_mode=WAL)", journalMode)})
	} else {
		checks = append(checks, doctorCheck{"db.journal_mode", true, journalMode})
	}

	// WAL file size check.
	dbPath := resolveDBPath(app)
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

		// Attempt a passive WAL checkpoint to reclaim space.
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

	// Zombie session detection: working agents with no recent activity (>10min stale).
	var zombieCount int
	cutoff := time.Now().Add(-10 * time.Minute).UTC().Format("2006-01-02T15:04:05Z")
	row = app.DB.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions WHERE state IN ('working','booting')
		 AND REPLACE(REPLACE(last_activity, ' ', 'T'), 'Z', '') < REPLACE(REPLACE(?, ' ', 'T'), 'Z', '')`,
		cutoff)
	if err := row.Scan(&zombieCount); err == nil && zombieCount > 0 {
		checks = append(checks, doctorCheck{"db.zombies", false,
			fmt.Sprintf("%d zombie session(s) with state=working but no activity in 10+ min", zombieCount)})
	} else {
		checks = append(checks, doctorCheck{"db.zombies", true, "none"})
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

	// Migration count / schema version.
	var migrationCount int
	var latestMigration string
	row = app.DB.QueryRow(ctx, "SELECT COUNT(*), MAX(name) FROM _migrations")
	if err := row.Scan(&migrationCount, &latestMigration); err == nil {
		checks = append(checks, doctorCheck{"db.schema", true,
			fmt.Sprintf("%d migration(s) applied, latest=%s", migrationCount, latestMigration)})
	}

	return checks
}

// checkMultiDB opens the other known SQLite databases and compares their
// schema versions. Reports mismatches that indicate one DB is behind.
func checkMultiDB(app *App) []doctorCheck {
	var checks []doctorCheck

	home, err := os.UserHomeDir()
	if err != nil {
		return checks
	}

	candidates := []struct {
		label string
		path  string
	}{
		{"db.system", filepath.Join(home, ".computecommander", "local.db")},
		{"db.legacy", filepath.Join(home, ".computecommander", "cc.db")},
	}

	// Active DB migration count for comparison.
	activeMigrations := -1
	if app.DB != nil {
		ctx := context.Background()
		row := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM _migrations")
		_ = row.Scan(&activeMigrations)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c.path); os.IsNotExist(err) {
			checks = append(checks, doctorCheck{c.label, true, "not present"})
			continue
		}

		altDB, err := db.NewDB(db.DatabaseConfig{
			Driver: "sqlite",
			SQLite: struct{ Path string `yaml:"path"` }{Path: c.path},
		})
		if err != nil {
			checks = append(checks, doctorCheck{c.label, false, fmt.Sprintf("cannot open: %v", err)})
			continue
		}
		defer altDB.Close()

		ctx := context.Background()

		// Integrity check.
		var integrity string
		row := altDB.QueryRow(ctx, "PRAGMA integrity_check")
		_ = row.Scan(&integrity)
		if integrity != "ok" {
			checks = append(checks, doctorCheck{c.label + ".integrity", false,
				fmt.Sprintf("integrity issues: %s", integrity)})
		}

		// WAL check and passive checkpoint.
		walPath := c.path + "-wal"
		if info, err := os.Stat(walPath); err == nil {
			walMB := float64(info.Size()) / (1024 * 1024)
			if walMB > 1.0 {
				checks = append(checks, doctorCheck{c.label + ".wal", false,
					fmt.Sprintf("%.1fMB WAL (run PRAGMA wal_checkpoint(TRUNCATE))", walMB)})
				_ = altDB.Exec(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
			}
		}

		// Migration count comparison.
		var altMigrations int
		var latestName string
		row = altDB.QueryRow(ctx, "SELECT COUNT(*), MAX(name) FROM _migrations")
		if err := row.Scan(&altMigrations, &latestName); err == nil {
			if activeMigrations >= 0 && altMigrations != activeMigrations {
				checks = append(checks, doctorCheck{c.label + ".schema", false,
					fmt.Sprintf("%d migration(s) applied (active DB has %d); latest=%s — run migrations to sync",
						altMigrations, activeMigrations, latestName)})
			} else {
				checks = append(checks, doctorCheck{c.label + ".schema", true,
					fmt.Sprintf("%d migration(s), latest=%s", altMigrations, latestName)})
			}
		}

		// Zombie sessions.
		var zombies int
		cutoff := time.Now().Add(-10 * time.Minute).UTC().Format("2006-01-02T15:04:05Z")
		row = altDB.QueryRow(ctx,
			`SELECT COUNT(*) FROM sessions WHERE state IN ('working','booting')
			 AND REPLACE(REPLACE(last_activity, ' ', 'T'), 'Z', '') < REPLACE(REPLACE(?, ' ', 'T'), 'Z', '')`,
			cutoff)
		if err := row.Scan(&zombies); err == nil && zombies > 0 {
			checks = append(checks, doctorCheck{c.label + ".zombies", false,
				fmt.Sprintf("%d zombie session(s) (stale working/booting agents)", zombies)})
		}
	}

	return checks
}

// checkBridgeHook verifies that the cmdr-bridge.sh hook exists, is executable,
// and that the bridge log shows recent activity (a proxy for DB write access).
func checkBridgeHook(app *App) []doctorCheck {
	var checks []doctorCheck

	home, err := os.UserHomeDir()
	if err != nil {
		return checks
	}

	hookPath := filepath.Join(home, ".claude", "hooks", "cmdr-bridge.sh")
	info, err := os.Stat(hookPath)
	if os.IsNotExist(err) {
		checks = append(checks, doctorCheck{"bridge.hook", false,
			fmt.Sprintf("not found at %s", hookPath)})
		return checks
	}
	if err != nil {
		checks = append(checks, doctorCheck{"bridge.hook", false, err.Error()})
		return checks
	}

	// Check executable bit.
	if info.Mode()&0111 == 0 {
		checks = append(checks, doctorCheck{"bridge.hook", false,
			fmt.Sprintf("%s is not executable; run chmod +x %s", hookPath, hookPath)})
	} else {
		checks = append(checks, doctorCheck{"bridge.hook", true,
			fmt.Sprintf("present and executable (%s)", hookPath)})
	}

	// Check bridge log for recent activity (within last 24h).
	logPath := "/tmp/cmdr-state/bridge.log"
	if logInfo, err := os.Stat(logPath); err == nil {
		age := time.Since(logInfo.ModTime())
		if age > 24*time.Hour {
			checks = append(checks, doctorCheck{"bridge.log", false,
				fmt.Sprintf("last activity %s ago (>24h; bridge may not be firing)", age.Round(time.Minute))})
		} else {
			checks = append(checks, doctorCheck{"bridge.log", true,
				fmt.Sprintf("last activity %s ago", age.Round(time.Second))})
		}

		// Check for recent WARN lines as an error indicator.
		out, err := exec.Command("grep", "-c", "\\[WARN\\]", logPath).Output()
		if err == nil {
			warnCount, _ := strconv.Atoi(strings.TrimSpace(string(out)))
			if warnCount > 10 {
				checks = append(checks, doctorCheck{"bridge.log.warns", false,
					fmt.Sprintf("%d WARN entries (run: grep WARN %s)", warnCount, logPath)})
			} else if warnCount > 0 {
				checks = append(checks, doctorCheck{"bridge.log.warns", true,
					fmt.Sprintf("%d WARN entries", warnCount)})
			}
		}
	} else {
		checks = append(checks, doctorCheck{"bridge.log", false,
			fmt.Sprintf("not found at %s (bridge has not run)", logPath)})
	}

	// Verify the DB the bridge would write to matches the active DB.
	activeDBPath := resolveDBPath(app)
	if activeDBPath != "" {
		checks = append(checks, checkBridgeDBAlignment(activeDBPath)...)
	}

	return checks
}

// checkBridgeDBAlignment checks whether the bridge hook's find_cmdr_db() resolution
// (which walks up from $PWD) would land on the same DB as the dashboard is reading.
func checkBridgeDBAlignment(activeDBPath string) []doctorCheck {
	var checks []doctorCheck

	// Simulate find_cmdr_db() from the current directory.
	cwd, err := os.Getwd()
	if err != nil {
		return checks
	}

	bridgeDB := ""
	dir := cwd
	for dir != "/" {
		candidate := filepath.Join(dir, ".computecommander", "local.db")
		if _, err := os.Stat(candidate); err == nil {
			bridgeDB = candidate
			break
		}
		dir = filepath.Dir(dir)
	}
	if bridgeDB == "" {
		home, _ := os.UserHomeDir()
		bridgeDB = filepath.Join(home, ".computecommander", "local.db")
	}

	// Normalize both paths before comparing.
	activeFull, _ := filepath.Abs(activeDBPath)
	bridgeFull, _ := filepath.Abs(bridgeDB)

	if activeFull != bridgeFull {
		checks = append(checks, doctorCheck{"bridge.db_alignment", false,
			fmt.Sprintf("dashboard reads %s but bridge writes to %s (agents from other dirs may be invisible)",
				activeFull, bridgeFull)})
	} else {
		checks = append(checks, doctorCheck{"bridge.db_alignment", true,
			fmt.Sprintf("dashboard and bridge use same DB (%s)", activeFull)})
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

// checkPaneBinaryFreshness checks if running pane processes use the current binary.
// If the binary on disk is newer than the running process's start time, the pane
// is stale and should be restarted to pick up the new binary.
func checkPaneBinaryFreshness() []doctorCheck {
	var checks []doctorCheck

	// Find the cmdr binary path.
	binaryPath, err := exec.LookPath("cmdr")
	if err != nil {
		return checks
	}
	binaryInfo, err := os.Stat(binaryPath)
	if err != nil {
		return checks
	}
	binaryMtime := binaryInfo.ModTime()

	// Find all cmdr --pane processes.
	out, err := exec.Command("pgrep", "-a", "-f", "cmdr.*--pane").Output()
	if err != nil {
		return checks
	}

	staleCount := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		// Use mtime of /proc/<pid> as a proxy for process start time.
		// A single stat on the proc dir suffices; non-existence means process exited.
		procInfo, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		if err != nil {
			continue
		}
		if binaryMtime.After(procInfo.ModTime()) {
			staleCount++
		}
	}

	if staleCount > 0 {
		checks = append(checks, doctorCheck{"pane.binary_freshness", false,
			fmt.Sprintf("%d pane process(es) using stale binary (binary updated since pane started; restart dashboard)", staleCount)})
	} else {
		checks = append(checks, doctorCheck{"pane.binary_freshness", true, "all pane processes use current binary"})
	}

	return checks
}

// resolveDBPath returns the absolute filesystem path to the active SQLite database.
func resolveDBPath(app *App) string {
	if app == nil || app.Config == nil {
		return ""
	}
	path := expandTildePath(app.Config.Database.SQLite.Path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
