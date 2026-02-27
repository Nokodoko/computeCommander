package mail

import (
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

func newTestStore(t *testing.T) MailStore {
	t.Helper()
	d, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := db.Migrate(d, "sqlite"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return NewMailStore(d, nil)
}

func newTestStoreWithResolver(t *testing.T, resolver AgentResolver) MailStore {
	t.Helper()
	d, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	if err := db.Migrate(d, "sqlite"); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	return NewMailStore(d, resolver)
}

func TestSendAndCheck(t *testing.T) {
	store := newTestStore(t)

	msg := &MailMessage{
		From:    "coordinator",
		To:      "builder-1",
		Subject: "task assignment",
		Body:    "build the auth module",
		Type:    TypeDispatch,
	}

	err := store.Send(msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msgs, err := store.Check("builder-1", CheckOpts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	got := msgs[0]
	if got.From != "coordinator" {
		t.Errorf("expected from=coordinator, got %q", got.From)
	}
	if got.To != "builder-1" {
		t.Errorf("expected to=builder-1, got %q", got.To)
	}
	if got.Subject != "task assignment" {
		t.Errorf("expected subject='task assignment', got %q", got.Subject)
	}
	if got.Body != "build the auth module" {
		t.Errorf("expected body='build the auth module', got %q", got.Body)
	}
	if got.Type != TypeDispatch {
		t.Errorf("expected type=dispatch, got %q", got.Type)
	}
	if got.Priority != PriorityNormal {
		t.Errorf("expected priority=normal, got %q", got.Priority)
	}
	if got.Read {
		t.Error("expected read=false")
	}
}

func TestCheckReturnsOnlyUnread(t *testing.T) {
	store := newTestStore(t)

	msg1 := &MailMessage{
		From: "coord", To: "builder-1", Subject: "msg1", Body: "body1", Type: TypeStatus,
	}
	msg2 := &MailMessage{
		From: "coord", To: "builder-1", Subject: "msg2", Body: "body2", Type: TypeStatus,
	}

	store.Send(msg1)
	store.Send(msg2)

	// Mark the first message read.
	msgs, _ := store.Check("builder-1", CheckOpts{})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	store.MarkRead(msgs[0].ID)

	// Check again; should only see one.
	msgs, err := store.Check("builder-1", CheckOpts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(msgs))
	}
	if msgs[0].Subject != "msg2" {
		t.Errorf("expected msg2, got %q", msgs[0].Subject)
	}
}

func TestCheckPriorityOrdering(t *testing.T) {
	store := newTestStore(t)

	// Send messages in low-to-high priority order.
	priorities := []Priority{PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent}
	for _, p := range priorities {
		store.Send(&MailMessage{
			From: "coord", To: "agent-1", Subject: string(p),
			Body: "body", Type: TypeStatus, Priority: p,
		})
	}

	msgs, err := store.Check("agent-1", CheckOpts{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Urgent should be first, low should be last.
	expected := []Priority{PriorityUrgent, PriorityHigh, PriorityNormal, PriorityLow}
	for i, msg := range msgs {
		if msg.Priority != expected[i] {
			t.Errorf("position %d: expected priority %q, got %q", i, expected[i], msg.Priority)
		}
	}
}

func TestCheckWithTypeFilter(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "dispatch", Body: "work", Type: TypeDispatch})
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "status", Body: "ok", Type: TypeStatus})

	msgs, err := store.Check("agent-1", CheckOpts{Type: TypeDispatch})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Type != TypeDispatch {
		t.Errorf("expected dispatch, got %q", msgs[0].Type)
	}
}

func TestCheckWithLimit(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 5; i++ {
		store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg", Body: "body", Type: TypeStatus})
	}

	msgs, err := store.Check("agent-1", CheckOpts{Limit: 2})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestListAll(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg1", Body: "body", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-2", Subject: "msg2", Body: "body", Type: TypeDispatch})

	msgs, err := store.List(ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestListFilterByAgent(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg1", Body: "body", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-2", Subject: "msg2", Body: "body", Type: TypeStatus})

	msgs, err := store.List(ListOpts{Agent: "agent-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].To != "agent-1" {
		t.Errorf("expected to=agent-1, got %q", msgs[0].To)
	}
}

func TestListFilterByFrom(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg1", Body: "body", Type: TypeStatus})
	store.Send(&MailMessage{From: "lead-1", To: "agent-1", Subject: "msg2", Body: "body", Type: TypeAssign})

	msgs, err := store.List(ListOpts{From: "lead-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].From != "lead-1" {
		t.Errorf("expected from=lead-1, got %q", msgs[0].From)
	}
}

func TestListFilterByType(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeDispatch})
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeError})

	msgs, err := store.List(ListOpts{Type: TypeError})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1, got %d", len(msgs))
	}
	if msgs[0].Type != TypeError {
		t.Errorf("expected error type, got %q", msgs[0].Type)
	}
}

