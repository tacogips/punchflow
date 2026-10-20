# PF-15 Ticket Presenters and Webhook Payloads Implementation Plan

**Status**: Ready
**Wave**: 3 | **Depends On**: PF-06, PF-07, PF-08, PF-09 | **Plan ID**: PF-15
**Design Reference**: design-docs/specs/design-webhooks.md#payload-and-headers; design-docs/specs/design-api-coverage.md#webhook-event-matrix-13-events; design-docs/references/kickflow-api.md section 4 (TicketSummary, Ticket, TicketWithStep, TicketDetail, TicketStep, TicketAssignee, TicketInput, TicketSection, SlipItem, Comment, TicketViewer, Attachment, WebhookRequestBody*, WebhookTenant); notes.md INF-16, INF-30, INF-33, INF-40, DIV-04
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Build the ticket-family JSON shapes used by `getTicket`, `listTickets`, `listTasks`, `listTicketLinks`, comments and viewers, and the real webhook `PayloadBuilder` (replacing the PF-09 stub in `internal/webhook/payload.go`). Webhook payloads must be byte-for-byte the same shapes as the API responses for the same state.

## Non-goals

- No list filtering/pagination queries (PF-22 owns `ticket_queries.go`). No engine logic. Do not change PF-08 presenter files; call them.

## Files

