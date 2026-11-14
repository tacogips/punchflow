# PF-15 Ticket Presenters and Webhook Payloads Implementation Plan

**Status**: Completed
**Wave**: 3 | **Depends On**: PF-06, PF-07, PF-08, PF-09 | **Plan ID**: PF-15
**Design Reference**: design-docs/specs/design-webhooks.md#payload-and-headers; design-docs/specs/design-api-coverage.md#webhook-event-matrix-13-events; design-docs/references/kickflow-api.md section 4 (TicketSummary, Ticket, TicketWithStep, TicketDetail, TicketStep, TicketAssignee, TicketInput, TicketSection, SlipItem, Comment, TicketViewer, Attachment, WebhookRequestBody*, WebhookTenant); notes.md INF-16, INF-30, INF-33, INF-40, DIV-04
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

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

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: A (parallel with PF-14 and PF-16). **Depends on**: PF-06, PF-07, PF-08, PF-09, all accepted at `44e40cc`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (this plan's `writePaths` and `verification` are authoritative).
**Design**: `design-docs/specs/design-webhooks.md`; `design-docs/specs/design-api-coverage.md` webhook event matrix; `design-docs/specs/architecture.md` "Shared-tree compile and ownership-transfer rule (session 464)" item 1; `design-docs/specs/notes.md` INF-16, INF-30, INF-33, INF-40, CF-06.

**Intent**: provide the ticket, comment, viewer and tenant presenters and the 13-event webhook payload builder. PF-22/PF-23/PF-24 depend on these in wave B.

**Why s463 failed (CF-06)**: the s463 attempt saved `internal/webhook/payload.go` referencing `present.WebhookTenant`, `present.Comment`, `present.TicketWithStep` and `present.TicketDetail` before any of them existed. `go build ./...` failed for PF-14 and PF-16. `44e40cc` restored `payload.go` to the PF-09 stub. No `internal/present` ticket/comment/viewer/tenant files exist. This is a fresh start for PF-15.

**Ownership change**: `internal/webhook/payload.go` is now in this plan's `writePaths` (transferred from PF-09). Keep `NewPayloads()`, the `Build` signature and default `json.Marshal` HTML escaping. `sharedPaths` is empty.

**Mandatory order (define before reference)**:
1. Write the intent snapshot, then record the pre-edit hash of `internal/webhook/payload.go`.
2. Create the presenter files and the store read file, each compiling on its own:
   - `internal/present/tenant.go` (`WebhookTenant`)
   - `internal/present/comment.go` (`Comment`, `Comments`, `AttachmentURL` variable)
   - `internal/present/viewer.go` (`TicketViewers`)
   - `internal/present/ticket.go` (`TicketSummary`, `Ticket`, `Tickets`, `TicketWithStep`, `TicketsWithStep`, `ProxyUserIDs`)
   - `internal/present/ticket_detail.go` (`TicketDetail`)
   - `internal/store/read_tickets.go`

   Run `go build ./...` after each file write and log the exit status. Signatures are the "Contracts" block above.
3. Write `internal/present/ticket_test.go` and `comment_test.go`. After each, run `go vet ./internal/present/...` and log the result.
4. Only after `go build ./...` exits 0 with every presenter in place: edit `internal/webhook/payload.go` in one write that references only existing symbols. Run `go build ./...` immediately and log it.
5. Write `internal/webhook/payload_test.go`, then run `go vet ./internal/webhook/...`.

**Pitfalls**:
- `internal/store` is shared with PF-14 in the same wave. Exported store functions you add start with `Ticket` or `Comment`; never `Eng`. Unexported helpers start with `pf15`.
- Imitate the existing presenter style: `internal/present/user.go:User`, `internal/present/workflow.go`, and the `store.Querier` reads in `internal/store/read_*.go`.
- Do not import `internal/engine`. `Build` uses only the passed Querier and is called inside the engine transaction.
- `Build` accepts exactly the 13 `model.WebhookEventTypes`. Any other type, including the banned `ticket_submitted` and `comment_deleted`, returns an error.
- INF-33: never call `SetEscapeHTML(false)` and never add a trailing newline.
- Never edit PF-08's `internal/present` files, `internal/store/read_*.go` other than `read_tickets.go`, `internal/model/*`, or any `internal/webhook` file other than `payload.go` and `payload_test.go`.

**Test cases**: the "Tests" bullets above, with the INF-33 escaping correction: a title containing `<b>` -> payload bytes contain `<b>` and do not contain raw `<b>`. Plus:
- a table test over all 13 event types -> each builds without error
- `ticket_submitted` -> error

### Verification (session 464)

The manifest `verification` array for PF-15 (logs `/tmp/pf15-s464-*.log`), copied to `tmp/punchflow-kickflow-dummy-s464/PF-15/attempt-<n>/`. Evidence:
- build/vet exit 0
- present log: exit 0, FAIL=0, SKIP=0, PASS>0
- webhook whole-package log: exit 0, FAIL=0, SKIP=0, with PF-09's `sign_test.go`, `service_test.go` and `dispatcher_test.go` tests visible as `--- PASS`
- gofmt prints nothing

### Done Criteria (session 464)

- [x] The progress log shows the order: presenters, then build exit 0, then the `payload.go` write, then build exit 0 (with timestamps or sequence numbers)
- [x] `Build` handles exactly the 13 event types (table test) and rejects others; the HTML-escape and JST timestamp tests PASS
- [x] Present and webhook logs pass uncached with counts; PF-09 tests stay green
- [x] Pre/post hashes of `payload.go` recorded; gofmt clean; files < 1000 lines

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: A (with PF-14a, PF-14b, PF-14c, PF-16, PF-25). **Depends on**: PF-06, PF-07, PF-08, PF-09, all accepted at `ea385c7`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (11 files, `sharedPaths` empty), and the manifest `verification` array is authoritative.

**State at `ea385c7`**: s464 attempts 1-3 delivered every task. The final logs: build/vet exit 0; present tests PASS=2 FAIL=0; whole webhook package PASS=29 FAIL=0 SKIP=0; the s464 Done Criteria are all checked. The run ended only because the whole wave was blocked.

**Intent**: re-verify with uncached `-s465-` evidence and get accepted. This is a verification attempt. Edit a PF-15 file only if a verification line fails because of a PF-15 file.

**Tasks**:
1. Write the intent snapshot and record the `shasum -a 256` of `internal/webhook/payload.go` (expected `e2dcc4806ac3249358abf88dd810aa1a3087c26ae19b4ff167baac14f4bf64d7`).
2. Run the manifest verification (step names `final-*`).
3. If a line fails:
   - in a PF-15 file: fix it with the compile gate (README rule 13), keeping the define-before-reference order;
   - in a sibling's file (PF-14a/b/c in `internal/store`, PF-16): rule 13 sibling breakage. Never edit it.
4. Record exit status, log path and counts for every line.

**Pitfalls**: PF-14a/PF-14b/PF-14c edit `internal/store` (`tickets.go`, `engine_lookup.go`, the new `engine_refs.go`) at the same time. Do not add or rename exported store functions. Keep `read_tickets.go` names unchanged.

### Done Criteria (session 465)

- [x] Every manifest verification line passes: build/vet exit 0; present and webhook tests exit 0, FAIL=0, SKIP=0, PASS>0 (webhook PASS includes the PF-09 tests); gofmt check exit 0
- [x] `payload.go` hash recorded; any edit followed by a logged compile gate; the progress log has exit status, log path and counts for each line

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as the s464 overlay mutation test) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). `internal/webhook/payload.go` is a transferred path from PF-09 (session 464): record its hash before editing and keep its exported signatures.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -

