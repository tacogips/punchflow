package webhook

import (
	"context"
	"errors"
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

func TestWebhookRetryThenSuccess(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	var requests atomic.Int32
	deliveryIDs := make(chan string, 3)
	signatures := make(chan string, 3)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		deliveryIDs <- r.Header.Get("X-Kickflow-Delivery")
		signatures <- r.Header.Get("X-Kickflow-Signature")
		if r.Header.Get("User-Agent") != "kickflow-Hookshot/v1" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers missing: %#v", r.Header)
		}
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("retry"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	secret := "secret"
	hook := model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Secret: &secret, Enabled: true}
	pf09InsertWebhook(t, ctx, db, hook)
	body := []byte(`{"x":1}`)
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "delivery-id", WebhookID: "hook", EventType: "ticket_created", Body: body, Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{RetryBase: 10 * time.Second})
	for attempt := 1; attempt <= 3; attempt++ {
		count, err := svc.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("attempt %d RunOnce = %d", attempt, count)
		}
		if got := <-deliveryIDs; got != "delivery-id" {
			t.Errorf("attempt %d delivery ID = %q", attempt, got)
		}
		if got, want := <-signatures, Sign([]byte(secret), body); got != want {
			t.Errorf("attempt %d signature = %q, want %q", attempt, got, want)
		}
		ev, err := store.GetWebhookEvent(ctx, db.Q(), "delivery-id")
		if err != nil {
			t.Fatal(err)
		}
		if ev.AttemptCount != attempt {
			t.Fatalf("attempt_count = %d, want %d", ev.AttemptCount, attempt)
		}
		if attempt < 3 {
			if ev.Status != "pending" {
				t.Fatalf("after attempt %d status = %q", attempt, ev.Status)
			}
			delay := Backoff(10*time.Second, attempt)
			if !ev.NextAttemptAt.Equal(clock.Now().Add(delay)) {
				t.Fatalf("attempt %d next retry = %s, want %s", attempt, ev.NextAttemptAt, clock.Now().Add(delay))
			}
			clock.Advance(delay)
		} else if ev.Status != "succeeded" {
			t.Fatalf("after success status = %q", ev.Status)
		}
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "delivery-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(attempts))
	}
	wantStatuses := []int{500, 500, 200}
	for i, want := range wantStatuses {
		if attempts[i].StatusCode == nil || *attempts[i].StatusCode != want {
			t.Errorf("attempt %d status = %#v, want %d", i+1, attempts[i].StatusCode, want)
		}
	}
	hook, err = store.GetWebhook(ctx, db.Q(), "hook")
	if err != nil {
		t.Fatal(err)
	}
	if hook.FirstFailureAt != nil {
		t.Errorf("first failure not cleared: %v", hook.FirstFailureAt)
	}
}

func TestWebhookRunCanceled(t *testing.T) {
	db, ctx := pf09TestDB(t)
	svc := New(db, pf09TestBuilder{}, nil, nil, Config{PollEvery: time.Millisecond})
	runCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := svc.Run(runCtx); err != nil {
		t.Fatalf("Run canceled context: %v", err)
	}
}

func TestWebhookRetryExhaustion(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{RetryBase: 10 * time.Second})
	for n := 1; n <= 11; n++ {
		count, err := svc.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("attempt %d: RunOnce = %d", n, count)
		}
		ev, err := store.GetWebhookEvent(ctx, db.Q(), "event")
		if err != nil {
			t.Fatal(err)
		}
		if ev.AttemptCount != n {
			t.Fatalf("attempt_count = %d, want %d", ev.AttemptCount, n)
		}
		if n < 11 {
			if ev.Status != "pending" {
				t.Fatalf("attempt %d status = %q", n, ev.Status)
			}
			clock.Set(ev.NextAttemptAt)
		} else if ev.Status != "failed" {
			t.Fatalf("final status = %q, want failed", ev.Status)
		}
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 11 {
		t.Fatalf("attempts = %d, want 11", len(attempts))
	}
}

