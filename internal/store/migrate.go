package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)"); err != nil {
		return fmt.Errorf("store: create migration table: %w", err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var current int
	if err = d.db.QueryRowContext(ctx, "SELECT coalesce(max(version),0) FROM schema_migrations").Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	for _, name := range names {
		prefix := strings.SplitN(name, "_", 2)[0]
		version, e := strconv.Atoi(prefix)
		if e != nil {
			return fmt.Errorf("store: invalid migration name %q: %w", name, e)
		}
		if version <= current {
			continue
		}
		body, e := migrationFiles.ReadFile("migrations/" + name)
		if e != nil {
			return fmt.Errorf("store: read migration %s: %w", name, e)
		}
		tx, e := d.db.BeginTx(ctx, nil)
		if e != nil {
			return fmt.Errorf("store: begin migration %d: %w", version, e)
		}
		if _, e = tx.ExecContext(ctx, string(body)); e != nil {
			tx.Rollback()
			return fmt.Errorf("store: apply migration %d: %w", version, e)
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,applied_at) VALUES(?,strftime('%s','now')*1000)", version); e != nil {
			tx.Rollback()
			return fmt.Errorf("store: record migration %d: %w", version, e)
		}
		if e = tx.Commit(); e != nil {
			return fmt.Errorf("store: commit migration %d: %w", version, e)
		}
		current = version
	}
	return nil
}
