package block

import (
	"context"
	"fmt"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

// RateLimiter manages sliding window rate counters for block rules.
type RateLimiter struct {
	db db.DB
}

// NewRateLimiter creates a new RateLimiter backed by the database.
func NewRateLimiter(database db.DB) *RateLimiter {
	return &RateLimiter{db: database}
}

// IsLimited checks whether the given rule+agent has exceeded its rate limit
// within the specified window.
func (rl *RateLimiter) IsLimited(ctx context.Context, ruleID, agentID, windowStr string, maxCount int) (bool, error) {
	window, err := time.ParseDuration(windowStr)
	if err != nil {
		return false, fmt.Errorf("parse window %q: %w", windowStr, err)
	}

	windowStart := time.Now().UTC().Add(-window).Format(time.RFC3339)

	query := "SELECT COALESCE(SUM(count), 0) FROM block_rate_limits WHERE rule_id = ? AND agent_id = ? AND window_start >= ?"
	row := rl.db.QueryRow(ctx, query, ruleID, agentID, windowStart)

	var total int
	if err := row.Scan(&total); err != nil {
		return false, fmt.Errorf("query rate limit: %w", err)
	}

	return total >= maxCount, nil
}

// Record increments the rate counter for the given rule+agent in the current window.
func (rl *RateLimiter) Record(ctx context.Context, ruleID, agentID string) error {
	now := time.Now().UTC()
	windowStart := now.Truncate(time.Minute).Format(time.RFC3339)

	// Try to increment existing counter
	query := `INSERT INTO block_rate_limits (rule_id, agent_id, window_start, count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT (rule_id, agent_id, window_start) DO UPDATE SET count = count + 1`

	return rl.db.Exec(ctx, query, ruleID, agentID, windowStart)
}

// Reset clears rate limit counters for a rule+agent combination.
func (rl *RateLimiter) Reset(ctx context.Context, ruleID, agentID string) error {
	query := "DELETE FROM block_rate_limits WHERE rule_id = ? AND agent_id = ?"
	return rl.db.Exec(ctx, query, ruleID, agentID)
}

// Cleanup removes expired rate limit entries older than the given window.
func (rl *RateLimiter) Cleanup(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	query := "DELETE FROM block_rate_limits WHERE window_start < ?"
	return rl.db.Exec(ctx, query, cutoff)
}

// Count returns the current count for a rule+agent within a window.
func (rl *RateLimiter) Count(ctx context.Context, ruleID, agentID, windowStr string) (int, error) {
	window, err := time.ParseDuration(windowStr)
	if err != nil {
		return 0, fmt.Errorf("parse window %q: %w", windowStr, err)
	}

	windowStart := time.Now().UTC().Add(-window).Format(time.RFC3339)

	query := "SELECT COALESCE(SUM(count), 0) FROM block_rate_limits WHERE rule_id = ? AND agent_id = ? AND window_start >= ?"
	row := rl.db.QueryRow(ctx, query, ruleID, agentID, windowStart)

	var total int
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("query rate count: %w", err)
	}
	return total, nil
}
