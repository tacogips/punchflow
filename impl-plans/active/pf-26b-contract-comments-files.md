# PF-26b Contract Scenarios: Comments and Files Implementation Plan

**Status**: Ready
**Wave**: A (s466; parallel with PF-26a, PF-26c..PF-26f) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26b
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table), operation matrix rows #71, #72 and #91-#95, INF-26, INF-30, INF-37; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

`internal/contract` proves kickflow compatibility against `design-docs/references/kickflow-openapi.yaml`. At `a2a82de` it has no live scenario for comments or files. PF-26b adds live HTTP scenarios for exactly these 7 operationIds: `uploadFile`, `getFile`, `listComments`, `createComment`, `getComment`, `updateComment`, `deleteComment`. Each checks status, schema, JST timestamps and pagination headers, plus the error format. A defect found in comment or file production code is fixed here, within the repair-eligible files.

## Non-goals

- No other operations. Ticket creation is setup only; `createTicket` is scored by PF-26a.
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `scenarios_users_org_test.go` or `internal/cli/serve_smoke_test.go`.
- No webhook delivery assertions. Do not call `env.DeliverWebhooks` (comment events are covered by `TestContractWebhookPayloads`).
- No `t.Skip`, no loosened validator, no special-casing, no `go.mod` edits, no git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26b-contract-comments-files.md`
- `internal/contract/scenarios_comments_files_test.go`
- `internal/api/v1/comments.go`
- `internal/api/v1/comments_test.go`
- `internal/api/v1/files.go`
- `internal/api/v1/files_test.go`
- `internal/present/comment.go`
- `internal/present/comment_test.go`
- `internal/store/files.go`
- `internal/engine/comments.go`
- `internal/engine/comments_test.go`
- `internal/files/files.go`
- `internal/files/download.go`
- `internal/files/files_test.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 14 files. Only the first two are new work. The rest are repair-eligible: edit one only for a defect your scenarios expose. `internal/files/files.go` is a contract stub: keep its exported identifiers and signatures.

## Deliverable: `internal/contract/scenarios_comments_files_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveCommentsFilesOperations = []string{...}`: exactly the 7 ids above.
- `func TestContractLiveCommentsFiles(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26b`.

Existing code to imitate (read only):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios`: operation map, schema compile via `responseSchemaURI`, `validateJSON`, `checkTimestamps`.
- `internal/contract/scenarios_users_org_test.go:TestContractScenarios`: the 200-response lookup (`"200"`, then `"200.0"`). No `content` means an empty body.
- `internal/api/v1/files_test.go:pf23UploadFile` and `pf23Request`: a multipart upload with part name `file`, sent through `env.DoRaw` with the header `Authorization: Bearer pf_pat_dev_employee`. `testutil.Env.Do` sends JSON only.
- `internal/api/v1/comments_test.go:pf23CreateExpenseTicket`: an expense-claim ticket whose `receipt` file input references the uploaded `signedId`. In `contract_test`, create the ticket over HTTP (POST `/v1/tickets` as Employee, team `sales`, status `in_progress`). Take the form field ids from `GET /v1/workflows/expense-claim` by field `code`. Send the receipt input as `{"formFieldId": <receipt id>, "files": [signedId]}`.
- `internal/api/v1/comments_test.go:TestCommentLifecycleAndWebhookEvents`: comment bodies, delete returning an empty body, and `?perPage=1` pagination.

Local helpers (same behavior in every PF-26 sub-plan):
- `pf26bCheck(t, op, resp)`:
  - status 200;
  - spec 200 without `content` -> zero-length body;
  - otherwise validate against the compiled 200 schema and run `checkTimestamps`;
  - spec `parameters[].name` includes `perPage` -> `Page`, `Per-Page` and `Total` are non-empty.

  Cache compiled schemas.
- `pf26bCheckError(t, resp, status, code)`:
  - `testutil.AssertError`;
  - validate against `file:///kickflow-openapi.json#/components/schemas/ErrorResponse`;
  - keys are a subset of `code`, `message` and `errors`;
  - `errors` is present if and only if status is 422;
  - no `doc_url`.
- Inventory check: `len == 7`; every id is in `apiv1.Operations()`; record the `t.Run` results and `t.Errorf` for every id without a passing subtest.

## Scenario sequence (one `testutil.New(t)`; sequential; never `t.Parallel`)

