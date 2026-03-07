package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// AgentResolver resolves broadcast addresses to individual agent names.
// This allows the mail system to expand @all, @builders, etc. without
// depending on the session store directly.
type AgentResolver interface {
	ResolveAddress(addr string) ([]string, error)
}

// AgentResolverFunc adapts a plain function into an AgentResolver.
type AgentResolverFunc func(addr string) ([]string, error)

func (f AgentResolverFunc) ResolveAddress(addr string) ([]string, error) {
	return f(addr)
}

// sqlStore implements MailStore using the db.DB interface.
type sqlStore struct {
	db       db.DB
	resolver AgentResolver
}

// NewMailStore creates a MailStore backed by the given database.
// The resolver is used to expand broadcast addresses. If nil, broadcast
// messages are stored with the broadcast address as-is (useful for testing).
func NewMailStore(database db.DB, resolver AgentResolver) MailStore {
	return &sqlStore{
		db:       database,
		resolver: resolver,
	}
}

// generateID creates a unique message ID based on timestamp and a simple counter.
func generateID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

func (s *sqlStore) Send(msg *MailMessage) error {
	ctx := context.Background()

	if msg.ID == "" {
		msg.ID = generateID()
	}
	if msg.Priority == "" {
		msg.Priority = PriorityNormal
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	var payloadStr *string
	if msg.Payload != nil {
		p := string(msg.Payload)
		payloadStr = &p
	}

	// Expand broadcast addresses.
	recipients := []string{msg.To}
	if IsBroadcast(msg.To) && s.resolver != nil {
		resolved, err := s.resolver.ResolveAddress(msg.To)
		if err != nil {
			return fmt.Errorf("resolve broadcast %s: %w", msg.To, err)
		}
		if len(resolved) > 0 {
			recipients = resolved
		}
	}

	// Insert one row per recipient.
	for i, to := range recipients {
		id := msg.ID
		if i > 0 {
			id = fmt.Sprintf("%s-%d", msg.ID, i)
		}
		// Pass SQL NULL for empty ProjectID to avoid FK constraint violation.
		var projectID any
		if msg.ProjectID != "" {
			projectID = msg.ProjectID
		}
		err := s.db.Exec(ctx,
			`INSERT INTO mail (id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at, project_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
			id, msg.From, to, msg.Subject, msg.Body,
			string(msg.Priority), string(msg.Type),
			msg.ThreadID, payloadStr,
			msg.CreatedAt.Format(time.RFC3339),
			projectID,
		)
		if err != nil {
			return fmt.Errorf("insert mail for %s: %w", to, err)
		}
	}

	return nil
}

func (s *sqlStore) Check(agent string, opts CheckOpts) ([]*MailMessage, error) {
	ctx := context.Background()

	var clauses []string
	var args []any

	clauses = append(clauses, "to_agent = ?")
	args = append(args, agent)

	clauses = append(clauses, "read = 0")

	if opts.Type != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, string(opts.Type))
	}

	query := `SELECT id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at
		FROM mail WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY
			CASE priority
				WHEN 'urgent' THEN 3
				WHEN 'high' THEN 2
				WHEN 'normal' THEN 1
				WHEN 'low' THEN 0
				ELSE 1
			END DESC,
			created_at ASC`

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	return s.queryMessages(ctx, query, args...)
}

func (s *sqlStore) List(opts ListOpts) ([]*MailMessage, error) {
	ctx := context.Background()

	var clauses []string
	var args []any

	if opts.Agent != "" {
		clauses = append(clauses, "to_agent = ?")
		args = append(args, opts.Agent)
	}
	if opts.From != "" {
		clauses = append(clauses, "from_agent = ?")
		args = append(args, opts.From)
	}
	if opts.Type != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, string(opts.Type))
	}
	if opts.ThreadID != nil {
		clauses = append(clauses, "thread_id = ?")
		args = append(args, *opts.ThreadID)
	}
	if opts.Unread {
		clauses = append(clauses, "read = 0")
	}
	if opts.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, opts.ProjectID)
	}

	query := `SELECT id, from_agent, to_agent, subject, body, priority, type, thread_id, payload, read, created_at
		FROM mail`

	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}

	query += `
		ORDER BY
			CASE priority
				WHEN 'urgent' THEN 3
				WHEN 'high' THEN 2
				WHEN 'normal' THEN 1
				WHEN 'low' THEN 0
				ELSE 1
			END DESC,
			created_at ASC`

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	return s.queryMessages(ctx, query, args...)
}

func (s *sqlStore) MarkRead(id string) error {
	ctx := context.Background()
	return s.db.Exec(ctx, "UPDATE mail SET read = 1 WHERE id = ?", id)
}

func (s *sqlStore) Reply(id string, body string) error {
	ctx := context.Background()

	// Look up the original message to get From, To, Subject, ThreadID.
	var origFrom, origTo, origSubject string
	var origThreadID *string

	err := s.db.QueryRow(ctx,
		"SELECT from_agent, to_agent, subject, thread_id FROM mail WHERE id = ?", id,
	).Scan(&origFrom, &origTo, &origSubject, &origThreadID)
	if err != nil {
		return fmt.Errorf("lookup original message %s: %w", id, err)
	}

	// Thread links to the original message ID if no thread exists yet.
	threadID := id
	if origThreadID != nil {
		threadID = *origThreadID
	}

	subject := origSubject
	if !strings.HasPrefix(subject, "Re: ") {
		subject = "Re: " + subject
	}

	reply := &MailMessage{
		ID:       generateID(),
		From:     origTo,
		To:       origFrom,
		Subject:  subject,
		Body:     body,
		Priority: PriorityNormal,
		Type:     TypeResult,
		ThreadID: &threadID,
	}

	return s.Send(reply)
}

func (s *sqlStore) Purge(opts PurgeOpts) (int, error) {
	ctx := context.Background()

	var clauses []string
	var args []any

	if opts.Agent != "" {
		clauses = append(clauses, "to_agent = ?")
		args = append(args, opts.Agent)
	}
	if !opts.Before.IsZero() {
		clauses = append(clauses, "created_at < ?")
		args = append(args, opts.Before.Format(time.RFC3339))
	}
	if opts.ReadOnly {
		clauses = append(clauses, "read = 1")
	}

	// Count first, then delete.
	countQuery := "SELECT COUNT(*) FROM mail"
	deleteQuery := "DELETE FROM mail"

	if len(clauses) > 0 {
		where := " WHERE " + strings.Join(clauses, " AND ")
		countQuery += where
		deleteQuery += where
	}

	var count int
	err := s.db.QueryRow(ctx, countQuery, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count for purge: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	err = s.db.Exec(ctx, deleteQuery, args...)
	if err != nil {
		return 0, fmt.Errorf("purge mail: %w", err)
	}

	return count, nil
}

// queryMessages executes a query and scans results into MailMessage slices.
func (s *sqlStore) queryMessages(ctx context.Context, query string, args ...any) ([]*MailMessage, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query mail: %w", err)
	}
	defer rows.Close()

	var msgs []*MailMessage
	for rows.Next() {
		var m MailMessage
		var priorityStr, typeStr string
		var threadID *string
		var payloadStr *string
		var readInt int
		var createdStr string

		err := rows.Scan(
			&m.ID, &m.From, &m.To, &m.Subject, &m.Body,
			&priorityStr, &typeStr, &threadID, &payloadStr,
			&readInt, &createdStr,
		)
		if err != nil {
			return nil, fmt.Errorf("scan mail row: %w", err)
		}

		m.Priority = Priority(priorityStr)
		m.Type = MessageType(typeStr)
		m.ThreadID = threadID
		m.Read = readInt != 0

		if payloadStr != nil {
			m.Payload = json.RawMessage(*payloadStr)
		}

		t, err := time.Parse(time.RFC3339, createdStr)
		if err != nil {
			// Try the SQLite default datetime format.
			t, err = time.Parse("2006-01-02 15:04:05", createdStr)
			if err != nil {
				return nil, fmt.Errorf("parse created_at %q: %w", createdStr, err)
			}
		}
		m.CreatedAt = t

		msgs = append(msgs, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return msgs, nil
}
