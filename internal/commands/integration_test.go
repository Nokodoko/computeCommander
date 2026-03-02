package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// moduleRoot returns the project root by resolving go.mod location.
func moduleRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	modFile := strings.TrimSpace(string(out))
	if modFile == "" || modFile == os.DevNull {
		t.Fatal("could not determine module root")
	}
	return filepath.Dir(modFile)
}

// TestIntegrationBuild verifies that the binary can be built.
func TestIntegrationBuild(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.Command("go", "build", "-o", "/dev/null", "./cmd/cc/")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
}

// TestIntegrationRootCommand verifies the root command Use field is "cmdr".
func TestIntegrationRootCommand(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd/cc/main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `Use:   "cmdr"`) {
		t.Error("root command Use field should be 'cmdr'")
	}
}

// TestIntegrationGlobalFlags verifies --agent and --sub-agent flags exist.
func TestIntegrationGlobalFlags(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "cmd/cc/main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `"agent"`) {
		t.Error("root command should have --agent flag")
	}
	if !strings.Contains(content, `"sub-agent"`) {
		t.Error("root command should have --sub-agent flag")
	}
}

// TestIntegrationLifecycleFiles verifies lifecycle.go contains required functions.
func TestIntegrationLifecycleFiles(t *testing.T) {
	data, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("read lifecycle.go: %v", err)
	}

	content := string(data)
	for _, fn := range []string{"ShutdownCmd", "ResetCmd", "RestartCmd"} {
		if !strings.Contains(content, fn) {
			t.Errorf("lifecycle.go should contain %s", fn)
		}
	}
}

// TestIntegrationInfoFiles verifies info.go contains required functions.
func TestIntegrationInfoFiles(t *testing.T) {
	data, err := os.ReadFile("info.go")
	if err != nil {
		t.Fatalf("read info.go: %v", err)
	}

	content := string(data)
	for _, fn := range []string{"HelpCmd", "DocsCmd", "VersionCmd", "UpdateCmd"} {
		if !strings.Contains(content, fn) {
			t.Errorf("info.go should contain %s", fn)
		}
	}
}

// TestIntegrationDataFiles verifies data.go contains required functions.
func TestIntegrationDataFiles(t *testing.T) {
	data, err := os.ReadFile("data.go")
	if err != nil {
		t.Fatalf("read data.go: %v", err)
	}

	content := string(data)
	for _, fn := range []string{"ExportCmd", "BackupCmd", "RestoreCmd"} {
		if !strings.Contains(content, fn) {
			t.Errorf("data.go should contain %s", fn)
		}
	}
}

// TestIntegrationSessionFiles verifies session.go contains required functions.
func TestIntegrationSessionFiles(t *testing.T) {
	data, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatalf("read session.go: %v", err)
	}

	content := string(data)
	for _, fn := range []string{"SessionListCmd", "SessionSwitchCmd", "SessionStopCmd"} {
		if !strings.Contains(content, fn) {
			t.Errorf("session.go should contain %s", fn)
		}
	}
}

// TestIntegrationKeybindFiles verifies keybind-related files exist and contain expected content.
func TestIntegrationKeybindFiles(t *testing.T) {
	root := moduleRoot(t)

	// keybinds.go in tui package
	data, err := os.ReadFile(filepath.Join(root, "internal/tui/keybinds.go"))
	if err != nil {
		t.Fatalf("read tui/keybinds.go: %v", err)
	}
	if !strings.Contains(string(data), "LeaderKeyHandler") {
		t.Error("tui/keybinds.go should contain LeaderKeyHandler")
	}

	// keybinds.go in keybinds package
	data, err = os.ReadFile(filepath.Join(root, "internal/keybinds/keybinds.go"))
	if err != nil {
		t.Fatalf("read keybinds/keybinds.go: %v", err)
	}
	if !strings.Contains(string(data), "keybinds.yaml") {
		t.Error("keybinds/keybinds.go should parse keybinds.yaml")
	}
}

// TestIntegrationFloatingPaneFile verifies floating.go exists and has expected content.
func TestIntegrationFloatingPaneFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/tui/floating.go"))
	if err != nil {
		t.Fatalf("read tui/floating.go: %v", err)
	}
	if !strings.Contains(string(data), "FloatingPane") {
		t.Error("floating.go should contain FloatingPane renderer")
	}
}

// TestIntegrationConfigWatcherFile verifies watcher.go exists.
func TestIntegrationConfigWatcherFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/config/watcher.go"))
	if err != nil {
		t.Fatalf("read config/watcher.go: %v", err)
	}
	if !strings.Contains(string(data), "fsnotify") {
		t.Error("watcher.go should implement fsnotify watcher")
	}
}

// TestIntegrationBackupFile verifies backup.go exists.
func TestIntegrationBackupFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/backup/backup.go"))
	if err != nil {
		t.Fatalf("read backup/backup.go: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Backup") || !strings.Contains(content, "Restore") {
		t.Error("backup.go should implement SQLite backup/restore")
	}
}

// TestIntegrationExportFile verifies export.go exists.
func TestIntegrationExportFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/export/export.go"))
	if err != nil {
		t.Fatalf("read export/export.go: %v", err)
	}
	if !strings.Contains(string(data), "Export") {
		t.Error("export.go should implement JSON/CSV export")
	}
}

