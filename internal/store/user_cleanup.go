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

// OutstandingAssignment identifies the ticket step a removed approver
// assignment belonged to.
type OutstandingAssignment struct {
	TicketID string
	StepID   string
}

// DeleteUserOutstandingAssignments removes the user's not-yet-completed approver
// assignments on in-progress tickets and returns the ticket step each removed
// assignment belonged to (ordered by ticket, then step).
func DeleteUserOutstandingAssignments(ctx context.Context, q Querier, userID string) ([]OutstandingAssignment, error) {
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT ta.ticket_id, ta.ticket_step_id FROM ticket_assignees ta JOIN tickets t ON t.id=ta.ticket_id WHERE ta.user_id=? AND ta.completed=0 AND t.status=? ORDER BY ta.ticket_id, ta.ticket_step_id",
		userID, model.TicketStatusInProgress)
	if err != nil {
		return nil, fmt.Errorf("store: list outstanding assignments: %w", err)
	}
	var out []OutstandingAssignment
	for rows.Next() {
		var x OutstandingAssignment
		if err := rows.Scan(&x.TicketID, &x.StepID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan outstanding assignment: %w", err)
		}
		out = append(out, x)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("store: close outstanding assignments: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if _, err := q.ExecContext(ctx, "DELETE FROM ticket_assignees WHERE user_id=? AND completed=0 AND ticket_id IN (SELECT id FROM tickets WHERE status=?)",
		userID, model.TicketStatusInProgress); err != nil {
		return nil, fmt.Errorf("store: delete outstanding assignments: %w", err)
	}
	return out, nil
}
