package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// DirectorySession represents an agent session scoped to a specific directory.
type DirectorySession struct {
	ID             string     `json:"id"`
	Directory      string     `json:"directory"`
	DisplayName    string     `json:"displayName"`
	ProjectID      string     `json:"projectId,omitempty"`
	AgentSessionID string     `json:"agentSessionId,omitempty"`
	Runtime        string     `json:"runtime"`
	Active         bool       `json:"active"`
	LastAccessedAt time.Time  `json:"lastAccessedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	StoppedAt      *time.Time `json:"stoppedAt,omitempty"`
}

// SessionManager manages multiple directory-scoped agent sessions.
// It can operate in two modes:
//   - DB-backed: queries the projects and sessions tables.
//   - In-memory: uses a local map (fallback when no DB is available).
type SessionManager struct {
	db        db.DB                        // nil = in-memory mode
	sessions  map[string]*DirectorySession // directory path -> session (in-memory fallback)
	activeDir string                       // currently focused directory
	mu        sync.RWMutex
	nextID    int
}

// NewSessionManager creates an in-memory session manager (backward compatible).
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*DirectorySession),
	}
}

// NewDBSessionManager creates a DB-backed session manager that queries
// projects and sessions tables for persistent session state.
func NewDBSessionManager(database db.DB) *SessionManager {
	return &SessionManager{
		db:       database,
		sessions: make(map[string]*DirectorySession),
	}
}

// IsDBBacked returns true if this session manager is backed by a database.
func (sm *SessionManager) IsDBBacked() bool {
	return sm.db != nil
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
// When DB-backed, merges registered projects with in-memory session state.
func (sm *SessionManager) ListSessions(includeStopped bool) []*DirectorySession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// If DB-backed, merge project data with in-memory state.
	if sm.db != nil {
		return sm.listSessionsFromDB(includeStopped)
	}

	var result []*DirectorySession
	for _, sess := range sm.sessions {
		if !includeStopped && sess.StoppedAt != nil {
			continue
		}
		result = append(result, sess)
	}
	return result
}

// listSessionsFromDB queries registered projects and enriches them with
// active session counts from the sessions table.
func (sm *SessionManager) listSessionsFromDB(includeStopped bool) []*DirectorySession {
	ctx := context.Background()

	rows, err := sm.db.Query(ctx,
		`SELECT p.id, p.name, p.path, p.active, p.registered_at, p.last_accessed_at,
			(SELECT COUNT(*) FROM sessions s WHERE s.project_id = p.id AND s.state NOT IN ('completed', 'zombie')) AS active_agents
		FROM projects p
		ORDER BY p.last_accessed_at DESC`)
	if err != nil {
		// Fall back to in-memory if DB query fails.
		var result []*DirectorySession
		for _, sess := range sm.sessions {
			if !includeStopped && sess.StoppedAt != nil {
				continue
			}
			result = append(result, sess)
		}
		return result
	}
	defer rows.Close()

	var result []*DirectorySession
	for rows.Next() {
		var projID, name, path, registeredAt, lastAccessedAt string
		var active int
		var activeAgents int
		if err := rows.Scan(&projID, &name, &path, &active, &registeredAt, &lastAccessedAt, &activeAgents); err != nil {
			continue
		}

		// Parse timestamps.
		regTime, _ := time.Parse(time.RFC3339, registeredAt)
		accessTime, _ := time.Parse(time.RFC3339, lastAccessedAt)

		sess := &DirectorySession{
			ID:             projID,
			Directory:      path,
			DisplayName:    name,
			ProjectID:      projID,
			Runtime:        "",
			Active:         activeAgents > 0,
			LastAccessedAt: accessTime,
			CreatedAt:      regTime,
		}

		// Overlay in-memory state if present.
		if memSess, ok := sm.sessions[path]; ok {
			if memSess.Runtime != "" {
				sess.Runtime = memSess.Runtime
			}
			if memSess.AgentSessionID != "" {
				sess.AgentSessionID = memSess.AgentSessionID
			}
			if memSess.StoppedAt != nil {
				sess.StoppedAt = memSess.StoppedAt
			}
		}

		if !includeStopped && sess.StoppedAt != nil {
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

	if sm.db != nil {
		ctx := context.Background()
		var count int
		row := sm.db.QueryRow(ctx,
			`SELECT COUNT(*) FROM projects WHERE active = 1`)
		if err := row.Scan(&count); err == nil {
			return count
		}
	}

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
