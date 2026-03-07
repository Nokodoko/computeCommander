package block

import (
	"context"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupRateDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a block rule for the FK constraint
	ctx := context.Background()
	err = database.Exec(ctx, `INSERT INTO block_rules (id, description, tool, match_config, action, message, severity)
		VALUES ('test-rule', 'test', 'Bash', '{}', 'block', 'test', 'high')`)
	if err != nil {
		t.Fatalf("insert rule: %v", err)
	}

	t.Cleanup(func() { database.Close() })
	return database
}

func TestRateLimiterRecord(t *testing.T) {
	database := setupRateDB(t)
	ctx := context.Background()
	rl := NewRateLimiter(database)

	if err := rl.Record(ctx, "test-rule", "agent-001"); err != nil {
		t.Fatalf("record: %v", err)
	}

	count, err := rl.Count(ctx, "test-rule", "agent-001", "1h")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestRateLimiterIsLimited(t *testing.T) {
	database := setupRateDB(t)
	ctx := context.Background()
	rl := NewRateLimiter(database)

	// Record 3 events
	for i := 0; i < 3; i++ {
		if err := rl.Record(ctx, "test-rule", "agent-001"); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	limited, err := rl.IsLimited(ctx, "test-rule", "agent-001", "1h", 3)
	if err != nil {
		t.Fatalf("is limited: %v", err)
	}
	if !limited {
		t.Fatal("expected limited after 3 records with max 3")
	}

	// Different agent should not be limited
	limited, err = rl.IsLimited(ctx, "test-rule", "agent-002", "1h", 3)
	if err != nil {
		t.Fatalf("is limited other agent: %v", err)
	}
	if limited {
		t.Fatal("expected not limited for different agent")
	}
}

func TestRateLimiterReset(t *testing.T) {
	database := setupRateDB(t)
	ctx := context.Background()
	rl := NewRateLimiter(database)

	if err := rl.Record(ctx, "test-rule", "agent-001"); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := rl.Reset(ctx, "test-rule", "agent-001"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	count, err := rl.Count(ctx, "test-rule", "agent-001", "1h")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0 after reset, got %d", count)
	}
}

func TestRateLimiterNotLimitedUnderMax(t *testing.T) {
	database := setupRateDB(t)
	ctx := context.Background()
	rl := NewRateLimiter(database)

	if err := rl.Record(ctx, "test-rule", "agent-001"); err != nil {
		t.Fatalf("record: %v", err)
	}

	limited, err := rl.IsLimited(ctx, "test-rule", "agent-001", "1h", 5)
	if err != nil {
		t.Fatalf("is limited: %v", err)
	}
	if limited {
		t.Fatal("expected not limited with 1 record and max 5")
	}
}
