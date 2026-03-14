package jira

import (
	"strings"
	"testing"
)

func TestPromptGenerate(t *testing.T) {
	pg := NewPromptGenerator("") // Use default template.

	issue := &JiraIssue{
		Key:                "ENG-142",
		Summary:            "Add OAuth2 token refresh",
		Description:        "Implement the OAuth2 token refresh flow.",
		AcceptanceCriteria: "- Tests pass for token refresh\n- No regressions in auth",
		Priority:           "High",
		Labels:             []string{"auth", "security"},
		IssueType:          "Story",
		Status:             "To Do",
	}

	result, err := pg.Generate(issue, "ENG-100", "Authentication Epic", "ENG")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Check that prompt contains key sections.
	if !strings.Contains(result.Prompt, "ENG-142") {
		t.Error("prompt should contain issue key")
	}
	if !strings.Contains(result.Prompt, "Add OAuth2 token refresh") {
		t.Error("prompt should contain summary")
	}
	if !strings.Contains(result.Prompt, "Outcomes") {
		t.Error("prompt should contain Outcomes section")
	}
	if !strings.Contains(result.Prompt, "ENG-100") {
		t.Error("prompt should contain epic key")
	}
	if !strings.Contains(result.Prompt, "auth, security") {
		t.Error("prompt should contain labels")
	}

	// Check outcomes were extracted.
	if len(result.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(result.Outcomes))
	}

	// Check hash is deterministic.
	result2, _ := pg.Generate(issue, "ENG-100", "Authentication Epic", "ENG")
	if result.PromptHash != result2.PromptHash {
		t.Error("same input should produce same hash")
	}
}

func TestPromptDeterministic(t *testing.T) {
	pg := NewPromptGenerator("")

	issue := &JiraIssue{
		Key:     "TEST-1",
		Summary: "Test determinism",
		Labels:  []string{},
	}

	r1, _ := pg.Generate(issue, "", "", "TEST")
	r2, _ := pg.Generate(issue, "", "", "TEST")

	if r1.PromptHash != r2.PromptHash {
		t.Error("prompt generation should be deterministic")
	}
	if r1.Prompt != r2.Prompt {
		t.Error("prompt text should be identical for same input")
	}
}

func TestExtractOutcomes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"bullet points", "- Outcome 1\n- Outcome 2\n- Outcome 3", 3},
		{"asterisk points", "* Outcome A\n* Outcome B", 2},
		{"mixed", "- Dash\n* Star", 2},
		{"no bullets", "Just a plain criteria", 1},
		{"empty", "", 0},
		{"with extra lines", "Header\n- Actual outcome\nFooter", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcomes := extractOutcomes(tt.input)
			if len(outcomes) != tt.expected {
				t.Errorf("expected %d outcomes, got %d: %v", tt.expected, len(outcomes), outcomes)
			}
		})
	}
}

func TestPromptWithEmptyFields(t *testing.T) {
	pg := NewPromptGenerator("")

	issue := &JiraIssue{
		Key:     "MIN-1",
		Summary: "Minimal issue",
		Labels:  []string{},
	}

	result, err := pg.Generate(issue, "", "", "")
	if err != nil {
		t.Fatalf("Generate with minimal fields: %v", err)
	}
	if result.Prompt == "" {
		t.Error("prompt should not be empty")
	}
	if result.PromptHash == "" {
		t.Error("hash should not be empty")
	}
}
