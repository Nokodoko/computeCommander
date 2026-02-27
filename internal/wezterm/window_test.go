package wezterm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const succeed = "\u2713"
const failed = "\u2717"

// mockRunner implements CommandRunner for testing.
type mockRunner struct {
	runOutput []byte
	runErr    error
	startErr  error
	calls     [][]string
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	return m.runOutput, m.runErr
}

func (m *mockRunner) Start(ctx context.Context, name string, args ...string) error {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	return m.startErr
}

func TestSpawnWindow(t *testing.T) {
	tt := []struct {
		name      string
		opts      SpawnWindowOpts
		startErr  error
		wantErr   bool
		wantClass string
	}{
		{
			name: "basic spawn",
			opts: SpawnWindowOpts{
				ZellijSession: "test-session",
				WorkDir:       "/tmp/workdir",
			},
			wantErr:   false,
			wantClass: "cc-test-session",
		},
		{
			name: "spawn with layout",
			opts: SpawnWindowOpts{
				ZellijSession: "build-agent",
				WorkDir:       "/home/user/project",
				Layout:        "compact-top",
			},
			wantErr:   false,
			wantClass: "cc-build-agent",
		},
		{
			name: "missing session name",
			opts: SpawnWindowOpts{
				WorkDir: "/tmp",
			},
			wantErr: true,
		},
		{
			name: "start fails",
			opts: SpawnWindowOpts{
				ZellijSession: "fail-session",
			},
			startErr: errors.New("wezterm not found"),
			wantErr:  true,
		},
	}

	t.Log("Given the need to test spawning Wezterm windows.")
	{
		for testID, test := range tt {
			t.Logf("\tTest %d:\tWhen spawning window with session %q", testID, test.opts.ZellijSession)
			{
				runner := &mockRunner{startErr: test.startErr}
				mgr := NewManagerWithRunner("cc", runner)

				err := mgr.SpawnWindow(context.Background(), test.opts)

				if test.wantErr {
					if err == nil {
						t.Errorf("\t%s\tTest %d:\tShould have received an error.", failed, testID)
					} else {
						t.Logf("\t%s\tTest %d:\tShould have received an error: %v", succeed, testID, err)
					}
					continue
				}

				if err != nil {
					t.Fatalf("\t%s\tTest %d:\tShould be able to spawn window: %v", failed, testID, err)
				}
				t.Logf("\t%s\tTest %d:\tShould be able to spawn window.", succeed, testID)

				// Verify wezterm was called with correct args
				if len(runner.calls) == 0 {
					t.Fatalf("\t%s\tTest %d:\tShould have called wezterm.", failed, testID)
				}

				call := runner.calls[0]
				if call[0] != "wezterm" {
					t.Errorf("\t%s\tTest %d:\tShould call wezterm: got %s", failed, testID, call[0])
				} else {
					t.Logf("\t%s\tTest %d:\tShould call wezterm.", succeed, testID)
				}

				// Check --class argument
				foundClass := false
				for i, arg := range call {
					if arg == "--class" && i+1 < len(call) {
						if call[i+1] == test.wantClass {
							foundClass = true
							t.Logf("\t%s\tTest %d:\tShould set WM_CLASS to %s.", succeed, testID, test.wantClass)
						}
					}
				}
				if !foundClass && test.wantClass != "" {
					t.Errorf("\t%s\tTest %d:\tShould set WM_CLASS to %s.", failed, testID, test.wantClass)
				}
			}
		}
	}
}

func TestListWindows(t *testing.T) {
	wmctrlOutput := `0x01600003  0 wezterm.cc-build-1  monty Build Agent 1
0x01600004  0 wezterm.cc-scout-2  monty Scout Agent 2
0x01600005  0 firefox.Navigator  monty Mozilla Firefox
0x01600006  0 wezterm.other-session  monty Other Session`

	tt := []struct {
		name       string
		output     string
		runErr     error
		wantCount  int
		wantNames  []string
		wantErr    bool
	}{
		{
			name:      "parse multiple windows",
			output:    wmctrlOutput,
			wantCount: 2,
			wantNames: []string{"build-1", "scout-2"},
			wantErr:   false,
		},
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "wmctrl fails",
			output:  "",
			runErr:  errors.New("wmctrl not found"),
			wantErr: true,
		},
	}

	t.Log("Given the need to test listing Wezterm windows.")
	{
		for testID, test := range tt {
			t.Logf("\tTest %d:\tWhen listing windows", testID)
			{
				runner := &mockRunner{
					runOutput: []byte(test.output),
					runErr:    test.runErr,
				}
				mgr := NewManagerWithRunner("cc", runner)

				windows, err := mgr.ListWindows()

				if test.wantErr {
					if err == nil {
						t.Errorf("\t%s\tTest %d:\tShould have received an error.", failed, testID)
					} else {
						t.Logf("\t%s\tTest %d:\tShould have received an error: %v", succeed, testID, err)
					}
					continue
				}

				if err != nil {
					t.Fatalf("\t%s\tTest %d:\tShould be able to list windows: %v", failed, testID, err)
				}
				t.Logf("\t%s\tTest %d:\tShould be able to list windows.", succeed, testID)

				if len(windows) != test.wantCount {
					t.Errorf("\t%s\tTest %d:\tShould return %d windows: got %d", failed, testID, test.wantCount, len(windows))
				} else {
					t.Logf("\t%s\tTest %d:\tShould return %d windows.", succeed, testID, test.wantCount)
				}

				for i, wantName := range test.wantNames {
					if i < len(windows) && windows[i].ZellijSession == wantName {
						t.Logf("\t%s\tTest %d:\tShould have session %q.", succeed, testID, wantName)
					} else {
						t.Errorf("\t%s\tTest %d:\tShould have session %q.", failed, testID, wantName)
					}
				}
			}
		}
	}
}