`internal/present/ticket.go` (TicketSummary/Ticket/TicketWithStep), `ticket_detail.go` (TicketDetail, inputs, sections, slip items, steps, assignees), `comment.go` (Comment, Attachment), `viewer.go` (TicketViewer), `tenant.go` (WebhookTenant), tests `ticket_test.go`, `comment_test.go`; `internal/store/read_tickets.go`; `internal/webhook/payload.go` (shared: replace the stub body, keep `NewPayloads()` and `Build` signatures) and `internal/webhook/payload_test.go`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-15-present-ticket-payloads.md`
- `internal/present/ticket.go`
- `internal/present/ticket_detail.go`
- `internal/present/comment.go`
- `internal/present/viewer.go`
- `internal/present/tenant.go`
- `internal/present/ticket_test.go`
- `internal/present/comment_test.go`
- `internal/store/read_tickets.go`
- `internal/webhook/payload_test.go`

### sharedPaths
- `internal/webhook/payload.go`

### sharedPathNotes
- `internal/webhook/payload.go`: created by PF-09 as a stub; implement Build for all 13 event types, keeping `NewPayloads()` and the `Build` signature

### artifactRoots
- (none)

## Contracts

```go
func TicketSummary(ctx context.Context, q store.Querier, id string) (TicketSummaryJSON, error)
func Ticket(ctx context.Context, q store.Querier, id string) (TicketJSON, error)
func Tickets(ctx context.Context, q store.Querier, ids []string) ([]TicketJSON, error)
func TicketWithStep(ctx context.Context, q store.Querier, id string) (TicketWithStepJSON, error)
func TicketsWithStep(ctx context.Context, q store.Querier, ids []string) ([]TicketWithStepJSON, error) // order kept
func TicketDetail(ctx context.Context, q store.Querier, id string) (TicketDetailJSON, error)
func Comment(ctx context.Context, q store.Querier, id string) (CommentJSON, error)
func Comments(ctx context.Context, q store.Querier, ids []string) ([]CommentJSON, error)
func TicketViewers(ctx context.Context, q store.Querier, ids []string) ([]TicketViewerJSON, error)
func WebhookTenant(ctx context.Context, q store.Querier) (WebhookTenantJSON, error)
func ProxyUserIDs(ctx context.Context, q store.Querier, userID, workflowID string, today string) ([]string, error) // INF-16
```

`webhook.Payloads.Build` produces `{"eventType", "tenant", "user", "data"}` with `json.Marshal` (INF-33): ping -> `data.message = "ping"`; `ticket_*` -> `data.ticket = TicketDetail`; `comment_*` -> `data.comment = Comment`, `data.ticket = TicketWithStep`; `user` = `present.User(ev.ActorUserID)`. An unknown event type is an error.

## Mapping Rules

- `TicketSummary.workflow`: list shapes use `Workflow`; `TicketDetail.workflow` uses `WorkflowInTicket` (schema says detail contains sections and viewers).
- `publicStatus` = `forced_public_type == forced_public` or (`follow_workflow` and workflow `publicTicket`). `useRouteStepCondition` true. `logsCount` = audit_logs with this ticket_id; `commentsCount` = non-deleted comments; `linkedTicketsCount` = links.
- `Ticket`: `author`, `authorTeam` (current team row or null), `proxyClientUser`, `subStatus` null, `labels` [] (DIV-04). `TicketWithStep`/`TicketDetail`: `ticketGuest` null, `triggerTicket` null, `nextTickets` [], `cloudSignDocument` null, `customNumberingValues` [].
- `TicketDetail.route` = `RouteDetail` of `tickets.route_id` or null.
- `steps[]`: TicketStep fields from `ticket_steps`; `skip` from row; `fallback`/`fallbackResult`; `author` null; `completed` = status completed; `assignees[]` with `stepOrder`, `current` (= step in the current group and ticket in_progress), `completed`, `completedAt`, `pending` false (INF-40), `user`, `proxyUserIds` (computed with `ProxyUserIDs` for non-completed rows, `[]` for completed), `actualApprover` (detail only; omit the key entirely in list shapes, per the schema note).
- `inputs[]`: `TicketInput` with `formField` (FormFieldInTicket), `value`, and resolved `generalMasterItems`, `users`, `teams`, `inputTickets`, `attachments` (each `[]` when empty). `ticketSections[]`: one per form section; `visible` is true for unconditional sections, and for `conditional` sections it is computed with a small local evaluator over the stored input values (operators `equal`, `not_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`; any other operator counts as satisfied; combination `all`/`any`). `viewRestriction` false, `viewable` true, `sectionViewers` [].
- `slipItems[]`: `SlipItem` with `inputs[]` (`SlipItemInput` incl. `slipField`).
- `Comment`: `attachments`, `user`, `mentionedUsers`, `deletedAt`; deleted -> body "" (INF-30). `Attachment.url` comes from the package variable `var AttachmentURL func(model.File) string` declared in `comment.go` (default returns ""); PF-16 sets it to `files.Store.SignedURL` at startup.

## Key Points and Pitfalls

- The attachment URL hook (`var AttachmentURL func(model.File) string`) is the only global; PF-16 sets it to `files.Store.SignedURL`. Tests set it to a fake.
- Do not import `internal/engine` (avoid cycles); recompute proxies here with your own query.
- `Build` must use only the passed Querier. It is called inside the engine transaction (from `webhook.Enqueue`).

## Tests (load default fixtures, then create ticket rows with SQL helpers mimicking the engine output)

- TicketDetail of an in_progress 2-step ticket -> required keys present (assert the exact key set), `steps[0].assignees[0].current` true, `proxyUserIds` of sales-manager = [sales-deputy id]
- list `TicketWithStep` has no `actualApprover` key; detail does (null when not completed)
- inputs of a master field -> `generalMasterItems` populated; file field -> `attachments[0].url` from the hook
- Comment deleted -> `body` "" and `deletedAt` set
- `Build` ping -> `{"eventType":"ping",...,"data":{"message":"ping"}}`; ticket_approved -> `data.ticket.id`; comment_created -> `data.comment` and `data.ticket.steps`
- a title with `<b>` -> payload bytes contain `<b>`
- all timestamps match the JST millisecond regex

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 3. **Depends on**: PF-06, PF-07, PF-08, PF-09.
**State at 410d3d8**: not started (`internal/present` has no ticket files; `internal/webhook/payload.go` is the PF-09 stub). Implement this plan as written, with these session-462 corrections:

- Event types: `Build` handles exactly the 13 values of `model.WebhookEventTypes`. `ping` -> ping body; the 10 `ticket_*` -> ticket body; `comment_created`/`comment_updated` -> comment body. Any other value (including the removed `ticket_submitted`, `comment_deleted`) returns an error. Comment deletion is delivered as `comment_updated` with `deletedAt` set (INF-30).
- Correction to the test list above: the bullet "a title with `<b>` -> payload bytes contain `<b>`" is wrong under INF-33, because `json.Marshal` escapes HTML characters. Correct expectations for a ticket title `<b>`:
  - The payload bytes contain the 13-byte escaped sequence `\u003cb\u003e`: a backslash, `u003c`, the letter `b`, a backslash, `u003e`. In Go test source, write it as the interpreted literal `"\\u003cb\\u003e"` or the raw literal `` `\u003cb\u003e` ``.
  - The payload bytes do NOT contain the 3-byte raw string `"<b>"` (`bytes.Contains(body, []byte("<b>"))` must be false).
  - Do not call `SetEscapeHTML(false)` or otherwise change `Build`'s encoding to make a raw-`<b>` assertion pass.
- `TicketStep.status` takes values only from `model.StepStatuses`; `fallbackResult` from `model.FallbackTypes` or null; ticket `status` from `model.TicketStatuses`.
- Sections in `TicketDetail.ticketSections` follow the PF-02/PF-08 section shape. A conditional form section's `visible` is computed from its own `conditionFields` and `combinationType`.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf15-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/present/... ./internal/webhook/... 2>&1 | tee /tmp/pf15-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'Test(Ticket|Comment|Viewer|WebhookTenant)' ./internal/present/... 2>&1 | tee /tmp/pf15-s462-present-test.log"; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/webhook/... 2>&1 | tee /tmp/pf15-s462-webhook-test.log'; echo "exit=$?"   # whole package: payload.go is shared with PF-09, whose tests must stay green
for f in /tmp/pf15-s462-present-test.log /tmp/pf15-s462-webhook-test.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/present/ticket.go internal/present/ticket_detail.go internal/present/comment.go internal/present/viewer.go internal/present/tenant.go internal/present/ticket_test.go internal/present/comment_test.go internal/store/read_tickets.go internal/webhook/payload.go internal/webhook/payload_test.go   # prints nothing
```

