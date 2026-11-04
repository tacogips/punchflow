package present

import (
	"context"
	"errors"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// LabelJSON is the kickflow Label schema.
type LabelJSON map[string]any

// SubStatusJSON is the kickflow SubStatus schema.
type SubStatusJSON map[string]any

// Label presents one label.
func Label(m model.Label) LabelJSON {
	return LabelJSON{"id": m.ID, "name": m.Name, "description": nullableString(m.Description), "color": m.Color, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}
}

// SubStatus presents one sub-status.
func SubStatus(m model.SubStatus) SubStatusJSON {
	return SubStatusJSON{"id": m.ID, "code": m.Code, "name": m.Name, "notes": nullableString(m.Notes), "allowWithdrawal": m.AllowWithdrawal, "allowRejection": m.AllowRejection, "allowDenial": m.AllowDenial, "createdAt": pf08Time(m.CreatedAt), "updatedAt": pf08Time(m.UpdatedAt)}
}

// Labels presents a label list; the result is never nil so it encodes as [].
func Labels(items []model.Label) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, Label(item))
	}
	return out
}

// SubStatuses presents a sub-status list; the result is never nil.
func SubStatuses(items []model.SubStatus) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, SubStatus(item))
	}
	return out
}

// subStatusByCode presents the sub-status with the code, or nil when the code
// is empty or no longer defined.
func subStatusByCode(ctx context.Context, q store.Querier, code *string) (any, error) {
	if code == nil || *code == "" {
		return nil, nil
	}
	item, err := store.SubStatusByCode(ctx, q, *code)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return SubStatus(item), nil
}

// ticketSubStatus presents the sub-status attached to a ticket, or nil.
func ticketSubStatus(ctx context.Context, q store.Querier, ticketID string) (any, error) {
	id, err := store.TicketSubStatusID(ctx, q, ticketID)
	if err != nil || id == nil {
		return nil, err
	}
	item, err := store.SubStatusByID(ctx, q, *id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return SubStatus(item), nil
}

// ticketLabels presents the labels attached to a ticket.
func ticketLabels(ctx context.Context, q store.Querier, ticketID string) ([]any, error) {
	items, err := store.TicketLabels(ctx, q, ticketID)
	if err != nil {
		return nil, err
	}
	return Labels(items), nil
}
