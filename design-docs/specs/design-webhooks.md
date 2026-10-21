# Webhook Delivery Design

Outgoing kickflow-compatible webhooks: registration, event fan-out, payload and signature, delivery worker, retries, persistence, manual resend, test delivery and arbitrary event firing.

## Overview

kickflow has no REST API for webhooks, so registration lives in the punchflow dev admin API (`/admin/api/webhooks...`, see `design-auth-admin.md`) and the UI. Delivery behavior follows `design-docs/references/kickflow-api.md` section 5. Package: `internal/webhook`.

```
engine action (tx) --events--> webhook.Enqueue(tx)      -> webhook_events rows (outbox, one per matching webhook)
commit ----------------------> dispatcher.Notify()
dispatcher loop --------------> due events -> POST (10 s timeout) -> webhook_attempts row -> schedule retry / done
```

## Technical Details

### Data model

| Table | Columns (main) |
|---|---|
| `webhooks` | id, name, url, secret (nullable), enabled, workflow_ids JSON (empty = all workflows), event_types JSON (empty = all 12 non-ping events), first_failure_at, disabled_at, created_at, updated_at |
| `webhook_events` | id (= `X-Kickflow-Delivery` UUID), webhook_id, event_type, ticket_id, comment_id, body BLOB (exact bytes sent), status (`pending`, `succeeded`, `failed`), attempt_count, next_attempt_at, last_status_code, created_seq, created_at, updated_at |
| `webhook_attempts` | id, event_id, attempt_no, started_at, duration_ms, request_headers JSON, status_code (nullable), response_body (first 64 KiB), error (nullable), manual (bool) |

Delivery history is kept until DB reset. Secrets are stored in plaintext because punchflow must compute HMACs; they are dev values and are never returned by list endpoints (admin API returns `hasSecret: true`; the value only on create/update responses).

### Fan-out rules

- `Enqueue(tx, events)` creates one `webhook_events` row per event and per enabled webhook whose `event_types` contains the event type (or is empty) and whose `workflow_ids` contains the ticket's workflow (or is empty). Webhooks receive events for every ticket regardless of visibility (Confirmed).
- `ping` targets exactly one webhook and ignores filters.
- Delivery id: new UUID per row (INF-09).
- Body bytes are built inside the transaction from the post-action state (INF-11) and stored; they never change afterwards.

### Event emission matrix

Events are produced by the engine (and the UI API) in this order within one action; every payload shows the state after the whole action (INF-11). The per-operation list is in `design-api-coverage.md` (webhook event matrix).

| Action | Events (in order) | Notes |
|---|---|---|
| Create `draft` | `ticket_created` | |
| Create `in_progress` | `ticket_created`, `ticket_opened`, and `ticket_completed` when the route needs no user action | Skipped groups and `actionType: none` steps are traversed at submission (INF-72) |
| Update draft/rejected fields | `ticket_updated` | Same-state `status` saves emit it only when something changed (INF-71) |
| Update + submit (`status: in_progress`) | optional `ticket_updated`, `ticket_opened`, and `ticket_completed` as above | |
| Update by an approver | `ticket_updated` | |
| Approve, step not yet complete | `ticket_approved` (`ticket_confirmed` for a confirm step) | Partial approval of a multi-assignee step still emits (INF-74) |
| Approve, last group done | `ticket_approved` (or `ticket_confirmed`), `ticket_completed` | |
| Send back (`to` any) | `ticket_rejected` | |
| Deny | `ticket_denied` | |
| Withdraw | `ticket_withdrawn` | |
| Archive | `ticket_archived` | |
| User deletion | one `ticket_archived` per archived ticket, attributed to the acting administrator; `ticket_completed` for a ticket that completes because a step lost its assignees | INF-79 |
| UI control operations: cancel approval, add/remove custom step, attach/detach label | `ticket_updated` (one per real change) | Raise is audited but emits no webhook event (INF-66) |
| Comment create / update / delete, or a comment sent with approve/reject/deny | `comment_created` / `comment_updated` | After the action event (INF-22) |

### Payload and headers

| Item | Value |
|---|---|
| Body | `{"eventType":..., "tenant": WebhookTenant, "user": User, "data": {...}}` encoded per INF-33 |
| `data` | ping `{message:"ping"}`; ticket_* `{ticket: TicketDetail}`; comment_* `{comment: Comment, ticket: TicketWithStep}` |
| `Content-Type` | `application/json` |
| `User-Agent` | `kickflow-Hookshot/v1` |
| `X-Kickflow-Delivery` | event row id |
| `X-Kickflow-Signature` | `sha256=` + lowercase hex HMAC-SHA256(secret, body); omitted when the webhook has no secret (Confirmed prose) |

Ticket payloads use the shared presenter with an explicit webhook policy. Values from restricted form sections are omitted from `inputs` and section inputs: `value` is null and resolved user/team/master/ticket references and attachments are empty. `formField` definitions remain present. This rule is independent of the event actor and differs from caller-filtered REST presentation (INF-SEC-02). Comment event ticket summaries keep their existing `TicketWithStep` shape.

### Dispatcher

