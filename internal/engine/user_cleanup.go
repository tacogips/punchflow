package engine

import (
	"context"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// CleanupDeletedUser applies kickflow's user-deletion rules inside the caller's
// transaction: the user's draft, in-progress and rejected tickets are archived,
// and the user's outstanding approver assignments on other in-progress tickets
// are removed. Suspension does neither. It returns a ticket_archived webhook
// event for every archived ticket, attributed to the acting administrator.
func (e *Engine) CleanupDeletedUser(ctx context.Context, tx *store.Tx, actor Actor, userID string) ([]model.WebhookEvent, error) {
	now := e.clock.Now()
	ids, err := store.UserOpenAuthoredTicketIDs(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	events := make([]model.WebhookEvent, 0, len(ids))
	for _, id := range ids {
		ticket, err := store.EngLoadTicket(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		ticket.Status = model.TicketStatusArchived
		ticket.ArchivedAt = &now
		ticket.UpdatedAt = now
		if err = store.EngUpdateTicket(ctx, tx, ticket); err != nil {
			return nil, err
		}
		events = append(events, pf14Event(model.WebhookEventTicketArchived, id, actor.UserID))
	}
	// Archived tickets are no longer in progress, so only assignments on
	// tickets authored by others remain to be removed.
	if _, err = store.DeleteUserOutstandingAssignments(ctx, tx, userID); err != nil {
		return nil, err
	}
	return events, nil
}
