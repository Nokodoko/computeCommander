package linkedin

import (
	"context"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// PostStore manages LinkedIn posts in SQLite.
type PostStore struct {
	db db.DB
}

// NewPostStore creates a PostStore backed by the given database.
func NewPostStore(database db.DB) *PostStore {
	return &PostStore{db: database}
}

// Create inserts a new post and returns its ID.
func (s *PostStore) Create(p *Post) (int64, error) {
	ctx := context.Background()

	err := s.db.Exec(ctx, `
		INSERT INTO linkedin_posts (topic, title, content, diagram_desc, source_project, target, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Topic, p.Title, p.Content, p.DiagramDesc,
		p.SourceProject, string(p.Target), string(p.Status),
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("insert post: %w", err)
	}

	// Get the last inserted ID.
	var id int64
	err = s.db.QueryRow(ctx, "SELECT last_insert_rowid()").Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}

	return id, nil
}

// Get retrieves a post by ID.
func (s *PostStore) Get(id int64) (*Post, error) {
	ctx := context.Background()

	row := s.db.QueryRow(ctx, `
		SELECT id, topic, title, content, diagram_desc, source_project, target, status,
			created_at, posted_at, feedback_rating, feedback_notes,
			engagement_likes, engagement_comments, engagement_reposts
		FROM linkedin_posts WHERE id = ?`, id)

	return scanPost(row)
}

// UpdateStatus changes the status of a post.
func (s *PostStore) UpdateStatus(id int64, status PostStatus) error {
	ctx := context.Background()
	return s.db.Exec(ctx, "UPDATE linkedin_posts SET status = ? WHERE id = ?", string(status), id)
}

// Approve marks a post as approved.
func (s *PostStore) Approve(id int64) error {
	return s.UpdateStatus(id, StatusApproved)
}

// Reject marks a post as rejected.
func (s *PostStore) Reject(id int64) error {
	return s.UpdateStatus(id, StatusRejected)
}

// MarkPosted marks a post as posted with the current timestamp.
func (s *PostStore) MarkPosted(id int64) error {
	ctx := context.Background()
	return s.db.Exec(ctx,
		"UPDATE linkedin_posts SET status = ?, posted_at = ? WHERE id = ?",
		string(StatusPosted), time.Now().UTC().Format(time.RFC3339), id,
	)
}

// SetFeedback stores a rating and optional notes for a post.
func (s *PostStore) SetFeedback(id int64, rating int, notes string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be 1-5, got %d", rating)
	}
	ctx := context.Background()
	return s.db.Exec(ctx,
		"UPDATE linkedin_posts SET feedback_rating = ?, feedback_notes = ? WHERE id = ?",
		rating, notes, id,
	)
}

// UpdateEngagement updates the engagement metrics for a post.
func (s *PostStore) UpdateEngagement(id int64, likes, comments, reposts int) error {
	ctx := context.Background()
	return s.db.Exec(ctx,
		"UPDATE linkedin_posts SET engagement_likes = ?, engagement_comments = ?, engagement_reposts = ? WHERE id = ?",
		likes, comments, reposts, id,
	)
}

// List returns posts matching the given filters, ordered by creation time descending.
func (s *PostStore) List(status PostStatus, limit int) ([]Post, error) {
	ctx := context.Background()

	query := `SELECT id, topic, title, content, diagram_desc, source_project, target, status,
		created_at, posted_at, feedback_rating, feedback_notes,
		engagement_likes, engagement_comments, engagement_reposts
		FROM linkedin_posts`

	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, string(status))
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		p, err := scanPostRow(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, *p)
	}
	return posts, rows.Err()
}

// PendingReview returns posts with status "pending_review".
func (s *PostStore) PendingReview() ([]Post, error) {
	return s.List(StatusPendingReview, 0)
}

// Stats computes aggregate statistics across all posts.
func (s *PostStore) Stats() (*PostStats, error) {
	ctx := context.Background()

	stats := &PostStats{}

	// Total posts.
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM linkedin_posts").Scan(&stats.TotalPosts); err != nil {
		return nil, fmt.Errorf("count posts: %w", err)
	}

	// Status counts.
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM linkedin_posts WHERE status = 'approved'").Scan(&stats.ApprovedCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM linkedin_posts WHERE status = 'rejected'").Scan(&stats.RejectedCount)
	s.db.QueryRow(ctx, "SELECT COUNT(*) FROM linkedin_posts WHERE status = 'posted'").Scan(&stats.PostedCount)

	// Average rating.
	s.db.QueryRow(ctx, "SELECT COALESCE(AVG(feedback_rating), 0) FROM linkedin_posts WHERE feedback_rating IS NOT NULL").Scan(&stats.AvgRating)

	// Engagement totals.
	s.db.QueryRow(ctx, "SELECT COALESCE(SUM(engagement_likes), 0) FROM linkedin_posts").Scan(&stats.TotalLikes)
	s.db.QueryRow(ctx, "SELECT COALESCE(SUM(engagement_comments), 0) FROM linkedin_posts").Scan(&stats.TotalComments)
	s.db.QueryRow(ctx, "SELECT COALESCE(SUM(engagement_reposts), 0) FROM linkedin_posts").Scan(&stats.TotalReposts)

	// Top-rated project.
	s.db.QueryRow(ctx, `
		SELECT source_project FROM linkedin_posts
		WHERE feedback_rating IS NOT NULL
		GROUP BY source_project
		ORDER BY AVG(feedback_rating) DESC
		LIMIT 1
	`).Scan(&stats.TopProject)

	return stats, nil
}

// scanPost scans a single post from a QueryRow result.
func scanPost(row *db.Row) (*Post, error) {
	var p Post
	var target, status string
	var createdAt string
	var postedAt *string
	var feedbackRating *int
	var diagramDesc, feedbackNotes, sourceProject *string

	err := row.Scan(
		&p.ID, &p.Topic, &p.Title, &p.Content, &diagramDesc, &sourceProject,
		&target, &status, &createdAt, &postedAt, &feedbackRating, &feedbackNotes,
		&p.EngagementLikes, &p.EngagementComments, &p.EngagementReposts,
	)
	if err != nil {
		return nil, fmt.Errorf("scan post: %w", err)
	}

	p.Target = PostTarget(target)
	p.Status = PostStatus(status)
	p.FeedbackRating = feedbackRating

	if diagramDesc != nil {
		p.DiagramDesc = *diagramDesc
	}
	if sourceProject != nil {
		p.SourceProject = *sourceProject
	}
	if feedbackNotes != nil {
		p.FeedbackNotes = *feedbackNotes
	}
	if createdAt != "" {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			p.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			p.CreatedAt = t
		}
	}
	if postedAt != nil {
		if t, err := time.Parse(time.RFC3339, *postedAt); err == nil {
			p.PostedAt = &t
		}
	}

	return &p, nil
}

// scanPostRow scans a post from a Rows iterator.
func scanPostRow(rows *db.Rows) (*Post, error) {
	var p Post
	var target, status string
	var createdAt string
	var postedAt *string
	var feedbackRating *int
	var diagramDesc, feedbackNotes, sourceProject *string

	err := rows.Scan(
		&p.ID, &p.Topic, &p.Title, &p.Content, &diagramDesc, &sourceProject,
		&target, &status, &createdAt, &postedAt, &feedbackRating, &feedbackNotes,
		&p.EngagementLikes, &p.EngagementComments, &p.EngagementReposts,
	)
	if err != nil {
		return nil, fmt.Errorf("scan post row: %w", err)
	}

	p.Target = PostTarget(target)
	p.Status = PostStatus(status)
	p.FeedbackRating = feedbackRating

	if diagramDesc != nil {
		p.DiagramDesc = *diagramDesc
	}
	if sourceProject != nil {
		p.SourceProject = *sourceProject
	}
	if feedbackNotes != nil {
		p.FeedbackNotes = *feedbackNotes
	}
	if createdAt != "" {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			p.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			p.CreatedAt = t
		}
	}
	if postedAt != nil {
		if t, err := time.Parse(time.RFC3339, *postedAt); err == nil {
			p.PostedAt = &t
		}
	}

	return &p, nil
}
