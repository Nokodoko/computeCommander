package isolation

import (
	"context"
	"testing"
	"time"

	"github.com/noko/computecommander/internal/platform/db"
)

func setupIsoDB(t *testing.T) db.DB {
	t.Helper()
	database, err := db.NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	if err := db.Migrate(database, "sqlite"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestManifestStoreCRUD(t *testing.T) {
	database := setupIsoDB(t)
	ctx := context.Background()
	store := NewManifestStore(database)

	m := &IsolationManifest{
		AgentID:    "ses-001",
		AgentName:  "builder-1",
		Capability: "builder",
		Grants: ResourceGrants{
			Filesystem: FilesystemGrants{
				Read:  []string{"internal/*"},
				Write: []string{"internal/auth/jwt.go"},
			},
			EnvVars: []string{"GOPATH", "HOME"},
			Network: NetworkGrants{DenyAll: true},
			Resources: ResourceLimits{
				CPUShares:    512,
				MemoryMB:     2048,
				DiskMB:       1024,
				MaxProcesses: 50,
			},
		},
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
	}

	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.Get(ctx, "ses-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AgentName != "builder-1" {
		t.Fatalf("expected builder-1, got %q", got.AgentName)
	}
	if len(got.Grants.Filesystem.Read) != 1 {
		t.Fatalf("expected 1 read grant, got %d", len(got.Grants.Filesystem.Read))
	}
	if got.Grants.Resources.MemoryMB != 2048 {
		t.Fatalf("expected 2048 MB memory, got %d", got.Grants.Resources.MemoryMB)
	}
}

func TestManifestStoreList(t *testing.T) {
	database := setupIsoDB(t)
	ctx := context.Background()
	store := NewManifestStore(database)

	for _, cap := range []string{"builder", "builder", "scout"} {
		m := &IsolationManifest{
			AgentID:    GenerateManifestID(),
			AgentName:  "agent-" + cap,
			Capability: cap,
			Grants:     ResourceGrants{},
			CreatedAt:  time.Now().UTC(),
			ExpiresAt:  time.Now().UTC().Add(time.Hour),
		}
		_ = store.Create(ctx, m)
	}

	all, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	builders, err := store.List(ctx, "builder")
	if err != nil {
		t.Fatalf("list builders: %v", err)
	}
	if len(builders) != 2 {
		t.Fatalf("expected 2 builders, got %d", len(builders))
	}
}

func TestManifestStoreDelete(t *testing.T) {
	database := setupIsoDB(t)
	ctx := context.Background()
	store := NewManifestStore(database)

	m := &IsolationManifest{
		AgentID:    "ses-delete",
		AgentName:  "test",
		Capability: "builder",
		Grants:     ResourceGrants{},
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}
	_ = store.Create(ctx, m)
	_ = store.Delete(ctx, "ses-delete")

	_, err := store.Get(ctx, "ses-delete")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestManifestIsExpired(t *testing.T) {
	m := &IsolationManifest{
		ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}
	if !m.IsExpired() {
		t.Fatal("expected expired")
	}

	m.ExpiresAt = time.Now().UTC().Add(time.Hour)
	if m.IsExpired() {
		t.Fatal("expected not expired")
	}
}

func TestManifestHasAccess(t *testing.T) {
	m := &IsolationManifest{
		Grants: ResourceGrants{
			Filesystem: FilesystemGrants{
				Read:  []string{"internal/*"},
				Write: []string{"internal/auth/jwt.go"},
			},
		},
	}

	if !m.HasReadAccess("internal/foo.go") {
		t.Fatal("expected read access to internal/foo.go")
	}
	if m.HasReadAccess("external/bar.go") {
		t.Fatal("expected no read access to external/bar.go")
	}
	if !m.HasWriteAccess("internal/auth/jwt.go") {
		t.Fatal("expected write access to internal/auth/jwt.go")
	}
}

func TestDefaultResourceLimits(t *testing.T) {
	defaults := DefaultResourceLimits()
	if defaults.CPUShares != 512 {
		t.Fatalf("expected 512 cpu shares, got %d", defaults.CPUShares)
	}
	if defaults.MemoryMB != 2048 {
		t.Fatalf("expected 2048 MB, got %d", defaults.MemoryMB)
	}
}

func TestGetBlockOverrides(t *testing.T) {
	// Empty overrides should return nil.
	m := &IsolationManifest{
		Grants: ResourceGrants{},
	}
	if got := m.GetBlockOverrides(); got != nil {
		t.Fatalf("expected nil for empty overrides, got %v", got)
	}

	// Explicit block override IDs should be returned.
	m.Grants.BlockOverrides = []string{"rule-force-push", "rule-secret-exposure"}
	overrides := m.GetBlockOverrides()
	if len(overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(overrides))
	}
	if overrides[0] != "rule-force-push" {
		t.Errorf("expected rule-force-push, got %q", overrides[0])
	}
	if overrides[1] != "rule-secret-exposure" {
		t.Errorf("expected rule-secret-exposure, got %q", overrides[1])
	}

	// Verify returned slice is a copy (not a reference to the original).
	overrides[0] = "modified"
	if m.Grants.BlockOverrides[0] == "modified" {
		t.Error("GetBlockOverrides should return a copy, not a reference")
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern, path string
		expected      bool
	}{
		{"internal/*", "internal/foo.go", true},
		{"internal/*", "external/foo.go", false},
		{"*", "anything", true},
		{"exact/match.go", "exact/match.go", true},
		{"exact/match.go", "other.go", false},
	}
	for _, tt := range tests {
		matched, _ := matchPattern(tt.pattern, tt.path)
		if matched != tt.expected {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, matched, tt.expected)
		}
	}
}
