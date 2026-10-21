# PF-14 Approval Engine Implementation Plan

**Status**: Completed (superseded by PF-14a/PF-14b/PF-14c)
**Wave**: 3 | **Depends On**: PF-02, PF-06, PF-07, PF-10 | **Plan ID**: PF-14
**Design Reference**: design-docs/specs/design-approval-engine.md (all); design-docs/specs/notes.md INF-11..INF-25, INF-28, INF-34..INF-36, INF-40, INF-41; design-docs/specs/design-api-coverage.md rows 56-70, 91-95, event matrix
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

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

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: A (s463; parallel with PF-15 and PF-16). **Depends on**: PF-06, PF-07, PF-08 and PF-10, all accepted at f82e374 (see `acceptedDependencies` in `impl-plans/active/punchflow-kickflow-dummy-s463-dispatch.json`).
**State at f82e374**: not started. `internal/engine/engine.go` (PF-10 stub) is the only engine file. `internal/store/tickets.go` and `internal/store/engine_lookup.go` do not exist.
**Path Declarations**: unchanged. 20 declared files (fanout limit < 300).

1. Implement the whole plan above. The session-462 corrections (model enum constants only, withdraw -> `rejected`, conditional sections, slip codes, `*string` GradeSymbol) still apply.
2. Wave-A coupling: PF-15 adds `internal/store/read_tickets.go` in the same package at the same time. Every exported store function you add starts with `Eng`. Grep `internal/store` before adding one. Do not add `Ticket*` names, which belong to PF-15's read helpers.
3. Accepted code is read-only. This covers `internal/model/*`, every existing `internal/store/*.go`, `internal/fixtures/*`, `fixtures/default/*`, `internal/kferr`, `internal/kftime`, and every exported signature in `engine.go`. If accepted code blocks you, record a reconciliation request (`file:line`, owner per `impl-plans/README.md` rule 12, command, log path) and do not edit the file. Integration review routes it; PF-27 does not repair other plans' files.
4. Verification: run the session-462 Verification block with every `-s462-` log path changed to `-s463-`. The exact commands are this plan's `verification` array in the s463 manifest. Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-<n>/`. Logs named `-s462-` are not evidence.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is met and proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, and all 17 listed tests show `--- PASS`
- [ ] The progress log has the intent snapshot, pre/post `shasum -a 256` of `internal/engine/engine.go`, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: A (parallel with PF-15 and PF-16). **Depends on**: PF-06, PF-07, PF-08, PF-10, all accepted at `44e40cc`. The PF-06 loader bug that failed all 8 s463 tests (`UNIQUE constraint failed: team_memberships`) was fixed in `44e40cc`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (this plan's `writePaths` and `verification` arrays are authoritative).
**Design**: `design-docs/specs/design-approval-engine.md`; `design-docs/specs/architecture.md` "Shared-tree compile and ownership-transfer rule (session 464)"; `design-docs/specs/notes.md` INF-11..INF-41 and CF-06.

**Intent**: finish the engine so that the PF-22/PF-23/PF-24 handlers can drive real ticket lifecycles and webhook events. Resume from the partial files at `44e40cc`; do not rewrite working code.

**State at 44e40cc** (static inspection; run the tests first to learn the real state):
- Present: `actions.go`, `comments.go`, `conditions.go`, `create.go`, `engine.go`, `forms.go`, `numbering.go`, `routing.go`, `update.go`, `visibility.go`, `setup_test.go`, `lifecycle_test.go`; `internal/store/tickets.go`, `internal/store/engine_lookup.go`.
- Missing files from `writePaths`: `progression.go` (may stay absent if the logic lives in `actions.go`; record which), `forms_test.go`, `routing_test.go`, `events_test.go`, `comments_test.go`.
- Present tests (8): `TestSubmitMultiStepApproveCompletes`, `TestDenyIsTerminal`, `TestSendBackToApplicantAndResubmit`, `TestSendBackToEarlierStep`, `TestWithdraw`, `TestParallelAllAndAny`, `TestProxyApproval`, `TestRouteBranching`.
- Missing tests (9), as specified in "Tests" above: `TestPurchaseConditionsAndNomination`, `TestLeaveAssigneeCustomizableAndAuthorConfirm` (put in `routing_test.go`); `TestCommentRules`, `TestComments` (`comments_test.go`); `TestFormValidation` (`forms_test.go`); `TestCreationRestricted`, `TestAdminWorking`, `TestVisibility`, `TestArchive` (`events_test.go` or `lifecycle_test.go`).

**Ownership change**: `internal/engine/engine.go` is now in this plan's `writePaths` (transferred from PF-10, `ownershipTransfers[]`). Its exported types, method names and signatures stay byte-identical; only bodies change. `sharedPaths` is empty.

**Steps**:
1. Write the intent snapshot and the pre-edit `shasum -a 256` of every existing file you will touch.
2. Run the manifest's `go build` and `go test` commands once, unchanged, to get the baseline (log `-s464-baseline`). Fix failures in the 8 existing tests first.
3. Add the 9 missing tests, then implement or fix the behavior until all 17 PASS.
4. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/engine/...`.
   - Log each result. Fix your own breakage before any other edit.

**Pitfalls**:
- `internal/store` is shared with PF-15 (`read_tickets.go`) in the same wave. New exported store functions start with `Eng`; never `Ticket*` or `Comment*`. Unexported helpers start with `pf14`. Never leave `tickets.go` with a missing import: s463 broke PF-15 with `undefined: time` at `tickets.go:56`.
- Never edit `internal/fixtures/*`, `fixtures/default/*` (now PF-16's), `internal/model/*`, `internal/store/*` files other than your two, or `internal/present/*`. PF-16 may atomically replace `fixtures/default/*.json` during this wave. Ids, codes and values do not change, so tests keyed by user/workflow codes stay valid. If a fixture load fails mid-wave, re-run once before filing a reconciliation request.
- Tests use the fake clock from `setup_test.go`; no `time.Sleep`.

**Test-case contract**: the 17 bullets in "Tests" above (input -> expected) are the acceptance tests; names must match exactly.

### Verification (session 464)

The manifest `verification` array for PF-14 (log names `/tmp/pf14-s464-*.log`), copied to `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-<n>/`. Evidence:
- build/vet exit 0
- `go test -count=1 -v -race ./internal/engine/...` exit 0, FAIL=0, SKIP=0, and 17 `--- PASS` lines, one per listed test name (check with `grep -c -- '--- PASS: Test' /tmp/pf14-s464-test.log`; subtests may add more)
- gofmt prints nothing; every file < 1000 lines; the enum `rg` prints nothing

### Done Criteria (session 464)

- [x] All 17 listed tests PASS uncached with `-race` (exit 0, FAIL=0, SKIP=0), with counts and log paths recorded
- [x] `engine.go` exported signatures unchanged (`git diff 44e40cc -- internal/engine/engine.go` is empty; note this in the log)
- [ ] Every source write was followed by a logged compile gate with exit 0 (rule 13)
- [x] New exported store functions start with `Eng`; gofmt clean; files < 1000 lines; enum rg empty

## Continuation (session 465) - superseded index

This plan is no longer dispatched (CF-07; design `architecture.md` "Plan split, evidence-reporting and wave rule (session 465)"). Its files and remaining work are split three ways:

| Plan | File | Owns |
|---|---|---|
| PF-14a | `impl-plans/active/pf-14a-engine-lifecycle.md` | `engine.go`, `actions.go`, `comments.go`, `visibility.go`, `numbering.go`, `setup_test.go`, `lifecycle_test.go`, `events_test.go`, `comments_test.go`, `internal/store/tickets.go` |
| PF-14b | `impl-plans/active/pf-14b-engine-routing.md` | `routing.go`, `conditions.go`, `routing_test.go`, `conditions_test.go`, `internal/store/engine_lookup.go` |
| PF-14c | `impl-plans/active/pf-14c-engine-forms-create.md` | `forms.go`, `create.go`, `update.go`, `forms_test.go`, `create_update_test.go`, `internal/store/engine_refs.go` |

`internal/engine/progression.go` is dropped (never created; `pf14Advance` is in `actions.go`). This file keeps its "Tests" section and Progress Log as history, and the split plans cite it. Nobody edits it in s465 except PF-27, which archives it to `impl-plans/completed/pf-14-engine.md`.

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Superseded in session 465; see the split plans.

## Progress Log

### Session: 2026-10-21 (session 463, attempt 1)
**Intent snapshot**: Implement PF-14 ticket engine behavior while preserving the PF-10 exported contract. Create the engine persistence and lookup helpers, form/condition/routing/action/visibility/comment/numbering implementations, and fixture-backed lifecycle tests. No accepted model, fixture, auth, time, store, or plan-index files are edited.
**Tasks Completed**: Added engine store persistence and lookups; wired the PF-10 methods to engine implementations; implemented core ticket create/update/submit, route resolution, form checks, progression and actions, visibility, comments and numbering; added eight lifecycle test cases. All newly exported store functions use the `Eng` prefix. Enum use is model-constant based and slip sections resolve by ID or code.
**Engine contract hash**: pre-edit `internal/engine/engine.go` sha256 `2e2d07ad0adde490754158198551b2a7d09ffbeb1c89e8ce391d8be4acdf2169`; post-edit sha256 `ff55503a0739acd6b08a791f39604223dfadada78e806e3ebd6b948aa72fe650`.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14-s463-build.log'; echo "exit=$?"` — exit 1; complete log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-build.log`; build is blocked by PF-15 `internal/webhook/payload.go` undefined `present.WebhookTenant`, `present.Comment`, `present.TicketWithStep`, and `present.TicketDetail`.
- `bash -o pipefail -c 'go vet ./internal/engine/... ./internal/store/... 2>&1 | tee /tmp/pf14-s463-vet.log'; echo "exit=$?"` — exit 0; complete log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v -race ./internal/engine/... 2>&1 | tee /tmp/pf14-s463-test.log'; echo "exit=$?"; echo "PASS=$(grep -c -- '--- PASS' /tmp/pf14-s463-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf14-s463-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf14-s463-test.log)` — exit 1; PASS=0 FAIL=8 SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-test.log`; every test fails during fixture setup before its assertions because `fixtures.Load` inserts a duplicate `(team_id,user_id)` membership.
- `bash -o pipefail -c 'gofmt -l internal/engine internal/store/tickets.go internal/store/engine_lookup.go 2>&1 | tee /tmp/pf14-s463-gofmt.log'; echo "exit=$?"` — exit 0; no output; log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-gofmt.log`.
- `bash -o pipefail -c 'wc -l internal/engine/*.go internal/store/tickets.go internal/store/engine_lookup.go | sort -n | tail -10 2>&1 | tee /tmp/pf14-s463-lines.log'; echo "exit=$?"` — exit 0; largest file is `internal/engine/actions.go` at 418 lines (all files < 1000); log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-lines.log`.
- `bash -o pipefail -c 'rg -n '\''"(in_progress|completed|rejected|denied|archived|draft|pending|skipped|approve|confirm|none|ticket_[a-z]+|comment_[a-z]+)"'\'' internal/engine --glob '\''!*_test.go'\'' 2>&1 | tee /tmp/pf14-s463-enums.log'; echo "exit=$?"` — exit 1 (no matches; production engine uses model constants); log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-enums.log`.
- `bash -o pipefail -c 'grep -c -- "--- PASS" /tmp/pf14-s463-test.log; grep -c -- "--- FAIL" /tmp/pf14-s463-test.log; grep -c -- "--- SKIP" /tmp/pf14-s463-test.log' 2>&1 | tee /tmp/pf14-s463-test-counts.log; echo "exit=$?"` — exit 0; counts 0/8/0; log `tmp/punchflow-kickflow-dummy-s463/PF-14/attempt-1/pf14-s463-test-counts.log`.
**Reconciliation requests**:
- PF-06 owns `internal/fixtures/load.go:130-179` (accepted path). The `teamIDs` map is keyed only by team code while memberships from multiple charts are loaded; `go test -count=1 -v -race ./internal/engine/...` fails at fixture load with `UNIQUE constraint failed: team_memberships.team_id, team_memberships.user_id`, log `/tmp/pf14-s463-test.log`. Please repair the chart-specific team ID mapping and re-accept PF-06; resume PF-14 by rerunning the engine tests against the embedded default fixtures.
- PF-15 owns `internal/webhook/payload.go` (verified in the session-463 ownership index). `go build ./...` fails because the PF-15 payload references four presenter types not yet defined, log `/tmp/pf14-s463-build.log`. Resume the full build after PF-15 lands and rerun this plan's build gate.
**Reconciliation status**: Behavioral tests and full build remain incomplete pending the owner-routed repairs above. The test file currently contains eight of the plan's 17 named cases; remaining test coverage and any failures after fixture setup are still outstanding.

### Session: 2026-10-21 (session 464, attempt 1)
**Intent snapshot**: Resume the existing PF-14 implementation; repair fixture-backed test transaction deadlocks in the owned `setup_test.go` helper; preserve the current engine contract and shared worker changes. The intended fix caches user/team IDs before actions enter the single-connection in-memory transaction. No production engine or store file was edited.
**Pre-edit hashes**: `internal/engine/setup_test.go` `f26365781f4fd6cd43a87db6ef2bc3455805d04d528e66be50b0578d907251dd` (initial read); before the cache-helper edit `b7b10da31918513adb1135eab6e85266246d6753f58d9eae811427613b800099`. Final `setup_test.go` sha256 `74d9aff6d0b3bd6e7de8b6d35388be6df00a172845e0c4e58510e5828e0e5790`. `internal/engine/engine.go` sha256 remains `ff55503a0739acd6b08a791f39604223dfadada78e806e3ebd6b948aa72fe650`; `git diff 44e40cc -- internal/engine/engine.go` is empty, so all exported signatures are unchanged.
**Tasks completed**: Fixed test setup's deadlock by resolving the sales team ID before `WithTx`, and cached all fixture user/team IDs during setup so `actor()`/`team()` remain transaction-safe. No acceptance tests were added in this attempt; nine required names remain missing: `TestPurchaseConditionsAndNomination`, `TestLeaveAssigneeCustomizableAndAuthorConfirm`, `TestCommentRules`, `TestComments`, `TestFormValidation`, `TestCreationRestricted`, `TestAdminWorking`, `TestVisibility`, `TestArchive`.
**Verification**:
- Baseline `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14-s464-baseline-build.log'` — exit 0; complete log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-baseline-build.log`.
- Baseline `go test -count=1 -v -race ./internal/engine/...` — first test hung in `pf14TestEnv.createExpense`; the initial run was interrupted before a shell exit footer, so it is not passing evidence. Diagnostic `go test -count=1 -v -race -timeout=20s -run ^TestSubmitMultiStepApproveCompletes$ ./internal/engine` — exit 1 by timeout; stack located reentrant DB access at `setup_test.go:52`; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-diagnose-test.log`.
- Post-edit focused `go test -count=1 -v -race -timeout=90s -run ^TestSubmitMultiStepApproveCompletes$ ./internal/engine` — exit 0, 1 PASS, 0 FAIL, 0 SKIP; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-diagnostic-fixed-test.log`.
- `go vet ./internal/engine/...` after each of the two `_test.go` writes — exit 0; logs `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-vet-after-setup.log` and `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-vet-after-cache.log`.
- Final `go build ./...` — exit 0; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-build.log`.
- Final `go vet ./internal/engine/... ./internal/store/...` — exit 0; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-vet.log`.
- Final `go test -count=1 -v -race ./internal/engine/...` — exit 0; PASS=8, FAIL=0, SKIP=0; only the eight existing tests ran, so the 17-test acceptance criterion is incomplete; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-test.log`.
- `gofmt -l internal/engine internal/store/tickets.go internal/store/engine_lookup.go` — exit 0, no output; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-gofmt.log`.
- File-size inventory — max file 418 lines (`internal/engine/actions.go`); log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-lines.log`.
- Enum literal `rg` check — exit 1, no matches in non-test engine code; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final-enums.log`.
**Remaining work**: The nine named tests and their required behaviors have not been implemented or verified. Keep all session-464 Done Criteria unchecked; resume PF-14 implementation before review.

### Session: 2026-10-21 (session 464, attempt 2)
**Intent snapshot**: Add the nine missing named acceptance tests and repair issues they expose while preserving the accepted engine API and shared files. Change only PF-14-owned engine/test files and this plan's progress record.
**Implementation updates**:
- Fixed `setup_test.go` fixture helper reentrant DB access by caching fixture users and active-chart teams before entering the single-connection transaction. Filled all form inputs in existing lifecycle fixtures so they satisfy Create completeness.
- Added the nine missing tests across `forms_test.go`, `routing_test.go`, `comments_test.go`, and `events_test.go`.
- Enforced 400 `invalid_parameter` for unknown/duplicate form field references and incomplete Create payloads; excluded approver-only fields from applicant submit validation and added approval-time validation for required approver fields.
- Required author-customizable nominees at route creation; stored selected assignees as ticket assignee rows; kept the author assigned to explicit author steps; auto-completed activated `none` steps and progressed through consecutive non-actionable groups.
- Enforced action-comment requirements/disabled-commenting rules and restricted send-back/deny to assignees or proxies on approve steps.
**Verification**:
- Final `go build ./...` — exit 0, complete log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-build.log`.
- Final `go vet ./internal/engine/... ./internal/store/...` — exit 0, complete log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-vet.log`.
- Final `go test -count=1 -v -race ./internal/engine/...` — exit 0; 24 PASS lines (17 named top-level tests including subtests), FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-test.log`.
- `gofmt -l internal/engine internal/store/tickets.go internal/store/engine_lookup.go` — exit 0, no output; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-gofmt.log`.
- File-size inventory — largest file `internal/engine/actions.go` at 554 lines; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-lines.log`.
- Enum literal `rg` — exit 1, no matches (expected); log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-enums.log`.
- Exported engine-store functions all have the `Eng` prefix; listing log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-store-exports.log`.
- `git diff 44e40cc -- internal/engine/engine.go` — exit 0, empty; log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-engine-contract.log`.
- Shared-tree `go build ./...` passed after the source-file batches: `actions.go` `/tmp/pf14-s464-build-after-actions.log`, `forms.go` `/tmp/pf14-s464-build-after-form-resolution.log`, `create.go` `/tmp/pf14-s464-build-after-create-completeness.log`, approver required-field helper `/tmp/pf14-s464-build-after-approver-required.log`, approval helper `/tmp/pf14-s464-build-after-approval-fields.log`, action call `/tmp/pf14-s464-build-after-check-approval.log`, custom assignees `/tmp/pf14-s464-build-after-custom-approvers.log`, nomination `/tmp/pf14-s464-build-after-nominee.log`, author step `/tmp/pf14-s464-build-after-author-step.log`, none progression `/tmp/pf14-s464-build-after-none-progress.log`; each exited 0. A first two-hunk `actions.go` edit was compiled after both hunks, so the per-write compile log criterion remains unchecked.
- `go vet ./internal/engine/...` passed after test file writes; logs include `pf14-s464-vet-after-complete-expense.log`, `pf14-s464-vet-after-route-inputs.log`, delegated `pf14-s464-vet-after-forms-test.log`, `pf14-s464-vet-after-routing-test.log`, `pf14-s464-vet-after-complete-inputs.log`, and `pf14-s464-vet-comments-fix3.log`, all under the attempt-1 evidence directory.
**Incomplete assigned work**: The remaining design-backed implementation gaps still need work before PF-14 is complete: type/reference validation across all form field kinds; the full condition operator set and team ancestry/grade-aware route target resolution; `higher_manager`/`higher_team` fallback resolution; proxy-applicant registration/date/workflow validation for create/update; and merging partial `UpdateForm` values without dropping untouched fields/references. These are PF-14-owned behaviors, not downstream-plan work.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
