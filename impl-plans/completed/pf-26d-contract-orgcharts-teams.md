# PF-26d Contract Scenarios: Organization Charts and Teams Implementation Plan

**Status**: Completed
**Wave**: A (s466; parallel with PF-26a..PF-26c, PF-26e, PF-26f) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26d
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table), operation matrix rows #28-#31 and #33-#42, INF-23; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

`internal/contract` validates punchflow against `design-docs/references/kickflow-openapi.yaml`. PF-26d adds live HTTP scenarios for exactly these 14 operationIds:
- org charts: `createOrganizationChart`, `deleteOrganizationChart`, `getOrganizationChart`, `updateOrganizationChart`, `activateOrganizationChart`
- teams: `listTeams`, `createTeam`, `getTeam`, `updateTeam`, `deleteTeam`, `listTeamMembers`, `createTeamMembers`, `deleteTeamMembers`, `updateTeamMember`

Each scenario checks status, schema, JST timestamps and pagination headers, plus the error format. Defects in org/team production code are fixed here, within the repair-eligible files.

## Non-goals

- No other operations. `listOrganizationCharts` and `getCurrentOrganizationChart` stay in `TestContractLiveReadScenarios`.
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `scenarios_users_org_test.go` or `internal/cli/serve_smoke_test.go`.
- No `t.Skip`, no loosened validator, no special-casing, no `go.mod` edits, no git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26d-contract-orgcharts-teams.md`
- `internal/contract/scenarios_orgcharts_teams_test.go`
- `internal/api/v1/orgcharts.go`
- `internal/api/v1/orgcharts_test.go`
- `internal/api/v1/teams.go`
- `internal/api/v1/teams_test.go`
- `internal/present/org.go`
- `internal/store/org.go`
- `internal/store/read_org.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 9 files. New work goes in the first two. The rest are repair-eligible only.

## Deliverable: `internal/contract/scenarios_orgcharts_teams_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveOrgChartsTeamsOperations = []string{...}`: exactly the 14 ids above.
- `func TestContractLiveOrgChartsTeams(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26d`.

Existing code to imitate (read only):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios` (operation map, `responseSchemaURI`, `validateJSON`, `checkTimestamps`).
- `TestContractScenarios` in the same file: the 200-response lookup (`"200"`, then `"200.0"`). No `content` means an empty body.
- `internal/api/v1/orgcharts_test.go` and `teams_test.go` for request bodies, in particular the membership bodies (members with `userId` and `leader`) and the JSON body on `DELETE .../memberships`. Use the spec request schemas as the authority for key names.

Local helpers (same behavior in every PF-26 sub-plan):
- `pf26dCheck(t, op, resp)`:
  - status 200;
  - spec 200 without `content` -> empty body;
  - otherwise validate against the 200 schema and run `checkTimestamps`;
  - spec `parameters[].name` includes `perPage` -> `Page`, `Per-Page` and `Total` are non-empty.

  Cache compiled schemas.
- `pf26dCheckError(t, resp, status, code)`:
  - `testutil.AssertError`;
  - validate against `#/components/schemas/ErrorResponse`;
  - keys are a subset of `code`, `message` and `errors`;
  - `errors` is present if and only if status is 422;
  - no `doc_url`.
- Inventory check: `len == 14`; every id is in `apiv1.Operations()`; record the `t.Run` results and `t.Errorf` for every id without a passing subtest.

## Scenario sequence (one `testutil.New(t)`; persona Admin; sequential; never `t.Parallel`)

Subtest names are `TestContractLiveOrgChartsTeams/<operationId>`, and error cases are `<operationId>/error_<code>`. Org chart path parameters are ids. Team path parameters accept the id or the `code`.

