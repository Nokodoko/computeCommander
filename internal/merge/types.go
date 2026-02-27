// Package merge implements a FIFO merge queue with 4-tier conflict resolution
// for integrating agent worktree branches into the canonical branch.
package merge

import (
	"time"
)

// MergeStatus represents the state of a merge queue entry.
type MergeStatus string

const (
	MergePending  MergeStatus = "pending"
	MergeMerging  MergeStatus = "merging"
	MergeMerged   MergeStatus = "merged"
	MergeConflict MergeStatus = "conflict"
	MergeFailed   MergeStatus = "failed"
)

// ResolutionTier represents the conflict resolution strategy used.
type ResolutionTier string

const (
	TierCleanMerge  ResolutionTier = "clean-merge"
	TierAutoResolve ResolutionTier = "auto-resolve"
	TierAIResolve   ResolutionTier = "ai-resolve"
	TierReimagine   ResolutionTier = "reimagine"
)

// MergeEntry represents a branch queued for merging.
type MergeEntry struct {
	BranchName    string          `db:"branch_name"`
	TaskID        string          `db:"task_id"`
	AgentName     string          `db:"agent_name"`
	FilesModified []string        `db:"files_modified"`
	EnqueuedAt    time.Time       `db:"enqueued_at"`
	Status        MergeStatus     `db:"status"`
	ResolvedTier  *ResolutionTier `db:"resolved_tier"`
}

// ListOpts configures filtering for queue listing.
type ListOpts struct {
	Status *MergeStatus // filter by status; nil means all
	Limit  int          // max entries to return; 0 means no limit
}

// MergeResult holds the outcome of a merge execution attempt.
type MergeResult struct {
	Success       bool
	Tier          ResolutionTier
	ConflictFiles []string
	Error         error
}

// MergeOpts configures a MergeExecutor.
type MergeOpts struct {
	DB               interface{} // db.DB — kept as interface to avoid import cycle
	WorktreeManager  interface{} // worktree.WorktreeManager — kept as interface
	ProjectRoot      string      // root of the git repository
	AIResolveEnabled bool        // whether tier 3 AI resolution is enabled
	ReimagineEnabled bool        // whether tier 4 reimagine is enabled
}