func TestListFilterUnread(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "c", Body: "d", Type: TypeStatus})

	// Mark the first one read.
	all, _ := store.List(ListOpts{})
	store.MarkRead(all[0].ID)

	msgs, err := store.List(ListOpts{Unread: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 unread, got %d", len(msgs))
	}
}

func TestListFilterByThreadID(t *testing.T) {
	store := newTestStore(t)

	threadID := "thread-42"
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeStatus, ThreadID: &threadID})
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "c", Body: "d", Type: TypeStatus})

	msgs, err := store.List(ListOpts{ThreadID: &threadID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1, got %d", len(msgs))
	}
	if msgs[0].ThreadID == nil || *msgs[0].ThreadID != threadID {
		t.Errorf("expected threadID=%s", threadID)
	}
}

func TestMarkRead(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "test", Body: "body", Type: TypeStatus})

	msgs, _ := store.Check("agent-1", CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Read {
		t.Fatal("expected unread")
	}

	err := store.MarkRead(msgs[0].ID)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	// Check should now return empty.
	msgs, _ = store.Check("agent-1", CheckOpts{})
	if len(msgs) != 0 {
		t.Errorf("expected 0 unread after MarkRead, got %d", len(msgs))
	}

	// List without unread filter should still show it.
	all, _ := store.List(ListOpts{Agent: "agent-1"})
	if len(all) != 1 {
		t.Fatalf("expected 1 in list, got %d", len(all))
	}
	if !all[0].Read {
		t.Error("expected read=true after MarkRead")
	}
}

func TestReply(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{
		From: "coordinator", To: "builder-1", Subject: "build auth",
		Body: "please build the auth module", Type: TypeDispatch,
	})

	msgs, _ := store.Check("builder-1", CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	err := store.Reply(msgs[0].ID, "auth module complete")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	// The reply should appear in coordinator's inbox.
	replies, err := store.Check("coordinator", CheckOpts{})
	if err != nil {
		t.Fatalf("Check coordinator: %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}

	reply := replies[0]
	if reply.From != "builder-1" {
		t.Errorf("expected from=builder-1, got %q", reply.From)
	}
	if reply.To != "coordinator" {
		t.Errorf("expected to=coordinator, got %q", reply.To)
	}
	if reply.Subject != "Re: build auth" {
		t.Errorf("expected subject='Re: build auth', got %q", reply.Subject)
	}
	if reply.Body != "auth module complete" {
		t.Errorf("expected body='auth module complete', got %q", reply.Body)
	}
	if reply.ThreadID == nil {
		t.Fatal("expected threadID to be set")
	}
	if *reply.ThreadID != msgs[0].ID {
		t.Errorf("expected threadID=%s, got %s", msgs[0].ID, *reply.ThreadID)
	}
}

func TestReplyPreservesExistingThread(t *testing.T) {
	store := newTestStore(t)

	threadID := "original-thread"
	store.Send(&MailMessage{
		From: "coord", To: "agent-1", Subject: "Re: something",
		Body: "followup", Type: TypeStatus, ThreadID: &threadID,
	})

	msgs, _ := store.Check("agent-1", CheckOpts{})
	err := store.Reply(msgs[0].ID, "my reply")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	replies, _ := store.Check("coord", CheckOpts{})
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if replies[0].ThreadID == nil || *replies[0].ThreadID != threadID {
		t.Errorf("expected threadID=%s preserved", threadID)
	}
	// Subject already has "Re: " prefix, should not double it.
	if replies[0].Subject != "Re: something" {
		t.Errorf("expected subject='Re: something', got %q", replies[0].Subject)
	}
}

func TestPurgeAll(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 3; i++ {
		store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg", Body: "body", Type: TypeStatus})
	}

	count, err := store.Purge(PurgeOpts{})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 purged, got %d", count)
	}

	msgs, _ := store.List(ListOpts{})
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after purge, got %d", len(msgs))
	}
}

