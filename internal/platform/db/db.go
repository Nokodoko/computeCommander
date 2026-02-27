// Package db provides a database abstraction layer supporting both Postgres and SQLite backends.
package db

import (
	"context"
	"fmt"
)

// DB is the primary database interface for ComputeCommander.
type DB interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (*Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) *Row
	Close() error
	Begin(ctx context.Context) (Tx, error)
	Driver() string
}

// Tx represents a database transaction.
type Tx interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (*Rows, error)
	Commit() error
	Rollback() error
}

// Rows wraps database result rows.
type Rows struct {
	scanner interface {
		Next() bool
		Scan(dest ...any) error
		Close() error
		Err() error
	}
}

func (r *Rows) Next() bool        { return r.scanner.Next() }
func (r *Rows) Scan(dest ...any) error { return r.scanner.Scan(dest...) }
func (r *Rows) Close() error      { return r.scanner.Close() }
func (r *Rows) Err() error        { return r.scanner.Err() }

// Row wraps a single database result row.
type Row struct {
	scanner interface {
		Scan(dest ...any) error
	}
}

func (r *Row) Scan(dest ...any) error { return r.scanner.Scan(dest...) }

// PostgresConfig holds Postgres connection settings.
type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
	PoolSize int    `yaml:"pool_size"`
}

// DSN builds a Postgres connection string.
func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

// DatabaseConfig holds the full database configuration.
type DatabaseConfig struct {
	Driver   string         `yaml:"driver"`
	Postgres PostgresConfig `yaml:"postgres"`
	SQLite   struct {
		Path string `yaml:"path"`
	} `yaml:"sqlite"`
}

// NewDB creates a DB connection based on the provided config.
// If driver is "postgres", it connects via pgx. If "sqlite", it opens a SQLite file.
// If driver is empty or "auto", it tries Postgres first, then falls back to SQLite.
func NewDB(cfg DatabaseConfig) (DB, error) {
	switch cfg.Driver {
	case "postgres":
		return NewPostgres(cfg.Postgres)
	case "sqlite":
		path := cfg.SQLite.Path
		if path == "" {
			path = ":memory:"
		}
		return NewSQLite(path)
	case "", "auto":
		db, err := NewPostgres(cfg.Postgres)
		if err == nil {
			return db, nil
		}
		path := cfg.SQLite.Path
		if path == "" {
			path = ":memory:"
		}
		return NewSQLite(path)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
}