1. `createOrganizationChart`: POST `/v1/organizationCharts` with a name -> 200, inactive (row #28). Keep it as C1. Create C2 as setup (it will be deleted).
2. `getOrganizationChart`: GET `/v1/organizationCharts/{C1}` -> 200.
3. `updateOrganizationChart`: PATCH C1 with a new name -> 200.
4. `createTeam`: POST `/v1/organizationCharts/{C1}/teams` with a name and code `pf26d-root` -> 200. Create the child `pf26d-child` with `parentId` = root id as setup.
5. `getTeam`: GET `/v1/organizationCharts/{C1}/teams/pf26d-root` -> 200.
6. `listTeams`: GET `/v1/organizationCharts/{C1}/teams` -> 200 (roots; the root team is listed).
7. `createTeamMembers`: POST `.../teams/pf26d-root/memberships` adding `sales-manager` (leader true), `sales-rep` (leader false) and `sales-deputy` (leader false) -> 200. Error `createTeamMembers/error_validation_failed`: add only a non-leader member to `pf26d-child`, which has no leader -> 422 `validation_failed` (row #40, leaderless team).
8. `listTeamMembers`: GET `.../teams/pf26d-root/memberships` -> 200. Link check with `?perPage=1` (3 members): `Link` contains `rel="next"`.
9. `updateTeamMember`: PATCH `.../teams/pf26d-root/memberships/{sales-rep id}` with leader true -> 200.
10. `deleteTeamMembers`: DELETE `.../teams/pf26d-root/memberships` with a JSON body naming `sales-manager` -> 200 (sales-rep is now the leader and sales-deputy remains). Error `deleteTeamMembers/error_validation_failed`: remove `sales-rep`, the last leader, while non-leader `sales-deputy` remains -> 422 `validation_failed` (row #41, leaderless remainder).
11. `updateTeam`: PATCH `.../teams/pf26d-child` with a new name -> 200. Error `updateTeam/error_validation_failed`: PATCH `pf26d-root` with `parentId` = child id (a cycle) -> 422 (row #37).
12. `deleteTeam`: DELETE `.../teams/pf26d-child` -> 200.
13. `activateOrganizationChart`: POST `/v1/organizationCharts/{C1}/activate` -> 200 (synchronous, INF-23).
14. `deleteOrganizationChart`: DELETE `/v1/organizationCharts/{C2}` -> 200. Error `deleteOrganizationChart/error_validation_failed`: DELETE C1, which is now active -> 422 (row #29).

Pitfalls:
- `env.Do` sends a JSON body on DELETE when `body != nil`. That is required for `deleteTeamMembers`.
- Resolve user ids with `env.UserID(t, "sales-manager")`, `env.UserID(t, "sales-rep")` and `env.UserID(t, "sales-deputy")`.
- `internal/api/v1/teams.go:498` rejects a removal only when `remaining > 0 && leaders == 0`. Removing every member is allowed, because an empty team is not a leaderless remainder. The step-10 negative case must therefore leave at least one non-leader member (`sales-deputy`).
- Keep every mutation inside your own charts C1/C2 until step 13. Do not delete or alter fixture teams.
- Error codes come from the matrix rows cited. If the server returns another documented code for a leaderless or cycle case, treat it as a defect candidate (see below). Never adjust the expectation to whatever the server emits.

## Defect handling (design "Defect repair and ownership transfer")

1. **Root cause in your writePaths**:
   - Make the smallest fix. Change same-basename tests only to match corrected behavior.
   - Run the compile gate.
   - Run the package guard `go test -count=1 -v -race ./internal/<pkg>/...`.
   - Log a transfer candidate `{path, fromPlan, toPlan: "PF-26d", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in the s465 manifest: `ownershipTransfers[]`, then `plans[].writePaths`, then s460/s462.
2. **Eligible for a sibling**: reconciliation request to that sibling.
3. **Other file**: reconciliation request with `file:line`, subtest and log path; report `implementationIncomplete: true`.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26d-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26d-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveOrgChartsTeams$' ./internal/contract/... 2>&1 | tee /tmp/pf26d-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26d-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26d-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26d-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveOrgChartsTeams/' /tmp/pf26d-s466-contract.log)" -ge 18; echo "exit=$?"   # 14 operations + 4 error subtests
test -z "$(gofmt -l internal/contract/scenarios_orgcharts_teams_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_orgcharts_teams_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required for each package with an edited file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26d-s466-guard-<pkg>.log'; echo "exit=$?"`, with counts, plus `test -z "$(gofmt -l <edited files>)"`.

## Done Criteria

- [x] `TestContractLiveOrgChartsTeams` passes with 14 operation subtests and 4 error subtests. The inventory check passes.
- [x] Every verification line shows exit 0, with log path and counts. FAIL=0, SKIP=0.
- [x] Production edits have transfer-candidate records and passing guards. Other defects are reconciliation requests. No production edits or reconciliation requests were needed.
- [x] `go build ./...` exits 0 at the end of every attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16 (fresh reads and hashes, intent snapshot, drift detection, compile gate, prefix `pf26d`). The evidence rule:
1. `verification` holds only must-pass commands.
2. Remaining work goes in `implementationIncomplete: true`, `implPlanUpdates` and this log.
3. High/Medium only for delivered-code defects, with `file:line`.
4. Every attempt ends with `go build ./...` exit 0.

## Progress Log

### Session: 2026-10-21 (s466 attempt 1)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/intent.md`; pre-node snapshot copied from the runtime's `fanoutChangeEvidencePath` to `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/pre-node-snapshot.json`.
**Tasks Completed**: Added `internal/contract/scenarios_orgcharts_teams_test.go` with exactly 14 operation IDs, ordered create/get/update/mutate/delete coverage, response and error schema checks, timestamp and pagination checks, a `rel="next"` assertion, and inventory self-check. Exercised four `validation_failed` cases. No production defect was exposed; no repair-eligible production file was edited. Source SHA-256: `2654f6abd59b350da9291c49510e3a15a05b02202b555b8e47a0c4ab09e4ef18`.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26d-s466-build.log'; printf 'exit=%s\n' "$?"` — exit 0; log `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/build.log` (empty diagnostics).
- `bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26d-s466-vet.log'; printf 'exit=%s\n' "$?"` — exit 0; log `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/vet.log` (empty diagnostics).
- `bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveOrgChartsTeams$' ./internal/contract/... 2>&1 | tee /tmp/pf26d-s466-contract.log"; printf 'exit=%s\n' "$?"` — exit 0; log `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/contract.log`; 19 PASS lines total, including 18 passing operation/error subtests; FAIL=0, SKIP=0.
- Count guard on `/tmp/pf26d-s466-contract.log` — exit 0; log `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/counts.log`; `PASS=19 FAIL=0 SKIP=0 SUBTEST_PASS=18` (minimum 18).
- `test -z "$(gofmt -l internal/contract/scenarios_orgcharts_teams_test.go)"` and `test "$(wc -l < internal/contract/scenarios_orgcharts_teams_test.go)" -lt 1000` — both exit 0; combined output log `tmp/punchflow-kickflow-dummy-s466/PF-26d/attempt-1/mechanical.log`; 233 source lines.
**Transfer candidates**: None; production files were not edited.
**Reconciliation requests**: None.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
