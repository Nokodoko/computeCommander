package tui

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

// DirectorySession represents an agent session scoped to a specific directory.
type DirectorySession struct {
	ID             string    `json:"id"`
	Directory      string    `json:"directory"`
	DisplayName    string    `json:"displayName"`
	AgentSessionID string    `json:"agentSessionId,omitempty"`
	Runtime        string    `json:"runtime"`
	Active         bool      `json:"active"`
	LastAccessedAt time.Time `json:"lastAccessedAt"`
	CreatedAt      time.Time `json:"createdAt"`
	StoppedAt      *time.Time `json:"stoppedAt,omitempty"`
}

// SessionManager manages multiple directory-scoped agent sessions.
// It tracks which directories have active sessions and allows switching
// between them in the agent_session pane.
type SessionManager struct {
	sessions  map[string]*DirectorySession // directory path -> session
	activeDir string                       // currently focused directory
	mu        sync.RWMutex
	nextID    int
}

// NewSessionManager creates a new session manager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*DirectorySession),
	}
}

// CreateSession starts a new agent session for the given directory.
// If a session already exists for this directory, it returns that session.
func (sm *SessionManager) CreateSession(directory, runtime string) *DirectorySession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if session already exists.
	if sess, ok := sm.sessions[directory]; ok {
		sess.Active = true
		sess.LastAccessedAt = time.Now()
		sm.activeDir = directory
		return sess
	}

	sm.nextID++
	now := time.Now()
	sess := &DirectorySession{
		ID:             fmt.Sprintf("dsess-%08x", sm.nextID),
		Directory:      directory,
		DisplayName:    filepath.Base(directory),
		Runtime:        runtime,
		Active:         true,
		LastAccessedAt: now,
		CreatedAt:      now,
	}

	sm.sessions[directory] = sess
	sm.activeDir = directory
	return sess
}

// SwitchSession changes the active session to the one at the given directory.
// Returns the session, or nil if no session exists for that directory.
func (sm *SessionManager) SwitchSession(directory string) *DirectorySession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[directory]
	if !ok {
		return nil
	}

	// Deactivate current session.
	if current, ok := sm.sessions[sm.activeDir]; ok {
		current.Active = false
	}

	sess.Active = true
	sess.LastAccessedAt = time.Now()
	sm.activeDir = directory
	return sess
}

// StopSession stops the session for the given directory.
func (sm *SessionManager) StopSession(directory string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sess, ok := sm.sessions[directory]
	if !ok {
		return fmt.Errorf("no session for directory: %s", directory)
	}

	now := time.Now()
	sess.Active = false
	sess.StoppedAt = &now

	// If this was the active session, clear the active directory.
	if sm.activeDir == directory {
		sm.activeDir = ""
	}

	return nil
}

// ListSessions returns all sessions, optionally including stopped ones.
func (sm *SessionManager) ListSessions(includesStopped bool) []*DirectorySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []*DirectorySession
	for _, sess := range sm.sessions {
		if !includesStopped && sess.StoppedAt != nil {
			continue
		}
		result = append(result, sess)
	}
	return result
}

// GetSession returns the session for the given directory, or nil.
func (sm *SessionManager) GetSession(directory string) *DirectorySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[directory]
}

// ActiveSession returns the currently active session, or nil.
func (sm *SessionManager) ActiveSession() *DirectorySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.activeDir == "" {
		return nil
	}
	return sm.sessions[sm.activeDir]
}

// SessionCount returns the number of active (non-stopped) sessions.
func (sm *SessionManager) SessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	count := 0
	for _, sess := range sm.sessions {
		if sess.StoppedAt == nil {
			count++
		}
	}
	return count
}

// StopAll stops all active sessions.
func (sm *SessionManager) StopAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for _, sess := range sm.sessions {
		if sess.StoppedAt == nil {
			sess.Active = false
			sess.StoppedAt = &now
		}
	}
	sm.activeDir = ""
}
