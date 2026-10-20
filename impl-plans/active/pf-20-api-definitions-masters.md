# PF-20 /v1 Categories, Folders, Routes, Workflows and General Masters Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-20
**Design Reference**: design-docs/specs/design-api-coverage.md rows 43-55 and 73-82; design-docs/references/kickflow-api.md section 3 "Categories", "Folders", "Routes", "Workflows", "General masters"; notes.md DIV-05, INF-02, INF-29, INF-42
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

---

## Intent and Context

Replace the PF-10 stubs for 23 operations: categories (4), folders (5), routes (2), workflows (2) and general masters with items (10). Routes and workflows are read-only fixture definitions (DIV-05).

## Non-goals

- No workflow or route editing. No CSV import/export. No changes outside the listed files.

## Files

`internal/api/v1/categories.go`, `folders.go`, `definitions.go`, `general_masters.go` (shared: replace stubs); tests `categories_test.go`, `folders_test.go`, `definitions_test.go`, `general_masters_test.go` (`package v1_test`); `internal/store/classification.go`, `internal/store/masters.go`, `internal/store/definitions.go` (new).

## Path Declarations

### writePaths
- `impl-plans/active/pf-20-api-definitions-masters.md`
- `internal/api/v1/categories_test.go`
- `internal/api/v1/folders_test.go`
- `internal/api/v1/definitions_test.go`
- `internal/api/v1/general_masters_test.go`
- `internal/store/classification.go`
- `internal/store/masters.go`
- `internal/store/definitions.go`

### sharedPaths
- `internal/api/v1/categories.go`
- `internal/api/v1/folders.go`
- `internal/api/v1/definitions.go`
- `internal/api/v1/general_masters.go`

### sharedPathNotes
- `internal/api/v1/categories.go`: PF-10 stub file; replace the bodies of operations 43-46
- `internal/api/v1/folders.go`: PF-10 stub file; replace the bodies of operations 47-51
- `internal/api/v1/definitions.go`: PF-10 stub file; replace the bodies of operations 52-55
- `internal/api/v1/general_masters.go`: PF-10 stub file; replace the bodies of operations 73-82

### artifactRoots
- (none)

## Handler Conventions

Same as PF-18 "Handler Conventions".

## Operation Notes

| Op | Perm | Rules |
|---|---|---|
| listCategories | any | paginated, sortBy per spec |
| create/update/deleteCategory | workflow_misc | code unique -> 422; because workflow definitions are read-only, deleting a category referenced by any workflow `categoryCodes` -> 422 `validation_failed` (inferred; log) |
| listFolders / getFolder | any | FolderDetail on get |
| createFolder | workflow_misc | `parentId` required (root exists); code unique |
| updateFolder / deleteFolder | workflow_misc | root (`editable` false) -> 403 `resource_not_editable`; delete folder with child folders, workflows or routes -> 422 `validation_failed` |
| listRoutes / getRoute | any | `status` filter if the spec defines one; RouteDetail on get |
| listWorkflows / getWorkflow | any | spec filters (e.g. `status`, `folderId`, `categoryId` as defined); WorkflowDetail on get; id or code |
| listGeneralMasters / getGeneralMaster | master_read or master | GeneralMaster with fields |
| create/update/deleteGeneralMaster | master | body per spec incl. `fields[]` (field codes unique; types from enum); delete cascades items |
| listGeneralMasterItems / getGeneralMasterItem | master_read or master | default order = master `defaultSortBy`; spec filters/sortBy |
| create/update/deleteGeneralMasterItem | master | `inputs[]` by field code or id, value type per field type (checkbox -> array within options, pull_down within options, number/integer/date formats); required fields -> 422; code unique per master |

## Key Points and Pitfalls

- Use `auth.Require(r, "master_read", "master")` for reads (either suffices); writes need `master`.
- Folder `fullName`, counts and `editable` come from PF-08 presenters; do not recompute.
- `getWorkflow`/`getRoute` accept a code: `store.ResolveID(ctx, q, "workflows", v)`.
- `deleteCategory` rule above is an inference (definitions cannot be edited); record it in the progress log as INF candidate for PF-27 to add to `notes.md`.

## Tests (`package v1_test`)

