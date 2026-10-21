package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

const (
	pf09ResponseLimit = 64 * 1024
	pf09DueBatchLimit = 1000
)

// errDeliveryGone reports that the webhook or delivery row disappeared (for
// example an administrator deleted the registration) before the attempt could be
// recorded. It cancels that single delivery and is never fatal to the dispatcher.
var errDeliveryGone = errors.New("webhook: delivery or registration no longer exists")

// Run starts the delivery loop. It scans persisted work at startup and exits on cancellation.
//
// Store errors raised while processing deliveries are logged and retried on the
// next poll; they never stop the loop. Only context cancellation ends Run.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollEvery)
	defer ticker.Stop()
	for {
		notify := s.notify
		if _, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Error("webhook dispatcher: delivery pass failed; retrying on next poll", "error", err)
			// Back off: ignore wake-up notifications and wait for the poll tick.
			notify = nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-notify:
		case <-ticker.C:
		}
	}
}

// RunOnce synchronously attempts due deliveries, admitting newly queued work while
// receiver requests are still in flight. Only one attempt per registration runs
// at a time; independent registrations share the configured worker bound.
//
// A webhook or delivery deleted concurrently cancels only that delivery. Other
// per-delivery store errors are collected and returned after the remaining
// deliveries were processed. Context cancellation returns immediately.
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	type deliveryResult struct {
		event     model.WebhookDelivery
		attempted int
		err       error
	}
	workers := s.cfg.Concurrency
	queues := make(map[string][]model.WebhookDelivery)
	readyOrder := make([]string, 0)
	ready := make(map[string]bool)
	known := make(map[string]bool)
	failedThisPass := make(map[string]bool)
	active := make(map[string]bool)
	results := make(chan deliveryResult, workers)
	attempts := 0
	var failures []error
	var poll *time.Ticker
	if s.cfg.PollEvery > 0 {
		poll = time.NewTicker(s.cfg.PollEvery)
		defer poll.Stop()
	}
	var pollC <-chan time.Time
	if poll != nil {
		pollC = poll.C
	}

	for {
		if ctx.Err() != nil {
			return attempts, ctx.Err()
		}
		// Refill due work on every scheduler pass. Known IDs remain excluded only
		// while queued or in flight, so a new retry deadline or resend generation
		// can be admitted as soon as the previous attempt completes. Processing
		// errors are held until the next RunOnce pass because no attempt may have
		// been persisted to make the row's next-attempt state advance.
		now := s.clock.Now()
		events, err := store.DueWebhookEvents(ctx, s.db.Q(), now, pf09DueBatchLimit)
		if err != nil {
			return attempts, err
		}
		for _, ev := range events {
			if known[ev.ID] || failedThisPass[ev.ID] {
				continue
			}
			queues[ev.WebhookID] = append(queues[ev.WebhookID], ev)
			known[ev.ID] = true
			if !active[ev.WebhookID] && !ready[ev.WebhookID] {
				readyOrder = append(readyOrder, ev.WebhookID)
				ready[ev.WebhookID] = true
			}
		}

		// Admit ready registrations in round-robin order. A registration is
		// returned to the tail only after its active attempt completes, so idle
		// registrations with due work get a turn before any backlog repeats.
		for len(active) < workers && len(readyOrder) > 0 {
			hookID := readyOrder[0]
			readyOrder = readyOrder[1:]
			delete(ready, hookID)
			if active[hookID] || len(queues[hookID]) == 0 {
				continue
			}
			ev := queues[hookID][0]
			queues[hookID] = queues[hookID][1:]
			active[hookID] = true
			go func(ev model.WebhookDelivery) {
				attempted, err := s.pf09Process(ctx, ev, s.clock.Now())
				if err != nil {
					err = fmt.Errorf("webhook delivery %s: %w", ev.ID, err)
				}
				result := deliveryResult{event: ev, err: err}
				if attempted {
					result.attempted = 1
				}
				select {
				case results <- result:
				case <-ctx.Done():
				}
			}(ev)
		}

		if len(active) == 0 {
			queued := false
			for _, events := range queues {
				queued = queued || len(events) > 0
			}
			if !queued {
				return attempts, errors.Join(failures...)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return attempts, ctx.Err()
		case <-s.notify:
		case <-pollC:
		case result := <-results:
			delete(active, result.event.WebhookID)
			delete(known, result.event.ID)
			if result.err != nil {
				failedThisPass[result.event.ID] = true
			}
			if len(queues[result.event.WebhookID]) > 0 && !ready[result.event.WebhookID] {
				readyOrder = append(readyOrder, result.event.WebhookID)
				ready[result.event.WebhookID] = true
			}
			attempts += result.attempted
			if result.err != nil && !errors.Is(result.err, context.Canceled) {
				failures = append(failures, result.err)
			}
		}
	}
}

