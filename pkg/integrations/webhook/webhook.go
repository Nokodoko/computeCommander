// Package webhook provides an event dispatcher that delivers ComputeCommander
// events to registered webhook endpoints via HTTP POST.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event represents a webhook event to dispatch.
type Event struct {
	// Type identifies the event (e.g., "agent.spawned", "merge.completed").
	Type string `json:"type"`

	// Payload carries the event-specific data.
	Payload any `json:"payload"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// Subscription is a registered webhook endpoint.
type Subscription struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// DispatchResult holds the outcome of a single webhook delivery attempt.
type DispatchResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
	Error      error  `json:"error,omitempty"`
}

// WebhookDispatcher manages webhook subscriptions and dispatches events.
type WebhookDispatcher struct {
	mu            sync.RWMutex
	subscriptions []Subscription
	client        *http.Client
}

// NewWebhookDispatcher creates a new dispatcher with default settings.
func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register adds a webhook subscription for the given URL and event types.
// If events is empty, the subscription receives all events.
func (d *WebhookDispatcher) Register(url string, events []string) error {
	if url == "" {
		return fmt.Errorf("webhook: URL is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check for duplicates.
	for _, sub := range d.subscriptions {
		if sub.URL == url {
			return fmt.Errorf("webhook: URL %q is already registered", url)
		}
	}

	d.subscriptions = append(d.subscriptions, Subscription{
		URL:    url,
		Events: events,
	})

	return nil
}

// Unregister removes a webhook subscription by URL.
func (d *WebhookDispatcher) Unregister(url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, sub := range d.subscriptions {
		if sub.URL == url {
			d.subscriptions = append(d.subscriptions[:i], d.subscriptions[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("webhook: URL %q not found", url)
}

// Subscriptions returns a copy of all registered subscriptions.
func (d *WebhookDispatcher) Subscriptions() []Subscription {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]Subscription, len(d.subscriptions))
	copy(result, d.subscriptions)
	return result
}

// Dispatch sends an event to all matching subscribers.
// It returns results for each delivery attempt.
func (d *WebhookDispatcher) Dispatch(event Event) []DispatchResult {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	d.mu.RLock()
	subs := make([]Subscription, len(d.subscriptions))
	copy(subs, d.subscriptions)
	d.mu.RUnlock()

	var results []DispatchResult
	for _, sub := range subs {
		if !matchesEvent(sub.Events, event.Type) {
			continue
		}

		result := d.deliver(event, sub.URL)
		results = append(results, result)
	}

	return results
}

// deliver sends a single event payload to a webhook URL.
func (d *WebhookDispatcher) deliver(event Event, url string) DispatchResult {
	body, err := json.Marshal(event)
	if err != nil {
		return DispatchResult{URL: url, Error: fmt.Errorf("marshal event: %w", err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return DispatchResult{URL: url, Error: fmt.Errorf("create request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ComputeCommander-Webhook/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return DispatchResult{URL: url, Error: fmt.Errorf("deliver webhook: %w", err)}
	}
	defer resp.Body.Close()

	return DispatchResult{URL: url, StatusCode: resp.StatusCode}
}

// matchesEvent checks if an event type matches a subscription's event filter.
// An empty events list means the subscription receives all events.
func matchesEvent(events []string, eventType string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if e == eventType {
			return true
		}
	}
	return false
}
