// Package mail provides the agent-to-agent messaging system for ComputeCommander.
package mail

import (
	"encoding/json"
	"time"
)

// MessageType classifies the purpose of a mail message.
type MessageType string

const (
	// Semantic types (human-readable status).
	TypeStatus   MessageType = "status"
	TypeQuestion MessageType = "question"
	TypeResult   MessageType = "result"
	TypeError    MessageType = "error"

	// Protocol types (structured coordination).
	TypeWorkerDone  MessageType = "worker_done"
	TypeMergeReady  MessageType = "merge_ready"
	TypeMerged      MessageType = "merged"
	TypeMergeFailed MessageType = "merge_failed"
	TypeEscalation  MessageType = "escalation"
	TypeHealthCheck MessageType = "health_check"
	TypeDispatch    MessageType = "dispatch"
	TypeAssign      MessageType = "assign"
)

// Priority determines message ordering. Higher priority messages surface first.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// priorityWeight maps Priority to a numeric weight for ORDER BY.
// Higher weight = higher priority = sorted first (DESC).
var priorityWeight = map[Priority]int{
	PriorityLow:    0,
	PriorityNormal: 1,
	PriorityHigh:   2,
	PriorityUrgent: 3,
}

// Weight returns the numeric sort weight for this priority.
func (p Priority) Weight() int {
	if w, ok := priorityWeight[p]; ok {
		return w
	}
	return 1 // default to normal
}

// Broadcast addresses for group messaging.
const (
	BroadcastAll       = "@all"
	BroadcastBuilders  = "@builders"
	BroadcastScouts    = "@scouts"
	BroadcastReviewers = "@reviewers"
	BroadcastLeads     = "@leads"
	BroadcastWorkers   = "@workers"
)

// IsBroadcast returns true if the address is a broadcast address.
func IsBroadcast(addr string) bool {
	switch addr {
	case BroadcastAll, BroadcastBuilders, BroadcastScouts,
		BroadcastReviewers, BroadcastLeads, BroadcastWorkers:
		return true
	}
	return false
}

// MailMessage is the core message type exchanged between agents.
type MailMessage struct {
	ID        string          `json:"id" db:"id"`
	From      string          `json:"from" db:"from_agent"`
	To        string          `json:"to" db:"to_agent"`
	Subject   string          `json:"subject" db:"subject"`
	Body      string          `json:"body" db:"body"`
	Priority  Priority        `json:"priority" db:"priority"`
	Type      MessageType     `json:"type" db:"type"`
	ThreadID  *string         `json:"threadId" db:"thread_id"`
	Payload   json.RawMessage `json:"payload" db:"payload"`
	Read      bool            `json:"read" db:"read"`
	CreatedAt time.Time       `json:"createdAt" db:"created_at"`
	ProjectID string          `json:"projectId,omitempty" db:"project_id"`
}
