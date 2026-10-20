# PF-26f Contract Scenarios: Proxies and Roles Implementation Plan

**Status**: Ready
**Wave**: A (s466; parallel with PF-26a..PF-26e) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26f
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table), operation matrix rows #20-#22, #24-#26 and #84-#90, INF-16, INF-43; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

`internal/contract` validates punchflow against `design-docs/references/kickflow-openapi.yaml`. PF-26f adds live HTTP scenarios for exactly these 13 operationIds:
- proxies: `createProxyApplicant`, `updateProxyApplicant`, `deleteProxyApplicant`, `createProxyApprover`, `updateProxyApprover`, `deleteProxyApprover`
- roles: `createRole`, `getRole`, `updateRole`, `deleteRole`, `createRoleMembers`, `listRoleMembers`, `deleteRoleMember`

Each scenario checks status, schema, JST timestamps and pagination headers, plus the error format. Defects in proxy/role production code are fixed here, within the repair-eligible files.

## Non-goals

- No other operations. `listProxyApplicants`, `listProxyApprovers`, `listRoles` and `listAuditLogs` stay in `TestContractLiveReadScenarios`.
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `scenarios_users_org_test.go` or `internal/cli/serve_smoke_test.go`.
- `internal/api/v1/roles_test.go` must keep its `audit_read` negative case. PF-27's `TestRepoInvariants/negative_tests_present` checks it.
- No `t.Skip`, no loosened validator, no special-casing, no `go.mod` edits, no git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26f-contract-proxies-roles.md`
- `internal/contract/scenarios_proxies_roles_test.go`
- `internal/api/v1/proxies.go`
- `internal/api/v1/proxies_test.go`
- `internal/api/v1/roles.go`
- `internal/api/v1/roles_test.go`
- `internal/present/proxy.go`
- `internal/present/role.go`
- `internal/store/proxies.go`
- `internal/store/read_proxies.go`
- `internal/store/roles.go`
- `internal/store/read_roles.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 12 files. New work goes in the first two. The rest are repair-eligible only.

## Deliverable: `internal/contract/scenarios_proxies_roles_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveProxiesRolesOperations = []string{...}`: exactly the 13 ids above.
- `func TestContractLiveProxiesRoles(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26f`.

Existing code to imitate (read only):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios` (operation map, `responseSchemaURI`, `validateJSON`, `checkTimestamps`).
- `TestContractScenarios` in the same file: the 200-response lookup (`"200"`, then `"200.0"`). No `content` means an empty body.
- `internal/api/v1/proxies_test.go` and `roles_test.go` for request bodies (`userId`, `proxyUserId`, `workflowIds`, the role `permissionList`, membership `userIds`). The spec request schemas are authoritative.

Local helpers (same behavior in every PF-26 sub-plan):
- `pf26fCheck(t, op, resp)`:
  - status 200;
  - spec 200 without `content` -> empty body;
  - otherwise validate against the 200 schema and run `checkTimestamps`;
  - spec `parameters[].name` includes `perPage` -> `Page`, `Per-Page` and `Total` are non-empty.

  Cache compiled schemas.
- `pf26fCheckError(t, resp, status, code)`:
  - `testutil.AssertError`;
  - validate against `#/components/schemas/ErrorResponse`;
  - keys are a subset of `code`, `message` and `errors`;
  - `errors` is present if and only if status is 422;
  - no `doc_url`.
- Inventory check: `len == 13`; every id is in `apiv1.Operations()`; record the `t.Run` results and `t.Errorf` for every id without a passing subtest.

## Scenario sequence (one `testutil.New(t)`; persona Admin unless stated; sequential; never `t.Parallel`)

Subtest names are `TestContractLiveProxiesRoles/<operationId>`, and error cases are `<operationId>/error_<code>`.

