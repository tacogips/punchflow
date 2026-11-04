# argus #523: Admin createWorkflow Implementation Plan

**Plan ID**: PLAN-523-CREATE-WORKFLOW
**Status**: Ready
**Wave**: 1 (no dependencies; runs in parallel with PLAN-520-RFC3339 and PLAN-DOCS-README)
**Design Reference**: design-docs/specs/design-auth-admin.md#runtime-workflow-creation-createworkflow-argus-523
**Created**: 2026-11-04
**Last Updated**: 2026-11-04

---

## Write Paths

- internal/admin/routes.go
- internal/admin/routes_test.go
- internal/admin/workflows.go
- internal/admin/workflows_test.go
- impl-plans/active/argus-523-admin-create-workflow.md

New files: `internal/admin/workflows.go` and `internal/admin/workflows_test.go` do not exist yet and are created by this plan.

## Shared Paths

none

## Artifact Roots

none. This plan installs no tools, downloads nothing and generates no build output; the Go build cache stays outside the repository.

## Intent and context

Argus tests need to add a workflow definition to a running punchflow without restarting it. Today `POST /admin/api/workflows` falls through to the reserved-namespace 404 registered in `internal/admin/routes.go:Register`. Add the `createWorkflow` admin operation exactly as the accepted design specifies (processing steps 1-7).

Existing code to imitate:

- Route rows: the `internal/admin/routes.go:operations` array.
- Auth helper: `internal/admin/tokens.go:pf24Principal`, which returns 401 `invalid_access_token` or 403 `missing_permission` through `Principal.Has`.
- Audit helper: `internal/admin/tokens.go:pf24Audit(ctx, q, p, action, data)`.
- Transactional handlers with audit: `internal/admin/webhooks.go:createWebhook` (lines 60-90 use `a.d.DB.WithTx` and `pf24Audit(r.Context(), tx, ...)`).
- Fixture validation primitives: `internal/fixtures/export.go:Export(ctx, q) (map[string][]byte, error)`, `internal/fixtures/parse.go:Parse(fs.FS) (*Set, error)`, `internal/fixtures/validate.go:Validate(*Set) error`, which returns `*fixtures.ValidationError{Problems []Problem{File, Path, Message}}`.
- Fixture workflow insert SQL: `internal/fixtures/load.go:306-315`.
- Response presenter: `internal/present/workflow.go:WorkflowDetail(ctx, q, id)`, as used by `internal/api/v1/definitions.go:getWorkflow`.
- Folder code to ID: `internal/store/ids.go:ResolveID(ctx, q, "folders", code)`.
- Restricted grant check: `auth.Principal.AllowsAny(target, permissions...)`, as in `internal/api/v1/scope.go:requireTarget`.
- Test harness: `internal/testutil` (`testutil.New`, `env.Do`, `testutil.AssertError`, personas `Admin` = `pf_pat_dev_admin` and `Employee` = `pf_pat_dev_employee`, and `env.DB` for direct store writes). Imitate `internal/admin/fixtures_test.go`.

## Non-goals

- No `GET /admin/api/workflows` and no `DELETE /admin/api/workflows/{id}` (design decision D8).
- No change to `internal/fixtures/*` (the validator must stay unchanged), `internal/store/*`, `internal/present/*` or `internal/api/v1/*`.
- No new exported helpers in other packages. Everything new lives in `internal/admin/workflows.go`.
- No README or design-doc edits. PLAN-DOCS-README owns README.md.
- No `--enable-admin-reset` gate.

## File-level changes

### internal/admin/routes.go (edit)

Add one row, placed after `resetFixtures` or grouped logically:

`{Name: "createWorkflow", Method: http.MethodPost, Pattern: "/admin/api/workflows", Handle: (*API).createWorkflow}`

### internal/admin/routes_test.go (edit)

In `TestOperationsContractAndUnknownRoute`, change the expected count from 23 to 24 in both the condition and the message. Add an assertion that an operation named `createWorkflow` with `POST /admin/api/workflows` exists.

### internal/admin/workflows.go (new, target under 300 lines)

Add `func (a *API) createWorkflow(w http.ResponseWriter, r *http.Request)` with private helpers. Required behavior, in this order:

