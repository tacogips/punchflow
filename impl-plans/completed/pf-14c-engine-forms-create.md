# PF-14c Engine Forms, Create and Update Implementation Plan

**Status**: Completed
**Wave**: A (s465) | **Depends On**: PF-06, PF-07, PF-08, PF-10 (all accepted at `ea385c7`) | **Plan ID**: PF-14c
**Design Reference**: design-docs/specs/design-approval-engine.md "Form validation" rules 1-7, "Proxy application", "State machine" (Create/Update rows), "Plan ownership (session 465)"; design-docs/specs/notes.md INF-20, INF-21, INF-26, INF-28, CF-07; design-docs/specs/architecture.md "Plan split, evidence-reporting and wave rule (session 465)"
**Supersedes (in part)**: impl-plans/active/pf-14-engine.md
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

PF-14c finishes ticket input handling. That means validating every form field kind, including references to users, teams, tickets, master items and uploaded files. It also means checking proxy applications (`proxyClientUserId`) on Create and Update, and making `UpdateForm` merge a partial patch instead of dropping untouched fields. These are the gaps that the PF-14 s464 attempt-2 log lists as "Incomplete assigned work".

State at `ea385c7`:
- `pf14NormalizeForm` (`forms.go:15`) and `pf14ValidateValue` (`forms.go:142`) do shape checks only.
  - No reference checks for `user`/`team`/`ticket`/master items/file signed ids.
  - No `allowedTicketStatus`, no checkbox/pull_down `options` check, no `regexpFormat`/length/decimalDigit checks.
- `create.go:81` copies `in.ProxyClientUserID` with no registration check. `update.go:68-75` does the same.
- `pf14UpdateForm` (`update.go:227`) needs to merge the patch into the stored inputs.
- `TestFormValidation` passes. Keep it passing.

## Non-goals

- Routing/conditions (PF-14b), lifecycle/actions/comments (PF-14a), HTTP handlers (PF-22/PF-23).
- Formula evaluation (calculation values stay `null`, INF-21). `button_api`/`button_kintone` are stored as given and never executed (DIV-03).
- No changes to `internal/model/*`, fixtures, migrations, or store files other than the new `engine_refs.go`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-14c-engine-forms-create.md`
- `internal/engine/forms.go`
- `internal/engine/create.go`
- `internal/engine/update.go`
- `internal/engine/forms_test.go`
- `internal/engine/create_update_test.go`
- `internal/store/engine_refs.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts (frozen; design "Plan ownership (session 465)")

- Owned functions called from sibling files. Their signatures stay fixed, and so do their results for inputs that work today:
  - `pf14Create`, `pf14Update`, `pf14UpdateForm`: `engine.go` (PF-14a) calls them with today's signatures.
  - `pf14ValidateApprovalInputs(def model.WorkflowDef, rows []model.TicketInput) error` (`actions.go`).
  - `pf14InputsFromRows(def, rows) []FieldInput` (`update.go`; called by `forms.go`).
  - `pf14NormalizeForm`, `mustActiveChart`, `pf14ErrorContext`.
- Owned test helpers that sibling tests call: `pf14CompleteInputs` (also called by `setup_test.go`, `lifecycle_test.go`, `routing_test.go`), `pf14FormDefinition` (`routing_test.go`), `pf14Input`. Keep their signatures and behavior.
- New store file `internal/store/engine_refs.go`. Exported names start with `Eng`, for example:
  - `EngUserExists(ctx, q Querier, id string) (bool, error)`
  - `EngTeamInActiveChart(ctx, q, id string) (bool, error)`
  - `EngTicketStatus(ctx, q, id string) (string, bool, error)`
  - `EngMasterItemInMaster(ctx, q, itemID, masterCode string) (bool, error)`
  - `EngActiveProxyApplicant(ctx, q, clientUserID, proxyUserID, workflowID, date string) (bool, error)`
  - Before adding one, grep `internal/store` for an existing reader (`read_proxies.go`, `GetUser*`, `TeamIDByCode`) and reuse it if it fits.
