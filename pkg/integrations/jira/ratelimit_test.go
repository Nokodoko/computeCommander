package jira

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestNewRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	if rl.CurrentRate() != rate.Limit(10) {
		t.Errorf("expected default rate 10, got %v", rl.CurrentRate())
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(100, 10)
	ctx := context.Background()

	// Should not block for burst requests.
	for i := 0; i < 10; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait returned error on burst request %d: %v", i, err)
		}
	}
}

func TestRateLimiter_CancelledContext(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	ctx, cancel := context.WithCancel(context.Background())

	// Consume the burst token.
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("first wait: %v", err)
	}

	cancel()

	// Should fail with cancelled context.
	if err := rl.Wait(ctx); err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestRateLimiter_AdaptReduction(t *testing.T) {
	rl := NewRateLimiter(10, 20)

	if rl.IsReduced() {
		t.Error("should not start in reduced mode")
	}

	// Simulate low remaining count.
	rl.AdaptFromHeaders(3, 0)
	if !rl.IsReduced() {
		t.Error("should be reduced when remaining < 5")
	}
	if rl.CurrentRate() != rate.Limit(5) {
		t.Errorf("expected rate 5, got %v", rl.CurrentRate())
	}
}

func TestRateLimiter_AdaptRecovery(t *testing.T) {
	rl := NewRateLimiter(10, 20)

	// Reduce first.
	rl.AdaptFromHeaders(3, 0)
	if !rl.IsReduced() {
		t.Fatal("expected reduced")
	}

	// Recover.
	rl.AdaptFromHeaders(20, 0)
	if rl.IsReduced() {
		t.Error("should recover when remaining > 15")
	}
	if rl.CurrentRate() != rate.Limit(10) {
		t.Errorf("expected rate 10, got %v", rl.CurrentRate())
	}
}

func TestRateLimiter_RetryAfter(t *testing.T) {
	rl := NewRateLimiter(10, 20)

	// Trigger retry-after.
	rl.AdaptFromHeaders(0, 100*time.Millisecond)

	// Rate should be 0 (full stop).
	if rl.CurrentRate() != 0 {
		t.Errorf("expected rate 0 during retry-after, got %v", rl.CurrentRate())
	}

	// Wait for recovery.
	time.Sleep(200 * time.Millisecond)

	if rl.CurrentRate() != rate.Limit(10) {
		t.Errorf("expected rate 10 after retry-after recovery, got %v", rl.CurrentRate())
	}
}