func TestWebhookNon200AndRedirectFail(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusFound, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			db, ctx := pf09TestDB(t)
			now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
			clock := kftime.NewFakeClock(now)
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
			defer receiver.Close()
			pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
			if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			svc := New(db, pf09TestBuilder{}, clock, nil, Config{MaxAttempts: 1})
			if count, err := svc.RunOnce(ctx); err != nil || count != 1 {
				t.Fatalf("RunOnce = %d, %v", count, err)
			}
			ev, err := store.GetWebhookEvent(ctx, db.Q(), "event")
			if err != nil {
				t.Fatal(err)
			}
			if ev.Status != "failed" {
				t.Fatalf("status = %q, want failed", ev.Status)
			}
			attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
			if err != nil {
				t.Fatal(err)
			}
			if len(attempts) != 1 || attempts[0].Error == nil {
				t.Fatalf("attempt record = %#v, want failure", attempts)
			}
		})
	}
}

func TestWebhookTimeout(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	svc := New(db, pf09TestBuilder{}, clock, client, Config{MaxAttempts: 1})
	if count, err := svc.RunOnce(ctx); err != nil || count != 1 {
		t.Fatalf("RunOnce = %d, %v", count, err)
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Error == nil || *attempts[0].Error == "" {
		t.Fatalf("timeout attempt = %#v", attempts)
	}
}

func TestWebhookAutoDisable(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	first := now.Add(-7 * 24 * time.Hour)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true, FirstFailureAt: &first})
	for _, id := range []string{"event", "other"} {
		if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: id, WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{MaxAttempts: 1})
	if count, err := svc.RunOnce(ctx); err != nil || count != 1 {
		t.Fatalf("RunOnce = %d, %v", count, err)
	}
	hook, err := store.GetWebhook(ctx, db.Q(), "hook")
	if err != nil {
		t.Fatal(err)
	}
	if hook.Enabled || hook.DisabledAt == nil || !hook.DisabledAt.Equal(now) {
		t.Fatalf("webhook failure state = %#v", hook)
	}
	other, err := store.GetWebhookEvent(ctx, db.Q(), "other")
	if err != nil {
		t.Fatal(err)
	}
	if other.Status != "failed" {
		t.Fatalf("other pending event status = %q", other.Status)
	}
}

func TestWebhookResend(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	resendSignature := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resendSignature <- r.Header.Get("X-Kickflow-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	secret := "old"
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Secret: &secret, Enabled: false})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{"stable":true}`), Status: "succeeded", AttemptCount: 1, NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Q().ExecContext(ctx, "UPDATE webhooks SET secret=? WHERE id=?", "current", "hook"); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{})
	if err := svc.Resend(ctx, db.Q(), "event"); err != nil {
		t.Fatal(err)
	}
	if count, err := svc.RunOnce(ctx); err != nil || count != 1 {
		t.Fatalf("RunOnce = %d, %v", count, err)
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || !attempts[0].Manual {
		t.Fatalf("resend attempts = %#v, want one manual attempt", attempts)
	}
	if got, want := <-resendSignature, Sign([]byte("current"), []byte(`{"stable":true}`)); got != want {
		t.Errorf("resend signature = %q, want current-secret signature %q", got, want)
	}
	ev, err := store.GetWebhookEvent(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Status != "succeeded" || string(ev.Body) != `{"stable":true}` {
		t.Fatalf("resent event = %#v", ev)
	}
}

func TestWebhookResendFailureCannotUndoReenable(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	started, release := make(chan struct{}), make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()
	first := now.Add(-7 * 24 * time.Hour)
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: false, FirstFailureAt: &first})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{}`), Status: "failed", AttemptCount: 1, NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{MaxAttempts: 1})
	if err := svc.Resend(ctx, db.Q(), "event"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := svc.RunOnce(ctx); done <- err }()
	<-started
	if err := store.SetWebhookEnabled(ctx, db.Q(), "hook", true, now); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	hook, err := store.GetWebhook(ctx, db.Q(), "hook")
	if err != nil {
		t.Fatal(err)
	}
	if !hook.Enabled || hook.FirstFailureAt != nil || hook.DisabledAt != nil || hook.StateGeneration != 1 {
		t.Fatalf("stale delivery changed re-enabled webhook state: %#v", hook)
	}
}

