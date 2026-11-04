package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Tx wraps a transaction and implements Querier.
type Tx struct{ tx *sql.Tx }

func (t *Tx) ExecContext(c context.Context, q string, a ...any) (sql.Result, error) {
	return t.tx.ExecContext(c, q, a...)
}
func (t *Tx) QueryContext(c context.Context, q string, a ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(c, q, a...)
}
func (t *Tx) QueryRowContext(c context.Context, q string, a ...any) *sql.Row {
	return t.tx.QueryRowContext(c, q, a...)
}

// WithTx commits when fn succeeds and rolls back when it fails. Use the supplied
// Querier throughout fn; DB.Q cannot be used within the callback with one connection.
func (d *DB) WithTx(ctx context.Context, fn func(tx *Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	if err = fn(&Tx{tx: tx}); err != nil {
		if rb := tx.Rollback(); rb != nil && !errors.Is(rb, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("store: rollback transaction: %w", rb))
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}
