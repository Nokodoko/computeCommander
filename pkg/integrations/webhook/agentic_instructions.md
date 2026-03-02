# pkg/integrations/webhook/ -- Webhook Event Dispatcher

## Purpose
Dispatches ComputeCommander events to registered webhook endpoints via HTTP POST. Supports event-type filtering on subscriptions, concurrent-safe subscription management, and per-endpoint delivery with timeout.

## Technology
- Go 1.25
- `net/http` for HTTP POST delivery
- `sync.RWMutex` for concurrent-safe subscription management
- `encoding/json` for event serialization

## Contents
| File | Description |
|------|-------------|
| `webhook.go` | `WebhookDispatcher` struct, `Register()`, `Unregister()`, `Subscriptions()`, `Dispatch()`, `Event`/`Subscription`/`DispatchResult` types |

## Key Functions

| Function | Signature | Returns | Description |
|----------|-----------|---------|-------------|
| `NewWebhookDispatcher` | `func NewWebhookDispatcher() *WebhookDispatcher` | `*WebhookDispatcher` | Creates dispatcher with 10s HTTP timeout |
| `Register` | `func (d *WebhookDispatcher) Register(url string, events []string) error` | `error` | Adds subscription; rejects duplicates |
| `Unregister` | `func (d *WebhookDispatcher) Unregister(url string) error` | `error` | Removes subscription by URL |
| `Subscriptions` | `func (d *WebhookDispatcher) Subscriptions() []Subscription` | `[]Subscription` | Returns a copy of all subscriptions |
| `Dispatch` | `func (d *WebhookDispatcher) Dispatch(event Event) []DispatchResult` | `[]DispatchResult` | Sends event to all matching subscribers, returns results |

## Data Types

### Event (struct)
Fields: Type (string, e.g., "agent.spawned", "merge.completed"), Payload (any), Timestamp

### Subscription (struct)
Fields: URL, Events ([]string, empty = all events)

### DispatchResult (struct)
Fields: URL, StatusCode, Error

### WebhookDispatcher (struct)
Fields: mu (RWMutex), subscriptions, client (*http.Client)

## Logging
- No structured logging; errors returned in DispatchResult

## CRUD Entry Points
- **Create**: `Register()` adds a new subscription
- **Read**: `Subscriptions()` lists all
- **Update**: N/A (re-register to change)
- **Delete**: `Unregister()` removes a subscription
- **Execute**: `Dispatch()` sends events to matching subscribers

## Style Guide
- Thread-safe: RWMutex protects subscription list
- Copy-on-read: `Subscriptions()` and `Dispatch()` work on copies
- User-Agent header: `ComputeCommander-Webhook/1.0`
- Event matching: empty events list means "subscribe to everything"
- HTTP timeout: 10s per delivery attempt

**Representative snippet (from `webhook.go`):**
```go
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
```
