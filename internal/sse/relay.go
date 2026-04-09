// Package sse provides an SSE relay client for the OpenBrain memory event stream.
// It subscribes to GET /events/memories and accumulates activity from both lewis and monty hosts.
package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MemoryEvent is an SSE event from the OpenBrain relay at /events/memories.
type MemoryEvent struct {
	ID         string   `json:"id"`
	ItemType   string   `json:"item_type"`
	RawContent string   `json:"raw_content"`
	Entities   Entities `json:"entities"`
	Source     string   `json:"source"`
	CapturedAt string   `json:"captured_at"`

	// Host is derived from entities.tags (e.g. "host:lewis").
	// Populated by the relay client after parsing.
	Host string `json:"-"`
}

// Entities holds tag metadata from the event.
type Entities struct {
	Tags []string `json:"tags"`
}

// ActivityEntry is a single timestamped activity record derived from a MemoryEvent.
type ActivityEntry struct {
	Host       string
	Summary    string
	CapturedAt time.Time
	Tags       []string
}

// HostCounts tracks per-host event counts.
type HostCounts struct {
	Lewis int
	Monty int
	Other int
}

// RelayClient subscribes to the SSE relay and accumulates activity.
// A background goroutine reconnects on disconnect with exponential backoff.
type RelayClient struct {
	url        string
	apiKey     string
	httpClient *http.Client

	mu         sync.Mutex
	events     []ActivityEntry // ring buffer, max maxEvents
	hostCounts HostCounts
	connected  bool
	lastError  string

	cancel context.CancelFunc
}

const maxEvents = 200

// NewRelayClient creates and starts a RelayClient that subscribes to the SSE relay.
func NewRelayClient(relayURL, apiKey string) *RelayClient {
	r := &RelayClient{
		url:    relayURL,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 0, // no timeout — long-lived SSE stream
		},
		events: make([]ActivityEntry, 0, maxEvents),
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	go r.loop(ctx)
	return r
}

// Connected returns whether the relay is currently connected.
func (r *RelayClient) Connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected
}

// LastError returns the last connection error message.
func (r *RelayClient) LastError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastError
}

// RecentEvents returns a copy of the most recent activity entries, newest last.
func (r *RelayClient) RecentEvents(limit int) []ActivityEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > len(r.events) {
		limit = len(r.events)
	}
	start := len(r.events) - limit
	if start < 0 {
		start = 0
	}
	cp := make([]ActivityEntry, len(r.events)-start)
	copy(cp, r.events[start:])
	return cp
}

// Counts returns the cumulative per-host event counts.
func (r *RelayClient) Counts() HostCounts {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hostCounts
}

// Close stops the background subscription goroutine.
func (r *RelayClient) Close() {
	r.cancel()
}

// ── Internal ─────────────────────────────────────────────────────────────────

// loop reconnects to the SSE relay until ctx is cancelled.
func (r *RelayClient) loop(ctx context.Context) {
	backoff := 2 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}

		err := r.subscribe(ctx)
		if ctx.Err() != nil {
			return
		}

		r.mu.Lock()
		r.connected = false
		if err != nil {
			r.lastError = err.Error()
			slog.Warn("sse relay: disconnected", "error", err, "backoff", backoff)
		}
		r.mu.Unlock()

		// Exponential backoff before reconnect.
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// subscribe opens the SSE stream and reads events until error or ctx cancellation.
func (r *RelayClient) subscribe(ctx context.Context) error {
	endpoint := r.url
	if r.apiKey != "" {
		if strings.Contains(endpoint, "?") {
			endpoint += "&api_key=" + r.apiKey
		} else {
			endpoint += "?api_key=" + r.apiKey
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from relay", resp.StatusCode)
	}

	r.mu.Lock()
	r.connected = true
	r.lastError = ""
	r.mu.Unlock()

	slog.Info("sse relay: connected", "url", r.url)

	scanner := bufio.NewScanner(resp.Body)
	var eventType, dataLine string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "": // blank line = event boundary
			if eventType == "memory" && dataLine != "" {
				r.handleEvent(dataLine)
			}
			eventType = ""
			dataLine = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	return fmt.Errorf("stream closed")
}

// handleEvent parses and stores a single memory event.
func (r *RelayClient) handleEvent(data string) {
	var ev MemoryEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		slog.Warn("sse relay: failed to parse event", "error", err)
		return
	}

	// Derive host from tags.
	ev.Host = extractHost(ev.Entities.Tags)

	// Parse timestamp.
	var ts time.Time
	if ev.CapturedAt != "" {
		if t, err := time.Parse(time.RFC3339, ev.CapturedAt); err == nil {
			ts = t
		}
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	entry := ActivityEntry{
		Host:       ev.Host,
		Summary:    ev.RawContent,
		CapturedAt: ts,
		Tags:       ev.Entities.Tags,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, entry)
	// Trim to maxEvents ring buffer.
	if len(r.events) > maxEvents {
		r.events = r.events[len(r.events)-maxEvents:]
	}

	switch ev.Host {
	case "lewis":
		r.hostCounts.Lewis++
	case "monty":
		r.hostCounts.Monty++
	default:
		r.hostCounts.Other++
	}
}

// extractHost finds a "host:<name>" tag in the tags list and returns the host name.
func extractHost(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "host:") {
			return strings.TrimPrefix(tag, "host:")
		}
	}
	return "unknown"
}
