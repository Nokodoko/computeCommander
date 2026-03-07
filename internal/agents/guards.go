package agents

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/noko/computecommander/internal/agentic/block"
)

// GuardRules defines tool enforcement rules matching spec 3.7.
type GuardRules struct {
	Global       GlobalRules                    `yaml:"global"`
	ByCapability map[Capability]CapabilityRules `yaml:"by_capability"`
	BlockEngine  *block.BlockRuleEngine         `yaml:"-"` // Agentic foundation block rule engine (nil-safe)
}

// GlobalRules are restrictions that apply to every agent.
type GlobalRules struct {
	BlockedCommands    []string `yaml:"blocked_commands"`
	BlockedPaths       []string `yaml:"blocked_paths"`
	DangerousPatterns  []string `yaml:"dangerous_patterns"`
}

// CapabilityRules are restrictions for a specific capability.
type CapabilityRules struct {
	Mode             string            `yaml:"mode"` // "read_only", "scoped_write", "full_write", "merge_only"
	CanSpawn         bool              `yaml:"can_spawn"`
	SpawnLimit       int               `yaml:"spawn_limit"`
	FileScopeEnforced bool             `yaml:"file_scope_enforced"`
	AllowedTools     []string          `yaml:"allowed_tools"`
	BlockedTools     []string          `yaml:"blocked_tools"`
	PreToolUse       map[string]ToolRule `yaml:"pre_tool_use"`
}

// ToolRule defines allow/deny patterns for a specific tool.
type ToolRule struct {
	Deny          bool     `yaml:"deny"`
	EnforceScope  bool     `yaml:"enforce_scope"`
	AllowPatterns []string `yaml:"allow_patterns"`
	DenyPatterns  []string `yaml:"deny_patterns"`
}

// DefaultGuardRules returns the spec 3.7 guard rules.
func DefaultGuardRules() *GuardRules {
	return &GuardRules{
		Global: GlobalRules{
			BlockedCommands: []string{
				"git push --force",
				"git reset --hard",
				"rm -rf /",
				"rm -rf ~",
				"sudo rm",
			},
			BlockedPaths: []string{
				".git/",
				".computecommander/",
				"/etc/",
				"/usr/",
			},
			DangerousPatterns: []string{
				`:()\{ :|:& \};:`,
				`> /dev/sd`,
			},
		},
		ByCapability: map[Capability]CapabilityRules{
			CapScout: {
				Mode:         "read_only",
				AllowedTools: []string{"Read", "Glob", "Grep", "Bash"},
				BlockedTools: []string{"Write", "Edit", "Spawn"},
				PreToolUse: map[string]ToolRule{
					"Write": {Deny: true},
					"Edit":  {Deny: true},
					"Bash": {
						AllowPatterns: []string{
							`^(cat|head|tail|less|grep|find|ls|tree|wc|file) `,
							`^git (status|log|diff|show|branch)`,
						},
						DenyPatterns: []string{
							`^(rm|mv|cp|mkdir|touch|chmod|chown)`,
							`>`,
							`\|.*>`,
						},
					},
				},
			},
			CapBuilder: {
				Mode:              "scoped_write",
				FileScopeEnforced: true,
				AllowedTools:      []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"},
				BlockedTools:      []string{"Spawn"},
				PreToolUse: map[string]ToolRule{
					"Write": {EnforceScope: true},
					"Edit":  {EnforceScope: true},
					"Bash": {
						AllowPatterns: []string{
							`^(go|npm|yarn|bun|cargo|make) `,
							`^git (add|commit|status|diff|log|show|branch)`,
						},
						DenyPatterns: []string{
							`^git (push|pull|fetch|reset)`,
						},
					},
				},
			},
			CapReviewer: {
				Mode:         "read_only",
				AllowedTools: []string{"Read", "Glob", "Grep", "Bash"},
				BlockedTools: []string{"Write", "Edit", "Spawn"},
				PreToolUse: map[string]ToolRule{
					"Write": {Deny: true},
					"Edit":  {Deny: true},
					"Bash": {
						AllowPatterns: []string{
							`^(cat|head|tail|less|grep|find|ls|tree|wc|file) `,
							`^git (status|log|diff|show|branch)`,
						},
						DenyPatterns: []string{
							`^(rm|mv|cp|mkdir|touch|chmod|chown)`,
							`>`,
							`\|.*>`,
						},
					},
				},
			},
			CapLead: {
				Mode:         "full_write",
				CanSpawn:     true,
				SpawnLimit:   5,
				AllowedTools: []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "Spawn"},
			},
			CapMerger: {
				Mode:         "merge_only",
				AllowedTools: []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"},
				BlockedTools: []string{"Spawn"},
				PreToolUse: map[string]ToolRule{
					"Bash": {
						AllowPatterns: []string{
							`^git (merge|rebase|cherry-pick|status|diff|log)`,
						},
						DenyPatterns: []string{
							`^git push`,
						},
					},
				},
			},
			CapCoordinator: {
				Mode:         "read_only",
				CanSpawn:     true,
				AllowedTools: []string{"Read", "Glob", "Grep", "Bash", "Spawn"},
				BlockedTools: []string{"Write", "Edit"},
			},
			CapSupervisor: {
				Mode:         "read_only",
				CanSpawn:     true,
				AllowedTools: []string{"Read", "Glob", "Grep", "Bash", "Spawn"},
				BlockedTools: []string{"Write", "Edit"},
			},
			CapMonitor: {
				Mode:         "read_only",
				AllowedTools: []string{"Read", "Glob", "Grep", "Bash"},
				BlockedTools: []string{"Write", "Edit", "Spawn"},
			},
		},
	}
}

