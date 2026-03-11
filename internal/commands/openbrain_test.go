package commands

import (
	"os"
	"path/filepath"
	"testing"
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
