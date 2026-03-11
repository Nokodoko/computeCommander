package commands

import (
	"strings"
	"testing"
)

func TestPromptLineFormat(t *testing.T) {
	info := promptInfo{
		ProjectName:  "myproject",
		ProjectDir:   "/home/user/myproject",
		ActiveAgents: 3,
	}

	line := formatPromptLine(info)

	if !strings.Contains(line, "myproject") {
		t.Errorf("expected line to contain project name, got %q", line)
	}
	if !strings.Contains(line, "3 agents") {
		t.Errorf("expected line to contain agent count, got %q", line)
	}
	if !strings.Contains(line, "Sessions") {
		t.Errorf("expected line to contain key hints, got %q", line)
	}
}

func TestPromptLineFormatNoAgents(t *testing.T) {
	info := promptInfo{
		ProjectName:  "testproject",
		ProjectDir:   "/tmp/testproject",
		ActiveAgents: 0,
	}

	line := formatPromptLine(info)

	if !strings.Contains(line, "no agents") {
		t.Errorf("expected 'no agents' when count is 0, got %q", line)
	}
}

func TestPromptLineFormatEmpty(t *testing.T) {
	info := promptInfo{}
	line := formatPromptLine(info)
	// Should not panic and should contain key hints.
	if !strings.Contains(line, "Sessions") {
		t.Errorf("expected key hints in empty prompt line, got %q", line)
	}
}