func TestWebhookPersistAcrossRestart(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	delivered := make(chan struct{}, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { delivered <- struct{}{}; w.WriteHeader(http.StatusOK) }))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "persisted", WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{PollEvery: time.Millisecond})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- svc.Run(runCtx) }()
	timer := time.NewTimer(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()
	select {
	case <-delivered:
	case <-timer.C:
		cancel()
		t.Fatal("persisted event was not delivered after dispatcher start")
	}
	for {
		ev, err := store.GetWebhookEvent(ctx, db.Q(), "persisted")
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if ev.Status == "succeeded" {
			break
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			cancel()
			t.Fatal("persisted attempt was not committed")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

func TestWebhookRegistrationsDeliverConcurrently(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	slowStarted, releaseSlow := make(chan struct{}), make(chan struct{})
	fastDelivered := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(slowStarted)
		<-releaseSlow
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fastDelivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer fast.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "slow", Name: "slow", URL: slow.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "fast", Name: "fast", URL: fast.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "slow-event", "slow", now)
	pf09InsertPending(t, ctx, db, "fast-event", "fast", now)
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{Concurrency: 2})
	done := make(chan error, 1)
	go func() { _, err := svc.RunOnce(ctx); done <- err }()
	<-slowStarted
	select {
	case <-fastDelivered:
	case <-time.After(time.Second):
		close(releaseSlow)
		t.Fatal("fast registration waited for slow receiver")
	}
	close(releaseSlow)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWebhookAdmitsNewRegistrationWhileAnotherBatchIsInFlight(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	slowStarted, releaseSlow := make(chan struct{}), make(chan struct{})
	fastDelivered := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-slowStarted:
		default:
			close(slowStarted)
		}
		<-releaseSlow
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fastDelivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer fast.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "slow", Name: "slow", URL: slow.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "fast", Name: "fast", URL: fast.URL, Enabled: true})
	backlog := pf09DueBatchLimit + 5
	if _, err := db.Q().ExecContext(ctx, `WITH RECURSIVE seq(i) AS (
		SELECT 0 UNION ALL SELECT i+1 FROM seq WHERE i < ?
	) INSERT INTO webhook_events(id,webhook_id,event_type,body,status,next_attempt_at,created_at,updated_at)
	SELECT printf('slow-%04d',i),'slow','ticket_created',x'7b7d','pending',?,?,? FROM seq`, backlog-1, store.Millis(now), store.Millis(now), store.Millis(now)); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{Concurrency: 2, PollEvery: time.Millisecond})
	done := make(chan error, 1)
	runCtx, cancel := context.WithCancel(ctx)
	go func() { _, err := svc.RunOnce(runCtx); done <- err }()
	select {
	case <-slowStarted:
	case err := <-done:
		cancel()
		t.Fatalf("dispatcher exited before slow delivery started: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("slow delivery did not start")
	}
	pf09InsertPending(t, ctx, db, "fast-event", "fast", now)
	svc.Notify()
	select {
	case <-fastDelivered:
	case <-time.After(10 * time.Second):
		cancel()
		close(releaseSlow)
		t.Fatal("new healthy registration was not admitted while the earlier batch was in flight")
	}
	cancel()
	close(releaseSlow)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestWebhookDispatcherRotatesRegistrationsWithBacklogs(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	startedA, startedB := make(chan struct{}), make(chan struct{})
	releaseA, releaseB := make(chan struct{}), make(chan struct{})
	aBacklogDelivered, healthyDelivered := make(chan struct{}, 1), make(chan struct{}, 1)
	var aRequests atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if aRequests.Add(1) == 1 {
			close(startedA)
			<-releaseA
		} else {
			select {
			case aBacklogDelivered <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(startedB)
		<-releaseB
		w.WriteHeader(http.StatusOK)
	}))
	defer b.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		healthyDelivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "a", Name: "a", URL: a.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "b", Name: "b", URL: b.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "healthy", Name: "healthy", URL: healthy.URL, Enabled: true})
	for i := 0; i < 4; i++ {
		pf09InsertPending(t, ctx, db, fmt.Sprintf("a-%d", i), "a", now)
		pf09InsertPending(t, ctx, db, fmt.Sprintf("b-%d", i), "b", now)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{Concurrency: 2, PollEvery: time.Millisecond})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := svc.RunOnce(runCtx); done <- err }()
	select {
	case <-startedA:
	case err := <-done:
		t.Fatalf("dispatcher exited before registration a started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("registration a did not start")
	}
	select {
	case <-startedB:
	case err := <-done:
		t.Fatalf("dispatcher exited before registration b started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("registration b did not start")
	}
	pf09InsertPending(t, ctx, db, "healthy-event", "healthy", now)
	svc.Notify()
	close(releaseA)
	select {
	case <-healthyDelivered:
	case <-aBacklogDelivered:
		cancel()
		close(releaseB)
		<-done
		t.Fatal("registration a received another backlog event before the healthy registration")
	case <-time.After(2 * time.Second):
		cancel()
		close(releaseB)
		<-done
		t.Fatal("healthy registration was starved by older backlogs")
	}
	cancel()
	close(releaseB)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce after cancellation = %v", err)
	}
}

