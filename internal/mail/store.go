package mail

import "time"

// MailStore is the interface for the agent mail system.
type MailStore interface {
	// Send delivers a message to one or more agents. Broadcast addresses
	// are expanded before storage.
	Send(msg *MailMessage) error

	// Check returns unread messages for the named agent, ordered by priority
	// (urgent first) then creation time.
	Check(agent string, opts CheckOpts) ([]*MailMessage, error)

	// List returns messages matching the filter criteria.
	List(opts ListOpts) ([]*MailMessage, error)

	// MarkRead marks a single message as read by its ID.
	MarkRead(id string) error

	// Reply creates a new message in the same thread as the original message.
	Reply(id string, body string) error

	// Purge deletes messages matching the criteria and returns the count deleted.
	Purge(opts PurgeOpts) (int, error)
}

// CheckOpts controls filtering for Check.
type CheckOpts struct {
	// Type filters to a specific message type. Empty means all types.
	Type MessageType

	// Limit caps the number of returned messages. 0 means no limit.
	Limit int
}

// ListOpts controls filtering for List.
type ListOpts struct {
	// Agent filters to messages addressed to this agent. Empty means all.
	Agent string

	// From filters to messages sent by this agent. Empty means all.
	From string

	// Type filters to this message type. Empty means all types.
	Type MessageType

	// ThreadID filters to messages in this thread. Nil means all threads.
	ThreadID *string

	// Unread filters to unread messages only when true.
	Unread bool

	// Limit caps the number of returned messages. 0 means no limit.
	Limit int
}

// PurgeOpts controls which messages get purged.
type PurgeOpts struct {
	// Agent purges messages addressed to this agent. Empty means all agents.
	Agent string

	// Before purges messages created before this time. Zero means no time filter.
	Before time.Time

	// ReadOnly purges only messages marked as read when true.
	ReadOnly bool
}