Name the payload tests `TestPayload*` in `payload_test.go`, and the presenter tests `TestTicket*`/`TestComment*`/`TestViewer*`/`TestWebhookTenant*`, so that the `-run` patterns cover them. Evidence: exit 0; FAIL=0; SKIP=0 in both logs; the webhook log also shows the PF-09 tests passing.

### Done Criteria (session 462)

- [ ] Contract functions exist; `NewPayloads().Build` handles exactly the 13 event types (table test) and rejects others
- [ ] HTML-escape test asserts the payload contains the escaped `\u003cb\u003e` sequence (backslash-u escapes) and does not contain raw `<b>`; JST timestamp regex test passes
- [ ] Uncached present (scoped) and webhook (full package) tests pass with counts recorded; build/vet exit 0; gofmt clean

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: A (s463; parallel with PF-14 and PF-16). **Depends on**: PF-06, PF-07, PF-08 and PF-09, all accepted at f82e374.
**State at f82e374**: not started. `internal/present` has no ticket, comment, viewer or tenant files. `internal/webhook/payload.go` is still the PF-09 stub.
**Path Declarations**: unchanged. 11 declared files (< 300).

1. Implement the whole plan above, with all session-462 corrections. That includes the INF-33 escaped `<b>` expectation, which replaces the old raw `<b>` bullet.
2. `internal/webhook/payload.go` is accepted PF-09's shared stub. Replace only the `Build` body and add unexported helpers. Keep `NewPayloads()`, the `Build` signature and default HTML escaping. PF-09's tests in `internal/webhook` (`sign_test.go`, `service_test.go`, `dispatcher_test.go`) must stay green in your whole-package run.
3. Wave-A coupling: PF-14 adds `Eng*` functions to `internal/store` at the same time. Exported functions you add in `internal/store/read_tickets.go` start with `Ticket` or `Comment` (README rule 10). Never add a name starting with `Eng`.
4. Accepted code is read-only. This covers the PF-08 files in `internal/present` (`present.go`, `user.go`, `org.go`, `role.go`, `classification.go`, `master.go`, `workflow*.go`, `route.go`, `proxy.go`, `audit.go`, `present_test.go`, `workflow_test.go`), the `internal/store/read_*.go` files, `internal/model/*`, and the `internal/webhook` files other than `payload.go`. When blocked: record a reconciliation request (README rule 12) and do not edit.
5. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-15/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0 in both the present and webhook logs, and the webhook log shows the PF-09 tests passing
- [ ] The progress log has the intent snapshot, pre/post hashes of `internal/webhook/payload.go`, and exit status, log path and counts for every command

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. `internal/webhook/payload.go` is a shared path created by PF-09: record its hash before editing and keep its exported signatures.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
