package block

import (
	"context"
	"testing"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupBlockDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestBlockRuleEngineEvaluateBlock(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: no-force-push
    description: "Block git push --force"
    tool: Bash
    match:
      command: "git push.*--force.*(?:main|master)"
    action: block
    message: "Force push to protected branches is prohibited"
    severity: critical
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	ctx := context.Background()
	result := engine.Evaluate(ctx, &EvalInput{
		Tool:    "Bash",
		Command: "git push --force origin main",
	})

	if !result.Matched {
		t.Fatal("expected rule to match")
	}
	if result.RuleID != "no-force-push" {
		t.Fatalf("expected rule no-force-push, got %q", result.RuleID)
	}
	if result.Action != ActionBlock {
		t.Fatalf("expected block action, got %q", result.Action)
	}
}

func TestBlockRuleEngineEvaluateAllow(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: no-force-push
    description: "Block force push"
    tool: Bash
    match:
      command: "git push.*--force"
    action: block
    message: "No force push"
    severity: critical
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	ctx := context.Background()
	result := engine.Evaluate(ctx, &EvalInput{
		Tool:    "Bash",
		Command: "git push origin feature-branch",
	})

	if result.Matched {
		t.Fatal("expected rule NOT to match normal push")
	}
}

func TestBlockRuleEngineEvaluateOverride(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: no-secret-read
    description: "Block reading secrets"
    tool: Read
    match:
      file_path: ".*\\.env$"
    action: block
    message: "Reading secrets is prohibited"
    severity: critical
    override: grant
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	ctx := context.Background()

	// Without grant: should block
	result := engine.Evaluate(ctx, &EvalInput{
		Tool:     "Read",
		FilePath: "config/.env",
	})
	if !result.Matched {
		t.Fatal("expected match without grant")
	}
	if result.Overridden {
		t.Fatal("expected NOT overridden without grant")
	}

	// With grant: should be overridden
	result = engine.Evaluate(ctx, &EvalInput{
		Tool:     "Read",
		FilePath: "config/.env",
		Grants:   []string{"no-secret-read"},
	})
	if !result.Matched {
		t.Fatal("expected match with grant")
	}
	if !result.Overridden {
		t.Fatal("expected overridden with grant")
	}
}

func TestBlockRuleEngineListRules(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: rule-1
    description: "Rule 1"
    tool: Bash
    match:
      command: "test"
    action: block
    message: "blocked"
    severity: high
  - id: rule-2
    description: "Rule 2"
    tool: Read
    match:
      file_path: "test"
    action: warn
    message: "warned"
    severity: low
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	listed := engine.ListRules()
	if len(listed) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(listed))
	}
}

func TestBlockRuleEngineEnableDisable(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: test-rule
    description: "Test"
    tool: Bash
    match:
      command: "dangerous"
    action: block
    message: "blocked"
    severity: high
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	ctx := context.Background()

	// Should match when enabled
	result := engine.Evaluate(ctx, &EvalInput{Tool: "Bash", Command: "dangerous"})
	if !result.Matched {
		t.Fatal("expected match when enabled")
	}

	// Disable and verify no match
	if err := engine.DisableRule("test-rule"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	result = engine.Evaluate(ctx, &EvalInput{Tool: "Bash", Command: "dangerous"})
	if result.Matched {
		t.Fatal("expected no match when disabled")
	}

	// Re-enable and verify match
	if err := engine.EnableRule("test-rule"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	result = engine.Evaluate(ctx, &EvalInput{Tool: "Bash", Command: "dangerous"})
	if !result.Matched {
		t.Fatal("expected match after re-enable")
	}
}

func TestBlockRuleEngineGetRule(t *testing.T) {
	database := setupBlockDB(t)
	engine := NewBlockRuleEngine(database)

	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: my-rule
    description: "My rule"
    tool: Bash
    match:
      command: "test"
    action: block
    message: "blocked"
    severity: high
`))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	engine.LoadRules(rules)

	rule, err := engine.GetRule("my-rule")
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if rule.ID != "my-rule" {
		t.Fatalf("expected rule my-rule, got %q", rule.ID)
	}

	_, err = engine.GetRule("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}
