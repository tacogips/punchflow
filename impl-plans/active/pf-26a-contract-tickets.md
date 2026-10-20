# PF-26a Contract Scenarios: Tickets Implementation Plan

**Status**: Ready
**Wave**: A (s466; parallel with PF-26b..PF-26f) | **Depends On**: none (all dependencies accepted at `a2a82de`) | **Plan ID**: PF-26a
**Design Reference**: design-docs/specs/design-api-coverage.md "Live scenario partition (session 466)" (rules 1-8, repair-eligibility table) and the operation matrix rows #57, #59-#70; design-docs/specs/architecture.md "PF-26 split and final-run rule (session 466)"
**Created**: 2026-10-21
**Last Updated**: 2026-10-21

---

## Intent and Context

punchflow is a local kickflow dummy. Its kickflow compatibility is proved mechanically by `internal/contract`. At `a2a82de` the package checks route coverage, schema compilation for all 96 operations, 22 live read scenarios and the 13 webhook payloads. PF-26a adds live HTTP scenarios for the 13 ticket operations not yet exercised. Each scenario checks the status code, validates the response against `design-docs/references/kickflow-openapi.yaml`, checks JST timestamps and pagination headers, and adds error-format checks. A defect found in ticket production code is fixed here, within the repair-eligible files below.

Operations (exactly these 13 operationIds):
`createTicket`, `getTicket`, `updateTicket`, `updateTicketForm`, `approveTicket`, `rejectTicket`, `denyTicket`, `withdrawTicket`, `archiveTicket`, `listTicketLinks`, `listViewers`, `createViewer`, `deleteViewer`.

## Non-goals

- Do not touch other groups' operations, or the 22 read operations already covered (`listTickets` and `listTasks` stay in `TestContractLiveReadScenarios`).
- Do not edit the frozen files `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `scenarios_users_org_test.go` or `internal/cli/serve_smoke_test.go`.
- No webhook delivery assertions. Do not call `env.DeliverWebhooks`.
- No `t.Skip`, no loosened validator, no special-casing of an operation, no edits to `go.mod`/`go.sum`.
- No git state changes, no repo-wide gofmt.

## Path Declarations

### writePaths
- `impl-plans/active/pf-26a-contract-tickets.md`
- `internal/contract/scenarios_tickets_test.go`
- `internal/api/v1/tickets.go`
- `internal/api/v1/tickets_test.go`
- `internal/api/v1/ticket_actions.go`
- `internal/api/v1/ticket_actions_test.go`
- `internal/api/v1/ticket_viewers.go`
- `internal/api/v1/ticket_viewers_test.go`
- `internal/present/ticket.go`
- `internal/present/ticket_test.go`
- `internal/present/ticket_detail.go`
- `internal/present/viewer.go`
- `internal/store/tickets.go`
- `internal/store/read_tickets.go`
- `internal/store/ticket_queries.go`
- `internal/engine/engine.go`
- `internal/engine/actions.go`
- `internal/engine/create.go`
- `internal/engine/update.go`
- `internal/engine/forms.go`
- `internal/engine/forms_test.go`
- `internal/engine/routing.go`
- `internal/engine/routing_test.go`
- `internal/engine/conditions.go`
- `internal/engine/conditions_test.go`
- `internal/engine/visibility.go`
- `internal/engine/numbering.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

Expansion: 27 files. Only the first two are new work. All other entries are repair-eligible: edit one only to fix a defect that one of your scenarios exposes (see "Defect handling").

## Deliverable: `internal/contract/scenarios_tickets_test.go` (new, `package contract_test`, < 1000 lines)

Symbols:
- `var liveTicketsOperations = []string{...}`: the 13 ids above, nothing else.
- `func TestContractLiveTickets(t *testing.T)`: the only test function.
- Every other package-level identifier starts with `pf26a`, for example `pf26aCheck`, `pf26aCheckError` and `pf26aCreateTicket`.

