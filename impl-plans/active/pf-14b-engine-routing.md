# PF-14b Engine Routing and Conditions Implementation Plan

**Status**: Completed
**Wave**: A (s465) | **Depends On**: PF-06, PF-07, PF-08, PF-10 (all accepted at `ea385c7`) | **Plan ID**: PF-14b
**Design Reference**: design-docs/specs/design-approval-engine.md "Route resolution", "Plan ownership (session 465)"; design-docs/specs/notes.md INF-13, INF-14, INF-17..INF-20, CF-07; design-docs/specs/architecture.md "Plan split, evidence-reporting and wave rule (session 465)"
**Supersedes (in part)**: impl-plans/active/pf-14-engine.md
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

PF-14b finishes route resolution so that tickets get the right approvers for every route in the default fixtures and for the condition operators in the design. It covers branch selection, step conditions, the condition operators, assignee resolution for every `stepType`, fallbacks, and the condition matching that section visibility uses.

State at `ea385c7` (from `internal/engine/conditions.go` and `routing.go`):
- `pf14Compare` (`conditions.go:68`) supports only `equal`, `not_equal`, `contains`, the four numeric comparisons, `is_true` and `is_false`.
- These operators are missing: `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` and `not_descendants_or_equal`.
- Missing field-key semantics: `author_grade` (compare by grade `level`), `author_team` (identity or ancestry), `checkbox_variable` (membership), `general_master_variable` (master item id).
- `pf14ResolveAssignees` (`routing.go:123`) has these gaps:
  - `team`/`dynamic_team`/`manager` targets ignore `GradeSymbol`/`GradeCodes`.
  - `manager` walks up but does not apply `FallbackType` (`direct_manager`/`higher_manager`/`higher_team`/`skip`/`no_fallback`, INF-19).
  - The `closest` rule "if the only leader is the author, use the parent" is not exact.
- `TestPurchaseConditionsAndNomination` and `TestLeaveAssigneeCustomizableAndAuthorConfirm` already pass. Keep them passing.

## Non-goals

- Lifecycle actions, progression and comments (PF-14a). Forms, Create/Update and proxy applicants (PF-14c).
- `combinationType: custom` and `fieldKey: other_variable` (rejected at fixture load, INF-18). Formula evaluation (INF-21).
- No changes to `internal/model/*`, fixtures or any other store file.

## Path Declarations

