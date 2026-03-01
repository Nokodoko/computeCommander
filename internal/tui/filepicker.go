package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FilePickerModel is a bubbletea Model that wraps FilePicker for standalone use
// inside a zellij pane (cmdr fp).
type FilePickerModel struct {
	fp *FilePicker
}

// NewFilePickerModel creates a bubbletea model for the file picker pane.
func NewFilePickerModel(startPath string) FilePickerModel {
	theme := DefaultTheme()
	return FilePickerModel{fp: NewFilePicker(startPath, theme)}
}

func (m FilePickerModel) Init() tea.Cmd { return nil }

func (m FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.fp.CursorUp()
		case "down", "j":
			m.fp.CursorDown()
		case "enter", "l", "right":
			m.fp.Enter()
		case "backspace", "h", "left":
			m.fp.GoUp()
		}
	case tea.WindowSizeMsg:
		m.fp.SetSize(msg.Width, msg.Height)
	}
	return m, nil
}

func (m FilePickerModel) View() string {
	return m.fp.View()
}

// RunFilePicker runs the file picker as a standalone bubbletea program.
func RunFilePicker(startPath string) error {
	m := NewFilePickerModel(startPath)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// FilePickerEntry represents a single directory entry in the file picker.
type FilePickerEntry struct {
	Name      string
	Path      string
	IsDir     bool
	HasSession bool   // Whether an agent session exists for this directory
	SessionID string  // Session ID if active
}

// FilePicker is the TUI component for the fp (file picker) pane.
// It provides directory navigation and session launching functionality.
type FilePicker struct {
	rootDir    string
	currentDir string
	entries    []FilePickerEntry
	cursor     int
	theme      *Theme
	width      int
	height     int
	sessions   map[string]string // path -> session ID mapping
}

// NewFilePicker creates a file picker starting at the given directory.
func NewFilePicker(rootDir string, theme *Theme) *FilePicker {
	if rootDir == "" {
		rootDir, _ = os.Getwd()
	}
	fp := &FilePicker{
		rootDir:    rootDir,
		currentDir: rootDir,
		theme:      theme,
		sessions:   make(map[string]string),
	}
	fp.loadEntries()
	return fp
}

// CurrentDir returns the current directory being browsed.
func (fp *FilePicker) CurrentDir() string {
	return fp.currentDir
}

// Selected returns the currently selected entry, or nil if none.
func (fp *FilePicker) Selected() *FilePickerEntry {
	if fp.cursor < 0 || fp.cursor >= len(fp.entries) {
		return nil
	}
	entry := fp.entries[fp.cursor]
	return &entry
}

// CursorDown moves the cursor down one entry.
func (fp *FilePicker) CursorDown() {
	if fp.cursor < len(fp.entries)-1 {
		fp.cursor++
	}
}

// CursorUp moves the cursor up one entry.
func (fp *FilePicker) CursorUp() {
	if fp.cursor > 0 {
		fp.cursor--
	}
}

// Enter navigates into the selected directory or triggers session creation.
func (fp *FilePicker) Enter() (selectedPath string, isDir bool) {
	sel := fp.Selected()
	if sel == nil {
		return "", false
	}

	if sel.IsDir {
		fp.currentDir = sel.Path
		fp.cursor = 0
		fp.loadEntries()
		return sel.Path, true
	}

	return sel.Path, false
}

// GoUp navigates to the parent directory.
func (fp *FilePicker) GoUp() {
	parent := filepath.Dir(fp.currentDir)
	if parent != fp.currentDir {
		fp.currentDir = parent
		fp.cursor = 0
		fp.loadEntries()
	}
}

// SetSize updates the pane dimensions.
func (fp *FilePicker) SetSize(width, height int) {
	fp.width = width
	fp.height = height
}

// SetSessionActive marks a directory as having an active session.
func (fp *FilePicker) SetSessionActive(dirPath, sessionID string) {
	fp.sessions[dirPath] = sessionID
	fp.refreshSessionMarkers()
}

// RemoveSession removes the session marker for a directory.
func (fp *FilePicker) RemoveSession(dirPath string) {
	delete(fp.sessions, dirPath)
	fp.refreshSessionMarkers()
}

// ActiveSessions returns the map of active session directories.
func (fp *FilePicker) ActiveSessions() map[string]string {
	return fp.sessions
}

// loadEntries reads the current directory and populates the entry list.
func (fp *FilePicker) loadEntries() {
	fp.entries = nil

	// Add parent directory entry (unless we're at root).
	if fp.currentDir != "/" {
		fp.entries = append(fp.entries, FilePickerEntry{
			Name:  "..",
			Path:  filepath.Dir(fp.currentDir),
			IsDir: true,
		})
	}

	dirEntries, err := os.ReadDir(fp.currentDir)
	if err != nil {
		return
	}

	// Sort: directories first, then files, alphabetically within each group.
	sort.Slice(dirEntries, func(i, j int) bool {
		di, dj := dirEntries[i].IsDir(), dirEntries[j].IsDir()
		if di != dj {
			return di // dirs first
		}
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	for _, entry := range dirEntries {
		name := entry.Name()
		// Skip hidden files/dirs.
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(fp.currentDir, name)
		fpEntry := FilePickerEntry{
			Name:  name,
			Path:  fullPath,
			IsDir: entry.IsDir(),
		}

		// Check if this directory has an active session.
		if sessionID, ok := fp.sessions[fullPath]; ok {
			fpEntry.HasSession = true
			fpEntry.SessionID = sessionID
		}

		fp.entries = append(fp.entries, fpEntry)
	}
}

// refreshSessionMarkers updates the session markers on current entries.
func (fp *FilePicker) refreshSessionMarkers() {
	for i := range fp.entries {
		if sessionID, ok := fp.sessions[fp.entries[i].Path]; ok {
			fp.entries[i].HasSession = true
			fp.entries[i].SessionID = sessionID
		} else {
			fp.entries[i].HasSession = false
			fp.entries[i].SessionID = ""
		}
	}
}

// View renders the file picker pane.
func (fp *FilePicker) View() string {
	var sb strings.Builder

	// Breadcrumb header.
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FFFF")).
		Width(fp.width)

	// Shorten path for display.
	displayPath := fp.currentDir
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(displayPath, home) {
		displayPath = "~" + displayPath[len(home):]
	}

	sb.WriteString(headerStyle.Render(fmt.Sprintf(" %s", displayPath)))
	sb.WriteString("\n")

	// Entries.
	maxVisible := fp.height - 3
	if maxVisible < 1 {
		maxVisible = 10
	}

	// Calculate scroll window.
	start := 0
	if fp.cursor >= maxVisible {
		start = fp.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(fp.entries) {
		end = len(fp.entries)
	}

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Width(fp.width)
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#00FFFF")).
		Width(fp.width)
	dirStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3B82F6")).
		Bold(true)
	sessionMarker := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")).
		Bold(true)

	for i := start; i < end; i++ {
		entry := fp.entries[i]

		var prefix, suffix string
		if entry.IsDir {
			prefix = dirStyle.Render("/")
		} else {
			prefix = " "
		}
		if entry.HasSession {
			suffix = sessionMarker.Render(" *")
		}

		line := fmt.Sprintf(" %s%s%s", prefix, entry.Name, suffix)

		if i == fp.cursor {
			sb.WriteString(selectedStyle.Render(line))
		} else {
			sb.WriteString(normalStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	// Footer with session count.
	activeCount := len(fp.sessions)
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Width(fp.width)
	sb.WriteString(footerStyle.Render(fmt.Sprintf(" %d sessions active", activeCount)))

	return sb.String()
}
