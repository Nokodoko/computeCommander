// Package isolation provides environment isolation for agent execution
// including filesystem access control, resource limits via cgroups v2,
// and mount namespace management.
package isolation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// IsolationManifest defines the resource grants and constraints for an agent.
type IsolationManifest struct {
	AgentID    string         `json:"agent_id"`
	AgentName  string         `json:"agent_name"`
	Capability string         `json:"capability"`
	Grants     ResourceGrants `json:"grants"`
	CreatedAt  time.Time      `json:"created"`
	ExpiresAt  time.Time      `json:"expires"`
}

// ResourceGrants defines all resource access grants for an agent.
type ResourceGrants struct {
	Filesystem     FilesystemGrants `json:"filesystem"`
	EnvVars        []string         `json:"env_vars"`
	Network        NetworkGrants    `json:"network"`
	Resources      ResourceLimits   `json:"resources"`
	BlockOverrides []string         `json:"block_overrides,omitempty"`
}

// FilesystemGrants defines readable and writable path patterns.
type FilesystemGrants struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

// NetworkGrants defines network access rules.
type NetworkGrants struct {
	Allow   []string `json:"allow"`
	DenyAll bool     `json:"deny_all"`
}

// ResourceLimits defines cgroup resource constraints.
type ResourceLimits struct {
	CPUShares    int `json:"cpu_shares"`
	MemoryMB     int `json:"memory_mb"`
	DiskMB       int `json:"disk_mb"`
	MaxProcesses int `json:"max_processes"`
}

// ManifestStore manages isolation manifest CRUD operations.
type ManifestStore struct {
	db db.DB
}

// NewManifestStore creates a new ManifestStore.
func NewManifestStore(database db.DB) *ManifestStore {
	return &ManifestStore{db: database}
}

// Create persists a new isolation manifest.
func (s *ManifestStore) Create(ctx context.Context, m *IsolationManifest) error {
	grantsJSON, err := json.Marshal(m.Grants)
	if err != nil {
		return fmt.Errorf("marshal grants: %w", err)
	}

	query := `INSERT INTO isolation_manifests (agent_id, agent_name, capability, grants, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`

	return s.db.Exec(ctx, query,
		m.AgentID, m.AgentName, m.Capability, string(grantsJSON),
		m.CreatedAt.Format(time.RFC3339), m.ExpiresAt.Format(time.RFC3339),
	)
}

// Get retrieves an isolation manifest by agent ID.
func (s *ManifestStore) Get(ctx context.Context, agentID string) (*IsolationManifest, error) {
	query := `SELECT agent_id, agent_name, capability, grants, created_at, expires_at
		FROM isolation_manifests WHERE agent_id = ?`

	row := s.db.QueryRow(ctx, query, agentID)

	var m IsolationManifest
	var grantsJSON, createdAt, expiresAt string

	if err := row.Scan(
		&m.AgentID, &m.AgentName, &m.Capability, &grantsJSON, &createdAt, &expiresAt,
	); err != nil {
		return nil, fmt.Errorf("get manifest for %s: %w", agentID, err)
	}

	if err := json.Unmarshal([]byte(grantsJSON), &m.Grants); err != nil {
		return nil, fmt.Errorf("unmarshal grants: %w", err)
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	m.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)

	return &m, nil
}

// Delete removes an isolation manifest.
func (s *ManifestStore) Delete(ctx context.Context, agentID string) error {
	return s.db.Exec(ctx, "DELETE FROM isolation_manifests WHERE agent_id = ?", agentID)
}

// List retrieves all active isolation manifests, optionally filtered by capability.
func (s *ManifestStore) List(ctx context.Context, capability string) ([]*IsolationManifest, error) {
	query := "SELECT agent_id, agent_name, capability, grants, created_at, expires_at FROM isolation_manifests"
	var args []any
	if capability != "" {
		query += " WHERE capability = ?"
		args = append(args, capability)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}
	defer rows.Close()

	var manifests []*IsolationManifest
	for rows.Next() {
		var m IsolationManifest
		var grantsJSON, createdAt, expiresAt string
		if err := rows.Scan(
			&m.AgentID, &m.AgentName, &m.Capability, &grantsJSON, &createdAt, &expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan manifest: %w", err)
		}
		_ = json.Unmarshal([]byte(grantsJSON), &m.Grants)
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		m.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
		manifests = append(manifests, &m)
	}

	return manifests, rows.Err()
}