### writePaths
- `impl-plans/active/pf-14b-engine-routing.md`
- `internal/engine/routing.go`
- `internal/engine/conditions.go`
- `internal/engine/routing_test.go`
- `internal/engine/conditions_test.go`
- `internal/store/engine_lookup.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts (frozen; design "Plan ownership (session 465)")

- Owned helpers that siblings call. Their signatures stay fixed, and so do their results for inputs that work today:
  - `pf14BuildRoute(ctx, tx, ticket *model.Ticket, def model.WorkflowDef, inputs []FieldInput, approvers []ApproverInput, nowTime int64) error` (called by create.go and update.go)
  - `pf14Contains(values []string, want string) bool` (comments.go, visibility.go)
  - `pf14InputValues(def, inputs) map[string]any`, `pf14SectionVisible(section, values) bool`, `pf14FieldValue(input FieldInput) (any, []string, error)`, `pf14Float(v any) (bool, float64)` (forms.go)
  - `pf14ConditionMatch(fields []model.ConditionField, combination string, values map[string]any) bool`
- `internal/store/engine_lookup.go` exports `EngTenantMode`, `EngActiveMemberships`, `EngTeamAncestors`, `EngLeadersOfTeam`, `EngUsersByTeam`, `EngActiveProxyApprovers`, `EngSignedFileExists` and `EngUserIsActiveMember`. They are unchanged in signature and behavior. New lookups get new `Eng`-prefixed names in this file (for example `EngGradeLevel(ctx, q, gradeID string) (int, error)`, `EngMembershipGrades(ctx, q, userID, teamID string) ([]string, error)`). Check the existing functions first and reuse them where they already fit.
- New unexported helpers are prefixed `pf14b` (for example `pf14bGradeMatches`, `pf14bApplyFallback`). New test helpers are prefixed `pf14b` and live in `conditions_test.go` or `routing_test.go`.
- Read-only for this plan: `actions.go`, `comments.go`, `visibility.go`, `numbering.go`, `engine.go`, `setup_test.go`, `lifecycle_test.go`, `events_test.go`, `comments_test.go`, `internal/store/tickets.go` (PF-14a); `forms.go`, `create.go`, `update.go`, `forms_test.go`, `internal/store/engine_refs.go` (PF-14c).

## Tasks

1. Write the intent snapshot and the pre-edit hashes. Run the baseline Verification (step `baseline`).
2. `conditions.go`: add the missing operators to `pf14Compare` without changing the existing cases:
   - `include`/`exclude`: membership of the expected value in an array actual value. A scalar actual is treated as a one-element array.
   - `is_empty`/`is_not_empty`: nil, `""` or an empty array counts as empty.
   - Numbers compare numerically through `pf14Float`. Strings compare exactly.
3. Field-key semantics in route step conditions (`StepConditionFieldKey*`), resolved inside `routing.go` before calling the matcher:
   - `author_grade`: compare grade `level` through a new helper; never change `pf14Float`.
   - `author_team`: identity, or ancestry for `descendants_or_equal`/`not_descendants_or_equal`, using `EngTeamAncestors`.
   - `checkbox_variable`: membership.
   - `general_master_variable`: master item id.
   - `text_variable`/`number_variable`: read the form field whose code is `variable`.
4. Grade targets: filter the members of `team`, `dynamic_team` and `manager` targets by `GradeSymbol`/`GradeCodes` (the `model.GradeSymbol*` constants; `nil` means no filter; `any_of` means the membership grade is in `GradeCodes`; the comparisons use grade `level`).
5. Manager resolution and fallbacks (INF-19): `closest` vs `all`, then the step's `FallbackType` when nothing is left after the self-approval and grade filters:
   - `direct_manager`/`higher_manager`: walk up the ancestry.
   - `higher_team`: try the parent team's members.
   - `skip`: the step becomes `skipped`, with `Fallback=true` and `FallbackResult` set to the fallback value.
   - `no_fallback`: `422 validation_failed` `errors.base`.
6. Add `TestStepConditionAndFallback` (`routing_test.go`) and `TestConditionOperators` (`conditions_test.go`), with the compile gate after each write. Final Verification (step `final`). Update Done Criteria and Progress Log.

## Key Points and Pitfalls

- `pf14ConditionMatch` is also used for section visibility (PF-14c's `forms.go`). An operator that works today must return the same result. Add cases; do not rewrite the switch semantics. In particular, keep `equal` accepting both `reflect.DeepEqual` and string equality.
- Use only the caller's `tx`. Look the active chart up once per call (`store.ActiveOrgChartID`).
- A non-skipped resolved step with zero assignees fails submit with `422 validation_failed` `errors.base`, except `assignee_customizable` (INF-20). Do not move that check into another plan's file.
- Self-approval filtering (`AllowSelfApproval=false`) happens before the fallback decision.
- Never edit fixtures to make a test pass. Build the edge cases inside the tests with in-memory `model.RouteStep`/`model.ConditionField` values, or by updating rows in the test DB inside `env.apply`. Use the `pf14Setup` personas: `ceo`, `sales-manager`, `sales-rep`, `eng-manager`, `engineer`, `hr-manager`, ...
- `internal/store` is shared with PF-14a (`tickets.go`) and PF-14c (`engine_refs.go`) in this wave. Use `Eng` prefixes, and keep imports complete before saving.

## Tests (input or situation -> expected)

- `TestPurchaseConditionsAndNomination`, `TestLeaveAssigneeCustomizableAndAuthorConfirm`: unchanged; still PASS.
- `TestStepConditionAndFallback` subtests:
  - a `conditional_skip` step whose condition holds -> ticket step `skipped`, `skip=true`
  - a `manager` step where the author is the only leader of the author team -> the parent team's leader is assigned
  - `manager` with `FallbackType=higher_manager` and no matching-grade leader in the author team -> an ancestor leader with a matching grade is assigned
  - `manager` with `FallbackType=skip` and no candidate -> step `skipped`, `fallback=true`, `fallbackResult="skip"`
  - `manager` with `FallbackType=no_fallback` and no candidate -> Create with `in_progress` returns `422 validation_failed` with `errors.base`, and nothing is persisted
  - a `team` target with `descendants: true` -> members of child teams are included; with `false` they are not
- `TestConditionOperators`: a table test over `pf14ConditionMatch`, one row per operator and value kind:
  - number: `greater_than` 60000 > 50000 -> true; string `"60000"` vs number 50000 -> true
  - string: `equal`/`not_equal`
  - checkbox: `include` `["yes"]` contains `"yes"` -> true; `exclude` -> false
  - empty: `is_empty` on nil, `""` and `[]` -> true; `is_not_empty` on `"x"` -> true
  - combination: `all` vs `any`
  - unknown operator -> false
  - grade `level` and team ancestry: covered through routing with a constructed step condition (`author_grade greater_than_or_equal`, `author_team descendants_or_equal`)

## Verification (manifest `verification` array for PF-14b; all must pass)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf14b-s465-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/engine/... ./internal/store/... 2>&1 | tee /tmp/pf14b-s465-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/engine/... 2>&1 | tee /tmp/pf14b-s465-test.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v ./internal/store/... 2>&1 | tee /tmp/pf14b-s465-store.log'; echo "exit=$?"
for f in /tmp/pf14b-s465-test.log /tmp/pf14b-s465-store.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
grep -c -E -- '--- PASS: (TestPurchaseConditionsAndNomination|TestLeaveAssigneeCustomizableAndAuthorConfirm|TestStepConditionAndFallback|TestConditionOperators) ' /tmp/pf14b-s465-test.log   # 4
test -z "$(gofmt -l internal/engine/routing.go internal/engine/conditions.go internal/engine/routing_test.go internal/engine/conditions_test.go internal/store/engine_lookup.go)"; echo "exit=$?"
```