- Calls but never edits: `pf14BuildRoute`, `pf14InputValues`, `pf14SectionVisible`, `pf14FieldValue`, `pf14Float` (PF-14b); `pf14Guard`, `pf14Event`, `pf14CanAct`, `pf14Number` and the `internal/store/tickets.go` `Eng*` functions (PF-14a); `EngSignedFileExists`, `EngUserIsActiveMember` (`engine_lookup.go`, PF-14b).
- New unexported identifiers are prefixed `pf14c`; new test helpers are prefixed `pf14c` and live in `create_update_test.go`.

## Tasks

1. Write the intent snapshot and the pre-edit hashes. Run the baseline Verification (step `baseline`).
2. `internal/store/engine_refs.go`: add the lookups above (Querier-based, `%w` wrapping). Run `go build ./...` afterwards.
3. `forms.go`: per-field-type validation (design rule 3), as a new `pf14c` helper that `pf14NormalizeForm` calls after `pf14ValidateValue`:
   - `text`/`text_long`: string, min/max length, `regexpFormat`
   - `number`/`integer`: numeric string or number, min/max value, `decimalDigit`
   - `checkbox`: string array within `options`
   - `pull_down`/`radio_button`: one of `options`
   - `date`: `YYYY-MM-DD`; `datetime`: `YYYY-MM-DD HH:mm`
   - `master`: an item of the field's master
   - `user`/`team`/`ticket`: existing ids (`ticket` also checks `allowedTicketStatus`)
   - `file`: existing signed ids (`EngSignedFileExists`)
   - `calculation`: stored null

   Errors are `422 validation_failed` with `errors.<fieldCode>`. Unknown or duplicate field references stay `400 invalid_parameter` (already implemented).
4. `create.go`/`update.go`: proxy application (design "Proxy application"). A non-null `proxyClientUserId` requires an active `ProxyApplicant` with `user = client` and `proxyUser = actor`, valid on today's JST date (`kftime.FormatDate(e.clock.Now())`; a `startsOn`/`endsOn` of nil is unbounded) and covering the workflow. A client that is suspended or deactivated is rejected too. Every failure is `422 validation_failed` with `errors.proxyClientUserId`. On success, author = actor and `ProxyClientUserID` = client.
5. `update.go` `pf14UpdateForm`: load the stored rows (`EngTicketInputs`), overlay only the patched fields (by `formFieldId`, else `formFieldCode`), validate, and persist the merged set. References of untouched fields stay. Both `inputs` and `slipItems` empty -> `400 invalid_parameter`.
6. Tests in `create_update_test.go` (below). Compile gate after each write. Final Verification (step `final`). Update Done Criteria and Progress Log.

## Key Points and Pitfalls

- Drafts skip required checks but not type checks (design rule 4). The required check runs only when the ticket is or becomes `in_progress`.
- Approver updates may only include `approver: true` fields, or all fields when `overwritable` (rule 2); validate the raw patch before merging.
- The default fixtures register `ceo` (client) <- `assistant` (proxy) for `expense-claim` only (`fixtures/default/proxy-applicants.json`), and `suspended-user` exists. Build expired, other-workflow and suspended-client cases by updating rows inside the test transaction (`env.apply`). Never edit fixtures.
- The field types present in the default fixtures are `number`, `text`, `text_long`, `date`, `master`, `file`, `pull_down`, `checkbox`, `user`, `team` and `ticket`. Test them through `Engine.Create`. Test `integer`, `radio_button`, `datetime` and `calculation` with a table test that calls your new `pf14c` validation helper on constructed `model.FormField` values.
- For `file`, insert a `files` row in the test DB (look at how PF-23/`internal/store` writes `files`) or use an unknown signed id for the negative case. Never call the HTTP upload API.
- `internal/store` is shared with PF-14a (`tickets.go`) and PF-14b (`engine_lookup.go`) in this wave. Save `engine_refs.go` complete, with all its imports, before any engine file references it.