### Session: 2026-10-21 (s463 attempt 1)
**Intent snapshot**: Implement the PF-15-only ticket/comment presenter and store read surface, its contract tests, and the shared `internal/webhook/payload.go` Build body. Keep PF-08 and PF-09 exported contracts intact; use standard `json.Marshal` HTML escaping. Record all verification under `tmp/punchflow-kickflow-dummy-s463/PF-15/attempt-1/`.
**Pre-edit hashes**: `impl-plans/active/pf-15-present-ticket-payloads.md` 5857c14b01ba4346614d66f7a9c5a44c7fd7203e9c1a4023fc26b107e9b02907; `internal/webhook/payload.go` 27c9581dbb0906ad2a49306b22b3e7edc697533da635863acf70c00f73e12a04.
**Tasks Completed**: Replaced the PF-09 stub body with PF-15 event validation, tenant/actor resolution, event-specific data composition, and standard `json.Marshal` serialization. `NewPayloads()` and `Build` signatures are unchanged.
**Post-edit hash**: `internal/webhook/payload.go` a0761a2352540d8946970941f62d2f5e644695217a7e3ac1025a8abd57882697.
**Verification**: `go build ./...` exit 1 (`/tmp/pf15-s463-build-current.log`, copied to `tmp/punchflow-kickflow-dummy-s463/PF-15/attempt-1/build-current.log`): PF-15 presenter symbols `WebhookTenant`, `Comment`, `TicketWithStep`, and `TicketDetail` are not implemented. Earlier moving-tree runs: vet exit 1 and present/webhook tests exit 1 with zero test cases, all due to a then-visible PF-14 compile error at `internal/store/tickets.go:56`; logs copied under attempt-1. `gofmt -l internal/webhook/payload.go` printed no paths.
**Reconciliation requests**: PF-14 owner, please ensure `internal/store/tickets.go` builds; observed `undefined: time` at line 56 during the initial moving-tree vet and test attempts (those commands were rerun after the file changed; current build proceeds and fails on the missing PF-15 presenters above).

