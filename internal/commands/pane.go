package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// clearScreen sends ANSI escape codes to clear the terminal and move the cursor
// to the top-left corner. Used by --pane mode commands in the zellij dashboard.
func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// hexToRGB converts a hex color string (e.g. "#FF5733" or "FF5733") to an
// "R;G;B" string suitable for ANSI 24-bit color escape sequences.
// Returns "255;255;255" as a fallback if parsing fails.
func hexToRGB(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "255;255;255"
	}
	r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return "255;255;255"
	}
	return fmt.Sprintf("%d;%d;%d", r, g, b)
}

// colorizeAgent wraps the given text in ANSI 24-bit foreground color escape sequences
// using the provided hex color. Returns the text unchanged if hex is empty.
func colorizeAgent(text, hex string) string {
	if hex == "" {
		return text
	}
	return fmt.Sprintf("\033[38;2;%sm%s\033[0m", hexToRGB(hex), text)
}

// watchDBFile sets up an fsnotify watcher on the SQLite DB file used by the app.
// Returns a channel that fires whenever the DB file is written to. This provides
// instant refresh when cmdr-bridge.sh (or any process) modifies the database,
// with zero coupling — no PID files, no signals, just filesystem events.
// Returns a closed channel (never fires) if the watcher cannot be created,
// so the caller can always safely select on it alongside a fallback ticker.
func watchDBFile(app *App) <-chan struct{} {
	ch := make(chan struct{}, 1)

	// Determine the DB file path from config.
	dbPath := ""
	if app.Config != nil {
		dbPath = app.Config.Database.SQLite.Path
	}
	if dbPath == "" {
		close(ch)
		return ch
	}

	// Also watch the WAL file — SQLite in WAL mode writes there first.
	walPath := dbPath + "-wal"

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		close(ch)
		return ch
	}

	// Watch the directory containing the DB file so we catch file creation
	// (e.g., WAL file appearing) as well as writes.
	dbDir := filepath.Dir(dbPath)
	if err := watcher.Add(dbDir); err != nil {
		watcher.Close()
		close(ch)
		return ch
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Only trigger on writes to the DB or WAL file.
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				name := filepath.Base(event.Name)
				dbBase := filepath.Base(dbPath)
				walBase := filepath.Base(walPath)
				if name != dbBase && name != walBase {
					continue
				}
				// Non-blocking send — coalesce multiple rapid writes into one refresh.
				select {
				case ch <- struct{}{}:
				default:
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return ch
}

// binaryWatcher monitors the cmdr binary on disk and re-execs the current
// process when a rebuild is detected. This prevents pane processes from
// running a stale (deleted) binary after `go build` replaces the executable.
//
// Call this from any --pane mode goroutine; it never returns under normal
// conditions (it execs or keeps watching). If the executable path cannot
// be resolved, or the binary disappears, it silently continues watching.
type binaryWatcher struct {
	exe     string
	modTime time.Time
}

// newBinaryWatcher snapshots the current executable's modification time.
// Returns nil if the executable cannot be stat'd (e.g., in tests).
func newBinaryWatcher() *binaryWatcher {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	info, err := os.Stat(exe)
	if err != nil {
		return nil
	}
	return &binaryWatcher{exe: exe, modTime: info.ModTime()}
}

// check returns true if the binary has changed on disk since the watcher
// was created. Safe to call from a ticker loop.
func (w *binaryWatcher) check() bool {
	if w == nil {
		return false
	}
	info, err := os.Stat(w.exe)
	if err != nil {
		return false
	}
	return !info.ModTime().Equal(w.modTime)
}

// reexec replaces the current process with a fresh invocation of the
// on-disk binary, preserving the original arguments and environment.
// This is the pane-mode equivalent of a hot reload.
func (w *binaryWatcher) reexec() {
	if w == nil {
		return
	}
	// Small delay so the build has time to finish writing the binary.
	time.Sleep(500 * time.Millisecond)
	_ = syscall.Exec(w.exe, os.Args, os.Environ())
}
