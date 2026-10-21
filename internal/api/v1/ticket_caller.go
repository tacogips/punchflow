package v1

import (
	"punchflow/internal/access"
	"punchflow/internal/model"
)

// pf16Caller describes the requesting user for section-level view restrictions.
func pf16Caller(userID string, permissions map[string]bool) *access.Caller {
	return &access.Caller{UserID: userID, TicketRead: permissions[model.PermissionTicketRead]}
}
