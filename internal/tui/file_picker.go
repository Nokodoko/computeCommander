package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// FilePicker embeds the external `fp` file-picker TUI inside a PTY.
// It behaves like AgentSession: spawn the process, forward keystrokes,
// and render its terminal output via a virtual terminal emulator.
type FilePicker struct {
	root    string
	cmd     *exec.Cmd
	ptyFile *os.File
	vterm   *VTerm
	mu      sync.Mutex
	width   int
	height  int
	running bool
	theme   *Theme
	lastErr string // last error message for display

	// notify signals the bubbletea event loop when new PTY output arrives.
	notify chan struct{}
}

// NewFilePicker constructs a FilePicker rooted at the given directory.
func NewFilePicker(root string, theme *Theme) *FilePicker {
	return &FilePicker{
		root:   root,
		theme:  theme,
		vterm:  NewVTerm(30, 20), // default size, resized on Start
		notify: make(chan struct{}, 1),
	}
}

// Notify returns the channel that signals new PTY output is available.
func (fp *FilePicker) Notify() <-chan struct{} {
	return fp.notify
}

// Start spawns the fp process inside a PTY.
func (fp *FilePicker) Start(width, height int) error {
	// Guard against double-start.
	if fp.running {
		return nil
	}

	fp.width = width
	fp.height = height

	// Initialize the virtual terminal with the pane dimensions.
	fp.vterm = NewVTerm(width, height)

	fpPath, err := exec.LookPath("fp")
	if err != nil {
		return fmt.Errorf("fp not found in PATH: %w", err)
	}

	fp.cmd = exec.Command(fpPath, "--no-icons", fp.root)
	fp.cmd.Env = append(os.Environ(), "NO_COLOR=1")

	ptmx, err := pty.StartWithSize(fp.cmd, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("start fp pty: %w", err)
	}
	fp.ptyFile = ptmx
	fp.mu.Lock()
	fp.running = true
	fp.mu.Unlock()

	go fp.readLoop()

	return nil
}

// readLoop reads from the PTY and feeds bytes into the virtual terminal.
func (fp *FilePicker) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := fp.ptyFile.Read(buf)
		if n > 0 {
			// VTerm.Write is internally synchronized.
			fp.vterm.Write(buf[:n])
			// Signal the bubbletea event loop that new output is available.
			select {
			case fp.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			if err != io.EOF {
				// Process ended unexpectedly.
			}
			fp.mu.Lock()
			fp.running = false
			fp.mu.Unlock()
			return
		}
	}
}

// Stop terminates the fp process.
func (fp *FilePicker) Stop() error {
	if fp.cmd != nil && fp.cmd.Process != nil {
		_ = fp.cmd.Process.Signal(os.Interrupt)
	}
	if fp.ptyFile != nil {
		_ = fp.ptyFile.Close()
	}
	fp.running = false
	return nil
}

// Refresh is a no-op for the PTY-based picker. The fp process handles
// its own filesystem watching. This satisfies the dashboard refresh loop.
func (fp *FilePicker) Refresh() error {
	return nil
}

// WriteInput sends keyboard input to the fp process.
func (fp *FilePicker) WriteInput(data []byte) error {
	if !fp.running || fp.ptyFile == nil {
		return fmt.Errorf("file picker not running")
	}
	_, err := fp.ptyFile.Write(data)
	return err
}

// Running returns whether the fp process is active.
func (fp *FilePicker) Running() bool {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.running
}

// SetSize updates the display dimensions, resizes the virtual terminal, and
// resizes the PTY if running.
func (fp *FilePicker) SetSize(w, h int) {
	fp.width = w
	fp.height = h
	if fp.vterm != nil {
		fp.vterm.Resize(w, h)
	}
	if fp.ptyFile != nil && fp.Running() {
		_ = pty.Setsize(fp.ptyFile, &pty.Winsize{
			Rows: uint16(h),
			Cols: uint16(w),
		})
	}
}

// SetLastError records an error message for display in the View.
func (fp *FilePicker) SetLastError(msg string) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.lastErr = msg
}

// View renders the fp terminal output for display. The virtual terminal
// processes all ANSI escape sequences internally, so only safe SGR codes
// appear in the output.
func (fp *FilePicker) View() string {
	if !fp.Running() {
		fp.mu.Lock()
		errMsg := fp.lastErr
		fp.mu.Unlock()
		if errMsg != "" {
			return fp.theme.Subtitle.Render("fp error: " + errMsg + "\nPress Enter to retry.")
		}
		return fp.theme.Subtitle.Render("  File picker not started. Press Enter to launch.")
	}

	return fp.vterm.Render()
}
