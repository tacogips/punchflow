# PF-22 /v1 Tickets, Actions and Viewers Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-14, PF-15, PF-16 | **Plan ID**: PF-22
**Design Reference**: design-docs/specs/design-api-coverage.md rows 56-70 and the webhook event matrix rows 2-11; design-docs/specs/design-approval-engine.md; design-docs/references/kickflow-api.md section 3 "Tickets"; notes.md INF-02, INF-11, INF-34, INF-36, INF-40, INF-41
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

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

- [ ] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the 10 ticket events asserted by `eventType`; the `not implemented` check exit 0; every file < 1000 lines; gofmt check exit 0
- [ ] Pre/post hashes of the transferred files; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). Handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
