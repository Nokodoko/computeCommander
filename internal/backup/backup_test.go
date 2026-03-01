package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()

	// Create a fake database file.
	dbPath := filepath.Join(dir, "local.db")
	if err := os.WriteFile(dbPath, []byte("fake-sqlite-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(dir, "backups")

	// Test backup.
	result, err := Backup(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}
	if result.SizeBytes == 0 {
		t.Error("expected non-zero backup size")
	}
	if result.Path == "" {
		t.Error("expected non-empty backup path")
	}
	if result.ID == "" {
		t.Error("expected non-empty backup ID")
	}

	// Verify backup file exists.
	if _, err := os.Stat(result.Path); err != nil {
		t.Errorf("backup file does not exist: %v", err)
	}

	// Modify the original DB to verify restore changes it back.
	if err := os.WriteFile(dbPath, []byte("modified-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test restore.
	restoreResult, err := Restore(result.Path, dbPath)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restoreResult.BackupPath != result.Path {
		t.Errorf("expected backup path %q, got %q", result.Path, restoreResult.BackupPath)
	}

	// Verify restored content.
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fake-sqlite-data" {
		t.Errorf("expected original data after restore, got %q", string(data))
	}
}

func TestBackupPermissions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "local.db")
	if err := os.WriteFile(dbPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(dir, "backups")
	result, err := Backup(dbPath, backupDir)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}

	// Verify restrictive permissions (0600).
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}
}

func TestRestoreNonexistent(t *testing.T) {
	_, err := Restore("/nonexistent/backup.db", "/tmp/db")
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
}

func TestListBackups(t *testing.T) {
	dir := t.TempDir()

	// Create some fake backup files.
	for _, name := range []string{"backup-20260228T140000.db", "backup-20260301T093000.db", "not-a-backup.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(backups) != 2 {
		t.Errorf("expected 2 backups, got %d", len(backups))
	}
}

func TestListBackupsEmpty(t *testing.T) {
	backups, err := ListBackups("/nonexistent/dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(backups))
	}
}