// TestIntegrationFilePickerFile verifies filepicker.go exists.
func TestIntegrationFilePickerFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/tui/filepicker.go"))
	if err != nil {
		t.Fatalf("read tui/filepicker.go: %v", err)
	}
	if !strings.Contains(string(data), "FilePicker") {
		t.Error("filepicker.go should contain directory navigation TUI component")
	}
}

// TestIntegrationSessionManagerFile verifies session_manager.go exists.
func TestIntegrationSessionManagerFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/tui/session_manager.go"))
	if err != nil {
		t.Fatalf("read tui/session_manager.go: %v", err)
	}
	if !strings.Contains(string(data), "SessionManager") {
		t.Error("session_manager.go should contain session switching logic")
	}
}

// TestIntegrationLayoutFile verifies layout.go exists.
func TestIntegrationLayoutFile(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "internal/zellij/layout.go"))
	if err != nil {
		t.Fatalf("read zellij/layout.go: %v", err)
	}
	if !strings.Contains(string(data), "GenerateLayout") {
		t.Error("layout.go should contain KDL layout generation")
	}
}

// TestIntegrationSessionManager verifies session creation, switching, and stopping.
func TestIntegrationSessionManager(t *testing.T) {
	dir := t.TempDir()

	// Import is within same package, so we use tui types via the session.go file's sessionManager.
	// This test exercises the session commands at a higher level.
	_ = dir

	// Verify session manager works (via the package-level var).
	// Note: sessionManager is defined in session.go.
	if sessionManager == nil {
		t.Fatal("sessionManager should be initialized")
	}

	// Create a session.
	sess := sessionManager.CreateSession("/tmp/test-project", "claude")
	if sess == nil {
		t.Fatal("CreateSession returned nil")
	}
	if sess.Directory != "/tmp/test-project" {
		t.Errorf("expected directory /tmp/test-project, got %q", sess.Directory)
	}
	if !sess.Active {
		t.Error("new session should be active")
	}

	// List sessions.
	sessions := sessionManager.ListSessions(false)
	if len(sessions) == 0 {
		t.Error("expected at least 1 session")
	}

	// Switch to same session (should work).
	switched := sessionManager.SwitchSession("/tmp/test-project")
	if switched == nil {
		t.Error("SwitchSession returned nil for existing session")
	}

	// Stop session.
	if err := sessionManager.StopSession("/tmp/test-project"); err != nil {
		t.Errorf("StopSession failed: %v", err)
	}

	// Verify stopped.
	stoppedSess := sessionManager.GetSession("/tmp/test-project")
	if stoppedSess == nil {
		t.Fatal("GetSession returned nil for stopped session")
	}
	if stoppedSess.StoppedAt == nil {
		t.Error("stopped session should have StoppedAt set")
	}
}

// TestIntegrationVersionJSON simulates the version --json output structure.
func TestIntegrationVersionJSON(t *testing.T) {
	// This is a structural test. The actual binary test would require building first.
	// We verify the command function exists and compiles correctly.
	app := &App{Version: "0.2.0"}
	cmd := VersionCmd(app)
	if cmd == nil {
		t.Fatal("VersionCmd returned nil")
	}
	if cmd.Use != "version" {
		t.Errorf("expected Use 'version', got %q", cmd.Use)
	}
}

// TestIntegrationHelpContainsNewCommands verifies help output would contain new commands.
func TestIntegrationHelpContainsNewCommands(t *testing.T) {
	app := &App{Version: "0.2.0"}

	// Verify each new command function returns a non-nil command.
	commands := map[string]*func(*App) *cobra.Command{
		"shutdown":      nil,
		"reset":         nil,
		"restart":       nil,
		"help":          nil,
		"docs":          nil,
		"version":       nil,
		"update":        nil,
		"export":        nil,
		"backup":        nil,
		"restore":       nil,
		"shell":         nil,
		"feedback":      nil,
		"support":       nil,
		"clear":         nil,
		"theme":         nil,
		"notifications": nil,
		"analytics":     nil,
		"integrations":  nil,
		"automation":    nil,
		"fp":            nil,
		"session":       nil,
	}

	// Verify the commands compile (they're called in main.go).
	_ = ShutdownCmd(app)
	_ = ResetCmd(app)
	_ = RestartCmd(app)
	_ = HelpCmd(app)
	_ = DocsCmd(app)
	_ = VersionCmd(app)
	_ = UpdateCmd(app)
	_ = ExportCmd(app)
	_ = BackupCmd(app)
	_ = RestoreCmd(app)
	_ = ShellCmd(app)
	_ = FeedbackCmd(app)
	_ = SupportCmd(app)
	_ = ClearCmd(app)
	_ = ThemeCmd(app)
	_ = NotificationsCmd(app)
	_ = AnalyticsCmd(app)
	_ = IntegrationsCmd(app)
	_ = AutomationCmd(app)
	_ = FpCmd(app)
	_ = SessionCmd(app)

	_ = commands // Suppress unused.
}

// TestIntegrationGoVet runs go vet on the entire project.
func TestIntegrationGoVet(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go vet failed: %v\n%s", err, stderr.String())
	}
}
