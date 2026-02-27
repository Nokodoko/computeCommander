package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresDB implements DB using pgx connection pool.
type postgresDB struct {
	pool *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed DB with connection pooling.
func NewPostgres(cfg PostgresConfig) (DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres parse config: %w", err)
	}

	if cfg.PoolSize > 0 {
		poolCfg.MaxConns = int32(cfg.PoolSize)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	return &postgresDB{pool: pool}, nil
}

func (p *postgresDB) Driver() string { return "postgres" }

func (p *postgresDB) Exec(ctx context.Context, query string, args ...any) error {
	_, err := p.pool.Exec(ctx, query, args...)
	return err
}

func (p *postgresDB) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{scanner: &pgxRows{rows: rows}}, nil
}

func (p *postgresDB) QueryRow(ctx context.Context, query string, args ...any) *Row {
	row := p.pool.QueryRow(ctx, query, args...)
	return &Row{scanner: &pgxRow{row: row}}
}

func (p *postgresDB) Close() error {
	p.pool.Close()
	return nil
}

func (p *postgresDB) Begin(ctx context.Context) (Tx, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pgxTx{tx: tx}, nil
}

// pgxRows adapts pgx.Rows to the scanner interface used by Rows.
type pgxRows struct {
	rows pgx.Rows
}

func (r *pgxRows) Next() bool           { return r.rows.Next() }
func (r *pgxRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgxRows) Close() error          { r.rows.Close(); return nil }
func (r *pgxRows) Err() error            { return r.rows.Err() }

// pgxRow adapts pgx.Row to the scanner interface used by Row.
type pgxRow struct {
	row pgx.Row
}

func (r *pgxRow) Scan(dest ...any) error { return r.row.Scan(dest...) }

// pgxTx implements Tx for Postgres.
type pgxTx struct {
	tx pgx.Tx
}

func (t *pgxTx) Exec(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.Exec(ctx, query, args...)
	return err
}

func (t *pgxTx) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{scanner: &pgxRows{rows: rows}}, nil
}

func (t *pgxTx) Commit() error   { return t.tx.Commit(context.Background()) }
func (t *pgxTx) Rollback() error { return t.tx.Rollback(context.Background()) }