- listCategories Employee -> 200 3 items; createCategory Employee -> 403; Admin create/update/delete unused -> 200; delete `expense` (used) -> 422
- folders: update root -> 403 `resource_not_editable`; delete `finance` (has workflow) -> 422; create child of `general` then delete -> 200
- listWorkflows -> 3 items with pagination headers; getWorkflow by code `expense-claim` -> keys include sectionList, routeConditions, slackChannel (null); getRoute `expense-large` -> 4 steps incl. two with stepOrder 2
- masters: Employee list -> 403; Master Editor persona via SAT caller `accountant` -> 200; create master with duplicate field codes -> 422; create item with pull_down value outside options -> 422; list items of cost-centers default sort by `defaultSortBy`
- unknown master id -> 404; delete master -> items gone

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-06, PF-08, PF-16.
**State at 410d3d8**: not started (PF-10 stubs in `general_masters.go`, `categories.go`, `folders.go` and `definitions.go`). Implement this plan as written, plus the CF-02 API hop (`architecture.md#fixtures`, input ID data flow):

- `createGeneralMasterItem`: one `store.NewID()` per input. Write `general_master_items.inputs` = `{fieldId: value}` and `input_ids` = `{fieldId: inputId}` in the same INSERT.
- `updateGeneralMasterItem`:
  - The request `inputs[]` addresses fields by `code`, so resolve each code to its field ID in this master (unknown code -> 422 `validation_failed`).
  - For a field already present in `input_ids`, keep its existing input ID and replace the value. A field with no prior input gets a new ID.
  - Delete IDs only for fields no longer present.
  - Write both columns in one UPDATE inside the tx.
- Responses use `present.GeneralMasterItem` (PF-08), so `inputs[].id` is the persisted ID.
- Enum validation for request bodies uses the `internal/model` lists:
  - master `defaultSortBy` in `model.MasterSortKeys` (`name|code`)
  - master field `fieldType` in `model.MasterFieldTypes`
  - `listRoutes` `status` filter in `visible|error` (spec query enum), `listWorkflows` `status` filter in `visible|invisible`
  - other values -> `400 invalid_parameter`
- Permissions are `model.PermissionWorkflowMisc` (categories, folders writes), `model.PermissionMaster`/`model.PermissionMasterRead`. Reads of categories, folders, routes and workflows need any authenticated user (INF-42).
- Name tests `TestCategor*`, `TestFolder*`, `TestWorkflow*`, `TestRoute*`, `TestGeneralMaster*`, `TestMaster*`; prefix helpers with `pf20`. Note: `TestRoute*` must not collide with PF-10's `TestPF10*` names.