Existing code to imitate (read it; do not edit it):
- `internal/contract/scenarios_users_org_test.go:TestContractLiveReadScenarios`: builds an operation map from `apiv1.Operations()`, compiles `responseSchemaURI(op.Pattern, strings.ToLower(op.Method))` with `schemaCompiler(t, loadSpec(t))`, then calls `validateJSON` and `checkTimestamps`.
- `internal/contract/scenarios_users_org_test.go:TestContractScenarios`: finds the spec 200 response as `responses["200"]`, falling back to `responses["200.0"]` (YAML numeric-key normalization). A 200 response without `content` means an empty body.
- `internal/api/v1/tickets_test.go:pf22CreateTicket`: the expense-claim create body (workflowId, authorTeamId, all six inputs). You cannot call it from `contract_test`. Resolve ids with `store.ResolveID(t.Context(), env.DB.Q(), "workflows", "expense-claim")` and `"teams", "sales"`, and read the form field ids from `GET /v1/workflows/expense-claim` (`sectionList[].formFields[].id` by `code`). Do not hard-code UUIDs copied from another package.
- `internal/api/v1/ticket_actions_test.go:TestTicketActionsOptionalBodiesAndTransitions`: who may act. Employee (sales-rep) creates; Manager (sales-manager) is the first assignee; reject takes `{"to": 0}`; withdraw yields status `rejected`.
- `internal/api/v1/ticket_viewers_test.go:TestViewerCreateListDelete`: viewer bodies.

Local helpers (behavior, not code):
- `pf26aCheck(t, op, resp)`:
  - status 200;
  - if the spec 200 response has no `content`, then `len(resp.Body) == 0`;
  - otherwise validate the body against the compiled schema and run `checkTimestamps` on the decoded value;
  - if the spec operation's `parameters[].name` includes `perPage`, the `Page`, `Per-Page` and `Total` headers are non-empty.

  Compile each schema once (cache by operationId).
- `pf26aCheckError(t, resp, status, code)`:
  - call `testutil.AssertError`;
  - validate the body against `file:///kickflow-openapi.json#/components/schemas/ErrorResponse`;
  - decode it into a map and check that its keys are a subset of `code`, `message` and `errors`, that `errors` is present only for status 422 (it is optional on 422), and that `doc_url` is never present.
- Inventory check:
  - At the start, assert `len(liveTicketsOperations) == 13` and that every id exists in `apiv1.Operations()`.
  - Record `ok := t.Run(id, ...)` per id.
  - At the end, `t.Errorf` for every listed id that has no subtest that ran and passed.

## Scenario sequence (one `env := testutil.New(t)`; subtests run sequentially; never `t.Parallel`)

Subtest names are `TestContractLiveTickets/<operationId>`, and error cases are `<operationId>/error_<code>`.

1. `createTicket`: Employee POST `/v1/tickets` with status `draft` (ticket D1) -> 200, schema-valid. Also create, as Employee with status `in_progress`, tickets A (to approve), R (reject), N (deny) and W (withdraw) through the same helper. Only the first create runs inside the `createTicket` subtest; the others are setup.
2. `getTicket`: Employee GET D1 -> 200, schema-valid. Error `getTicket/error_resource_not_found`: GET `/v1/tickets/00000000-0000-4000-8000-000000000000` -> 404 `resource_not_found`.
3. `updateTicket`: Employee PATCH `/v1/tickets/{D1}` with a changed `title` (read the request schema `updateTicket` in the spec for allowed keys) -> 200, schema-valid.
4. `updateTicketForm`: Manager PATCH `/v1/tickets/{A}/form` with one input that `tickets_test.go:TestTicketUpdateFormHTTP` shows is accepted: the `purpose` field, given a new value -> 200. Error `updateTicketForm/error_invalid_parameter`: an empty JSON object body -> 400 `invalid_parameter`.
5. `approveTicket`: Manager POST `/v1/tickets/{A}/approve` with no body -> 200. Error `approveTicket/error_user_not_assigned`: Employee (the author, not an assignee) approves R -> 403 `user_not_assigned`.
6. `rejectTicket`: Manager POST `/v1/tickets/{R}/reject` with `{"to": 0}` -> 200, status `rejected`.
7. `denyTicket`: Manager POST `/v1/tickets/{N}/deny` -> 200, status `denied`.
8. `withdrawTicket`: Employee POST `/v1/tickets/{W}/withdraw` -> 200, status `rejected`.
9. `listTicketLinks`: Employee GET `/v1/tickets/{D1}/links` -> 200 (an empty array is valid).
10. `createViewer`: Employee POST `/v1/tickets/{D1}/viewers` with `{"userId": <engineer id>}` -> 200. Add a second viewer with `{"teamId": <finance team id>}` as setup. Error `createViewer/error_invalid_parameter`: a body with both `userId` and `teamId` -> 400 `invalid_parameter`.
11. `listViewers`: Employee GET `/v1/tickets/{D1}/viewers` -> 200, with pagination headers if the spec declares `perPage`. Link check: GET `...?perPage=1` (Total is 2) -> the `Link` header contains `rel="next"`. If `listViewers` declares no `perPage`, do the Link check on any other paginated operation you call and note the choice in the progress log.
12. `deleteViewer`: Employee DELETE `/v1/tickets/{D1}/viewers/{viewerId}` (id from the step-11 schema-validated list response; `createViewer` has an empty 200 body per the spec) -> 200, checked by `pf26aCheck` (empty-body rule from the spec).
13. `archiveTicket`: Employee POST `/v1/tickets/{D1}/archive` -> 200, status `archived`. Error `archiveTicket/error_invalid_state`: archiving D1 again -> 422 `invalid_state` (INF-24). The 422 body may carry `errors`; the helper allows it.

