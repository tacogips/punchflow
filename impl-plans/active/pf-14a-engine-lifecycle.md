# PF-14a Engine Lifecycle and Actions Implementation Plan

**Status**: Completed
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

- [x] All 14 named tests PASS uncached with `-race`; engine package FAIL=0, SKIP=0; `./internal/store/...` PASS with FAIL=0
- [x] `engine.go` exported surface unchanged against `ea385c7`; frozen helpers unchanged in signature
- [x] The progress log records a compile gate (exit status) after every source write, or states that no source write was needed
- [x] gofmt clean on owned files; every owned file < 1000 lines; progress log has exit status, log path and counts for each command

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14a-s465-<step>.log`, evidence copies under `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

### Session: 2026-10-21 (s465 attempt 1)

**Intent snapshot**: Source behavior was already implemented. No Go source edits were needed; this session reran the required current-tree gates and updated this plan's progress and completion criteria.

**Pre-edit source identity**: `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-1/pre-edit-sha256.txt` records SHA-256 for all ten owned source/test files. All ten hashes remained unchanged through final verification. No source-write compile gate was needed; baseline `go build ./...` and `go vet ./internal/engine/... ./internal/store/...` both exited 0.

**Verification evidence** (complete logs copied under `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-1/`):

- `go build ./...`: exit 0; `/tmp/pf14a-s465-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `/tmp/pf14a-s465-vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=24, FAIL=0, SKIP=0; `/tmp/pf14a-s465-test.log`. All 14 required named tests passed.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `/tmp/pf14a-s465-store.log`.
- Required test count command: 14; both test count summary commands reported the counts above.
- Owned-file `gofmt` check: exit 0; every owned file is below 1000 lines (largest: `internal/engine/actions.go`, 554 lines).
- Exported `engine.go` contract check against `ea385c7`: exit 0; `/tmp/pf14a-s465-contract.log` is empty, so no exported signature/type line changed.
- Negative enum-literal proof: the specified `rg` command exited 1 (no matches); captured at `/tmp/pf14a-s465-enum-check.log`. This is author self-check evidence, not a required passing command.

**Progress**: PF-14a verification is complete on the shared tree. No source behavior remains for this plan. The plan remains in `active/` for downstream workflow finalization. A concurrent change appeared in `internal/fixtures/testdata/minimal/tenant.json`; that path is outside PF-14a ownership and was preserved.

### Session: 2026-10-21 (s465 attempt 2)

**Reason**: PF-14b changed `internal/engine/conditions.go` after attempt 1, so the shared engine tree moved. Attempt 1 evidence remains preserved; this attempt reran the required gates against the newer shared tree.

**Verification evidence** (complete logs copied under `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-2/`):

- `go build ./...`: exit 0; `/tmp/pf14a-s465-final-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `/tmp/pf14a-s465-final-vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=24, FAIL=0, SKIP=0; `/tmp/pf14a-s465-final-test.log`. All 14 required named tests passed.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `/tmp/pf14a-s465-final-store.log`.
- Required test count command: 14; the test count summaries reported engine PASS=24, FAIL=0, SKIP=0 and store PASS=10, FAIL=0, SKIP=0.
- Owned-file `gofmt` check: exit 0; all owned Go files remain below 1000 lines.
- Exported `engine.go` contract check against `ea385c7`: exit 0; `/tmp/pf14a-s465-final-contract.log` is empty.

**Final progress**: PF-14a assigned implementation and verification are complete on this newer shared tree. No source-write compile gate was needed. Other concurrent changes shown by `git status --short` remain owned by PF-14b/PF-15/PF-16 and were not edited here. Formal review and workflow finalization remain downstream.

### Session: 2026-10-21 (s465 attempts 3-7 reconciliation)

**Intent snapshot**: No PF-14a Go source or test files were edited. Later sibling changes moved the shared engine package; verification was repeated and every attempt's logs were retained under its numbered evidence directory.

