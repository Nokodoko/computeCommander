package linkedin

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/noko/computecommander/internal/platform/db"
)

// Generator orchestrates the LinkedIn post generation pipeline:
// scan projects -> select topic -> build prompt -> generate content -> deliver.
type Generator struct {
	cfg            Config
	postStore      *PostStore
	topicStore     *TopicStore
	scanner        *Scanner
	trendAnalyzer  *TrendAnalyzer
	contentBuilder *ContentBuilder
	delivery       *Delivery
}

// NewGenerator creates a fully wired Generator with all dependencies.
func NewGenerator(database db.DB, cfg Config) *Generator {
	return &Generator{
		cfg:            cfg,
		postStore:      NewPostStore(database),
		topicStore:     NewTopicStore(database),
		scanner:        NewScanner(cfg),
		trendAnalyzer:  NewTrendAnalyzer(nil),
		contentBuilder: NewContentBuilder(),
		delivery:       NewDelivery(cfg.RecipientEmail),
	}
}

// PostStore returns the underlying post store for direct access.
func (g *Generator) PostStore() *PostStore {
	return g.postStore
}

// TopicStore returns the underlying topic store for direct access.
func (g *Generator) TopicStore() *TopicStore {
	return g.topicStore
}

// Generate runs the full generation pipeline and returns the result.
// This is the main entry point called by `cc linkedin generate`.
func (g *Generator) Generate() (*GenerateResult, error) {
	result := &GenerateResult{}

	// Step 1: Seed default topics if needed.
	if err := g.topicStore.SeedDefaults(); err != nil {
		return nil, fmt.Errorf("seed topics: %w", err)
	}

	// Step 2: Select the next topic.
	topic, err := g.topicStore.NextTopic()
	if err != nil {
		return nil, fmt.Errorf("select topic: %w", err)
	}

	// Step 3: Scan projects for context.
	insights, err := g.scanner.ScanAll()
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("scan warning: %v", err))
	}

	// Step 4: Fetch trending topics.
	trends, err := g.trendAnalyzer.FetchTrends(5)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("trends warning: %v", err))
	}

	// Step 5: Build the generation prompt.
	prompt := g.contentBuilder.BuildPrompt(topic, insights, trends)

	// Step 6: Generate content via claude -p.
	content, err := g.generateWithClaude(prompt.FullPrompt)
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}

	// Step 7: Parse the generated content.
	post, err := parseGeneratedContent(content)
	if err != nil {
		return nil, fmt.Errorf("parse generated content: %w", err)
	}

	post.Topic = topic.Topic
	post.SourceProject = topic.SourceProject
	post.Status = StatusPendingReview

	// Step 8: Store the post.
	id, err := g.postStore.Create(post)
	if err != nil {
		return nil, fmt.Errorf("store post: %w", err)
	}
	post.ID = id

	// Step 9: Mark topic as used.
	if err := g.topicStore.MarkUsed(topic.ID); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("mark topic used: %v", err))
	}

	// Step 10: Prepare email delivery.
	deliveryResult, err := g.delivery.Prepare(post)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("delivery prep: %v", err))
	} else {
		// Print delivery info for the Claude Code session to pick up.
		fmt.Printf("EMAIL_SUBJECT: %s\n", deliveryResult.EmailSubject)
		fmt.Printf("EMAIL_TO: %s\n", deliveryResult.RecipientEmail)
		fmt.Printf("EMAIL_BODY_START\n%s\nEMAIL_BODY_END\n", deliveryResult.EmailBody)
	}

	// Step 11: Send desktop notification.
	if err := g.delivery.NotifyPostReady(post); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("notification: %v", err))
	}

	result.Post = post
	return result, nil
}

// GenerateDryRun runs the generation pipeline but skips DB storage, email, and notifications.
// It returns the raw generated post content for evaluation purposes.
func (g *Generator) GenerateDryRun() (string, error) {
	// Seed and select topic.
	if err := g.topicStore.SeedDefaults(); err != nil {
		return "", fmt.Errorf("seed topics: %w", err)
	}
	topic, err := g.topicStore.NextTopic()
	if err != nil {
		return "", fmt.Errorf("select topic: %w", err)
	}

	// Gather context (errors are non-fatal).
	insights, _ := g.scanner.ScanAll()
	trends, _ := g.trendAnalyzer.FetchTrends(5)

	// Build and run the prompt.
	prompt := g.contentBuilder.BuildPrompt(topic, insights, trends)
	content, err := g.generateWithClaude(prompt.FullPrompt)
	if err != nil {
		return "", fmt.Errorf("generate content: %w", err)
	}
	return content, nil
}

// generateWithClaude invokes claude -p with the given prompt.
func (g *Generator) generateWithClaude(prompt string) (string, error) {
	cmd := exec.Command("claude", "-p", prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude -p: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseGeneratedContent parses the structured output from Claude into a Post.
func parseGeneratedContent(content string) (*Post, error) {
	post := &Post{
		Target: TargetPersonal,
	}

	// Split on the --- separator.
	parts := strings.SplitN(content, "---", 2)

	if len(parts) == 2 {
		// Parse header fields.
		header := parts[0]
		post.Content = strings.TrimSpace(parts[1])

		for _, line := range strings.Split(header, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "TITLE:") {
				post.Title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
			} else if strings.HasPrefix(line, "DIAGRAM:") {
				post.DiagramDesc = strings.TrimSpace(strings.TrimPrefix(line, "DIAGRAM:"))
			} else if strings.HasPrefix(line, "TARGET:") {
				target := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(line, "TARGET:")))
				if target == "employer" {
					post.Target = TargetEmployer
				}
			}
		}
	} else {
		// No separator found; treat entire content as the post body.
		post.Content = strings.TrimSpace(content)
		post.Title = extractTitle(post.Content)
	}

	if post.Title == "" {
		post.Title = "Untitled Post"
	}

	return post, nil
}

// extractTitle attempts to extract a title from the first line of content.
func extractTitle(content string) string {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) > 0 {
		title := strings.TrimSpace(lines[0])
		// Remove common markdown/formatting.
		title = strings.TrimLeft(title, "#")
		title = strings.TrimSpace(title)
		if len(title) > 80 {
			title = title[:80] + "..."
		}
		return title
	}
	return ""
}

// Preview returns information about the next topic that would be generated.
func (g *Generator) Preview() (*Topic, []TrendItem, error) {
	if err := g.topicStore.SeedDefaults(); err != nil {
		return nil, nil, fmt.Errorf("seed topics: %w", err)
	}

	topic, err := g.topicStore.NextTopic()
	if err != nil {
		return nil, nil, fmt.Errorf("next topic: %w", err)
	}

	trends, _ := g.trendAnalyzer.FetchTrends(5)

	return topic, trends, nil
}
