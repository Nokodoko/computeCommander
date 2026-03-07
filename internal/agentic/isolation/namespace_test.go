package isolation

import (
	"os"
	"testing"
)

func TestEnvFilter(t *testing.T) {
	manifest := &IsolationManifest{
		Grants: ResourceGrants{
			EnvVars: []string{"HOME", "PATH"},
		},
	}

	filtered := EnvFilter(manifest)
	// Should only include HOME and PATH from the environment
	for _, env := range filtered {
		parts := splitEnvVar(env)
		if parts[0] != "HOME" && parts[0] != "PATH" {
			t.Fatalf("unexpected env var in filtered: %q", parts[0])
		}
	}
}

func TestSplitEnvVar(t *testing.T) {
	parts := splitEnvVar("HOME=/home/user")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "HOME" {
		t.Fatalf("expected HOME, got %q", parts[0])
	}
	if parts[1] != "/home/user" {
		t.Fatalf("expected /home/user, got %q", parts[1])
	}
}

func TestNamespaceManagerNew(t *testing.T) {
	nm := NewNamespaceManager("/tmp/test-ns")
	if nm.baseDir != "/tmp/test-ns" {
		t.Fatalf("expected /tmp/test-ns, got %q", nm.baseDir)
	}
}

func TestNamespaceManagerCreateFiltered(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ns-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	nm := NewNamespaceManager(tmpDir)
	manifest := &IsolationManifest{
		Grants: ResourceGrants{
			Filesystem: FilesystemGrants{
				Read:  []string{"/tmp"},
				Write: []string{"/tmp"},
			},
		},
	}

	agentDir, err := nm.CreateFiltered("test-agent", manifest)
	if err != nil {
		t.Fatalf("create filtered: %v", err)
	}
	if agentDir == "" {
		t.Fatal("expected non-empty agent dir")
	}
}
