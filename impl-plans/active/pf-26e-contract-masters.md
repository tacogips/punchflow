# PF-26e Contract Scenarios: Categories, Folders and General Masters Implementation Plan

**Status**: Ready
**Wave**: A (s466; parallel with PF-26a..PF-26d, PF-26f) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26e
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table), operation matrix rows #44-#46, #48-#50, #74 and #76-#82; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

`internal/contract` validates punchflow against `design-docs/references/kickflow-openapi.yaml`. PF-26e adds live HTTP scenarios for exactly these 14 operationIds:
- categories: `createCategory`, `deleteCategory`, `updateCategory`
- folders: `createFolder`, `deleteFolder`, `updateFolder`
- general masters: `createGeneralMaster`, `updateGeneralMaster`, `deleteGeneralMaster`, `listGeneralMasterItems`, `createGeneralMasterItem`, `getGeneralMasterItem`, `updateGeneralMasterItem`, `deleteGeneralMasterItem`

Each scenario checks status, schema, JST timestamps and pagination headers, plus the error format. Defects in this group's production code are fixed here, within the repair-eligible files.

## Non-goals

- No other operations. `listCategories`, `listFolders`, `getFolder`, `listGeneralMasters`, `getGeneralMaster`, routes and workflows stay in `TestContractLiveReadScenarios`.
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `scenarios_users_org_test.go` or `internal/cli/serve_smoke_test.go`.
- No `t.Skip`, no loosened validator, no special-casing, no `go.mod` edits, no git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26e-contract-masters.md`
- `internal/contract/scenarios_masters_test.go`
- `internal/api/v1/categories.go`
- `internal/api/v1/categories_test.go`
- `internal/api/v1/folders.go`
- `internal/api/v1/folders_test.go`
- `internal/api/v1/general_masters.go`
- `internal/api/v1/general_masters_test.go`
- `internal/present/classification.go`
- `internal/present/master.go`
- `internal/store/classification.go`
- `internal/store/read_classification.go`
- `internal/store/masters.go`
- `internal/store/read_masters.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 14 files. New work goes in the first two. The rest are repair-eligible only.

## Deliverable: `internal/contract/scenarios_masters_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveMastersOperations = []string{...}`: exactly the 14 ids above.
- `func TestContractLiveMasters(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26e`.

Existing code to imitate (read only):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios` (operation map, `responseSchemaURI`, `validateJSON`, `checkTimestamps`).
- `TestContractScenarios` in the same file: the 200-response lookup (`"200"`, then `"200.0"`). No `content` means an empty body.
- `internal/api/v1/categories_test.go`, `folders_test.go` and `general_masters_test.go` for request bodies, especially general master `fields` and item `inputs` (custom field values by field type, row #79). The spec request schemas are authoritative for key names.

Local helpers (same behavior in every PF-26 sub-plan):
- `pf26eCheck(t, op, resp)`:
  - status 200;
  - spec 200 without `content` -> empty body;
  - otherwise validate against the 200 schema and run `checkTimestamps`;
  - spec `parameters[].name` includes `perPage` -> `Page`, `Per-Page` and `Total` are non-empty.

  Cache compiled schemas.
- `pf26eCheckError(t, resp, status, code)`:
  - `testutil.AssertError`;
  - validate against `#/components/schemas/ErrorResponse`;
  - keys are a subset of `code`, `message` and `errors`;
  - `errors` is present if and only if status is 422;
  - no `doc_url`.
- Inventory check: `len == 14`; every id is in `apiv1.Operations()`; record the `t.Run` results and `t.Errorf` for every id without a passing subtest.

## Scenario sequence (one `testutil.New(t)`; persona Admin unless stated; sequential; never `t.Parallel`)

Subtest names are `TestContractLiveMasters/<operationId>`, and error cases are `<operationId>/error_<code>`. Folder, general master and item path parameters accept the id or the `code`. Category paths use ids.