- One dispatcher per server process, started by `serve` with the server context and stopped on shutdown (waits for in-flight attempts up to the 10 s timeout).
- Wake-ups: `Notify()` after commits that enqueued events, a 1 s ticker, and startup (so persisted pending events survive restarts).
- Selection: due rows (`status = pending AND next_attempt_at <= now`) ordered by `next_attempt_at, created_seq`, restricted in SQL (before the batch limit) to rows of enabled webhooks plus rows with a pending manual resend, so a backlog of undeliverable rows of disabled webhooks never starves enabled ones; the rows of disabled webhooks stay `pending` untouched. At most one in-flight attempt per webhook (preserves first-attempt order per webhook); at most 4 concurrent attempts overall.
- HTTP client: 10 s timeout; redirects not followed (3xx is a failure); proxy from environment disabled. No TLS verification override (dev receivers normally use plain `http://localhost`).
- Outcome: exactly `200` = success (`succeeded`, clears webhook `first_failure_at`). Anything else, timeout or connection error = failure: record the attempt, and if `attempt_count < 11` set `next_attempt_at = now + backoff(attempt_count)` (INF-08), otherwise `failed`. Auto-disable per INF-10.
- No database transaction is held during the HTTP call.
- Concurrent deletion: deleting a webhook cascades to its events and attempts. If the webhook or delivery row is gone before lookup or by the time the attempt is recorded (including while the HTTP request is in flight), that single delivery is cancelled silently and the batch continues. Any other per-delivery store error is logged and retried on the next poll (without the wake-up fast path); it never stops the dispatcher or the HTTP server. Only context cancellation ends the dispatcher.
- Testability: the dispatcher takes a `kftime.Clock`, an `*http.Client`, and a `Backoff func(retry int) time.Duration`; tests use a zero backoff and an httptest receiver. `RunOnce(ctx)` processes currently due rows synchronously so tests do not sleep.

### Manual operations (admin API, `integration` permission)

| Operation | Behavior |
|---|---|
| Send test (`POST /admin/api/webhooks/{id}/ping`) | Enqueues a `ping` for that webhook and returns the event row |
| Resend (`POST /admin/api/webhook-events/{eventId}/resend`) | Sets the row back to `pending`, `next_attempt_at = now`; the next attempt is flagged `manual`, reuses the delivery id and body, recomputes the signature with the current secret (INF-09). Allowed in any status, also for disabled webhooks (the attempt is still made) |
| Fire event (`POST /admin/api/webhook-events`) | Body `{eventType, ticketId?, commentId?, webhookIds?}`. Builds the payload from the current state of the referenced ticket/comment (ticket required for ticket_* and comment_*, comment required for comment_*), actor = the admin. With `webhookIds` it targets those webhooks ignoring their filters; without it, normal fan-out rules apply. Returns the created event rows |
| List / inspect | `GET /admin/api/webhooks/{id}/events` (paginated like /v1), `GET /admin/api/webhook-events/{eventId}` with its attempts |

Webhook create, and update that changes `url` or `secret`, enqueue a `ping` (Confirmed). Re-enabling a disabled webhook is done with `PATCH {enabled: true}`, which clears `disabled_at` and `first_failure_at`. `first_failure_at` is cleared only by a successful delivery or by this disabled-to-enabled transition; ordinary edits (name, filters, url, secret, or `enabled: true` on an already enabled webhook) preserve it, so edits cannot postpone automatic disabling.

PATCH reads existing configuration to merge omitted fields, but persists ordinary configuration columns separately from delivery state. The failure clock and enabled/disabled timestamps are changed only by an explicit `enabled` transition, using SQL state updates within the PATCH transaction; stale configuration snapshots therefore cannot erase a dispatcher failure recorded after the read (WH-05).

### Required tests (`internal/webhook/*_test.go`)

| Test | Checks |
|---|---|
| TestSignatureVector | Fixed secret + body containing `<`, `>`, `&` -> body contains `<`, `>`, `&`; signature equals a precomputed hex value; receiver-side verification helper accepts it |
| TestHeaders | `User-Agent`, `Content-Type`, `X-Kickflow-Delivery`, signature presence/absence by secret |
| TestFanOutFilters | event and workflow filters, disabled webhook skipped, ping ignores filters |
| TestRetryThenSuccess | receiver returns 500 twice then 200: 3 attempts, same delivery id, status succeeded |
| TestRetryExhaustion | always 500: 11 attempts then `failed` |
| TestNon200AndRedirectFail | 201, 302, 404 count as failures |
| TestTimeout | receiver sleeps past a shortened client timeout -> failure recorded with error |
| TestAutoDisable | failure with `first_failure_at` 7 days old (fake clock) disables the webhook |
| TestResendAndFire | resend reuses id/body with a new attempt; fire builds each of the 13 event types |
| TestPersistAcrossRestart | pending row created before dispatcher start is delivered after start |

End-to-end event emission from lifecycle actions is covered in `internal/engine/events_test.go` plus one API-level test per event in `internal/api/v1/*_test.go` using the test receiver from `internal/testutil`.

## References

- `design-docs/references/kickflow-api.md` section 5
- `design-docs/specs/notes.md` INF-08 to INF-11, INF-33
- `design-docs/specs/design-api-coverage.md` webhook event matrix
