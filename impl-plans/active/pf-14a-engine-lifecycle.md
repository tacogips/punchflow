# PF-14a Engine Lifecycle and Actions Implementation Plan

**Status**: Ready
**Wave**: A (s465) | **Depends On**: PF-06, PF-07, PF-08, PF-10 (all accepted at `ea385c7`) | **Plan ID**: PF-14a
**Design Reference**: design-docs/specs/design-approval-engine.md "State machine", "Step progression", "Comments with actions", "Plan ownership (session 465)"; design-docs/specs/architecture.md "Plan split, evidence-reporting and wave rule (session 465)"; design-docs/specs/notes.md INF-11..INF-16, INF-22, INF-24, INF-25, INF-35, INF-36, INF-41, CF-07
**Supersedes (in part)**: impl-plans/active/pf-14-engine.md (read its "Tests" and session 462-464 sections for history; this plan is authoritative for the files below)
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

PF-14 (the approval engine) did not fit in one attempt and was split into PF-14a/PF-14b/PF-14c (CF-07). This plan owns the ticket state machine: Approve/SendBack/Deny/Withdraw/Archive, step progression (`pf14Advance`), action comments, comments with mentions, visibility, ticket numbering, the INF-41 check order, the `engine.go` surface and the engine store writes in `internal/store/tickets.go`.

State at `ea385c7`: this behavior is implemented. The s464 attempt-2 log (`impl-plans/active/pf-14-engine.md`, Progress Log) records 24 PASS and 0 FAIL for `go test -count=1 -v -race ./internal/engine/...` (log `tmp/punchflow-kickflow-dummy-s464/PF-14/attempt-1/pf14-s464-final2-test.log`). The only Done criterion it left unchecked was a logged compile gate after every source write. The remaining work is therefore:

1. Re-run the baseline uncached and record s465 evidence.
2. Fix any failure in the 14 named tests that this plan owns.
3. Keep them green while PF-14b and PF-14c edit sibling files in the same package.
4. Record a compile gate for every write this plan makes. If this plan makes no source write, record that and give the baseline build/vet logs.

## Non-goals

- Routing, conditions and assignee resolution (PF-14b: `routing.go`, `conditions.go`, `internal/store/engine_lookup.go`).
- Form validation, Create, Update, UpdateForm and proxy-applicant checks (PF-14c: `forms.go`, `create.go`, `update.go`).
- HTTP handlers, presenters, webhook payloads (PF-22, PF-15). No audit log writes.
- Do not rewrite working code. No refactoring, renaming or moving of functions between files.

## Path Declarations

