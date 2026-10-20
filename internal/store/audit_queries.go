package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"punchflow/internal/model"
)

// AuditLogList returns audit logs matching the optional filters and total count.
func AuditLogList(ctx context.Context, q Querier, offset, limit int, userID string, start, end *time.Time, desc bool) ([]model.AuditLog, int, error) {
	where := " WHERE 1=1"
	args := []any{}
	if userID != "" {
		where += " AND user_id=?"
		args = append(args, userID)
	}
	if start != nil {
		where += " AND created_at>=?"
		args = append(args, Millis(*start))
	}
	if end != nil {
		where += " AND created_at<=?"
		args = append(args, Millis(*end))
	}
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count audit logs: %w", err)
	}
	order := "ASC"
	if desc {
		order = "DESC"
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := q.QueryContext(ctx, `SELECT id,user_id,action,data,remote_ip,request_id,system_type,ticket_id,created_at FROM audit_logs`+where+` ORDER BY created_at `+order+`, id `+order+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list audit logs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []model.AuditLog{}
	for rows.Next() {
		var item model.AuditLog
		var userID, data, remoteIP, requestID, systemType, ticketID sql.NullString
		var createdAt int64
		if err := rows.Scan(&item.ID, &userID, &item.Action, &data, &remoteIP, &requestID, &systemType, &ticketID, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("store: scan audit log: %w", err)
		}
		item.UserID = pf21AuditStringPtr(userID)
		item.RemoteIP = pf21AuditStringPtr(remoteIP)
		item.RequestID = pf21AuditStringPtr(requestID)
		item.SystemType = pf21AuditStringPtr(systemType)
		item.TicketID = pf21AuditStringPtr(ticketID)
		if data.Valid {
			item.Data = []byte(data.String)
		}
		item.CreatedAt = Time(createdAt)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: iterate audit logs: %w", err)
	}
	return out, total, nil
}

func pf21AuditStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
