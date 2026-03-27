// Package linkedin provides a LinkedIn post generation pipeline for ComputeCommander.
// It scans user projects for technical insights, generates ByteByteGo-style posts,
// and delivers them via email for human review before publishing.
package linkedin

import "time"

// PostStatus tracks the lifecycle of a generated post.
type PostStatus string

const (
	StatusDraft         PostStatus = "draft"
	StatusPendingReview PostStatus = "pending_review"
	StatusApproved      PostStatus = "approved"
	StatusPosted        PostStatus = "posted"
	StatusRejected      PostStatus = "rejected"
)

// PostTarget indicates the audience context for a post.
type PostTarget string

const (
	TargetPersonal PostTarget = "personal"
	TargetEmployer PostTarget = "employer"
)

// Post represents a generated LinkedIn post with all metadata.
type Post struct {
	ID                  int64      `json:"id"`
	Topic               string     `json:"topic"`
	Title               string     `json:"title"`
	Content             string     `json:"content"`
	DiagramDesc         string     `json:"diagram_desc,omitempty"`
	SourceProject       string     `json:"source_project,omitempty"`
	Target              PostTarget `json:"target"`
	Status              PostStatus `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	PostedAt            *time.Time `json:"posted_at,omitempty"`
	FeedbackRating      *int       `json:"feedback_rating,omitempty"`
	FeedbackNotes       string     `json:"feedback_notes,omitempty"`
	EngagementLikes     int        `json:"engagement_likes"`
	EngagementComments  int        `json:"engagement_comments"`
	EngagementReposts   int        `json:"engagement_reposts"`
}

// Topic represents a content topic in the generation queue.
type Topic struct {
	ID            int64      `json:"id"`
	Topic         string     `json:"topic"`
	SourceProject string     `json:"source_project,omitempty"`
	Priority      int        `json:"priority"`
	Used          bool       `json:"used"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	AvgRating     float64    `json:"avg_rating"`
}

// ProjectInsight holds extracted information from scanning a user project.
type ProjectInsight struct {
	Project      string   `json:"project"`
	Path         string   `json:"path"`
	RecentFiles  []string `json:"recent_files,omitempty"`
	CommitLog    string   `json:"commit_log,omitempty"`
	Architecture string   `json:"architecture,omitempty"`
	KeyPatterns  []string `json:"key_patterns,omitempty"`
	DataPoints   []string `json:"data_points,omitempty"`
}

// TrendItem represents a trending topic from an RSS feed.
type TrendItem struct {
	Title     string    `json:"title"`
	Link      string    `json:"link"`
	Source    string    `json:"source"`
	Published time.Time `json:"published,omitzero"`
}

// GenerateResult holds the output of a post generation run.
type GenerateResult struct {
	Post     *Post    `json:"post"`
	Warnings []string `json:"warnings,omitempty"`
}

// PostStats aggregates engagement and rating data across posts.
type PostStats struct {
	TotalPosts      int     `json:"total_posts"`
	ApprovedCount   int     `json:"approved_count"`
	RejectedCount   int     `json:"rejected_count"`
	PostedCount     int     `json:"posted_count"`
	AvgRating       float64 `json:"avg_rating"`
	TotalLikes      int     `json:"total_likes"`
	TotalComments   int     `json:"total_comments"`
	TotalReposts    int     `json:"total_reposts"`
	TopProject      string  `json:"top_project,omitempty"`
}

// Config holds configuration for the LinkedIn post generator.
type Config struct {
	// RecipientEmail is the email address to send review drafts to.
	RecipientEmail string `json:"recipient_email" yaml:"recipient_email"`

	// Projects maps project names to their filesystem paths.
	Projects map[string]string `json:"projects" yaml:"projects"`

	// HooksDir is the path to the Claude Code hooks directory.
	HooksDir string `json:"hooks_dir" yaml:"hooks_dir"`

	// DBPath is the path to the SQLite database for post/topic storage.
	// If empty, uses the project's default database.
	DBPath string `json:"db_path,omitempty" yaml:"db_path,omitempty"`
}

// DefaultConfig returns the default LinkedIn generator configuration.
func DefaultConfig() Config {
	return Config{
		RecipientEmail: "cmonty614@gmail.com",
		Projects: map[string]string{
			"computeCommander": "/home/n0ko/Programs/ai/computeCommander",
			"openbrain":        "/home/n0ko/Programs/ai/openbrain",
			"trustgraph":       "",
			"rayne":            "/home/n0ko/Portfolio/rayne",
		},
		HooksDir: "/home/n0ko/.claude/hooks",
	}
}
