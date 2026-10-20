# PF-09 Webhook Delivery Implementation Plan

**Status**: Ready
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-09
**Design Reference**: design-docs/specs/design-webhooks.md (all sections); design-docs/specs/notes.md INF-08, INF-09, INF-10, INF-33
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Implement the webhook outbox and delivery machinery: fan-out of events to webhooks inside the caller's transaction, HMAC signing, the dispatcher with 10 s timeout and 1 + 10 attempts with exponential backoff, attempt persistence, auto-disable, manual resend and targeted enqueue (ping/fire). Payload bytes come from a `PayloadBuilder`; the real builder is PF-15, and this plan ships a stub `payload.go` with the pinned constructor.

## Non-goals

- No payload content (PF-15 rewrites `payload.go`). No admin HTTP endpoints (PF-24). No server wiring (PF-16).

## Files

`internal/webhook/service.go` (Service, Enqueue, EnqueueTo, Notify, Resend), `dispatcher.go` (Run, RunOnce), `sign.go`, `backoff.go`, `payload.go` (stub), tests `sign_test.go`, `service_test.go`, `dispatcher_test.go`; `internal/store/webhooks.go` (CRUD and queries for `webhooks`, `webhook_events`, `webhook_attempts`).

## Path Declarations

### writePaths
- `impl-plans/active/pf-09-webhook-delivery.md`
- `internal/webhook/service.go`
- `internal/webhook/dispatcher.go`
- `internal/webhook/sign.go`
- `internal/webhook/backoff.go`
- `internal/webhook/payload.go`
- `internal/webhook/sign_test.go`
- `internal/webhook/service_test.go`
- `internal/webhook/dispatcher_test.go`
- `internal/store/webhooks.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

```go
type PayloadBuilder interface {
    Build(ctx context.Context, q store.Querier, ev model.WebhookEvent) ([]byte, error)
}
type Config struct {
    RetryBase   time.Duration // default 10s
    Timeout     time.Duration // default 10s
    MaxAttempts int           // default 11 (1 + 10 retries)
    Concurrency int           // default 4
    PollEvery   time.Duration // default 1s
}
type Service struct{ /* db, builder, clock, client, cfg, notify chan */ }
func New(db *store.DB, b PayloadBuilder, clock kftime.Clock, client *http.Client, cfg Config) *Service // client nil -> &http.Client{Timeout: cfg.Timeout, CheckRedirect: return http.ErrUseLastResponse, Transport with Proxy nil}
func (s *Service) Enqueue(ctx context.Context, q store.Querier, evs []model.WebhookEvent) error
func (s *Service) EnqueueTo(ctx context.Context, q store.Querier, ev model.WebhookEvent, webhookIDs []string) ([]string, error) // ignores filters; returns event ids
func (s *Service) Notify()                                   // non-blocking
func (s *Service) Run(ctx context.Context) error             // loop until ctx done; waits for in-flight attempts
func (s *Service) RunOnce(ctx context.Context) (int, error)  // process due events now, synchronously; returns attempts made
func (s *Service) Resend(ctx context.Context, q store.Querier, eventID string) error
func Sign(secret, body []byte) string                        // "sha256=" + lowercase hex HMAC-SHA256
func Verify(secret, body []byte, header string) bool         // constant-time; for tests and README
func Backoff(base time.Duration, retry int) time.Duration    // base*2^(retry-1), cap 1h; retry >= 1