func TestWebhookAdmitsDueRetryWhileAnotherRegistrationIsBusy(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	slowStarted, releaseSlow := make(chan struct{}), make(chan struct{})
	var retryRequests atomic.Int32
	retryStarted := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(slowStarted)
		<-releaseSlow
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	retry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if retryRequests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		retryStarted <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer retry.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "slow", Name: "slow", URL: slow.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "retry", Name: "retry", URL: retry.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "slow-event", "slow", now)
	pf09InsertPending(t, ctx, db, "retry-event", "retry", now)
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{Concurrency: 2, PollEvery: time.Millisecond, RetryBase: 10 * time.Second})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := svc.RunOnce(runCtx); done <- err }()
	select {
	case <-slowStarted:
	case err := <-done:
		t.Fatalf("dispatcher exited before slow registration started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("slow registration did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		ev, err := store.GetWebhookEvent(ctx, db.Q(), "retry-event")
		if err != nil {
			t.Fatal(err)
		}
		if ev.AttemptCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial retry attempt was not recorded")
		}
		time.Sleep(time.Millisecond)
	}
	clock.Advance(Backoff(10*time.Second, 1))
	svc.Notify()
	select {
	case <-retryStarted:
	case <-time.After(2 * time.Second):
		cancel()
		close(releaseSlow)
		<-done
		t.Fatal("due retry was not admitted while another registration remained busy")
	}
	cancel()
	close(releaseSlow)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce after cancellation = %v", err)
	}
}

func TestWebhookAdmitsResendWhileAnotherRegistrationIsBusy(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	slowStarted, releaseSlow := make(chan struct{}), make(chan struct{})
	firstResendStarted, releaseFirstResend := make(chan struct{}), make(chan struct{})
	secondResendStarted := make(chan struct{}, 1)
	var resendRequests atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(slowStarted)
		<-releaseSlow
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	resend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if resendRequests.Add(1) == 1 {
			close(firstResendStarted)
			<-releaseFirstResend
		} else {
			secondResendStarted <- struct{}{}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer resend.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "slow", Name: "slow", URL: slow.URL, Enabled: true})
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "resend", Name: "resend", URL: resend.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "slow-event", "slow", now)
	pf09InsertPending(t, ctx, db, "resend-event", "resend", now)
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{Concurrency: 2, PollEvery: time.Millisecond})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := svc.RunOnce(runCtx); done <- err }()
	select {
	case <-slowStarted:
	case err := <-done:
		t.Fatalf("dispatcher exited before slow registration started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("slow registration did not start")
	}
	select {
	case <-firstResendStarted:
	case err := <-done:
		t.Fatalf("dispatcher exited before resend registration started: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("initial delivery did not start")
	}
	if err := svc.Resend(ctx, db.Q(), "resend-event"); err != nil {
		t.Fatal(err)
	}
	close(releaseFirstResend)
	select {
	case <-secondResendStarted:
	case <-time.After(2 * time.Second):
		cancel()
		close(releaseSlow)
		<-done
		t.Fatal("accepted resend was not admitted while another registration remained busy")
	}
	cancel()
	close(releaseSlow)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce after cancellation = %v", err)
	}
}