Evidence: build/vet `exit=0`; tests `exit=0`, FAIL=0, SKIP=0, PASS>0; the named count is 4; gofmt `exit=0`. The whole engine package must pass, including PF-14a's 14 tests and PF-14c's tests (behavior-preservation guard). Copy logs to `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-<n>/`. Negative proof for `authorSelfCheck.checks` (not verification): the enum literal `rg` over `routing.go`/`conditions.go` (production code uses `model` constants) is expected to exit 1.

## Done Criteria

- [x] All operators of "Route resolution" are implemented; existing operator results unchanged
- [x] Grade targets, `closest`/`all` and all five `FallbackType` values follow INF-19
- [x] The 4 named tests PASS; whole engine package and `./internal/store/...` FAIL=0, SKIP=0
- [x] Frozen helper signatures unchanged; new identifiers `pf14b`/`Eng` prefixed; gofmt clean; files < 1000 lines
- [x] Compile gate results and final verification recorded in the progress log

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14b-s465-<step>.log`, evidence under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

### Session 465, attempt 1

**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/intent-snapshot.txt`. Pre-edit hashes: `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/pre-edit-hashes.txt`. Post-edit hashes: `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/post-edit-hashes.txt`.

**Implemented**:
- Added `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` and `not_descendants_or_equal` comparisons while retaining existing comparison cases.
- Route step conditions now resolve author grade levels, author team identity/ancestry, field-code variables, checkbox membership and master item IDs. A matching `conditional_skip` marks the step skipped.
- Team/dynamic-team and manager targets filter by membership grade level; manager `closest`/`all`, self-approval filtering and manager/team fallback paths are resolved. Missing non-skipped assignees fail through `errors.base`; `skip` stores fallback metadata.
- Fallback candidates also honor `allowSelfApproval`; the higher-team regression verifies that the author is excluded when self-approval is disabled.
- Added `TestConditionOperators` and expanded `TestStepConditionAndFallback`, including persisted skip/fallback outcomes and no-persistence on `no_fallback` failure.
- Frozen helper signatures and `internal/store/engine_lookup.go` were not changed.

