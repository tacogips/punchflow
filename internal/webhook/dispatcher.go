package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"punchflow/internal/model"
	"punchflow/internal/store"
)

const pf09ResponseLimit = 64 * 1024

// Run starts the delivery loop. It scans persisted work at startup and exits on cancellation.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollEvery)
	defer ticker.Stop()
	for {
		if _, err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.notify:
		case <-ticker.C:
		}
	}
}

// RunOnce synchronously attempts all currently due deliveries, returning the number attempted.
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	now := s.clock.Now()
	events, err := store.DueWebhookEvents(ctx, s.db.Q(), now, 1000)
	if err != nil {
		return 0, err
	}
	attempts := 0
	for _, ev := range events {
		if err := ctx.Err(); err != nil {
			return attempts, err
		}
		hook, err := store.GetWebhook(ctx, s.db.Q(), ev.WebhookID)
		if err != nil {
			return attempts, err
		}
		manual := s.pf09TakeManual(ev.ID)
		if !hook.Enabled && !manual {
			continue
		}
		if err := pf09WebhookURL(hook.URL); err != nil {
			if err := s.pf09Record(ctx, ev, hook, manual, nil, nil, err.Error(), 0, now); err != nil {
				return attempts, err
			}
			attempts++
			continue
		}
		if err := s.pf09Deliver(ctx, ev, hook, manual); err != nil {
			return attempts, err
		}
		attempts++
	}
	return attempts, nil
}

func (s *Service) pf09TakeManual(id string) bool {
	s.manualMu.Lock()
	defer s.manualMu.Unlock()
	manual := s.manual[id]
	delete(s.manual, id)
	return manual
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
	return s.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := store.InsertWebhookAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		if err := store.MarkEventAttempt(ctx, tx, ev.ID, count, next, eventStatus, status, now); err != nil {
			return err
		}
		if err := store.SetWebhookFailureState(ctx, tx, hook.ID, first, disable, now); err != nil {
			return err
		}
		return nil
	})
}
