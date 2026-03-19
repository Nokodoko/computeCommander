package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		"## Existing": "old content\n",
		"## ToDelete": "will be removed\n",
	}
	newSections := map[string]string{
		"## Existing": "new content\n",
		"## Added":    "brand new\n",
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

// ─── New OpenBrain knowledge entry tests ─────────────────────────────────────

func TestOpenBrainRuntimeColor(t *testing.T) {
	cases := []struct {
		runtime string
		want    string
	}{
		{"claude", "\033[34m"},
		{"pi", "\033[35m"},
		{"gemini", "\033[36m"},
		{"codex", "\033[32m"},
		{"goose", "\033[33m"},
		{"unknown", "\033[2m"},
		{"Claude", "\033[34m"}, // case-insensitive
	}
	for _, tc := range cases {
		got := runtimeColor(tc.runtime)
		if got != tc.want {
			t.Errorf("runtimeColor(%q) = %q, want %q", tc.runtime, got, tc.want)
		}
	}
}

func TestOpenBrainEntryTypeGlyph(t *testing.T) {
	cases := []struct {
		entryType string
		wantGlyph string
		wantColor string
	}{
		{"decision", "D", "\033[1;37m"},
		{"discovery", "?", "\033[36m"},
		{"warning", "!", "\033[1;33m"},
		{"solution", "S", "\033[32m"},
		{"context", "~", "\033[2m"},
		{"unknown", "\302\267", "\033[2m"}, // middle dot
	}
	for _, tc := range cases {
		glyph, color := entryTypeGlyph(tc.entryType)
		if glyph != tc.wantGlyph {
			t.Errorf("entryTypeGlyph(%q) glyph = %q, want %q", tc.entryType, glyph, tc.wantGlyph)
		}
		if color != tc.wantColor {
			t.Errorf("entryTypeGlyph(%q) color = %q, want %q", tc.entryType, color, tc.wantColor)
		}
	}
}

func TestOpenBrainWriteInsert(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	err := app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, detail, runtime, agent_name, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test-project", "decision", "Use zellij for layouts", "PTY too complex", "claude", "unix-coder", "tui,arch",
	)
	if err != nil {
		t.Fatalf("insert knowledge entry: %v", err)
	}

	// Verify it was inserted.
	var summary, detail, runtime string
	row := app.DB.QueryRow(ctx,
		"SELECT summary, COALESCE(detail, ''), runtime FROM openbrain_entries WHERE project_name = ? AND entry_type = ?",
		"test-project", "decision")
	if err := row.Scan(&summary, &detail, &runtime); err != nil {
		t.Fatalf("query inserted entry: %v", err)
	}
	if summary != "Use zellij for layouts" {
		t.Errorf("expected summary 'Use zellij for layouts', got %q", summary)
	}
	if detail != "PTY too complex" {
		t.Errorf("expected detail 'PTY too complex', got %q", detail)
	}
	if runtime != "claude" {
		t.Errorf("expected runtime 'claude', got %q", runtime)
	}
}

func TestOpenBrainWriteDedup(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert first entry.
	err := app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime)
		VALUES (?, ?, ?, ?)`,
		"test-project", "warning", "Do not nest zellij", "claude",
	)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Insert duplicate — should be ignored (no error, no duplicate row).
	err = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime)
		VALUES (?, ?, ?, ?)`,
		"test-project", "warning", "Do not nest zellij", "pi",
	)
	if err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}

	// Count rows — should be exactly 1.
	var count int
	row := app.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM openbrain_entries WHERE project_name = ? AND entry_type = ? AND summary = ?",
		"test-project", "warning", "Do not nest zellij")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after dedup, got %d", count)
	}
}

func TestOpenBrainWriteInvalidType(t *testing.T) {
	if !validEntryTypes["decision"] {
		t.Error("decision should be valid")
	}
	if validEntryTypes["telemetry"] {
		t.Error("telemetry should not be valid")
	}
	if validEntryTypes[""] {
		t.Error("empty string should not be valid")
	}
}