// IsAllowed checks whether a tool invocation is permitted for the given capability.
// It returns (allowed bool, reason string). When allowed is false, reason explains why.
func (g *GuardRules) IsAllowed(cap Capability, tool string, args string) (bool, string) {
	// Check global blocked commands first.
	for _, blocked := range g.Global.BlockedCommands {
		if strings.Contains(args, blocked) {
			return false, fmt.Sprintf("global block: command %q is prohibited", blocked)
		}
	}

	// Check global blocked paths.
	for _, blockedPath := range g.Global.BlockedPaths {
		if strings.Contains(args, blockedPath) {
			return false, fmt.Sprintf("global block: path %q is prohibited", blockedPath)
		}
	}

	// Check global dangerous patterns.
	for _, pattern := range g.Global.DangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, args); matched {
			return false, fmt.Sprintf("global block: dangerous pattern %q matched", pattern)
		}
	}

	// Check capability-specific rules.
	capRules, ok := g.ByCapability[cap]
	if !ok {
		return false, fmt.Sprintf("no rules defined for capability %q", cap)
	}

	// Check blocked tools.
	for _, blocked := range capRules.BlockedTools {
		if tool == blocked {
			return false, fmt.Sprintf("%s: tool %q is blocked", cap, tool)
		}
	}

	// Check allowed tools (if the list is non-empty, the tool must be in it).
	if len(capRules.AllowedTools) > 0 {
		found := false
		for _, allowed := range capRules.AllowedTools {
			if tool == allowed {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("%s: tool %q is not in the allowed list", cap, tool)
		}
	}

	// Check pre-tool-use rules.
	if rule, hasRule := capRules.PreToolUse[tool]; hasRule {
		if rule.Deny {
			return false, fmt.Sprintf("%s: tool %q is denied by pre_tool_use rule", cap, tool)
		}

		// Check deny patterns.
		for _, pattern := range rule.DenyPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			if re.MatchString(args) {
				return false, fmt.Sprintf("%s: args match deny pattern %q for tool %q", cap, pattern, tool)
			}
		}

		// Check allow patterns (if specified, at least one must match).
		if len(rule.AllowPatterns) > 0 {
			matched := false
			for _, pattern := range rule.AllowPatterns {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if re.MatchString(args) {
					matched = true
					break
				}
			}
			if !matched {
				return false, fmt.Sprintf("%s: args do not match any allow pattern for tool %q", cap, tool)
			}
		}
	}

	// Check agentic foundation BlockRuleEngine (if initialized).
	// This runs after existing global blocks and capability rules.
	// Evaluation order: existing global blocks first, then capability rules,
	// then BlockRuleEngine.Evaluate(). The first failing check short-circuits.
	if g.BlockEngine != nil {
		input := &block.EvalInput{
			Tool:    tool,
			Command: args,
			AgentID: string(cap),
		}
		// For file-path tools, extract the file path from args
		if tool == "Read" || tool == "Write" || tool == "Edit" {
			input.FilePath = args
		}
		result := g.BlockEngine.Evaluate(context.Background(), input)
		if result.Matched && !result.Overridden && result.Action == block.ActionBlock {
			return false, fmt.Sprintf("block rule %q: %s", result.RuleID, result.Message)
		}
	}

	return true, ""
}
