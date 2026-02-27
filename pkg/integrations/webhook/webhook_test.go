package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegister(t *testing.T) {
	d := NewWebhookDispatcher()

	err := d.Register("https://example.com/hook", []string{"agent.spawned"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	subs := d.Subscriptions()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	if subs[0].URL != "https://example.com/hook" {
		t.Errorf("expected URL 'https://example.com/hook', got %q", subs[0].URL)
	}

	if len(subs[0].Events) != 1 || subs[0].Events[0] != "agent.spawned" {
		t.Errorf("expected events [agent.spawned], got %v", subs[0].Events)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	d := NewWebhookDispatcher()

	err := d.Register("https://example.com/hook", nil)
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	err = d.Register("https://example.com/hook", nil)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegisterEmptyURL(t *testing.T) {
	d := NewWebhookDispatcher()

	err := d.Register("", nil)
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestUnregister(t *testing.T) {
	d := NewWebhookDispatcher()

	_ = d.Register("https://example.com/hook", nil)

	err := d.Unregister("https://example.com/hook")
	if err != nil {
		t.Fatalf("unregister: %v", err)
	}

	if len(d.Subscriptions()) != 0 {
		t.Error("expected 0 subscriptions after unregister")
	}
}

func TestUnregisterNotFound(t *testing.T) {
	d := NewWebhookDispatcher()

	err := d.Unregister("https://example.com/nope")
	if err == nil {
		t.Error("expected error for unregistering non-existent URL")
	}
}

func TestDispatch(t *testing.T) {
	var received Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
		}

		if r.Header.Get("User-Agent") != "ComputeCommander-Webhook/1.0" {
			t.Errorf("expected User-Agent header, got %q", r.Header.Get("User-Agent"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher()
	_ = d.Register(srv.URL, []string{"agent.spawned"})

	results := d.Dispatch(Event{
		Type:      "agent.spawned",
		Payload:   map[string]string{"name": "builder-1"},
		Timestamp: time.Now(),
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Error != nil {
		t.Errorf("expected no error, got %v", results[0].Error)
	}
	if results[0].StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", results[0].StatusCode)
	}

	if received.Type != "agent.spawned" {
		t.Errorf("expected event type 'agent.spawned', got %q", received.Type)
	}
}

func TestDispatchFilteredOut(t *testing.T) {
	d := NewWebhookDispatcher()
	_ = d.Register("https://example.com/hook", []string{"merge.completed"})

	results := d.Dispatch(Event{
		Type:    "agent.spawned",
		Payload: nil,
	})

	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching event, got %d", len(results))
	}
}

func TestDispatchAllEvents(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher()
	_ = d.Register(srv.URL, nil) // nil means all events

	d.Dispatch(Event{Type: "anything.goes", Payload: nil})

	if !called {
		t.Error("expected webhook to be called for catch-all subscription")
	}
}

func TestMatchesEvent(t *testing.T) {
	tests := []struct {
		events    []string
		eventType string
		want      bool
	}{
		{nil, "agent.spawned", true},
		{[]string{}, "agent.spawned", true},
		{[]string{"agent.spawned"}, "agent.spawned", true},
		{[]string{"merge.completed"}, "agent.spawned", false},
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
	}

	for _, tt := range tests {
		got := matchesEvent(tt.events, tt.eventType)
		if got != tt.want {
			t.Errorf("matchesEvent(%v, %q) = %v, want %v", tt.events, tt.eventType, got, tt.want)
		}
	}
}
