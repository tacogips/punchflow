# PF-22 /v1 Tickets, Actions and Viewers Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-14, PF-15, PF-16 | **Plan ID**: PF-22
**Design Reference**: design-docs/specs/design-api-coverage.md rows 56-70 and the webhook event matrix rows 2-11; design-docs/specs/design-approval-engine.md; design-docs/references/kickflow-api.md section 3 "Tickets"; notes.md INF-02, INF-11, INF-34, INF-36, INF-40, INF-41
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

---

## Intent and Context

Expose the engine through the 15 ticket operations: list/search, tasks, get, create, update, partial form update, approve, send back, deny, withdraw, archive, links and viewers. Every state-changing call runs the engine and enqueues its webhook events in the same transaction, then notifies the dispatcher after commit. This plan also proves, through the HTTP API and the test receiver, that each ticket_* event is emitted, signed and persisted.

## Non-goals

- No engine changes (log reconciliation requests). No comment or file endpoints (PF-23).

## Files

`internal/api/v1/tickets.go`, `ticket_actions.go`, `ticket_viewers.go` (shared: replace stubs); tests `tickets_test.go`, `ticket_actions_test.go`, `ticket_viewers_test.go`, `ticket_webhooks_test.go` (`package v1_test`); `internal/store/ticket_queries.go` (new: list/filter queries for tickets and tasks, viewers CRUD, links list).

## Path Declarations

### writePaths
- `impl-plans/active/pf-22-api-tickets.md`
- `internal/api/v1/tickets_test.go`
- `internal/api/v1/ticket_actions_test.go`
- `internal/api/v1/ticket_viewers_test.go`
- `internal/api/v1/ticket_webhooks_test.go`
- `internal/store/ticket_queries.go`

### sharedPaths
- `internal/api/v1/tickets.go`
- `internal/api/v1/ticket_actions.go`
- `internal/api/v1/ticket_viewers.go`

### sharedPathNotes
- `internal/api/v1/tickets.go`: PF-10 stub file; replace the bodies of operations 56-61
- `internal/api/v1/ticket_actions.go`: PF-10 stub file; replace the bodies of operations 62-66
- `internal/api/v1/ticket_viewers.go`: PF-10 stub file; replace the bodies of operations 67-70

### artifactRoots
- (none)

## Handler Conventions

Same as PF-18 "Handler Conventions", plus for engine calls:

```
actor := engine.Actor{UserID: p.User.ID, Permissions: p.Permissions}
err := a.d.DB.WithTx(ctx, func(tx *store.Tx) error {
    evs, err := a.d.Engine.<Method>(ctx, tx, actor, ...)        // engine owns guards and ordering (INF-41)
    if err != nil { return err }
    if err := store.InsertAuditLog(ctx, tx, ... TicketID ...); err != nil { return err }
    if err := a.d.Events.Enqueue(ctx, tx, evs); err != nil { return err }
    resp, err = present.TicketDetail(ctx, tx, id)
    return err
})
if err == nil { a.d.Events.Notify() }
```

Audit actions: `ticket.create`, `ticket.update`, `ticket.updateForm`, `ticket.approve`, `ticket.reject`, `ticket.deny`, `ticket.withdraw`, `ticket.archive`, `ticketViewer.create`, `ticketViewer.delete`; always set `TicketID`.

## Operation Notes

| Op | Rules |
|---|---|
| listTickets | all spec filters (`status[]` with bare value allowed, `subStatusIds[]` -> always empty match when non-empty, `workflowId`, `authorId`, `authorTeamFullName` <= 300, `ticketNumber`, created/updated/opened/completed/archived ranges via `httpx.QueryTime`, `assigneeUserId` + `assigneeStatus[]` (`all`, `current`, `approved`), `stepTitle` <= 300); visibility INF-36 in SQL (`ticket_read`/`ticket_write` see all); sortBy createdAt/updatedAt; default createdAt desc; returns TicketWithStep |
| listTasks | INF-34 and INF-40 (`pending=true` -> []); sortBy also `openedAt`, `assignedToMeAt`; `assignedToMeAt*` ranges |
| getTicket | `Engine.CanView` then `present.TicketDetail` |
| createTicket | decode `engine.CreateInput`; `Engine.Create` |
| updateTicket | decode `engine.UpdateInput`; `Engine.Update` |
| updateTicketForm | decode `engine.FormPatch`; `Engine.UpdateForm` |
| approve/reject/deny | body optional (`Content-Length: 0` allowed); `reject` requires integer `to` (missing -> 400 `invalid_parameter`) |
| withdraw/archive | no body |
| listTicketLinks | CanView; paginated `Ticket` list |
| listViewers | CanView; default order team name then user e-mail; sortBy createdAt; paginated |
| createViewer | CanView; workflow `allowEditingOfViewers` or `ticket_write`; exactly one of `userId`/`teamId` (single or array) -> else 400; `gradeId`/`descendants` only with teamId -> else 400; unknown ids -> 422; empty 200 |
| deleteViewer | same permission; unknown viewer -> 404; empty 200 |

