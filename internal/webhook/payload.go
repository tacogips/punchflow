package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

// Payloads builds Kickflow-compatible event bodies.
type Payloads struct{}

// NewPayloads returns a webhook payload builder.
func NewPayloads() *Payloads { return &Payloads{} }

// Build returns the JSON body for one supported webhook event.
func (p *Payloads) Build(ctx context.Context, q store.Querier, ev model.WebhookEvent) ([]byte, error) {
	if !slices.Contains(model.WebhookEventTypes, ev.Type) {
		return nil, fmt.Errorf("webhook: unsupported event type %q", ev.Type)
	}
	tenant, err := present.WebhookTenant(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("webhook: resolve tenant: %w", err)
	}
	user, err := present.User(ctx, q, ev.ActorUserID, present.UserOpts{})
	if err != nil {
		return nil, fmt.Errorf("webhook: resolve actor: %w", err)
	}
	data := map[string]any{}
	switch ev.Type {
	case model.WebhookEventPing:
		data["message"] = "ping"
	case model.WebhookEventCommentCreated, model.WebhookEventCommentUpdated:
		comment, e := present.Comment(ctx, q, ev.CommentID)
		if e != nil {
			return nil, fmt.Errorf("webhook: build comment: %w", e)
		}
		ticket, e := present.TicketWithStep(ctx, q, ev.TicketID)
		if e != nil {
			return nil, fmt.Errorf("webhook: build ticket: %w", e)
		}
		data["comment"], data["ticket"] = comment, ticket
	default:
		ticket, e := present.TicketDetailWebhook(ctx, q, ev.TicketID)
		if e != nil {
			return nil, fmt.Errorf("webhook: build ticket: %w", e)
		}
		data["ticket"] = ticket
	}
	body, err := json.Marshal(map[string]any{"eventType": ev.Type, "tenant": tenant, "user": user, "data": data})
	if err != nil {
		return nil, fmt.Errorf("webhook: encode payload: %w", err)
	}
	return body, nil
}