Proxies:
1. `createProxyApplicant`: POST `/v1/proxyApplicants` with user `engineer`, proxy user `eng-manager` and `workflowIds: []` (empty means all workflows) -> 200. Keep its id. Error `createProxyApplicant/error_validation_failed`: the fixture pair user `ceo` / proxy `assistant` already exists -> 422 (row #20).
2. `updateProxyApplicant`: PATCH `/v1/proxyApplicants/{id}` with `workflowIds` = [expense-claim id, the same id again] -> 200. The duplicate is removed (row #21); assert that the returned workflow list has length 1, reading the array key from the schema.
3. `deleteProxyApplicant`: DELETE `/v1/proxyApplicants/{id}` -> 200.
4. `createProxyApprover`: POST `/v1/proxyApprovers` with user `finance-manager`, proxy user `accountant` and `workflowIds: []` -> 200. Error `createProxyApprover/error_missing_permission`: as Employee (no `user` permission; tenant `selfServiceProxy` is false) -> 403 `missing_permission`.
5. `updateProxyApprover`: PATCH `/v1/proxyApprovers/{id}` with one workflow id -> 200.
6. `deleteProxyApprover`: DELETE `/v1/proxyApprovers/{id}` -> 200.

Roles:

7. `createRole`: POST `/v1/roles` with a name and a `permissionList` containing one valid permission (for example `audit`) -> 200. Keep the id as R.
8. `getRole`: GET `/v1/roles/{R}` -> 200. Error `getRole/error_resource_not_found`: unknown UUID -> 404.
9. `updateRole`: PATCH `/v1/roles/{R}` with a new name -> 200. Error `updateRole/error_resource_not_editable`: PATCH the Super Admin role `00000000-0000-4000-8000-000000000029` -> 403 `resource_not_editable` (INF-43).
10. `createRoleMembers`: POST `/v1/roles/{R}/memberships` with the users `engineer` and `accountant` -> 200.
11. `listRoleMembers`: GET `/v1/roles/{R}/memberships` -> 200 with pagination headers. Link check with `?perPage=1` (2 members): `Link` contains `rel="next"`.
12. `deleteRoleMember`: DELETE `/v1/roles/{R}/memberships/{engineer id}` -> 200. Error `deleteRoleMember/error_validation_failed`: remove `admin`, the last Super Admin member, from role `...029` -> 422 `validation_failed` (INF-43).
13. `deleteRole`: DELETE `/v1/roles/{R}` -> 200.

Pitfalls:
- Fixture roles (`fixtures/default/roles.json`): Super Admin `...029` has `admin`; Ticket Admin `...030`; Master Editor `...031`; Integration Admin `...032`. Never mutate them except in the rejected error cases.
- Admin is a super admin, so INF-43's own-role protection does not apply to it.
- Resolve workflow ids with `GET /v1/workflows/expense-claim` or `store.ResolveID(..., "workflows", "expense-claim")`, and user ids with `env.UserID`.

## Defect handling (design "Defect repair and ownership transfer")

1. **Root cause in your writePaths**:
   - Make the smallest fix. Change same-basename tests only to match corrected behavior, and never remove the `audit_read` case.
   - Run the compile gate.
   - Run the package guard `go test -count=1 -v -race ./internal/<pkg>/...`.
   - Log a transfer candidate `{path, fromPlan, toPlan: "PF-26f", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in the s465 manifest: `ownershipTransfers[]`, then `plans[].writePaths`, then s460/s462.
2. **Eligible for a sibling**: reconciliation request to that sibling.
3. **Other file**: reconciliation request with `file:line`, subtest and log path; report `implementationIncomplete: true`.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26f/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26f-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26f-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveProxiesRoles$' ./internal/contract/... 2>&1 | tee /tmp/pf26f-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26f-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26f-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26f-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveProxiesRoles/' /tmp/pf26f-s466-contract.log)" -ge 18; echo "exit=$?"   # 13 operations + 5 error subtests
test -z "$(gofmt -l internal/contract/scenarios_proxies_roles_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_proxies_roles_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required for each package with an edited file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26f-s466-guard-<pkg>.log'; echo "exit=$?"`, with counts, plus `test -z "$(gofmt -l <edited files>)"`.

## Done Criteria

- [ ] `TestContractLiveProxiesRoles` passes with 13 operation subtests and 5 error subtests. The inventory check passes.
- [ ] Every verification line shows exit 0, with log path and counts. FAIL=0, SKIP=0.
- [ ] Production edits have transfer-candidate records and passing guards. Other defects are reconciliation requests.
- [ ] `go build ./...` exits 0 at the end of every attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16 (fresh reads and hashes, intent snapshot, drift detection, compile gate, prefix `pf26f`). The evidence rule:
1. `verification` holds only must-pass commands.
2. Remaining work goes in `implementationIncomplete: true`, `implPlanUpdates` and this log.
3. High/Medium only for delivered-code defects, with `file:line`.
4. Every attempt ends with `go build ./...` exit 0.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Transfer candidates**: -
**Reconciliation requests**: -