Subtest names are `TestContractLiveCommentsFiles/<operationId>`, and error cases are `<operationId>/error_<code>`.

1. `uploadFile`: Employee multipart upload of a small PDF -> 200, schema-valid, `signedId` non-empty. Error `uploadFile/error_<code>`: a multipart body without the `file` part -> the status and code that `files_test.go:TestUploadRejectsInvalidFormAndStandardPlan` asserts for that case. Read that test and use its values.
2. Setup: an Employee `in_progress` expense ticket T that attaches the `signedId`.
3. `getFile`: Employee GET `/v1/files/{signedId}` -> 200, schema-valid. Do not download `url`; it is a punchflow signed URL, not a spec operation. Error `getFile/error_resource_not_found`: upload a second file and do not attach it; GET it -> 404 `resource_not_found` (INF-26).
4. `createComment`: Employee POST `/v1/tickets/{T}/comments` with `{"body": "contract comment"}` -> 200. Create a second comment as setup.
5. `getComment`: GET `/v1/tickets/{T}/comments/{id}` -> 200. Error `getComment/error_resource_not_found`: unknown comment UUID -> 404.
6. `updateComment`: Employee PATCH with `{"body": "updated"}` -> 200. Error `updateComment/error_missing_permission`: Manager (sales-manager, the assignee who can see T) PATCHes the Employee's comment -> 403 `missing_permission` (INF-37).
7. `listComments`: Employee GET `/v1/tickets/{T}/comments` -> 200 with pagination headers. Link check: `?perPage=1` with 2 comments -> `Link` contains `rel="next"`.
8. `deleteComment`: Employee DELETE the first comment -> 200, empty body as the spec dictates (checked by `pf26bCheck`). A follow-up `getComment` must still be 200 and schema-valid with `deletedAt` set (INF-30). Run this follow-up inside the `deleteComment` subtest.

Pitfalls:
- Multipart requests must not set `Content-Type: application/json`. Use the writer's `FormDataContentType()`.
- `getFile` returns 404 for an unattached upload by design; attach before the success case.
- Decode ids from responses, and resolve users with `env.UserID`.

## Defect handling (design "Defect repair and ownership transfer")

1. **Root cause in your writePaths**:
   - Make the smallest fix. Change a same-basename `_test.go` file only to match corrected behavior; never weaken it.
   - Run the compile gate.
   - Run the package guard `go test -count=1 -v -race ./internal/<pkg>/...` (`api/v1`, `present`, `store`, `engine` or `files`).
   - Log a transfer candidate `{path, fromPlan, toPlan: "PF-26b", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in the s465 manifest: `ownershipTransfers[]` first, then `plans[].writePaths`, then the s460/s462 manifests.
2. **Eligible for a sibling** (for example ticket creation and engine files belong to PF-26a): reconciliation request to that sibling; do not edit.
3. **Any other file**: reconciliation request with `file:line`, subtest and log path; report `implementationIncomplete: true`.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26b/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26b-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26b-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveCommentsFiles$' ./internal/contract/... 2>&1 | tee /tmp/pf26b-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26b-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26b-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26b-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveCommentsFiles/' /tmp/pf26b-s466-contract.log)" -ge 11; echo "exit=$?"   # 7 operations + 4 error subtests
test -z "$(gofmt -l internal/contract/scenarios_comments_files_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_comments_files_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required for each package with an edited file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26b-s466-guard-<pkg>.log'; echo "exit=$?"`, with counts, plus `test -z "$(gofmt -l <edited files>)"`.

## Done Criteria

- [ ] `TestContractLiveCommentsFiles` passes with 7 operation subtests and 4 error subtests (uploadFile, getFile, getComment, updateComment). The inventory check passes.
- [ ] Every verification line shows exit 0, with log path and counts. FAIL=0, SKIP=0.
- [ ] Production edits have transfer-candidate records and passing guards. Other defects are reconciliation requests.
- [ ] `go build ./...` exits 0 at the end of every attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16 (fresh reads and pre/post `shasum -a 256`, intent snapshot, drift detection, compile gate, prefix `pf26b`). The evidence rule:
1. `verification` holds only must-pass commands.
2. Remaining work goes in `implementationIncomplete: true`, `implPlanUpdates` and this log, never in findings or risks.
3. High/Medium only for delivered-code defects, with `file:line`.
4. Every attempt ends with `go build ./...` exit 0.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Transfer candidates**: -
**Reconciliation requests**: -
