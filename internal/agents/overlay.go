package agents

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/noko/computecommander/pkg/runtimes"
)

//go:embed templates/*.tmpl
var overlayFS embed.FS

// overlayData is the template context for overlay generation.
type overlayData struct {
	Capability  string
	TaskSpec    string
	Constraints []string
	Tools       overlayTools
}

type overlayTools struct {
	Allowed []string
	Blocked []string
}

// capabilityDefaults returns the default constraints and tool lists per capability.
func capabilityDefaults(cap Capability) (constraints []string, allowed, blocked []string) {
	switch cap {
	case CapScout:
		constraints = []string{"read_only", "no_spawn", "no_git_write"}
		allowed = []string{"Read", "Glob", "Grep", "Bash"}
		blocked = []string{"Write", "Edit", "Spawn"}
	case CapBuilder:
		constraints = []string{"file_scope_enforced", "no_spawn", "no_git_push"}
		allowed = []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"}
		blocked = []string{"Spawn"}
	case CapReviewer:
		constraints = []string{"read_only", "no_spawn", "no_git_write"}
		allowed = []string{"Read", "Glob", "Grep", "Bash"}
		blocked = []string{"Write", "Edit", "Spawn"}
	case CapLead:
		constraints = []string{"can_spawn"}
		allowed = []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "Spawn"}
		blocked = nil
	case CapMerger:
		constraints = []string{"merge_only", "no_spawn", "no_git_push"}
		allowed = []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash"}
		blocked = []string{"Spawn"}
	case CapCoordinator:
		constraints = []string{"read_only", "can_spawn"}
		allowed = []string{"Read", "Glob", "Grep", "Bash", "Spawn"}
		blocked = []string{"Write", "Edit"}
	case CapSupervisor:
		constraints = []string{"read_only", "can_spawn"}
		allowed = []string{"Read", "Glob", "Grep", "Bash", "Spawn"}
		blocked = []string{"Write", "Edit"}
	case CapMonitor:
		constraints = []string{"read_only", "no_spawn"}
		allowed = []string{"Read", "Glob", "Grep", "Bash"}
		blocked = []string{"Write", "Edit", "Spawn"}
	}
	return
}

// BuildOverlay generates an OverlayContent for the given capability and task spec path.
// It renders the embedded template with capability-specific defaults.
func BuildOverlay(cap Capability, taskSpec string) (*runtimes.OverlayContent, error) {
	if !ValidCapability(cap) {
		return nil, fmt.Errorf("build overlay: invalid capability %q", cap)
	}

	constraints, allowed, blocked := capabilityDefaults(cap)

	data := overlayData{
		Capability:  string(cap),
		TaskSpec:    taskSpec,
		Constraints: constraints,
		Tools: overlayTools{
			Allowed: allowed,
			Blocked: blocked,
		},
	}

	tmpl, err := template.ParseFS(overlayFS, "templates/overlay.tmpl")
	if err != nil {
		return nil, fmt.Errorf("build overlay parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("build overlay execute template: %w", err)
	}

	return &runtimes.OverlayContent{
		Content: buf.String(),
	}, nil
}
