package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"punchflow/internal/model"
)

// ListWebhooks returns a page and total count, ordered by creation time and ID.
func ListWebhooks(ctx context.Context, q Querier, offset, limit int) ([]model.Webhook, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM webhooks").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count webhooks: %w", err)
	}
	rows, err := q.QueryContext(ctx, "SELECT id,name,url,secret,enabled,workflow_ids,event_types,first_failure_at,disabled_at,created_at,updated_at FROM webhooks ORDER BY created_at,id LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list webhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.Webhook, 0)
	for rows.Next() {
		v, err := pf09ScanWebhook(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate webhooks: %w", err)
	}
	return out, total, nil
}

// GetWebhook loads a webhook by ID.
func GetWebhook(ctx context.Context, q Querier, id string) (model.Webhook, error) {
	row := q.QueryRowContext(ctx, "SELECT id,name,url,secret,enabled,workflow_ids,event_types,first_failure_at,disabled_at,created_at,updated_at FROM webhooks WHERE id=?", id)
	v, err := pf09ScanWebhook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Webhook{}, ErrNotFound
	}
	return v, err
}

// InsertWebhook inserts a webhook, assigning an ID and timestamps when absent.
func InsertWebhook(ctx context.Context, q Querier, v model.Webhook) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	workflows, err := json.Marshal(v.WorkflowIDs)
	if err != nil {
		return fmt.Errorf("store: encode webhook workflows: %w", err)
	}
	events, err := json.Marshal(v.EventTypes)
	if err != nil {
		return fmt.Errorf("store: encode webhook event types: %w", err)
	}
	_, err = q.ExecContext(ctx, "INSERT INTO webhooks(id,name,url,secret,enabled,workflow_ids,event_types,first_failure_at,disabled_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", v.ID, v.Name, v.URL, v.Secret, v.Enabled, string(workflows), string(events), pf09MillisPtr(v.FirstFailureAt), pf09MillisPtr(v.DisabledAt), Millis(v.CreatedAt), Millis(v.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert webhook: %w", err)
	}
	return nil
}

