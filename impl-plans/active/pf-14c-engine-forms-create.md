# PF-14c Engine Forms, Create and Update Implementation Plan

**Status**: Ready
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
- Approver updates may only include `approver: true` fields, or all fields when `overwritable` (rule 2). Keep the current behavior.
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
- `TestUpdateFormMerge`: create an expense-claim draft with amount, purpose and cost-center; `UpdateForm` with only `purpose` -> amount and the cost-center item id/reference are unchanged; empty patch -> 400 `invalid_parameter`

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

- [ ] Form validation rules 1-7 hold for every field kind; references validated
- [ ] Proxy application checks on Create and Update follow the design; author and proxy client stored correctly
- [ ] `UpdateForm` merges partial patches
- [ ] The 4 named tests PASS; whole engine package and `./internal/store/...` FAIL=0, SKIP=0
- [ ] Frozen signatures unchanged; new identifiers `pf14c`/`Eng` prefixed; gofmt clean; files < 1000 lines; compile gate logged after every write

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14c-s465-<step>.log`, evidence under `tmp/punchflow-kickflow-dummy-s465/PF-14c/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

(none yet)
