# PF-14b Engine Routing and Conditions Implementation Plan

**Status**: Ready
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

- [ ] All operators of "Route resolution" are implemented; existing operator results unchanged
- [ ] Grade targets, `closest`/`all` and all five `FallbackType` values follow INF-19
- [ ] The 4 named tests PASS; whole engine package and `./internal/store/...` FAIL=0, SKIP=0
- [ ] Frozen helper signatures unchanged; new identifiers `pf14b`/`Eng` prefixed; gofmt clean; files < 1000 lines
- [ ] Compile gate logged after every write; progress log complete

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-14). Session values: accepted commit `ea385c7`, logs `/tmp/pf14b-s465-<step>.log`, evidence under `tmp/punchflow-kickflow-dummy-s465/PF-14b/attempt-<n>/`. Manifest: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

## Progress Log

(none yet)
