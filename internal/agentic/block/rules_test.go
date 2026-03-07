package block

import (
	"testing"
)

func TestParseRules(t *testing.T) {
	yaml := []byte(`
version: 1
rules:
  - id: no-force-push
    description: "Block git push --force"
    tool: Bash
    match:
      command: "git push.*--force"
    action: block
    message: "Force push prohibited"
    severity: critical
  - id: no-secret-read
    description: "Block reading secrets"
    tool: Read
    match:
      file_path: ".*\\.env$"
    action: block
    message: "Reading secrets prohibited"
    severity: critical
    override: grant
`)
	rules, err := ParseRules(yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].ID != "no-force-push" {
		t.Fatalf("expected no-force-push, got %q", rules[0].ID)
	}
	if rules[1].Override != OverrideGrant {
		t.Fatalf("expected grant override, got %q", rules[1].Override)
	}
}

func TestBlockRuleMatches(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: test-rule
    description: "Test"
    tool: Bash
    match:
      command: "rm -rf"
    action: block
    message: "No rm -rf"
    severity: critical
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := rules[0]

	// Should match
	if !rule.Matches(&EvalInput{Tool: "Bash", Command: "rm -rf /tmp"}) {
		t.Fatal("expected match for rm -rf")
	}

	// Wrong tool
	if rule.Matches(&EvalInput{Tool: "Read", Command: "rm -rf /tmp"}) {
		t.Fatal("expected no match for wrong tool")
	}

	// Non-matching command
	if rule.Matches(&EvalInput{Tool: "Bash", Command: "ls -la"}) {
		t.Fatal("expected no match for ls")
	}
}

func TestBlockRuleDepthMatch(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: depth-limit
    description: "Depth limit"
    tool: Task
    match:
      depth: ">5"
    action: block
    message: "Too deep"
    severity: high
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := rules[0]

	// Depth 3: should NOT match (not exceeding 5)
	if rule.Matches(&EvalInput{Tool: "Task", Depth: 3}) {
		t.Fatal("expected no match at depth 3")
	}

	// Depth 6: should match (exceeding 5)
	if !rule.Matches(&EvalInput{Tool: "Task", Depth: 6}) {
		t.Fatal("expected match at depth 6")
	}

	// Depth 5: should NOT match (not exceeding, equal)
	if rule.Matches(&EvalInput{Tool: "Task", Depth: 5}) {
		t.Fatal("expected no match at depth 5 (equal, not exceeding)")
	}
}

func TestBlockRuleFilePathMatch(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: no-env-read
    description: "Block .env"
    tool: Read
    match:
      file_path: ".*\\.env$"
    action: block
    message: "No .env"
    severity: critical
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := rules[0]

	if !rule.Matches(&EvalInput{Tool: "Read", FilePath: "config/.env"}) {
		t.Fatal("expected match for .env")
	}
	if rule.Matches(&EvalInput{Tool: "Read", FilePath: "config/app.yaml"}) {
		t.Fatal("expected no match for .yaml")
	}
}

func TestBlockRuleCanOverride(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: overridable
    description: "Overridable rule"
    tool: Read
    match:
      file_path: "test"
    action: block
    message: "blocked"
    severity: high
    override: grant
  - id: not-overridable
    description: "Not overridable"
    tool: Read
    match:
      file_path: "test"
    action: block
    message: "blocked"
    severity: critical
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !rules[0].CanOverride() {
		t.Fatal("expected overridable")
	}
	if rules[1].CanOverride() {
		t.Fatal("expected not overridable")
	}
}

func TestBlockRuleIsOverridden(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: my-rule
    description: "Test"
    tool: Read
    match:
      file_path: "test"
    action: block
    message: "blocked"
    severity: high
    override: grant
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := rules[0]

	if rule.IsOverridden([]string{}) {
		t.Fatal("expected not overridden with empty grants")
	}
	if !rule.IsOverridden([]string{"my-rule"}) {
		t.Fatal("expected overridden with matching grant")
	}
	if !rule.IsOverridden([]string{"*"}) {
		t.Fatal("expected overridden with wildcard grant")
	}
	if rule.IsOverridden([]string{"other-rule"}) {
		t.Fatal("expected not overridden with non-matching grant")
	}
}

func TestBlockRuleHasRateLimit(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: rate-limited
    description: "Rate limited"
    tool: Bash
    match:
      command: "test"
      count_window: "1h"
      count_max: 3
    action: block
    message: "blocked"
    severity: medium
  - id: not-rate-limited
    description: "Not rate limited"
    tool: Bash
    match:
      command: "test"
    action: block
    message: "blocked"
    severity: high
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !rules[0].HasRateLimit() {
		t.Fatal("expected rate limited")
	}
	if rules[1].HasRateLimit() {
		t.Fatal("expected not rate limited")
	}
}

func TestDisabledRuleDoesNotMatch(t *testing.T) {
	rules, err := ParseRules([]byte(`
version: 1
rules:
  - id: test-rule
    description: "Test"
    tool: Bash
    match:
      command: "test"
    action: block
    message: "blocked"
    severity: high
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rules[0].Enabled = false

	if rules[0].Matches(&EvalInput{Tool: "Bash", Command: "test"}) {
		t.Fatal("disabled rule should not match")
	}
}
