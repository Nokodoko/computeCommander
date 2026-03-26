package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/noko/computecommander/internal/linkedin"
)

// LinkedInCmd returns the "linkedin" command group for LinkedIn post generation.
func LinkedInCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "linkedin",
		Short:   "LinkedIn post generator",
		Long:    "Generate, manage, and review LinkedIn posts from project insights.",
		GroupID: "CONTENT",
	}

	cmd.AddCommand(linkedinGenerateCmd(app))
	cmd.AddCommand(linkedinPreviewCmd(app))
	cmd.AddCommand(linkedinApproveCmd(app))
	cmd.AddCommand(linkedinRejectCmd(app))
	cmd.AddCommand(linkedinFeedbackCmd(app))
	cmd.AddCommand(linkedinHistoryCmd(app))
	cmd.AddCommand(linkedinTopicsCmd(app))
	cmd.AddCommand(linkedinStatsCmd(app))

	return cmd
}

func newGenerator(app *App) *linkedin.Generator {
	cfg := linkedin.DefaultConfig()
	return linkedin.NewGenerator(app.DB, cfg)
}

func linkedinGenerateCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate a new LinkedIn post",
		Long:  "Scan projects, select a topic, generate content via Claude, and prepare for review.",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := newGenerator(app)

			result, err := gen.Generate()
			if err != nil {
				return fmt.Errorf("generate: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Post generated successfully!\n")
			fmt.Printf("  ID:      %d\n", result.Post.ID)
			fmt.Printf("  Title:   %s\n", result.Post.Title)
			fmt.Printf("  Topic:   %s\n", result.Post.Topic)
			fmt.Printf("  Project: %s\n", result.Post.SourceProject)
			fmt.Printf("  Status:  %s\n", result.Post.Status)

			if len(result.Warnings) > 0 {
				fmt.Printf("\nWarnings:\n")
				for _, w := range result.Warnings {
					fmt.Printf("  - %s\n", w)
				}
			}

			fmt.Printf("\nNext steps:\n")
			fmt.Printf("  Review:  check your email at %s\n", linkedin.DefaultConfig().RecipientEmail)
			fmt.Printf("  Approve: cc linkedin approve %d\n", result.Post.ID)
			fmt.Printf("  Reject:  cc linkedin reject %d\n", result.Post.ID)

			return nil
		},
	}
}

func linkedinPreviewCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "preview",
		Short: "Preview the next scheduled post topic",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := newGenerator(app)

			topic, trends, err := gen.Preview()
			if err != nil {
				return fmt.Errorf("preview: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				data, _ := json.MarshalIndent(map[string]any{
					"topic":  topic,
					"trends": trends,
				}, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("Next Topic:\n")
			fmt.Printf("  Topic:    %s\n", topic.Topic)
			fmt.Printf("  Project:  %s\n", topic.SourceProject)
			fmt.Printf("  Priority: %d\n", topic.Priority)
			fmt.Printf("  Rating:   %.1f\n", topic.AvgRating)

			if len(trends) > 0 {
				fmt.Printf("\nTrending AI Topics:\n")
				for _, t := range trends {
					fmt.Printf("  - %s (%s)\n", t.Title, t.Source)
				}
			}

			return nil
		},
	}
}

func linkedinApproveCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a pending post for publishing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid post ID: %w", err)
			}

			gen := newGenerator(app)
			store := gen.PostStore()

			post, err := store.Get(id)
			if err != nil {
				return fmt.Errorf("get post %d: %w", id, err)
			}

			if post.Status != linkedin.StatusPendingReview {
				return fmt.Errorf("post %d has status %q, expected %q", id, post.Status, linkedin.StatusPendingReview)
			}

			if err := store.Approve(id); err != nil {
				return fmt.Errorf("approve post %d: %w", id, err)
			}

			fmt.Printf("Post %d approved: %s\n", id, post.Title)
			fmt.Printf("Copy the plain text from your review email and paste it to LinkedIn.\n")
			return nil
		},
	}
}

func linkedinRejectCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a pending post",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid post ID: %w", err)
			}

			gen := newGenerator(app)
			store := gen.PostStore()

			if err := store.Reject(id); err != nil {
				return fmt.Errorf("reject post %d: %w", id, err)
			}

			fmt.Printf("Post %d rejected.\n", id)
			return nil
		},
	}
}

func linkedinFeedbackCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback <id> <rating>",
		Short: "Rate a past post (1-5)",
		Long:  "Provide feedback on a posted or approved post. Rating: 1 (poor) to 5 (excellent).",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid post ID: %w", err)
			}

			rating, err := strconv.Atoi(args[1])
			if err != nil || rating < 1 || rating > 5 {
				return fmt.Errorf("rating must be 1-5")
			}

			notes, _ := cmd.Flags().GetString("notes")

			gen := newGenerator(app)
			store := gen.PostStore()

			if err := store.SetFeedback(id, rating, notes); err != nil {
				return fmt.Errorf("set feedback: %w", err)
			}

			// Update topic ratings.
			post, err := store.Get(id)
			if err == nil && post.SourceProject != "" {
				gen.TopicStore().UpdateRating(post.SourceProject)
			}

			fmt.Printf("Feedback recorded for post %d: %d/5\n", id, rating)
			return nil
		},
	}
	cmd.Flags().String("notes", "", "Optional feedback notes")
	return cmd
}

func linkedinHistoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List generated posts",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := newGenerator(app)
			store := gen.PostStore()

			limit, _ := cmd.Flags().GetInt("limit")
			statusFilter, _ := cmd.Flags().GetString("status")

			posts, err := store.List(linkedin.PostStatus(statusFilter), limit)
			if err != nil {
				return fmt.Errorf("list posts: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				data, _ := json.MarshalIndent(posts, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(posts) == 0 {
				fmt.Println("No posts found.")
				return nil
			}

			fmt.Printf("%-4s %-12s %-40s %-12s %-6s\n", "ID", "Status", "Title", "Project", "Rating")
			fmt.Println(strings.Repeat("-", 80))
			for _, p := range posts {
				title := p.Title
				if len(title) > 38 {
					title = title[:38] + ".."
				}
				rating := "-"
				if p.FeedbackRating != nil {
					rating = fmt.Sprintf("%d/5", *p.FeedbackRating)
				}
				fmt.Printf("%-4d %-12s %-40s %-12s %-6s\n",
					p.ID, p.Status, title, p.SourceProject, rating)
			}

			return nil
		},
	}
	cmd.Flags().Int("limit", 20, "Maximum number of posts to show")
	cmd.Flags().String("status", "", "Filter by status (draft, pending_review, approved, posted, rejected)")
	return cmd
}

func linkedinTopicsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "topics",
		Short: "Show topic queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := newGenerator(app)
			store := gen.TopicStore()

			// Seed defaults if needed.
			if err := store.SeedDefaults(); err != nil {
				return fmt.Errorf("seed defaults: %w", err)
			}

			unusedOnly, _ := cmd.Flags().GetBool("unused")
			topics, err := store.List(unusedOnly)
			if err != nil {
				return fmt.Errorf("list topics: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				data, _ := json.MarshalIndent(topics, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(topics) == 0 {
				fmt.Println("No topics in queue.")
				return nil
			}

			fmt.Printf("%-4s %-4s %-50s %-15s %-5s %-6s\n", "ID", "Pri", "Topic", "Project", "Used", "Rating")
			fmt.Println(strings.Repeat("-", 90))
			for _, t := range topics {
				topic := t.Topic
				if len(topic) > 48 {
					topic = topic[:48] + ".."
				}
				used := " "
				if t.Used {
					used = "Y"
				}
				fmt.Printf("%-4d %-4d %-50s %-15s %-5s %-6.1f\n",
					t.ID, t.Priority, topic, t.SourceProject, used, t.AvgRating)
			}

			return nil
		},
	}
	cmd.Flags().Bool("unused", false, "Show only unused topics")
	return cmd
}

func linkedinStatsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show engagement trends and monthly summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := newGenerator(app)
			store := gen.PostStore()

			stats, err := store.Stats()
			if err != nil {
				return fmt.Errorf("get stats: %w", err)
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				data, _ := json.MarshalIndent(stats, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("LinkedIn Post Statistics\n")
			fmt.Printf("========================\n\n")
			fmt.Printf("Posts:\n")
			fmt.Printf("  Total:     %d\n", stats.TotalPosts)
			fmt.Printf("  Approved:  %d\n", stats.ApprovedCount)
			fmt.Printf("  Rejected:  %d\n", stats.RejectedCount)
			fmt.Printf("  Posted:    %d\n", stats.PostedCount)
			if stats.TotalPosts > 0 {
				approvalRate := float64(stats.ApprovedCount+stats.PostedCount) / float64(stats.TotalPosts) * 100
				fmt.Printf("  Approval:  %.0f%%\n", approvalRate)
			}

			fmt.Printf("\nQuality:\n")
			fmt.Printf("  Avg Rating: %.1f/5\n", stats.AvgRating)
			if stats.TopProject != "" {
				fmt.Printf("  Top Project: %s\n", stats.TopProject)
			}

			fmt.Printf("\nEngagement:\n")
			fmt.Printf("  Likes:    %d\n", stats.TotalLikes)
			fmt.Printf("  Comments: %d\n", stats.TotalComments)
			fmt.Printf("  Reposts:  %d\n", stats.TotalReposts)

			return nil
		},
	}
}
