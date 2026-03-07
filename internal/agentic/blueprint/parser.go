package blueprint

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseFile parses a blueprint YAML file from disk.
func ParseFile(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read blueprint %s: %w", path, err)
	}
	return Parse(data)
}

// Parse parses blueprint YAML data.
func Parse(data []byte) (*Blueprint, error) {
	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parse blueprint YAML: %w", err)
	}

	if err := Validate(&bp); err != nil {
		return nil, fmt.Errorf("validate blueprint: %w", err)
	}

	// Apply defaults
	if bp.Version == 0 {
		bp.Version = 1
	}
	if bp.RetryLimit == 0 {
		bp.RetryLimit = 3
	}
	if bp.Timeout == "" {
		bp.Timeout = "30m"
	}
	if bp.Status == "" {
		bp.Status = StatusPending
	}
	if bp.CreatedAt.IsZero() {
		bp.CreatedAt = time.Now().UTC()
	}
	if bp.UpdatedAt.IsZero() {
		bp.UpdatedAt = bp.CreatedAt
	}
	if bp.Gates == nil {
		bp.Gates = []string{}
	}
	if bp.DependsOn == nil {
		bp.DependsOn = []string{}
	}
	if bp.ContextGrants == nil {
		bp.ContextGrants = []ContextGrant{}
	}
	if bp.VerifySteps == nil {
		bp.VerifySteps = []VerifyStep{}
	}

	return &bp, nil
}

// Validate checks that a blueprint has all required fields.
func Validate(bp *Blueprint) error {
	if bp.Name == "" {
		return fmt.Errorf("blueprint name is required")
	}
	if bp.Agent == "" {
		return fmt.Errorf("blueprint agent is required")
	}
	if bp.Capability == "" {
		return fmt.Errorf("blueprint capability is required")
	}

	// Validate context grants
	for i, g := range bp.ContextGrants {
		if g.Action != "read" && g.Action != "write" {
			return fmt.Errorf("context[%d].action must be 'read' or 'write', got %q", i, g.Action)
		}
		if g.Path == "" {
			return fmt.Errorf("context[%d].path is required", i)
		}
	}

	// Validate verify steps
	for i, v := range bp.VerifySteps {
		if v.Command == "" {
			return fmt.Errorf("verify[%d].command is required", i)
		}
		switch v.Expect {
		case "exit_0", "contains", "not_contains", "regex":
			// Valid
		default:
			return fmt.Errorf("verify[%d].expect must be exit_0|contains|not_contains|regex, got %q", i, v.Expect)
		}
		if v.Expect != "exit_0" && v.Value == "" {
			return fmt.Errorf("verify[%d].value is required for expect=%q", i, v.Expect)
		}
	}

	// Validate gates
	validGates := map[string]bool{"lint": true, "typecheck": true, "test": true, "security": true, "format": true}
	for i, g := range bp.Gates {
		if !validGates[g] {
			return fmt.Errorf("gates[%d] must be lint|typecheck|test|security|format, got %q", i, g)
		}
	}

	return nil
}

// Serialize converts a blueprint to YAML bytes.
func Serialize(bp *Blueprint) ([]byte, error) {
	return yaml.Marshal(bp)
}
