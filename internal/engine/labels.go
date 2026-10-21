package engine

import (
	"context"
	"errors"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// AttachLabel attaches a label to a ticket. Any user who can view the ticket
// may do so (INF-64). Attaching an attached label changes nothing and emits no
// event; a real change emits ticket_updated.
func (e *Engine) AttachLabel(ctx context.Context, tx *store.Tx, a Actor, ticketID, labelID string) ([]model.WebhookEvent, error) {
	return e.changeLabel(ctx, tx, a, ticketID, labelID, true)
}

// DetachLabel removes a label from a ticket; detaching an absent label is a no-op.
func (e *Engine) DetachLabel(ctx context.Context, tx *store.Tx, a Actor, ticketID, labelID string) ([]model.WebhookEvent, error) {
	return e.changeLabel(ctx, tx, a, ticketID, labelID, false)
}

func (e *Engine) changeLabel(ctx context.Context, tx *store.Tx, a Actor, ticketID, labelID string, attach bool) ([]model.WebhookEvent, error) {
	t, err := e.pf15Guard(ctx, tx, a, ticketID)
	if err != nil {
		return nil, err
	}
	if _, err = store.LabelByID(ctx, tx, labelID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, kferr.ValidationFailed(map[string][]string{"labelId": {"is invalid"}})
		}
		return nil, err
	}
	now := e.clock.Now()
	var changed bool
	if attach {
		changed, err = store.AddTicketLabel(ctx, tx, ticketID, labelID, store.Millis(now))
	} else {
		changed, err = store.RemoveTicketLabel(ctx, tx, ticketID, labelID)
	}
	if err != nil || !changed {
		return []model.WebhookEvent{}, err
	}
	t.UpdatedAt = now
	if err = store.EngUpdateTicket(ctx, tx, t); err != nil {
		return nil, err
	}
	return []model.WebhookEvent{pf14Event(model.WebhookEventTicketUpdated, ticketID, a.UserID)}, nil
}
