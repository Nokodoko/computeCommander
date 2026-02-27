package agents

import (
	"strings"
	"testing"
)

func TestBuildOverlay_AllCapabilities(t *testing.T) {
	for _, cap := range AllCapabilities() {
		t.Run(string(cap), func(t *testing.T) {
			overlay, err := BuildOverlay(cap, "/tmp/spec.md")
			if err != nil {
				t.Fatalf("BuildOverlay(%q) error: %v", cap, err)
			}
			if overlay == nil {
				t.Fatal("overlay is nil")
			}
			if overlay.Content == "" {
				t.Fatal("overlay content is empty")
			}

			// Verify capability name appears in output.
			if !strings.Contains(overlay.Content, string(cap)) {
				t.Errorf("overlay content does not contain capability %q", cap)
			}
		})
	}
}

func TestBuildOverlay_InvalidCapability(t *testing.T) {
	_, err := BuildOverlay(Capability("hacker"), "")
	if err == nil {
		t.Error("expected error for invalid capability")
	}
}

func TestBuildOverlay_ContainsTaskSpec(t *testing.T) {
	overlay, err := BuildOverlay(CapBuilder, "/path/to/spec.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay.Content, "/path/to/spec.md") {
		t.Error("overlay should contain the task spec path")
	}
}

func TestBuildOverlay_ScoutHasReadOnly(t *testing.T) {
	overlay, err := BuildOverlay(CapScout, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay.Content, "read_only") {
		t.Error("scout overlay should mention read_only constraint")
	}
}

func TestBuildOverlay_BuilderHasWriteTools(t *testing.T) {
	overlay, err := BuildOverlay(CapBuilder, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay.Content, "Write") {
		t.Error("builder overlay should list Write tool")
	}
	if !strings.Contains(overlay.Content, "Edit") {
		t.Error("builder overlay should list Edit tool")
	}
}

func TestBuildOverlay_EmptyTaskSpec(t *testing.T) {
	overlay, err := BuildOverlay(CapReviewer, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still render without the task spec section header.
	if overlay.Content == "" {
		t.Error("overlay should not be empty even without task spec")
	}
}

func TestCapabilityDefaults(t *testing.T) {
	// Scout should have read-only constraints and no Write/Edit.
	constraints, allowed, blocked := capabilityDefaults(CapScout)
	if len(constraints) == 0 {
		t.Error("scout should have constraints")
	}
	hasReadOnly := false
	for _, c := range constraints {
		if c == "read_only" {
			hasReadOnly = true
		}
	}
	if !hasReadOnly {
		t.Error("scout constraints should include read_only")
	}
	if len(allowed) == 0 {
		t.Error("scout should have allowed tools")
	}
	if len(blocked) == 0 {
		t.Error("scout should have blocked tools")
	}

	// Lead should be able to spawn.
	constraints, allowed, _ = capabilityDefaults(CapLead)
	hasCanSpawn := false
	for _, c := range constraints {
		if c == "can_spawn" {
			hasCanSpawn = true
		}
	}
	if !hasCanSpawn {
		t.Error("lead constraints should include can_spawn")
	}
	hasSpawnTool := false
	for _, a := range allowed {
		if a == "Spawn" {
			hasSpawnTool = true
		}
	}
	if !hasSpawnTool {
		t.Error("lead allowed tools should include Spawn")
	}
}
