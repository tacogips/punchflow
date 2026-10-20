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

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 2. **Depends on**: PF-02 (phase 1: Go enum lists, `validate.go` enum checks, `RouteTarget.GradeSymbol *string`, `WorkflowSection.Code/Conditional/CombinationType`).
**Design**: `design-docs/specs/architecture.md#fixtures` (CF-02 input ID data flow, canonical form, round trip); `design-api-coverage.md` enum contract rule 5; `notes.md` CF-01, CF-02.

### State at 410d3d8 (audited)

- `parse.go`, `validate.go`, `load.go` and `export.go` exist. There are no `fixtures_test.go` or `validate_test.go`, so nothing in this package was ever tested.
- CF-02 is open:
  - The `general_master_items` INSERT at `load.go:261` has no `input_ids` column.
  - `export.go:274` builds `MasterInputFixture{FieldCode, Value}` without `ID`.
  - An item with no inputs exports `"inputs": null`.
  - `testdata/minimal/general-masters.json` was produced by that buggy export: `"id": ""` for an input, `"inputs": null`.
- CF-01 data drift: `testdata/minimal` (user/route/workflow `status: "active"`) and the 10 `testdata/invalid/*` directories contain about 39 non-target `"active"`/`"member"` literals.

### File-level changes

1. `load.go`, master items: build `inputIDs := map[fieldID]inputID` next to the existing `inputs` map, using `in.ID`. Insert it into the `input_ids` column in the same INSERT (JSON via the existing `jsonValue` helper). An item with no inputs writes `{}` for both.
2. `export.go`, master items:
   - Also select `input_ids`, decode it, and set `MasterInputFixture.ID = inputIDs[fieldID]`.
   - Always emit `Inputs` as a non-nil slice (`[]` when empty).
   - Keep the sort by `FieldCode`.
   - A stored value whose field ID has no input ID is an export error naming the item code. Never write `""`.
3. `validate.go`: no change in this session (PF-02 owns the enum checks this phase; PF-07 calls `Validate` in parallel). If a test proves a non-enum defect, fix it, and log the hash and the reason.
4. `testdata/minimal/*.json`: make every enum value canonical per the design table:
   - user `status` `activated`, `userType` `normal`, route `status` `visible`, workflow `status` `visible`, `gradeSymbol` `null`, no `sectionType: "conditional"`
   - give the input a fixed UUID (not `""`)
   - item 2 gets `"inputs": []` or one input with its own UUID

   Then fix the load/export bugs, run `Export(Load(minimal))` once in a scratch test, review the output, and store it as the canonical bytes.
5. `testdata/invalid/<case>/`: rebuild every case directory as a copy of the canonical `minimal` with exactly one target defect, so each case produces only its intended problem.
   - Keep the existing 10 cases: unknown-key, duplicate-id, bad-enum, dangling-user-code, two-current-charts, custom-combination, other-variable, both-thresholds, route-no-else, pat-without-user.
   - Add 5 cases: `step-type-grade`, `permission-audit-read`, `event-ticket-submitted` (a webhook with that event), `user-type-member`, `route-status-active`.
6. New tests `fixtures_test.go` and `validate_test.go` (`package fixtures`; test names start with `TestLoad`, `TestExport`, `TestReset` or `TestValidate`; helpers prefixed `pf06`). They cover the original "Tests" section plus the session-462 cases below.

### Key points

- Use only the tx Querier inside `WithTx` (single-connection deadlock).
- Canonical bytes: `json.MarshalIndent(v, "", "  ")` + `"\n"`. Key order is the struct field order of `types.go`/`definition.go` as changed by PF-02. Nil pointers are `null`. `omitempty` fields are omitted when empty.
- Do not edit `types.go` or `internal/model` (frozen). Log a reconciliation request instead.
- Do not touch `fixtures/default` (PF-07) or `default_roundtrip_test.go` (PF-16).

### Tests (input -> expected), in addition to the original list

