package commands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/noko/computecommander/internal/platform/db"
	"github.com/noko/computecommander/pkg/integrations/jira"
)

// PromptExecLog represents a row in jira_prompt_log.
type PromptExecLog struct {
	ID            int64  `json:"id"`
	InstanceName  string `json:"instanceName"`
	IssueKey      string `json:"issueKey"`
	PromptHash    string `json:"promptHash"`
	JiraCommentID string `json:"jiraCommentId,omitempty"`
	Status        string `json:"status"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
	BatchID       string `json:"batchId,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

// --- Tea message types ---

// jiraPromptExecResultMsg carries the outcome of a prompt exec with comment ID.
type jiraPromptExecResultMsg struct {
	issueKey   string
	promptHash string
	commentID  string
	logID      int64
	err        error
}

// jiraBatchExecResultMsg aggregates results from a batch prompt execution.
type jiraBatchExecResultMsg struct {
	total     int
	succeeded int
	failed    int
	errors    []string
	batchID   string
}

// jiraUndoResultMsg carries the outcome of an undo operation.
type jiraUndoResultMsg struct {
	issueKey string
	logID    int64
	err      error
}

// jiraLogEntriesMsg carries execution log entries for display.
type jiraLogEntriesMsg struct {
	entries []PromptExecLog
	err     error
}

// --- Prompt execution ---

// execPromptCmdWithLog generates a prompt, posts it as a Jira comment, and logs to DB.
// For parent tasks (Story/Task/Bug), it recursively fetches sub-tasks and generates
// a prompt that includes all sub-task material with orchestrator instructions.
func (m *jiraPaneModel) execPromptCmdWithLog(issueKey, batchID string) tea.Cmd {
	return func() tea.Msg {
		if m.syncEngine == nil {
			return jiraPromptExecResultMsg{issueKey: issueKey, err: fmt.Errorf("no sync engine")}
		}
		issue, err := m.syncEngine.GetCachedIssue(m.ctx, issueKey)
		if err != nil {
			logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
				InstanceName: m.inst.Name,
				IssueKey:     issueKey,
				PromptHash:   "",
				Status:       "failed",
				ErrorMessage: err.Error(),
				BatchID:      batchID,
			})
			return jiraPromptExecResultMsg{issueKey: issueKey, err: err}
		}

		pg := jira.NewPromptGenerator(m.app.Config.Jira.PromptTemplate)

		// For parent-type issues, fetch sub-tasks and use recursive generation.
		var result *jira.PromptResult
		if !isSubTaskType(issue.IssueType) {
			subTasks, _ := m.syncEngine.GetSubTasks(m.ctx, issueKey)
			if len(subTasks) > 0 {
				result, err = pg.GenerateRecursive(issue, subTasks, "", "", m.projectKey)
			} else {
				result, err = pg.Generate(issue, "", "", m.projectKey)
			}
		} else {
			result, err = pg.Generate(issue, "", "", m.projectKey)
		}
		if err != nil {
			logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
				InstanceName: m.inst.Name,
				IssueKey:     issueKey,
				PromptHash:   "",
				Status:       "failed",
				ErrorMessage: err.Error(),
				BatchID:      batchID,
			})
			return jiraPromptExecResultMsg{issueKey: issueKey, err: err}
		}

		commentBody := fmt.Sprintf("*Generated Prompt (cmdr)*\n\nHash: %s\n\n---\n\n%s",
			result.PromptHash[:12], result.Prompt)
		client := newJiraClient(m.inst, m.app.Config)
		commentID, err := client.AddCommentWithID(m.ctx, issueKey, commentBody)
		if err != nil {
			logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
				InstanceName: m.inst.Name,
				IssueKey:     issueKey,
				PromptHash:   result.PromptHash[:12],
				Status:       "failed",
				ErrorMessage: err.Error(),
				BatchID:      batchID,
			})
			return jiraPromptExecResultMsg{issueKey: issueKey, err: fmt.Errorf("comment failed: %w", err)}
		}

		logID, _ := logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
			InstanceName:  m.inst.Name,
			IssueKey:      issueKey,
			PromptHash:    result.PromptHash[:12],
			JiraCommentID: commentID,
			Status:        "success",
			BatchID:       batchID,
		})

		return jiraPromptExecResultMsg{
			issueKey:   issueKey,
			promptHash: result.PromptHash[:12],
			commentID:  commentID,
			logID:      logID,
		}
	}
}