Pitfalls:
- Decode ids from responses (`{"id": ...}`); viewer ids come from the schema-validated listViewers response because createViewer returns an empty body per the OpenAPI spec.
- The `Integration` persona sends `X-Caller-Id` taken from the `testutil.Env.CallerFor` field (a string field, not a method). This plan does not use it.
- Approving A may complete only the first step. Do not assert `completed`.
- Do not assert hand-written key lists. Schema validation is the field-name check (design rule 4).

## Defect handling (design "Defect repair and ownership transfer")

When a scenario fails on a valid request, find the root-cause file.
1. **In your writePaths**:
   - Make the smallest fix. Change a same-basename `_test.go` file only to match the corrected behavior; never delete or weaken an assertion.
   - Run the compile gate (`go build ./...` after a non-test write, `go vet <pkg>` after a test write).
   - Run the package guard: `go test -count=1 -v -race ./internal/<pkg>/...` (one of `internal/api/v1`, `internal/present`, `internal/store`, `internal/engine`).
   - Record a transfer candidate in your progress log: `{path, fromPlan, toPlan: "PF-26a", declaringManifest, reason, guardVerification}`. Look up `fromPlan` in `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`: `ownershipTransfers[]` first, then `plans[].writePaths`, then the s460/s462 manifests.
2. **Repair-eligible for a sibling** (PF-26b..PF-26f; see the design table): do not edit it. Add a reconciliation request addressed to that sibling in your log.
3. **Any other file**, for example `fixtures/default/*.json`, `internal/httpx`, `internal/present/workflow_form.go` or `internal/engine/comments.go`: do not edit it. Add a reconciliation request with `file:line`, the failing subtest and the log path, and report `implementationIncomplete: true`.

Guard failures in a sibling's in-flight file follow README rule 13 (re-check at your final verification). `internal/engine/engine.go` is a contract stub: keep every exported identifier and signature.

