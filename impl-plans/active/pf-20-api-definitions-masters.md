# PF-20 /v1 Categories, Folders, Routes, Workflows and General Masters Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-20
**Design Reference**: design-docs/specs/design-api-coverage.md rows 43-55 and 73-82; design-docs/references/kickflow-api.md section 3 "Categories", "Folders", "Routes", "Workflows", "General masters"; notes.md DIV-05, INF-02, INF-29, INF-42
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

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

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-13). Handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
