package jiraboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandBasic(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Create a minimal intake.
	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Test Onboarding",
			ProjectKey:       "TST",
			Environments:     []string{"prod", "staging"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(tickets) == 0 {
		t.Fatal("expected tickets, got none")
	}

	// Count by type.
	epicCount, storyCount, taskCount := 0, 0, 0
	for _, ticket := range tickets {
		switch ticket.IssueType {
		case "Epic":
			epicCount++
		case "Story":
			storyCount++
		case "Task":
			taskCount++
		}
	}

	if epicCount == 0 {
		t.Error("expected at least one epic")
	}
	if storyCount == 0 {
		t.Error("expected at least one story")
	}
	if taskCount == 0 {
		t.Error("expected at least one task")
	}

	t.Logf("Expanded: %d epics, %d stories, %d tasks (total: %d)",
		epicCount, storyCount, taskCount, len(tickets))
}

func TestExpandEnvironmentPseudoDimension(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	envs := []string{"prod-us-east-1", "staging-us-east-1", "dev"}
	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Env Test",
			ProjectKey:       "ENV",
			Environments:     envs,
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// Find agent-install tasks — should have one per environment.
	agentInstallTasks := 0
	for _, ticket := range tickets {
		if ticket.IssueType == "Task" && ticket.TrackID == "agent-deploy" {
			agentInstallTasks++
		}
	}

	// Should be: 1 OS * 1 cloud * 3 environments = 3 tasks.
	if agentInstallTasks != len(envs) {
		t.Errorf("expected %d agent-deploy tasks (one per env), got %d", len(envs), agentInstallTasks)
	}
}

func TestExpandExcludeWhen(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Include macOS + on-prem, which should be excluded by the agent-deploy story.
	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Exclude Test",
			ProjectKey:       "EXC",
			Environments:     []string{"prod"},
			CloudProviders:   []string{"on-prem"},
			OperatingSystems: []string{"macos"},
			Databases:        []string{},
			AppArchitectures: []string{"vm-based"},
			StorageSystems:   []string{},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// Agent-deploy story for macos+on-prem should be excluded.
	for _, ticket := range tickets {
		if ticket.IssueType == "Story" && ticket.TrackID == "agent-deploy" {
			if dims := ticket.Dimensions; dims != nil {
				os, hasOS := dims["os"]
				cp, hasCP := dims["cloud_provider"]
				if hasOS && hasCP && os.ID == "macos" && cp.ID == "on-prem" {
					t.Error("macos + on-prem agent-deploy story should be excluded")
				}
			}
		}
	}
}

func TestExpandDimensionPruning(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Only select postgres — redis/mysql/etc should not appear.
	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Prune Test",
			ProjectKey:       "PRN",
			Environments:     []string{"prod"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	for _, ticket := range tickets {
		if ticket.TrackID == "db-monitoring" {
			if db, ok := ticket.Dimensions["database"]; ok {
				if db.ID != "postgres" {
					t.Errorf("expected only postgres in pruned output, got %s", db.ID)
				}
			}
		}
	}
}

func TestExpandTrackExclusion(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Track Exclude Test",
			ProjectKey:       "TEX",
			Environments:     []string{"prod"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
			ExcludeTracks:    []string{"apm-instrumentation"},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	for _, ticket := range tickets {
		if ticket.TrackID == "apm-instrumentation" {
			t.Error("apm-instrumentation track should be excluded")
		}
	}
}

func TestPreview(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Preview Test",
			ProjectKey:       "PVW",
			Environments:     []string{"prod", "staging"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres", "redis"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)
	preview, err := expander.Preview()
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if preview.TotalTickets == 0 {
		t.Error("expected non-zero total_tickets")
	}
	if len(preview.ByType) == 0 {
		t.Error("expected non-empty by_type")
	}
	if len(preview.ByPhase) == 0 {
		t.Error("expected non-empty by_phase")
	}

	t.Logf("Preview: total=%d, by_type=%v, by_phase=%v",
		preview.TotalTickets, preview.ByType, preview.ByPhase)
}

func TestDeterministicKeys(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Deterministic Test",
			ProjectKey:       "DET",
			Environments:     []string{"prod"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)

	// Run twice — keys should be identical.
	tickets1, err := expander.Expand()
	if err != nil {
		t.Fatalf("first Expand failed: %v", err)
	}

	tickets2, err := expander.Expand()
	if err != nil {
		t.Fatalf("second Expand failed: %v", err)
	}

	if len(tickets1) != len(tickets2) {
		t.Fatalf("ticket count mismatch: %d vs %d", len(tickets1), len(tickets2))
	}

	for i := range tickets1 {
		if tickets1[i].DeterministicKey != tickets2[i].DeterministicKey {
			t.Errorf("key mismatch at %d: %s vs %s",
				i, tickets1[i].DeterministicKey, tickets2[i].DeterministicKey)
		}
	}

	// Also check no duplicate keys.
	seen := make(map[string]bool)
	for _, ticket := range tickets1 {
		if seen[ticket.DeterministicKey] {
			t.Errorf("duplicate deterministic key: %s", ticket.DeterministicKey)
		}
		seen[ticket.DeterministicKey] = true
	}
}

func TestNoUnresolvedPlaceholders(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName:      "Placeholder Test",
			ProjectKey:       "PLH",
			Environments:     []string{"prod"},
			CloudProviders:   []string{"aws"},
			OperatingSystems: []string{"linux"},
			Databases:        []string{"postgres"},
			AppArchitectures: []string{"containerized"},
			StorageSystems:   []string{"s3"},
		},
	}

	expander := NewExpander(tmpl, intake)
	tickets, err := expander.Expand()
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	for _, ticket := range tickets {
		// Check summary for unresolved placeholders.
		if containsPlaceholder(ticket.Summary) {
			t.Errorf("unresolved placeholder in summary: %s (key: %s)",
				ticket.Summary, ticket.DeterministicKey)
		}
	}
}

// containsPlaceholder checks for {word} patterns that look like unresolved placeholders.
func containsPlaceholder(s string) bool {
	inBrace := false
	braceContent := ""
	for _, c := range s {
		if c == '{' {
			inBrace = true
			braceContent = ""
			continue
		}
		if c == '}' && inBrace {
			// Check if content looks like a placeholder (letters, underscores, dots).
			if isPlaceholderContent(braceContent) {
				return true
			}
			inBrace = false
			continue
		}
		if inBrace {
			braceContent += string(c)
		}
	}
	return false
}

func isPlaceholderContent(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func TestValidateIntakeAgainstTemplate(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Valid intake.
	intake := &IntakeFile{
		Intake: IntakeConfig{
			ProjectName: "Valid Test",
			Databases:   []string{"postgres"},
		},
	}
	warnings := ValidateIntakeAgainstTemplate(intake, tmpl)
	if len(warnings) > 0 {
		t.Errorf("expected no warnings for valid intake, got: %v", warnings)
	}

	// Invalid intake with unknown database.
	intake = &IntakeFile{
		Intake: IntakeConfig{
			ProjectName: "Invalid Test",
			Databases:   []string{"oracle"},
		},
	}
	warnings = ValidateIntakeAgainstTemplate(intake, tmpl)
	if len(warnings) == 0 {
		t.Error("expected warning for unknown database 'oracle'")
	}
}

func TestTaskAppliesWhen(t *testing.T) {
	// DBM task should only apply for postgres, mysql, sqlserver.
	task := TaskTemplate{
		AppliesWhen: map[string][]string{
			"database": {"postgres", "mysql", "sqlserver"},
		},
	}

	// Create a minimal expander.
	expander := &Expander{}

	// Should apply for postgres.
	dims := map[string]DimensionValue{
		"database": {ID: "postgres", Label: "PostgreSQL"},
	}
	if !expander.taskApplies(task, dims) {
		t.Error("expected task to apply for postgres")
	}

	// Should not apply for mongodb.
	dims = map[string]DimensionValue{
		"database": {ID: "mongodb", Label: "MongoDB"},
	}
	if expander.taskApplies(task, dims) {
		t.Error("expected task to NOT apply for mongodb")
	}
}

// findTemplateDir is defined in engine_test.go — reuse via package scope.
var _ = findTemplateDir // compile-time check that it exists

func testFindTemplateDir(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "templates", "jira-board")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find templates/jira-board directory")
		}
		dir = parent
	}
}
