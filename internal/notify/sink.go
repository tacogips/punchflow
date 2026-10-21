package notify

import (
	"context"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

// EventSink is the transactional outbox interface the HTTP handlers use; it is
// satisfied by the webhook service.
type EventSink interface {
	Enqueue(ctx context.Context, q store.Querier, events []model.WebhookEvent) error
	Notify()
}

// Sink decorates an EventSink so that every enqueued engine event is also fanned
// out to the notification inbox inside the same transaction.
type Sink struct {
	inner   EventSink
	service *Service
}

// NewSink wraps inner. Enqueue first enqueues the webhook deliveries and then
// writes the notifications; either failure rolls the transaction back.
func NewSink(inner EventSink, service *Service) *Sink {
	return &Sink{inner: inner, service: service}
}

// Enqueue implements EventSink.
func (s *Sink) Enqueue(ctx context.Context, q store.Querier, events []model.WebhookEvent) error {
	if err := s.inner.Enqueue(ctx, q, events); err != nil {
		return err
	}
	return s.service.Fanout(ctx, q, events)
}

// Notify implements EventSink.
func (s *Sink) Notify() { s.inner.Notify() }
