// Package block provides the declarative hard block rule engine for preventing
// dangerous operations at the tool boundary. Rules are loaded from YAML files
// and evaluated before every tool invocation.
package block

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Action determines what happens when a rule matches.
type Action string

const (
	ActionBlock Action = "block"
	ActionWarn  Action = "warn"
)

// Severity indicates the importance of a block rule.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// OverrideType indicates whether a block rule can be overridden.
type OverrideType string

const (
	OverrideGrant OverrideType = "grant"
	OverrideNone  OverrideType = "none"
)

// BlockRule defines a single block rule matching tool invocations.
type BlockRule struct {
	ID          string       `yaml:"id" json:"id"`
	Description string       `yaml:"description" json:"description"`
	Tool        string       `yaml:"tool" json:"tool"`
	Match       BlockMatch   `yaml:"match" json:"match"`
	Action      Action       `yaml:"action" json:"action"`
	Message     string       `yaml:"message" json:"message"`
	Severity    Severity     `yaml:"severity" json:"severity"`
	Override    OverrideType `yaml:"override,omitempty" json:"override,omitempty"`
	Enabled     bool         `yaml:"-" json:"enabled"`

	// Compiled regex patterns (not serialized)
	commandRe  *regexp.Regexp
	filePathRe *regexp.Regexp
}

// BlockMatch defines the conditions that trigger a rule.
type BlockMatch struct {
	Command     string `yaml:"command,omitempty" json:"command,omitempty"`
	FilePath    string `yaml:"file_path,omitempty" json:"file_path,omitempty"`
	Depth       string `yaml:"depth,omitempty" json:"depth,omitempty"`
	CountWindow string `yaml:"count_window,omitempty" json:"count_window,omitempty"`
	CountMax    int    `yaml:"count_max,omitempty" json:"count_max,omitempty"`
}

// RuleFile represents the on-disk YAML rule file format.
type RuleFile struct {
	Version int         `yaml:"version"`
	Rules   []BlockRule `yaml:"rules"`
}

// EvalInput provides the context for evaluating a block rule.
type EvalInput struct {
	Tool      string   // Tool name (Bash, Read, Write, etc.)
	Command   string   // For Bash: the shell command
	FilePath  string   // For Read/Write/Edit: the file path
	AgentID   string   // Agent session ID
	AgentName string   // Human-readable agent name
	Depth     int      // Agent spawn depth
	Grants    []string // Explicit grants from isolation manifest
}

// EvalResult is the outcome of evaluating a block rule against an input.
type EvalResult struct {
	Matched     bool         `json:"matched"`
	RuleID      string       `json:"rule_id,omitempty"`
	Action      Action       `json:"action,omitempty"`
	Message     string       `json:"message,omitempty"`
	Severity    Severity     `json:"severity,omitempty"`
	Overridden  bool         `json:"overridden,omitempty"`
	RateLimited bool         `json:"rate_limited,omitempty"`
}

// LoadRulesFromFile loads block rules from a YAML file.
func LoadRulesFromFile(path string) ([]BlockRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules file %s: %w", path, err)
	}
	return ParseRules(data)
}

// ParseRules parses YAML block rule data.
func ParseRules(data []byte) ([]BlockRule, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse rules YAML: %w", err)
	}

	for i := range rf.Rules {
		rf.Rules[i].Enabled = true
		if err := rf.Rules[i].compile(); err != nil {
			return nil, fmt.Errorf("compile rule %q: %w", rf.Rules[i].ID, err)
		}
	}

	return rf.Rules, nil
}

// compile pre-compiles regex patterns for performance.
func (r *BlockRule) compile() error {
	if r.Match.Command != "" {
		re, err := regexp.Compile(r.Match.Command)
		if err != nil {
			return fmt.Errorf("compile command pattern %q: %w", r.Match.Command, err)
		}
		r.commandRe = re
	}
	if r.Match.FilePath != "" {
		re, err := regexp.Compile(r.Match.FilePath)
		if err != nil {
			return fmt.Errorf("compile file_path pattern %q: %w", r.Match.FilePath, err)
		}
		r.filePathRe = re
	}
	return nil
}

// Matches checks if the rule matches the given evaluation input.
func (r *BlockRule) Matches(input *EvalInput) bool {
	if !r.Enabled {
		return false
	}

	// Tool name must match
	if r.Tool != input.Tool {
		return false
	}

	// Check command pattern
	if r.commandRe != nil {
		if !r.commandRe.MatchString(input.Command) {
			return false
		}
	}

	// Check file path pattern
	if r.filePathRe != nil {
		if !r.filePathRe.MatchString(input.FilePath) {
			return false
		}
	}

	// Check depth
	if r.Match.Depth != "" {
		maxDepth := parseDepthThreshold(r.Match.Depth)
		if maxDepth > 0 && input.Depth <= maxDepth {
			return false
		}
	}

	return true
}

// CanOverride checks if this rule can be overridden by an explicit grant.
func (r *BlockRule) CanOverride() bool {
	return r.Override == OverrideGrant
}

// IsOverridden checks if the rule is overridden by any of the given grants.
func (r *BlockRule) IsOverridden(grants []string) bool {
	if !r.CanOverride() {
		return false
	}
	for _, g := range grants {
		if g == r.ID || g == "*" {
			return true
		}
	}
	return false
}

// HasRateLimit returns true if this rule has rate limiting configured.
func (r *BlockRule) HasRateLimit() bool {
	return r.Match.CountWindow != "" && r.Match.CountMax > 0
}

// ParseCountWindow parses the count_window duration string.
func (r *BlockRule) ParseCountWindow() (time.Duration, error) {
	if r.Match.CountWindow == "" {
		return 0, fmt.Errorf("no count_window configured")
	}
	return parseDuration(r.Match.CountWindow)
}

// parseDepthThreshold parses a depth string like ">5" and returns the threshold.
func parseDepthThreshold(s string) int {
	s = strings.TrimPrefix(s, ">")
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// parseDuration parses duration strings like "1h", "30m", "5s".
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}
