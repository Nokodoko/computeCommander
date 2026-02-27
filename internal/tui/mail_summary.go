package tui

import (
	"fmt"
	"strings"

	"github.com/noko/computecommander/internal/mail"
)

// MailSummary renders unread mail count and recent message previews.
type MailSummary struct {
	store       mail.MailStore
	unread      int
	recent      []*mail.MailMessage
	previewMax  int
	theme       *Theme
}

// NewMailSummary constructs a MailSummary.
func NewMailSummary(store mail.MailStore, theme *Theme) *MailSummary {
	return &MailSummary{
		store:      store,
		previewMax: 5,
		theme:      theme,
	}
}

// Refresh fetches unread count and recent messages.
func (m *MailSummary) Refresh() error {
	msgs, err := m.store.List(mail.ListOpts{
		Unread: true,
		Limit:  100,
	})
	if err != nil {
		return fmt.Errorf("mail summary refresh unread: %w", err)
	}
	m.unread = len(msgs)

	recent, err := m.store.List(mail.ListOpts{
		Limit: m.previewMax,
	})
	if err != nil {
		return fmt.Errorf("mail summary refresh recent: %w", err)
	}
	m.recent = recent

	return nil
}

// UnreadCount returns the number of unread messages.
func (m *MailSummary) UnreadCount() int {
	return m.unread
}

// View renders the mail summary as a string.
func (m *MailSummary) View() string {
	var b strings.Builder

	title := fmt.Sprintf("Mail (%d unread)", m.unread)
	b.WriteString(m.theme.Title.Render(title))
	b.WriteString("\n")

	if len(m.recent) == 0 {
		b.WriteString(m.theme.Subtitle.Render("  No messages"))
		return b.String()
	}

	for _, msg := range m.recent {
		readMark := " "
		if !msg.Read {
			readMark = "*"
		}
		line := fmt.Sprintf(" %s [%s] %s -> %s: %s",
			readMark,
			string(msg.Type),
			truncate(msg.From, 10),
			truncate(msg.To, 10),
			truncate(msg.Subject, 30),
		)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// CompactView returns a one-line summary for the status bar.
func (m *MailSummary) CompactView() string {
	return fmt.Sprintf("Mail: %d unread", m.unread)
}