## Key Points and Pitfalls

- Never call `a.d.DB.Q()` inside the tx; presenters and `Enqueue` take `tx`.
- `Notify` only after a successful commit.
- Visibility filtering must happen in SQL before pagination, so `Total` counts only visible tickets.
- Decode with `httpx.DecodeJSON` so wrong types give 400 `invalid_parameter` (e.g. `"to": "1"`).

## Tests (`package v1_test`, receiver via `env.RegisterReceiverWebhook(t, "s3cret")`)

- create expense-claim (Employee, amount 5000, team sales, in_progress) -> 200 TicketDetail; receiver after `DeliverWebhooks` gets `ticket_created` then `ticket_opened`; each has `X-Kickflow-Signature` verified with `webhook.Verify`, `User-Agent: kickflow-Hookshot/v1`; `webhook_events` and `webhook_attempts` rows exist
- approve Manager, approve Finance -> `ticket_approved` x2 then `ticket_completed`; webhook `data.ticket.status` = completed in the last
- amount 200000 (expense-large): Manager, Finance, then Integration persona with CallerFor `ceo` approve, then CallerFor `accountant` approve -> `ticket_confirmed` then `ticket_completed`
- reject to 0 -> `ticket_rejected`; PATCH amount + status in_progress -> `ticket_updated`, `ticket_opened`; updateTicketForm -> `ticket_updated`
- deny -> `ticket_denied`; withdraw -> `ticket_withdrawn`; archive (Admin) completed ticket -> `ticket_archived`
- approve by Employee (not assigned) -> 403 `user_not_assigned`; Employee GET an engineer's ticket -> 403 `invisible_ticket`; unknown id -> 404
- reject with `"to":"x"` -> 400; approve with empty body and `Content-Length: 0` -> 200
- listTickets Employee -> only visible tickets, `Total` correct, `status[]=completed` filter, bare `status=draft` works, `status=a&status=b` -> 400; Admin (ticket_read via Super Admin) sees all
- listTasks Manager -> ticket awaiting sales-manager; Integration persona with CallerFor `sales-deputy` sees the same ticket (proxy, INF-34); `pending=true` -> []
- viewers: add team viewer with grade -> listed; userId and teamId both -> 400; delete -> 200 empty
- receiver responds 500 then 200 -> second `DeliverWebhooks` after advancing the fake clock past the backoff delivers with the same `X-Kickflow-Delivery`

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-14, PF-15, PF-16.
**State at 410d3d8**: not started (PF-10 stubs). Implement this plan as written. Session-462 points:

- The ten ticket events proven through the receiver are exactly `ticket_created`, `ticket_updated`, `ticket_opened`, `ticket_approved`, `ticket_confirmed`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `ticket_withdrawn`, `ticket_archived` (`model.WebhookEvent*`). Assert each by `eventType` in the received bodies.
- Query enum validation:
  - `listTickets` `status[]` in `model.TicketStatuses`; `assigneeStatus[]` in `all|current|approved` (spec query enum; not a step status)
  - `listTasks` `status[]` in `draft|in_progress|rejected|completed|deleted` (`deleted` matches `archived`, INF-34)
  - other values -> `400 invalid_parameter`