**Compile gates**:
- `go build ./...` after the conditions implementation: exit 0, `/tmp/pf14b-s465-gate-conditions.log`.
- Routing compile gate first reported an undefined local `er`; corrected before further work. The corrected `go build ./...` exited 0, `/tmp/pf14b-s465-gate-routing-fix.log`.
- Routing tests vet first reported a Querier/Tx mismatch; corrected before final checks. The subsequent `go build ./...` and `go vet ./internal/engine/...` exited 0, `/tmp/pf14b-s465-gate-routing-fix2-build.log`, `/tmp/pf14b-s465-gate-routing-fix2-vet.log`.
- Subsequent fallback implementation `go build ./...` exited 0, `/tmp/pf14b-s465-gate-fallback-build.log` and `/tmp/pf14b-s465-gate-manager-fallback-build.log`; routing test writes passed `go vet ./internal/engine/...`, `/tmp/pf14b-s465-gate-routing-integration-vet.log`, `/tmp/pf14b-s465-gate-manager-fallback-vet.log`.

**Final verification, current source**:
- `go build ./...`: exit 0, complete log `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/logs/pf14b-s465-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0, complete log `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/logs/pf14b-s465-vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0, PASS=72, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/logs/pf14b-s465-test.log`. The four required tests pass (count=4).
- `go test -count=1 -v ./internal/store/...`: exit 0, PASS=10, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/logs/pf14b-s465-store.log`.
- `gofmt -l` on the five owned Go files returned no paths (exit 0); all files are below 1000 lines.
- The required negative literal proof found no fallback enum literals in `routing.go`; `rg` exit 1 is recorded in `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-1/logs/pf14b-s465-negative-fallback-literals.log`.

**Resolved earlier attempts**: baseline `go build ./...` passed and baseline engine tests were PASS=24, FAIL=0, SKIP=0 (`logs/pf14b-s465-baseline-build.log`, `logs/pf14b-s465-baseline-engine.log`). An early focused run exposed a fixture assumption in the descendant membership test; the assertion was corrected to match the canonical fixture membership, then the focused regressions and final full gates passed. An earlier route compile failure and vet failure are retained under `logs/` and were corrected as recorded above.

**Progress**: all PF-14b implementation and assigned verification criteria are complete. Downstream formal review and integration acceptance remain owned by later workflow steps.

**Final rerun after self-approval fallback correction**: build exit 0, vet exit 0, engine PASS=72/FAIL=0/SKIP=0, store PASS=10/FAIL=0/SKIP=0, required named tests PASS=4, gofmt exit 0. Complete current-source logs are the paths listed above and were refreshed after the correction.

### Session 465, review repair attempt 2 — PF14B-TI-01

**Finding addressed**: `TestStepConditionAndFallback` now covers manager grade walk-up from `sales-east`. A closest `>= manager` target returns exactly `sales-manager` and excludes `sales-deputy`; `>= executive` returns exactly `ceo`. `higher_manager` and `direct_manager` fallbacks with the `>= manager` target each return exactly `sales-manager` and exclude `sales-deputy`.

**Post-write compile gate**: `go vet ./internal/engine/...` exit 0, complete log `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-2/pf14b-s465-a2-manager-vet.log`.

