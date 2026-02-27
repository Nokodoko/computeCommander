package agents

import "testing"

func TestDefaultGuardRules(t *testing.T) {
	rules := DefaultGuardRules()

	if len(rules.Global.BlockedCommands) == 0 {
		t.Error("expected global blocked commands")
	}
	if len(rules.Global.BlockedPaths) == 0 {
		t.Error("expected global blocked paths")
	}

	// Verify all capabilities have rules.
	for _, cap := range AllCapabilities() {
		if _, ok := rules.ByCapability[cap]; !ok {
			t.Errorf("missing rules for capability %q", cap)
		}
	}
}

func TestIsAllowed_GlobalBlock(t *testing.T) {
	rules := DefaultGuardRules()

	allowed, reason := rules.IsAllowed(CapBuilder, "Bash", "git push --force origin main")
	if allowed {
		t.Error("expected global block for git push --force")
	}
	if reason == "" {
		t.Error("expected reason for denial")
	}
}

func TestIsAllowed_GlobalPathBlock(t *testing.T) {
	rules := DefaultGuardRules()

	allowed, _ := rules.IsAllowed(CapBuilder, "Write", ".git/config")
	if allowed {
		t.Error("expected block for .git/ path")
	}
}

func TestIsAllowed_ScoutReadOnly(t *testing.T) {
	rules := DefaultGuardRules()

	// Scout cannot write.
	allowed, reason := rules.IsAllowed(CapScout, "Write", "foo.go")
	if allowed {
		t.Errorf("scout should not be allowed Write, got reason: %s", reason)
	}

	// Scout cannot edit.
	allowed, reason = rules.IsAllowed(CapScout, "Edit", "foo.go")
	if allowed {
		t.Errorf("scout should not be allowed Edit, got reason: %s", reason)
	}

	// Scout can read.
	allowed, _ = rules.IsAllowed(CapScout, "Read", "foo.go")
	if !allowed {
		t.Error("scout should be allowed Read")
	}

	// Scout can grep.
	allowed, _ = rules.IsAllowed(CapScout, "Grep", "pattern")
	if !allowed {
		t.Error("scout should be allowed Grep")
	}
}

func TestIsAllowed_ScoutBashPatterns(t *testing.T) {
	rules := DefaultGuardRules()

	// Scout can run read-only bash commands.
	allowed, _ := rules.IsAllowed(CapScout, "Bash", "cat main.go")
	if !allowed {
		t.Error("scout should be allowed 'cat' via Bash")
	}

	// Scout cannot run destructive bash.
	allowed, _ = rules.IsAllowed(CapScout, "Bash", "rm -rf foo")
	if allowed {
		t.Error("scout should NOT be allowed 'rm' via Bash")
	}
}

func TestIsAllowed_BuilderScoped(t *testing.T) {
	rules := DefaultGuardRules()

	// Builder can use Write.
	allowed, _ := rules.IsAllowed(CapBuilder, "Write", "internal/foo.go")
	if !allowed {
		t.Error("builder should be allowed Write within scope")
	}

	// Builder cannot spawn.
	allowed, reason := rules.IsAllowed(CapBuilder, "Spawn", "scout-1")
	if allowed {
		t.Errorf("builder should not be allowed Spawn, got reason: %s", reason)
	}

	// Builder can use git add/commit.
	allowed, _ = rules.IsAllowed(CapBuilder, "Bash", "git add .")
	if !allowed {
		t.Error("builder should be allowed 'git add'")
	}

	// Builder cannot git push.
	allowed, _ = rules.IsAllowed(CapBuilder, "Bash", "git push origin main")
	if allowed {
		t.Error("builder should NOT be allowed 'git push'")
	}
}

func TestIsAllowed_LeadCanSpawn(t *testing.T) {
	rules := DefaultGuardRules()

	allowed, _ := rules.IsAllowed(CapLead, "Spawn", "builder-1")
	if !allowed {
		t.Error("lead should be allowed to Spawn")
	}

	allowed, _ = rules.IsAllowed(CapLead, "Write", "foo.go")
	if !allowed {
		t.Error("lead should be allowed Write")
	}
}

func TestIsAllowed_MergerGitPatterns(t *testing.T) {
	rules := DefaultGuardRules()

	// Merger can merge.
	allowed, _ := rules.IsAllowed(CapMerger, "Bash", "git merge feature-branch")
	if !allowed {
		t.Error("merger should be allowed 'git merge'")
	}

	// Merger cannot push.
	allowed, _ = rules.IsAllowed(CapMerger, "Bash", "git push origin main")
	if allowed {
		t.Error("merger should NOT be allowed 'git push'")
	}

	// Merger cannot spawn.
	allowed, _ = rules.IsAllowed(CapMerger, "Spawn", "foo")
	if allowed {
		t.Error("merger should NOT be allowed Spawn")
	}
}

func TestIsAllowed_CoordinatorReadOnlySpawn(t *testing.T) {
	rules := DefaultGuardRules()

	// Coordinator can spawn.
	allowed, _ := rules.IsAllowed(CapCoordinator, "Spawn", "lead-1")
	if !allowed {
		t.Error("coordinator should be allowed Spawn")
	}

	// Coordinator cannot write.
	allowed, _ = rules.IsAllowed(CapCoordinator, "Write", "foo.go")
	if allowed {
		t.Error("coordinator should NOT be allowed Write")
	}
}

func TestIsAllowed_MonitorReadOnly(t *testing.T) {
	rules := DefaultGuardRules()

	allowed, _ := rules.IsAllowed(CapMonitor, "Read", "foo.go")
	if !allowed {
		t.Error("monitor should be allowed Read")
	}

	allowed, _ = rules.IsAllowed(CapMonitor, "Write", "foo.go")
	if allowed {
		t.Error("monitor should NOT be allowed Write")
	}

	allowed, _ = rules.IsAllowed(CapMonitor, "Spawn", "scout-1")
	if allowed {
		t.Error("monitor should NOT be allowed Spawn")
	}
}

func TestIsAllowed_UnknownCapability(t *testing.T) {
	rules := DefaultGuardRules()

	allowed, _ := rules.IsAllowed(Capability("hacker"), "Bash", "ls")
	if allowed {
		t.Error("unknown capability should be denied")
	}
}
