package present

import (
	"context"
	"encoding/json"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func AuditLog(ctx context.Context, q store.Querier, m model.AuditLog) (AuditLogJSON, error) {
	var user any
	if m.UserID != nil {
		u, e := User(ctx, q, *m.UserID, UserOpts{})
		if e != nil && e != store.ErrNotFound {
			return nil, e
		}
		if e == nil {
			user = u
		}
	}
	var data any
	if len(m.Data) > 0 {
		if json.Unmarshal(m.Data, &data) != nil {
			data = nil
		}
	}
	return AuditLogJSON{"id": m.ID, "user": user, "action": m.Action, "data": data, "remoteIp": m.RemoteIP, "requestId": m.RequestID, "systemType": m.SystemType, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.CreatedAt)}, nil
}