### Session: 2026-10-21 (s464 attempt 1)
**Intent snapshot**: Implement only PF-15 ticket/comment/viewer/tenant presenters, the PF-15-owned ticket read surface, and the transferred webhook payload builder; preserve PF-08/PF-09 contracts and default `json.Marshal` escaping. Per-edit intent snapshots and command logs are under `tmp/punchflow-kickflow-dummy-s464/PF-15/attempt-1/`.
**Pre-edit hashes**: `internal/webhook/payload.go` `27c9581dbb0906ad2a49306b22b3e7edc697533da635863acf70c00f73e12a04`.
**Tasks Completed**: Added ticket summary/detail/list-step presenters, comments with deleted-body and attachment URL behavior, schema-aligned comment/attachment fields, ticket viewers, webhook tenant, conditional section evaluation, related input resolution and ticket-associated store reads. Replaced the payload stub body while preserving `NewPayloads()` and `Build` signatures; validated exactly 13 event types and rejected removed/unknown values. Added exact key-set, two-step ticket, proxy ID, comment, master input, payload event and escaping contract tests.
**Source-write/build order**: tenant presenter -> build-01 exit 1 (unrelated PF-16 `tmp/.../pre-edit/internal/fixtures` and `internal/testutil` incomplete snapshot); store read file -> build-02/03 exit 1 (`pf08StringPtr` reference corrected) -> build-04 exit 0; comment presenter -> build-05 exit 0; viewer presenter -> build-06 exit 0; ticket presenter -> build-07 exit 0; ticket detail presenter -> build-08 exit 0; only then payload body write -> build-09 exit 0. Additional detail changes -> build-10 exit 0; cleanup import attempt build-12 exit 1, restored required `encoding/json` import -> build-13 exit 0. Logs copied under attempt-1.
**Payload hash**: post-edit `e2dcc4806ac3249358abf88dd810aa1a3087c26ae19b4ff167baac14f4bf64d7`.
**Verification**: final source-matched `go build ./...` exit 0 (`/tmp/pf15-s464-build-final4.log`); final `go vet ./internal/present/... ./internal/webhook/...` exit 0 (`/tmp/pf15-s464-vet-final4.log`); uncached present tests exit 0, PASS=2 FAIL=0 SKIP=0 (`/tmp/pf15-s464-present-final4.log`); uncached whole webhook package race tests exit 0, PASS=29 FAIL=0 SKIP=0 (`/tmp/pf15-s464-webhook-final4.log`; includes PF-09 signature, service and dispatcher tests); `gofmt -l` printed no paths. Earlier present/webhook test logs showed a fixture expectation mismatch and nil timestamp panic; both were corrected and the final source-matched reruns pass. The initial build failures and intermediate vet/test failures are preserved in their numbered attempt logs.
**Post-modification agent check**: `.claude/agents/check-and-test-after-modify.md` invoked as `/root/check_after_modify`; current-tree build/vet/present/webhook verification all exit 0, PASS=2/29, FAIL=0, SKIP=0; complete logs in `tmp/punchflow-kickflow-dummy-s464/PF-15/attempt-2/`.
**Reconciliation requests**: None. PF-16 transient incomplete snapshot was absent from final build and no foreign repair was made.