### writePaths
- `impl-plans/active/pf-14a-engine-lifecycle.md`
- `internal/engine/engine.go`
- `internal/engine/actions.go`
- `internal/engine/comments.go`
- `internal/engine/visibility.go`
- `internal/engine/numbering.go`
- `internal/engine/setup_test.go`
- `internal/engine/lifecycle_test.go`
- `internal/engine/events_test.go`
- `internal/engine/comments_test.go`
- `internal/store/tickets.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

`internal/engine/engine.go` was transferred PF-10 -> PF-14 in s464, and moves to PF-14a in s465 (manifest `ownershipTransfers[]`).

## Contracts (frozen; design "Plan ownership (session 465)")

- `engine.go`: every exported type, method name and signature stays byte-identical to `ea385c7` (`git diff ea385c7 -- internal/engine/engine.go` must show only body changes, and is expected to be empty).
- Helpers this plan owns that sibling plans call. Keep their signatures and current results: `pf14Guard(ctx, q store.Querier, a Actor, id string) (model.Ticket, error)`, `pf14Event(kind, id, actor string) model.WebhookEvent`, `pf14CanAct(ctx, tx, a, t) ([]model.TicketStep, []int, error)`, `pf14Advance`, `pf14Number(ctx, tx, ticket *model.Ticket, def model.WorkflowDef, now time.Time) error`.
- Every exported function in `internal/store/tickets.go` (`EngLoadTicket` ... `EngLinkTickets`) keeps its signature and behavior. PF-14b and PF-14c call them.
- Test helpers used by sibling test files: `pf14Setup`, `pf14TestEnv` and its methods, `pf14CreateWorkflowTicket`, `pf14EventTypes`, `assertPF14FieldError`, `assertPF14ErrorCode`. Keep their signatures.
- Helpers this plan calls but does not own. Never edit them: `pf14ValidateApprovalInputs` (`forms.go`, PF-14c), `pf14Contains` (`routing.go`, PF-14b), `pf14CompleteInputs` and `pf14FormDefinition` (`forms_test.go`, PF-14c), and the `store.Eng*` functions in `engine_lookup.go` (PF-14b).

## Tasks

1. Write the intent snapshot and the pre-edit `shasum -a 256` of all 10 owned source and test files.
2. Run the Verification commands once as the baseline (step name `baseline`).
3. If any of the 14 owned tests fails, find the cause.
   - If the cause is in an owned file, fix it there (minimal change), with the compile gate after each write.
   - If the cause is in a sibling's file (PF-14b/PF-14c), handle it as sibling breakage (README rule 13). Re-check it at final verification. Never edit the sibling's file.
4. Run the final Verification commands (step name `final`) and update the Done Criteria and Progress Log.

## Key Points and Pitfalls

- Use only the caller's `*store.Tx` as the Querier; never `*store.DB` (single-connection deadlock; s464 fixed one in `setup_test.go` by caching ids before `WithTx`).
- INF-41 order: exists (404) -> visible (403 `invisible_ticket`) -> `admin_working` (403 `admin_is_working`) -> status (422 `invalid_state`) -> assigned (403 `user_not_assigned`) -> comment rules (422) -> input/assignment checks (422).
- Withdraw sets `model.TicketStatusRejected`. Enum literals in production code come only from `internal/model` constants.
- `internal/store` is also edited by PF-14b (`engine_lookup.go`) and PF-14c (new `engine_refs.go`) in this wave. Never add an exported store function that is not prefixed `Eng`, and never leave `tickets.go` with a missing import.
- PF-14b and PF-14c are editing `routing.go`, `conditions.go`, `forms.go`, `create.go` and `update.go` at the same time. `TestRouteBranching`, `TestParallelAllAndAny` and `TestSubmitMultiStepApproveCompletes` go through their code. A transient failure there is sibling breakage, not a reason to edit their files.

## Tests (must PASS by name; specifications in pf-14-engine.md "Tests")

- `lifecycle_test.go`: `TestSubmitMultiStepApproveCompletes`, `TestDenyIsTerminal`, `TestSendBackToApplicantAndResubmit`, `TestSendBackToEarlierStep`, `TestWithdraw`, `TestParallelAllAndAny`, `TestProxyApproval`, `TestRouteBranching`
- `events_test.go`: `TestCreationRestricted`, `TestAdminWorking`, `TestVisibility`, `TestArchive`
- `comments_test.go`: `TestCommentRules`, `TestComments`

## Verification (manifest `verification` array for PF-14a; all must pass)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14a-s465-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/engine/... ./internal/store/... 2>&1 | tee /tmp/pf14a-s465-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/engine/... 2>&1 | tee /tmp/pf14a-s465-test.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v ./internal/store/... 2>&1 | tee /tmp/pf14a-s465-store.log'; echo "exit=$?"
for f in /tmp/pf14a-s465-test.log /tmp/pf14a-s465-store.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
grep -c -E -- '--- PASS: (TestSubmitMultiStepApproveCompletes|TestDenyIsTerminal|TestSendBackToApplicantAndResubmit|TestSendBackToEarlierStep|TestWithdraw|TestParallelAllAndAny|TestProxyApproval|TestRouteBranching|TestCreationRestricted|TestAdminWorking|TestVisibility|TestArchive|TestCommentRules|TestComments) ' /tmp/pf14a-s465-test.log   # 14
test -z "$(gofmt -l internal/engine/engine.go internal/engine/actions.go internal/engine/comments.go internal/engine/visibility.go internal/engine/numbering.go internal/engine/setup_test.go internal/engine/lifecycle_test.go internal/engine/events_test.go internal/engine/comments_test.go internal/store/tickets.go)"; echo "exit=$?"
bash -o pipefail -c 'git diff ea385c7 -- internal/engine/engine.go | grep -E "^[-+](func|type|\t[A-Z])" | tee /tmp/pf14a-s465-contract.log; test ! -s /tmp/pf14a-s465-contract.log'; echo "exit=$?"   # exit=0: no exported signature/type line changed in engine.go
```

Evidence each must show: build/vet `exit=0`; engine and store tests `exit=0`, FAIL=0, SKIP=0, PASS>0; the named-test count is 14; gofmt `exit=0`; contract check `exit=0`. Copy logs to `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-<n>/`. Production-code enum check (no hard-coded status/event literals) goes into `authorSelfCheck.checks` as a negative proof: `rg -n '"(in_progress|completed|rejected|denied|archived|draft|pending|skipped|ticket_[a-z]+|comment_[a-z]+)"' internal/engine/actions.go internal/engine/comments.go internal/engine/visibility.go internal/engine/numbering.go internal/engine/engine.go` is expected to exit 1 (no match).

## Done Criteria

- [ ] All 14 named tests PASS uncached with `-race`; engine package FAIL=0, SKIP=0; `./internal/store/...` PASS with FAIL=0
- [ ] `engine.go` exported surface unchanged against `ea385c7`; frozen helpers unchanged in signature
- [ ] The progress log records a compile gate (exit status) after every source write, or states that no source write was needed
- [ ] gofmt clean on owned files; every owned file < 1000 lines; progress log has exit status, log path and counts for each command

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14a-s465-<step>.log`, evidence copies under `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

(none yet)
