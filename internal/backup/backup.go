// Package backup provides SQLite database backup and restore operations.
package backup

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupResult contains metadata about a completed backup.
type BackupResult struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// RestoreResult contains metadata about a completed restore.
type RestoreResult struct {
	BackupPath string `json:"backupPath"`
	RestoredAt string `json:"restoredAt"`
}

// generateBackupID creates a backup ID in the format "bak-{8hex}".
func generateBackupID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID.
		return fmt.Sprintf("bak-%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return fmt.Sprintf("bak-%x", b)
}

// Backup creates a copy of the SQLite database file at the given source path.
// If outputDir is empty, it defaults to .computecommander/backups/.
func Backup(dbPath, outputDir string) (*BackupResult, error) {
	if outputDir == "" {
		outputDir = filepath.Join(".computecommander", "backups")
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	now := time.Now()
	filename := fmt.Sprintf("backup-%s.db", now.Format("20060102T150405"))
	destPath := filepath.Join(outputDir, filename)

	// Copy the database file atomically: write to temp, then rename.
	tmpPath := destPath + ".tmp"

	src, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database for backup: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create backup temp file: %w", err)
	}

	size, err := io.Copy(dst, src)
	if err != nil {
		dst.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("copy database: %w", err)
	}

	if err := dst.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("close backup temp file: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("rename backup file: %w", err)
	}

	// Set restrictive permissions on backup file.
	if err := os.Chmod(destPath, 0o600); err != nil {
		// Non-fatal, continue.
		_ = err
	}

	return &BackupResult{
		ID:        generateBackupID(),
		Path:      destPath,
		SizeBytes: size,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

// Restore copies a backup file over the current database.
// It validates the backup exists and is readable before overwriting.
func Restore(backupPath, dbPath string) (*RestoreResult, error) {
	// Validate backup file exists and is readable.
	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("backup file not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("backup path is a directory, not a file")
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("backup file is empty")
	}

	// Copy backup to temp path, then rename over the DB.
	tmpPath := dbPath + ".restore.tmp"

	src, err := os.Open(backupPath)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create restore temp file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("copy backup: %w", err)
	}

	if err := dst.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("close restore temp file: %w", err)
	}

	// Atomic rename over the database file.
	if err := os.Rename(tmpPath, dbPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("rename restored database: %w", err)
	}

	now := time.Now()
	return &RestoreResult{
		BackupPath: backupPath,
		RestoredAt: now.Format(time.RFC3339),
	}, nil
}

// ListBackups returns all backup files in the given directory.
func ListBackups(dir string) ([]BackupResult, error) {
	if dir == "" {
		dir = filepath.Join(".computecommander", "backups")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	var backups []BackupResult
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupResult{
			Path:      filepath.Join(dir, entry.Name()),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().Format(time.RFC3339),
		})
	}

	return backups, nil
}
