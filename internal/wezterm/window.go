// Package wezterm provides window management for Wezterm terminals.
// It spawns new Wezterm windows that dwm manages as X11 clients,
// each running a Zellij session with a specified layout.
package wezterm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WindowManager spawns and manages Wezterm windows.
type WindowManager interface {
	SpawnWindow(ctx context.Context, opts SpawnWindowOpts) error
	FocusWindow(sessionName string) error
	ListWindows() ([]*Window, error)
}

// SpawnWindowOpts configures a new Wezterm window.
type SpawnWindowOpts struct {
	ZellijSession string   // Zellij session name to create (used for WM_CLASS)
	WorkDir       string   // Working directory
	Layout        string   // Zellij layout file path (used when Command is empty)
	Command       []string // If set, run this command instead of zellij
}

// Window represents a Wezterm window running a Zellij session.
type Window struct {
	PID           int
	Title         string
	ZellijSession string
	WorkDir       string
}

// WindowError wraps window operation errors with context.
type WindowError struct {
	Op   string
	Name string
	Err  error
}

func (e *WindowError) Error() string {
	return fmt.Sprintf("wezterm %s %q: %v", e.Op, e.Name, e.Err)
}

func (e *WindowError) Unwrap() error {
	return e.Err
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	Start(ctx context.Context, name string, args ...string) error
}

// execRunner implements CommandRunner using os/exec.
type execRunner struct{}

func (e *execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (e *execRunner) Start(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	// Detach from parent - don't wait for completion
	go cmd.Wait()
	return nil
}

// Manager implements WindowManager by shelling out to wezterm CLI.
type Manager struct {
	classPrefix string
	runner      CommandRunner
}

// NewManager creates a Manager with the given class prefix for WM_CLASS.
func NewManager(classPrefix string) *Manager {
	if classPrefix == "" {
		classPrefix = "cc"
	}
	return &Manager{
		classPrefix: classPrefix,
		runner:      &execRunner{},
	}
}

// NewManagerWithRunner creates a Manager with a custom command runner (for testing).
func NewManagerWithRunner(classPrefix string, runner CommandRunner) *Manager {
	if classPrefix == "" {
		classPrefix = "cc"
	}
	return &Manager{
		classPrefix: classPrefix,
		runner:      runner,
	}
}

// SpawnWindow creates a new Wezterm window (new dwm X11 client).
// The window gets a WM_CLASS of "<prefix>-<session>" for dwm identification.
//
// When opts.Command is set, that command runs directly inside wezterm (with
// ZELLIJ env vars unset). This is used for the Bubbletea TUI dashboard.
//
// When opts.Command is empty, the window runs a Zellij session:
//  1. wezterm start --class cc-<session> -- creates new X11 window
//  2. zellij --session <session> --new-session-with-layout <layout> -- starts zellij
//  3. Layout defines panes (agent picker, mail, etc.)
//
// Environment: ZELLIJ, ZELLIJ_SESSION_NAME, and ZELLIJ_PANE_ID are always
// cleared in the spawned shell so zellij doesn't attempt to nest sessions.
func (m *Manager) SpawnWindow(ctx context.Context, opts SpawnWindowOpts) error {
	if opts.ZellijSession == "" {
		return &WindowError{Op: "spawn", Name: "", Err: fmt.Errorf("zellij session name is required")}
	}

	wmClass := fmt.Sprintf("%s-%s", m.classPrefix, opts.ZellijSession)

	var shellCmd string
	if len(opts.Command) > 0 {
		// Run an explicit command (e.g. "cmdr dashboard --tui") instead of zellij.
		// Still unset zellij env vars so the TUI doesn't think it's nested.
		quotedArgs := make([]string, len(opts.Command))
		for i, a := range opts.Command {
			quotedArgs[i] = fmt.Sprintf("%q", a)
		}
		shellCmd = fmt.Sprintf("unset ZELLIJ ZELLIJ_SESSION_NAME ZELLIJ_PANE_ID; exec %s", strings.Join(quotedArgs, " "))
	} else {
		// Build zellij command.
		// Use --new-session-with-layout (not --layout) so that zellij always
		// creates a new session. With --layout + --session, zellij 0.43+
		// interprets it as "add tabs to existing session" which fails when the
		// session doesn't exist yet.
		zellijArgs := []string{"zellij", "--session", opts.ZellijSession}
		if opts.Layout != "" {
			zellijArgs = append(zellijArgs, "--new-session-with-layout", opts.Layout)
		}
		zellijCmd := strings.Join(zellijArgs, " ")

		// Clear inherited zellij env vars so the new session isn't treated as
		// nested inside whatever zellij session the user is running cmdr from.
		shellCmd = fmt.Sprintf("unset ZELLIJ ZELLIJ_SESSION_NAME ZELLIJ_PANE_ID; %s", zellijCmd)
	}

	// Build wezterm args - spawn a NEW window (dwm client)
	args := []string{
		"start",
		"--class", wmClass,
	}

	if opts.WorkDir != "" {
		args = append(args, "--cwd", opts.WorkDir)
	}

	// Tell wezterm to run the command directly (bypasses zsh auto-start)
	args = append(args, "--")
	args = append(args, "sh", "-c", shellCmd)

	if err := m.runner.Start(ctx, "wezterm", args...); err != nil {
		return &WindowError{Op: "spawn", Name: opts.ZellijSession, Err: err}
	}

	// Brief pause to let window spawn
	time.Sleep(100 * time.Millisecond)

	return nil
}

// FocusWindow attempts to focus a window by its Zellij session name.
// Uses wmctrl to find and activate the window by WM_CLASS.
func (m *Manager) FocusWindow(sessionName string) error {
	wmClass := fmt.Sprintf("%s-%s", m.classPrefix, sessionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := m.runner.Run(ctx, "wmctrl", "-x", "-a", wmClass)
	if err != nil {
		return &WindowError{Op: "focus", Name: sessionName, Err: err}
	}

	return nil
}

// ListWindows returns all Wezterm windows managed by this prefix.
// Uses wmctrl to enumerate windows and filters by WM_CLASS prefix.
func (m *Manager) ListWindows() ([]*Window, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := m.runner.Run(ctx, "wmctrl", "-l", "-x")
	if err != nil {
		return nil, &WindowError{Op: "list", Name: "", Err: err}
	}

	return m.parseWindowList(string(out)), nil
}

// parseWindowList parses wmctrl -l -x output into Window structs.
func (m *Manager) parseWindowList(output string) []*Window {
	var windows []*Window
	prefix := m.classPrefix + "-"

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// wmctrl -l -x format: <window_id> <desktop> <class> <host> <title>
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		class := fields[2]
		// Class format is "instance.class" - we want the class part
		parts := strings.Split(class, ".")
		if len(parts) > 1 {
			class = parts[1]
		}

		if !strings.HasPrefix(class, prefix) {
			continue
		}

		sessionName := strings.TrimPrefix(class, prefix)
		title := ""
		if len(fields) > 4 {
			title = strings.Join(fields[4:], " ")
		}

		windows = append(windows, &Window{
			Title:         title,
			ZellijSession: sessionName,
		})
	}

	return windows
}
