package gate

import (
	"context"
	"testing"
)

func TestMockRunner(t *testing.T) {
	runner := NewMockRunner(map[string]MockResult{
		"echo hello": {Stdout: "hello\n", ExitCode: 0},
		"false":      {Stdout: "", Stderr: "failed", ExitCode: 1},
	})

	stdout, _, code, err := runner.Run(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stdout != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", stdout)
	}

	_, stderr, code, err := runner.Run(context.Background(), "false")
	if err != nil {
		t.Fatalf("run false: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if stderr != "failed" {
		t.Fatalf("expected 'failed', got %q", stderr)
	}
}

func TestMockRunnerUnknownCommand(t *testing.T) {
	runner := NewMockRunner(map[string]MockResult{})

	_, _, code, _ := runner.Run(context.Background(), "unknown")
	if code != 1 {
		t.Fatalf("expected exit code 1 for unknown command, got %d", code)
	}
}

func TestDefaultGateConfigs(t *testing.T) {
	configs := DefaultGateConfigs()
	if len(configs) < 4 {
		t.Fatalf("expected at least 4 default gates, got %d", len(configs))
	}
	if configs[0].Name != GateFormat {
		t.Fatalf("expected first gate to be format, got %q", configs[0].Name)
	}
}

func TestValidGateName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"lint", true},
		{"typecheck", true},
		{"test", true},
		{"security", true},
		{"format", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if ValidGateName(tt.name) != tt.valid {
			t.Errorf("ValidGateName(%q) = %v, want %v", tt.name, !tt.valid, tt.valid)
		}
	}
}

func TestGateOrder(t *testing.T) {
	order := GateOrder()
	if len(order) != 5 {
		t.Fatalf("expected 5 gates in order, got %d", len(order))
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Fatal("short string should not be truncated")
	}
	if len(truncate("hello world this is a long string", 5)) != 5 {
		t.Fatal("long string should be truncated to 5")
	}
}