### Session: 2026-10-21 (s464 attempt 3; PF15-TI-01 repair)
**Intent snapshot**: Correct the removed/unknown event assertion in `internal/webhook/payload_test.go` only. Valid ticket/comment references must ensure only the allowlist gate can reject the three listed event types; require nil body and the explicit `unsupported event type` error. Per-edit intent and complete evidence are under `tmp/punchflow-kickflow-dummy-s464/PF-15/attempt-3/`.
**Tasks Completed**: Added `TicketID: "ticket"` and `CommentID: "comment"` to each negative event. The test now checks a non-nil error containing `unsupported event type` and a nil body. Kept `ticket_submitted`, `comment_deleted`, and `future_event`. No production payload logic changed.
**Payload hash**: `internal/webhook/payload.go` remains `e2dcc4806ac3249358abf88dd810aa1a3087c26ae19b4ff167baac14f4bf64d7`.
**Verification**: `go build -o /dev/null ./...` exit 0 (`attempt-3/build.log`); `go vet ./internal/webhook/...` exit 0 (`attempt-3/vet.log`); `go test -count=1 -v -run 'Test(Ticket|Comment|Viewer|WebhookTenant)' ./internal/present/...` exit 0, PASS=2 FAIL=0 SKIP=0 (`attempt-3/present-test.log`); `go test -count=1 -v -race ./internal/webhook/...` exit 0, PASS=29 FAIL=0 SKIP=0, including PF-09 webhook signing/service/dispatcher package coverage (`attempt-3/webhook-test.log`); `gofmt -l internal/webhook/payload_test.go` printed no paths. The gate-disabled overlay mutation command `go test -count=1 -overlay attempt-3/payload-gate-disabled-overlay.json -run '^TestPayloadsBuildSupportedEventsAndEscaping$' -v ./internal/webhook/` exited 1 as expected and failed on all three accepted removed events (`attempt-3/mutation-test.log`); this demonstrates the repaired assertion detects a disabled gate. The overlay source is isolated to `attempt-3/payload-mutated.go`; repository `payload.go` is unchanged.
**Post-modification agent check**: `.claude/agents/check-and-test-after-modify.md` invoked as `/root/check_after_modify`; `go build -o /dev/null ./...`, webhook vet and uncached webhook race package checks exit 0. Agent report confirms PASS=29 FAIL=0 SKIP=0; logs at `attempt-3/build.log`, `attempt-3/vet.log`, and `attempt-3/webhook-test.log`.
**Reconciliation requests**: None.

### Session: 2026-10-21 (s465 attempt 4)
**Intent snapshot**: Re-verify the delivered PF-15 source on the s465 shared tree; make no source behavior changes unless a PF-15 verification line fails. Plan progress edit intent is preserved at `tmp/punchflow-kickflow-dummy-s465/PF-15/attempt-4/progress-edit-intent.txt`.
**Tasks Completed**: Revalidated the ticket/comment/viewer/tenant presenters, ticket read surface and webhook payload builder against the s465 acceptance criteria. The `NewPayloads()` and `Build` signatures remain unchanged. The table test covers all 13 accepted event types; removed and unknown types are rejected; payload HTML escaping and JST millisecond timestamps are asserted.
**Payload hash**: `internal/webhook/payload.go` SHA-256 `e2dcc4806ac3249358abf88dd810aa1a3087c26ae19b4ff167baac14f4bf64d7` (matches the expected session-465 hash; source unchanged in this attempt).
**Verification**: `go build ./...` exit 0 (`/tmp/pf15-s465-build.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-15/attempt-4/build.log`); `go vet ./internal/present/... ./internal/webhook/...` exit 0 (`/tmp/pf15-s465-vet.log`, copied to `attempt-4/vet.log`); uncached scoped presenter tests exit 0, PASS=2 FAIL=0 SKIP=0 (`/tmp/pf15-s465-present-test.log`, copied to `attempt-4/present-test.log`); uncached whole webhook race tests exit 0, PASS=29 FAIL=0 SKIP=0 (`/tmp/pf15-s465-webhook-test.log`, copied to `attempt-4/webhook-test.log`; includes PF-09 signing, service and dispatcher tests); pass/fail/skip counts command exit 0 (`/tmp/pf15-s465-counts.log`, copied to `attempt-4/counts.log`); gofmt clean check exit 0 (`/tmp/pf15-s465-gofmt.log`, copied to `attempt-4/gofmt.log`).
**Reconciliation requests**: None.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
