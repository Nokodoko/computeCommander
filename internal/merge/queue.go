package merge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// ErrQueueEmpty is returned when the queue has no pending entries.
var ErrQueueEmpty = errors.New("merge queue is empty")

// ErrNotFound is returned when a branch is not found in the queue.
var ErrNotFound = errors.New("merge entry not found")

// MergeQueue defines the FIFO merge queue interface per spec 3.4.3.
type MergeQueue interface {
	Enqueue(entry *MergeEntry) error
	Dequeue() (*MergeEntry, error)
	Peek() (*MergeEntry, error)
	Status(branch string) (*MergeEntry, error)
	List(opts ListOpts) ([]*MergeEntry, error)
}

// SQLQueue implements MergeQueue backed by the merge_queue table.
type SQLQueue struct {
	db db.DB
}

// NewSQLQueue creates a new SQL-backed merge queue.
func NewSQLQueue(d db.DB) *SQLQueue {
	return &SQLQueue{db: d}
}

// Enqueue inserts a new entry into the merge queue with pending status.
func (q *SQLQueue) Enqueue(entry *MergeEntry) error {
	if entry.BranchName == "" {
		return fmt.Errorf("enqueue: branch name is required")
	}
	if entry.TaskID == "" {
		return fmt.Errorf("enqueue: task ID is required")
	}
	if entry.AgentName == "" {
		return fmt.Errorf("enqueue: agent name is required")
	}

	entry.Status = MergePending
	entry.EnqueuedAt = time.Now().UTC()

	filesJSON, err := json.Marshal(entry.FilesModified)
	if err != nil {
		return fmt.Errorf("enqueue: marshal files_modified: %w", err)
	}

	ctx := context.Background()
	// Pass SQL NULL for empty ProjectID to avoid FK constraint violation.
	var projectID any
	if entry.ProjectID != "" {
		projectID = entry.ProjectID
	}
	err = q.db.Exec(ctx,
		`INSERT INTO merge_queue (branch_name, task_id, agent_name, files_modified, enqueued_at, status, project_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.BranchName, entry.TaskID, entry.AgentName,
		string(filesJSON), entry.EnqueuedAt.Format(time.RFC3339),
		string(entry.Status), projectID,
	)
	if err != nil {
		return fmt.Errorf("enqueue %s: %w", entry.BranchName, err)
	}

	return nil
}

// Dequeue removes and returns the oldest pending entry from the queue.
// The entry's status is atomically set to "merging".
func (q *SQLQueue) Dequeue() (*MergeEntry, error) {
	ctx := context.Background()

	tx, err := q.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue: begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(ctx,
		`SELECT branch_name, task_id, agent_name, files_modified, enqueued_at, status, resolved_tier
		 FROM merge_queue
		 WHERE status = ?
		 ORDER BY enqueued_at ASC
		 LIMIT 1`,
		string(MergePending),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue: query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("dequeue: rows err: %w", err)
		}
		return nil, ErrQueueEmpty
	}

	entry, err := scanEntry(rows)
	if err != nil {
		return nil, fmt.Errorf("dequeue: scan: %w", err)
	}

	err = tx.Exec(ctx,
		`UPDATE merge_queue SET status = ? WHERE branch_name = ?`,
		string(MergeMerging), entry.BranchName,
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue: update status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("dequeue: commit: %w", err)
	}

	entry.Status = MergeMerging
	return entry, nil
}

// Peek returns the oldest pending entry without removing it.
func (q *SQLQueue) Peek() (*MergeEntry, error) {
	ctx := context.Background()

	rows, err := q.db.Query(ctx,
		`SELECT branch_name, task_id, agent_name, files_modified, enqueued_at, status, resolved_tier
		 FROM merge_queue
		 WHERE status = ?
		 ORDER BY enqueued_at ASC
		 LIMIT 1`,
		string(MergePending),
	)
	if err != nil {
		return nil, fmt.Errorf("peek: query: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("peek: rows err: %w", err)
		}
		return nil, ErrQueueEmpty
	}

	entry, err := scanEntry(rows)
	if err != nil {
		return nil, fmt.Errorf("peek: scan: %w", err)
	}

	return entry, nil
}

// Status returns the current queue entry for a given branch.
func (q *SQLQueue) Status(branch string) (*MergeEntry, error) {
	ctx := context.Background()

	rows, err := q.db.Query(ctx,
		`SELECT branch_name, task_id, agent_name, files_modified, enqueued_at, status, resolved_tier
		 FROM merge_queue
		 WHERE branch_name = ?`,
		branch,
	)
	if err != nil {
		return nil, fmt.Errorf("status %s: query: %w", branch, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("status %s: rows err: %w", branch, err)
		}
		return nil, ErrNotFound
	}

	entry, err := scanEntry(rows)
	if err != nil {
		return nil, fmt.Errorf("status %s: scan: %w", branch, err)
	}

	return entry, nil
}

// List returns queue entries filtered by the given options.
func (q *SQLQueue) List(opts ListOpts) ([]*MergeEntry, error) {
	ctx := context.Background()

	query := `SELECT branch_name, task_id, agent_name, files_modified, enqueued_at, status, resolved_tier
		 FROM merge_queue`
	var args []any
	var clauses []string

	if opts.Status != nil {
		clauses = append(clauses, "status = ?")
		args = append(args, string(*opts.Status))
	}
	if opts.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, opts.ProjectID)
	}

	if len(clauses) > 0 {
		query += ` WHERE ` + clauses[0]
		for _, c := range clauses[1:] {
			query += ` AND ` + c
		}
	}

	query += ` ORDER BY enqueued_at ASC`

	if opts.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, opts.Limit)
	}

	rows, err := q.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list: query: %w", err)
	}
	defer rows.Close()

	var entries []*MergeEntry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("list: scan: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list: rows err: %w", err)
	}

	return entries, nil
}

// UpdateStatus updates the status and optionally the resolved tier for a branch.
func (q *SQLQueue) UpdateStatus(branch string, status MergeStatus, tier *ResolutionTier) error {
	ctx := context.Background()

	var tierVal any
	if tier != nil {
		tierVal = string(*tier)
	}

	err := q.db.Exec(ctx,
		`UPDATE merge_queue SET status = ?, resolved_tier = ? WHERE branch_name = ?`,
		string(status), tierVal, branch,
	)
	if err != nil {
		return fmt.Errorf("update status %s: %w", branch, err)
	}

	return nil
}

// scanner is the interface satisfied by *db.Rows for scanning a single row.
type scanner interface {
	Scan(dest ...any) error
}

// scanEntry scans a merge_queue row into a MergeEntry.
func scanEntry(rows *db.Rows) (*MergeEntry, error) {
	var entry MergeEntry
	var filesJSON string
	var enqueuedAt string
	var status string
	var resolvedTier *string

	if err := rows.Scan(
		&entry.BranchName,
		&entry.TaskID,
		&entry.AgentName,
		&filesJSON,
		&enqueuedAt,
		&status,
		&resolvedTier,
	); err != nil {
		return nil, err
	}

	entry.Status = MergeStatus(status)

	if resolvedTier != nil {
		tier := ResolutionTier(*resolvedTier)
		entry.ResolvedTier = &tier
	}

	if err := json.Unmarshal([]byte(filesJSON), &entry.FilesModified); err != nil {
		// Fall back: treat as empty if not valid JSON
		entry.FilesModified = nil
	}

	t, err := time.Parse(time.RFC3339, enqueuedAt)
	if err != nil {
		// Try alternate format
		t, err = time.Parse("2006-01-02 15:04:05", enqueuedAt)
		if err != nil {
			entry.EnqueuedAt = time.Time{}
		} else {
			entry.EnqueuedAt = t
		}
	} else {
		entry.EnqueuedAt = t
	}

	return &entry, nil
}
