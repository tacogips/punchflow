# PF-26c Contract Scenarios: Users and Grades Implementation Plan

**Status**: Completed
**Wave**: A (s466; parallel with PF-26a, PF-26b, PF-26d..PF-26f) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26c
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table), operation matrix rows #3, #5-#12, #14, #16-#18, INF-24; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

`internal/contract` validates punchflow against `design-docs/references/kickflow-openapi.yaml`. PF-26c adds live HTTP scenarios for exactly these 13 operationIds:
- users: `createUser`, `deleteUser`, `updateUser`, `lookupUserByEmail`, `reinviteUser`, `suspendUser`, `reactivateUser`, `listUserTeams`, `listUserRoles`
- grades: `createGrade`, `deleteGrade`, `updateGrade`, `setDefaultGrade`

Each scenario checks status, schema, JST timestamps and pagination headers, plus the error format. PF-26c also owns `internal/contract/scenarios_users_org_test.go`, for one mechanical hoist that the PF-27 aggregate coverage test needs.

## Non-goals

- No other operations. `getCurrentUser`, `listUsers`, `getUser`, `listGrades` and `getGrade` stay in `TestContractLiveReadScenarios`.
- In `scenarios_users_org_test.go`, change nothing except the hoist described below. Do not change `TestContractScenarios` or any subtest name or behavior.
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go` or `internal/cli/serve_smoke_test.go`.
- No `t.Skip`, no loosened validator, no special-casing, no `go.mod` edits, no git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26c-contract-users-grades.md`
- `internal/contract/scenarios_users_grades_test.go`
- `internal/contract/scenarios_users_org_test.go`
- `internal/api/v1/users.go`
- `internal/api/v1/users_test.go`
- `internal/api/v1/users_status.go`
- `internal/api/v1/grades.go`
- `internal/api/v1/grades_test.go`
- `internal/present/user.go`
- `internal/store/users.go`
- `internal/store/read_users.go`
- `internal/store/grades.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 12 files. New work goes in the first three. The others are repair-eligible only.

## Deliverable 1: hoist in `internal/contract/scenarios_users_org_test.go`

- Move the local `cases` slice of `TestContractLiveReadScenarios` (22 entries; lines 54-64 at `a2a82de`) to a package-level variable `liveReadScenarios`. Use an anonymous struct element type `struct{ operationID, path string }`; do not add a named type.
- The test then ranges over `liveReadScenarios`. Subtest names, order, persona (`testutil.Admin`), checks and count (22) stay identical.
- Do this first, in a single write, then run `go vet ./internal/contract/...`. Pre/post `shasum -a 256` go in the progress log.

## Deliverable 2: `internal/contract/scenarios_users_grades_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveUsersGradesOperations = []string{...}`: exactly the 13 ids above.
- `func TestContractLiveUsersGrades(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26c`.

Existing code to imitate (read only):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios` (operation map, `responseSchemaURI`, `validateJSON`, `checkTimestamps`).
- `TestContractScenarios` in the same file: the 200-response lookup (`"200"`, then `"200.0"`). No `content` means an empty body.
- `internal/api/v1/users_test.go` and `grades_test.go` for request bodies. Read the request schemas `createUser`, `updateUser`, `createGrade` and `updateGrade` in the spec for required keys.

Local helpers (same behavior in every PF-26 sub-plan):
- `pf26cCheck(t, op, resp)`:
  - status 200;
  - spec 200 without `content` -> empty body;
  - otherwise validate against the 200 schema and run `checkTimestamps`;
  - spec `parameters[].name` includes `perPage` -> `Page`, `Per-Page` and `Total` are non-empty.

  Cache compiled schemas.
- `pf26cCheckError(t, resp, status, code)`:
  - `testutil.AssertError`;
  - validate against `#/components/schemas/ErrorResponse`;
  - keys are a subset of `code`, `message` and `errors`;
  - `errors`, when present, is allowed only for status 422; 422 `invalid_state` may omit field-level errors;
  - no `doc_url`.
- Inventory check: `len == 13`; every id is in `apiv1.Operations()`; record the `t.Run` results and `t.Errorf` for every id without a passing subtest.

## Scenario sequence (one `testutil.New(t)`; sequential; persona Admin unless stated; never `t.Parallel`)

Subtest names are `TestContractLiveUsersGrades/<operationId>`, and error cases are `<operationId>/error_<code>`. User and grade path parameters accept the id or the `code`.

1. `createUser`: POST `/v1/users` with a new email such as `pf26c-new@punchflow.test`, a code such as `pf26c-new`, and the required names -> 200. The status is `invited` (row #3). Keep its id as U.
2. `updateUser`: PATCH `/v1/users/{U}` changing one name field -> 200.
3. `lookupUserByEmail`: GET `/v1/users/lookupByEmail?email=sales-rep@punchflow.test` -> 200. Use the query parameter name from the spec.
4. `reinviteUser`: POST `/v1/users/{U}/reinvite` -> 200 (invited -> invited).
5. `suspendUser`: POST `/v1/users/engineer/suspend` -> 200, status `suspended`. Error `suspendUser/error_invalid_state`: suspend `invited-user` -> 422 `invalid_state` (INF-24).
6. `reactivateUser`: POST `/v1/users/engineer/reactivate` -> 200, status `activated`.
7. `listUserTeams`: GET `/v1/users/sales-rep/teams` -> 200. Link check with `?perPage=1`: if `Total` > 1, assert that `Link` contains `rel="next"`. If `Total` is 1, do the Link check on `listUserTeams` for another user whose `Total` is > 1, and log your choice.
8. `listUserRoles`: GET `/v1/users/admin/roles` -> 200. Error `listUserRoles/error_missing_permission`: as Employee -> 403 `missing_permission`.
9. `deleteUser`: DELETE `/v1/users/{U}` -> 200, schema-valid UserDetail with status `deactivated` (row #5).
10. Error `deleteUser/error_resource_not_found`: DELETE `/v1/users/00000000-0000-4000-8000-000000000000` -> 404 `resource_not_found`.
11. `createGrade`: POST `/v1/grades` with code `pf26c-grade`, an unused level (60), and a name -> 200. Create a second grade, `pf26c-temp` at level 70, as setup. Error `createGrade/error_validation_failed`: level 10 (used by the fixture grade `staff`) -> 422 `validation_failed` (row #14).
12. `updateGrade`: PATCH `/v1/grades/pf26c-grade` changing the name -> 200.
13. `setDefaultGrade`: POST `/v1/grades/pf26c-grade/default` -> 200.
14. `deleteGrade`: DELETE `/v1/grades/pf26c-temp` -> 200 (empty-body rule from the spec).

Pitfalls:
- Fixture users are listed in `fixtures/default/users.json`: `engineer` is activated, `invited-user` is invited, `suspended-user` is suspended.
- Do not delete or suspend fixture users that other assertions in this test rely on. Reactivate `engineer` in step 6.
- Tenant `allowedEmailDomains` is empty, so any domain is accepted. Do not test `domain_not_allowed`; it needs the admin tenant API.

## Defect handling (design "Defect repair and ownership transfer")

1. **Root cause in your writePaths**:
   - Make the smallest fix. Change same-basename tests only to match corrected behavior.
   - Run the compile gate.
   - Run the package guard `go test -count=1 -v -race ./internal/<pkg>/...`.
   - Log a transfer candidate `{path, fromPlan, toPlan: "PF-26c", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in the s465 manifest: `ownershipTransfers[]`, then `plans[].writePaths`, then s460/s462.
2. **Eligible for a sibling** (for example `internal/present/org.go` belongs to PF-26d): reconciliation request to that sibling.
3. **Other file**: reconciliation request with `file:line`, subtest and log path; report `implementationIncomplete: true`.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26c-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^(TestContractLiveUsersGrades|TestContractLiveReadScenarios)$' ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26c-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26c-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26c-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveUsersGrades/' /tmp/pf26c-s466-contract.log)" -ge 17; echo "exit=$?"   # 13 operations + 4 error subtests
test "$(grep -c -- '--- PASS: TestContractLiveReadScenarios/' /tmp/pf26c-s466-contract.log)" -eq 22; echo "exit=$?"   # hoist kept all 22
test -z "$(gofmt -l internal/contract/scenarios_users_grades_test.go internal/contract/scenarios_users_org_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_users_grades_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required for each package with an edited production file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26c-s466-guard-<pkg>.log'; echo "exit=$?"`, with counts, plus `test -z "$(gofmt -l <edited files>)"`.

## Done Criteria

- [x] `liveReadScenarios` exists, and `TestContractLiveReadScenarios` still passes 22 subtests with unchanged names.
- [x] `TestContractLiveUsersGrades` passes with 13 operation subtests and 4 error subtests. The inventory check passes.
- [x] Every final-source verification line shows exit 0, with log path and counts. FAIL=0, SKIP=0.
- [x] No production edits or transfer candidates were needed; no other defects require reconciliation.
- [x] `go build ./...` exits 0 at the end of the final attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16 (fresh reads and hashes, intent snapshot, drift detection, compile gate, prefix `pf26c`). The evidence rule:
1. `verification` holds only must-pass commands.
2. Remaining work goes in `implementationIncomplete: true`, `implPlanUpdates` and this log.
3. High/Medium only for delivered-code defects, with `file:line`.
4. Every attempt ends with `go build ./...` exit 0.

## Progress Log

### Session: 2026-10-21 (s466; PF-26c Step 6)
**Intent snapshot**: `internal/contract/scenarios_users_org_test.go` pre-hoist SHA-256 `ddbf7a56312fce359c46e14caa024d9400a35f5a44427185e4a90e4b4378b49a`; post-hoist/gofmt SHA-256 recorded in `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-3/scenarios_users_org_test.post-hoist.sha256`. Node-boundary source snapshot: `/Users/taco/gits/tacogips/punchflow/tmp/riela-fanout/512785D1-6398-4B27-A55D-950F5C6E3D40/1DCB3CDE-8C26-4310-93CA-CD1DF418DA33.json`. Per-edit intents and source snapshots are under `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-1/` and `attempt-3/`.
**Tasks Completed**: Hoisted all 22 existing live-read cases to `liveReadScenarios`, preserving order and names. Added schema-validated live scenarios for all 13 users/grades operations and four required error cases, including pagination headers, one `Link rel="next"` check, JST timestamps, empty-body handling, ErrorResponse validation, and operation inventory self-check.
**Verification (final source)**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26c-s466-build.log'` — exit 0; complete log copied to `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-3/build.log`.
- `bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-vet.log'` — exit 0; complete log copied to `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-3/vet.log`.
- `bash -o pipefail -c "go test -count=1 -v -race -run '^(TestContractLiveUsersGrades|TestContractLiveReadScenarios)$' ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-contract.log"` — exit 0; 41 PASS lines, 0 FAIL, 0 SKIP; 17 users/grades subtests and 22 live-read subtests; complete log copied to `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-3/contract.log`.
- Count/format/size guards: `PASS=41 FAIL=0 SKIP=0`; users/grades subtest count >=17; live-read subtest count =22; `gofmt -l` empty; scenario file 271 lines (<1000). All exited 0.
**Resolved earlier attempts**: Attempt 1 `go build ./...` exited 1 because pre-edit evidence snapshots used a `.go` suffix and were discovered as sources; preserved snapshots were renamed `.txt`, and final build passed. Attempt 2 contract run exited 1 because a 422 `invalid_state` response correctly omitted field-level `errors`; clarified the helper to enforce the accepted one-way rule “errors only on 422” (it does not require `errors` on every 422), consistent with the dispatch acceptance and OpenAPI ErrorResponse. The final uncached contract run passes.
**Transfer candidates**: None; no production paths edited.
**Reconciliation requests**: None.
**Downstream pending**: Formal integration review and any review-dependent finalization remain with later workflow steps.

### Session: 2026-10-21 (s466; PF-26c Step 6 verification rerun)
**Tasks Completed**: Revalidated the existing PF-26c implementation on the current source. No source changes were needed. Logs are complete under `tmp/punchflow-kickflow-dummy-s466/PF-26c/attempt-4/`.
**Verification (current source)**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26c-s466-step6-rerun-build.log'` — exit 0; log copied to `attempt-4/build.log`.
- `bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-step6-rerun-vet.log'` — exit 0; log copied to `attempt-4/vet.log`.
- `bash -o pipefail -c "go test -count=1 -v -race -run '^(TestContractLiveUsersGrades|TestContractLiveReadScenarios)$' ./internal/contract/... 2>&1 | tee /tmp/pf26c-s466-step6-rerun-contract.log"` — exit 0; 41 PASS, 0 FAIL, 0 SKIP; 17 users/grades subtests and 22 live-read subtests; log copied to `attempt-4/contract.log`.
- `gofmt -l internal/contract/scenarios_users_grades_test.go internal/contract/scenarios_users_org_test.go` — empty output; scenario file is 271 lines (<1000).
**Downstream pending**: Formal integration review and PF-27 finalization remain owned by later workflow steps.
- Final assertion guards (`/tmp/pf26c-s466-step6-rerun-guards.log`, copied to `attempt-4/guards.log`) — exit 0; users/grades PASS subtests >=17, read PASS subtests =22, FAIL=0, SKIP=0, gofmt clean, and scenario file <1000 lines (271).
