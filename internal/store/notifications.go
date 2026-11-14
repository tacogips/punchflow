package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Notification is one local inbox entry.
type Notification struct {
	ID          string
	UserID      string
	Kind        string
	TicketID    string
	CommentID   *string
	ActorUserID *string
	Excerpt     string
	DedupeKey   *string
	ReadAt      *time.Time
	CreatedAt   time.Time
}

// InsertNotification stores one notification. A non-nil DedupeKey makes the
// insert idempotent per user; it reports whether a row was created.
func InsertNotification(ctx context.Context, q Querier, n Notification) (bool, error) {
	result, err := q.ExecContext(ctx, `INSERT INTO notifications(id,user_id,kind,ticket_id,comment_id,actor_user_id,excerpt,dedupe_key,read_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`,
		n.ID, n.UserID, n.Kind, n.TicketID, n.CommentID, n.ActorUserID, n.Excerpt, n.DedupeKey, NullMillis(n.ReadAt), Millis(n.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("store: insert notification: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: notification rows affected: %w", err)
	}
	return count > 0, nil
}

// ListNotifications returns a page of the user's notifications, newest first,
// and the total number matching.
func ListNotifications(ctx context.Context, q Querier, userID string, unreadOnly bool, offset, limit int) ([]Notification, int, error) {
	where := " WHERE user_id=?"
	if unreadOnly {
		where += " AND read_at IS NULL"
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM notifications`+where, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count notifications: %w", err)
	}
	rows, err := q.QueryContext(ctx, `SELECT id,user_id,kind,ticket_id,comment_id,actor_user_id,excerpt,dedupe_key,read_at,created_at FROM notifications`+where+` ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		var comment, actor, dedupe sql.NullString
		var read sql.NullInt64
		var created int64
		if err = rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.TicketID, &comment, &actor, &n.Excerpt, &dedupe, &read, &created); err != nil {
			return nil, 0, fmt.Errorf("store: scan notification: %w", err)
		}
		n.CommentID, n.ActorUserID, n.DedupeKey = pf15StringPtr(comment), pf15StringPtr(actor), pf15StringPtr(dedupe)
		n.ReadAt = TimePtr(read)
		n.CreatedAt = Time(created)
		out = append(out, n)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate notifications: %w", err)
	}
	return out, total, nil
}

// CountUnreadNotifications returns the user's unread notification count.
func CountUnreadNotifications(ctx context.Context, q Querier, userID string) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE user_id=? AND read_at IS NULL`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count unread notifications: %w", err)
	}
	return n, nil
}

// MarkNotificationRead marks one of the user's notifications read. It returns
// ErrNotFound when the notification does not exist or belongs to another user.
func MarkNotificationRead(ctx context.Context, q Querier, userID, id string, now time.Time) error {
	var exists int
	if err := q.QueryRowContext(ctx, `SELECT 1 FROM notifications WHERE id=? AND user_id=?`, id, userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("store: find notification: %w", err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE notifications SET read_at=? WHERE id=? AND user_id=? AND read_at IS NULL`, Millis(now), id, userID); err != nil {
		return fmt.Errorf("store: mark notification read: %w", err)
	}
	return nil
}

// MarkAllNotificationsRead marks every unread notification of the user read and
// returns how many changed.
func MarkAllNotificationsRead(ctx context.Context, q Querier, userID string, now time.Time) (int, error) {
	result, err := q.ExecContext(ctx, `UPDATE notifications SET read_at=? WHERE user_id=? AND read_at IS NULL`, Millis(now), userID)
	if err != nil {
		return 0, fmt.Errorf("store: mark all notifications read: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: notifications rows affected: %w", err)
	}
	return int(count), nil
}
