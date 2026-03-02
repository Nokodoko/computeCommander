package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

// AgentSession manages an embedded PTY terminal running an agent command.
type AgentSession struct {
	cmd      *exec.Cmd
	ptyFile  *os.File
	vterm    *VTerm
	mu       sync.Mutex
	width    int
	height   int
	agentCmd string
	running  bool
	theme    *Theme

	// notify is a buffered channel that signals the bubbletea event loop
	// when new PTY output has been written to the VTerm. The readLoop sends
	// a non-blocking signal after each Write; the Dashboard subscribes via
	// a tea.Cmd that waits on this channel.
	notify chan struct{}
}

// NewAgentSession constructs an AgentSession with the given command string.
func NewAgentSession(agentCmd string, theme *Theme) *AgentSession {
	return &AgentSession{
		agentCmd: agentCmd,
		theme:    theme,
		vterm:    NewVTerm(80, 24), // default size, resized on Start
		notify:   make(chan struct{}, 1),
	}
}

// Notify returns the channel that signals new PTY output is available.
// The Dashboard subscribes to this via a tea.Cmd to trigger re-renders
// promptly when agent output arrives, rather than waiting for the next tick.
func (a *AgentSession) Notify() <-chan struct{} {
	return a.notify
}

// Start spawns the PTY process. It is safe to call multiple times;
// subsequent calls while the process is already running are no-ops.
func (a *AgentSession) Start(width, height int) error {
	// Guard against double-start.
	if a.Running() {
		return nil
	}

	a.width = width
	a.height = height

	// Initialize the virtual terminal with the pane dimensions.
	a.vterm = NewVTerm(width, height)

	parts := shellSplit(a.agentCmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty agent command")
	}

	a.cmd = exec.Command(parts[0], parts[1:]...)
	a.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(a.cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}
	a.ptyFile = ptmx
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	// Read PTY output in background.
	go a.readLoop()

	return nil
}

// readLoop reads from the PTY and feeds bytes into the virtual terminal.
func (a *AgentSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := a.ptyFile.Read(buf)
		if n > 0 {
			// VTerm.Write is internally synchronized.
			a.vterm.Write(buf[:n])
			// Signal the bubbletea event loop that new output is available.
			// Non-blocking send: if a signal is already pending, skip.
			select {
			case a.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				// Process ended.
			}
			a.mu.Lock()
			a.running = false
			a.mu.Unlock()
			return
		}
	}
}

// Stop terminates the PTY process.
func (a *AgentSession) Stop() error {
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Signal(os.Interrupt)
	}
	if a.ptyFile != nil {
		_ = a.ptyFile.Close()
	}
	a.running = false
	return nil
}

// Resize updates the PTY window size and the virtual terminal buffer.
func (a *AgentSession) Resize(width, height int) {
	a.width = width
	a.height = height
	if a.vterm != nil {
		a.vterm.Resize(width, height)
	}
	if a.ptyFile != nil && a.Running() {
		_ = pty.Setsize(a.ptyFile, &pty.Winsize{
			Rows: uint16(height),
			Cols: uint16(width),
		})
	}
}

// WriteInput sends keyboard input to the PTY.
func (a *AgentSession) WriteInput(data []byte) error {
	if !a.running || a.ptyFile == nil {
		return fmt.Errorf("agent session not running")
	}
	_, err := a.ptyFile.Write(data)
	return err
}

// Running returns whether the PTY process is active.
func (a *AgentSession) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// View renders the terminal output for display. The virtual terminal processes
// all ANSI escape sequences internally, so only safe SGR (color/style) codes
// appear in the output. Cursor movement, screen clearing, alternate buffer,
// and other control sequences are consumed by the VTerm and never leak into
// the bubbletea rendering pipeline.
func (a *AgentSession) View() string {
	if !a.Running() {
		return a.theme.Subtitle.Render("Agent session not started. Press Enter to launch.")
	}

	return a.vterm.Render()
}

// SetSize updates display dimensions.
func (a *AgentSession) SetSize(w, h int) {
	a.Resize(w, h)
}

// shellSplit performs a simple shell-like splitting of a command string.
func shellSplit(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(ch)
			}
		} else if ch == '\'' || ch == '"' {
			inQuote = true
			quoteChar = ch
		} else if ch == ' ' || ch == '\t' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
