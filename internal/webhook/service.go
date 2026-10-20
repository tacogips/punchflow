package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// PayloadBuilder builds the immutable body persisted in the webhook outbox.
type PayloadBuilder interface {
	Build(context.Context, store.Querier, model.WebhookEvent) ([]byte, error)
}

// Config controls retry and dispatcher behavior.
type Config struct {
	RetryBase   time.Duration
	Timeout     time.Duration
	MaxAttempts int
	Concurrency int
	PollEvery   time.Duration
}

// Service owns webhook enqueue and delivery behavior.
type Service struct {
	db       *store.DB
	builder  PayloadBuilder
	clock    kftime.Clock
	client   *http.Client
	cfg      Config
	notify   chan struct{}
	manualMu sync.Mutex
	manual   map[string]bool
}

// New creates a service with safe local-delivery defaults.
func New(db *store.DB, b PayloadBuilder, clock kftime.Clock, client *http.Client, cfg Config) *Service {
	if cfg.RetryBase <= 0 {
		cfg.RetryBase = 10 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 11
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = time.Second
	}
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil}}
	}
	return &Service{db: db, builder: b, clock: clock, client: client, cfg: cfg, notify: make(chan struct{}, 1), manual: make(map[string]bool)}
}

// Enqueue fans out non-ping events to enabled registrations matching event and workflow filters.
func (s *Service) Enqueue(ctx context.Context, q store.Querier, evs []model.WebhookEvent) error {
	for _, ev := range evs {
		if ev.Type == model.WebhookEventPing {
			continue
		}
		workflowID := ""
		if ev.TicketID != "" {
			if err := q.QueryRowContext(ctx, "SELECT workflow_id FROM tickets WHERE id=?", ev.TicketID).Scan(&workflowID); err != nil {
				return fmt.Errorf("webhook: find ticket workflow: %w", err)
			}
		}
		hooks, err := store.EnabledWebhooks(ctx, q)
		if err != nil {
			return err
		}
		body, err := s.builder.Build(ctx, q, ev)
		if err != nil {
			return fmt.Errorf("webhook: build %s payload: %w", ev.Type, err)
		}
		now := s.clock.Now()
		for _, hook := range hooks {
			if !pf09Matches(ev.Type, workflowID, hook.EventTypes, hook.WorkflowIDs) {
				continue
			}
			if err := store.InsertWebhookEvent(ctx, q, model.WebhookDelivery{ID: store.NewID(), WebhookID: hook.ID, EventType: ev.Type, TicketID: pf09StringPtr(ev.TicketID), CommentID: pf09StringPtr(ev.CommentID), ActorUserID: pf09StringPtr(ev.ActorUserID), Body: append([]byte(nil), body...), Status: model.WebhookStatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		}
	}
	return nil
}

// EnqueueTo targets explicit webhook IDs and deliberately ignores filters.
func (s *Service) EnqueueTo(ctx context.Context, q store.Querier, ev model.WebhookEvent, webhookIDs []string) ([]string, error) {
	body, err := s.builder.Build(ctx, q, ev)
	if err != nil {
		return nil, fmt.Errorf("webhook: build %s payload: %w", ev.Type, err)
	}
	now := s.clock.Now()
	ids := make([]string, 0, len(webhookIDs))
	for _, id := range webhookIDs {
		if _, err := store.GetWebhook(ctx, q, id); err != nil {
			return nil, err
		}
		deliveryID := store.NewID()
		if err := store.InsertWebhookEvent(ctx, q, model.WebhookDelivery{ID: deliveryID, WebhookID: id, EventType: ev.Type, TicketID: pf09StringPtr(ev.TicketID), CommentID: pf09StringPtr(ev.CommentID), ActorUserID: pf09StringPtr(ev.ActorUserID), Body: append([]byte(nil), body...), Status: model.WebhookStatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
			return nil, err
		}
		ids = append(ids, deliveryID)
	}
	return ids, nil
}

// Notify wakes the run loop without blocking.
func (s *Service) Notify() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Resend queues a delivery again, preserving its ID and body.
func (s *Service) Resend(ctx context.Context, q store.Querier, eventID string) error {
	ev, err := store.GetWebhookEvent(ctx, q, eventID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	if err := store.MarkEventAttempt(ctx, q, eventID, ev.AttemptCount, now, model.WebhookStatusPending, nil, now); err != nil {
		return err
	}
	s.manualMu.Lock()
	s.manual[eventID] = true
	s.manualMu.Unlock()
	s.Notify()
	return nil
}

func pf09Matches(eventType, workflowID string, eventTypes, workflowIDs []string) bool {
	return (len(eventTypes) == 0 || pf09Contains(eventTypes, eventType)) && (len(workflowIDs) == 0 || workflowID != "" && pf09Contains(workflowIDs, workflowID))
}
func pf09Contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func pf09StringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func pf09WebhookURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("unsupported webhook URL scheme")
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("webhook URL host is empty")
	}
	return nil
}