// UpdateWebhook replaces webhook configuration without modifying delivery state.
func UpdateWebhook(ctx context.Context, q Querier, v model.Webhook) error {
	workflows, err := json.Marshal(v.WorkflowIDs)
	if err != nil {
		return fmt.Errorf("store: encode webhook workflows: %w", err)
	}
	events, err := json.Marshal(v.EventTypes)
	if err != nil {
		return fmt.Errorf("store: encode webhook event types: %w", err)
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = time.Now()
	}
	result, err := q.ExecContext(ctx, "UPDATE webhooks SET name=?,url=?,secret=?,workflow_ids=?,event_types=?,updated_at=? WHERE id=?", v.Name, v.URL, v.Secret, string(workflows), string(events), Millis(v.UpdatedAt), v.ID)
	if err != nil {
		return fmt.Errorf("store: update webhook: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count updated webhook: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetWebhookEnabled applies an explicit enable/disable transition while preserving
// failure state unless enabling from the currently disabled state.
func SetWebhookEnabled(ctx context.Context, q Querier, id string, enabled bool, now time.Time) error {
	result, err := q.ExecContext(ctx, `UPDATE webhooks SET
		enabled=?,
		first_failure_at=CASE WHEN ?=1 AND enabled=0 THEN NULL ELSE first_failure_at END,
		disabled_at=CASE WHEN ?=1 THEN NULL WHEN enabled=1 AND ?=0 THEN ? ELSE disabled_at END,
		updated_at=? WHERE id=?`, enabled, enabled, enabled, enabled, Millis(now), Millis(now), id)
	if err != nil {
		return fmt.Errorf("store: set webhook enabled state: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count webhook enabled state: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWebhook removes a webhook and its delivery history.
func DeleteWebhook(ctx context.Context, q Querier, id string) error {
	result, err := q.ExecContext(ctx, "DELETE FROM webhooks WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("store: delete webhook: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count deleted webhook: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// EnabledWebhooks returns enabled registrations in stable order.
func EnabledWebhooks(ctx context.Context, q Querier) ([]model.Webhook, error) {
	rows, err := q.QueryContext(ctx, "SELECT id,name,url,secret,enabled,workflow_ids,event_types,first_failure_at,disabled_at,created_at,updated_at FROM webhooks WHERE enabled=1 ORDER BY created_at,id")
	if err != nil {
		return nil, fmt.Errorf("store: list enabled webhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.Webhook, 0)
	for rows.Next() {
		v, err := pf09ScanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate enabled webhooks: %w", err)
	}
	return out, nil
}

// InsertWebhookEvent persists immutable payload bytes as a pending outbox row.
func InsertWebhookEvent(ctx context.Context, q Querier, v model.WebhookDelivery) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = v.CreatedAt
	}
	if v.NextAttemptAt.IsZero() {
		v.NextAttemptAt = v.CreatedAt
	}
	if v.Status == "" {
		v.Status = model.WebhookStatusPending
	}
	_, err := q.ExecContext(ctx, "INSERT INTO webhook_events(id,webhook_id,event_type,ticket_id,comment_id,actor_user_id,body,status,attempt_count,next_attempt_at,last_status_code,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", v.ID, v.WebhookID, v.EventType, v.TicketID, v.CommentID, v.ActorUserID, v.Body, v.Status, v.AttemptCount, Millis(v.NextAttemptAt), v.LastStatusCode, Millis(v.CreatedAt), Millis(v.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: insert webhook event: %w", err)
	}
	return nil
}

const dueWebhookEventColumns = "e.id,e.webhook_id,e.event_type,e.ticket_id,e.comment_id,e.actor_user_id,e.body,e.status,e.attempt_count,e.next_attempt_at,e.last_status_code,e.created_at,e.updated_at"

// DueWebhookEvents returns pending rows ready for delivery, oldest first.
//
// Eligibility is applied in SQL before the limit so a backlog of rows that cannot
// be delivered never starves deliverable ones: only rows of enabled webhooks are
// returned, plus the rows listed in manualIDs (operator resends, which are
// attempted even when the webhook is disabled).
func DueWebhookEvents(ctx context.Context, q Querier, now time.Time, limit int, manualIDs []string) ([]model.WebhookDelivery, error) {
	query := "SELECT " + dueWebhookEventColumns + " FROM webhook_events e JOIN webhooks w ON w.id=e.webhook_id WHERE e.status='pending' AND e.next_attempt_at<=?"
	args := []any{Millis(now)}
	if len(manualIDs) == 0 {
		query += " AND w.enabled=1"
	} else {
		placeholders := make([]byte, 0, len(manualIDs)*2)
		for i, id := range manualIDs {
			if i > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
			args = append(args, id)
		}
		query += " AND (w.enabled=1 OR e.id IN (" + string(placeholders) + "))"
	}
	query += " ORDER BY e.next_attempt_at,e.rowid LIMIT ?"
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query due webhook events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.WebhookDelivery, 0)
	for rows.Next() {
		v, err := pf09ScanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate due webhook events: %w", err)
	}
	return out, nil
}

// GetWebhookEvent loads one delivery row.
func GetWebhookEvent(ctx context.Context, q Querier, id string) (model.WebhookDelivery, error) {
	v, err := pf09ScanDelivery(q.QueryRowContext(ctx, "SELECT id,webhook_id,event_type,ticket_id,comment_id,actor_user_id,body,status,attempt_count,next_attempt_at,last_status_code,created_at,updated_at FROM webhook_events WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.WebhookDelivery{}, ErrNotFound
	}
	return v, err
}

// ListWebhookEvents returns a page and total count for one webhook.
func ListWebhookEvents(ctx context.Context, q Querier, webhookID string, offset, limit int) ([]model.WebhookDelivery, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM webhook_events WHERE webhook_id=?", webhookID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count webhook events: %w", err)
	}
	rows, err := q.QueryContext(ctx, "SELECT id,webhook_id,event_type,ticket_id,comment_id,actor_user_id,body,status,attempt_count,next_attempt_at,last_status_code,created_at,updated_at FROM webhook_events WHERE webhook_id=? ORDER BY created_at DESC,rowid DESC LIMIT ? OFFSET ?", webhookID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list webhook events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.WebhookDelivery, 0)
	for rows.Next() {
		v, err := pf09ScanDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate webhook events: %w", err)
	}
	return out, total, nil
}

// MarkEventAttempt updates delivery state after one HTTP attempt.
func MarkEventAttempt(ctx context.Context, q Querier, id string, count int, next time.Time, status string, lastStatus *int, now time.Time) error {
	result, err := q.ExecContext(ctx, "UPDATE webhook_events SET attempt_count=?,next_attempt_at=?,status=?,last_status_code=?,updated_at=? WHERE id=?", count, Millis(next), status, lastStatus, Millis(now), id)
	if err != nil {
		return fmt.Errorf("store: mark webhook attempt: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count marked webhook attempt: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// InsertWebhookAttempt stores request and response details for one attempt.
func InsertWebhookAttempt(ctx context.Context, q Querier, v model.WebhookAttempt) error {
	if v.ID == "" {
		v.ID = NewID()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = v.StartedAt
	}
	headers, err := json.Marshal(v.RequestHeaders)
	if err != nil {
		return fmt.Errorf("store: encode webhook attempt headers: %w", err)
	}
	_, err = q.ExecContext(ctx, "INSERT INTO webhook_attempts(id,event_id,attempt_no,started_at,duration_ms,request_headers,status_code,response_body,error,manual,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", v.ID, v.EventID, v.AttemptNo, Millis(v.StartedAt), v.DurationMS, string(headers), v.StatusCode, v.ResponseBody, v.Error, v.Manual, Millis(v.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: insert webhook attempt: %w", err)
	}
	return nil
}

// ListWebhookAttempts returns attempts in attempt-number order.
func ListWebhookAttempts(ctx context.Context, q Querier, eventID string) ([]model.WebhookAttempt, error) {
	rows, err := q.QueryContext(ctx, "SELECT id,event_id,attempt_no,started_at,duration_ms,request_headers,status_code,response_body,error,manual,created_at FROM webhook_attempts WHERE event_id=? ORDER BY attempt_no", eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list webhook attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]model.WebhookAttempt, 0)
	for rows.Next() {
		var v model.WebhookAttempt
		var started, created int64
		var headers string
		var status sql.NullInt64
		var msg sql.NullString
		var manual int
		if err := rows.Scan(&v.ID, &v.EventID, &v.AttemptNo, &started, &v.DurationMS, &headers, &status, &v.ResponseBody, &msg, &manual, &created); err != nil {
			return nil, fmt.Errorf("store: scan webhook attempt: %w", err)
		}
		v.StartedAt = Time(started)
		v.CreatedAt = Time(created)
		if status.Valid {
			x := int(status.Int64)
			v.StatusCode = &x
		}
		if msg.Valid {
			x := msg.String
			v.Error = &x
		}
		v.Manual = manual != 0
		if err := json.Unmarshal([]byte(headers), &v.RequestHeaders); err != nil {
			return nil, fmt.Errorf("store: decode webhook attempt headers: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate webhook attempts: %w", err)
	}
	return out, nil
}

// SetWebhookFailureState records the first failure or clears it after success; disable marks remaining pending deliveries failed.
func SetWebhookFailureState(ctx context.Context, q Querier, id string, first *time.Time, disable bool, now time.Time) error {
	var disabled any
	if disable {
		disabled = Millis(now)
	}
	result, err := q.ExecContext(ctx, "UPDATE webhooks SET first_failure_at=?,enabled=CASE WHEN ? THEN 0 ELSE enabled END,disabled_at=CASE WHEN ? THEN ? ELSE disabled_at END,updated_at=? WHERE id=?", pf09MillisPtr(first), disable, disable, disabled, Millis(now), id)
	if err != nil {
		return fmt.Errorf("store: update webhook failure state: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: count webhook failure state: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	if disable {
		if _, err := q.ExecContext(ctx, "UPDATE webhook_events SET status='failed',updated_at=? WHERE webhook_id=? AND status='pending'", Millis(now), id); err != nil {
			return fmt.Errorf("store: fail pending webhook events: %w", err)
		}
	}
	return nil
}

func pf09ScanWebhook(row interface{ Scan(...any) error }) (model.Webhook, error) {
	var v model.Webhook
	var secret sql.NullString
	var enabled int
	var workflows, events string
	var first, disabled sql.NullInt64
	var created, updated int64
	if err := row.Scan(&v.ID, &v.Name, &v.URL, &secret, &enabled, &workflows, &events, &first, &disabled, &created, &updated); err != nil {
		return model.Webhook{}, fmt.Errorf("store: scan webhook: %w", err)
	}
	if secret.Valid {
		x := secret.String
		v.Secret = &x
	}
	v.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(workflows), &v.WorkflowIDs); err != nil {
		return model.Webhook{}, fmt.Errorf("store: decode webhook workflows: %w", err)
	}
	if err := json.Unmarshal([]byte(events), &v.EventTypes); err != nil {
		return model.Webhook{}, fmt.Errorf("store: decode webhook event types: %w", err)
	}
	if first.Valid {
		x := Time(first.Int64)
		v.FirstFailureAt = &x
	}
	if disabled.Valid {
		x := Time(disabled.Int64)
		v.DisabledAt = &x
	}
	v.CreatedAt = Time(created)
	v.UpdatedAt = Time(updated)
	return v, nil
}
func pf09ScanDelivery(row interface{ Scan(...any) error }) (model.WebhookDelivery, error) {
	var v model.WebhookDelivery
	var ticket, comment, actor sql.NullString
	var status sql.NullInt64
	var next, created, updated int64
	if err := row.Scan(&v.ID, &v.WebhookID, &v.EventType, &ticket, &comment, &actor, &v.Body, &v.Status, &v.AttemptCount, &next, &status, &created, &updated); err != nil {
		return model.WebhookDelivery{}, fmt.Errorf("store: scan webhook event: %w", err)
	}
	if ticket.Valid {
		x := ticket.String
		v.TicketID = &x
	}
	if comment.Valid {
		x := comment.String
		v.CommentID = &x
	}
	if actor.Valid {
		x := actor.String
		v.ActorUserID = &x
	}
	if status.Valid {
		x := int(status.Int64)
		v.LastStatusCode = &x
	}
	v.NextAttemptAt = Time(next)
	v.CreatedAt = Time(created)
	v.UpdatedAt = Time(updated)
	return v, nil
}
func pf09MillisPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return Millis(*t)
}
