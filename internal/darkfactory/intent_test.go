package darkfactory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyWithOutcomes(t *testing.T) {
	verifier := NewIntentVerifier(t.TempDir())

	prompt := `# TEST-1: Test issue

Some description.

## Outcomes

- Tests pass for all new code
- API responds with 200 on valid requests
- No security vulnerabilities introduced

## Constraints

- Project: TEST
`
	result := verifier.Verify(prompt)
	if len(result.Outcomes) != 3 {
		t.Errorf("expected 3 outcomes, got %d", len(result.Outcomes))
	}

	// With no objectives loaded, outcomes should get default pass scores.
	if !result.Valid {
		t.Errorf("expected valid result with no objectives (default pass), got invalid with score %.2f", result.Score)
	}
}

func TestVerifyNoOutcomes(t *testing.T) {
	verifier := NewIntentVerifier(t.TempDir())

	prompt := `# TEST-1: No outcomes section

Just a description without outcomes.
`
	result := verifier.Verify(prompt)
	if result.Valid {
		t.Error("expected invalid result when no outcomes section exists")
	}
	if len(result.Blockers) == 0 {
		t.Error("expected blockers when no outcomes found")
	}
}

func TestVerifyWithObjectives(t *testing.T) {
	tmpDir := t.TempDir()
	personalDir := filepath.Join(tmpDir, "personal")
	os.MkdirAll(personalDir, 0755)

	// Write objectives with overlapping keywords.
	os.WriteFile(filepath.Join(personalDir, "objectives.md"), []byte(`
# Objectives
- Tests pass for all new code
- Code follows security best practices
- API endpoints validate input
`), 0644)

	verifier := NewIntentVerifier(tmpDir)

	prompt := `# TEST-1: Auth feature

Description.

## Outcomes

- Tests pass for all new code
- API responds with 200 on valid requests

## Constraints

- Project: TEST
`
	result := verifier.Verify(prompt)
	if len(result.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(result.Outcomes))
	}

	// First outcome should have high alignment (exact match with objective).
	if result.Outcomes[0].Score < 0.5 {
		t.Errorf("expected high score for exact-match outcome, got %.2f", result.Outcomes[0].Score)
	}
}

func TestClassifyPredicate(t *testing.T) {
	cases := []struct {
		outcome string
		want    string
	}{
		{"Tests pass for all code", "contains_pattern"},
		{"No hardcoded secrets", "negation_check"},
		{"At least 3 test cases", "count_check"},
		{"Function has docstring", "structural_check"},
		{"No unused imports", "negation_check"},
		{"Follows RESTful conventions", "semantic_check"},
	}

	for _, tc := range cases {
		got := classifyPredicate(tc.outcome)
		if got != tc.want {
			t.Errorf("classifyPredicate(%q) = %s, want %s", tc.outcome, got, tc.want)
		}
	}
}

func TestExtractPromptOutcomes(t *testing.T) {
	prompt := `# Title

## Outcomes

- First outcome
- Second outcome
- Third outcome

## Constraints

- Some constraint
`
	outcomes := extractPromptOutcomes(prompt)
	if len(outcomes) != 3 {
		t.Errorf("expected 3 outcomes, got %d: %v", len(outcomes), outcomes)
	}
	if outcomes[0] != "First outcome" {
		t.Errorf("expected 'First outcome', got %s", outcomes[0])
	}
}

func TestExtractPromptOutcomesEmpty(t *testing.T) {
	prompt := "# Title\n\nNo outcomes section here."
	outcomes := extractPromptOutcomes(prompt)
	if len(outcomes) != 0 {
		t.Errorf("expected 0 outcomes, got %d", len(outcomes))
	}
}

func TestKeywordOverlap(t *testing.T) {
	score := keywordOverlap("tests pass for all new code", "tests pass for all new code")
	if score < 0.9 {
		t.Errorf("identical strings should have high overlap, got %.2f", score)
	}

	score = keywordOverlap("completely different text here", "nothing in common at all")
	if score > 0.3 {
		t.Errorf("unrelated strings should have low overlap, got %.2f", score)
	}
}

func TestIsWellFormed(t *testing.T) {
	if !isWellFormed("Tests pass for all code") {
		t.Error("should be well-formed")
	}
	if !isWellFormed("API responds with 200") {
		t.Error("should be well-formed")
	}
	if isWellFormed("xyz abc") {
		t.Error("should not be well-formed")
	}
}
