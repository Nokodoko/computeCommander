package block

import (
	"context"
	"fmt"
	"sync"

	"github.com/noko/computecommander/internal/platform/db"
)

// BlockRuleEngine loads, caches, and evaluates block rules.
type BlockRuleEngine struct {
	mu    sync.RWMutex
	rules []BlockRule
	db    db.DB
	rate  *RateLimiter
}

// NewBlockRuleEngine creates a new engine with the given database for rate limit persistence.
func NewBlockRuleEngine(database db.DB) *BlockRuleEngine {
	return &BlockRuleEngine{
		db:   database,
		rate: NewRateLimiter(database),
	}
}

// LoadFromFile loads and caches rules from a YAML file. Can be called multiple
// times to reload rules (e.g., on fsnotify events).
func (e *BlockRuleEngine) LoadFromFile(path string) error {
	rules, err := LoadRulesFromFile(path)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.rules = rules
	e.mu.Unlock()
	return nil
}

// LoadFromFiles loads rules from multiple YAML files, merging them in order.
// Later files override rules with the same ID from earlier files.
func (e *BlockRuleEngine) LoadFromFiles(paths ...string) error {
	var allRules []BlockRule
	seen := make(map[string]int) // rule ID -> index in allRules

	for _, path := range paths {
		rules, err := LoadRulesFromFile(path)
		if err != nil {
			// Skip missing files (e.g., custom.yaml may not exist)
			continue
		}
		for _, r := range rules {
			if idx, exists := seen[r.ID]; exists {
				allRules[idx] = r // Override
			} else {
				seen[r.ID] = len(allRules)
				allRules = append(allRules, r)
			}
		}
	}

	e.mu.Lock()
	e.rules = allRules
	e.mu.Unlock()
	return nil
}

// LoadRules directly sets the rules (useful for testing).
func (e *BlockRuleEngine) LoadRules(rules []BlockRule) {
	e.mu.Lock()
	e.rules = rules
	e.mu.Unlock()
}

// Evaluate checks all loaded rules against the given input.
// It returns the first matching blocking result, or a non-matched result if no rules fire.
func (e *BlockRuleEngine) Evaluate(ctx context.Context, input *EvalInput) *EvalResult {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Matches(input) {
			continue
		}

		// Check override grants
		if rule.IsOverridden(input.Grants) {
			return &EvalResult{
				Matched:    true,
				RuleID:     rule.ID,
				Action:     rule.Action,
				Message:    rule.Message,
				Severity:   rule.Severity,
				Overridden: true,
			}
		}

		// Check rate limiting
		if rule.HasRateLimit() {
			limited, err := e.rate.IsLimited(ctx, rule.ID, input.AgentID, rule.Match.CountWindow, rule.Match.CountMax)
			if err == nil && !limited {
				// Record the hit and allow
				_ = e.rate.Record(ctx, rule.ID, input.AgentID)
				continue // Not yet at limit
			}
			if err == nil && limited {
				return &EvalResult{
					Matched:     true,
					RuleID:      rule.ID,
					Action:      rule.Action,
					Message:     rule.Message,
					Severity:    rule.Severity,
					RateLimited: true,
				}
			}
			// On error, fall through to normal check
		}

		return &EvalResult{
			Matched:  true,
			RuleID:   rule.ID,
			Action:   rule.Action,
			Message:  rule.Message,
			Severity: rule.Severity,
		}
	}

	return &EvalResult{Matched: false}
}

// ListRules returns all currently loaded rules.
func (e *BlockRuleEngine) ListRules() []BlockRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]BlockRule, len(e.rules))
	copy(result, e.rules)
	return result
}

// GetRule returns a specific rule by ID.
func (e *BlockRuleEngine) GetRule(id string) (*BlockRule, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, r := range e.rules {
		if r.ID == id {
			rule := r
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("rule %q not found", id)
}

// EnableRule enables a rule by ID.
func (e *BlockRuleEngine) EnableRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == id {
			e.rules[i].Enabled = true
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// DisableRule disables a rule by ID.
func (e *BlockRuleEngine) DisableRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == id {
			e.rules[i].Enabled = false
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// RuleCount returns the number of loaded rules.
func (e *BlockRuleEngine) RuleCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.rules)
}
