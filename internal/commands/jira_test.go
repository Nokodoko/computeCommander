package commands

import (
	"context"
	"testing"

	"github.com/noko/computecommander/internal/config"
)

func TestJiraLegacyQueryEmpty(t *testing.T) {
	app := testApp(t)

	tasks, err := queryJiraLegacyTasks(context.Background(), app)
	if err != nil {
		t.Fatalf("queryJiraLegacyTasks with empty DB: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestJiraLegacyQueryWithTasks(t *testing.T) {
	app := testApp(t)

	ctx := context.Background()

	// Insert a task group.
	err := app.DB.Exec(ctx,
		"INSERT INTO task_groups (id, name, status) VALUES (?, ?, ?)",
		"tg-test-1", "Test Task", "active",
	)
	if err != nil {
		t.Fatalf("insert task group: %v", err)
	}

	// Insert a member.
	err = app.DB.Exec(ctx,
		"INSERT INTO task_group_members (group_id, issue_id) VALUES (?, ?)",
		"tg-test-1", "agent-1",
	)
	if err != nil {
		t.Fatalf("insert task group member: %v", err)
	}

	tasks, err := queryJiraLegacyTasks(ctx, app)
	if err != nil {
		t.Fatalf("queryJiraLegacyTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "tg-test-1" {
		t.Errorf("expected id tg-test-1, got %s", tasks[0].ID)
	}
	if tasks[0].Name != "Test Task" {
		t.Errorf("expected name 'Test Task', got %s", tasks[0].Name)
	}
	if tasks[0].MemberCount != 1 {
		t.Errorf("expected 1 member, got %d", tasks[0].MemberCount)
	}
}

func TestJiraStatusIcon(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"active", "\033[32m●\033[0m"},
		{"In Progress", "\033[32m●\033[0m"},
		{"completed", "\033[36m✓\033[0m"},
		{"Done", "\033[36m✓\033[0m"},
		{"failed", "\033[31m✗\033[0m"},
		{"Blocked", "\033[31m✗\033[0m"},
		{"To Do", "\033[33m○\033[0m"},
		{"unknown", "\033[33m○\033[0m"},
	}
	for _, tc := range cases {
		got := jiraStatusIcon(tc.status)
		if got != tc.want {
			t.Errorf("jiraStatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestJiraHasConfig(t *testing.T) {
	app := testApp(t)

	// No Jira instances configured.
	if hasJiraConfig(app) {
		t.Error("expected hasJiraConfig to return false with no instances")
	}

	// Add an instance.
	app.Config.Jira.Instances = []config.JiraInstance{
		{Name: "test", BaseURL: "https://test.atlassian.net"},
	}
	if !hasJiraConfig(app) {
		t.Error("expected hasJiraConfig to return true with instances")
	}
}

func TestJiraResolveInstance(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Jira.Instances = []config.JiraInstance{
		{Name: "corp", BaseURL: "https://corp.atlassian.net"},
		{Name: "oss", BaseURL: "https://oss.atlassian.net"},
	}

	// Default (first).
	inst, err := resolveInstance(cfg, "")
	if err != nil {
		t.Fatalf("resolveInstance default: %v", err)
	}
	if inst.Name != "corp" {
		t.Errorf("expected 'corp', got %s", inst.Name)
	}

	// By name.
	inst, err = resolveInstance(cfg, "oss")
	if err != nil {
		t.Fatalf("resolveInstance by name: %v", err)
	}
	if inst.Name != "oss" {
		t.Errorf("expected 'oss', got %s", inst.Name)
	}

	// Not found.
	_, err = resolveInstance(cfg, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
}

func TestJiraListInstances(t *testing.T) {
	app := testApp(t)
	app.Config.Jira.Instances = []config.JiraInstance{
		{Name: "test", BaseURL: "https://test.atlassian.net", Auth: config.JiraAuth{Type: "pat"}},
	}

	// Should not error.
	err := listJiraInstances(app, false)
	if err != nil {
		t.Fatalf("listJiraInstances: %v", err)
	}
}