// AddGrant adds a resource grant to an existing manifest.
func (s *ManifestStore) AddGrant(ctx context.Context, agentID string, grantType string, value json.RawMessage) error {
	m, err := s.Get(ctx, agentID)
	if err != nil {
		return err
	}

	switch grantType {
	case "filesystem":
		var fg FilesystemGrants
		if err := json.Unmarshal(value, &fg); err != nil {
			return fmt.Errorf("unmarshal filesystem grant: %w", err)
		}
		m.Grants.Filesystem.Read = append(m.Grants.Filesystem.Read, fg.Read...)
		m.Grants.Filesystem.Write = append(m.Grants.Filesystem.Write, fg.Write...)
	case "env":
		var vars []string
		if err := json.Unmarshal(value, &vars); err != nil {
			return fmt.Errorf("unmarshal env grant: %w", err)
		}
		m.Grants.EnvVars = append(m.Grants.EnvVars, vars...)
	case "network":
		var ng NetworkGrants
		if err := json.Unmarshal(value, &ng); err != nil {
			return fmt.Errorf("unmarshal network grant: %w", err)
		}
		m.Grants.Network.Allow = append(m.Grants.Network.Allow, ng.Allow...)
	case "resource":
		var rl ResourceLimits
		if err := json.Unmarshal(value, &rl); err != nil {
			return fmt.Errorf("unmarshal resource grant: %w", err)
		}
		if rl.CPUShares > 0 {
			m.Grants.Resources.CPUShares = rl.CPUShares
		}
		if rl.MemoryMB > 0 {
			m.Grants.Resources.MemoryMB = rl.MemoryMB
		}
		if rl.DiskMB > 0 {
			m.Grants.Resources.DiskMB = rl.DiskMB
		}
		if rl.MaxProcesses > 0 {
			m.Grants.Resources.MaxProcesses = rl.MaxProcesses
		}
	case "block_override":
		var ruleIDs []string
		if err := json.Unmarshal(value, &ruleIDs); err != nil {
			return fmt.Errorf("unmarshal block override grant: %w", err)
		}
		m.Grants.BlockOverrides = append(m.Grants.BlockOverrides, ruleIDs...)
	default:
		return fmt.Errorf("unknown grant type: %s", grantType)
	}

	// Update in DB
	grantsJSON, _ := json.Marshal(m.Grants)
	return s.db.Exec(ctx, "UPDATE isolation_manifests SET grants = ? WHERE agent_id = ?",
		string(grantsJSON), agentID)
}

// IsExpired checks if the manifest has expired.
func (m *IsolationManifest) IsExpired() bool {
	return time.Now().UTC().After(m.ExpiresAt)
}

// HasReadAccess checks if the manifest grants read access to a path.
func (m *IsolationManifest) HasReadAccess(path string) bool {
	return matchesAnyPattern(path, m.Grants.Filesystem.Read)
}

// HasWriteAccess checks if the manifest grants write access to a path.
func (m *IsolationManifest) HasWriteAccess(path string) bool {
	return matchesAnyPattern(path, m.Grants.Filesystem.Write)
}

// GetBlockOverrides returns the list of block rule IDs that this manifest's grants override.
// Uses the explicit BlockOverrides field rather than filesystem paths.
func (m *IsolationManifest) GetBlockOverrides() []string {
	if len(m.Grants.BlockOverrides) == 0 {
		return nil
	}
	result := make([]string, len(m.Grants.BlockOverrides))
	copy(result, m.Grants.BlockOverrides)
	return result
}

// DefaultResourceLimits returns the default resource limits.
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		CPUShares:    512,
		MemoryMB:     2048,
		DiskMB:       1024,
		MaxProcesses: 50,
	}
}

// GenerateManifestID creates a random ID for manifest references.
func GenerateManifestID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "iso-" + hex.EncodeToString(b)
}

// matchesAnyPattern checks if a path matches any of the given glob-like patterns.
// This is a simple implementation; production would use filepath.Match.
func matchesAnyPattern(path string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := matchPattern(p, path); matched {
			return true
		}
	}
	return false
}

// matchPattern provides simple glob matching (* matches any sequence).
func matchPattern(pattern, path string) (bool, error) {
	// Simple implementation: exact match or wildcard suffix
	if pattern == path {
		return true, nil
	}
	if pattern == "*" {
		return true, nil
	}
	// Check if pattern ends with /*
	if len(pattern) > 2 && pattern[len(pattern)-2:] == "/*" {
		prefix := pattern[:len(pattern)-2]
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true, nil
		}
	}
	// Check if pattern ends with *
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true, nil
		}
	}
	return false, nil
}