## Tests (input or situation -> expected)

- `TestFormValidation` (existing): unchanged; PASS.
- `TestFormFieldTypes` subtests:
  - purchase-request `reviewer` = unknown uuid -> 422 `errors.reviewer`; an existing user id -> ok
  - `approving-team` = a team of the inactive chart or an unknown id -> 422; an active-chart team -> ok
  - `related` = an unknown ticket -> 422
  - `vendor`/expense `cost-center` = an item of another master -> 422; an item of the field's master -> ok
  - expense `receipt` = unknown signed id -> 422 `errors.receipt`
  - leave `reason` (pull_down) = a value not in `options` -> 422
  - purchase `urgent` (checkbox) with a value not in `options` -> 422
  - `expense-date` = `2026/10/21` -> 422; `2026-10-21` -> ok
  - table over constructed fields: `integer` `1.5` -> error; `datetime` `2026-10-21 09:30` ok, `2026-10-21` error; `radio_button` outside options -> error; `calculation` any value -> stored null
- `TestProxyApplicantCreate` subtests:
  - `assistant` creates expense-claim with `proxyClientUserId` = ceo -> ok; author = assistant, `ProxyClientUserID` = ceo
  - same for purchase-request (workflow not covered) -> 422 `errors.proxyClientUserId`
  - registration with `endsOn` yesterday -> 422
  - `sales-rep` (no registration) with client ceo -> 422
  - client switched to `suspended` status -> 422
  - Update of a draft setting an invalid client -> 422
- `TestUpdateFormMerge`: create an expense-claim draft with amount, purpose and cost-center; `UpdateForm` with only `purpose` -> amount and the cost-center item id/reference are unchanged; empty patch -> 400 `invalid_parameter`. A partial applicant `Update` -> 400 `invalid_parameter` with stored rows unchanged; a current approver's approver-note-only `Update` preserves applicant values and RefIDs.

## Verification (manifest `verification` array for PF-14c; all must pass)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14c-s465-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/engine/... ./internal/store/... 2>&1 | tee /tmp/pf14c-s465-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/engine/... 2>&1 | tee /tmp/pf14c-s465-test.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v ./internal/store/... 2>&1 | tee /tmp/pf14c-s465-store.log'; echo "exit=$?"
for f in /tmp/pf14c-s465-test.log /tmp/pf14c-s465-store.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
grep -c -E -- '--- PASS: (TestFormValidation|TestFormFieldTypes|TestProxyApplicantCreate|TestUpdateFormMerge) ' /tmp/pf14c-s465-test.log   # 4
test -z "$(gofmt -l internal/engine/forms.go internal/engine/create.go internal/engine/update.go internal/engine/forms_test.go internal/engine/create_update_test.go internal/store/engine_refs.go)"; echo "exit=$?"
```

Evidence: build/vet `exit=0`; tests `exit=0`, FAIL=0, SKIP=0, PASS>0; the named count is 4; gofmt `exit=0`. The whole engine package must pass, including the PF-14a and PF-14b tests. Copy logs to `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-<n>/`. Negative proof (for `authorSelfCheck.checks` only): the enum literal `rg` over `forms.go`/`create.go`/`update.go` is expected to exit 1.

## Done Criteria

- [x] Form validation rules 1-7 hold for every field kind; references validated
- [x] Proxy application checks on Create and Update follow the design; author and proxy client stored correctly
- [x] `UpdateForm` merges partial patches
- [x] The 4 named tests PASS; whole engine package and `./internal/store/...` FAIL=0, SKIP=0
- [x] With `overwritable=false`, approver Update and UpdateForm reject applicant-owned field patches with 400 `invalid_parameter` and unchanged rows; approver-owned fields remain editable, and `overwritable=true` permits the amount patch
- [x] An author who is not the current approver cannot Update or UpdateForm an `in_progress` ticket; both paths return 403 `user_not_assigned` and preserve stored rows
- [x] Frozen signatures unchanged; new identifiers `pf14c`/`Eng` prefixed; gofmt clean; files < 1000 lines; compile gate logged after every write

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14c-s465-<step>.log`, evidence under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

