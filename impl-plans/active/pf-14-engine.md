# PF-14 Approval Engine Implementation Plan

**Status**: Ready
**Wave**: 3 | **Depends On**: PF-02, PF-06, PF-07, PF-10 | **Plan ID**: PF-14
**Design Reference**: design-docs/specs/design-approval-engine.md (all); design-docs/specs/notes.md INF-11..INF-25, INF-28, INF-34..INF-36, INF-40, INF-41; design-docs/specs/design-api-coverage.md rows 56-70, 91-95, event matrix
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Implement every ticket state change behind the `internal/engine/engine.go` contract pinned by PF-10: create/update/submit, form validation, route branching and step resolution, approval progression (serial, parallel groups, all/any, proxy), send back, deny, withdraw, archive, visibility, and comments (including action comments). The engine returns `[]model.WebhookEvent`; the caller enqueues them in the same transaction. Tests run against the embedded default fixtures (`punchflow.DefaultFixtures` loaded with `fixtures.Load`).

## Non-goals

- No HTTP handling, presenters or webhook payloads (PF-22, PF-15). No audit log writes (handlers do that).
- No formula evaluation (INF-21), no sub-status, labels, pull-up or revoke (DIV-04).
- Do not change any signature in `engine.go` (PF-10 contract); you may add unexported code and new files.

## Files

`internal/engine/engine.go` (shared: replace stub bodies only), new `create.go`, `update.go`, `forms.go`, `conditions.go`, `routing.go`, `progression.go`, `actions.go`, `visibility.go`, `comments.go`, `numbering.go`; tests `setup_test.go`, `lifecycle_test.go`, `forms_test.go`, `routing_test.go`, `events_test.go`, `comments_test.go`; store `internal/store/tickets.go` (ticket, input, slip, step, assignee, custom-approver, link, comment, file-attachment reads/writes for the engine) and `internal/store/engine_lookup.go` (memberships, team ancestry, leaders, grades, proxies, workflow/route defs, tenant mode, file by signed id).

## Path Declarations

### writePaths
- `impl-plans/active/pf-14-engine.md`
- `internal/engine/create.go`
- `internal/engine/update.go`
- `internal/engine/forms.go`
- `internal/engine/conditions.go`
- `internal/engine/routing.go`
- `internal/engine/progression.go`
- `internal/engine/actions.go`
- `internal/engine/visibility.go`
- `internal/engine/comments.go`
- `internal/engine/numbering.go`
- `internal/engine/setup_test.go`
- `internal/engine/lifecycle_test.go`
- `internal/engine/forms_test.go`
- `internal/engine/routing_test.go`
- `internal/engine/events_test.go`
- `internal/engine/comments_test.go`
- `internal/store/tickets.go`
- `internal/store/engine_lookup.go`

### sharedPaths
- `internal/engine/engine.go`

### sharedPathNotes
- `internal/engine/engine.go`: created by PF-10 as the contract stub; replace the method bodies only, keeping every exported type and signature

### artifactRoots
- (none)

## Tasks

1. Store queries (`tickets.go`, `engine_lookup.go`), all Querier-based. Prefix every exported function with `Eng` (e.g. `EngLoadTicket`, `EngInsertSteps`, `EngLeadersOfTeam`) so they cannot collide with PF-15's `read_tickets.go`, which is written in parallel. Reuse PF-08's `Get*`/`*ByCode` read helpers where they fit.
2. Conditions evaluator (`conditions.go`): operators and field kinds per the "Route resolution" section of the design.
3. Form validation (`forms.go`): rules 1-7 of the design "Form validation" section; returns normalized rows to persist.
4. Routing (`routing.go`): branch selection (INF-17), step conditions, assignee resolution per stepType (INF-19, INF-20), self-approval, fallbacks, parallel groups (INF-13).
5. Progression (`progression.go`): group activation, `none` steps (INF-15), completion (INF-14), proxy acting (INF-16), event selection.
6. Actions (`create.go`, `update.go`, `actions.go`): the state machine table, guard order INF-41, tenant `admin_working`, timestamps, counters.
7. Numbering (`numbering.go`, INF-35) and visibility (`visibility.go`, INF-36).
8. Comments (`comments.go`): add/edit/delete, mentions `<@uuid>`, attachments (max 10, signed ids must exist), INF-22, INF-30, INF-37, `commentingEnabled`.
9. Tests listed below.

