package linkedin

import (
	"context"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// TopicStore manages the LinkedIn topic queue in SQLite.
type TopicStore struct {
	db db.DB
}

// NewTopicStore creates a TopicStore backed by the given database.
func NewTopicStore(database db.DB) *TopicStore {
	return &TopicStore{db: database}
}

// SeedDefaults inserts the default topic queue from the spec if the table is empty.
func (s *TopicStore) SeedDefaults() error {
	ctx := context.Background()

	var count int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM linkedin_topics").Scan(&count); err != nil {
		return fmt.Errorf("count topics: %w", err)
	}
	if count > 0 {
		return nil // Already seeded.
	}

	defaults := []struct {
		topic         string
		sourceProject string
		priority      int
	}{
		{"I Built 45 Hooks That Think Before My AI Acts -- Intent engineering pipeline", "Claude Code Hooks", 1},
		{"My AI Agents Have a Mail System -- Inter-agent communication", "computeCommander", 2},
		{"The 5-Runtime Problem: Making AI Tools Portable -- Adapter pattern", "computeCommander", 3},
		{"Context Routing: How I Classify Every Prompt in 50ms -- Automatic context injection", "openbrain", 4},
		{"AI Agents in Git Worktrees: Isolation Without Docker -- Concurrent agent work", "computeCommander", 5},
		{"Graph-Native Knowledge for AI Reasoning -- Structured knowledge backends", "trustgraph", 6},
		{"Vector DBs for Incident Memory -- Learning from failures", "rayne", 7},
		{"The Bias Detector I Built for My AI Pipeline -- Responsible AI", "Claude Code Hooks", 8},
		{"MCP Servers: Building the Context Layer for AI -- MCP architecture", "openbrain", 9},
		{"Building a Queen: Multi-Team AI Orchestration -- Colony architecture", "computeCommander", 10},
	}

	for _, d := range defaults {
		err := s.db.Exec(ctx,
			"INSERT INTO linkedin_topics (topic, source_project, priority) VALUES (?, ?, ?)",
			d.topic, d.sourceProject, d.priority,
		)
		if err != nil {
			return fmt.Errorf("seed topic %q: %w", d.topic, err)
		}
	}

	return nil
}

// NextTopic returns the highest-priority unused topic, weighted by feedback ratings.
// Topics from projects with higher average ratings get priority.
func (s *TopicStore) NextTopic() (*Topic, error) {
	ctx := context.Background()

	// Select the next unused topic, prioritizing:
	// 1. Topics from projects with higher average post ratings
	// 2. Lower priority number (higher priority)
	row := s.db.QueryRow(ctx, `
		SELECT t.id, t.topic, t.source_project, t.priority, t.used, t.used_at, t.avg_rating
		FROM linkedin_topics t
		LEFT JOIN (
			SELECT source_project, AVG(feedback_rating) as project_avg
			FROM linkedin_posts
			WHERE feedback_rating IS NOT NULL
			GROUP BY source_project
		) p ON t.source_project = p.source_project
		WHERE t.used = 0
		ORDER BY COALESCE(p.project_avg, 3.0) DESC, t.priority ASC
		LIMIT 1
	`)

	var t Topic
	var usedInt int
	var usedAt *string
	err := row.Scan(&t.ID, &t.Topic, &t.SourceProject, &t.Priority, &usedInt, &usedAt, &t.AvgRating)
	if err != nil {
		return nil, fmt.Errorf("query next topic: %w", err)
	}
	t.Used = usedInt != 0
	if usedAt != nil {
		parsed, _ := time.Parse(time.RFC3339, *usedAt)
		t.UsedAt = &parsed
	}

	return &t, nil
}

// MarkUsed marks a topic as used with the current timestamp.
func (s *TopicStore) MarkUsed(id int64) error {
	ctx := context.Background()
	return s.db.Exec(ctx,
		"UPDATE linkedin_topics SET used = 1, used_at = ? WHERE id = ?",
		time.Now().UTC().Format(time.RFC3339), id,
	)
}

// List returns all topics, optionally filtering to unused only.
func (s *TopicStore) List(unusedOnly bool) ([]Topic, error) {
	ctx := context.Background()

	query := `SELECT id, topic, source_project, priority, used, used_at, avg_rating
		FROM linkedin_topics`
	if unusedOnly {
		query += " WHERE used = 0"
	}
	query += " ORDER BY priority ASC"

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		var usedInt int
		var usedAt *string
		if err := rows.Scan(&t.ID, &t.Topic, &t.SourceProject, &t.Priority, &usedInt, &usedAt, &t.AvgRating); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		t.Used = usedInt != 0
		if usedAt != nil {
			parsed, _ := time.Parse(time.RFC3339, *usedAt)
			t.UsedAt = &parsed
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

// Add inserts a new topic into the queue.
func (s *TopicStore) Add(topic, sourceProject string, priority int) error {
	ctx := context.Background()
	return s.db.Exec(ctx,
		"INSERT INTO linkedin_topics (topic, source_project, priority) VALUES (?, ?, ?)",
		topic, sourceProject, priority,
	)
}

// UpdateRating recalculates the average rating for a topic's source project.
func (s *TopicStore) UpdateRating(sourceProject string) error {
	ctx := context.Background()
	return s.db.Exec(ctx, `
		UPDATE linkedin_topics SET avg_rating = (
			SELECT COALESCE(AVG(feedback_rating), 0)
			FROM linkedin_posts
			WHERE source_project = linkedin_topics.source_project
			AND feedback_rating IS NOT NULL
		)
		WHERE source_project = ?
	`, sourceProject)
}
