package store

import (
	"context"
	"encoding/json"
	"fmt"
	"punchflow/internal/model"
	"time"
)

// InsertAuditLog inserts an audit record, filling its ID and creation time when absent.
func InsertAuditLog(ctx context.Context, q Querier, a model.AuditLog) error {
	if a.ID == "" {
		a.ID = NewID()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	data := any(nil)
	if len(a.Data) > 0 {
		var compact json.RawMessage = a.Data
		if !json.Valid(compact) {
			return fmt.Errorf("store: audit data is not valid JSON")
		}
		data = string(compact)
	}
	_, err := q.ExecContext(ctx, "INSERT INTO audit_logs(id,user_id,action,data,remote_ip,request_id,system_type,ticket_id,created_at) VALUES(?,?,?,?,?,?,?,?,?)", a.ID, a.UserID, a.Action, data, a.RemoteIP, a.RequestID, a.SystemType, a.TicketID, Millis(a.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: insert audit log: %w", err)
	}
	return nil
}