## Key Points and Pitfalls

- Each method runs inside the caller's `*store.Tx`; use only `tx` as the Querier. Never touch `*store.DB` (single connection deadlock).
- Guard order (INF-41): ticket exists (404 `resource_not_found`) -> `CanView` (403 `invisible_ticket`) -> tenant mode `admin_working` (403) -> status (422 `invalid_state`) -> role (403 `user_not_assigned` / `missing_permission`) -> comment rules (422 `validation_failed` `errors.comment`) -> input/assignment checks (422).
- Create: workflow must be `visible` (otherwise 422 `validation_failed`); INF-28 creation restriction -> 403 `ticket_create_restricted`; `authorTeamId` must be a team in the active chart where the actor is a member -> else 422 `validation_failed` `errors.authorTeamId`; `status` only draft/in_progress -> else 400 `invalid_parameter`. Store `author_team_snapshot` {id, name, fullName, code}.
- Events (INF-11): created [+ opened]; update -> `ticket_updated` when any non-status field was supplied, then `ticket_opened` on draft/rejected -> in_progress; approve -> `ticket_approved` if any acted row is on an `approve` step else `ticket_confirmed`, then `ticket_completed` if the ticket completed; then `comment_created` for an action comment. `ActorUserID` = actor.
- Resubmit rebuilds steps and assignees from scratch (delete old rows) and re-resolves the route. Send back keeps rows and resets completion (INF-12).
- Parallel: the actor acts on every non-completed assignee row in every in-progress step of the current group where they are the user or an active proxy. A proxy for two different assignees acts for both.
- `requiredApprovalsPercent` math: `ceil(percent * n / 100)` with integer arithmetic; n = assignee count of that step; minimum 1.
- `assignee_customizable` approvers come from `UpdateInput.Approvers` sent by a current approver (INF-20); Update by an approver ignores all other non-form fields.
- Numbering at `opened` happens on the first submit only (resubmit keeps the number). Counter is `workflows.ticket_number_counter` incremented in the same tx.
- Ticket links: a `ticket` field with `autoLink` true inserts both directions into `ticket_links`.
- Files: an input `files` signed id must exist in `files`; record `file_attachments` rows (ticket, form field or slip item, or comment).
- Dates for proxy validity and numbering use JST (`kftime.FormatDate(clock.Now())`).

## Tests (default fixtures; personas by user code; amounts as strings)

- TestSubmitMultiStepApproveCompletes: sales-rep creates expense-claim amount 5000, team sales, in_progress -> steps [sales-manager], [finance-manager, accountant]; approve sales-manager, approve finance-manager -> completed; events created, opened, approved, approved, completed; ticketNumber `EXP-<year>-1`
- TestDenyIsTerminal: deny by sales-manager -> denied, deniedAt set; approve afterwards -> 422 invalid_state
- TestSendBackToApplicantAndResubmit: approve step 1, finance-manager reject to 0 -> rejected, currentStep 0; author PATCH amount + status in_progress -> events updated, opened; steps rebuilt; number unchanged
- TestSendBackToEarlierStep: from step 2 reject to 1 -> in_progress, currentStep 1, step-1 assignee not completed; to 2 from step 2 -> 422 errors.to
- TestWithdraw: author withdraws -> rejected + ticket_withdrawn; sales-manager withdraw -> 403 missing_permission
- TestParallelAllAndAny: amount 200000 -> expense-large; group 2 = finance(any) + ceo(all); finance-manager approves -> still group 2; ceo approves -> group 3 (accountant confirm) -> accountant approve -> events include confirmed then completed
- TestProxyApproval: sales-deputy approves step 1 for sales-manager -> actualApprover sales-deputy; proxy registration with endsOn yesterday -> 403 user_not_assigned
- TestRouteBranching: amount 20000000 -> 422 errors.base ["Amount exceeds the limit"], nothing persisted
- TestPurchaseConditionsAndNomination: approvers nominee=[eng-manager], reviewer=engineer, approving-team=platform, urgent [yes], amount 60000 -> step 3 skipped, step 5 included; missing nominee -> 422; approve before budget-code filled -> 422 input_required_before_approve
- TestLeaveAssigneeCustomizableAndAuthorConfirm: hr-manager applies -> step 1 = ceo (walk-up); ceo approve with empty delegate -> 422 assignment_required_before_approve; ceo PATCH approvers delegate=[hr-manager], approve; step 3 `none` auto-completes; step 4 author confirm -> ticket_confirmed + ticket_completed
- TestCommentRules: leave-request approve with comment -> 422 errors.comment; purchase reject without comment -> 422; expense approve with comment -> comment row + comment_created after ticket_approved
- TestFormValidation: unknown formFieldCode -> 400; missing field in create -> 400; draft with empty required -> ok; submit -> 422 errors.amount; `reason`=other shows section 2 and requires `detail`
- TestCreationRestricted: finance-manager creates purchase-request -> 403 ticket_create_restricted
- TestAdminWorking: tenant mode admin_working -> approve 403 admin_is_working (after visibility)
- TestVisibility: engineer cannot view sales-rep's expense -> CanView 403 invisible_ticket; hr-manager (ticket_read) can; draft visible only to author and ticket_read holders
- TestArchive: author archives draft -> archived; author archives in_progress -> 403 missing_permission; hr-manager (ticket_write) archives completed -> ok; again -> 422 invalid_state
- TestComments: add with `<@uuid of accountant>` -> mentioned users; edit by non-author -> 403; delete -> deletedAt set, body "", comment_updated

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 3. **Depends on**: PF-06, PF-07, PF-08, PF-10 (and transitively PF-02).
**State at 410d3d8**: not started. `internal/engine/engine.go` (111 lines) holds only the PF-10 stubs. Implement this whole plan as written above, with these session-462 corrections:

