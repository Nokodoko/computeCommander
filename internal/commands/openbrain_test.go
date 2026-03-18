package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenBrainExtractSections(t *testing.T) {
	// Create a temp MEMORY.md.
	dir := t.TempDir()
	memFile := filepath.Join(dir, "MEMORY.md")
	content := `# Test Memory

Some content here.

## Architecture Decisions

Decision 1: Use zellij.
Decision 2: No PTY embedding.

## User Environment

The user runs Linux.
`
	if err := os.WriteFile(memFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sections := extractSections(memFile)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}

	if _, ok := sections["# Test Memory"]; !ok {
		t.Error("missing '# Test Memory' section")
	}
	if _, ok := sections["## Architecture Decisions"]; !ok {
		t.Error("missing '## Architecture Decisions' section")
	}
	if _, ok := sections["## User Environment"]; !ok {
		t.Error("missing '## User Environment' section")
	}
}

func TestOpenBrainDiffSections(t *testing.T) {
	oldSections := map[string]string{
		"## Existing":  "old content\n",
		"## ToDelete":  "will be removed\n",
	}
	newSections := map[string]string{
		"## Existing":  "new content\n",
		"## Added":     "brand new\n",
	}

	entries := diffSections("/test/MEMORY.md", oldSections, newSections)

	// Expect: Existing modified, ToDelete deleted, Added added.
	if len(entries) != 3 {
		t.Fatalf("expected 3 diff entries, got %d", len(entries))
	}

	ops := make(map[string]string)
	for _, e := range entries {
		ops[e.Section] = e.Operation
	}

	if ops["## Existing"] != "modified" {
		t.Errorf("expected '## Existing' modified, got %q", ops["## Existing"])
	}
	if ops["## ToDelete"] != "deleted" {
		t.Errorf("expected '## ToDelete' deleted, got %q", ops["## ToDelete"])
	}
	if ops["## Added"] != "added" {
		t.Errorf("expected '## Added' added, got %q", ops["## Added"])
	}
}

func TestOpenBrainHashFileContent(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.md")

	// Non-existent file returns empty.
	if h := hashFileContent(f); h != "" {
		t.Errorf("expected empty hash for non-existent file, got %q", h)
	}

	// Write content.
	os.WriteFile(f, []byte("hello"), 0o644)
	h1 := hashFileContent(f)
	if h1 == "" {
		t.Error("expected non-empty hash")
	}

	// Same content = same hash.
	h2 := hashFileContent(f)
	if h1 != h2 {
		t.Errorf("same content should produce same hash: %q vs %q", h1, h2)
	}

	// Different content = different hash.
	os.WriteFile(f, []byte("world"), 0o644)
	h3 := hashFileContent(f)
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}

func TestOpenBrainOpColor(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"added", "\033[32m"},
		{"modified", "\033[33m"},
		{"deleted", "\033[31m"},
		{"unknown", "\033[2m"},
	}
	for _, tc := range cases {
		got := openBrainOpColor(tc.op)
		if got != tc.want {
			t.Errorf("openBrainOpColor(%q) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestOpenBrainAgentActivityQuery(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Pre-seed agent lifecycle events in the events table.
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	events := []struct {
		agent     string
		eventType string
		data      string
	}{
		{"pi-coder", "agent.registered", "runtime=pi capability=builder"},
		{"unix-coder", "agent.working", "runtime=claude capability=builder"},
		{"code-review", "agent.completed", "runtime=claude capability=reviewer"},
	}

	for _, e := range events {
		err := app.DB.Exec(ctx,
			`INSERT INTO events (agent_name, event_type, level, data, created_at)
			VALUES ($1, $2, $3, $4, $5)`,
			e.agent, e.eventType, "info", e.data, now,
		)
		if err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}

	// Query agent activity.
	entries := queryAgentActivity(ctx, app, 5)
	if len(entries) != 3 {
		t.Fatalf("expected 3 agent activity entries, got %d", len(entries))
	}

	// Verify first entry (most recent by DESC order, but insertion order matters for same timestamp).
	found := false
	for _, e := range entries {
		if e.AgentName == "pi-coder" && e.EventType == "agent.registered" {
			found = true
			if e.Runtime != "pi" {
				t.Errorf("expected runtime=pi, got %q", e.Runtime)
			}
			if e.Capability != "builder" {
				t.Errorf("expected capability=builder, got %q", e.Capability)
			}
		}
	}
	if !found {
		t.Error("expected to find pi-coder agent.registered event")
	}
}

func TestOpenBrainAgentActivityEmpty(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	entries := queryAgentActivity(ctx, app, 5)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty DB, got %d", len(entries))
	}
}

func TestOpenBrainAgentActivityNilApp(t *testing.T) {
	ctx := context.Background()
	entries := queryAgentActivity(ctx, nil, 5)
	if entries != nil {
		t.Errorf("expected nil entries for nil app, got %v", entries)
	}
}

func TestOpenBrainAgentActivityLimit(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert 10 events.
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for i := 0; i < 10; i++ {
		_ = app.DB.Exec(ctx,
			`INSERT INTO events (agent_name, event_type, level, data, created_at)
			VALUES ($1, $2, $3, $4, $5)`,
			"test-agent", "agent.heartbeat", "info", "heartbeat", now,
		)
	}

	// Query with limit 5.
	entries := queryAgentActivity(ctx, app, 5)
	if len(entries) != 5 {
		t.Errorf("expected 5 entries with limit=5, got %d", len(entries))
	}
}