Additional tests (input -> expected):
- create item in `cost-centers` with `inputs [{code: budget-owner, value: x}]` -> 200; response `inputs[0].id` is a UUID different from `inputs[0].field.id`; `SELECT input_ids` matches it
- update the same item's value -> same `inputs[0].id` as before; value changed
- get a fixture item `cc-sales` -> `inputs[0].id` equals the fixture input id of `cc-sales` (`00000000-0000-4000-8000-000000000042` at 410d3d8). Read it from `fixtures/default/general-masters.json` in the test rather than hard-coding it
- `defaultSortBy: createdAt` on createGeneralMaster -> 400 `invalid_parameter`

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf20-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf20-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Categor|Folder|Workflow|Route|GeneralMaster|Master)' ./internal/api/v1/... 2>&1 | tee /tmp/pf20-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf20-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf20-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf20-s462-test.log)"
gofmt -l internal/api/v1/categories.go internal/api/v1/folders.go internal/api/v1/definitions.go internal/api/v1/general_masters.go internal/api/v1/categories_test.go internal/api/v1/folders_test.go internal/api/v1/definitions_test.go internal/api/v1/general_masters_test.go internal/store/classification.go internal/store/masters.go internal/store/definitions.go   # prints nothing
rg -n 'not implemented' internal/api/v1/categories.go internal/api/v1/folders.go internal/api/v1/definitions.go internal/api/v1/general_masters.go   # prints nothing (exit 1)
```

### Done Criteria (session 462)

- [ ] 23 operations implemented; CF-02 API hop closed (create assigns IDs, update preserves them, get returns fixture IDs), proven by tests
- [ ] Listed tests pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean; inferred rules recorded in the progress log

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-06 and PF-08 (accepted at f82e374) and PF-16 (wave A).
**State at f82e374**: not started. The four handler files are PF-10 stubs. `internal/store/classification.go`, `masters.go` and `definitions.go` do not exist. CF-02: the load, export, read and present halves are accepted. The API hop (create assigns input IDs, update keeps them) is this plan's to close; see `design-docs/specs/notes.md` "Status at session 463".
**Path Declarations**: unchanged. 12 declared files (< 300).

1. Implement the whole plan above, with the session-462 points and its CF-02 tests.
2. Never edit `internal/store/read_masters.go`, `read_classification.go` or `read_definitions.go`, or `internal/present/master.go` (accepted PF-08). Never edit `internal/fixtures/*` (accepted PF-06), `fixtures/default/*` (PF-07, shared only by PF-16), or `internal/api/v1/api.go`, `routes.go` or `routes_test.go` (PF-10). If the presenter or reader returns a wrong input ID, record a reconciliation request against PF-08 (README rule 12) instead of working around it in the handler.
3. Record the inferred category-deletion rule in the progress log as an INF candidate. PF-27 appends it to `notes.md`.
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-20/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0, the CF-02 create/update/get tests PASS, and the `not implemented` rg prints nothing
- [ ] The progress log has the intent snapshot, pre/post hashes of the four shared files, the INF candidates, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-06, PF-08 (accepted at `44e40cc`) and PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 43-55 and 73-82; `architecture.md#fixtures`, the general master item input ID data flow (CF-02).

**Intent**: unchanged. Implement 23 operations (categories, folders, workflows/routes, general masters) and close the CF-02 API hop.

**Session-464 changes**:
1. `internal/api/v1/categories.go`, `folders.go`, `definitions.go` and `general_masters.go` are in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty. Replace bodies only. Never edit `api.go`, `routes.go`, `routes_test.go`, `internal/testutil/*`, `internal/server/*`, `internal/fixtures/*` or `fixtures/default/*` (PF-16).
2. Fixture ids: PF-16 rewrote `fixtures/default/*.json` in canonical form in wave A. Only array order changed and zero-value keys were dropped; ids and codes are unchanged. Tests that read `inputs[].id` from `fixtures/default/general-masters.json` must parse the file, not hard-code positions.
3. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/api/v1/...`.
   - Log each result.
   - Prefixes: unexported identifiers start with `pf20`. New exported store functions start with `Category`, `Folder`, `Workflow`, `Route` or `GeneralMaster`.
4. Guard: the manifest `-run 'TestPF10'` line must PASS. Logs are `/tmp/pf20-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-20/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0; the CF-02 create/update/get tests PASS; PF10 guard PASS; the `not implemented` rg empty
- [ ] INF candidates recorded in the progress log; pre/post hashes of the four transferred files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B1 (with PF-18, PF-19, PF-21). It is dispatched as soon as PF-16 is accepted. It may run while PF-14a/b/c are still running. **Depends on**: PF-06, PF-08 (accepted at `ea385c7`), PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (12 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: implement the 23 operations, close the CF-02 API hop (create assigns input IDs, update preserves them, get returns the fixture IDs), and keep the PF10 guard green. Logs are `/tmp/pf20-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-20/attempt-<n>/`.

**Pitfalls added in s465**:
- `internal/store` may still be edited by PF-14a/b/c (`Eng*` names). Use your `Categor*`/`Folder*`/`Definition*`/`Master*` store names.
- Never touch `internal/engine/*`, `fixtures/default/*` or `internal/fixtures/*` (PF-16).

### Done Criteria (session 465)

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0, including the CF-02 create/update/get tests; the `not implemented` check exit 0; gofmt check exit 0
- [x] INF candidates recorded; pre/post hashes of the four transferred files; source-write compile gates recorded; the earlier PF-19 artifact blocker is resolved

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). Handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: 2026-10-21 (s465 attempt 1)
**Intent snapshot**: Implemented the four transferred PF-10 handler files; added category/folder/definition/master store queries and mutations; added four `package v1_test` API test files. Plan and design aligned: PF-20 owns the 23 operations and CF-02 input ID API hop; PF-16 was accepted in `fanoutItem.acceptedPlanIds`, satisfying the declared dependency. No downstream wiring was pulled forward.

**Tasks Completed**:
- Implemented list/create/update/delete categories with permission, validation, pagination, and the inferred used-category rejection.
- Implemented folder list/create/get/update/delete, detail presenters, path resolution, root edit protection, cycle guard, and delete guards for children/workflows/routes.
- Implemented read-only routes and workflows with status filters, documented route folder filter, sorting, pagination, code-or-ID detail resolution, and PF-08 detail presenters.
- Implemented general master and item list/create/get/update/delete, permission split, enum/field/value/date validation, and persisted input ID generation/preservation. Create/update/get CF-02 regression test reads the `cc-sales` expected ID from `fixtures/default/general-masters.json` and confirms the DB `input_ids` column.
- Added API tests for pagination, field/response keys, permission errors, delete guards, invalid enums/values, default master item sort, and CF-02 create/update/get.
- Fixed single-connection SQLite list deadlocks by closing list cursors before nested store reads.

**Four transferred handler hashes**:
- `categories.go`: pre `f9275143aede36b61cffdc353e17790fa95e32c3c80a1b266bd641d2d3a0115e`; post `90469102bae513c61b8b19fdbe586045eae3b61dc5f42cc1ca58500b3f11c853`.
- `folders.go`: pre `8628656cd1672c182e79d50ec15bb9beadff1b5960cb308761a2d9de4a4cbbbe`; post `4c9c58f659efe2fb64348c2e69e43ba917268d93222d5a8301958b1bba9a25c6`.
- `definitions.go`: pre `c46dacca5cadf7da934235f9d0962c2df3d3212e24ce2599bfa8040336a0a0c9`; post `35a6fa1b337ab438d74e8ba1a133c95aab3a7cb02af48feff1fb0aee4fa1ae6b`.
- `general_masters.go`: pre `2cfa4a1ef82a396374a431f71485af55ab20093fd2474886339f6a76cee8e2a0`; post `5134846277df1435b5d1b8b7fc0da6852c5f4a345e4bfae8eb194045f28787ef`.

**Compile gates after source writes**: all logs are in `tmp/punchflow-kickflow-dummy-s465/PF-20/attempt-1/`. Initial handlers/store writes compiled except transient gates that caught (a) a concurrent PF-19 compile error in `internal/api/v1/proxies.go`, and (b) two local helper-call errors during general-master helper completion. They were corrected/reconciled; subsequent source-write gates passed (`edit1-build`, `edit2a-build`, `edit14a-build`, `edit14b-build`, `edit14c-build`, `edit18-build`, `edit21b-build`, and final build). The route-filter signature intermediate gate (`edit21a-build`) failed because its handler caller had not yet been updated; the next gate passed. The cursor fixes each compiled successfully.

**Verification** (final source; complete logs copied under the attempt directory):
- `go build ./...`: exit 0, `pf20-s465-final-build.log`.
- `go vet ./internal/api/v1/...`: exit 0, `pf20-s465-final-vet.log`.
- `go test -count=1 -v -race -run 'Test(Categor|Folder|Workflow|Route|GeneralMaster|Master)' ./internal/api/v1/...`: exit 0, PASS=5 FAIL=0 SKIP=0, `pf20-s465-final-test.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2 FAIL=0 SKIP=0, `pf20-s465-final-pf10guard.log`.
- `gofmt -l` on all 11 owned Go files: exit 0, no output; not-implemented must-pass check: exit 0, no matches. Aggregate: `gate-summary.txt`.

**Inferred behavior candidate for PF-27**: deleting a category referenced by a fixture workflow `categoryCodes` returns 422 `validation_failed` (`categoryId`); route this to `design-docs/specs/notes.md` as an INF candidate.

**Prior failed attempts**: the first test attempt stalled on the SQLite cursor deadlock and was interrupted; two subsequent runs failed only on incorrect folder test expectations, which were corrected. The original first-run tee output was overwritten during retry and is unavailable; `prior-attempt-summary.md` records this evidence limitation. Final source verification above is fresh and complete.

**Earlier reconciliation**: the transient PF-19 compiler failure in `internal/api/v1/proxies.go` cleared on a later shared-tree build. A later PF-19 artifact failure is recorded in the final self-check below; no foreign files were edited.

### Session: 2026-10-21 (s465 final self-check)
**Naming correction**: updated the new exported list functions to `Category*`, `Folder*`, `Route*`, `Workflow*` and `GeneralMaster*`; all new unexported store helpers now use the `pf20` prefix. Duplicate-key detection is local to PF-20 API handlers as `pf20IsUniqueViolation`.

**Current-source verification**:
- `go vet ./internal/api/v1/...`: exit 0, `attempt-1/pf20-s465-final-vet.log`.
- `go test -count=1 -v -race -run 'Test(Categor|Folder|Workflow|Route|GeneralMaster|Master)' ./internal/api/v1/...`: exit 0, PASS=5 FAIL=0 SKIP=0, `attempt-1/pf20-s465-final-test.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2 FAIL=0 SKIP=0, `attempt-1/pf20-s465-final-pf10guard.log`.
- `gofmt -l` on the 11 owned Go files: exit 0, empty; `not implemented` and legacy-name check: exit 0, no matches.
- Final `go build ./...` exits 1 only in foreign PF-19 evidence source `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-2/teams_mutated.go` (`undefined: API`); complete output with `exit=1`: `attempt-1/pf20-s465-gate-after-gofmt.log`. Compile gates after the naming correction and gofmt show the same PF-19 failure after local PF-20 compile errors cleared.

**Updated hashes after naming correction**: `classification.go` 8fd81c67c3392cad22533d89fe1f63fde45f9cdb3cd7ebf220a564f46abffd4a; `definitions.go` 25eba08650a767add6bef271304079f514af9745b17923b8c17b20a9e5ee4e69; `masters.go` 53ea708c3fbeab8eb37b3f2a9c91f4f5c9fed95d3dc4661f2bf86bf3cba1bb76; `categories.go` a9923b48dd64f27296f961b76e606676cc20d821cf71d836e8d78f4c1f68133e; `folders.go` 56e3ecd170d955379b4633cfccc626150e3be5f2d0fdb9b131cc224981ca07b1; `definitions.go` 96b76d7aa5c37184f7d40e0629e81e9c3e5828279290732cbe02dfe1fa360110; `general_masters.go` ecf703bed45f5143ed73eec5863144e1fe590b01935a2f4ee179422f82e23bf2.

**Reconciliation request**: PF-19 must remove or repair its generated `teams_mutated.go` artifact, then PF-20 must rerun `go build ./...` on the same combined tree. No foreign path was edited.

### Session: 2026-10-21 (s465 attempt 2, blocker rerun)
**Dependency readiness**: PF-16 is present in the runtime-owned `acceptedPlanIds` for PF-20; implementation and focused tests were already present. No source or test edits were needed in this continuation.

**Current-tree verification**: All logs are complete and copied under `tmp/punchflow-kickflow-dummy-s465/PF-20/attempt-2/`.
- `go build ./...`: exit 0, `build.log` (also `/tmp/pf20-s465-rerun-build.log`). This resolves the previously logged PF-19 artifact blocker; PF-19 renamed the temporary source to `_teams_mutated.go`.
- `go vet ./internal/api/v1/...`: exit 0, `vet.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0, `pf10guard.log`.
- `go test -count=1 -v -race -run 'Test(Categor|Folder|Workflow|Route|GeneralMaster|Master)' ./internal/api/v1/...`: exit 0, PASS=5, FAIL=0, SKIP=0, including `TestGeneralMasterItemsAndInputIDHop`, `test.log`.
- `gofmt` cleanliness check across the 11 owned Go files: exit 0, no files listed. The must-pass `not implemented` check exited 0 with no matches.
- `pre-verification-sha256.txt` and `post-verification-sha256.txt` match for all 11 owned Go files; no implementation drift occurred during the rerun.

PF-20 implementation criteria are complete. Formal combined-tree integration review and downstream workflow review remain assigned to later steps.