- Enum values come only from `internal/model` (`design-api-coverage.md` enum contract rule 1).
  - Statuses: `model.TicketStatus*`, `model.StepStatus*`; step types: `model.StepType*`; events: `model.WebhookEvent*`.
  - Withdraw sets `model.TicketStatusRejected`; there is no withdrawn status.
  - Send-back and deny change the ticket, never a step status to `approved`/`denied`/`remanded`, which do not exist.
  - Events are exactly `ticket_created`, `ticket_updated`, `ticket_opened`, `ticket_approved`, `ticket_confirmed`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `ticket_withdrawn`, `ticket_archived`, `comment_created`, `comment_updated`.
- Conditional sections (after PF-02/PF-07): a `form` section with `Conditional == true` is shown only when its own `ConditionFields` match under its `CombinationType` (`all`/`any`). There is no separate `conditional` section type. `leave-request`: `reason` = `other` -> the "Other details" section is visible and `detail` is required.
- Slip items: `SlipItemInput.SlipSectionCode` matches `WorkflowSection.Code` (`expense-claim` slip section code `lines`). `SlipSectionID` matches the section `id`.
- Route targets: `RouteTarget.GradeSymbol` is `*string`. `nil` means no grade filter; `any_of` means the membership grade is in `GradeCodes`.
- Fallback: `RouteStep.FallbackType` values come from `model.FallbackTypes`. Store `ticket_steps.fallback_result` with the same value.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/engine/... ./internal/store/... 2>&1 | tee /tmp/pf14-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/engine/... 2>&1 | tee /tmp/pf14-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf14-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf14-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf14-s462-test.log)"
gofmt -l internal/engine internal/store/tickets.go internal/store/engine_lookup.go   # prints nothing
wc -l internal/engine/*.go internal/store/tickets.go internal/store/engine_lookup.go | sort -n | tail -2   # every file < 1000
rg -n '"(in_progress|completed|rejected|denied|archived|draft|pending|skipped|approve|confirm|none|ticket_[a-z]+|comment_[a-z]+)"' internal/engine --glob '!*_test.go'   # prints nothing: production code uses model constants
```

Evidence: exit 0 for each command; FAIL=0; SKIP=0. Each of the 17 listed tests (`TestSubmitMultiStepApproveCompletes` ... `TestComments`) appears as `--- PASS`.

### Done Criteria (session 462)

- [ ] All engine methods implemented with unchanged PF-10 signatures; the 17 listed tests pass uncached with `-race`, with counts recorded
- [ ] Engine production code uses only `internal/model` enum constants (rg check empty); conditional sections and slip codes follow the PF-02 model
- [ ] build/vet exit 0; gofmt clean; files < 1000 lines

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. `internal/engine/engine.go` is a shared path created by PF-10: record its hash before editing and keep all exported signatures.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
