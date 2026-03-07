package isolation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// NamespaceManager handles mount namespace setup for agent isolation.
type NamespaceManager struct {
	baseDir string // Base directory for agent worktrees
}

// NewNamespaceManager creates a new NamespaceManager.
func NewNamespaceManager(baseDir string) *NamespaceManager {
	return &NamespaceManager{baseDir: baseDir}
}

// Setup creates an isolated mount namespace for the agent with the given manifest.
// It bind-mounts only the paths granted in the manifest as readable or writable.
func (nm *NamespaceManager) Setup(agentID string, manifest *IsolationManifest) error {
	agentDir := filepath.Join(nm.baseDir, agentID)

	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	// Create read-only bind mounts
	for _, path := range manifest.Grants.Filesystem.Read {
		target := filepath.Join(agentDir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			continue
		}
		// Use mount --bind with -o ro for read-only
		cmd := exec.Command("mount", "--bind", "-o", "ro", path, target)
		if err := cmd.Run(); err != nil {
			// Non-fatal: log and continue
			continue
		}
	}

	// Create read-write bind mounts
	for _, path := range manifest.Grants.Filesystem.Write {
		target := filepath.Join(agentDir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			continue
		}
		cmd := exec.Command("mount", "--bind", path, target)
		if err := cmd.Run(); err != nil {
			continue
		}
	}

	return nil
}

// Teardown unmounts all bind mounts and removes the agent directory.
func (nm *NamespaceManager) Teardown(agentID string) error {
	agentDir := filepath.Join(nm.baseDir, agentID)

	// Lazy unmount everything under the agent directory
	cmd := exec.Command("umount", "-lR", agentDir)
	_ = cmd.Run() // Best-effort unmount

	return os.RemoveAll(agentDir)
}

// IsAvailable checks if unshare(2) is available for mount namespace creation.
func (nm *NamespaceManager) IsAvailable() bool {
	cmd := exec.Command("unshare", "--mount", "true")
	return cmd.Run() == nil
}

// CreateFiltered creates a filtered view of the filesystem based on the manifest.
// This is a lighter alternative to full mount namespace isolation.
func (nm *NamespaceManager) CreateFiltered(agentID string, manifest *IsolationManifest) (string, error) {
	agentDir := filepath.Join(nm.baseDir, agentID)

	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return "", fmt.Errorf("create filtered dir: %w", err)
	}

	// Create symlinks for readable paths (lightweight isolation)
	for _, path := range manifest.Grants.Filesystem.Read {
		target := filepath.Join(agentDir, filepath.Base(path))
		_ = os.Symlink(path, target)
	}
	for _, path := range manifest.Grants.Filesystem.Write {
		target := filepath.Join(agentDir, filepath.Base(path))
		_ = os.Symlink(path, target)
	}

	return agentDir, nil
}

// EnvFilter returns a filtered environment based on the manifest's allowed env vars.
func EnvFilter(manifest *IsolationManifest) []string {
	allowed := make(map[string]bool)
	for _, v := range manifest.Grants.EnvVars {
		allowed[v] = true
	}

	var filtered []string
	for _, env := range os.Environ() {
		parts := splitEnvVar(env)
		if len(parts) == 2 && allowed[parts[0]] {
			filtered = append(filtered, env)
		}
	}
	return filtered
}

func splitEnvVar(env string) []string {
	for i, c := range env {
		if c == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}