- `TestValidateMinimal`: `Validate(Parse(testdata/minimal))` -> nil
- `TestValidateInvalidCases`: each of the 15 invalid cases -> `*ValidationError` whose `Problems` are exactly one entry, with the expected `File` and `Path`. One exception: `duplicate-id` may report the two sites of the duplicate.
- `TestLoadExportRoundTripMinimal`: `Export(Load(minimal))` is byte-identical to each `testdata/minimal/*.json`; a second load+export is byte-identical again
- `TestExportMasterInputIDs`: after Load, `SELECT input_ids` for ITEM1 = `{"<fieldId>":"<inputId>"}`; Export -> `inputs[0].id` equals the fixture input ID; an item without inputs exports `"inputs": []`
- `TestExportRouteTargetGradeSymbolNull`: a target with `gradeSymbol: null` round-trips as `null`
- `TestResetClearsStateAndFiles`, `TestLoadNonEmptyDatabase` (original list)

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf06-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/fixtures/... 2>&1 | tee /tmp/pf06-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'Test(Load|Export|Reset|Validate)' ./internal/fixtures/... 2>&1 | tee /tmp/pf06-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf06-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf06-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf06-s462-test.log)"
gofmt -l internal/fixtures/parse.go internal/fixtures/validate.go internal/fixtures/load.go internal/fixtures/export.go internal/fixtures/fixtures_test.go internal/fixtures/validate_test.go   # prints nothing
rg -n '"status": *"active"|"userType": *"member"|"sectionType": *"conditional"|"gradeSymbol": *""|"id": *""|"inputs": *null' internal/fixtures/testdata/minimal   # prints nothing (exit 1)
wc -l internal/fixtures/*.go | sort -n | tail -2   # every file < 1000
```

The `-run` pattern excludes PF-02's `TestEnumRule*` (it reads `fixtures/default`, which PF-07 edits in parallel) and PF-16's later default round-trip test. Evidence: exit 0; FAIL=0; SKIP=0; PASS includes the 15 invalid subtests.

### Done Criteria (session 462)

- [ ] CF-02 load/export half closed: `input_ids` written on load and restored as `inputs[].id` on export; `inputs` never `null`; tests prove it
- [ ] `testdata/minimal` is canonical and round-trips byte-identically; 15 invalid cases each yield only their target problem
- [ ] `fixtures_test.go` and `validate_test.go` exist and pass uncached with counts recorded; build/vet exit 0; gofmt clean; files < 1000 lines

## Verification (session 460, superseded)

```bash
go build ./... 2>&1 | tee /tmp/pf06-build.log
go vet ./internal/fixtures/... 2>&1 | tee /tmp/pf06-vet.log
go test ./internal/fixtures/... 2>&1 | tee /tmp/pf06-test.log
gofmt -l internal/fixtures   # prints nothing (types.go untouched)
```

## Done Criteria (session 460, superseded by the session 462 Done Criteria above)

- Contract symbols exist; all rules implemented with tests; round-trip and stability tests pass; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-06 implementation attempt 1
**Intent snapshot**: Added in `tmp/punchflow-kickflow-dummy-s460/PF-06/attempt-1/intent.md`; input hashes are in `intent-input-sha256.txt`. Planned source changes are Parse, Validate, Load/Reset, Export, and testdata under the declared PF-06 write paths.
**Tasks Completed**: Drafted `parse.go`, `validate.go`, `load.go`, and `export.go`; created canonical minimal fixture data and invalid fixture directories. Parsing, the current set of validation rules, FK-ordered fixture insertion, Reset cleanup, export, and canonical writing are present. Tests and final required gates are not complete.
**Verification already run**:
- `go test ./internal/fixtures/...` after source draft: exit 0, package compiled, no test files yet; `/tmp/pf06-compile.log`.
- `go run tmp/punchflow-kickflow-dummy-s460/PF-06/attempt-1/generate-fixtures.go`: exit 0; generated `testdata/minimal` by loading and exporting a representative set; `/tmp/pf06-fixture-generate.log`.
- An earlier compile attempt printed `undefined: strings` and failed, but its original log was overwritten by the later successful package compile; do not treat it as current passing evidence.
- Required `go build`, `go vet`, behavioral test suite, and final `gofmt -l` gates have not been run.
**Reconciliation requests**:
- PF-27 must resolve the frozen contract gap before PF-06 can prove byte-identical round-trip for all fixture values: `internal/fixtures/types.go` defines `MasterInputFixture.ID`, but PF-02's `internal/model/master.go` defines `GeneralMasterItem.Inputs` as `map[string]any`, and `internal/store/migrations/0001_init.sql` stores `general_master_items.inputs` as JSON `fieldID -> value`. The ID cannot be recovered by `Export`; the representative exported `general-masters.json` currently loses the original nested input ID. Relevant implementation locations: `internal/fixtures/load.go` (`pf06InsertTx`, mapping `FieldCode` to field ID/value), `internal/fixtures/export.go` (reconstruction), and `internal/fixtures/testdata/minimal/general-masters.json` (input ID emitted empty). Impact: assigned acceptance criterion “Export(Load(minimal)) byte-identical to testdata/minimal” is not satisfied for this value, and integrity/round-trip tests are not yet added. Resume only after PF-27 serially reconciles the storage contract or records an explicit accepted representation decision; then update PF-06 implementation/tests within authorized paths and rerun all four plan verification commands.
