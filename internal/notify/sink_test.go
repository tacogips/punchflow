package notify

import (
	"context"
	"errors"
	"testing"

	"punchflow/internal/engine"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type fakeSink struct {
	err      error
	enqueued int
	notified int
}

func (f *fakeSink) Enqueue(context.Context, store.Querier, []model.WebhookEvent) error {
	f.enqueued++
	return f.err
}
func (f *fakeSink) Notify() { f.notified++ }

type panicViewer struct{}

func (panicViewer) CanView(context.Context, store.Querier, engine.Actor, string) error {
	panic("viewer must not be called")
}

func TestSinkEnqueueFailureSkipsFanout(t *testing.T) {
	inner := &fakeSink{err: errors.New("outbox unavailable")}
	// A nil querier would panic if the fan-out ran after a failed enqueue; an
	// event without a ticket never reaches the store either way.
	sink := NewSink(inner, New(panicViewer{}, nil))
	err := sink.Enqueue(t.Context(), nil, []model.WebhookEvent{{Type: model.WebhookEventTicketApproved, TicketID: "t1"}})
	if !errors.Is(err, inner.err) {
		t.Fatalf("Enqueue error = %v, want the inner error", err)
	}
	if inner.enqueued != 1 {
		t.Fatalf("inner enqueued %d times", inner.enqueued)
	}
}

func TestSinkForwardsNotify(t *testing.T) {
	inner := &fakeSink{}
	sink := NewSink(inner, New(panicViewer{}, nil))
	sink.Notify()
	if inner.notified != 1 {
		t.Fatalf("Notify forwarded %d times", inner.notified)
	}
}

func TestFanoutIgnoresEventsWithoutNotifications(t *testing.T) {
	service := New(panicViewer{}, nil)
	events := []model.WebhookEvent{
		{Type: model.WebhookEventPing},
		{Type: model.WebhookEventTicketCreated, TicketID: "t1"},
		{Type: model.WebhookEventTicketUpdated, TicketID: "t1"},
		{Type: model.WebhookEventCommentUpdated, TicketID: "t1", CommentID: "c1"},
		{Type: model.WebhookEventTicketApproved},
	}
	// A nil querier proves no event above reads or writes the store.
	if err := service.Fanout(t.Context(), nil, events); err != nil {
		t.Fatalf("Fanout: %v", err)
	}
}