- **Attempt 3**: PF-14b `internal/engine/routing.go` had unresolved references to `er`, `pf14bStepConditionMatch`, `pf14bFallbackAssignees`, `pf14bUserMatchesGrade`, and `pf14bUserMatchesTargets`. `go build`, vet, and engine tests exited 1. This transient foreign compile failure was resolved in the later passing attempts. Store tests, gofmt, and the engine exported-contract check passed. Logs: `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-3/`.
- **Attempt 4**: after PF-14b helpers landed, build, vet, engine tests and store tests all exited 0; engine PASS=24/FAIL=0/SKIP=0, store PASS=10/FAIL=0/SKIP=0, required named-test count=14; gofmt and contract checks passed. Logs: `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-4/`.
- **Attempt 5**: included PF-14b `conditions_test.go`; build/vet/tests/store/gofmt/contract all passed. Engine PASS=44/FAIL=0/SKIP=0, store PASS=10/FAIL=0/SKIP=0, required named-test count=14. Logs: `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-5/`.
- **Attempt 6**: PF-14b `routing_test.go` was added with `store.Querier` values passed where helpers require `*store.Tx`; vet and engine tests exited 1 before tests ran. The errors were in the sibling-owned test file and were fixed in its later update. Build, store tests, gofmt and contract checks passed. Logs: `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-6/`.
- **Attempt 7 (latest shared tree)**: build and vet exited 0. The 14 PF-14a named tests passed; engine tests exited 1 with PASS=64, FAIL=4, SKIP=0 because PF-14c-owned `internal/engine/create_update_test.go` has failures at lines 86 (unknown signed file accepted), 152 (calculation field normalization), and 162 (valid proxy applicant `authorTeamId` rejected). Store tests exited 0 with PASS=10/FAIL=0/SKIP=0; named-test count=14; gofmt and exported-contract checks exited 0. Logs: `/tmp/pf14a-s465-stable-{build,vet,test,store,contract}.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-7/`.

**Reconciliation request**: PF-14c owns `internal/engine/create_update_test.go` and the related create/forms/update implementation. Fix the three failing subtests above within PF-14c write paths, then rerun `go test -count=1 -v -race ./internal/engine/...`; PF-14a must rerun its required gates on that source before this plan's engine-package completion criterion can be checked. PF-14a did not edit foreign files.

**Current status**: PF-14a's own 14 named lifecycle tests pass, but the assigned whole-engine-package gate is not currently green due to PF-14c-owned failures. Keep this plan In Progress until the owner-routed repair passes. Formal review and workflow finalization remain downstream.

### Session: 2026-10-21 (s465 attempt 8 reconciliation)

**Intent snapshot**: No PF-14a Go source or test files were edited. The attempt-7 PF-14c reconciliation request is resolved for this shared tree: the current uncached race suite passes the whole engine package after the PF-14c repair. Pre- and post-verification hashes for all ten PF-14a-owned Go files match in `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-8/`; no source-write compile gate was needed.

**Verification evidence** (complete logs copied under `tmp/punchflow-kickflow-dummy-s465/PF-14a/attempt-8/`):

- `go build ./...`: exit 0; `build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=72, FAIL=0, SKIP=0; all 14 required PF-14a named tests passed; `engine-test.log` and `named-count.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `store-test.log`.
- Gofmt check: exit 0; line-count inventory confirms every owned Go file is below 1000 lines (largest: `internal/engine/actions.go`, 554); `gofmt.log` and `line-counts.txt`.
- Exported `engine.go` contract check against `ea385c7`: exit 0; `contract.log` is empty. The independent read-only audit also confirmed the frozen helper and Eng* store signatures match the base.
- Source identity: all ten owned source/test SHA-256 values are unchanged between `pre-edit-sha256.txt` and `post-verification-sha256.txt`; `git diff --quiet ea385c7 -- <owned source paths>` exited 0.
- Negative enum-literal self-check: the specified `rg` command observed exit 1 (no matches), recorded in `enum-check.log` and `enum-check-status.log`; this is author self-check evidence, not a required passing command.

**Final progress**: PF-14a's implementation and required verification are complete on the current shared tree. The previous attempt-7 failure in PF-14c-owned `internal/engine/create_update_test.go` no longer reproduces; no PF-14a source change was necessary. Formal review and workflow finalization remain downstream.
