package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// AuditFilter selects the audit logs to export; it mirrors the audit log list
// filters (user and creation period).
type AuditFilter struct {
	UserID string
	Start  *time.Time
	End    *time.Time
	Desc   bool
}

// WriteAuditLogs writes every audit log matching the filter, not just one page.
func WriteAuditLogs(ctx context.Context, q store.Querier, w io.Writer, filter AuditFilter) error {
	logs, _, err := store.AuditLogList(ctx, q, 0, -1, filter.UserID, filter.Start, filter.End, filter.Desc)
	if err != nil {
		return fmt.Errorf("export: list audit logs: %w", err)
	}
	out, err := NewWriter(w)
	if err != nil {
		return err
	}
	if err = out.Write([]string{"Created at", "Action", "User ID", "User name", "User email", "Ticket ID", "System type", "Remote IP", "Request ID", "Log ID", "Data"}); err != nil {
		return err
	}
	users := map[string]model.User{}
	for _, log := range logs {
		var user model.User
		if log.UserID != nil {
			cached, ok := users[*log.UserID]
			if !ok {
				cached, err = store.GetUser(ctx, q, *log.UserID)
				if err != nil && !errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("export: load audit user: %w", err)
				}
				users[*log.UserID] = cached
			}
			user = cached
		}
		row := []string{
			kftime.Format(log.CreatedAt), log.Action, deref(log.UserID),
			strings.TrimSpace(user.LastName + " " + user.FirstName), user.Email,
			deref(log.TicketID), deref(log.SystemType), deref(log.RemoteIP), deref(log.RequestID), log.ID, string(log.Data),
		}
		if err = out.Write(row); err != nil {
			return err
		}
	}
	return out.Flush()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
