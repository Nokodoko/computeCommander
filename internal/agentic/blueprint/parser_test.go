package blueprint

import (
	"testing"
)

func TestParseValidBlueprint(t *testing.T) {
	yaml := []byte(`
id: bp-12345678
name: "Add JWT middleware"
agent: unix-coder
capability: builder
context:
  - action: read
    path: "internal/auth/*.go"
  - action: write
    path: "internal/auth/jwt.go"
inputs:
  spec: "JWT RS256 with 1h expiry"
outputs:
  files:
    - "internal/auth/jwt.go"
  tests:
    - "internal/auth/jwt_test.go"
verify:
  - command: "go test ./internal/auth/..."
    expect: exit_0
gates:
  - lint
  - test
retry_limit: 3
timeout: "30m"
`)
	bp, err := Parse(yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if bp.ID != "bp-12345678" {
		t.Fatalf("expected ID bp-12345678, got %q", bp.ID)
	}
	if bp.Name != "Add JWT middleware" {
		t.Fatalf("expected name 'Add JWT middleware', got %q", bp.Name)
	}
	if bp.Agent != "unix-coder" {
		t.Fatalf("expected agent unix-coder, got %q", bp.Agent)
	}
	if bp.Capability != "builder" {
		t.Fatalf("expected capability builder, got %q", bp.Capability)
	}
	if len(bp.ContextGrants) != 2 {
		t.Fatalf("expected 2 context grants, got %d", len(bp.ContextGrants))
	}
	if len(bp.VerifySteps) != 1 {
		t.Fatalf("expected 1 verify step, got %d", len(bp.VerifySteps))
	}
	if len(bp.Gates) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(bp.Gates))
	}
}

func TestParseMinimalBlueprint(t *testing.T) {
	yaml := []byte(`
name: "Simple task"
agent: unix-coder
capability: builder
`)
	bp, err := Parse(yaml)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if bp.RetryLimit != 3 {
		t.Fatalf("expected default retry_limit 3, got %d", bp.RetryLimit)
	}
	if bp.Timeout != "30m" {
		t.Fatalf("expected default timeout 30m, got %q", bp.Timeout)
	}
	if bp.Status != StatusPending {
		t.Fatalf("expected default status pending, got %q", bp.Status)
	}
}

func TestParseMissingName(t *testing.T) {
	yaml := []byte(`
agent: unix-coder
capability: builder
`)
	_, err := Parse(yaml)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseMissingAgent(t *testing.T) {
	yaml := []byte(`
name: "test"
capability: builder
`)
	_, err := Parse(yaml)
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestParseInvalidGate(t *testing.T) {
	yaml := []byte(`
name: "test"
agent: unix-coder
capability: builder
gates:
  - invalid_gate
`)
	_, err := Parse(yaml)
	if err == nil {
		t.Fatal("expected error for invalid gate")
	}
}

func TestParseInvalidContextAction(t *testing.T) {
	yaml := []byte(`
name: "test"
agent: unix-coder
capability: builder
context:
  - action: execute
    path: "/tmp"
`)
	_, err := Parse(yaml)
	if err == nil {
		t.Fatal("expected error for invalid context action")
	}
}

func TestParseInvalidVerifyExpect(t *testing.T) {
	yaml := []byte(`
name: "test"
agent: unix-coder
capability: builder
verify:
  - command: "echo hi"
    expect: invalid
`)
	_, err := Parse(yaml)
	if err == nil {
		t.Fatal("expected error for invalid verify expect")
	}
}

func TestSerialize(t *testing.T) {
	bp := &Blueprint{
		ID:         "bp-test",
		Name:       "Test",
		Agent:      "unix-coder",
		Capability: "builder",
	}
	data, err := Serialize(bp)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty output")
	}
}
