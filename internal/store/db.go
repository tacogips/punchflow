package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// Querier is the subset of database operations used by store clients. Pass a Tx
// inside WithTx; using DB.Q from a transaction callback deadlocks because DB
// deliberately allows only one SQLite connection.
type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// DB owns the single serialized SQLite connection.
type DB struct{ db *sql.DB }

// Open creates the database directory, configures SQLite and applies migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("store: database path is empty")
	}
	pragmas := "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	var dsn string
	if path == ":memory:" {
		dsn = "file::memory:?" + pragmas
	} else {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("store: resolve database path: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
			return nil, fmt.Errorf("store: create database directory: %w", err)
		}
		u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
		dsn = u.String() + "?" + pragmas
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping sqlite: %w", err)
	}
	out := &DB{db: db}
	if err = out.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return out, nil
}

// Close releases the SQLite connection.
func (d *DB) Close() error { return d.db.Close() }

// Q returns the DB querier for use outside transactions. Inside WithTx use its
// callback argument: calling Q there blocks forever with the single connection.
func (d *DB) Q() Querier { return d.db }

// IsEmpty reports whether the tenant table has no row.
func (d *DB) IsEmpty(ctx context.Context) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM tenant").Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: count tenant: %w", err)
	}
	return n == 0, nil
}

// migrationFS is kept local to this package so migrations ship with the binary.
var _ embed.FS