// execBatchPromptCmd runs prompt generation for multiple keys sequentially.
func (m *jiraPaneModel) execBatchPromptCmd(keys []string) tea.Cmd {
	return func() tea.Msg {
		batchID := generateBatchID()
		var succeeded, failed int
		var errors []string

		for _, key := range keys {
			// Run each inline (not as tea.Cmd) since we're already in a Cmd callback.
			if m.syncEngine == nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: no sync engine", key))
				continue
			}
			issue, err := m.syncEngine.GetCachedIssue(m.ctx, key)
			if err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: %v", key, err))
				logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
					InstanceName: m.inst.Name,
					IssueKey:     key,
					Status:       "failed",
					ErrorMessage: err.Error(),
					BatchID:      batchID,
				})
				continue
			}

			pg := jira.NewPromptGenerator(m.app.Config.Jira.PromptTemplate)

			// For parent-type issues, fetch sub-tasks and use recursive generation.
			var result *jira.PromptResult
			if !isSubTaskType(issue.IssueType) {
				subTasks, _ := m.syncEngine.GetSubTasks(m.ctx, key)
				if len(subTasks) > 0 {
					result, err = pg.GenerateRecursive(issue, subTasks, "", "", m.projectKey)
				} else {
					result, err = pg.Generate(issue, "", "", m.projectKey)
				}
			} else {
				result, err = pg.Generate(issue, "", "", m.projectKey)
			}
			if err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: %v", key, err))
				logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
					InstanceName: m.inst.Name,
					IssueKey:     key,
					Status:       "failed",
					ErrorMessage: err.Error(),
					BatchID:      batchID,
				})
				continue
			}

			commentBody := fmt.Sprintf("*Generated Prompt (cmdr)*\n\nHash: %s\n\n---\n\n%s",
				result.PromptHash[:12], result.Prompt)
			client := newJiraClient(m.inst, m.app.Config)
			commentID, err := client.AddCommentWithID(m.ctx, key, commentBody)
			if err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("%s: comment failed: %v", key, err))
				logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
					InstanceName:  m.inst.Name,
					IssueKey:      key,
					PromptHash:    result.PromptHash[:12],
					Status:        "failed",
					ErrorMessage:  err.Error(),
					BatchID:       batchID,
				})
				continue
			}

			logPromptExecution(m.ctx, m.app.DB, PromptExecLog{
				InstanceName:  m.inst.Name,
				IssueKey:      key,
				PromptHash:    result.PromptHash[:12],
				JiraCommentID: commentID,
				Status:        "success",
				BatchID:       batchID,
			})
			succeeded++
		}

		return jiraBatchExecResultMsg{
			total:     len(keys),
			succeeded: succeeded,
			failed:    failed,
			errors:    errors,
			batchID:   batchID,
		}
	}
}

// undoPromptCmd finds the last successful prompt execution for an issue and deletes the Jira comment.
func (m *jiraPaneModel) undoPromptCmd(issueKey string) tea.Cmd {
	return func() tea.Msg {
		entry, err := getLastSuccessfulExec(m.ctx, m.app.DB, m.inst.Name, issueKey)
		if err != nil {
			return jiraUndoResultMsg{issueKey: issueKey, err: err}
		}
		if entry == nil {
			return jiraUndoResultMsg{issueKey: issueKey, err: fmt.Errorf("no successful prompt execution found for %s", issueKey)}
		}
		if entry.JiraCommentID == "" {
			return jiraUndoResultMsg{issueKey: issueKey, err: fmt.Errorf("no comment ID recorded for log entry %d", entry.ID)}
		}

		client := newJiraClient(m.inst, m.app.Config)
		if err := client.DeleteComment(m.ctx, issueKey, entry.JiraCommentID); err != nil {
			return jiraUndoResultMsg{issueKey: issueKey, logID: entry.ID, err: fmt.Errorf("delete comment failed: %w", err)}
		}

		// Mark log entry as undone.
		markExecUndone(m.ctx, m.app.DB, entry.ID)

		return jiraUndoResultMsg{issueKey: issueKey, logID: entry.ID}
	}
}

// fetchLogEntriesCmd fetches execution log entries for display.
func (m *jiraPaneModel) fetchLogEntriesCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := queryPromptLog(m.ctx, m.app.DB, "", 50)
		return jiraLogEntriesMsg{entries: entries, err: err}
	}
}

// selectedOrAllKeys returns selected keys, or all visible issue keys if none selected.
func (m *jiraPaneModel) selectedOrAllKeys() []string {
	keys := m.pane.SelectedKeys()
	if len(keys) > 0 {
		return keys
	}
	return m.pane.AllIssueKeys()
}

// --- DB operations ---

