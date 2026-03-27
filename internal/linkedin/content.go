package linkedin

import (
	"fmt"
	"strings"
)

// ContentBuilder assembles the prompt and context for Claude to generate a post.
// The actual LLM generation happens in a Claude Code session (via claude -p).
// This package prepares the structured input that the session uses.
type ContentBuilder struct{}

// NewContentBuilder creates a ContentBuilder.
func NewContentBuilder() *ContentBuilder {
	return &ContentBuilder{}
}

// GenerationPrompt holds the structured prompt for Claude to generate a post.
type GenerationPrompt struct {
	// SystemContext is the style guide and constraints for generation.
	SystemContext string `json:"system_context"`

	// TopicContext is the specific topic and project context.
	TopicContext string `json:"topic_context"`

	// TrendContext is relevant trending topics for cross-referencing.
	TrendContext string `json:"trend_context"`

	// ProjectInsights are the extracted insights from scanned projects.
	ProjectInsights string `json:"project_insights"`

	// FullPrompt is the assembled prompt ready for claude -p.
	FullPrompt string `json:"full_prompt"`
}

// BuildPrompt constructs the generation prompt from topic, insights, and trends.
func (c *ContentBuilder) BuildPrompt(topic *Topic, insights []ProjectInsight, trends []TrendItem) *GenerationPrompt {
	prompt := &GenerationPrompt{}

	prompt.SystemContext = styleGuide

	prompt.TopicContext = fmt.Sprintf("Topic: %s\nSource Project: %s\nPriority: %d",
		topic.Topic, topic.SourceProject, topic.Priority)

	// Build project insights context.
	var insightParts []string
	for _, insight := range insights {
		if insight.Project == topic.SourceProject || topic.SourceProject == "" {
			part := fmt.Sprintf("=== %s (%s) ===\n", insight.Project, insight.Path)
			if insight.CommitLog != "" {
				part += fmt.Sprintf("Recent commits:\n%s\n\n", truncate(insight.CommitLog, 500))
			}
			if len(insight.RecentFiles) > 0 {
				part += fmt.Sprintf("Recently changed files:\n%s\n\n", strings.Join(insight.RecentFiles, "\n"))
			}
			if insight.Architecture != "" {
				part += fmt.Sprintf("Architecture:\n%s\n\n", truncate(insight.Architecture, 1000))
			}
			if len(insight.KeyPatterns) > 0 {
				part += fmt.Sprintf("Key patterns: %s\n", strings.Join(insight.KeyPatterns, ", "))
			}
			if len(insight.DataPoints) > 0 {
				part += fmt.Sprintf("Data points: %s\n", strings.Join(insight.DataPoints, ", "))
			}
			insightParts = append(insightParts, part)
		}
	}
	prompt.ProjectInsights = strings.Join(insightParts, "\n")

	// Build trend context.
	if len(trends) > 0 {
		var trendParts []string
		for _, t := range trends {
			trendParts = append(trendParts, fmt.Sprintf("- %s (%s): %s", t.Title, t.Source, t.Link))
		}
		prompt.TrendContext = "Trending AI topics for potential cross-reference:\n" + strings.Join(trendParts, "\n")
	}

	// Assemble the full prompt for claude -p.
	prompt.FullPrompt = assemblePrompt(prompt)

	return prompt
}

// assemblePrompt creates the final prompt string for claude -p.
func assemblePrompt(p *GenerationPrompt) string {
	var parts []string

	parts = append(parts, "You are a LinkedIn post generator for a senior AI/DevOps engineer.")
	parts = append(parts, "")
	parts = append(parts, "## Style Guide")
	parts = append(parts, p.SystemContext)
	parts = append(parts, "")
	parts = append(parts, "## Topic")
	parts = append(parts, p.TopicContext)
	parts = append(parts, "")

	if p.ProjectInsights != "" {
		parts = append(parts, "## Project Context")
		parts = append(parts, p.ProjectInsights)
		parts = append(parts, "")
	}

	if p.TrendContext != "" {
		parts = append(parts, "## Current Trends")
		parts = append(parts, p.TrendContext)
		parts = append(parts, "")
	}

	parts = append(parts, "## Instructions")
	parts = append(parts, generateInstructions)

	return strings.Join(parts, "\n")
}

const styleGuide = `ByteByteGo-Inspired LinkedIn Style:
1. Lead with a question or bold claim. "What if your CI/CD pipeline could think?" not "We implemented AI in our pipeline."
2. Show architecture, not just prose. Every post needs a visual element description.
3. Concrete numbers over vague claims. "45 hooks fire on every prompt" not "we have lots of automation."
4. Technical depth, accessible language. Explain the WHY before the HOW.
5. Personal voice. First person singular for personal posts, first person plural for employer posts.
6. End with engagement. Ask a question professionals can answer from their experience.
7. 2000 character sweet spot. LinkedIn truncates at ~3000 chars. Aim for 1500-2000.

Format Template:
[Hook Line -- provocative question or surprising statement]
[1-2 sentence problem statement]
[Architecture diagram or visual description in ASCII/text]
[3-5 numbered points explaining the system/approach]
[Result or impact statement]
[CTA -- question to audience or invitation to connect]
#AI #SystemDesign #DevOps [relevant hashtags]

IMPORTANT RULES:
- computeCommander is based on a fork of jaymin West's overstory project. Reference overstory to drive traffic to jaymin's project; do NOT claim it as original work.
- rayne content: employer is "a leading tech company". NO MTTR numbers, no specific metrics. Architecture patterns are shareable.
- Content MUST reference real implementations from the user's projects. No generic AI platitudes.`

const generateInstructions = `Generate a LinkedIn post following the style guide above.

Output format (respond with ONLY this, no markdown fences):
TITLE: <post title>
DIAGRAM: <description of a technical diagram/visual to accompany the post>
TARGET: <personal or employer>
---
<the full post content, ready to copy-paste to LinkedIn>

Rules:
- The post content should be 1500-2000 characters
- Include 3-5 relevant hashtags at the end
- Reference specific technical details from the project context provided
- Be concrete: mention specific numbers, patterns, tools
- End with an engaging question for the audience`

// truncate shortens a string to the given max length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