1. `p, ok := pf24Principal(w, r, model.PermissionWorkflow)`. This gives 401 or 403.
2. Decode in two stages:
   - `var raw json.RawMessage; httpx.DecodeJSON(r, &raw)`. This reuses the content-type check (400 `invalid_content_type`), the size limit (`payload_too_large`) and malformed JSON (400 `invalid_json`); see `internal/httpx/json.go:DecodeJSON`.
   - An empty `raw` (DecodeJSON returns nil for an empty body) -> `kferr.InvalidJSON()`.
   - Strict-decode `raw` into `model.WorkflowDef` with `json.NewDecoder(bytes.NewReader(raw))` plus `DisallowUnknownFields()`. An unknown-field error -> `kferr.InvalidParameter("unknown field <name>")`. A `*json.UnmarshalTypeError` -> `kferr.InvalidParameter(...)`. A trailing second JSON value -> `kferr.InvalidJSON()`.
   - `httpx.DecodeJSON` ignores unknown fields, so do not decode directly into WorkflowDef with it.
   - If `def.ID == ""`, set `def.ID = store.NewID()` (UUID v4).
3. `a.d.DB.WithTx(ctx, func(tx *store.Tx) error {...})`, which wraps steps 3-6. Do not use `a.d.DB.Q()` inside the callback, because it deadlocks (see the comment at `internal/store/db.go:23`).
   - Step 3, duplicate check: `SELECT id, code FROM workflows WHERE id=? OR code=?` over all statuses. If there is a match, return `kferr.New(http.StatusConflict, "conflict", "workflow id already exists")` or the message for `code`, as appropriate.
   - Step 4, validation:
     - `files, err := fixtures.Export(ctx, tx)`, then `set, err := fixtures.Parse(fstest.MapFS{name: {Data: bytes}})`. `testing/fstest` is stdlib and acceptable; a tiny local `fs.FS` is equally fine.
     - `base := problems(fixtures.Validate(set))`, then `set.Workflows = append(set.Workflows, def)` and `cand := problems(fixtures.Validate(set))`.
     - New problems are the multiset difference `cand - base` over the full `fixtures.Problem` struct (File, Path, Message): count base occurrences in a `map[fixtures.Problem]int`, then walk cand and decrement.
     - If any new problems remain, return `kferr.InvalidParameter(joined)`, where `joined` is the new problems as `file: path: message` joined by `"; "` in cand order (which is already sorted).
     - The `problems` helper uses `errors.As` to reach `*fixtures.ValidationError`. A nil error gives an empty slice. Any other error (export or parse failure) is returned as an internal error, not a 400.
   - Step 5, folder scope: `folderID, err := store.ResolveID(ctx, tx, "folders", def.FolderCode)`. Validation already guarantees the code exists. Then if `!p.AllowsAny(folderID, model.PermissionWorkflow)`, return `kferr.MissingPermission("Missing permission")`.
   - Step 6, persist: insert with the same SQL as `internal/fixtures/load.go:311`. `definition_json` is `string(json.Marshal(def))`, the same as the loader's local `jsonValue` closure at `internal/fixtures/load.go:90`. Timestamps are `store.Millis(a.d.Clock.Now())`. If the insert error string contains `UNIQUE constraint failed`, map it to the same 409. Then `pf24Audit(ctx, tx, p, "workflow.create", map[string]string{"workflowId": def.ID})`.
4. After a successful commit: `j, err := present.WorkflowDetail(ctx, a.d.DB.Q(), def.ID)` and `httpx.WriteJSON(w, http.StatusCreated, j)`. Write any error with `kferr.Write`.

Pitfalls:

- Do not filter problems by path prefix. That was design finding DR-001: custom validations use `workflows[n]...` paths, and collisions can be reported against other files.
- Append the candidate LAST. Inserting it elsewhere shifts existing paths and breaks the diff.
- Run the duplicate check before validation, so that a duplicate workflow id or code is a 409, not a 400 `duplicate id` or `duplicate code`.
- Every rejection must return from the WithTx callback so that nothing is inserted or audited. Never write the HTTP response inside the callback.
- `a.d.Clock` is wired in production (`internal/server/server.go:237`) and by testutil. Do not change server wiring.
- Send test requests with `Content-Type: application/json`. `env.Do` already does this; check before using `DoRaw`.

### internal/admin/workflows_test.go (new)

Package `admin_test`. Build the candidate with a helper:

- Read `fixtures/default/workflows.json` (path relative to the repo root; from the test directory use `../../fixtures/default/workflows.json`, or locate the root the way `internal/testutil` locates fixtures) and take element 0 (`expense-claim`) as a `map[string]any`.
- Set `code` to a fresh value such as `argus-asset-registration` and `name` to a new name.
- Recursively replace every `"id"` string value with a fresh UUID (`store.NewID()` or crypto/rand), including the top-level id.

