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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf20-build.log
go vet ./internal/api/... ./internal/store/... 2>&1 | tee /tmp/pf20-vet.log
go test -race -run 'Test(Categor|Folder|Workflow|Route|GeneralMaster|Master)' ./internal/api/v1/... 2>&1 | tee /tmp/pf20-test.log
gofmt -l internal/api/v1/categories*.go internal/api/v1/folders*.go internal/api/v1/definitions*.go internal/api/v1/general_masters*.go internal/store/classification.go internal/store/masters.go internal/store/definitions.go   # prints nothing
```

## Done Criteria

- [ ] 23 operations implemented; tests pass with `-race`; gofmt clean; inferred rules recorded in the progress log

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. Handler files are shared with PF-10 (stub creator).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
