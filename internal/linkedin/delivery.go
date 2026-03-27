package linkedin

import (
	"fmt"
	"os/exec"
)

// Delivery handles sending post review emails and desktop notifications.
// Email delivery is designed to work within a Claude Code session where
// Gmail MCP tools are available. The actual email sending is done by
// claude -p invoking Gmail MCP, or by the CLI printing the email content
// for manual use.
type Delivery struct {
	recipientEmail string
	renderer       *Renderer
}

// NewDelivery creates a Delivery configured to send to the given email.
func NewDelivery(recipientEmail string) *Delivery {
	return &Delivery{
		recipientEmail: recipientEmail,
		renderer:       NewRenderer(),
	}
}

// DeliveryResult holds the output of a delivery attempt.
type DeliveryResult struct {
	EmailSubject  string `json:"email_subject"`
	EmailBody     string `json:"email_body"`
	RecipientEmail string `json:"recipient_email"`
	Notified      bool   `json:"notified"`
}

// Prepare renders the post into email format ready for delivery.
// This does not send the email -- the caller (typically a Claude Code session
// via claude -p) uses Gmail MCP tools to actually deliver it.
func (d *Delivery) Prepare(p *Post) (*DeliveryResult, error) {
	subject := d.renderer.RenderSubject(p)
	body, err := d.renderer.RenderEmail(p)
	if err != nil {
		return nil, fmt.Errorf("render email: %w", err)
	}

	result := &DeliveryResult{
		EmailSubject:   subject,
		EmailBody:      body,
		RecipientEmail: d.recipientEmail,
	}

	return result, nil
}

// Notify sends a desktop notification via notify-send (dunst).
func (d *Delivery) Notify(title, body string) error {
	cmd := exec.Command("notify-send",
		"--urgency=normal",
		"--app-name=ComputeCommander",
		"--icon=linkedin",
		title,
		body,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notify-send: %w", err)
	}
	return nil
}

// NotifyPostReady sends a dunst notification that a post is ready for review.
func (d *Delivery) NotifyPostReady(p *Post) error {
	return d.Notify(
		"LinkedIn Post Ready for Review",
		fmt.Sprintf("%s\nRun: cc linkedin approve %d", p.Title, p.ID),
	)
}