// pf09Process delivers one due row. It reports whether an attempt was recorded.
func (s *Service) pf09Process(ctx context.Context, ev model.WebhookDelivery, now time.Time) (bool, error) {
	hook, err := store.GetWebhook(ctx, s.db.Q(), ev.WebhookID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Deleted between batch selection and lookup: cancel this delivery.
			return false, nil
		}
		return false, err
	}
	manual := ev.ManualResendGeneration > ev.ManualResendCompletedGeneration
	if !hook.Enabled && !manual && !ev.Diagnostic {
		// Disabled after batch selection.
		return false, nil
	}
	if err := pf09WebhookURL(hook.URL); err != nil {
		err = s.pf09Record(ctx, ev, hook, manual, nil, nil, err.Error(), 0, now)
		return pf09Recorded(err)
	}
	return pf09Recorded(s.pf09Deliver(ctx, ev, hook, manual))
}

// pf09Recorded maps a record result to (attempted, error), treating a concurrent
// deletion as a cancelled delivery rather than a failure.
func pf09Recorded(err error) (bool, error) {
	if errors.Is(err, errDeliveryGone) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) pf09Deliver(ctx context.Context, ev model.WebhookDelivery, hook model.Webhook, manual bool) error {
	started := s.clock.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(ev.Body))
	if err != nil {
		return s.pf09Record(ctx, ev, hook, manual, nil, nil, err.Error(), 0, started)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "kickflow-Hookshot/v1")
	request.Header.Set("X-Kickflow-Delivery", ev.ID)
	if hook.Secret != nil && *hook.Secret != "" {
		request.Header.Set("X-Kickflow-Signature", Sign([]byte(*hook.Secret), ev.Body))
	}
	headers := make(map[string]string, len(request.Header))
	for key, values := range request.Header {
		if len(values) > 0 {
			headers[http.CanonicalHeaderKey(key)] = values[0]
		}
	}
	response, err := s.client.Do(request)
	if err != nil {
		return s.pf09Record(ctx, ev, hook, manual, headers, nil, err.Error(), s.clock.Now().Sub(started), started)
	}
	defer func() { _ = response.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, pf09ResponseLimit))
	if readErr != nil {
		err = readErr
	}
	var status *int
	if response.StatusCode != 0 {
		code := response.StatusCode
		status = &code
	}
	message := ""
	if err != nil {
		message = err.Error()
	} else if response.StatusCode != http.StatusOK {
		message = fmt.Sprintf("unexpected webhook status %d", response.StatusCode)
	}
	return s.pf09Record(ctx, ev, hook, manual, headers, status, message, s.clock.Now().Sub(started), started, body)
}

func (s *Service) pf09Record(ctx context.Context, ev model.WebhookDelivery, hook model.Webhook, manual bool, headers map[string]string, status *int, message string, elapsed time.Duration, started time.Time, responseBody ...[]byte) error {
	now := s.clock.Now()
	count := ev.AttemptCount + 1
	success := status != nil && *status == http.StatusOK && message == ""
	eventStatus := model.WebhookStatusFailed
	next := now
	first := hook.FirstFailureAt
	disable := false
	if success {
		eventStatus = model.WebhookStatusSucceeded
		first = nil
	} else {
		if first == nil {
			v := now
			first = &v
		}
		if count < s.cfg.MaxAttempts {
			eventStatus = model.WebhookStatusPending
			next = now.Add(Backoff(s.cfg.RetryBase, count))
		}
		disable = now.Sub(*first) >= 7*24*time.Hour
	}
	var body []byte
	if len(responseBody) > 0 {
		body = responseBody[0]
	}
	attempt := model.WebhookAttempt{ID: store.NewID(), EventID: ev.ID, AttemptNo: count, StartedAt: started, DurationMS: elapsed.Milliseconds(), RequestHeaders: headers, StatusCode: status, ResponseBody: body, Manual: manual, CreatedAt: now}
	if message != "" {
		attempt.Error = &message
	}
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		// Update the event first: a missing row means the webhook (and its
		// events) was deleted while the request was in flight.
		if err := store.RecordWebhookAttempt(ctx, tx, ev, manual, count, next, eventStatus, status, now); err != nil {
			return err
		}
		if err := store.InsertWebhookAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		return store.SetWebhookFailureStateAtGeneration(ctx, tx, hook.ID, first, disable, now, hook.StateGeneration)
	})
	if errors.Is(err, store.ErrNotFound) {
		return errDeliveryGone
	}
	return err
}
