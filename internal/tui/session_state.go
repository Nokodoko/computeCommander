package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionState represents the persisted state of all sessions.
type SessionState struct {
	Version   int                 `json:"version"`
	SavedAt   time.Time           `json:"savedAt"`
	PID       int                 `json:"pid"`
	ActiveDir string              `json:"activeDir"`
	Sessions  []*DirectorySession `json:"sessions"`
}

// SaveState writes the current session state to the given path.
// It uses a temp file + rename pattern to avoid partial writes.
func (sm *SessionManager) SaveState(path string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var sessions []*DirectorySession
	for _, sess := range sm.sessions {
		sessions = append(sessions, sess)
	}

	state := &SessionState{
		Version:   1,
		SavedAt:   time.Now(),
		PID:       os.Getpid(),
		ActiveDir: sm.activeDir,
		Sessions:  sessions,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}

	// Write to temp file first, then rename for atomicity.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "session-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// LoadState reads session state from the given path.
func LoadState(path string) (*SessionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session state: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse session state: %w", err)
	}

	if state.Version != 1 {
		return nil, fmt.Errorf("unsupported session state version: %d", state.Version)
	}

	return &state, nil
}

// IsStale returns true if the state was saved more than maxAge ago.
func (s *SessionState) IsStale(maxAge time.Duration) bool {
	return time.Since(s.SavedAt) > maxAge
}

// IsPIDAlive checks if the process that saved the state is still running.
func (s *SessionState) IsPIDAlive() bool {
	if s.PID <= 0 {
		return false
	}
	// Check /proc/<pid>/ existence (Linux-specific).
	_, err := os.Stat(fmt.Sprintf("/proc/%d", s.PID))
	return err == nil
}

// RestoreState populates the session manager from a previously saved state.
// Unlike CreateSession, this preserves original session IDs.
func (sm *SessionManager) RestoreState(state *SessionState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, sess := range state.Sessions {
		if sess.Directory == "" {
			continue
		}
		// Preserve the original session, including its ID.
		sm.sessions[sess.Directory] = sess
	}

	if state.ActiveDir != "" {
		sm.activeDir = state.ActiveDir
	}

	return nil
}

// StartAutosave begins periodic state saving. Returns a stop function.
// The autosave goroutine writes state every interval while sessions exist.
func (sm *SessionManager) StartAutosave(path string, interval time.Duration) func() {
	var once sync.Once
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				sm.mu.RLock()
				count := len(sm.sessions)
				sm.mu.RUnlock()
				if count > 0 {
					_ = sm.SaveState(path)
				}
			}
		}
	}()

	return func() {
		once.Do(func() { close(done) })
	}
}