func TestPurgeByAgent(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-2", Subject: "c", Body: "d", Type: TypeStatus})

	count, err := store.Purge(PurgeOpts{Agent: "agent-1"})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 purged, got %d", count)
	}

	msgs, _ := store.List(ListOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(msgs))
	}
	if msgs[0].To != "agent-2" {
		t.Errorf("expected agent-2 to remain, got %q", msgs[0].To)
	}
}

func TestPurgeReadOnly(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "c", Body: "d", Type: TypeStatus})

	msgs, _ := store.Check("agent-1", CheckOpts{})
	store.MarkRead(msgs[0].ID)

	count, err := store.Purge(PurgeOpts{ReadOnly: true})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 purged, got %d", count)
	}

	remaining, _ := store.List(ListOpts{})
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(remaining))
	}
}

func TestPurgeBefore(t *testing.T) {
	store := newTestStore(t)

	// Send a message with explicit old timestamp.
	old := &MailMessage{
		From: "coord", To: "agent-1", Subject: "old", Body: "old",
		Type: TypeStatus, CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	store.Send(old)

	// Send a recent message.
	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "new", Body: "new", Type: TypeStatus})

	count, err := store.Purge(PurgeOpts{Before: time.Now().Add(-1 * time.Hour)})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 purged, got %d", count)
	}

	remaining, _ := store.List(ListOpts{})
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
	if remaining[0].Subject != "new" {
		t.Errorf("expected 'new' to remain, got %q", remaining[0].Subject)
	}
}

func TestPurgeNoneMatching(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "a", Body: "b", Type: TypeStatus})

	count, err := store.Purge(PurgeOpts{Agent: "nonexistent"})
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 purged, got %d", count)
	}
}

func TestBroadcastExpansion(t *testing.T) {
	resolver := AgentResolverFunc(func(addr string) ([]string, error) {
		switch addr {
		case BroadcastAll:
			return []string{"coordinator", "builder-1", "builder-2", "scout-1"}, nil
		case BroadcastBuilders:
			return []string{"builder-1", "builder-2"}, nil
		case BroadcastScouts:
			return []string{"scout-1"}, nil
		default:
			return nil, nil
		}
	})

	store := newTestStoreWithResolver(t, resolver)

	err := store.Send(&MailMessage{
		From: "coordinator", To: BroadcastBuilders,
		Subject: "build notice", Body: "start building", Type: TypeDispatch,
	})
	if err != nil {
		t.Fatalf("Send broadcast: %v", err)
	}

	// builder-1 should see it.
	msgs, err := store.Check("builder-1", CheckOpts{})
	if err != nil {
		t.Fatalf("Check builder-1: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for builder-1, got %d", len(msgs))
	}

	// builder-2 should see it.
	msgs, err = store.Check("builder-2", CheckOpts{})
	if err != nil {
		t.Fatalf("Check builder-2: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for builder-2, got %d", len(msgs))
	}

	// scout-1 should NOT see it (broadcast was @builders only).
	msgs, err = store.Check("scout-1", CheckOpts{})
	if err != nil {
		t.Fatalf("Check scout-1: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for scout-1, got %d", len(msgs))
	}
}

func TestBroadcastAll(t *testing.T) {
	resolver := AgentResolverFunc(func(addr string) ([]string, error) {
		if addr == BroadcastAll {
			return []string{"coord", "builder-1", "scout-1"}, nil
		}
		return nil, nil
	})

	store := newTestStoreWithResolver(t, resolver)

	store.Send(&MailMessage{
		From: "system", To: BroadcastAll,
		Subject: "announcement", Body: "system update", Type: TypeStatus,
	})

	for _, agent := range []string{"coord", "builder-1", "scout-1"} {
		msgs, err := store.Check(agent, CheckOpts{})
		if err != nil {
			t.Fatalf("Check %s: %v", agent, err)
		}
		if len(msgs) != 1 {
			t.Errorf("expected 1 message for %s, got %d", agent, len(msgs))
		}
	}
}

