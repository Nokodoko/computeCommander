package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// LazyGitPane embeds lazygit inside a PTY using the VTerm emulator.
// It follows the identical pattern as FilePicker and AgentSession.
type LazyGitPane struct {
	root    string // project root directory
	cmd     *exec.Cmd
	ptyFile *os.File
	vterm   *VTerm
	mu      sync.Mutex
	width   int
	height  int
	running bool
	theme   *Theme

	// notify signals the bubbletea event loop when new PTY output arrives.
	notify chan struct{}
}

// NewLazyGitPane constructs a LazyGitPane rooted at the given directory.
func NewLazyGitPane(root string, theme *Theme) *LazyGitPane {
	return &LazyGitPane{
		root:   root,
		theme:  theme,
		vterm:  NewVTerm(30, 20), // default size, resized on Start
		notify: make(chan struct{}, 1),
	}
}

// Notify returns the channel that signals new PTY output is available.
func (lg *LazyGitPane) Notify() <-chan struct{} {
	return lg.notify
}

// Start spawns the lazygit process inside a PTY.
func (lg *LazyGitPane) Start(width, height int) error {
	// Guard against double-start.
	if lg.Running() {
		return nil
	}

	lg.width = width
	lg.height = height

	// Initialize the virtual terminal with the pane dimensions.
	lg.vterm = NewVTerm(width, height)

	lgPath, err := exec.LookPath("lazygit")
	if err != nil {
		return fmt.Errorf("lazygit not found in PATH: %w", err)
	}

	lg.cmd = exec.Command(lgPath)
	lg.cmd.Dir = lg.root
	lg.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(lg.cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("start lazygit pty: %w", err)
	}
	lg.ptyFile = ptmx
	lg.mu.Lock()
	lg.running = true
	lg.mu.Unlock()

	go lg.readLoop()

	return nil
}

// readLoop reads from the PTY and feeds bytes into the virtual terminal.
func (lg *LazyGitPane) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := lg.ptyFile.Read(buf)
		if n > 0 {
			// VTerm.Write is internally synchronized.
			lg.vterm.Write(buf[:n])
			// Signal the bubbletea event loop that new output is available.
			select {
			case lg.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				// Process ended unexpectedly.
			}
			lg.mu.Lock()
			lg.running = false
			lg.mu.Unlock()
			return
		}
	}
}

// Stop terminates the lazygit process.
func (lg *LazyGitPane) Stop() error {
	if lg.cmd != nil && lg.cmd.Process != nil {
		_ = lg.cmd.Process.Signal(os.Interrupt)
	}
	if lg.ptyFile != nil {
		_ = lg.ptyFile.Close()
	}
	lg.mu.Lock()
	lg.running = false
	lg.mu.Unlock()
	return nil
}

// WriteInput sends keyboard input to the lazygit process.
func (lg *LazyGitPane) WriteInput(data []byte) error {
	if !lg.Running() || lg.ptyFile == nil {
		return fmt.Errorf("lazygit not running")
	}
	_, err := lg.ptyFile.Write(data)
	return err
}

// Running returns whether the lazygit process is active.
func (lg *LazyGitPane) Running() bool {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	return lg.running
}

// SetSize updates the display dimensions, resizes the virtual terminal, and
// resizes the PTY if running.
func (lg *LazyGitPane) SetSize(w, h int) {
	lg.width = w
	lg.height = h
	if lg.vterm != nil {
		lg.vterm.Resize(w, h)
	}
	if lg.ptyFile != nil && lg.Running() {
		_ = pty.Setsize(lg.ptyFile, &pty.Winsize{
			Rows: uint16(h),
			Cols: uint16(w),
		})
	}
}

// View renders the lazygit terminal output for display.
func (lg *LazyGitPane) View() string {
	if !lg.Running() {
		return lg.theme.Subtitle.Render("  lazygit not started. Press Enter to launch.")
	}

	return lg.vterm.Render()
}