Cases (situation -> expected):

- Admin POST of the valid candidate -> 201, and the body `id` equals the candidate id. `GET /v1/workflows` then contains the code, and `GET /v1/workflows/{id}` returns 200. Creating a ticket on it as Employee succeeds with 200: imitate the `POST /v1/tickets` body from `internal/api/v1/ticket_required_nested_test.go`, supplying the form values that expense-claim requires. The audit log has a `workflow.create` row; query `env.DB.Q()` on the `audit_logs` table.
- Candidate posted without `id` (top-level id removed) -> 201 with a generated UUID id.
- No Authorization header (use `env.DoRaw` with a request without a token, or the persona helper's empty-token path) -> 401 `invalid_access_token`.
- Employee persona -> 403 `missing_permission`.
- Body `{` -> 400 `invalid_json`. Body with an extra field `"bogus": 1` -> 400 `invalid_parameter`.
- `routeCode: "no-such-route"` -> 400 `invalid_parameter`, message contains `routeCode`. `folderCode: "no-such-folder"` -> 400, message contains `folderCode`.
- Form custom validation with `combinationType: "bogus"` on a form field (add a `customValidations` entry to a form field) -> 400, message contains `workflows[` and `combinationType`. The same check for a detail (slip) custom validation, if the cloned workflow has slip fields; otherwise clone one that has them, or add a slip section, following `internal/fixtures/validate_custom_test.go`.
- 51 valid form custom validations -> 400, message contains `limit of 50 form validations`.
- Candidate top-level `id` set to a seeded fixture token ID (read from `fixtures/default/tokens.json`) -> 400, message contains `tokens.json` and `duplicate id`. Candidate id set to a seeded user ID (from `fixtures/default/users.json`) -> 400, message contains `duplicate id`.
- Duplicate `code` `expense-claim` with fresh ids -> 409 `conflict`. Duplicate top-level `id` equal to the existing expense-claim workflow id with a fresh code -> 409 `conflict`.
- After every 400 and 409 case: the `/v1/workflows` total count is unchanged, and `audit_logs` has no new `workflow.create` row.
- Unrelated-state tolerance:
  - Through `env.DB`, insert a webhook row whose `workflow_ids` JSON names a random nonexistent UUID. Copy the column list from `internal/fixtures/load.go`'s webhook insert, or use the admin webhook API and then UPDATE `workflow_ids`.
  - Assert that `fixtures.Validate` on the export now fails with a `webhooks.json` problem.
  - POST a valid candidate -> 201.

## Invariants

- `internal/fixtures/validate.go` and `validate_custom.go` are unchanged.
- The existing admin operations keep their names, methods and patterns. Unknown admin routes still return JSON `endpoint_not_found`.
- Rejected requests leave no workflow row and no audit row.
- Touched files stay under 1000 lines.

## Concurrency and drift protocol

Before each edit, freshly read the target and record `shasum -a 256`. Record it again after the edit. If an owned file changed unexpectedly, stop and report drift. Touch only the four files above and this plan file. No git write operations.

## Acceptance criteria

- [ ] The `createWorkflow` row is registered, and `routes_test.go` expects 24 operations including it.
- [ ] Processing steps 1-7 are implemented in `internal/admin/workflows.go`, with the baseline-diff validation.
- [ ] Every test case listed above exists and passes.
- [ ] `gofmt -l` is clean and `go vet` passes for `internal/admin`.

## Verification (foreground; record exit status and log path)

1. `gofmt -l internal/admin 2>&1 | tee /tmp/plan523-gofmt.log`: empty output, exit 0.
2. `go test -count=1 ./internal/admin/... 2>&1 | tee /tmp/plan523-admin.log`: `ok`, exit 0.
3. `go test -count=1 ./internal/fixtures/... ./internal/api/v1/ -run 'Workflow|Fixture' 2>&1 | tee /tmp/plan523-related.log`: `ok`, exit 0. This shows no regression in the reused packages. PLAN-520 writes a test file in `internal/api/v1/` during the same wave. If this command fails to compile on a file this plan does not own, rerun it once that sibling's file is complete before reporting drift. Never edit that file.
4. `go vet ./internal/admin/... 2>&1 | tee /tmp/plan523-vet.log`: exit 0.

The full `go build ./... && go vet ./... && go test ./...` gate belongs to reconcile and integration review.

## Progress Log

### Session: 2026-11-04 (plan created)
**Tasks Completed**: Plan authored from the accepted design (DR-001 resolution included).
**Notes**: Implementation not started.
