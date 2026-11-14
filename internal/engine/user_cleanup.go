package engine

import (
	"context"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// CleanupDeletedUser (INF-79) applies kickflow's user-deletion rules inside the caller's
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
		ticket.ClosedAt = &now
		ticket.UpdatedAt = now
		if err = e.pf15SyncSubStatus(ctx, tx, &ticket, now); err != nil {
			return nil, err
		}
		if err = store.EngUpdateTicket(ctx, tx, ticket); err != nil {
			return nil, err
		}
		events = append(events, pf14Event(model.WebhookEventTicketArchived, id, actor.UserID))
	}
	// Archived tickets are no longer in progress, so only assignments on
	// tickets authored by others remain to be removed.
	err = store.DeleteUserOutstandingAssignments(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	// INF-LIFE-API-06: deleting an approver removes the identity but does not
	// satisfy or bypass the approval obligation. Empty current steps remain
	// waiting for administrator repair, even if a fallback was configured.
	return events, nil
}

// CleanupDeletedTeam archives unfinished tickets authored from the deleted
// team while preserving completed tickets' author-team snapshots (API-05).
func (e *Engine) CleanupDeletedTeam(ctx context.Context, tx *store.Tx, actor Actor, teamID string) ([]model.WebhookEvent, error) {
	ids, err := store.TeamOpenAuthoredTicketIDs(ctx, tx, teamID)
	if err != nil {
		return nil, err
	}
	now := e.clock.Now()
	events := make([]model.WebhookEvent, 0, len(ids))
	for _, id := range ids {
		ticket, er := store.EngLoadTicket(ctx, tx, id)
		if er != nil {
			return nil, er
		}
		ticket.Status, ticket.ArchivedAt, ticket.ClosedAt, ticket.UpdatedAt = model.TicketStatusArchived, &now, &now, now
		if er = e.pf15SyncSubStatus(ctx, tx, &ticket, now); er != nil {
			return nil, er
		}
		if er = store.EngUpdateTicket(ctx, tx, ticket); er != nil {
			return nil, er
		}
		events = append(events, pf14Event(model.WebhookEventTicketArchived, id, actor.UserID))
	}
	return events, nil
}
