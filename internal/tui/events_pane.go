package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// Event represents a single event in the events pane.
type Event struct {
	Time    time.Time
	Source  string // agent name or "system"
	Type    string // "log", "state", "error", "mail"
	Message string
}

// EventsPane displays a merged system log and agent activity feed.
type EventsPane struct {
	db            db.DB
	events        []Event
	maxEvents     int
	theme         *Theme
	colorResolver AgentColorResolver
	width         int
	height        int
	offset        int // scroll offset
}

// NewEventsPane constructs an EventsPane.
func NewEventsPane(theme *Theme) *EventsPane {
	return &EventsPane{
		maxEvents: 500,
		theme:     theme,
	}
}

// SetDB wires an optional database so the pane can refresh from the events table.
func (ep *EventsPane) SetDB(database db.DB) {
	ep.db = database
}

// SetColorResolver sets the function used to resolve agent colors for source names.
func (ep *EventsPane) SetColorResolver(resolver AgentColorResolver) {
	ep.colorResolver = resolver
}

// Refresh reads the most recent events from the database and replaces the in-memory list.
// It is a no-op when no database has been wired via SetDB.
func (ep *EventsPane) Refresh() error {
	if ep.db == nil {
		return nil
	}

	rows, err := ep.db.Query(context.Background(),
		`SELECT agent_name, session_id, event_type, level, data, created_at
		 FROM events
		 ORDER BY id DESC
		 LIMIT ?`, ep.maxEvents)
	if err != nil {
		return fmt.Errorf("events pane refresh: %w", err)
	}
	defer rows.Close()

	var fetched []Event
	for rows.Next() {
		var agentName, sessionID, eventType, level, data, createdStr string
		if err := rows.Scan(&agentName, &sessionID, &eventType, &level, &data, &createdStr); err != nil {
			return fmt.Errorf("events pane scan: %w", err)
		}

		t, _ := time.Parse("2006-01-02 15:04:05", createdStr)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, createdStr)
		}

		source := agentName
		if source == "" {
			source = sessionID
		}

		// Map event_type to display type.
		displayType := "log"
		switch eventType {
		case "spawn", "session_end", "session_sweep":
			displayType = "state"
		case "error":
			displayType = "error"
		case "tool_end", "tool_start":
			displayType = "log"
		}

		msg := eventType
		if data != "" {
			msg = eventType + " " + data
		}

		fetched = append(fetched, Event{
			Time:    t,
			Source:  source,
			Type:    displayType,
			Message: msg,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("events pane rows: %w", err)
	}

	// Reverse so oldest is first (display is chronological, newest at bottom).
	for i, j := 0, len(fetched)-1; i < j; i, j = i+1, j-1 {
		fetched[i], fetched[j] = fetched[j], fetched[i]
	}

	ep.events = fetched
	// Reset scroll so new events auto-scroll to bottom.
	ep.offset = 0
	return nil
}

// AddEvent appends an event to the list, trimming to maxEvents.
func (ep *EventsPane) AddEvent(e Event) {
	ep.events = append(ep.events, e)
	if len(ep.events) > ep.maxEvents {
		ep.events = ep.events[len(ep.events)-ep.maxEvents:]
	}
}

// Clear removes all events.
func (ep *EventsPane) Clear() {
	ep.events = nil
	ep.offset = 0
}

// ScrollUp moves the view up.
func (ep *EventsPane) ScrollUp() {
	if ep.offset > 0 {
		ep.offset--
	}
}

// ScrollDown moves the view down.
func (ep *EventsPane) ScrollDown() {
	h := ep.height
	if h <= 0 {
		h = 10
	}
	if ep.offset < len(ep.events)-h {
		ep.offset++
	}
}

// SetSize updates display dimensions.
func (ep *EventsPane) SetSize(w, h int) {
	ep.width = w
	ep.height = h
}

// View renders the events list.
func (ep *EventsPane) View() string {
	if len(ep.events) == 0 {
		return ep.theme.Subtitle.Render("  No events")
	}

	h := ep.height
	if h <= 0 {
		h = 10
	}
	w := ep.width
	if w <= 0 {
		w = 60
	}

	// Auto-scroll to bottom if not manually scrolled.
	if ep.offset == 0 && len(ep.events) > h {
		ep.offset = len(ep.events) - h
	}

	end := ep.offset + h
	if end > len(ep.events) {
		end = len(ep.events)
	}

	var lines []string
	for i := ep.offset; i < end; i++ {
		e := ep.events[i]
		ts := e.Time.Format("15:04:05")
		sourceName := truncate(e.Source, 12)
		if ep.colorResolver != nil {
			if hex := ep.colorResolver(e.Source); hex != "" {
				sourceName = AgentColorStyle(hex).Render(sourceName)
			}
		}
		source := fmt.Sprintf("[%-12s]", sourceName)

		var typeStr string
		switch e.Type {
		case "log":
			typeStr = ep.theme.EventLog.Render("log:  ")
		case "state":
			typeStr = ep.theme.EventState.Render("state:")
		case "error":
			typeStr = ep.theme.EventError.Render("error:")
		case "mail":
			typeStr = ep.theme.EventMail.Render("mail: ")
		default:
			typeStr = e.Type + ":"
		}

		line := fmt.Sprintf("%s %s %s %s", ts, source, typeStr, e.Message)
		if len(line) > w {
			line = line[:w]
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
