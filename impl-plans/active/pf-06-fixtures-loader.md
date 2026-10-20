# PF-06 Fixtures Loader Implementation Plan

**Status**: Ready
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-06
**Design Reference**: design-docs/specs/architecture.md#fixtures; design-docs/specs/notes.md INF-04, INF-14, INF-18; design-docs/specs/command.md (fixtures subcommands)
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Parse, validate, load, reset and export fixture sets: JSON definitions of tenant, org, users, masters, workflows, routes, proxies, tokens and webhooks, in the struct shapes pinned by `internal/fixtures/types.go` and `internal/model/definition.go` (PF-02). The default set (`fixtures/default`, PF-07) is written in parallel, so this plan tests against its own small `testdata` set.

## Non-goals

- Do not write `fixtures/default/*` (PF-07) or change `types.go` / `internal/model` (frozen; log reconciliation requests).
- No CLI or HTTP wiring (PF-16, PF-24). Tickets, comments, files, audit logs, sessions and webhook history are never loaded or exported.

## Files

`internal/fixtures/parse.go`, `validate.go`, `load.go`, `export.go`, `fixtures_test.go`, `validate_test.go`, `testdata/minimal/*.json` (a small valid set: 1 tenant, 2 grades, 3 users, 1 chart with 2 teams, 1 super admin role, root folder, 1 category, 1 master with 1 field and 2 items, 1 route with 2 steps, 1 workflow, 1 proxy approver, 1 PAT token, 1 webhook), `testdata/invalid/*` (one subdirectory per invalid case).

## Path Declarations

### writePaths
- `impl-plans/active/pf-06-fixtures-loader.md`
- `internal/fixtures/parse.go`
- `internal/fixtures/validate.go`
- `internal/fixtures/load.go`
- `internal/fixtures/export.go`
- `internal/fixtures/fixtures_test.go`
- `internal/fixtures/validate_test.go`
- `internal/fixtures/testdata`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

```go
type Set struct {
    Tenant TenantFile; Grades []GradeFixture; Users []UserFixture; OrgCharts []OrgChartFixture
    Roles []RoleFixture; Categories []CategoryFixture; Folders []FolderFixture
    GeneralMasters []GeneralMasterFixture; Routes []model.RouteDef; Workflows []model.WorkflowDef
    ProxyApplicants, ProxyApprovers []ProxyFixture; Tokens []TokenFixture; Webhooks []WebhookFixture
}
type Problem struct{ File, Path, Message string }
type ValidationError struct{ Problems []Problem }  // Error() joins "file: path: message" lines
type Summary struct{ Counts map[string]int }        // table name -> rows inserted

func Parse(fsys fs.FS) (*Set, error)                // reads the known file names at fsys root; ignores other files (e.g. .gitkeep); missing optional files -> empty
func Validate(s *Set) error                          // *ValidationError
func Load(ctx context.Context, db *store.DB, fsys fs.FS, clock kftime.Clock) (Summary, error)        // requires db.IsEmpty
func Reset(ctx context.Context, db *store.DB, fsys fs.FS, filesDir string, clock kftime.Clock) (Summary, error)
func Export(ctx context.Context, q store.Querier) (map[string][]byte, error) // file name -> canonical bytes
func WriteDir(dir string, files map[string][]byte) error
func DefaultFS(root fs.FS) (fs.FS, error)            // fs.Sub(root, "fixtures/default") for punchflow.DefaultFixtures
```

Required files: `tenant.json`, `grades.json`, `users.json`, `organization-charts.json`, `roles.json`, `folders.json`. Optional (default empty array): the rest.

## Validation Rules

- JSON decoded with `DisallowUnknownFields` -> unknown keys are problems.
- Every `id` is a UUID, unique within its file, and globally unique across all ids; codes unique per entity type (teams: per chart; items: per master).
- Enums are valid (`model/enums.go`): user status, plan, mode, step types, action types, condition types, symbols, field types, token kinds, event types.
- References by code resolve: user/team/grade/folder/category/route/master/item/form field (route step condition `variable` must be a form-field code of some workflow that uses the route), workflow codes in proxies and webhooks, `memberCodes`.
- Exactly one org chart with `current: true`; exactly one role with `editable: false` (super admin) and at least one member; exactly one folder without `parentCode` (root, `editable: false`); no parent cycles in teams or folders.
- INF-14: each route step has exactly one of `requiredApprovalsNumber > 0` or `requiredApprovalsPercent > 0`.
- INF-18: reject `combinationType: "custom"` anywhere and `fieldKey: "other_variable"`.
- Every workflow `routeConditions` list is non-empty and ends with `always` or `field_otherwise`; a condition has exactly one of `routeCode` and `errorMessage`.
- Token: kind `pat` needs `userCode`; kind `sat` must not have one; `token` non-empty.
- All problems are collected (not fail-fast) and sorted by file then path.

## Load / Reset / Export

- `Load`: Parse + Validate, then insert everything in one `WithTx` in FK order, using the fixture ids. Timestamps = `clock.Now()`. Tokens: `token_hash = sha256 hex(token)` (same algorithm as `auth.HashToken`; do not import `internal/auth`, reimplement the one-liner) and `fixture_plaintext = token`. Workflows/routes: `definition_json = json.Marshal(def)`; `folder_id` resolved from `folderCode`. Users with status `invited` get a `user_invitations` row expiring 7 days after now. Webhooks: workflow codes -> ids.
- `Reset`: in one tx `store.TruncateAll` then the same inserts; after commit, delete every file in `filesDir` (ignore a missing dir). Validation failure -> nothing changes.
- `Export`: read rows back into the fixture structs and emit `json.MarshalIndent(v, "", "  ")` + `"\n"` per file. Arrays sorted by `code` (tokens/webhooks/proxies/charts/roles by `id`, which have no code); ordered collections (sections, fields, steps, route conditions, options, memberships in their team order) keep stored order. Only tokens with non-NULL `fixture_plaintext` are exported. Always emit every file name, `[]` for empty lists.

## Key Points and Pitfalls

- Canonical form is the byte format PF-07 hand-writes and PF-16 asserts against `fixtures/default`: 2-space indent, struct field order, trailing newline, `null` for nil pointers (do not use `omitempty` behavior on export unless the struct tag already has it).
- Use the tx `Querier` for every insert inside `WithTx`. Never call `db.Q()` inside the callback (single connection deadlock).
- `Load` on a non-empty DB -> error `"database is not empty"` (CLI maps it to exit 1).

## Tests (input -> expected)

- `testdata/minimal` -> Validate nil; Load counts match the file contents
- `Export(Load(minimal))` -> `Load` into a fresh DB of that export -> second `Export` byte-identical to the first (stability)
- `Export(Load(minimal))` byte-identical to `testdata/minimal` (keep the testdata in canonical form; generate it once with `Export`, then review it)
- Reset after inserting extra rows (a ticket row, a file in filesDir) -> only fixture data remains and filesDir is empty
- invalid cases each -> `*ValidationError` containing the expected file and path: unknown key, duplicate id, bad enum, dangling userCode, two current charts, `custom` combination, `other_variable`, step with both number and percent, route conditions not ending with an else, pat token without userCode
- Load into non-empty DB -> error, no changes

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf06-build.log
go vet ./internal/fixtures/... 2>&1 | tee /tmp/pf06-vet.log
go test ./internal/fixtures/... 2>&1 | tee /tmp/pf06-test.log
gofmt -l internal/fixtures   # prints nothing (types.go untouched)
```

## Done Criteria

- [ ] Contract symbols exist; all rules implemented with tests; round-trip and stability tests pass; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
