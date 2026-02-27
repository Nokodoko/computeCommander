package merge

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	RunInDir(dir, name string, args ...string) ([]byte, error)
}

// execRunner implements CommandRunner using os/exec.
type execRunner struct{}

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

// MergeExecutor orchestrates branch merges with tiered conflict resolution.
type MergeExecutor struct {
	queue            *SQLQueue
	projectRoot      string
	runner           CommandRunner
	aiResolveEnabled bool
	reimagineEnabled bool
}

// NewMergeExecutor creates a MergeExecutor with the given options.
func NewMergeExecutor(opts MergeOpts) *MergeExecutor {
	d, _ := opts.DB.(*interface{})
	_ = d // DB is used via queue

	return &MergeExecutor{
		projectRoot:      opts.ProjectRoot,
		runner:           &execRunner{},
		aiResolveEnabled: opts.AIResolveEnabled,
		reimagineEnabled: opts.ReimagineEnabled,
	}
}

// NewMergeExecutorWithQueue creates a MergeExecutor with an explicit queue and runner.
func NewMergeExecutorWithQueue(queue *SQLQueue, projectRoot string, runner CommandRunner, aiResolve, reimagine bool) *MergeExecutor {
	return &MergeExecutor{
		queue:            queue,
		projectRoot:      projectRoot,
		runner:           runner,
		aiResolveEnabled: aiResolve,
		reimagineEnabled: reimagine,
	}
}

// Execute attempts to merge the given entry's branch into the target branch
// using the 4-tier resolution strategy.
func (m *MergeExecutor) Execute(entry *MergeEntry, targetBranch string) (*MergeResult, error) {
	if entry == nil {
		return nil, fmt.Errorf("execute: entry is nil")
	}
	if targetBranch == "" {
		return nil, fmt.Errorf("execute: target branch is required")
	}

	// Tier 1: Clean Merge
	result := m.tier1CleanMerge(entry.BranchName, targetBranch)
	if result.Success {
		m.updateEntryStatus(entry, MergeMerged, TierCleanMerge)
		return result, nil
	}

	// Abort the failed merge before trying next tier
	m.abortMerge()

	// Tier 2: Auto-Resolve
	result = m.tier2AutoResolve(entry.BranchName, targetBranch)
	if result.Success {
		m.updateEntryStatus(entry, MergeMerged, TierAutoResolve)
		return result, nil
	}

	// Abort the failed merge before trying next tier
	m.abortMerge()

	// Tier 3: AI Resolve (stub)
	if m.aiResolveEnabled {
		result = m.tier3AIResolve(entry.BranchName, targetBranch)
		if result.Success {
			m.updateEntryStatus(entry, MergeMerged, TierAIResolve)
			return result, nil
		}
		m.abortMerge()
	}

	// Tier 4: Reimagine (stub)
	if m.reimagineEnabled {
		result = m.tier4Reimagine(entry.BranchName, targetBranch)
		if result.Success {
			m.updateEntryStatus(entry, MergeMerged, TierReimagine)
			return result, nil
		}
		m.abortMerge()
	}

	// All tiers exhausted
	m.updateEntryStatus(entry, MergeFailed, "")
	return &MergeResult{
		Success:       false,
		Tier:          result.Tier,
		ConflictFiles: result.ConflictFiles,
		Error:         fmt.Errorf("all resolution tiers exhausted for branch %s", entry.BranchName),
	}, nil
}

// tier1CleanMerge attempts a simple git merge --no-edit.
func (m *MergeExecutor) tier1CleanMerge(branch, target string) *MergeResult {
	// Ensure we're on the target branch
	if _, err := m.runner.RunInDir(m.projectRoot, "git", "checkout", target); err != nil {
		return &MergeResult{
			Success: false,
			Tier:    TierCleanMerge,
			Error:   fmt.Errorf("tier1: checkout %s: %w", target, err),
		}
	}

	// Attempt clean merge
	_, err := m.runner.RunInDir(m.projectRoot, "git", "merge", "--no-edit", branch)
	if err != nil {
		conflicts := m.getConflictFiles()
		return &MergeResult{
			Success:       false,
			Tier:          TierCleanMerge,
			ConflictFiles: conflicts,
			Error:         fmt.Errorf("tier1: merge %s: %w", branch, err),
		}
	}

	return &MergeResult{
		Success: true,
		Tier:    TierCleanMerge,
	}
}

// tier2AutoResolve attempts merge with strategies for trivial conflicts.
func (m *MergeExecutor) tier2AutoResolve(branch, target string) *MergeResult {
	// Ensure we're on the target branch
	if _, err := m.runner.RunInDir(m.projectRoot, "git", "checkout", target); err != nil {
		return &MergeResult{
			Success: false,
			Tier:    TierAutoResolve,
			Error:   fmt.Errorf("tier2: checkout %s: %w", target, err),
		}
	}

	// Try merge with recursive strategy favoring theirs for trivial conflicts
	_, err := m.runner.RunInDir(m.projectRoot, "git", "merge", "--no-edit", "-X", "theirs", branch)
	if err != nil {
		conflicts := m.getConflictFiles()
		return &MergeResult{
			Success:       false,
			Tier:          TierAutoResolve,
			ConflictFiles: conflicts,
			Error:         fmt.Errorf("tier2: auto-resolve %s: %w", branch, err),
		}
	}

	return &MergeResult{
		Success: true,
		Tier:    TierAutoResolve,
	}
}

// tier3AIResolve is a stub that logs the AI resolution attempt.
func (m *MergeExecutor) tier3AIResolve(branch, target string) *MergeResult {
	log.Printf("[merge] tier3: AI resolution would be attempted for branch %s into %s", branch, target)

	return &MergeResult{
		Success: false,
		Tier:    TierAIResolve,
		Error:   fmt.Errorf("tier3: AI resolve not implemented"),
	}
}

// tier4Reimagine is a stub that logs the reimagine attempt.
func (m *MergeExecutor) tier4Reimagine(branch, target string) *MergeResult {
	log.Printf("[merge] tier4: reimagine would be attempted for branch %s into %s", branch, target)

	return &MergeResult{
		Success: false,
		Tier:    TierReimagine,
		Error:   fmt.Errorf("tier4: reimagine not implemented"),
	}
}

// abortMerge aborts a merge in progress.
func (m *MergeExecutor) abortMerge() {
	_, _ = m.runner.RunInDir(m.projectRoot, "git", "merge", "--abort")
}

// getConflictFiles returns the list of files with merge conflicts.
func (m *MergeExecutor) getConflictFiles() []string {
	out, err := m.runner.RunInDir(m.projectRoot, "git", "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// updateEntryStatus updates the queue entry status if a queue is available.
func (m *MergeExecutor) updateEntryStatus(entry *MergeEntry, status MergeStatus, tier ResolutionTier) {
	entry.Status = status
	if tier != "" {
		entry.ResolvedTier = &tier
	}

	if m.queue != nil {
		var tierPtr *ResolutionTier
		if tier != "" {
			tierPtr = &tier
		}
		_ = m.queue.UpdateStatus(entry.BranchName, status, tierPtr)
	}
}
