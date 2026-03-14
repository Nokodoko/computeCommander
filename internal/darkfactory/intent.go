package darkfactory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IntentVerifier validates prompt outcomes against project objectives.
type IntentVerifier struct {
	ObjectivesDir string // ~/.claude/intent/
}

// VerifyResult holds the outcome of intent verification.
type VerifyResult struct {
	Valid    bool           `json:"valid"`
	Score    float64        `json:"score"`
	Outcomes []OutcomeCheck `json:"outcomes"`
	Blockers []string       `json:"blockers,omitempty"`
}

// OutcomeCheck represents a single outcome verification.
type OutcomeCheck struct {
	Text    string  `json:"text"`
	Type    string  `json:"type"`
	Aligned bool    `json:"aligned"`
	Score   float64 `json:"score"`
	Reason  string  `json:"reason,omitempty"`
}

// NewIntentVerifier creates an IntentVerifier with the given objectives directory.
func NewIntentVerifier(objectivesDir string) *IntentVerifier {
	if objectivesDir == "" {
		home, _ := os.UserHomeDir()
		objectivesDir = filepath.Join(home, ".claude", "intent")
	}
	return &IntentVerifier{ObjectivesDir: objectivesDir}
}

// Verify validates a prompt's # Outcomes section against project objectives.
func (v *IntentVerifier) Verify(prompt string) *VerifyResult {
	outcomes := extractPromptOutcomes(prompt)
	if len(outcomes) == 0 {
		return &VerifyResult{
			Valid:    false,
			Score:    0,
			Blockers: []string{"No # Outcomes section found in prompt"},
		}
	}

	objectives := v.loadObjectives()

	var checks []OutcomeCheck
	var totalScore float64

	for _, outcome := range outcomes {
		check := v.classifyAndScore(outcome, objectives)
		checks = append(checks, check)
		totalScore += check.Score
	}

	avgScore := totalScore / float64(len(checks))
	valid := avgScore >= 0.7

	result := &VerifyResult{
		Valid:    valid,
		Score:    avgScore,
		Outcomes: checks,
	}

	if !valid {
		result.Blockers = append(result.Blockers,
			fmt.Sprintf("Average alignment score %.2f is below threshold 0.70", avgScore))
	}

	return result
}

// extractPromptOutcomes parses the # Outcomes section from a prompt.
func extractPromptOutcomes(prompt string) []string {
	var outcomes []string
	inOutcomes := false

	for _, line := range strings.Split(prompt, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Outcomes") || strings.HasPrefix(trimmed, "# Outcomes") {
			inOutcomes = true
			continue
		}

		if inOutcomes && strings.HasPrefix(trimmed, "#") {
			break // Next section
		}

		if inOutcomes && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
			outcome := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			outcomes = append(outcomes, outcome)
		}
	}

	return outcomes
}

// loadObjectives reads objective files from the objectives directory.
func (v *IntentVerifier) loadObjectives() []string {
	var objectives []string

	paths := []string{
		filepath.Join(v.ObjectivesDir, "personal", "objectives.md"),
		filepath.Join(v.ObjectivesDir, "work", "objectives.md"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				objectives = append(objectives, strings.TrimPrefix(line, "- "))
			}
		}
	}

	return objectives
}

// classifyAndScore classifies an outcome and scores its alignment with objectives.
func (v *IntentVerifier) classifyAndScore(outcome string, objectives []string) OutcomeCheck {
	predType := classifyPredicate(outcome)

	// If no objectives are loaded, give a neutral score.
	if len(objectives) == 0 {
		return OutcomeCheck{
			Text:    outcome,
			Type:    predType,
			Aligned: true,
			Score:   0.8,
			Reason:  "No objectives loaded; default pass",
		}
	}

	// Simple keyword matching for alignment scoring.
	bestScore := 0.0
	bestReason := "No matching objective found"

	outcomeLower := strings.ToLower(outcome)
	for _, obj := range objectives {
		objLower := strings.ToLower(obj)
		score := keywordOverlap(outcomeLower, objLower)
		if score > bestScore {
			bestScore = score
			bestReason = fmt.Sprintf("Aligned with: %s", obj)
		}
	}

	// Minimum score of 0.5 for well-formed outcomes.
	if bestScore < 0.5 && isWellFormed(outcome) {
		bestScore = 0.5
		bestReason = "Well-formed outcome, weak objective alignment"
	}

	return OutcomeCheck{
		Text:    outcome,
		Type:    predType,
		Aligned: bestScore >= 0.7,
		Score:   bestScore,
		Reason:  bestReason,
	}
}

// classifyPredicate determines the predicate type for an outcome.
func classifyPredicate(outcome string) string {
	lower := strings.ToLower(outcome)

	switch {
	case strings.Contains(lower, "test") && strings.Contains(lower, "pass"):
		return "contains_pattern"
	case strings.Contains(lower, "no ") || strings.Contains(lower, "never") || strings.Contains(lower, "without"):
		return "negation_check"
	case strings.Contains(lower, "at least") || strings.Contains(lower, "count") || strings.Contains(lower, "number"):
		return "count_check"
	case strings.Contains(lower, "function") || strings.Contains(lower, "struct") || strings.Contains(lower, "interface"):
		return "structural_check"
	case strings.Contains(lower, "import") || strings.Contains(lower, "unused"):
		return "ast_check"
	default:
		return "semantic_check"
	}
}

// keywordOverlap computes a simple keyword overlap score between two strings.
func keywordOverlap(a, b string) float64 {
	aWords := strings.Fields(a)
	bWords := strings.Fields(b)

	if len(aWords) == 0 || len(bWords) == 0 {
		return 0
	}

	bSet := make(map[string]bool)
	for _, w := range bWords {
		if len(w) > 2 { // Skip short words.
			bSet[w] = true
		}
	}

	matches := 0
	total := 0
	for _, w := range aWords {
		if len(w) > 2 {
			total++
			if bSet[w] {
				matches++
			}
		}
	}

	if total == 0 {
		return 0
	}
	return float64(matches) / float64(total)
}

// isWellFormed checks if an outcome looks like a testable assertion.
func isWellFormed(outcome string) bool {
	lower := strings.ToLower(outcome)
	actionWords := []string{"pass", "fail", "return", "respond", "create", "update",
		"delete", "validate", "include", "contain", "support", "handle"}
	for _, w := range actionWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
