package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	sm := NewSessionManager()
	sm.CreateSession("/home/user/project-a", "claude")
	sm.CreateSession("/home/user/project-b", "codex")

	dir := t.TempDir()
	path := filepath.Join(dir, "session-state.json")

	if err := sm.SaveState(path); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if state.Version != 1 {
		t.Errorf("version = %d, want 1", state.Version)
	}
	if len(state.Sessions) != 2 {
		t.Errorf("sessions count = %d, want 2", len(state.Sessions))
	}
	if state.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", state.PID, os.Getpid())
	}

	// Verify session data round-trips correctly.
	found := map[string]bool{}
	for _, sess := range state.Sessions {
		found[sess.Directory] = true
	}
	if !found["/home/user/project-a"] || !found["/home/user/project-b"] {
		t.Errorf("missing sessions in loaded state: %v", found)
	}
}

func TestRestoreState(t *testing.T) {
	// Create state with known session IDs.
	now := time.Now()
	state := &SessionState{
		Version:   1,
		SavedAt:   now,
		PID:       12345,
		ActiveDir: "/home/user/project-a",
		Sessions: []*DirectorySession{
			{
				ID:             "dsess-original-1",
				Directory:      "/home/user/project-a",
				DisplayName:    "project-a",
				Runtime:        "claude",
				Active:         true,
				LastAccessedAt: now,
				CreatedAt:      now,
			},
			{
				ID:             "dsess-original-2",
				Directory:      "/home/user/project-b",
				DisplayName:    "project-b",
				Runtime:        "codex",
				Active:         false,
				LastAccessedAt: now,
				CreatedAt:      now,
			},
		},
	}

	sm := NewSessionManager()
	if err := sm.RestoreState(state); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	// Verify sessions are restored with original IDs.
	sessA := sm.GetSession("/home/user/project-a")
	if sessA == nil {
		t.Fatal("session A not restored")
	}
	if sessA.ID != "dsess-original-1" {
		t.Errorf("session A ID = %q, want %q", sessA.ID, "dsess-original-1")
	}

	sessB := sm.GetSession("/home/user/project-b")
	if sessB == nil {
		t.Fatal("session B not restored")
	}
	if sessB.ID != "dsess-original-2" {
		t.Errorf("session B ID = %q, want %q", sessB.ID, "dsess-original-2")
	}

	// Verify active directory was restored.
	active := sm.ActiveSession()
	if active == nil {
		t.Fatal("no active session after restore")
	}
	if active.Directory != "/home/user/project-a" {
		t.Errorf("active dir = %q, want %q", active.Directory, "/home/user/project-a")
	}
}

func TestRestoreStateSkipsEmptyDirectory(t *testing.T) {
	state := &SessionState{
		Version: 1,
		SavedAt: time.Now(),
		Sessions: []*DirectorySession{
			{ID: "dsess-1", Directory: "", Runtime: "claude"},
			{ID: "dsess-2", Directory: "/valid/path", Runtime: "claude"},
		},
	}

	sm := NewSessionManager()
	if err := sm.RestoreState(state); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if sm.SessionCount() != 1 {
		t.Errorf("session count = %d, want 1 (empty dir should be skipped)", sm.SessionCount())
	}
}

func TestStalenessCheck(t *testing.T) {
	state := &SessionState{
		Version: 1,
		SavedAt: time.Now().Add(-25 * time.Hour),
	}

	if !state.IsStale(24 * time.Hour) {
		t.Error("expected state to be stale (25h old, threshold 24h)")
	}

	fresh := &SessionState{
		Version: 1,
		SavedAt: time.Now().Add(-1 * time.Hour),
	}

	if fresh.IsStale(24 * time.Hour) {
		t.Error("expected state to not be stale (1h old, threshold 24h)")
	}
}

func TestCorruptStateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-state.json")

	// Write corrupt data.
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

func TestLoadStateMissingFile(t *testing.T) {
	_, err := LoadState("/nonexistent/path/session-state.json")
	if err == nil {
		t.Fatal("expected error for missing state file")
	}
}

func TestLoadStateUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session-state.json")

	if err := os.WriteFile(path, []byte(`{"version": 99}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestAutosave(t *testing.T) {
	sm := NewSessionManager()
	sm.CreateSession("/home/user/project", "claude")

	dir := t.TempDir()
	path := filepath.Join(dir, "session-state.json")

	stop := sm.StartAutosave(path, 50*time.Millisecond)
	defer stop()

	// Wait for at least one autosave cycle.
	time.Sleep(150 * time.Millisecond)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("autosave did not create state file")
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState after autosave: %v", err)
	}
	if len(state.Sessions) != 1 {
		t.Errorf("autosaved sessions = %d, want 1", len(state.Sessions))
	}
}

func TestSaveStateAtomicity(t *testing.T) {
	sm := NewSessionManager()
	sm.CreateSession("/home/user/project", "claude")

	dir := t.TempDir()
	path := filepath.Join(dir, "session-state.json")

	// Save twice -- second should overwrite cleanly.
	if err := sm.SaveState(path); err != nil {
		t.Fatalf("first SaveState: %v", err)
	}
	if err := sm.SaveState(path); err != nil {
		t.Fatalf("second SaveState: %v", err)
	}

	state, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(state.Sessions) != 1 {
		t.Errorf("sessions = %d, want 1", len(state.Sessions))
	}
}
