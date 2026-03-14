package jira

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter controls Jira API request rate with adaptive backoff
// based on Jira's X-RateLimit-Remaining and Retry-After headers.
type RateLimiter struct {
	limiter  *rate.Limiter
	mu       sync.Mutex
	baseRate rate.Limit
	reduced  bool
}

// NewRateLimiter creates a RateLimiter with the given requests-per-second and burst.
func NewRateLimiter(rps int, burst int) *RateLimiter {
	if rps <= 0 {
		rps = 10
	}
	if burst <= 0 {
		burst = 20
	}
	baseRate := rate.Limit(rps)
	return &RateLimiter{
		limiter:  rate.NewLimiter(baseRate, burst),
		baseRate: baseRate,
	}
}

// Wait blocks until a token is available or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	return r.limiter.Wait(ctx)
}

// AdaptFromHeaders adjusts rate based on Jira response headers.
// remaining is from X-RateLimit-Remaining, retryAfter from Retry-After.
func (r *RateLimiter) AdaptFromHeaders(remaining int, retryAfter time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if retryAfter > 0 {
		r.limiter.SetLimit(0) // Full stop
		time.AfterFunc(retryAfter, func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.limiter.SetLimit(r.baseRate)
			r.reduced = false
		})
		return
	}

	if remaining < 5 && !r.reduced {
		r.limiter.SetLimit(r.baseRate / 2)
		r.reduced = true
	} else if remaining > 15 && r.reduced {
		r.limiter.SetLimit(r.baseRate)
		r.reduced = false
	}
}

// IsReduced returns true if the rate limiter is currently in reduced mode.
func (r *RateLimiter) IsReduced() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reduced
}

// CurrentRate returns the current effective rate limit.
func (r *RateLimiter) CurrentRate() rate.Limit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.limiter.Limit()
}
