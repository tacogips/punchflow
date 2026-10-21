package webhook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func pf09InsertPending(t *testing.T, ctx context.Context, db *store.DB, id, webhookID string, at time.Time) {
	t.Helper()
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: id, WebhookID: webhookID, EventType: "ticket_created", Body: []byte(`{}`), Status: model.WebhookStatusPending, NextAttemptAt: at, CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
}

// WH-03: a backlog of due rows belonging to disabled webhooks must not starve
// deliverable rows of enabled webhooks, while manual resends still go out.
func TestWebhookDisabledBacklogDoesNotStarveEnabled(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	var disabledHits atomic.Int32
	var enabledHits atomic.Int32
	disabledReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		disabledHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer disabledReceiver.Close()
	enabledReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		enabledHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer enabledReceiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "off", Name: "off", URL: disabledReceiver.URL, Enabled: false})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "on", Name: "on", URL: enabledReceiver.URL, Enabled: true})
	const backlog = pf09DueBatchLimit + 5
	old := now.Add(-time.Hour)
	if err := db.WithTx(ctx, func(tx *store.Tx) error {
		for i := 0; i < backlog; i++ {
			ev := model.WebhookDelivery{ID: fmt.Sprintf("old-%04d", i), WebhookID: "off", EventType: "ticket_created", Body: []byte(`{}`), Status: model.WebhookStatusPending, NextAttemptAt: old, CreatedAt: old}
			if err := store.InsertWebhookEvent(ctx, tx, ev); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pf09InsertPending(t, ctx, db, "fresh", "on", now)

	svc := New(db, pf09TestBuilder{}, clock, nil, Config{})
	count, err := svc.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || enabledHits.Load() != 1 || disabledHits.Load() != 0 {
		t.Fatalf("RunOnce = %d, enabled hits = %d, disabled hits = %d; want 1, 1, 0", count, enabledHits.Load(), disabledHits.Load())
	}
	fresh, err := store.GetWebhookEvent(ctx, db.Q(), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != model.WebhookStatusSucceeded {
		t.Fatalf("fresh status = %q, want succeeded", fresh.Status)
	}
	// Disabled rows are preserved untouched (still pending), not failed or dropped.
	row, err := store.GetWebhookEvent(ctx, db.Q(), "old-0000")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != model.WebhookStatusPending || row.AttemptCount != 0 {
		t.Fatalf("disabled backlog row = %#v, want untouched pending", row)
	}

	// A manual resend of one disabled-webhook row is still attempted, even with
	// the backlog ahead of it.
	if err := svc.Resend(ctx, db.Q(), "old-0004"); err != nil {
		t.Fatal(err)
	}
	count, err = svc.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || disabledHits.Load() != 1 {
		t.Fatalf("manual resend RunOnce = %d, disabled hits = %d; want 1, 1", count, disabledHits.Load())
	}
}

// WH-04: rows deleted between batch selection and lookup cancel only that delivery.
func TestWebhookDeletedBetweenSelectionAndLookup(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	var hits atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "event", "hook", now)
	svc := New(db, pf09TestBuilder{}, kftime.NewFakeClock(now), nil, Config{})
	if err := svc.Resend(ctx, db.Q(), "event"); err != nil {
		t.Fatal(err)
	}
	events, err := store.DueWebhookEvents(ctx, db.Q(), now, 10, nil)
	if err != nil || len(events) != 1 {
		t.Fatalf("due events = %d, %v", len(events), err)
	}
	if err := store.DeleteWebhook(ctx, db.Q(), "hook"); err != nil {
		t.Fatal(err)
	}
	attempted, err := svc.pf09Process(ctx, events[0], now)
	if err != nil || attempted {
		t.Fatalf("pf09Process after deletion = %v, %v; want false, nil", attempted, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("receiver was called %d times for a deleted webhook", hits.Load())
	}
	if svc.pf09TakeManual("event") {
		t.Fatal("manual flag survived deletion of the delivery")
	}
}

// WH-04: deleting the webhook while its HTTP request is outstanding must not
// surface an error, and the rest of the batch is still delivered.
func TestWebhookDeletedDuringOutstandingRequest(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	var deleteErr atomic.Value
	doomed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := store.DeleteWebhook(ctx, db.Q(), "doomed"); err != nil {
			deleteErr.Store(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer doomed.Close()
	var survivorHits atomic.Int32
	survivor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		survivorHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer survivor.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "doomed", Name: "doomed", URL: doomed.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "survivor", Name: "survivor", URL: survivor.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "doomed-event", "doomed", now)
	pf09InsertPending(t, ctx, db, "survivor-event", "survivor", now)

	svc := New(db, pf09TestBuilder{}, kftime.NewFakeClock(now), nil, Config{})
	count, err := svc.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce returned error after concurrent deletion: %v", err)
	}
	if v := deleteErr.Load(); v != nil {
		t.Fatalf("delete in handler: %v", v)
	}
	if count != 1 || survivorHits.Load() != 1 {
		t.Fatalf("RunOnce = %d, survivor hits = %d; want 1, 1", count, survivorHits.Load())
	}
	if _, err := store.GetWebhookEvent(ctx, db.Q(), "doomed-event"); err == nil {
		t.Fatal("doomed event still exists")
	}
}

// WH-04: the dispatcher loop survives a deletion mid-request and a transient
// store failure, and keeps delivering afterwards; only cancellation stops it.
func TestWebhookRunSurvivesDeletionAndStoreErrors(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	doomed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = store.DeleteWebhook(ctx, db.Q(), "doomed")
		w.WriteHeader(http.StatusOK)
	}))
	defer doomed.Close()
	delivered := make(chan struct{}, 64)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "doomed", Name: "doomed", URL: doomed.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "good", Name: "good", URL: good.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "doomed-event", "doomed", now)

	svc := New(db, pf09TestBuilder{}, kftime.NewFakeClock(now), nil, Config{PollEvery: 5 * time.Millisecond})
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Run(runCtx) }()

	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			select {
			case err := <-done:
				t.Fatalf("Run stopped early: %v", err)
			case <-time.After(2 * time.Millisecond):
			}
		}
	}
	waitFor("doomed webhook deleted", func() bool {
		_, err := store.GetWebhook(ctx, db.Q(), "doomed")
		return err != nil
	})

	// Force attempt recording to fail with a store error, then recover.
	if _, err := db.Q().ExecContext(ctx, "CREATE TRIGGER pf09_block_attempts BEFORE INSERT ON webhook_attempts BEGIN SELECT RAISE(ABORT, 'forced store failure'); END"); err != nil {
		t.Fatal(err)
	}
	pf09InsertPending(t, ctx, db, "good-event", "good", now)
	svc.Notify()
	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("good webhook was not requested while recording is failing")
	}
	select {
	case err := <-done:
		t.Fatalf("Run stopped on a store error: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if _, err := db.Q().ExecContext(ctx, "DROP TRIGGER pf09_block_attempts"); err != nil {
		t.Fatal(err)
	}
	waitFor("good event recorded after store recovery", func() bool {
		ev, err := store.GetWebhookEvent(ctx, db.Q(), "good-event")
		return err == nil && ev.Status == model.WebhookStatusSucceeded
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}