func TestBroadcastWithoutResolver(t *testing.T) {
	// Without a resolver, broadcast address is stored as-is.
	store := newTestStore(t)

	store.Send(&MailMessage{
		From: "coord", To: BroadcastAll,
		Subject: "test", Body: "body", Type: TypeStatus,
	})

	// The message is stored with to_agent = "@all".
	msgs, _ := store.List(ListOpts{Agent: BroadcastAll})
	if len(msgs) != 1 {
		t.Errorf("expected 1 message with to_agent=@all, got %d", len(msgs))
	}
}

func TestSendWithPayload(t *testing.T) {
	store := newTestStore(t)

	payload := []byte(`{"task_id":"task-42","branch":"feat/auth"}`)
	store.Send(&MailMessage{
		From: "coord", To: "builder-1", Subject: "work",
		Body: "build it", Type: TypeDispatch, Payload: payload,
	})

	msgs, _ := store.Check("builder-1", CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if string(msgs[0].Payload) != string(payload) {
		t.Errorf("expected payload %s, got %s", payload, msgs[0].Payload)
	}
}

func TestIsBroadcast(t *testing.T) {
	tt := []struct {
		addr string
		want bool
	}{
		{BroadcastAll, true},
		{BroadcastBuilders, true},
		{BroadcastScouts, true},
		{BroadcastReviewers, true},
		{BroadcastLeads, true},
		{BroadcastWorkers, true},
		{"builder-1", false},
		{"coordinator", false},
		{"@unknown", false},
		{"", false},
	}

	for _, tc := range tt {
		got := IsBroadcast(tc.addr)
		if got != tc.want {
			t.Errorf("IsBroadcast(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestPriorityWeight(t *testing.T) {
	tt := []struct {
		p    Priority
		want int
	}{
		{PriorityLow, 0},
		{PriorityNormal, 1},
		{PriorityHigh, 2},
		{PriorityUrgent, 3},
		{Priority("unknown"), 1}, // default to normal
	}

	for _, tc := range tt {
		got := tc.p.Weight()
		if got != tc.want {
			t.Errorf("Priority(%q).Weight() = %d, want %d", tc.p, got, tc.want)
		}
	}
}

func TestCheckDifferentAgents(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "for-1", Body: "body", Type: TypeStatus})
	store.Send(&MailMessage{From: "coord", To: "agent-2", Subject: "for-2", Body: "body", Type: TypeStatus})

	msgs1, _ := store.Check("agent-1", CheckOpts{})
	if len(msgs1) != 1 {
		t.Errorf("expected 1 for agent-1, got %d", len(msgs1))
	}

	msgs2, _ := store.Check("agent-2", CheckOpts{})
	if len(msgs2) != 1 {
		t.Errorf("expected 1 for agent-2, got %d", len(msgs2))
	}

	msgs3, _ := store.Check("agent-3", CheckOpts{})
	if len(msgs3) != 0 {
		t.Errorf("expected 0 for agent-3, got %d", len(msgs3))
	}
}

func TestListWithLimit(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 10; i++ {
		store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "msg", Body: "body", Type: TypeStatus})
	}

	msgs, err := store.List(ListOpts{Limit: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3, got %d", len(msgs))
	}
}

func TestSendDefaultPriority(t *testing.T) {
	store := newTestStore(t)

	store.Send(&MailMessage{From: "coord", To: "agent-1", Subject: "test", Body: "body", Type: TypeStatus})

	msgs, _ := store.Check("agent-1", CheckOpts{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1, got %d", len(msgs))
	}
	if msgs[0].Priority != PriorityNormal {
		t.Errorf("expected normal priority, got %q", msgs[0].Priority)
	}
}

func TestMessageTypes(t *testing.T) {
	store := newTestStore(t)

	types := []MessageType{
		TypeStatus, TypeQuestion, TypeResult, TypeError,
		TypeWorkerDone, TypeMergeReady, TypeMerged, TypeMergeFailed,
		TypeEscalation, TypeHealthCheck, TypeDispatch, TypeAssign,
	}

	for _, mt := range types {
		err := store.Send(&MailMessage{
			From: "coord", To: "agent-1", Subject: string(mt),
			Body: "body", Type: mt,
		})
		if err != nil {
			t.Errorf("Send type %q: %v", mt, err)
		}
	}

	msgs, err := store.List(ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(msgs) != len(types) {
		t.Errorf("expected %d messages, got %d", len(types), len(msgs))
	}
}