### Session: 2026-10-21 — PF-14c implementation

**Tasks Completed**: field-kind and reference validation; proxy applicant checks on Create/Update; partial UpdateForm field merge including reference IDs; assigned engine/store tests.

**Implementation**:
- `forms.go`: added `pf14cValidateFormValue` for field shape, options, lengths, regex, finite numeric/range/decimal, date/datetime, calculation-null and user/team/ticket/master/file reference checks. NaN and infinities are rejected; file fields validate `Files` signed IDs.
- `create.go`/`update.go`: `proxyClientUserId` requires the actor/client/workflow registration for the current JST date and an activated, non-deactivated client. `UpdateForm` overlays partial fields on stored inputs and reconstructs typed references before revalidation; empty patches return `400 invalid_parameter`.
- `engine_refs.go`: added the owned `Eng`-prefixed existence and proxy-applicant readers. Frozen helper signatures and existing shared results are unchanged.
- `create_update_test.go`: added `TestFormFieldTypes` (including a NaN rejection case), `TestProxyApplicantCreate` and `TestUpdateFormMerge`; existing `TestFormValidation` remains unchanged.

**Incremental compile gates after non-test Go writes** (`go build ./...`; full logs are under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-1/`):
- `build-after-store.log`: exit 0; `build-after-store-correction.log`: exit 0.
- `build-after-forms.log`: exit 1 (initial validator return-order error plus concurrent PF-14b missing symbols); `build-after-forms-fix.log`: exit 1 (PF-14b-owned `routing.go` symbols remained); `build-after-forms-import.log`: exit 1 (PF-14b-owned route symbols remained).
- `build-after-create.log` and `build-after-update.log`: exit 1 while PF-14b `routing.go` was being written; no PF-14c source errors remained.
- `build-check-routes.log`: exit 0 after PF-14b symbols landed; `build-after-status-fix.log`, `build-after-files.log`, `build-after-ref-preservation.log`, `build-after-calc.log`, and `build-after-gofmt.log`: exit 0.

**Test-file vet gates after each `_test.go` write** (`go vet ./internal/engine`): `vet-after-tests.log`, `vet-after-test-format.log`, `vet-after-test-fixes.log`, `vet-after-criteria-tests.log`, `vet-after-reference-test.log`, and `vet-after-calc-test.log`: exit 0.

**Final verification** (final source; commands and complete logs are in the evidence directory):
- `go build ./...`: exit 0, `final-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0, `final-vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=72, FAIL=0, SKIP=0; all four named tests PASS; `final-engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `final-store-test.log`.
- Named-test count command: 4; `final-counts.log`. Gofmt selection check: exit 0; `final-gofmt.log`. Latest-source rerun includes the finite-number regression case.
- `git diff --check` on owned Go source/test paths: exit 0. All owned Go files are below 1000 lines.

**Resolved test attempt**: an earlier test run failed two assertions due to test setup (wrong workflow for file reference, missing calculation field ID, and nonmember proxy test team). Corrections are documented in `prior-test-attempt-summary.md`; the full uncached final-source run above passes. The initial tee log was replaced by the final rerun at the same `/tmp` path.

**Post-edit hashes**: `forms.go` `f3ce5310a19a0ba656c74d95276cdfd589c190a575c5222ec868235d0c6b967a`; `create.go` `ed240be399caaca1f2b12a30638e6b0cfeb8e5b3ebf2b57080a53137558b10e5`; `update.go` `765979b1947b07ec52c6d58ef20f7b231c83d6b2caec5ff657f66858a1aaba38`; `create_update_test.go` `83643dd667585a8f1fca016c15caf9778840f83060e9adfa4838104c64a1fa46`; `engine_refs.go` `df3e2d4620d0bba18f2904e591a49a4084a38cf7cd92fdde7ca6a4abde26ef9f`.


### Session: 2026-10-21 — PF14C-TI-01 test-integrity repair

**Finding addressed**: `PF14C-TI-01` (mid). `TestUpdateFormMerge` now records persisted amount, purpose and cost-center row presence and fails if any are missing. Existing value assertions, the cost-center `RefIDs` assertion, and the empty-patch `invalid_parameter` assertion remain intact. The correction is limited to `internal/engine/create_update_test.go`.

**Attempt-2 verification** (complete logs under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-2/`):
- `go vet ./internal/engine`: exit 0; `vet-after-test-edit-rerun.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `final-vet.log`.
- `go build ./...`: exit 0; `final-build.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=72, FAIL=0, SKIP=0; `final-engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `final-store-test.log`.
- Named test count: 4; `final-named-tests-count.log`. `gofmt` check for `create_update_test.go`: exit 0; `final-gofmt.log`.
- Updated `create_update_test.go` SHA-256: `83643dd667585a8f1fca016c15caf9778840f83060e9adfa4838104c64a1fa46`.

### Session: 2026-10-21 — PF-14c current-source verification rerun

No source or test files changed in this rerun. The repaired `TestUpdateFormMerge` source remains at SHA-256 `83643dd667585a8f1fca016c15caf9778840f83060e9adfa4838104c64a1fa46`; its row-presence checks and prior value, reference-ID, and empty-patch assertions passed again.

**Attempt-3 verification** (complete logs under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-3/`):
- `go build ./...`: exit 0; `build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=72, FAIL=0, SKIP=0; `engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `store-test.log`.
- Required named-test count: 4; `named-test-count.log`. Owned Go files are gofmt-clean; exit 0.
- Independent re-review of PF14C-TI-01 remains a downstream review step; this implementation report does not claim that decision.

### Session: 2026-10-21 — PF14C-ADV-01/-02 adversarial repair

**Findings addressed**: `PF14C-ADV-01` and `PF14C-ADV-02` from `comm-008325`. For current-approver direct Updates with inputs, `pf14Update` now overlays the patch onto stored rows via `pf14cInputsFromRows` and `pf14cMergeInputs`, preserving applicant values and references. Applicant direct Updates with inputs now run `pf14ValidateCreateCompleteness` before normalization or persistence; `Inputs == nil` paths remain unchanged.

**Regression coverage**: `TestUpdateFormMerge/applicant_update_missing_field_is_invalid_parameter` asserts HTTP 400/code `invalid_parameter` and unchanged stored row count, values and RefIDs. `TestUpdateFormMerge/approver_update_keeps_applicant_inputs` resolves an unfinished assignee on the current step dynamically, patches only `approver-note`, and asserts amount, purpose and cost-center values/RefIDs plus the new note.

**Pre-fix reproduction**: the two new subtests failed on the pre-fix production code as expected: applicant Update returned nil instead of `invalid_parameter`, and the approver Update dropped amount, purpose and cost-center. Complete log: `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-4/pre-fix-repro.log` (exit 1; intentional negative proof, not a required verification gate).

**Attempt-4 final-source verification** (complete logs under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-4/`):
- `go build ./...`: exit 0; `build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=79, FAIL=0, SKIP=0; all four named PF-14c tests pass and both new subtests pass; `engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `store-test.log`.
- Named PF-14c test count: 4; applicant and approver regression subtest counts: 1 each. Gofmt selection check: exit 0. `update.go`: 356 lines.
- Required `go vet ./internal/engine` after the test write initially exited 1 on a map-value assignment in the new snapshot helper; the helper was corrected, then the same vet command exited 0. Both logs are preserved as `posttest-vet-initial-fail.log` and `posttest-vet.log`.
- Final source hashes: `update.go` `5166458f8622937a34a096afeb06f18798e446a72c452dba563b3293a6ea2c36`; `create_update_test.go` `76362a89d07df33259d93e9a12746a7c54d62db5ea8317a0b42bcfdce18ece80`.
- Read-only patch review by `/root/pf14c_patch_review` found no concrete correctness gaps. Independent adversarial re-review remains downstream; this progress entry does not claim acceptance.
- A later independent post-modification `go build ./...` rerun exited 1 after sibling artifact `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-2/teams_mutated.go` appeared. The file is outside PF-14c ownership and has undefined `API` references at lines 38, 39, 49, 63, 113, 185, 207, 298, 335 and 368. Full current shared-tree failure log: `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-4/build-current-shared-tree-fail.log`. The earlier attempt-4 build passed before this artifact appeared and is not a current shared-tree pass. Reconciliation owner: PF-19; resume PF-14c verification after the PF-19 owner or integration review resolves/excludes this temporary Go source and rerun `go build ./...` with exit 0.

### Session: 2026-10-21 — PF-14c verification after PF-19 reconciliation

No Go source or test files changed in this attempt. PF-19's accepted repair renamed its generated mutation file to `_teams_mutated.go`, excluding it from Go package discovery. The attempt-4 shared-tree blocker is retained above as historical evidence and is resolved by these current-source gates.

**Attempt-5 verification** (complete logs under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-5/`):
- `go build ./...`: exit 0; `build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=79, FAIL=0, SKIP=0; all four named PF-14c tests PASS; `engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `store-test.log`.
- Named-test count: 4; gofmt selection check: exit 0; all six owned Go files are below 1000 lines; see `named-test-count.log`, `gofmt.log`, and `line-counts.log`.
- `pre-rerun-source.sha256` and `post-rerun-source.sha256` match exactly, confirming the source verified is the source previously reviewed and no source drift occurred during the rerun.
- PF-14c implementation gates are complete. Independent re-review, combined-tree integration review, and PF-27 final gates remain downstream; no acceptance decision is claimed here.

### Session: 2026-10-21 — PF14C-ADV-03/-04 adversarial repair (attempt 6)

**Findings addressed**: `PF14C-ADV-03` and `PF14C-ADV-04` from `comm-008376`.

**Changes**:
- `update.go`: retained the frozen `pf14Update` signature as a delegate to `pf14cUpdate(..., partial bool)`. The in-progress guard now returns the `pf14CanAct` error for an author who is not a current approver. Approver input patches are resolved by ID, then code, and checked against `FormField.Approver` when `WorkflowDef.Overwritable` is false before any write. `UpdateForm` passes its raw input patch to the core; partial applicant patches merge with stored rows, while direct applicant `Update` retains the completeness check. Slip-only partial updates reconstruct references through `pf14cInputsFromRows`.
- `create_update_test.go`: added rejection and no-write assertions for approver amount patches through both Update paths, positive controls for approver-note and overwritable amount patches, and author Update/UpdateForm rejection assertions for `403 user_not_assigned` with unchanged rows.
- The targeted regressions reproduced all four defects against the pre-fix source (exit 1; expected failure) in `pre-fix-repro.log`; the current-source focused run passes all six new cases in `focused-test.log`.

**Attempt-6 final-source verification** (complete logs under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-6/`):
- `go build ./...`: exit 0; `final-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0; `final-vet.log`; the required post-test-edit `go vet ./internal/engine` also exited 0 (`vet-after-tests.log`, `vet-after-assertion.log`).
- `go test -count=1 -v -race ./internal/engine/...`: exit 0; PASS=87, FAIL=0, SKIP=0, zero cached lines; all four named PF-14c tests pass; `final-engine-test.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0; PASS=10, FAIL=0, SKIP=0; `final-store-test.log`.
- Named-test count is 4; gofmt selection check exits 0; `update.go` is 389 lines and `create_update_test.go` is 625 lines. Final SHA-256: `update.go` `8fce1f94659d82709f7da4cf4b914098d37311cafdb6d1a246e65cad88d2d3cc`; `create_update_test.go` `d27123ee26893e130c85290e7a81f128c322ecf71b34344da4488bdb34c3a916`.
- Independent test-integrity and adversarial re-review, combined-tree integration review, and PF-27 final gate remain downstream; this implementation progress does not claim their decisions.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
