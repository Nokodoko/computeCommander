// Package worktree manages git worktree lifecycle for agent isolation.
// Each agent operates in its own git worktree to prevent file conflicts.
package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeState represents the lifecycle state of a worktree.
type WorktreeState string

const (
	WorktreeActive    WorktreeState = "active"
	WorktreeCompleted WorktreeState = "completed"
	WorktreeMerged    WorktreeState = "merged"
	WorktreeOrphaned  WorktreeState = "orphaned"
)

// Worktree represents a git worktree managed by ComputeCommander.
type Worktree struct {
	Path      string
	Branch    string
	Agent     string
	TaskID    string
	ProjectID string
	CreatedAt time.Time
	State     WorktreeState
}

// WorktreeStatus holds detailed status information about a worktree.
type WorktreeStatus struct {
	Path         string
	Branch       string
	State        WorktreeState
	IsClean      bool
	CommitCount  int
	LastActivity time.Time
}

// CreateOpts configures worktree creation.
type CreateOpts struct {
	Branch    string // branch name for the new worktree
	Agent     string // agent identifier
	TaskID    string // task identifier
	ProjectID string // project ID for scoping
	BaseDir   string // base directory for worktrees
}

// CleanOpts configures which worktrees to clean.
type CleanOpts struct {
	States []WorktreeState // states to clean (e.g., completed, orphaned)
	DryRun bool            // if true, report but don't remove
	Force  bool            // force removal even with uncommitted changes
}

// WorktreeManager defines the interface for managing git worktrees.
type WorktreeManager interface {
	Create(opts CreateOpts) (*Worktree, error)
	List() ([]*Worktree, error)
	Status(path string) (*WorktreeStatus, error)
	Clean(opts CleanOpts) (int, error)
	Remove(path string, force bool) error
}

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
	RunInDir(dir, name string, args ...string) ([]byte, error)
}

// execRunner implements CommandRunner using os/exec.
type execRunner struct{}

func (e *execRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (e *execRunner) RunInDir(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Manager implements WorktreeManager by shelling out to git commands.
type Manager struct {
	baseDir string
	runner  CommandRunner
}

// NewManager creates a Manager with the given base directory for worktrees.
func NewManager(baseDir string) *Manager {
	return &Manager{
		baseDir: baseDir,
		runner:  &execRunner{},
	}
}

// NewManagerWithRunner creates a Manager with a custom command runner (for testing).
func NewManagerWithRunner(baseDir string, runner CommandRunner) *Manager {
	return &Manager{
		baseDir: baseDir,
		runner:  runner,
	}
}

// Create creates a new git worktree with a new branch.
func (m *Manager) Create(opts CreateOpts) (*Worktree, error) {
	if opts.Branch == "" {
		return nil, fmt.Errorf("create worktree: branch is required")
	}

	baseDir := opts.BaseDir
	if baseDir == "" {
		baseDir = m.baseDir
	}

	worktreePath := filepath.Join(baseDir, opts.Branch)

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return nil, fmt.Errorf("create worktree parent dir: %w", err)
	}

	args := []string{"worktree", "add", "-b", opts.Branch, worktreePath}
	_, err := m.runner.Run("git", args...)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}

	wt := &Worktree{
		Path:      worktreePath,
		Branch:    opts.Branch,
		Agent:     opts.Agent,
		TaskID:    opts.TaskID,
		ProjectID: opts.ProjectID,
		CreatedAt: time.Now(),
		State:     WorktreeActive,
	}

	return wt, nil
}

// List returns all git worktrees known to git.
func (m *Manager) List() ([]*Worktree, error) {
	out, err := m.runner.Run("git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	return parseWorktreeList(string(out)), nil
}

// parseWorktreeList parses `git worktree list --porcelain` output.
func parseWorktreeList(output string) []*Worktree {
	var worktrees []*Worktree
	var current *Worktree

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				worktrees = append(worktrees, current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current = &Worktree{
				Path:      strings.TrimPrefix(line, "worktree "),
				State:     WorktreeActive,
				CreatedAt: time.Now(),
			}
		} else if strings.HasPrefix(line, "branch ") {
			if current != nil {
				ref := strings.TrimPrefix(line, "branch ")
				// Strip refs/heads/ prefix
				current.Branch = strings.TrimPrefix(ref, "refs/heads/")
			}
		} else if line == "bare" {
			if current != nil {
				current.Branch = "(bare)"
			}
		} else if line == "detached" {
			if current != nil {
				current.Branch = "(detached)"
			}
		}
	}

	// Append last entry if output didn't end with blank line
	if current != nil {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// Status returns detailed status for a specific worktree.
func (m *Manager) Status(path string) (*WorktreeStatus, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("status worktree %s: %w", path, err)
	}

	status := &WorktreeStatus{
		Path:         path,
		State:        WorktreeActive,
		LastActivity: info.ModTime(),
	}

	// Get branch name
	branchOut, err := m.runner.RunInDir(path, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		status.Branch = strings.TrimSpace(string(branchOut))
	}

	// Check if working tree is clean
	statusOut, err := m.runner.RunInDir(path, "git", "status", "--porcelain")
	if err == nil {
		status.IsClean = strings.TrimSpace(string(statusOut)) == ""
	}

	// Count commits ahead of main
	countOut, err := m.runner.RunInDir(path, "git", "rev-list", "--count", "HEAD", "--not", "main")
	if err == nil {
		fmt.Sscanf(strings.TrimSpace(string(countOut)), "%d", &status.CommitCount)
	}

	return status, nil
}

// Remove removes a git worktree.
func (m *Manager) Remove(path string, force bool) error {
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}

	_, err := m.runner.Run("git", args...)
	if err != nil {
		return fmt.Errorf("remove worktree %s: %w", path, err)
	}

	return nil
}

// Clean removes worktrees matching the given states.
// Returns the number of worktrees removed.
func (m *Manager) Clean(opts CleanOpts) (int, error) {
	worktrees, err := m.List()
	if err != nil {
		return 0, fmt.Errorf("clean worktrees: %w", err)
	}

	stateSet := make(map[WorktreeState]bool)
	for _, s := range opts.States {
		stateSet[s] = true
	}

	// If no states specified, default to completed and orphaned
	if len(stateSet) == 0 {
		stateSet[WorktreeCompleted] = true
		stateSet[WorktreeOrphaned] = true
	}

	removed := 0
	for _, wt := range worktrees {
		if !stateSet[wt.State] {
			continue
		}

		if opts.DryRun {
			removed++
			continue
		}

		if err := m.Remove(wt.Path, opts.Force); err != nil {
			// Log but continue cleaning other worktrees
			continue
		}
		removed++
	}

	// Also run git worktree prune to clean stale entries
	if !opts.DryRun {
		_, _ = m.runner.Run("git", "worktree", "prune")
	}

	return removed, nil
}
