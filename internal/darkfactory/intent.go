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

// HasOutputsHeading checks if the prompt contains an # Outputs/Outcomes heading.
func HasOutputsHeading(prompt string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		if isOutcomeHeading(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// Verify validates a prompt's # Outcomes section against project objectives.
func (v *IntentVerifier) Verify(prompt string) *VerifyResult {
	outcomes := extractPromptOutcomes(prompt)
	if len(outcomes) == 0 {
		if HasOutputsHeading(prompt) {
			return &VerifyResult{
				Valid:    false,
				Score:    0,
				Blockers: []string{"# Outputs heading found but no list items — use bullet (- ) or numbered (1. ) format"},
			}
		}
		return &VerifyResult{
			Valid:    false,
			Score:    0,
			Blockers: []string{"No # Outputs section found in prompt"},
		}
	}

	objectives := v.LoadObjectives()

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

// isOutcomeHeading checks if a line is an outcome/output section heading.
// Matches: # Outcome, # Outcomes, # Output, # Outputs (and ## variants).
func isOutcomeHeading(line string) bool {
	lower := strings.ToLower(line)
	// Strip leading #s and whitespace.
	for _, prefix := range []string{"### ", "## ", "# "} {
		if strings.HasPrefix(lower, prefix) {
			word := strings.TrimSpace(strings.TrimPrefix(lower, prefix))
			switch word {
			case "outcome", "outcomes", "output", "outputs":
				return true
			}
		}
	}
	return false
}

// extractPromptOutcomes parses the # Outcomes/Outputs section from a prompt.
// Accepts bullet lists (- / *) and numbered lists (1. / 2.).
func extractPromptOutcomes(prompt string) []string {
	var outcomes []string
	inOutcomes := false

	for _, line := range strings.Split(prompt, "\n") {
		trimmed := strings.TrimSpace(line)

		if isOutcomeHeading(trimmed) {
			inOutcomes = true
			continue
		}

		if inOutcomes && strings.HasPrefix(trimmed, "#") {
			break // Next section
		}

		if !inOutcomes {
			continue
		}

		// Bullet list: - item or * item
		if strings.HasPrefix(trimmed, "- ") {
			outcomes = append(outcomes, strings.TrimPrefix(trimmed, "- "))
			continue
		}
		if strings.HasPrefix(trimmed, "* ") {
			outcomes = append(outcomes, strings.TrimPrefix(trimmed, "* "))
			continue
		}

		// Numbered list: 1. item, 2. item, etc.
		if len(trimmed) > 2 {
			dotIdx := strings.Index(trimmed, ". ")
			if dotIdx > 0 && dotIdx <= 3 {
				prefix := trimmed[:dotIdx]
				allDigits := true
				for _, c := range prefix {
					if c < '0' || c > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					outcomes = append(outcomes, trimmed[dotIdx+2:])
					continue
				}
			}
		}
	}

	return outcomes
}

// LoadObjectives reads objective files from the objectives directory.
func (v *IntentVerifier) LoadObjectives() []string {
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
		score := KeywordOverlap(outcomeLower, objLower)
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

// KeywordOverlap computes a simple keyword overlap score between two strings.
func KeywordOverlap(a, b string) float64 {
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
