package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// NewID returns a lowercase RFC 4122 UUID v4 string.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("store: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// IsUUID reports whether s has the canonical UUID textual shape.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// Millis converts an instant to Unix milliseconds.
func Millis(t time.Time) int64 { return t.UnixNano() / int64(time.Millisecond) }

// Time converts Unix milliseconds to an instant.
func Time(ms int64) time.Time { return time.UnixMilli(ms) }

// NullMillis converts an optional instant to a SQL-compatible value.
func NullMillis(t *time.Time) any {
	if t == nil {
		return nil
	}
	return Millis(*t)
}

// TimePtr converts a nullable SQL millisecond value to an optional instant.
func TimePtr(ms sql.NullInt64) *time.Time {
	if !ms.Valid {
		return nil
	}
	t := Time(ms.Int64)
	return &t
}

var codeTables = map[string]bool{"users": true, "grades": true, "teams": true, "categories": true, "folders": true, "general_masters": true, "general_master_items": true, "routes": true, "workflows": true}

// ResolveID resolves an ID or code in a known code-bearing table.
func ResolveID(ctx context.Context, q Querier, table, idOrCode string) (string, error) {
	if !codeTables[table] {
		return "", fmt.Errorf("store: table %q is not allowed for ID resolution", table)
	}
	var id string
	err := q.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE id=? OR code=? LIMIT 1", idOrCode, idOrCode).Scan(&id)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: resolve %s: %w", table, err)
	}
	return id, nil
}