1. `createCategory`: POST `/v1/categories` with a name -> 200. Keep the id as K. Error `createCategory/error_missing_permission`: as Employee -> 403 `missing_permission` (needs `workflow_misc`).
2. `updateCategory`: PATCH `/v1/categories/{K}` with a new name -> 200.
3. `deleteCategory`: DELETE `/v1/categories/{K}` -> 200. Error `deleteCategory/error_resource_not_found`: DELETE K again -> 404.
4. `createFolder`: POST `/v1/folders` with a name, code `pf26e-folder` and `parentId` = id of the fixture folder `root` (resolve it with `GET /v1/folders/root`) -> 200.
5. `updateFolder`: PATCH `/v1/folders/pf26e-folder` with a new name -> 200. Error `updateFolder/error_resource_not_editable`: PATCH `/v1/folders/root` -> 403 `resource_not_editable` (row #50).
6. `deleteFolder`: DELETE `/v1/folders/pf26e-folder` (empty, no workflows) -> 200.
7. `createGeneralMaster`: POST `/v1/generalMasters` with code `pf26e-master`, a name, and one `text` field with code `note` -> 200. Error `createGeneralMaster/error_missing_permission`: as Employee -> 403.
8. `updateGeneralMaster`: PATCH `/v1/generalMasters/pf26e-master` with a new name -> 200.
9. `createGeneralMasterItem`: POST `/v1/generalMasters/pf26e-master/items` with code `pf26e-item`, a name, and an input for field `note` -> 200. Create a second item, `pf26e-item2`, as setup.
10. `getGeneralMasterItem`: GET `.../items/pf26e-item` -> 200. Error `getGeneralMasterItem/error_resource_not_found`: unknown item code -> 404.
11. `listGeneralMasterItems`: GET `.../items` -> 200 with pagination headers. Link check with `?perPage=1` (2 items): `Link` contains `rel="next"`.
12. `updateGeneralMasterItem`: PATCH `.../items/pf26e-item` with a new name -> 200. The response keeps its input ids (CF-02); schema validation covers the shape.
13. `deleteGeneralMasterItem`: DELETE `.../items/pf26e-item2` -> 200.
14. `deleteGeneralMaster`: DELETE `/v1/generalMasters/pf26e-master` -> 200 (row #77: deletes its items).

Pitfalls:
- Never mutate the fixture masters `cost-centers` or `vendors`, or the fixture folders. Fixture workflows reference them.
- The persona Admin holds the Super Admin role (all permissions). Employee (`sales-rep`) holds no role.
- Do not test folder deletion with workflows (row #49). Fixture folder membership is not part of this plan's assertions.

## Defect handling (design "Defect repair and ownership transfer")

1. **Root cause in your writePaths**:
   - Make the smallest fix. Change same-basename tests only to match corrected behavior.
   - Run the compile gate.
   - Run the package guard `go test -count=1 -v -race ./internal/<pkg>/...`.
   - Log a transfer candidate `{path, fromPlan, toPlan: "PF-26e", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in the s465 manifest: `ownershipTransfers[]`, then `plans[].writePaths`, then s460/s462.
2. **Eligible for a sibling**: reconciliation request to that sibling.
3. **Other file** (for example `fixtures/default/*.json`, `internal/present/workflow*.go`): reconciliation request with `file:line`, subtest and log path; report `implementationIncomplete: true`.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26e/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26e-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26e-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveMasters$' ./internal/contract/... 2>&1 | tee /tmp/pf26e-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26e-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26e-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26e-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveMasters/' /tmp/pf26e-s466-contract.log)" -ge 19; echo "exit=$?"   # 14 operations + 5 error subtests
test -z "$(gofmt -l internal/contract/scenarios_masters_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_masters_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required for each package with an edited file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26e-s466-guard-<pkg>.log'; echo "exit=$?"`, with counts, plus `test -z "$(gofmt -l <edited files>)"`.

## Done Criteria

- [ ] `TestContractLiveMasters` passes with 14 operation subtests and 5 error subtests. The inventory check passes.
- [ ] Every verification line shows exit 0, with log path and counts. FAIL=0, SKIP=0.
- [ ] Production edits have transfer-candidate records and passing guards. Other defects are reconciliation requests.
- [ ] `go build ./...` exits 0 at the end of every attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16 (fresh reads and hashes, intent snapshot, drift detection, compile gate, prefix `pf26e`). The evidence rule:
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
