package jiraboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTemplate(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Verify meta.
	if tmpl.Meta.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", tmpl.Meta.Version)
	}
	if tmpl.Meta.ProjectType != "org-generator" {
		t.Errorf("expected project_type org-generator, got %s", tmpl.Meta.ProjectType)
	}
	if tmpl.Meta.DefaultProjectKey != "DD" {
		t.Errorf("expected default_project_key DD, got %s", tmpl.Meta.DefaultProjectKey)
	}
}

func TestPartialMerge(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	// Verify partials were merged into dimensions.
	expectedDims := []string{"cloud_provider", "os", "database", "app_architecture", "storage"}
	for _, dim := range expectedDims {
		values, ok := tmpl.Dimensions[dim]
		if !ok {
			t.Errorf("dimension %s not found in merged template", dim)
			continue
		}
		if len(values) == 0 {
			t.Errorf("dimension %s has no values after merge", dim)
		}
	}

	// Verify specific dimension values.
	dbValues := tmpl.Dimensions["database"]
	foundPostgres := false
	for _, dv := range dbValues {
		if dv.ID == "postgres" {
			foundPostgres = true
			if dv.Label != "PostgreSQL" {
				t.Errorf("postgres label: expected PostgreSQL, got %s", dv.Label)
			}
			if dv.IntegrationPath() != "integrations-core/postgres" {
				t.Errorf("postgres integration_path: expected integrations-core/postgres, got %s", dv.IntegrationPath())
			}
			if len(dv.RequiredParams()) == 0 {
				t.Error("postgres should have required_params")
			}
		}
	}
	if !foundPostgres {
		t.Error("postgres not found in database dimension values")
	}
}

func TestValidate(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	tmpl, err := engine.LoadTemplate("org-generator")
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	errs := engine.Validate(tmpl)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("validation error: %s: %s", e.Field, e.Message)
		}
	}
}

func TestValidateRejectsEmptyTemplate(t *testing.T) {
	engine := NewEngine("")
	tmpl := &BoardTemplate{}

	errs := engine.Validate(tmpl)
	if len(errs) == 0 {
		t.Error("expected validation errors for empty template")
	}
}

func TestHasCycle(t *testing.T) {
	// No cycle.
	tracks := []Track{
		{ID: "a", DependsOn: []string{}},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"b"}},
	}
	if hasCycle(tracks) {
		t.Error("expected no cycle")
	}

	// Cycle: a -> b -> c -> a.
	tracks = []Track{
		{ID: "a", DependsOn: []string{"c"}},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"b"}},
	}
	if !hasCycle(tracks) {
		t.Error("expected cycle detection")
	}
}

func TestLoadIntake(t *testing.T) {
	// Create a temporary intake file.
	dir := t.TempDir()
	intakePath := filepath.Join(dir, "intake.yaml")

	content := `
intake:
  project_name: "Test Corp Onboarding"
  project_key: "TEST"
  environments:
    - "prod"
    - "staging"
  cloud_providers:
    - "aws"
  operating_systems:
    - "linux"
  databases:
    - "postgres"
    - "redis"
  app_architectures:
    - "containerized"
  storage_systems:
    - "s3"
`
	if err := os.WriteFile(intakePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write intake: %v", err)
	}

	intake, err := LoadIntake(intakePath)
	if err != nil {
		t.Fatalf("LoadIntake failed: %v", err)
	}

	if intake.Intake.ProjectName != "Test Corp Onboarding" {
		t.Errorf("expected project_name 'Test Corp Onboarding', got '%s'", intake.Intake.ProjectName)
	}
	if intake.Intake.ProjectKey != "TEST" {
		t.Errorf("expected project_key 'TEST', got '%s'", intake.Intake.ProjectKey)
	}
	if len(intake.Intake.Environments) != 2 {
		t.Errorf("expected 2 environments, got %d", len(intake.Intake.Environments))
	}
}

func TestLoadIntakeRequiresProjectName(t *testing.T) {
	dir := t.TempDir()
	intakePath := filepath.Join(dir, "intake.yaml")

	content := `
intake:
  project_key: "TEST"
  environments:
    - "prod"
`
	if err := os.WriteFile(intakePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write intake: %v", err)
	}

	_, err := LoadIntake(intakePath)
	if err == nil {
		t.Error("expected error for missing project_name")
	}
}

func TestListTemplates(t *testing.T) {
	templateDir := findTemplateDir(t)
	engine := NewEngine(templateDir)

	templates, err := engine.ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}

	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	found := false
	for _, tmpl := range templates {
		if tmpl.ProjectType == "org-generator" {
			found = true
			break
		}
	}
	if !found {
		t.Error("org-generator template not found in list")
	}
}

// findTemplateDir locates the templates/jira-board directory relative to the test file.
func findTemplateDir(t *testing.T) string {
	t.Helper()

	// Walk up from the test file to find the project root.
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
