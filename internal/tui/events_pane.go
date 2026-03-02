package tui

import (
	"fmt"
	"strings"
	"time"
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
	events    []Event
	maxEvents int
	theme     *Theme
	width     int
	height    int
	offset    int // scroll offset
}

// NewEventsPane constructs an EventsPane.
func NewEventsPane(theme *Theme) *EventsPane {
	return &EventsPane{
		maxEvents: 500,
		theme:     theme,
	}
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
		source := fmt.Sprintf("[%-12s]", truncate(e.Source, 12))

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