- Withdraw results in ticket `status: rejected` (assert it).
- Name tests `TestTicket*`, `TestTickets*`, `TestTask*`, `TestViewer*`, `TestTicketWebhook*`; prefix helpers with `pf22`.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf22-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf22-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Ticket|Tickets|Task|Viewer|TicketWebhook)' ./internal/api/v1/... 2>&1 | tee /tmp/pf22-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf22-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf22-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf22-s462-test.log)"
gofmt -l internal/api/v1/tickets.go internal/api/v1/ticket_actions.go internal/api/v1/ticket_viewers.go internal/api/v1/tickets_test.go internal/api/v1/ticket_actions_test.go internal/api/v1/ticket_viewers_test.go internal/api/v1/ticket_webhooks_test.go internal/store/ticket_queries.go   # prints nothing
rg -n 'not implemented' internal/api/v1/tickets.go internal/api/v1/ticket_actions.go internal/api/v1/ticket_viewers.go   # prints nothing (exit 1)
wc -l internal/api/v1/ticket*.go internal/store/ticket_queries.go | sort -n | tail -2   # every file < 1000
```

### Done Criteria (session 462)

- [ ] 15 operations implemented; the ten `ticket_*` events proven via HTTP + receiver with verified signatures; query enums validated
- [ ] Listed tests pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean; files < 1000 lines

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-14, PF-15 and PF-16 (all wave A).
**State at f82e374**: not started. `tickets.go`, `ticket_actions.go` and `ticket_viewers.go` are PF-10 stubs. `internal/store/ticket_queries.go` does not exist.
**Path Declarations**: unchanged. 9 declared files (< 300).

1. Implement the whole plan above, with the session-462 points. Handlers call the engine (PF-14) inside one `store.WithTx` and enqueue the returned events in that same tx. Responses come from the PF-15 presenters.
2. Webhook retry tests advance the fake clock (`testutil`) past the backoff and call `DeliverWebhooks`. Never `time.Sleep` for retry delays, and never start the dispatcher goroutine.
3. Do not edit `internal/engine/*` (PF-14), `internal/present/*` (PF-08/PF-15), `internal/testutil/*` (PF-16), or `internal/api/v1/api.go`, `routes.go` or `routes_test.go` (PF-10). Engine or presenter defects become reconciliation requests against PF-14 or PF-15 (README rule 12).
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-22/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0, the 10 ticket events asserted by `eventType`, the `not implemented` rg empty, and every file < 1000 lines
- [ ] The progress log has the intent snapshot, pre/post hashes of the shared files, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-14 (engine), PF-15 (ticket presenters and payloads), PF-16 (testutil, receiver, server).
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 56-70 and the webhook event matrix; `design-approval-engine.md`; `design-webhooks.md`; INF-11, INF-12, INF-24, INF-34, INF-36 in `notes.md`.

**Intent**: unchanged. Implement 15 ticket operations. Prove the ten `ticket_*` events through HTTP plus the testutil receiver, with verified `X-Kickflow-Signature`.

**Session-464 changes**:
1. `internal/api/v1/tickets.go`, `ticket_actions.go` and `ticket_viewers.go` are in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty. Replace bodies only.
2. Use the engine, presenter and testutil contracts exactly as PF-14/PF-15/PF-16 left them. Read their progress logs for the final exported names. Never edit:
   - `internal/engine/*`
   - `internal/present/*`
   - `internal/webhook/*`
   - `internal/testutil/*`
   - `internal/server/*`
   - `api.go`, `routes.go`, `routes_test.go`

   A missing behavior there is a reconciliation request to PF-14, PF-15 or PF-16 (README rule 12).
3. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/api/v1/...`.
   - Log each result.
   - Prefixes: unexported identifiers start with `pf22`. New exported store functions start with `TicketQuery`, which is not used by PF-15's `Ticket*` read helpers: grep first.
4. Webhook tests call `Env.DeliverWebhooks` and use the fake clock. Never `time.Sleep`, and never start the dispatcher.
5. Guard: the manifest `-run 'TestPF10'` line must PASS. Logs are `/tmp/pf22-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-22/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0; the 10 ticket events asserted by `eventType`; PF10 guard PASS; the `not implemented` rg empty; every file < 1000 lines
- [ ] Pre/post hashes of the transferred files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B2 (with PF-23, PF-24). **Depends on**: PF-14a, PF-14b, PF-14c, PF-15, PF-16. The single PF-14 dependency is replaced by its three split plans (design `architecture.md` "Plan split, evidence-reporting and wave rule (session 465)" item 1).
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (9 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: implement the 15 ticket operations as thin handlers over `engine.Engine`, and prove the ten `ticket_*` events through HTTP plus the testutil receiver with verified signatures. Logs are `/tmp/pf22-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-<n>/`.

**Engine contract in s465**: call only the exported surface of `internal/engine/engine.go` (`New`, `Create`, `Update`, `UpdateForm`, `Approve`, `SendBack`, `Deny`, `Withdraw`, `Archive`, `CanView`, and the input types). It is frozen and unchanged since `ea385c7`. Never edit `internal/engine/*` or the `Eng*` store files. An engine defect becomes a reconciliation request to the owner by file: `actions.go`/`comments.go`/`visibility.go`/`numbering.go` -> PF-14a; `routing.go`/`conditions.go` -> PF-14b; `forms.go`/`create.go`/`update.go` -> PF-14c.

### Done Criteria (session 465)

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the 10 ticket events asserted by `eventType`; the `not implemented` check exit 0; every file < 1000 lines; gofmt check exit 0
- [x] Pre/post hashes of the transferred files; every source write followed by a logged compile gate with exit 0
- [x] Review follow-up PF22-TI-01: HTTP coverage for `updateTicketForm` and `listTicketLinks`, hidden-ticket GET, unassigned approve, and Admin list-all passes without changing the six existing test functions.

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). Handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: 2026-10-21 (session 465 attempt 1)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-1/edit-intent-*.md`; source evidence and complete logs are in the same attempt directory.
**Tasks Completed**:
- Implemented ticket/task list and query filters, create/get/update/form update, approve/send-back/reject/deny/withdraw/archive handlers, link listing, viewer create/list/delete, transactional audit records, and ticket query store helpers.
- Added HTTP tests for ticket detail/list, task behavior, transitions, viewers, and webhook delivery/signatures/retry. Corrected test assumptions for single-page pagination and fixture user names.
- Fixed viewer list composition to pass ticket IDs to the PF-15 presenter while preserving the store query's paginated order.
**Verification**:
- `go build ./...`: exit 0; final log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-1/build-final.log`. The build immediately after the last non-test Go edit also exited 0 (`build-viewer-fix.log`).
- `go vet ./internal/api/v1/...`: exit 0 after the latest test write; `vet-final3.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2 FAIL=0 SKIP=0; `pf10-final.log`.
- Scoped race suite `go test -count=1 -v -race -run 'Test(Ticket|Tickets|Task|Viewer|TicketWebhook)' ./internal/api/v1/...`: exit 1, PASS=3 FAIL=3 SKIP=0; `test-final4.log`. Viewer create/list/delete, draft ticket create/get/list, and webhook retry test pass. Action, lifecycle webhook and task tests cannot set up an in-progress ticket with the documented ID-only form input and receive 422 for required `amount` and `purpose`.
- `gofmt -l` check: exit 0; no files listed (`gofmt-final.log`). `rg -n 'not implemented'` check: expected rg exit 1/no matches, wrapper exit 0 (`stub-final.log`). All eight Go files are below 1000 lines (`line-counts-final.txt`). Source SHA-256 values are in `source-sha256-final.txt`.
- Earlier scoped race attempt `test-initial.log` failed before compiling due to transient PF-23 test helpers; this was superseded by the final-source run and is not an implementation failure.
**Reconciliation requests**:
- To PF-14c, `internal/engine/create.go` `pf14ValidateCreateCompleteness`: the accepted OpenAPI contract permits `formFieldId` alone, but required-value validation looks up `values[field.Code]`; `pf14InputValues` only sets that key when the request also supplies `formFieldCode`. Current PF-22 ID-only create requests reproduce 422 `amount`/`purpose` required errors, blocking in-progress creation and dependent action/task/webhook behavioral verification. Fix within PF-14c ownership and rerun PF-22 scoped race suite. PF-22 intentionally does not change engine files or conceal the contract with duplicate codes.
**Remaining work**: Resolve the PF-14c reconciliation, rerun required plan suite and verify all ten ticket webhook event types/signatures through the receiver. Then re-evaluate the session-465 Done Criteria; they remain unchecked.

### Session: 2026-10-21 (session 465 attempt 2; PF-22 continuation)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/edit-intent-plan-progress.md`.
**Source identity**: unchanged for the initial continuation rerun; its before/after hashes are in `source-sha256-before.txt` and `source-sha256-after.txt`. Subsequent review-driven changes and final hashes are recorded below in the attempt-2 follow-up.
**Tasks Completed**:
- Re-ran the required PF-22 gates on the current shared tree after PF-14b repaired ID-only form input resolution in its owned `internal/engine/conditions.go` path.
- Confirmed the ticket receiver test asserts all ten `ticket_*` event types and verifies signatures; the passing lifecycle and retry tests cover persisted event/attempt delivery behavior.
- Closed the session-465 Done Criteria for implementation. Formal integration review and PF-27 final gates remain downstream workflow work.
**Verification**:
- `go build ./...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-build.log`.
- `go vet ./internal/api/v1/...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-vet.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-pf10guard.log`.
- `go test -count=1 -v -race -run 'Test(Ticket|Tickets|Task|Viewer|TicketWebhook)' ./internal/api/v1/...`: exit 0, PASS=6, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-test.log`.
- PF-22 `gofmt -l` selected-file check: exit 0, no files listed; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-gofmt.log`.
- Negative proof `rg -n 'not implemented' internal/api/v1/tickets.go internal/api/v1/ticket_actions.go internal/api/v1/ticket_viewers.go`: rg found no matches (rg exit 1), wrapper exit 0; author-self-check evidence log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-stubcheck.log`.
- All PF-22 Go files are below 1000 lines; complete counts in `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-line-counts.log` (largest: `tickets.go`, 339 lines).
**Reconciliation requests**: None outstanding for PF-22. The earlier PF-14c request was addressed by PF-14b within its authorized path; this continuation verified the downstream PF-22 suite against that source.
**Remaining work**: None for PF-22 implementation. Await the serial combined-tree integration review and PF-27 final gate as downstream workflow steps.

#### Attempt-2 review follow-up (current source)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/edit-intent-final-progress.md`; per-edit intentions are in the same directory.
**Implementation changes**:
- `internal/api/v1/tickets.go` and `internal/store/ticket_queries.go`: visibility-qualified ticket IDs now feed the final SQL count/page query, eliminating the million-row cap and keeping visibility filtering before pagination. `ticket_read`/`ticket_write` list-all behavior and assigned task constraints are retained.
- Task proxy matching now checks the active proxy record in the assignee-to-caller direction and respects its date and workflow scope. `assignedToMeAt` filters and sort use `ticket_assignees.assigned_at` for matching direct or proxy assignments.
- `internal/api/v1/tickets_test.go`: assert an unrelated finance ticket is hidden from the employee total, the configured sales deputy can see the manager's task, and assignment-time order is independent of creation order.
- `internal/api/v1/ticket_webhooks_test.go`: assert `ticket_completed` payload status is `completed`.
**Verification on final source**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf22-s465-build-final.log'; echo "exit=$?"`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-build-final.log`.
- `bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf22-s465-vet-final.log'; echo "exit=$?"`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-vet-final.log`.
- `bash -o pipefail -c "go test -count=1 -v -run 'TestPF10' ./internal/api/v1/... 2>&1 | tee /tmp/pf22-s465-pf10guard-final.log"; echo "exit=$?"`: exit 0, PASS=2, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-pf10guard-final.log`.
- `bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Ticket|Tickets|Task|Viewer|TicketWebhook)' ./internal/api/v1/... 2>&1 | tee /tmp/pf22-s465-test-final-current.log"; echo "exit=$?"`: exit 0, PASS=6, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-test-final-current.log`.
- Selected-file `gofmt -l` check: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-gofmt-final.log`.
- Negative `rg -n 'not implemented' ...` proof: no matches, rg exit 1 and wrapper exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-stubcheck-final.log`.
- All eight PF-22 Go files are below 1000 lines; counts in `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/pf22-s465-line-counts-final.log`. Final SHA-256 values are in `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-2/source-sha256-final.txt`.
**Reconciliation requests**: None outstanding for PF-22. The initial attempt-2 PF-22 suite passed after PF-14b's fix; the review follow-up then addressed the remaining list/task acceptance gaps within PF-22's declared paths.
**Remaining work**: None for PF-22 implementation. Serial combined-tree integration review and PF-27 final gate remain downstream workflow work.

**Superseded intermediate gate outcomes (attempt 2)**:
- The first build after the list-handler edit exited 1 for an unused `errors` import (`pf22-s465-build-after-list-handler.log`); removing that import was followed by build exit 0 (`pf22-s465-build-after-import-fix.log`).
- Vet immediately after the initial ticket test write exited 1 because the test referenced undefined `testutil.Engineer` (`pf22-s465-vet-after-ticket-tests.log`); changing to the existing Finance persona was followed by vet exit 0 (`pf22-s465-vet-after-persona-fix.log`).
- First expanded race run exited 1, PASS=4 FAIL=2 SKIP=0: finance visibility setup used the wrong team and the proxy direction was missing (`pf22-s465-test-after-review-fixes.log`). Both were corrected; subsequent targeted checks passed.
- The next race run exited 1, PASS=5 FAIL=1 SKIP=0 because unrelated seeded tasks entered the assignment-order assertion (`pf22-s465-test-final.log`). The test was isolated by its assignment-time range; the following run exited 1, PASS=5 FAIL=1 SKIP=0 because timestamps did not match the accepted parser format (`pf22-s465-test-final2.log`). Using the supported millisecond-Z format was followed by the final current-source suite PASS=6 FAIL=0 SKIP=0.
- These intermediate failures are retained as complete logs and superseded by the final source-matched verification above; none remain unresolved.

### Session: 2026-10-21 (session 465 attempt 3; PF22-TI-01 test-integrity repair)
**Review source**: test-integrity decision `comm-008429`, finding `PF22-TI-01` (mid), assigned to PF-22. Direct feedback required form-patch HTTP success/empty-body error, link-list HTTP/pagination/hidden/unknown cases, hidden-ticket GET, unassigned approve, and Admin list-all; the six existing tests were preserved.
**Intent snapshots**: `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-3/edit-intent-tickets-tests.md`, `edit-intent-actions-tests.md`, `edit-intent-test-corrections.md`, and `edit-intent-plan-progress.md`.
**Changes**:
- Added `TestTicketAuthorizationGuards`, `TestTicketUpdateFormHTTP`, and `TestTicketLinksHTTP` in `internal/api/v1/tickets_test.go`.
- Added `TestTicketActionsUnassignedApprove` in `internal/api/v1/ticket_actions_test.go`.
- The first attempt-3 run showed that an in-progress form patch must be made by the current approver, so the test uses Manager. It also showed that `Link` is omitted when no relation applies to the empty list; the test now asserts the required Page, Per-Page and Total headers. No production change or engine reconciliation was needed.
- Existing six test functions were not edited; no test skips, sleeps or production test branches were added.
**Verification**:
- `go build ./...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-3/build-final.log`.
- `go vet ./internal/api/v1/...`: exit 0 after each test-file write and final source; logs `vet-after-ticket-tests.log`, `vet-after-actions-test.log`, `vet-after-test-correction.log`, and `vet-final.log` in attempt-3.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0; `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-3/pf10guard-final.log`.
- `go test -count=1 -v -race -run 'Test(Ticket|Tickets|Task|Viewer|TicketWebhook)' ./internal/api/v1/...`: final exit 0, PASS=10, FAIL=0, SKIP=0, CACHED=0; `tmp/punchflow-kickflow-dummy-s465/PF-22/attempt-3/test-final.log`. It includes the four added tests and all six existing tests.
- Selected-file `gofmt -l` check: exit 0, no paths; `gofmt.log`. `go build` and `go vet` final logs are complete and empty on success.
- The negative stub proof found no matches (rg exit 1; wrapper exit 0); log `stubcheck.log`. All individual ticket Go files and `ticket_queries.go` are under 1000 lines; counts `line-counts-verified.log`.
- SHA-256 for the eight PF-22 source files is recorded in `source-sha256-final.txt`.
**Superseded attempt-3 verification**: the first race run exited 1, PASS=8, FAIL=2, SKIP=0 because the form test used Employee instead of the assigned approver and the link test required a Link relation for an empty list. Both test assumptions were corrected and the final uncached race run passed. The first line-count wrapper also exited 1 because it treated `wc`'s aggregate `total` row as a source file; the corrected per-file check passed. Logs are preserved in attempt-3.
**Reconciliation requests**: None. The current PF-22 implementation and required verification are complete. Combined-tree integration review and PF-27 final gates remain downstream workflow steps.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
