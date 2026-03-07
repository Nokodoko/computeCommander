package holdout

import (
	"os"
	"testing"
)

func TestWriteAndReadKeyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "key-test-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	content := "AGE-SECRET-KEY-TEST-CONTENT"
	if err := WriteKeyFile(tmpFile.Name(), content); err != nil {
		t.Fatalf("write key: %v", err)
	}

	got, err := ReadKeyFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	if got != content {
		t.Fatalf("expected %q, got %q", content, got)
	}
}

func TestReadKeyFileNotFound(t *testing.T) {
	_, err := ReadKeyFile("/nonexistent/path/key.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteKeyFilePermissions(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "key-perm-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if err := WriteKeyFile(tmpFile.Name(), "secret"); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Fatalf("expected 0600 permissions, got %o", perm)
	}
}
