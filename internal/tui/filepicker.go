package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
			// forward arrow keys to the PTY if running
			if m.fp.Running() {
				_ = m.fp.WriteInput([]byte("\x1b[A"))
			}
		case "down", "j":
			if m.fp.Running() {
				_ = m.fp.WriteInput([]byte("\x1b[B"))
			}
		case "enter", "l", "right":
			if m.fp.Running() {
				_ = m.fp.WriteInput([]byte("\r"))
			}
		case "backspace", "h", "left":
			if m.fp.Running() {
				_ = m.fp.WriteInput([]byte("\x7f"))
			}
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
	theme := DefaultTheme()
	fp := NewFilePicker(startPath, theme)
	m := FilePickerModel{fp: fp}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