// logPromptExecution inserts a record into jira_prompt_log.
func logPromptExecution(ctx context.Context, database db.DB, entry PromptExecLog) (int64, error) {
	if database == nil {
		return 0, nil
	}
	err := database.Exec(ctx,
		`INSERT INTO jira_prompt_log (instance_name, issue_key, prompt_hash, jira_comment_id, status, error_message, batch_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.InstanceName, entry.IssueKey, entry.PromptHash,
		entry.JiraCommentID, entry.Status, entry.ErrorMessage, entry.BatchID,
	)
	if err != nil {
		return 0, err
	}

	// Get the last inserted ID.
	row := database.QueryRow(ctx, `SELECT last_insert_rowid()`)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, nil // non-fatal
	}
	return id, nil
}

// queryPromptLog queries execution log entries with optional issue key filter.
func queryPromptLog(ctx context.Context, database db.DB, issueKey string, limit int) ([]PromptExecLog, error) {
	if database == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	var query string
	var args []any
	if issueKey != "" {
		query = `SELECT id, instance_name, issue_key, prompt_hash, COALESCE(jira_comment_id, ''),
				 status, COALESCE(error_message, ''), COALESCE(batch_id, ''), created_at
				 FROM jira_prompt_log WHERE issue_key = ? ORDER BY created_at DESC LIMIT ?`
		args = []any{issueKey, limit}
	} else {
		query = `SELECT id, instance_name, issue_key, prompt_hash, COALESCE(jira_comment_id, ''),
				 status, COALESCE(error_message, ''), COALESCE(batch_id, ''), created_at
				 FROM jira_prompt_log ORDER BY created_at DESC LIMIT ?`
		args = []any{limit}
	}

	rows, err := database.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query prompt log: %w", err)
	}
	defer rows.Close()

	var entries []PromptExecLog
	for rows.Next() {
		var e PromptExecLog
		if err := rows.Scan(&e.ID, &e.InstanceName, &e.IssueKey, &e.PromptHash,
			&e.JiraCommentID, &e.Status, &e.ErrorMessage, &e.BatchID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan prompt log: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// queryPromptLogByBatch queries log entries for a specific batch ID.
func queryPromptLogByBatch(ctx context.Context, database db.DB, batchID string, limit int) ([]PromptExecLog, error) {
	if database == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := database.Query(ctx,
		`SELECT id, instance_name, issue_key, prompt_hash, COALESCE(jira_comment_id, ''),
		 status, COALESCE(error_message, ''), COALESCE(batch_id, ''), created_at
		 FROM jira_prompt_log WHERE batch_id = ? ORDER BY created_at DESC LIMIT ?`,
		batchID, limit)
	if err != nil {
		return nil, fmt.Errorf("query prompt log by batch: %w", err)
	}
	defer rows.Close()

	var entries []PromptExecLog
	for rows.Next() {
		var e PromptExecLog
		if err := rows.Scan(&e.ID, &e.InstanceName, &e.IssueKey, &e.PromptHash,
			&e.JiraCommentID, &e.Status, &e.ErrorMessage, &e.BatchID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan prompt log: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// getLastSuccessfulExec finds the most recent success log entry for an issue.
func getLastSuccessfulExec(ctx context.Context, database db.DB, instanceName, issueKey string) (*PromptExecLog, error) {
	if database == nil {
		return nil, fmt.Errorf("no database")
	}

	row := database.QueryRow(ctx,
		`SELECT id, instance_name, issue_key, prompt_hash, COALESCE(jira_comment_id, ''),
		 status, COALESCE(error_message, ''), COALESCE(batch_id, ''), created_at
		 FROM jira_prompt_log
		 WHERE instance_name = ? AND issue_key = ? AND status = 'success'
		 ORDER BY created_at DESC LIMIT 1`,
		instanceName, issueKey)

	var e PromptExecLog
	if err := row.Scan(&e.ID, &e.InstanceName, &e.IssueKey, &e.PromptHash,
		&e.JiraCommentID, &e.Status, &e.ErrorMessage, &e.BatchID, &e.CreatedAt); err != nil {
		return nil, nil // no rows = nil, nil
	}
	return &e, nil
}

// getPromptLogByID fetches a single log entry by ID.
func getPromptLogByID(ctx context.Context, database db.DB, id int64) (*PromptExecLog, error) {
	if database == nil {
		return nil, fmt.Errorf("no database")
	}

	row := database.QueryRow(ctx,
		`SELECT id, instance_name, issue_key, prompt_hash, COALESCE(jira_comment_id, ''),
		 status, COALESCE(error_message, ''), COALESCE(batch_id, ''), created_at
		 FROM jira_prompt_log WHERE id = ?`, id)

	var e PromptExecLog
	if err := row.Scan(&e.ID, &e.InstanceName, &e.IssueKey, &e.PromptHash,
		&e.JiraCommentID, &e.Status, &e.ErrorMessage, &e.BatchID, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("log entry %d not found", id)
	}
	return &e, nil
}

// markExecUndone updates a log entry status to "undone".
func markExecUndone(ctx context.Context, database db.DB, id int64) {
	if database == nil {
		return
	}
	_ = database.Exec(ctx, `UPDATE jira_prompt_log SET status = 'undone' WHERE id = ?`, id)
}

// isSubTaskType returns true if the issue type represents a sub-task.
func isSubTaskType(issueType string) bool {
	switch issueType {
	case "Sub-task", "Subtask", "Sub-Task":
		return true
	default:
		return false
	}
}

// generateBatchID creates a short random batch ID.
func generateBatchID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
