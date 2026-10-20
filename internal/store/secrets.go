package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// ServerSecret returns a stable secret, creating 32 random bytes on first use.
func ServerSecret(ctx context.Context, q Querier, name string) ([]byte, error) {
	var value []byte
	err := q.QueryRowContext(ctx, "SELECT value FROM server_secrets WHERE name=?", name).Scan(&value)
	if err == nil {
		return value, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("store: read server secret: %w", err)
	}
	value = make([]byte, 32)
	if _, err = rand.Read(value); err != nil {
		return nil, fmt.Errorf("store: generate server secret: %w", err)
	}
	_, err = q.ExecContext(ctx, "INSERT OR IGNORE INTO server_secrets(name,value,created_at) VALUES(?,?,?)", name, value, Millis(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("store: save server secret: %w", err)
	}
	if err = q.QueryRowContext(ctx, "SELECT value FROM server_secrets WHERE name=?", name).Scan(&value); err != nil {
		return nil, fmt.Errorf("store: reread server secret: %w", err)
	}
	return value, nil
}