**Refreshed verification**: `go build ./...` exit 0; `go vet ./internal/engine/... ./internal/store/...` exit 0; uncached `go test -count=1 -v -race ./internal/engine/...` exit 0 with PASS=72/FAIL=0/SKIP=0; uncached `go test -count=1 -v ./internal/store/...` exit 0 with PASS=10/FAIL=0/SKIP=0; named-test count=4; gofmt exit 0. Complete logs, count summary, and hashes are under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-2/`.

**Review disposition**: PF14B-TI-01 is addressed in implementation and regression coverage; independent re-review remains pending.

### Session 465, adversarial repair attempt 3 — PF14B-ADV-01

**Finding addressed**: `pf14ResolveAssignees` now builds the root-plus-descendant team ID set with `pf14bTargetTeamIDs` when a team/dynamic-team target has `descendants=true` and a grade filter. A candidate passes when any active membership in that target subtree satisfies the existing grade matcher. `TestStepConditionAndFallback` creates two users whose only membership is in `platform`: the manager-grade user is included and the staff-grade user is excluded. Exact results are asserted for descendants=true (`eng-manager` plus the qualifying child-only user) and descendants=false (`eng-manager` only). Manager and fallback assertions from PF14B-TI-01 remain unchanged.

**Compile gates**: `go build ./...` after the routing implementation write exited 0 (`tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-3/pf14b-s465-a3-build.log`). `go vet ./internal/engine/...` after the test write exited 0 (`tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-3/pf14b-s465-a3-posttest-vet.log`).

**Refreshed verification, current source**: `go build ./...` exit 0; `go vet ./internal/engine/... ./internal/store/...` exit 0; uncached `go test -count=1 -v -race ./internal/engine/...` exit 0, PASS=72/FAIL=0/SKIP=0; uncached `go test -count=1 -v ./internal/store/...` exit 0, PASS=10/FAIL=0/SKIP=0; the four named tests pass; `gofmt -l` is empty; `git diff --quiet HEAD -- internal/store/engine_lookup.go` exits 0. Complete logs, counts and post-edit hashes are under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-3/`. `internal/engine/routing.go` is 541 lines.

**Review disposition**: PF14B-ADV-01 is addressed in code and regression coverage. Independent adversarial re-review remains pending.

### Session 465, integration repair attempt 4 — INT465-PF14B-01

**Finding repair**: `internal/engine/routing.go` now checks author-customizable nominee count only when the route condition has not already skipped the step. The minimum defaults to 1, reads `MinCustomAssignees` when configured, returns the existing `validation_failed` `errors.base` message when below minimum, and marks a zero-nominee/minimum-zero step skipped. `internal/engine/routing_test.go` adds four cases: matching conditional skip with no nominees, minimum zero with no nominees, minimum two with one nominee, and the default minimum-one regression. Successful cases assert `status=skipped`, `skip=true`, and no assignees; error cases assert HTTP 422, code, exact field/message, and no persisted ticket.

**Compile/test gate blocker**: after the source write, `go build ./...` exited 0 (`attempt-4/build-routing.log`). After the test write, `go vet ./internal/engine/...` exited 0 (`attempt-4/vet-after-test-write.log`) and gofmt was clean. The final `go build ./...` exited 1 because the untracked, PF-21-owned `internal/store/roles.go:68` references undefined `boolInt`; full log: `attempt-4/build-final.log`. This is foreign breakage per Shared Execution Protocol; PF-14b did not edit that path. The engine/store final behavioral suites and the four named test count are pending until PF-21 repairs its declared path and a fresh shared-tree build passes.

**Current attempt hashes**: pre-edit `attempt-4/pre-edit-hashes.txt`; pre-plan-update `attempt-4/pre-plan-hashes.txt`. Per-edit intent records are `attempt-4/intent-routing.txt`, `intent-routing-tests.txt`, `intent-routing-tests-cleanup.txt`, and `intent-plan-update.txt`.