func TestOpenBrainWriteTTL(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert with explicit expires_at.
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	err := app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		"test-project", "context", "temp context", "claude", expiresAt,
	)
	if err != nil {
		t.Fatalf("insert with TTL: %v", err)
	}

	// Verify expires_at was set.
	var gotExpires string
	row := app.DB.QueryRow(ctx,
		"SELECT COALESCE(expires_at, '') FROM openbrain_entries WHERE summary = ?", "temp context")
	if err := row.Scan(&gotExpires); err != nil {
		t.Fatalf("query expires_at: %v", err)
	}
	if gotExpires == "" {
		t.Error("expected expires_at to be set")
	}
}

func TestOpenBrainReadEntries(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert test entries.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	entries := []struct {
		project   string
		entryType string
		summary   string
		runtime   string
	}{
		{"myproject", "decision", "Use zellij layouts", "claude"},
		{"myproject", "warning", "Do not nest zellij", "pi"},
		{"myproject", "solution", "Fixed NULL scan with COALESCE", "claude"},
		{"other", "decision", "Different project", "claude"},
	}
	for _, e := range entries {
		err := app.DB.Exec(ctx,
			`INSERT OR IGNORE INTO openbrain_entries
			(project_name, entry_type, summary, runtime, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			e.project, e.entryType, e.summary, e.runtime, now,
		)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Query for myproject.
	results, err := queryKnowledgeEntries(ctx, app, "myproject", 20, "72h", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 entries for myproject, got %d", len(results))
	}

	// Verify filtering by type.
	results, err = queryKnowledgeEntries(ctx, app, "myproject", 20, "72h", "decision,warning")
	if err != nil {
		t.Fatalf("query with type filter: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 entries for decision+warning, got %d", len(results))
	}
}

func TestOpenBrainReadEmpty(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Query for non-existent project — should return empty, not error.
	results, err := queryKnowledgeEntries(ctx, app, "nonexistent", 20, "72h", "")
	if err != nil {
		t.Fatalf("query should not error on empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 entries, got %d", len(results))
	}
}

func TestOpenBrainReadSinceBoundary(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert an entry exactly at the boundary (1 hour ago).
	oneHourAgo := time.Now().Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	err := app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		"boundary-test", "discovery", "Found at boundary", "claude", oneHourAgo,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query with since=2h — should include the entry.
	results, err := queryKnowledgeEntries(ctx, app, "boundary-test", 20, "2h", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 entry within 2h, got %d", len(results))
	}

	// Query with since=30m — should exclude the entry.
	results, err = queryKnowledgeEntries(ctx, app, "boundary-test", 20, "30m", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 entries within 30m, got %d", len(results))
	}
}

func TestOpenBrainPruneExpired(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert an expired entry.
	pastTime := time.Now().Add(-1 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	err := app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		"prune-test", "context", "Expired entry", "claude", pastTime,
	)
	if err != nil {
		t.Fatalf("insert expired: %v", err)
	}

	// Insert a non-expired entry.
	futureTime := time.Now().Add(24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	err = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		"prune-test", "context", "Active entry", "claude", futureTime,
	)
	if err != nil {
		t.Fatalf("insert active: %v", err)
	}

	// Prune expired.
	count, err := pruneExpiredEntriesCount(ctx, app)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 pruned, got %d", count)
	}

	// Verify only the active entry remains.
	var remaining int
	row := app.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM openbrain_entries WHERE project_name = ?", "prune-test")
	if err := row.Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("expected 1 remaining entry, got %d", remaining)
	}
}

func TestOpenBrainParseTTL(t *testing.T) {
	cases := []struct {
		input    string
		wantDur  time.Duration
		wantErr  bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"", 0, true},
		{"invalid", 0, true},
	}
	for _, tc := range cases {
		dur, err := parseTTL(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTTL(%q) expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTTL(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if dur != tc.wantDur {
			t.Errorf("parseTTL(%q) = %v, want %v", tc.input, dur, tc.wantDur)
		}
	}
}

func TestOpenBrainFormatAge(t *testing.T) {
	cases := []struct {
		createdAt string
		wantPfx   string // prefix the age string should start with
	}{
		{time.Now().Add(-30 * time.Minute).UTC().Format("2006-01-02 15:04:05"), "30m"},
		{time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02 15:04:05"), "2h"},
		{time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02 15:04:05"), "2d"},
		{"invalid", ""},
	}
	for _, tc := range cases {
		got := formatAge(tc.createdAt)
		if tc.wantPfx == "" {
			if got != "" {
				t.Errorf("formatAge(%q) = %q, want empty", tc.createdAt, got)
			}
			continue
		}
		if !strings.HasPrefix(got, tc.wantPfx) {
			t.Errorf("formatAge(%q) = %q, want prefix %q", tc.createdAt, got, tc.wantPfx)
		}
	}
}

func TestOpenBrainDeriveProjectName(t *testing.T) {
	// This test just verifies the function doesn't panic and returns something.
	name := deriveProjectName()
	if name == "" {
		t.Error("deriveProjectName returned empty string")
	}
}

func TestOpenBrainReadPerformance(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert 100 entries.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for i := 0; i < 100; i++ {
		// Use unique summaries to avoid dedup.
		summary := strings.Repeat("x", 40) + strings.Repeat("0", 3-len(itoa(i))) + itoa(i)
		_ = app.DB.Exec(ctx,
			`INSERT OR IGNORE INTO openbrain_entries
			(project_name, entry_type, summary, runtime, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			"perf-test", "context", summary, "claude", now,
		)
	}

	// Measure query time.
	start := time.Now()
	results, err := queryKnowledgeEntries(ctx, app, "perf-test", 20, "72h", "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 20 {
		t.Errorf("expected 20 results (limited), got %d", len(results))
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("query took %v, want <100ms", elapsed)
	}
}

// itoa is a simple int-to-string helper for tests.
func itoa(i int) string {
	return strings.TrimSpace(strings.Replace(time.Duration(i).String(), "ns", "", 1))
}

func TestOpenBrainWriteCmd(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	cmd := OpenBrainCmd(app)
	cmd.SetArgs([]string{"write",
		"--type", "decision",
		"--summary", "Use zellij for TUI layout",
		"--detail", "PTY embedding was too complex",
		"--project", "cmd-test",
		"--runtime", "claude",
		"--agent", "unix-coder",
		"--tags", "tui,architecture",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("write cmd: %v", err)
	}

	// Verify it was inserted.
	var summary string
	row := app.DB.QueryRow(ctx,
		"SELECT summary FROM openbrain_entries WHERE project_name = ? AND entry_type = ?",
		"cmd-test", "decision")
	if err := row.Scan(&summary); err != nil {
		t.Fatalf("query: %v", err)
	}
	if summary != "Use zellij for TUI layout" {
		t.Errorf("expected summary 'Use zellij for TUI layout', got %q", summary)
	}
}

func TestOpenBrainWriteCmdInvalidType(t *testing.T) {
	app := testApp(t)

	cmd := OpenBrainCmd(app)
	cmd.SetArgs([]string{"write",
		"--type", "telemetry",
		"--summary", "Should fail",
	})

	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid entry type")
	}
}

func TestOpenBrainReadCmdJSON(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Insert a test entry.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_ = app.DB.Exec(ctx,
		`INSERT OR IGNORE INTO openbrain_entries
		(project_name, entry_type, summary, runtime, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		"json-test", "solution", "Fixed the bug", "claude", now,
	)

	cmd := OpenBrainCmd(app)
	cmd.SetArgs([]string{"read", "--project", "json-test", "--json"})

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := cmd.Execute(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("read cmd: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	var output strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	outStr := output.String()
	if !strings.Contains(outStr, `"success":true`) && !strings.Contains(outStr, `"success": true`) {
		t.Errorf("expected JSON with success:true, got %s", outStr)
	}
	if !strings.Contains(outStr, "Fixed the bug") {
		t.Errorf("expected JSON to contain entry summary, got %s", outStr)
	}
}

func TestOpenBrainEntryTypeConstraint(t *testing.T) {
	app := testApp(t)
	ctx := context.Background()

	// Attempt to insert with invalid entry_type — should fail due to CHECK constraint.
	err := app.DB.Exec(ctx,
		`INSERT INTO openbrain_entries
		(project_name, entry_type, summary, runtime)
		VALUES (?, ?, ?, ?)`,
		"test-project", "telemetry", "Should fail", "claude",
	)
	if err == nil {
		t.Error("expected CHECK constraint violation for invalid entry_type")
	}
}
