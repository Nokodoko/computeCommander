package commands

import (
	"context"
	"testing"
)

func TestJiraQueryEmpty(t *testing.T) {
	app := testApp(t)


	tasks, err := queryJiraTasks(context.Background(), app)
	if err != nil {
		t.Fatalf("queryJiraTasks with empty DB: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestJiraQueryWithTasks(t *testing.T) {
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

	tasks, err := queryJiraTasks(ctx, app)
	if err != nil {
		t.Fatalf("queryJiraTasks: %v", err)
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
		{"completed", "\033[36m✓\033[0m"},
		{"failed", "\033[31m✗\033[0m"},
		{"pending", "\033[33m○\033[0m"},
		{"unknown", "\033[33m○\033[0m"},
	}
	for _, tc := range cases {
		got := jiraStatusIcon(tc.status)
		if got != tc.want {
			t.Errorf("jiraStatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