## Verification (all must exit 0; logs copied to `tmp/punchflow-kickflow-dummy-s466/PF-26a/attempt-<n>/`)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26a-s466-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... 2>&1 | tee /tmp/pf26a-s466-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run '^TestContractLiveTickets$' ./internal/contract/... 2>&1 | tee /tmp/pf26a-s466-contract.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf26a-s466-contract.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf26a-s466-contract.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf26a-s466-contract.log)"   # FAIL=0 SKIP=0
test "$(grep -c -- '--- PASS: TestContractLiveTickets/' /tmp/pf26a-s466-contract.log)" -ge 18; echo "exit=$?"   # 13 operations + 5 error subtests
test -z "$(gofmt -l internal/contract/scenarios_tickets_test.go)"; echo "exit=$?"
test "$(wc -l < internal/contract/scenarios_tickets_test.go)" -lt 1000; echo "exit=$?"
```

Conditional guard (required, and must pass, for each package in which you edited a file): `bash -o pipefail -c 'go test -count=1 -v -race ./internal/<pkg>/... 2>&1 | tee /tmp/pf26a-s466-guard-<pkg>.log'; echo "exit=$?"`, with its PASS/FAIL/SKIP counts. Also run `test -z "$(gofmt -l <each edited file>)"`.

## Done Criteria

- [x] `TestContractLiveTickets` passes with 13 operation subtests and 5 error subtests (getTicket, updateTicketForm, approveTicket, createViewer, archiveTicket). The inventory check passes.
- [x] Every verification line shows exit 0, with log path and counts in the Progress Log. FAIL=0, SKIP=0.
- [x] Every production edit has a transfer-candidate record and a passing package guard. Every non-eligible defect is a reconciliation request. (No production edits or reconciliation requests were needed.)
- [x] `go build ./...` exits 0 at the end of every attempt.

## Execution Protocol

Follow `impl-plans/README.md` "Shared Execution Protocol" rules 1-16: fresh reads with pre/post `shasum -a 256` for every existing file you edit, intent snapshot, drift detection, compile gate, and identifier prefix `pf26a`. The evidence rule:
1. The `verification` array holds only must-pass commands. Expected non-zero proofs go into `authorSelfCheck.checks` text.
2. Remaining work is reported as `implementationIncomplete: true` plus `implPlanUpdates` and this log, never as a High/Medium finding or risk.
3. High/Medium labels are only for defects in delivered code, with `file:line`. Sibling breakage is a blocker naming the owner.
4. Every attempt ends with `go build ./...` exit 0.

## Progress Log

### Session: 2026-10-21
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s466/PF-26a/attempt-1/intent-scenarios.md`; edit intents: `edit-intent-003.md` through `edit-intent-006.md`.
**Tasks Completed**: Added `internal/contract/scenarios_tickets_test.go` with all 13 ticket operations and five named error subtests. Success responses validate against the operation's OpenAPI 200 schema (or empty-body contract); errors validate against `ErrorResponse`; JST timestamps, pagination headers, one `Link rel="next"`, and inventory are checked. `createViewer` is checked as an empty response per the spec; the viewer ID is read from the validated list response. No production files were modified.
**Verification** (final source, scenario SHA-256 `ed62c38a689190766df315d3e81e0982f2f974d9a6bbe9e08e348d64d66c12ad`; complete logs also copied under `tmp/punchflow-kickflow-dummy-s466/PF-26a/attempt-1/`):
- `go build ./...`: exit 0; `/tmp/pf26a-s466-build.log`.
- `go vet ./internal/contract/...`: exit 0; `/tmp/pf26a-s466-vet.log`.
- `go test -count=1 -v -race -run '^TestContractLiveTickets$' ./internal/contract/...`: exit 0; PASS=19, FAIL=0, SKIP=0; 13 operation plus 5 nested error PASS lines; `/tmp/pf26a-s466-contract.log`.
- `test "$(grep -c -- '--- PASS: TestContractLiveTickets/' /tmp/pf26a-s466-contract.log)" -ge 18`: exit 0.
- `test -z "$(gofmt -l internal/contract/scenarios_tickets_test.go)"`: exit 0.
- `test "$(wc -l < internal/contract/scenarios_tickets_test.go)" -lt 1000`: exit 0; 385 lines.
- Structural evidence `/tmp/pf26a-s466-structure.log`: PASS=19, FAIL=0, SKIP=0; subtest-count, gofmt, and line-limit checks each exit 0.
- The final build was run after vet and test and exited 0.
**Prior attempts**: The initial contract failure is preserved at `/tmp/pf26a-s466-contract-attempt1-failed.log` and `tmp/punchflow-kickflow-dummy-s466/PF-26a/attempt-1/prior-contract-failed.log`; it exposed the incorrect requirement that every 422 contain `errors`. The helper now allows omission on 422 and still rejects `errors` on other statuses. The initial post-format vet attempt failed on sibling in-flight `internal/contract/scenarios_masters_test.go:138:51` (`undefined: compiler`); final vet passed after the sibling file was completed. A passing intermediate focused run lacked separately named error subtests; those were added and the final run reports all 18 named subtests passing.
**Plan wording reconciliations**: The OpenAPI createViewer 200 has no response content, so viewer IDs come from listViewers. `errors` is optional for 422 and forbidden on other statuses, matching the accepted criterion and the observed schema-valid response.
**Transfer candidates**: None; no production edits.
**Reconciliation requests**: None.
**Plan hashes**: pre-edit `8be77002dc38c03e4d3223b77d9121b7c63ef29bdc95d45bfbb94100fc6400b5`; final post-edit hash is reported in Step 6 evidence.
