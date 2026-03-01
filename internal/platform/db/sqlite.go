package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Time format strings that modernc.org/sqlite may produce when storing time.Time
// as TEXT. The driver uses Go's default time.String() format which includes
// monotonic clock readings. We also accept RFC3339 and SQLite datetime() format
// for records inserted via the sqlite3 CLI or other tools.
var sqliteTimeFormats = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",       // Go time.String() without monotonic
	"2006-01-02T15:04:05Z07:00",                       // RFC3339
	"2006-01-02T15:04:05.999999999Z07:00",              // RFC3339Nano
	"2006-01-02 15:04:05",                              // SQLite datetime('now')
	"2006-01-02T15:04:05Z",                             // RFC3339 UTC shorthand
}

// parseSQLiteTime attempts to parse a time string using known formats.
func parseSQLiteTime(s string) (time.Time, error) {
	// Strip monotonic clock suffix (e.g., " m=+0.000229229")
	if idx := len(s) - 1; idx > 0 {
		for i := len(s) - 1; i >= 0; i-- {
			if s[i] == 'm' && i+1 < len(s) && s[i+1] == '=' {
				s = s[:i-1] // trim trailing " m=..."
				break
			}
		}
	}

	for _, layout := range sqliteTimeFormats {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse SQLite time %q", s)
}

// sqliteTimeScan intercepts Scan calls to convert TEXT columns to time.Time.
// modernc.org/sqlite returns time values as strings; database/sql cannot
// automatically scan strings into time.Time targets.
func sqliteTimeScan(scanner interface{ Scan(dest ...any) error }, dest ...any) error {
	// Build temporary scan targets: replace *time.Time with *sql.NullString
	temps := make([]any, len(dest))
	type timeTarget struct {
		idx    int
		isPtr  bool // **time.Time (nullable) vs *time.Time
	}
	var targets []timeTarget

	for i, d := range dest {
		switch d.(type) {
		case *time.Time:
			ns := &sql.NullString{}
			temps[i] = ns
			targets = append(targets, timeTarget{idx: i})
		case **time.Time:
			ns := &sql.NullString{}
			temps[i] = ns
			targets = append(targets, timeTarget{idx: i, isPtr: true})
		default:
			temps[i] = d
		}
	}

	// No time fields — pass through directly
	if len(targets) == 0 {
		return scanner.Scan(dest...)
	}

	if err := scanner.Scan(temps...); err != nil {
		return err
	}

	// Convert scanned strings back to time.Time
	for _, tt := range targets {
		ns := temps[tt.idx].(*sql.NullString)
		if !ns.Valid || ns.String == "" {
			if tt.isPtr {
				*(dest[tt.idx].(**time.Time)) = nil
			}
			continue
		}
		t, err := parseSQLiteTime(ns.String)
		if err != nil {
			return fmt.Errorf("column %d: %w", tt.idx, err)
		}
		if tt.isPtr {
			*(dest[tt.idx].(**time.Time)) = &t
		} else {
			*(dest[tt.idx].(*time.Time)) = t
		}
	}
	return nil
}

// sqliteRowsScanner wraps sql.Rows with time-aware scanning.
type sqliteRowsScanner struct {
	rows *sql.Rows
}

func (s *sqliteRowsScanner) Next() bool              { return s.rows.Next() }
func (s *sqliteRowsScanner) Close() error             { return s.rows.Close() }
func (s *sqliteRowsScanner) Err() error               { return s.rows.Err() }
func (s *sqliteRowsScanner) Scan(dest ...any) error   { return sqliteTimeScan(s.rows, dest...) }

// sqliteRowScanner wraps sql.Row with time-aware scanning.
type sqliteRowScanner struct {
	row *sql.Row
}

func (s *sqliteRowScanner) Scan(dest ...any) error { return sqliteTimeScan(s.row, dest...) }

// sqliteDB implements DB using modernc.org/sqlite (pure Go, no CGO).
type sqliteDB struct {
	conn *sql.DB
}

// NewSQLite opens a SQLite database at the given path.
// Use ":memory:" for an in-memory database.
func NewSQLite(path string) (DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	// Set busy_timeout FIRST so subsequent PRAGMAs wait instead of failing
	// when multiple processes (dashboard panes) open the DB concurrently.
	if _, err := conn.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sqlite pragma busy_timeout: %w", err)
	}
	// WAL mode for concurrent reads.
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}

	// Single writer connection for SQLite; WAL mode handles concurrent readers
	// across separate processes (each dashboard pane is its own process).
	conn.SetMaxOpenConns(1)

	return &sqliteDB{conn: conn}, nil
}

func (s *sqliteDB) Driver() string { return "sqlite" }

func (s *sqliteDB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := s.conn.ExecContext(ctx, query, args...)
	return err
}

func (s *sqliteDB) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := s.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{scanner: &sqliteRowsScanner{rows: rows}}, nil
}

func (s *sqliteDB) QueryRow(ctx context.Context, query string, args ...any) *Row {
	row := s.conn.QueryRowContext(ctx, query, args...)
	return &Row{scanner: &sqliteRowScanner{row: row}}
}

func (s *sqliteDB) Close() error {
	return s.conn.Close()
}

func (s *sqliteDB) Begin(ctx context.Context) (Tx, error) {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

// sqliteTx implements Tx for SQLite.
type sqliteTx struct {
	tx *sql.Tx
}

func (t *sqliteTx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.ExecContext(ctx, query, args...)
	return err
}

func (t *sqliteTx) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{scanner: &sqliteRowsScanner{rows: rows}}, nil
}

func (t *sqliteTx) Commit() error   { return t.tx.Commit() }
func (t *sqliteTx) Rollback() error { return t.tx.Rollback() }
