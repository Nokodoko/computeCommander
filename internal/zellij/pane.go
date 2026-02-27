// Package zellij provides pane management for the Zellij terminal multiplexer.
// It shells out to the zellij CLI to create, list, send keys, capture, and close panes.
package zellij

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// PaneManager defines the interface for managing Zellij panes.
type PaneManager interface {
	CreatePane(opts CreatePaneOpts) (*Pane, error)
	ListPanes() ([]*Pane, error)
	SendKeys(paneID string, keys string) error
	CapturePaneContent(paneID string, lines int) (string, error)
	ClosePane(paneID string) error
}

// CreatePaneOpts configures a new pane.
type CreatePaneOpts struct {
	Layout   string   // "default", "vertical", "horizontal"
	WorkDir  string   // working directory for the pane
	Command  []string // command to run in the pane
	Name     string   // pane name
	Floating bool     // whether the pane should float
}

// Pane represents a Zellij pane.
type Pane struct {
	ID         string
	Name       string
	Title      string
	WorkDir    string
	IsFloating bool
	Command    string
}

// PaneOpts configures SpawnPane behavior.
type PaneOpts struct {
	WorkDir  string
	Floating bool
	Layout   string
}

// AttachOpts configures AttachFloating behavior.
type AttachOpts struct {
	Width  int
	Height int
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// execRunner implements CommandRunner using os/exec.
type execRunner struct{}

func (e *execRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// Manager implements PaneManager by shelling out to the zellij CLI.
type Manager struct {
	sessionPrefix string
	runner        CommandRunner
}

// NewManager creates a Manager with the given session prefix.
func NewManager(sessionPrefix string) *Manager {
	return &Manager{
		sessionPrefix: sessionPrefix,
		runner:        &execRunner{},
	}
}

// NewManagerWithRunner creates a Manager with a custom command runner (for testing).
func NewManagerWithRunner(sessionPrefix string, runner CommandRunner) *Manager {
	return &Manager{
		sessionPrefix: sessionPrefix,
		runner:        runner,
	}
}

// CreatePane creates a new Zellij pane with the given options.
func (m *Manager) CreatePane(opts CreatePaneOpts) (*Pane, error) {
	args := []string{"action", "new-pane"}

	if opts.Floating {
		args = append(args, "--floating")
	}

	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}

	switch opts.Layout {
	case "vertical":
		args = append(args, "--direction", "right")
	case "horizontal":
		args = append(args, "--direction", "down")
	case "default", "":
		// no direction flag needed
	}

	if opts.WorkDir != "" {
		args = append(args, "--cwd", opts.WorkDir)
	}

	if len(opts.Command) > 0 {
		args = append(args, "--")
		args = append(args, opts.Command...)
	}

	_, err := m.runner.Run("zellij", args...)
	if err != nil {
		return nil, fmt.Errorf("create pane: %w", err)
	}

	pane := &Pane{
		ID:         opts.Name, // zellij uses name as identifier in many commands
		Name:       opts.Name,
		WorkDir:    opts.WorkDir,
		IsFloating: opts.Floating,
	}
	if len(opts.Command) > 0 {
		pane.Command = strings.Join(opts.Command, " ")
	}

	return pane, nil
}

// ListPanes returns all panes in the current Zellij session.
func (m *Manager) ListPanes() ([]*Pane, error) {
	out, err := m.runner.Run("zellij", "action", "list-clients")
	if err != nil {
		// Fallback: try query-tab-names for basic info
		out, err = m.runner.Run("zellij", "action", "query-tab-names")
		if err != nil {
			return nil, fmt.Errorf("list panes: %w", err)
		}
	}

	return parsePaneList(string(out)), nil
}

// parsePaneList parses zellij output into Pane structs.
func parsePaneList(output string) []*Pane {
	var panes []*Pane
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		panes = append(panes, &Pane{
			Name:  line,
			Title: line,
		})
	}
	return panes
}

// SendKeys sends keystrokes to a specific pane.
func (m *Manager) SendKeys(paneID string, keys string) error {
	args := []string{"action", "write-chars", keys}
	_, err := m.runner.Run("zellij", args...)
	if err != nil {
		return fmt.Errorf("send keys to pane %s: %w", paneID, err)
	}
	return nil
}

// CapturePaneContent captures the visible content of a pane.
func (m *Manager) CapturePaneContent(paneID string, lines int) (string, error) {
	args := []string{"action", "dump-screen", "/dev/stdout"}
	out, err := m.runner.Run("zellij", args...)
	if err != nil {
		return "", fmt.Errorf("capture pane %s: %w", paneID, err)
	}

	content := string(out)
	if lines > 0 {
		content = lastNLines(content, lines)
	}
	return content, nil
}

// lastNLines returns the last n lines from a string.
func lastNLines(s string, n int) string {
	all := strings.Split(s, "\n")
	if len(all) <= n {
		return s
	}
	return strings.Join(all[len(all)-n:], "\n")
}

// ClosePane closes a pane by ID/name.
func (m *Manager) ClosePane(paneID string) error {
	args := []string{"action", "close-pane"}
	_, err := m.runner.Run("zellij", args...)
	if err != nil {
		return fmt.Errorf("close pane %s: %w", paneID, err)
	}
	return nil
}

// SpawnPane creates a named pane running the given command.
func (m *Manager) SpawnPane(name, cmd string, opts PaneOpts) (string, error) {
	command := strings.Fields(cmd)
	createOpts := CreatePaneOpts{
		Name:     name,
		Command:  command,
		WorkDir:  opts.WorkDir,
		Floating: opts.Floating,
		Layout:   opts.Layout,
	}
	pane, err := m.CreatePane(createOpts)
	if err != nil {
		return "", err
	}
	return pane.ID, nil
}

// AttachFloating focuses a floating pane by ID.
func (m *Manager) AttachFloating(paneID string, opts AttachOpts) error {
	args := []string{"action", "toggle-floating-panes"}
	_, err := m.runner.Run("zellij", args...)
	if err != nil {
		return fmt.Errorf("attach floating pane %s: %w", paneID, err)
	}
	return nil
}

// CapturePane is a convenience alias for CapturePaneContent.
func (m *Manager) CapturePane(paneID string, lines int) (string, error) {
	return m.CapturePaneContent(paneID, lines)
}