func TestWebhookAutoDisablePreservesAcceptedResend(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()
	firstFailure := now.Add(-7 * 24 * time.Hour)
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true, FirstFailureAt: &firstFailure})
	pf09InsertPending(t, ctx, db, "event", "hook", now)
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{MaxAttempts: 1})
	done := make(chan error, 1)
	go func() {
		count, err := svc.RunOnce(ctx)
		if err == nil && count != 2 {
			err = fmt.Errorf("RunOnce attempted %d deliveries, want 2", count)
		}
		done <- err
	}()
	<-started
	if err := svc.Resend(ctx, db.Q(), "event"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	hook, err := store.GetWebhook(ctx, db.Q(), "hook")
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.GetWebhookEvent(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if hook.Enabled || event.Status != model.WebhookStatusFailed || event.AttemptCount != 2 || event.ManualResendGeneration != 1 || event.ManualResendCompletedGeneration != 1 {
		t.Fatalf("accepted resend was not attempted after auto-disable: webhook=%#v event=%#v", hook, event)
	}
	due, err := store.DueWebhookEvents(ctx, db.Q(), now, 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("completed resend remained due = %#v, %v; want no pending event", due, err)
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[1].AttemptNo != 2 || !attempts[1].Manual {
		t.Fatalf("attempts = %#v, want original and manual resend", attempts)
	}
}

func TestWebhookResendDuringOutstandingAttemptIsPreserved(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	pf09InsertPending(t, ctx, db, "event", "hook", now)
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{})
	done := make(chan error, 1)
	go func() {
		count, err := svc.RunOnce(ctx)
		if err == nil && count != 2 {
			err = fmt.Errorf("RunOnce attempted %d deliveries, want 2", count)
		}
		done <- err
	}()
	<-started
	if err := svc.Resend(ctx, db.Q(), "event"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	event, err := store.GetWebhookEvent(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if event.Status != model.WebhookStatusSucceeded || event.AttemptCount != 2 || event.ManualResendGeneration != 1 || event.ManualResendCompletedGeneration != 1 {
		t.Fatalf("outstanding attempt did not admit accepted resend: %#v", event)
	}
	attempts, err := store.ListWebhookAttempts(ctx, db.Q(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[1].AttemptNo != 2 || !attempts[1].Manual {
		t.Fatalf("attempts = %#v, want original and manual resend", attempts)
	}
}

func TestWebhookNoSecretOmitsSignature(t *testing.T) {
	db, ctx := pf09TestDB(t)
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	clock := kftime.NewFakeClock(now)
	noSignature := make(chan bool, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		noSignature <- r.Header.Get("X-Kickflow-Signature") == ""
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	pf09InsertWebhook(t, ctx, db, model.Webhook{ID: "hook", Name: "hook", URL: receiver.URL, Enabled: true})
	if err := store.InsertWebhookEvent(ctx, db.Q(), model.WebhookDelivery{ID: "event", WebhookID: "hook", Body: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(db, pf09TestBuilder{}, clock, nil, Config{})
	if count, err := svc.RunOnce(ctx); err != nil || count != 1 {
		t.Fatalf("RunOnce = %d, %v", count, err)
	}
	if !<-noSignature {
		t.Error("unsigned webhook request included X-Kickflow-Signature")
	}
}