**Attempt-5 repair follow-through**: The skip-aware author-customizable branch now also avoids inserting assignee rows when a route condition has skipped that step. Other step types retain their existing assignee insertion behavior. `TestStepConditionAndFallback/author_customizable_minimum_assignees/conditional_skip_omits_nominees` supplies a nominee while matching the skip condition and verifies that the stored skipped step has no assignees. The compile gate after the routing write and vet gate after the test write both passed; intents and logs are preserved in `attempt-5/`.

**Status**: The attempt-4 PF-21 compile blocker cleared in the shared tree. Attempt-5 final-source verification passes: `go build ./...` exit 0; `go vet ./internal/engine/... ./internal/store/...` exit 0; uncached race engine suite PASS=80/FAIL=0/SKIP=0; uncached store suite PASS=10/FAIL=0/SKIP=0; all four named PF-14b tests PASS; gofmt is clean; every owned Go file is under 1000 lines. Logs and counts are under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-5/`. Attempt-4 failure remains documented as historical evidence. Implementation is complete; independent review and combined-tree integration acceptance remain downstream.

**Attempt-5 verification details**:
- `go build ./...`: exit 0, `attempt-5/final-build.log`.
- `go vet ./internal/engine/... ./internal/store/...`: exit 0, `attempt-5/final-vet.log`.
- `go test -count=1 -v -race ./internal/engine/...`: exit 0, PASS=80/FAIL=0/SKIP=0, `attempt-5/final-engine.log`.
- `go test -count=1 -v ./internal/store/...`: exit 0, PASS=10/FAIL=0/SKIP=0, `attempt-5/final-store.log`.
- Four required named test count: 4, `attempt-5/final-named-count.txt`.
- `gofmt -l` returned no paths; file line counts and source hashes are recorded with attempt-5 evidence.

### Session 465, integration repair attempt 6 — INT465-PF14B-02

**Finding repair**: `pf14InputValues` now derives field codes from `def.SectionList` and adds the code-keyed value for ID-only inputs, while retaining the existing ID and code assignments and skipping unknown IDs. Explicit non-empty `FormFieldCode` values retain precedence regardless of input order. Added `TestInputValuesResolveIDOnlyInputs` for ID and code mapping, exact code-only map preservation (including `byID[""]`), and explicit-code precedence. Added a `TestStepConditionAndFallback` case that creates equivalent code-keyed and ID-only in-progress expense claims, proves the numeric amount condition skips the same step, and checks an empty ID-only purpose still returns `422 validation_failed` under `errors[purpose]` without persisting a ticket.

**Per-write gates**: `go build ./...` after `conditions.go` exited 0 (`attempt-6/build-after-conditions-write.log`). `go vet ./internal/engine/...` after the test writes exited 0 (`attempt-6/vet-after-conditions-test-fix.log`, `vet-after-routing-test-fix2.log`); initial vet attempts exposed only test edit issues (missing `reflect` import, undefined model symbol, and pointer assumption for `Error.Errors`) and are retained in `attempt-6/`.

**Current-source verification**: `go build ./...` exit 0; `go vet ./internal/engine/... ./internal/store/...` exit 0; uncached engine race suite PASS=89/FAIL=0/SKIP=0; uncached store suite PASS=10/FAIL=0/SKIP=0; PF-18/PF-23 API guard suite PASS=16/FAIL=0/SKIP=0; focused ID-only regressions PASS=12/FAIL=0/SKIP=0; the four named PF-14b tests PASS=4; `gofmt -l` is empty. Full logs and exact counts are under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-6/`. `forms.go` and `create.go` retain the required hashes `f3ce5310...` and `ed240be3...`; untouched `update.go` hash is `8fce1f94...`. All changed Go files remain below 1000 lines.

**Status**: INT465-PF14B-02 is implemented with current-source behavioral coverage. Independent review and combined-tree integration acceptance remain downstream.