// payload.go stub (PF-15 replaces the body, keeps these two signatures)
type Payloads struct{ /* unexported */ }
func NewPayloads() *Payloads                           // returns a builder; stub Build returns an error "payloads not implemented"
func (p *Payloads) Build(ctx context.Context, q store.Querier, ev model.WebhookEvent) ([]byte, error)
```

Store functions in `internal/store/webhooks.go` (Querier-based): `ListWebhooks`, `GetWebhook`, `InsertWebhook`, `UpdateWebhook`, `DeleteWebhook`, `EnabledWebhooks`, `InsertWebhookEvent`, `DueWebhookEvents(now, limit)`, `GetWebhookEvent`, `ListWebhookEvents(webhookID, offset, limit) (rows, total)`, `MarkEventAttempt`, `InsertWebhookAttempt`, `ListWebhookAttempts(eventID)`, `SetWebhookFailureState`. PF-24 calls these.

## Behavior

- `Enqueue`: for each event, select enabled webhooks whose `event_types` is empty or contains the type, and (for ticket/comment events) whose `workflow_ids` is empty or contains the ticket's workflow (look up `tickets.workflow_id`). `ping` events are never fanned out by `Enqueue`; use `EnqueueTo`. Build the body ONCE per event (`builder.Build`) and store the same bytes in each row. Each row gets a new UUID (the delivery id), `status pending`, `attempt_count 0`, `next_attempt_at = now`.
- Attempt: headers `Content-Type: application/json`, `User-Agent: kickflow-Hookshot/v1`, `X-Kickflow-Delivery: <event id>`, `X-Kickflow-Signature: Sign(secret, body)` only when the secret is non-empty. Success iff status code == 200. Record an attempt row (headers JSON, status, first 64 KiB of the response, error text, duration, `manual` flag). On success: event `succeeded`; webhook `first_failure_at = NULL`. On failure: `attempt_count++`; if `< MaxAttempts`, `next_attempt_at = now + Backoff(base, attempt_count)`, else `failed`; webhook `first_failure_at` set if NULL; if `now - first_failure_at >= 7*24h`, disable the webhook (`enabled=0`, `disabled_at=now`) and mark its pending events `failed` (INF-10).
- Dispatcher: due = `status='pending' AND next_attempt_at <= now` ordered by `next_attempt_at, rowid`; at most one in-flight attempt per webhook and `Concurrency` overall. Events of a disabled webhook are not attempted unless the row's next attempt is manual (resend).
- `Resend`: status -> `pending`, `next_attempt_at = now`, flag the next attempt `manual`; works on succeeded/failed rows and disabled webhooks; signature uses the current secret (INF-09).
- No DB transaction is open during the HTTP call: read the event and webhook, release, POST, then write the result in a short `WithTx`.

## Key Points and Pitfalls

- Use the clock for every `now`. Tests use `kftime.FakeClock` plus `RunOnce`, never `time.Sleep` for backoff. Backoff is computed from the clock, so a test advances the fake clock past `next_attempt_at` and calls `RunOnce` again.
- 3xx counts as failure: the client must not follow redirects.
- `Run` must return when ctx is cancelled and not leak goroutines (test with a cancelled context and `-race`).
- `Notify` must never block (buffered channel of size 1 with a select default).

## Tests (input -> expected)

Use an `httptest.Server` receiver and a fake builder returning `{"eventType":"ticket_created","x":"<a&b>"}` encoded with `json.Marshal`.
- `Sign([]byte("secret"), body)` equals a precomputed constant (compute once with `openssl dgst -sha256 -hmac secret`, record in the test); the body contains `<a&b>`; `Verify` true, tampered body false
- headers present; no secret -> no signature header
- filters: event type not selected -> no row; other workflow -> no row; disabled webhook -> no row; `EnqueueTo` ignores filters
- receiver 500,500,200 -> 3 attempts, same `X-Kickflow-Delivery`, status succeeded, `first_failure_at` cleared
- always 500 -> 11 attempts then `failed`; delays equal `Backoff(base, n)`
- 201 / 302 / 404 -> failure each
- receiver sleeping beyond a 100 ms test timeout -> failure with error text
- failure with `first_failure_at` = now-7d -> webhook disabled, other pending events failed
- `Resend` on succeeded -> new manual attempt, same id and body bytes
- pending row inserted before `Run` starts -> delivered after start (persisted outbox)
- `Backoff(10s, 1)` = 10s, `(10s, 10)` = 1h cap

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf09-build.log
go vet ./internal/webhook/... ./internal/store/... 2>&1 | tee /tmp/pf09-vet.log
go test -race ./internal/webhook/... 2>&1 | tee /tmp/pf09-test.log
gofmt -l internal/webhook internal/store/webhooks.go   # prints nothing
```

## Done Criteria

- [ ] Contract symbols exist (including the `NewPayloads`/`Build` stub); all tests pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
