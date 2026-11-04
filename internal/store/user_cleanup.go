package store

import (
	"context"
	"fmt"

	"punchflow/internal/model"
)

// UserOpenAuthoredTicketIDs returns the IDs of the user's draft, in-progress and
// rejected tickets, which are archived when the user is deleted.
func UserOpenAuthoredTicketIDs(ctx context.Context, q Querier, userID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT id FROM tickets WHERE author_id=? AND status IN (?,?,?) ORDER BY created_at,id",
		userID, model.TicketStatusDraft, model.TicketStatusInProgress, model.TicketStatusRejected)
	if err != nil {
		return nil, fmt.Errorf("store: list authored open tickets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan authored ticket: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read authored tickets: %w", err)
	}
	return ids, nil
}

// DeleteUserOutstandingAssignments removes the user's not-yet-completed approver
// assignments on in-progress tickets. The waiting steps remain in place for
// administrator repair (INF-LIFE-API-06).
func DeleteUserOutstandingAssignments(ctx context.Context, q Querier, userID string) error {
	if _, err := q.ExecContext(ctx, "DELETE FROM ticket_assignees WHERE user_id=? AND completed=0 AND ticket_id IN (SELECT id FROM tickets WHERE status=?)",
		userID, model.TicketStatusInProgress); err != nil {
		return fmt.Errorf("store: delete outstanding assignments: %w", err)
	}
	return nil
}
