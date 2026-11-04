# PF-09 Webhook Delivery Implementation Plan

**Status**: Completed
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

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 2. **Depends on**: PF-02 (phase 1 renames `model.WebhookStatusDelivered` to `WebhookStatusSucceeded = "succeeded"` and fixes `WebhookEventTypes` to the 13 kickflow events).

### Why

The session-460 implementation exists. It needs re-verification under the session-462 evidence rule, plus a check that it is consistent with the corrected PF-02 enums.

### Tasks

1. `rg -n '"delivered"|"succeeded"|WebhookStatus' internal/webhook internal/store/webhooks.go`. Delivery status values written or compared must be exactly `pending`, `succeeded`, `failed` (`design-webhooks.md` data model). Where PF-09 code uses a string literal for these, prefer the `model.WebhookStatus*` constants. That edit is optional, but if a literal differs from the design, it must be fixed.
2. Check the test expectations against INF-33 HTML escaping. The fake builder value `<a&b>`, encoded with `json.Marshal`, yields the escaped sequence `\u003ca\u0026b\u003e` (backslash-u escapes for `<`, `&` and `>`; the Go interpreted literal is `"\\u003ca\\u0026b\\u003e"`). If `sign_test.go` asserts the raw string `"<a&b>"` inside `json.Marshal` output, or a signature computed over unescaped bytes, correct the test to assert the escaped form, and assert that the raw `"<a&b>"` is absent. Do not change `Sign` and do not disable HTML escaping. If the test signs a hand-written byte string that is not produced by `json.Marshal`, leave it as it is and record that in the progress log.
3. Do not touch `internal/webhook/payload.go` (PF-15 replaces it in phase 3).

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf09-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/webhook/... 2>&1 | tee /tmp/pf09-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/webhook/... 2>&1 | tee /tmp/pf09-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf09-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf09-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf09-s462-test.log)"
gofmt -l internal/webhook internal/store/webhooks.go   # prints nothing
```

Evidence: exit 0; FAIL=0; SKIP=0; PASS >= 15 (12 top-level tests + 3 subtests in session 460).

### Done Criteria (session 462)

- [x] Delivery statuses are exactly `pending|succeeded|failed`; uncached `-race -v` tests pass with counts recorded; build/vet exit 0; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-09 implementation
**Intent snapshot**: Created `internal/store/webhooks.go` for webhook CRUD/outbox/attempt operations; created `internal/webhook/service.go`, `dispatcher.go`, `sign.go`, `backoff.go`, and the PF-15 seam `payload.go`; added `sign_test.go`, `service_test.go`, and `dispatcher_test.go`. Pre-edit hashes and per-edit intentions are preserved under `tmp/punchflow-kickflow-dummy-s460/PF-09/attempt-1/`.
**Tasks Completed**: Implemented filter-based transactional enqueue and targeted enqueue, HMAC signing and verification, capped retry calculation, persisted outbox polling, HTTP delivery with no redirects by default, attempt records, retry exhaustion, success reset, 7-day auto-disable, manual resend, and payload builder stub. Added tests for signature vector, missing-secret header omission, event/workflow/disabled filters, 500/500/200 retry and delay progression, 11-attempt exhaustion, 201/302/404 failure, timeout, 7-day auto-disable, resend with current-secret signature, persisted startup, cancellation, and backoff.
**Verification**:
- `go build ./...` exited 0; complete log `/tmp/pf09-build-final.log`.
- `go vet ./internal/webhook/... ./internal/store/...` exited 0; complete log `/tmp/pf09-vet.log`.
- `go test -race ./internal/webhook/...` exited 0; complete log `/tmp/pf09-test-final.log`; all package tests passed.
- `go test -race -count=1 -v ./internal/webhook/...` exited 0; 12 top-level tests and 3 non-200 subtests passed; complete log `/tmp/pf09-test-verbose.log`.
- `gofmt -l internal/webhook internal/store/webhooks.go` exited 0 and printed no files; complete log `/tmp/pf09-gofmt.log`.
- Earlier verification attempt: the initial `go test -race ./internal/webhook/...` exited 1 on transient shared-tree compile errors in `internal/store/read_masters.go` and `internal/store/read_proxies.go`, outside PF-09 paths; complete log `/tmp/pf09-test.log`. The initial `go build ./...` exited 0 (`/tmp/pf09-build.log`). After the sibling compile errors resolved, an intermediate race run exposed a test cancellation omission in `TestWebhookPersistAcrossRestart`; the test was corrected and the final uncached verbose race run passed.
**Reconciliation requests**: None. Formal independent review and serial combined-tree integration remain downstream workflow steps.

### Session: 2026-10-20 PF-09 session 462 continuation
**Intent snapshots**: Per-edit intentions and pre-edit hashes are recorded in `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/edit-intent-001.md` and `edit-intent-002.md`.
**Tasks Completed**: Confirmed persisted delivery statuses use the PF-02 model constants for exactly `pending`, `succeeded`, and `failed`. Added a `json.Marshal` assertion for the INF-33 escaped `\\u003ca\\u0026b\\u003e` sequence and the absence of raw `<a&b>`. The existing signature-vector body is hand-written escaped JSON (not produced by `json.Marshal`), so it and its expected signature were retained. `internal/webhook/payload.go` was not changed; its PF-15 ownership remains intact.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf09-s462-build.log'; echo "exit=$?"` exited 0; complete log `/tmp/pf09-s462-build.log`; evidence copy `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/logs/pf09-s462-build.log`.
- `bash -o pipefail -c 'go vet ./internal/webhook/... 2>&1 | tee /tmp/pf09-s462-vet.log'; echo "exit=$?"` exited 0; complete log `/tmp/pf09-s462-vet.log`; evidence copy `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/logs/pf09-s462-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/webhook/... 2>&1 | tee /tmp/pf09-s462-test.log'; echo "exit=$?"` exited 0; PASS=15, FAIL=0, SKIP=0; complete log `/tmp/pf09-s462-test.log`; evidence copy `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/logs/pf09-s462-test.log`.
- `gofmt -l internal/webhook internal/store/webhooks.go` exited 0 and printed no files; complete log `/tmp/pf09-s462-gofmt.log`; evidence copy `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/logs/pf09-s462-gofmt.log`.
- Status scan found no `"delivered"` value; `git diff --quiet -- internal/webhook/payload.go` exited 0. Final source hashes are in `tmp/punchflow-kickflow-dummy-s462/PF-09/attempt-1/source-after.sha256`.
**Reconciliation requests**: None. Independent formal review and serial combined-tree integration remain downstream workflow steps.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
