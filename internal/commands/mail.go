package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/mail"
)

// MailCmd returns the "mail" command group for inter-agent messaging.
func MailCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mail",
		Short:   "Inter-agent messaging",
		Long:    "Send, check, list, and manage inter-agent mail messages.",
		GroupID: "MESSAGING",
	}

	cmd.AddCommand(mailSendCmd(app))
	cmd.AddCommand(mailCheckCmd(app))
	cmd.AddCommand(mailListCmd(app))
	cmd.AddCommand(mailReadCmd(app))
	cmd.AddCommand(mailReplyCmd(app))
	cmd.AddCommand(mailPurgeCmd(app))

	return cmd
}

func mailSendCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			subject, _ := cmd.Flags().GetString("subject")
			body, _ := cmd.Flags().GetString("body")
			msgType, _ := cmd.Flags().GetString("type")
			priority, _ := cmd.Flags().GetString("priority")

			if to == "" {
				return fmt.Errorf("--to is required")
			}
			if body == "" && len(args) > 0 {
				body = strings.Join(args, " ")
			}

			msg := &mail.MailMessage{
				From:     from,
				To:       to,
				Subject:  subject,
				Body:     body,
				Type:     mail.MessageType(msgType),
				Priority: mail.Priority(priority),
			}

			if err := app.MailStore.Send(msg); err != nil {
				return fmt.Errorf("send mail: %w", err)
			}

			fmt.Printf("Message sent to %s (id: %s)\n", to, msg.ID)
			return nil
		},
	}

	cmd.Flags().String("from", "cli", "Sender name")
	cmd.Flags().String("to", "", "Recipient agent name (required)")
	cmd.Flags().String("subject", "", "Message subject")
	cmd.Flags().String("body", "", "Message body")
	cmd.Flags().String("type", "status", "Message type")
	cmd.Flags().String("priority", "normal", "Priority (low, normal, high, urgent)")

	return cmd
}

func mailCheckCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <agent>",
		Short: "Check unread messages for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := args[0]
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			limit, _ := cmd.Flags().GetInt("limit")

			msgs, err := app.MailStore.Check(agent, mail.CheckOpts{Limit: limit})
			if err != nil {
				return fmt.Errorf("check mail: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(msgs)
			}

			if len(msgs) == 0 {
				fmt.Printf("No unread messages for %s\n", agent)
				return nil
			}

			for _, m := range msgs {
				fmt.Printf("[%s] %s -> %s: %s\n", m.Type, m.From, m.To, m.Subject)
			}
			fmt.Printf("\n%d unread message(s)\n", len(msgs))
			return nil
		},
	}

	cmd.Flags().Int("limit", 0, "Max messages to return (0 = no limit)")

	return cmd
}

func mailListCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			paneMode, _ := cmd.Flags().GetBool("pane")
			agent, _ := cmd.Flags().GetString("agent")
			from, _ := cmd.Flags().GetString("from")
			unread, _ := cmd.Flags().GetBool("unread")
			limit, _ := cmd.Flags().GetInt("limit")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			listOpts := mail.ListOpts{
				Agent:  agent,
				From:   from,
				Unread: unread,
				Limit:  limit,
			}

			if paneMode {
				return runMailListPane(cmd, app, listOpts)
			}

			msgs, err := app.MailStore.List(listOpts)
			if err != nil {
				return fmt.Errorf("list mail: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(msgs)
			}

			if len(msgs) == 0 {
				fmt.Println("No messages found.")
				return nil
			}

			for _, m := range msgs {
				readMark := " "
				if !m.Read {
					readMark = "*"
				}
				fmt.Printf("%s [%s] %s -> %s: %s (%s)\n",
					readMark, m.Type, m.From, m.To, m.Subject, m.ID)
			}
			fmt.Printf("\n%d message(s)\n", len(msgs))
			return nil
		},
	}

	cmd.Flags().String("agent", "", "Filter by recipient agent")
	cmd.Flags().String("from", "", "Filter by sender")
	cmd.Flags().Bool("unread", false, "Show only unread messages")
	cmd.Flags().Int("limit", 0, "Max messages (0 = no limit)")
	cmd.Flags().Bool("pane", false, "Run in long-lived pane mode (for zellij dashboard)")

	return cmd
}

// runMailListPane runs mail list in long-lived pane mode, refreshing periodically.
func runMailListPane(cmd *cobra.Command, app *App, opts mail.ListOpts) error {
	ctx := cmd.Context()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	render := func() {
		clearScreen()
		msgs, err := app.MailStore.List(opts)
		if err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			return
		}

		if len(msgs) == 0 {
			fmt.Println("\033[2mNo messages.\033[0m")
			return
		}

		for _, m := range msgs {
			readMark := " "
			if !m.Read {
				readMark = "\033[33m*\033[0m"
			}
			typeColor := "\033[36m"
			switch m.Type {
			case mail.TypeError, mail.TypeMergeFailed:
				typeColor = "\033[31m"
			case mail.TypeEscalation:
				typeColor = "\033[35m"
			case mail.TypeDispatch, mail.TypeAssign:
				typeColor = "\033[33m"
			}
			fmt.Printf("%s %s%-8s\033[0m \033[1m%-10s\033[0m -> %-10s %s\n",
				readMark,
				typeColor,
				truncate(string(m.Type), 8),
				truncate(m.From, 10),
				truncate(m.To, 10),
				truncate(m.Subject, 30),
			)
		}
		fmt.Printf("\n\033[2m%d message(s)\033[0m\n", len(msgs))
	}

	// Initial render.
	render()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			render()
		}
	}
}

func mailReadCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "read <message-id>",
		Short: "Mark a message as read",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.MailStore.MarkRead(args[0]); err != nil {
				return fmt.Errorf("mark read: %w", err)
			}
			fmt.Printf("Marked %s as read\n", args[0])
			return nil
		},
	}
}

func mailReplyCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reply <message-id>",
		Short: "Reply to a message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, _ := cmd.Flags().GetString("body")
			if body == "" {
				return fmt.Errorf("--body is required")
			}
			if err := app.MailStore.Reply(args[0], body); err != nil {
				return fmt.Errorf("reply: %w", err)
			}
			fmt.Printf("Reply sent to message %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().String("body", "", "Reply body text (required)")

	return cmd
}

func mailPurgeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Delete messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			agent, _ := cmd.Flags().GetString("agent")
			readOnly, _ := cmd.Flags().GetBool("read-only")

			opts := mail.PurgeOpts{
				Agent:    agent,
				ReadOnly: readOnly,
			}

			count, err := app.MailStore.Purge(opts)
			if err != nil {
				return fmt.Errorf("purge mail: %w", err)
			}

			fmt.Printf("Purged %d message(s)\n", count)
			return nil
		},
	}

	cmd.Flags().String("agent", "", "Purge only messages for this agent")
	cmd.Flags().Bool("read-only", false, "Purge only read messages")

	return cmd
}
