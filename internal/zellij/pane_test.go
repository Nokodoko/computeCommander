package zellij

import (
	"fmt"
	"strings"
	"testing"
)

// mockRunner records commands and returns preset output.
type mockRunner struct {
	calls  []string
	output []byte
	err    error
}

func (m *mockRunner) Run(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return m.output, m.err
}

func TestNewManager(t *testing.T) {
	m := NewManager("cc")
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.sessionPrefix != "cc" {
		t.Errorf("sessionPrefix = %q, want %q", m.sessionPrefix, "cc")
	}
}

func TestCreatePane(t *testing.T) {
	tt := []struct {
		name    string
		opts    CreatePaneOpts
		wantErr bool
		wantCmd string
	}{
		{
			name: "basic pane",
			opts: CreatePaneOpts{
				Name: "worker-1",
			},
			wantCmd: "zellij action new-pane --name worker-1",
		},
		{
			name: "floating pane with command",
			opts: CreatePaneOpts{
				Name:     "scout",
				Floating: true,
				Command:  []string{"claude", "--task", "scout"},
			},
			wantCmd: "zellij action new-pane --floating --name scout -- claude --task scout",
		},
		{
			name: "vertical layout with workdir",
			opts: CreatePaneOpts{
				Name:    "builder",
				Layout:  "vertical",
				WorkDir: "/tmp/worktree",
			},
			wantCmd: "zellij action new-pane --name builder --direction right --cwd /tmp/worktree",
		},
		{
			name: "horizontal layout",
			opts: CreatePaneOpts{
				Name:   "reviewer",
				Layout: "horizontal",
			},
			wantCmd: "zellij action new-pane --name reviewer --direction down",
		},
		{
			name:    "runner error",
			opts:    CreatePaneOpts{Name: "fail"},
			wantErr: true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			runner := &mockRunner{}
			if tc.wantErr {
				runner.err = fmt.Errorf("zellij not found")
			}
			m := NewManagerWithRunner("cc", runner)

			pane, err := m.CreatePane(tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pane.Name != tc.opts.Name {
				t.Errorf("pane.Name = %q, want %q", pane.Name, tc.opts.Name)
			}
			if pane.IsFloating != tc.opts.Floating {
				t.Errorf("pane.IsFloating = %v, want %v", pane.IsFloating, tc.opts.Floating)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(runner.calls))
			}
			if runner.calls[0] != tc.wantCmd {
				t.Errorf("command = %q\n   want = %q", runner.calls[0], tc.wantCmd)
			}
		})
	}
}

func TestListPanes(t *testing.T) {
	runner := &mockRunner{
		output: []byte("tab-1\ntab-2\ntab-3\n"),
	}
	m := NewManagerWithRunner("cc", runner)

	panes, err := m.ListPanes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}
	if panes[0].Name != "tab-1" {
		t.Errorf("panes[0].Name = %q, want %q", panes[0].Name, "tab-1")
	}
}

func TestListPanesFallback(t *testing.T) {
	callCount := 0
	runner := &mockRunner{}
	// First call fails (list-clients), second succeeds (query-tab-names)
	origRun := runner.Run
	_ = origRun
	m := NewManagerWithRunner("cc", &fallbackRunner{callCount: &callCount})

	panes, err := m.ListPanes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}
}

type fallbackRunner struct {
	callCount *int
}

func (f *fallbackRunner) Run(name string, args ...string) ([]byte, error) {
	*f.callCount++
	cmd := strings.Join(args, " ")
	if strings.Contains(cmd, "list-clients") {
		return nil, fmt.Errorf("not supported")
	}
	if strings.Contains(cmd, "query-tab-names") {
		return []byte("main\nworker\n"), nil
	}
	return nil, fmt.Errorf("unknown command: %s", cmd)
}

func TestSendKeys(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("cc", runner)

	err := m.SendKeys("pane-1", "ls -la\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
}

func TestSendKeysError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("write failed")}
	m := NewManagerWithRunner("cc", runner)

	err := m.SendKeys("pane-1", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCapturePaneContent(t *testing.T) {
	runner := &mockRunner{
		output: []byte("line1\nline2\nline3\nline4\nline5\n"),
	}
	m := NewManagerWithRunner("cc", runner)

	content, err := m.CapturePaneContent("pane-1", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), content)
	}
}

func TestCapturePaneContentAll(t *testing.T) {
	runner := &mockRunner{
		output: []byte("line1\nline2\n"),
	}
	m := NewManagerWithRunner("cc", runner)

	content, err := m.CapturePaneContent("pane-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "line1\nline2\n" {
		t.Errorf("content = %q, want %q", content, "line1\nline2\n")
	}
}

func TestClosePane(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("cc", runner)

	err := m.ClosePane("pane-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}
}

func TestClosePaneError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("close failed")}
	m := NewManagerWithRunner("cc", runner)

	err := m.ClosePane("pane-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSpawnPane(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("cc", runner)

	id, err := m.SpawnPane("worker", "claude --task build", PaneOpts{
		WorkDir:  "/tmp/work",
		Floating: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "worker" {
		t.Errorf("id = %q, want %q", id, "worker")
	}
}

func TestAttachFloating(t *testing.T) {
	runner := &mockRunner{}
	m := NewManagerWithRunner("cc", runner)

	err := m.AttachFloating("pane-1", AttachOpts{Width: 80, Height: 24})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCapturePane(t *testing.T) {
	runner := &mockRunner{
		output: []byte("content here\n"),
	}
	m := NewManagerWithRunner("cc", runner)

	content, err := m.CapturePane("pane-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "content here\n" {
		t.Errorf("content = %q, want %q", content, "content here\n")
	}
}

func TestLastNLines(t *testing.T) {
	tt := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{
			name:  "fewer lines than requested",
			input: "a\nb",
			n:     5,
			want:  "a\nb",
		},
		{
			name:  "exact number",
			input: "a\nb\nc",
			n:     3,
			want:  "a\nb\nc",
		},
		{
			name:  "trim to last 2",
			input: "a\nb\nc\nd",
			n:     2,
			want:  "c\nd",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got := lastNLines(tc.input, tc.n)
			if got != tc.want {
				t.Errorf("lastNLines(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
			}
		})
	}
}

func TestParsePaneList(t *testing.T) {
	tt := []struct {
		name   string
		input  string
		wantN  int
	}{
		{"empty", "", 0},
		{"single", "tab-1", 1},
		{"multiple", "tab-1\ntab-2\ntab-3", 3},
		{"with blank lines", "tab-1\n\ntab-2\n", 2},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			panes := parsePaneList(tc.input)
			if len(panes) != tc.wantN {
				t.Errorf("parsePaneList returned %d panes, want %d", len(panes), tc.wantN)
			}
		})
	}
}

// Verify Manager satisfies PaneManager interface at compile time.
var _ PaneManager = (*Manager)(nil)