func TestFocusWindow(t *testing.T) {
	tt := []struct {
		name        string
		sessionName string
		runErr      error
		wantErr     bool
	}{
		{
			name:        "focus existing window",
			sessionName: "build-agent",
			wantErr:     false,
		},
		{
			name:        "focus missing window",
			sessionName: "nonexistent",
			runErr:      errors.New("window not found"),
			wantErr:     true,
		},
	}

	t.Log("Given the need to test focusing Wezterm windows.")
	{
		for testID, test := range tt {
			t.Logf("\tTest %d:\tWhen focusing window %q", testID, test.sessionName)
			{
				runner := &mockRunner{runErr: test.runErr}
				mgr := NewManagerWithRunner("cc", runner)

				err := mgr.FocusWindow(test.sessionName)

				if test.wantErr {
					if err == nil {
						t.Errorf("\t%s\tTest %d:\tShould have received an error.", failed, testID)
					} else {
						t.Logf("\t%s\tTest %d:\tShould have received an error: %v", succeed, testID, err)
					}
					continue
				}

				if err != nil {
					t.Fatalf("\t%s\tTest %d:\tShould be able to focus window: %v", failed, testID, err)
				}
				t.Logf("\t%s\tTest %d:\tShould be able to focus window.", succeed, testID)

				// Verify wmctrl was called
				if len(runner.calls) == 0 {
					t.Fatalf("\t%s\tTest %d:\tShould have called wmctrl.", failed, testID)
				}

				call := runner.calls[0]
				if call[0] != "wmctrl" {
					t.Errorf("\t%s\tTest %d:\tShould call wmctrl: got %s", failed, testID, call[0])
				} else {
					t.Logf("\t%s\tTest %d:\tShould call wmctrl.", succeed, testID)
				}
			}
		}
	}
}

func TestSpawnWindowZellijCommand(t *testing.T) {
	t.Log("Given the need to verify the zellij command passed to wezterm.")
	{
		t.Logf("\tTest 0:\tWhen spawning with a layout, the shell command must use --new-session-with-layout and unset zellij env vars")
		{
			runner := &mockRunner{}
			mgr := NewManagerWithRunner("cc", runner)

			err := mgr.SpawnWindow(context.Background(), SpawnWindowOpts{
				ZellijSession: "dashboard",
				Layout:        "/path/to/layout.kdl",
			})
			if err != nil {
				t.Fatalf("\t%s\tTest 0:\tShould be able to spawn window: %v", failed, err)
			}

			if len(runner.calls) == 0 {
				t.Fatalf("\t%s\tTest 0:\tShould have called wezterm.", failed)
			}

			// The last arg to wezterm start -- sh -c "<shellCmd>" is the shell command.
			call := runner.calls[0]
			shellCmd := call[len(call)-1]

			if !strings.Contains(shellCmd, "unset ZELLIJ ZELLIJ_SESSION_NAME ZELLIJ_PANE_ID") {
				t.Errorf("\t%s\tTest 0:\tShell command should unset zellij env vars: got %q", failed, shellCmd)
			} else {
				t.Logf("\t%s\tTest 0:\tShell command should unset zellij env vars.", succeed)
			}

			if !strings.Contains(shellCmd, "--new-session-with-layout") {
				t.Errorf("\t%s\tTest 0:\tShell command should use --new-session-with-layout: got %q", failed, shellCmd)
			} else {
				t.Logf("\t%s\tTest 0:\tShell command should use --new-session-with-layout.", succeed)
			}

			if strings.Contains(shellCmd, " --layout ") {
				t.Errorf("\t%s\tTest 0:\tShell command should NOT use --layout (use --new-session-with-layout instead): got %q", failed, shellCmd)
			} else {
				t.Logf("\t%s\tTest 0:\tShell command should NOT use bare --layout flag.", succeed)
			}
		}
	}
}

func TestWindowError(t *testing.T) {
	t.Log("Given the need to test WindowError behavior.")
	{
		innerErr := errors.New("underlying error")
		we := &WindowError{Op: "spawn", Name: "test-session", Err: innerErr}

		t.Logf("\tTest 0:\tWhen checking error string")
		{
			got := we.Error()
			want := `wezterm spawn "test-session": underlying error`
			if got == want {
				t.Logf("\t%s\tTest 0:\tShould format error correctly.", succeed)
			} else {
				t.Errorf("\t%s\tTest 0:\tShould format error correctly: got %q, want %q", failed, got, want)
			}
		}

		t.Logf("\tTest 1:\tWhen unwrapping error")
		{
			if errors.Unwrap(we) == innerErr {
				t.Logf("\t%s\tTest 1:\tShould unwrap to inner error.", succeed)
			} else {
				t.Errorf("\t%s\tTest 1:\tShould unwrap to inner error.", failed)
			}
		}
	}
}
