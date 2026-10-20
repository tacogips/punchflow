# PF-08 Present Core Implementation Plan

**Status**: In Progress
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-08
**Design Reference**: design-docs/specs/architecture.md#package-layout (`internal/present`); design-docs/references/kickflow-api.md section 4; design-docs/specs/notes.md DIV-03, DIV-05, DIV-08
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

`internal/present` builds kickflow response JSON. A presenter must emit every `required` property of its OpenAPI schema with the exact property name, using `null` only where the schema allows it, and `[]` (never `null`) for required arrays. This plan covers every non-ticket schema plus the store reads they need. Ticket, comment, viewer and webhook-tenant shapes are PF-15. The contract test (PF-26) validates the output against `design-docs/references/kickflow-openapi.yaml`.

## Non-goals

- No list queries with filters/pagination (handler plans own those); only get-by-id and batch-by-ids loaders.
- No ticket shapes (PF-15). Do not edit `internal/model` (frozen).

## Files

`internal/present/present.go` (shared helpers), `user.go`, `org.go` (grade, team, member user, org chart), `role.go`, `classification.go` (category, folder), `master.go`, `workflow.go`, `workflow_form.go` (sections, form/slip fields), `route.go`, `proxy.go`, `audit.go`, and `present_test.go`, `workflow_test.go`; store reads `internal/store/read_users.go`, `read_org.go`, `read_roles.go`, `read_classification.go`, `read_masters.go`, `read_definitions.go`, `read_proxies.go`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-08-present-core.md`
- `internal/present/present.go`
- `internal/present/user.go`
- `internal/present/org.go`
- `internal/present/role.go`
- `internal/present/classification.go`
- `internal/present/master.go`
- `internal/present/workflow.go`
- `internal/present/workflow_form.go`
- `internal/present/route.go`
- `internal/present/proxy.go`
- `internal/present/audit.go`
- `internal/present/present_test.go`
- `internal/present/workflow_test.go`
- `internal/store/read_users.go`
- `internal/store/read_org.go`
- `internal/store/read_roles.go`
- `internal/store/read_classification.go`
- `internal/store/read_masters.go`
- `internal/store/read_definitions.go`
- `internal/store/read_proxies.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

JSON struct types are named `<Schema>JSON`. Fields follow the schema property order, JSON tags equal property names, no `omitempty` on required properties, `*T` for nullable values, `json.RawMessage` for free-form `data`/`value`. Timestamps are `string` produced with `kftime.Format`; dates are `*string`.

```go
// loaders take a Querier (never *store.DB) so they work inside transactions
func User(ctx context.Context, q store.Querier, id string, o UserOpts) (UserJSON, error)
func Users(ctx context.Context, q store.Querier, ids []string, o UserOpts) ([]UserJSON, error) // input order kept; missing ids skipped
func UserFromModel(u model.User, o UserOpts) UserJSON
type UserOpts struct{ LastUsedOn bool; CustomFields bool; UserType bool } // UserType false for MemberUser lists
func UserDetail(ctx context.Context, q store.Querier, id string, o UserOpts) (UserDetailJSON, error)
func Grade(m model.Grade) GradeJSON
func Grades(ctx context.Context, q store.Querier, ids []string) ([]GradeJSON, error)
func Team(ctx context.Context, q store.Querier, id string) (TeamJSON, error)
func TeamDetail(ctx context.Context, q store.Querier, id string) (TeamDetailJSON, error) // users capped at 100
func MemberUsers(ctx context.Context, q store.Querier, teamID string, offset, limit int) ([]MemberUserJSON, error)
func OrganizationChart(ctx context.Context, q store.Querier, id string) (OrganizationChartJSON, error) // also used as OrganizationChartDetail
func Role(ctx context.Context, q store.Querier, id string) (RoleJSON, error)
func RoleDetail(ctx context.Context, q store.Querier, id string) (RoleDetailJSON, error)
func Category(m model.Category) CategoryJSON
func Folder(ctx context.Context, q store.Querier, id string) (FolderJSON, error)
func FolderDetail(ctx context.Context, q store.Querier, id string) (FolderDetailJSON, error)
func GeneralMaster(ctx context.Context, q store.Querier, id string) (GeneralMasterJSON, error)
func GeneralMasterItem(ctx context.Context, q store.Querier, id string) (GeneralMasterItemJSON, error)
func GeneralMasterItems(ctx context.Context, q store.Querier, ids []string) ([]GeneralMasterItemJSON, error)
func Workflow(ctx context.Context, q store.Querier, id string) (WorkflowJSON, error)
func WorkflowInTicket(ctx context.Context, q store.Querier, id string) (WorkflowInTicketJSON, error)
func WorkflowDetail(ctx context.Context, q store.Querier, id string) (WorkflowDetailJSON, error)
func Route(ctx context.Context, q store.Querier, id string) (RouteJSON, error)
func RouteDetail(ctx context.Context, q store.Querier, id string) (RouteDetailJSON, error)
func ProxyApplicant(ctx context.Context, q store.Querier, id string) (ProxyApplicantJSON, error)
func ProxyApprover(ctx context.Context, q store.Querier, id string) (ProxyApproverJSON, error)
func AuditLog(ctx context.Context, q store.Querier, m model.AuditLog) (AuditLogJSON, error)
```

Not-found loaders return an error wrapping `store.ErrNotFound`. Store read functions (`read_*.go`) are named `Get<Entity>(ctx, q, id)` and `Get<Entities>(ctx, q, ids)` returning model rows, plus helpers: `TeamFullName`, `FolderFullName`, `ActiveOrgChartID`, `CountTeamUsers`, `CountFolderWorkflows`, `CountFolderRoutes`, `CountMasterItems`, `CountRoleUsers`, `CountOrgChartTeams`, `CountOrgChartMemberships`, `WorkflowDefByID`, `RouteDefByID`, `WorkflowIDByCode`, `RouteIDByCode`, `UserIDByCode`, `TeamIDByCode(chartID, code)`, `GradeIDByCode`, `MasterItemIDByCode`.

## Mapping Rules (from definitions to kickflow shapes)

- `Team.fullName` = names from the root joined with `/` (inferred format; log it). `Folder.fullName` the same over folders. `Folder.pipelinesCount` = 0.
- `OrganizationChart.activationPlan` = null. `Role.editable` from the row; `RoleDetail.permissionList[].folders/generalMasters/teams` resolved from stored ids (DIV-08).
- `Workflow`: `versionId` = workflow id (single version, DIV-05), `versionNumber` 1, `current` true, `author`/`versionAuthor` null, `reportFormats` [], `externalPublish` null, `nextTicketNumberValue` = counter+1 or null when the format is null, `nextTicketNumberValueByKey` null, `notes`/`titleFormula` from the def. `WorkflowInTicket`: `cloudSignSetting`, `ticketNumberKey`, `excelTemplate` null; `customNumberings`/`customNumberingSettings` []. `WorkflowDetail`: `slackChannel`, `chatworkRoom`, `microsoftTeamsChannel`, `googleChatSetting`, `reportUploadGoogleDrive`, `reportUploadBox` null (DIV-03); `ticketNumberingTiming` object per the `TicketNumberingTiming` schema; `availableTeams`, `watchers` ([]), `workflowFilterVisibilities` ([]), `routeConditions` with `route` = `Route` JSON or null.
- Form fields: every FormField / FormFieldDetail property; code references (master, grade, team) resolved to full objects; unknown optional detail objects (approverEditSetting, ticketCopySetting, customValidations, externalApiSetting, kintone...) null or [] per schema nullability (DIV-03/DIV-06). Section `viewRestriction` false, `sectionViewers` [].
- Route steps: `users` from `userCodes`; `targets[].team`/`grades`; `routeStepCondition` with resolved `grade`/`team`/`generalMaster` (RouteStepConditionGeneralMaster shape)/`generalMasterItem`; `subStatus` null.
- User: `image` object with three null URLs; `customFields` only when `o.CustomFields` and non-empty; `lastUsedOn` only when `o.LastUsedOn`; `userType` only when `o.UserType`. `UserDetail.userLineWorksAccount` null; `invitation` from `user_invitations` or null.
- GeneralMasterItem `inputs[].field` resolved by field id.

## Key Points and Pitfalls

- Read the property lists in `kickflow-api.md` section 4 for each schema; a missing required key is a contract-test failure later. Write a table-driven test per presenter that marshals the result and asserts the exact key set.
- Keep `workflow.go` + `workflow_form.go` under 1000 lines each (split further if needed).
- `Users` with an empty id list returns `[]UserJSON{}` (not nil).

## Tests (input -> expected)

Seed rows with SQL in a test helper (default fixtures are not loadable in this wave): a tenant, 3 users, 2 grades, a chart with parent/child teams and memberships, a role, root and child folders, a category, a master with a field and an item, a route def with 2 steps and a condition, a workflow def with a form and a slip section and 2 route conditions.
- `UserFromModel` -> keys exactly {id,email,code,firstName,lastName,fullName,employeeId,image,status,locale,createdAt,updatedAt} plus opts-driven keys; `fullName` = `lastName + " " + firstName` (inferred; log it)
- `TeamDetail(child)` -> `parent` set, `fullName` "Company/Sales", `users[].grades` non-empty, `leader` bool
- `FolderDetail(child)` -> `ancestors` [root], `editable` true; root -> `editable` false
- `WorkflowDetail` -> required integration keys present and null; `sectionList[0].formFields[*].code` match the def; `routeConditions[0].route.code` resolved; `errorMessage` branch has `route: null`
- `RouteDetail` -> steps ordered by stepOrder then position; condition field `grade` object resolved
- every timestamp matches `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+09:00$`
- missing id -> `errors.Is(err, store.ErrNotFound)`

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 2. **Depends on**: PF-02 (phase 1: `RouteTarget.GradeSymbol *string`, `WorkflowSection.Code/Conditional/CombinationType`, enum lists; PF-02 also changed two `GradeSymbol` literals in `present_test.go` to `pf08String("equal")`, so re-hash that file before editing).
**Design**: `architecture.md#fixtures` (CF-02 input ID data flow: store read + presenter hop); `design-api-coverage.md` enum contract (Section type row); `notes.md` CF-02.

### State at 410d3d8 (audited)

- `store.GetGeneralMasterItem` (`internal/store/read_masters.go:55`) does not select `input_ids`.
- `present.GeneralMasterItem` (`internal/present/master.go:47`) emits the field ID as `inputs[].id`, which is wrong (CF-02).
- `present/workflow_form.go:24` emits `sectionType` straight from the def and has no `code`, `conditional` or `combinationType` keys.
- The session-460 tests never ran uncached with counts.

### File-level changes

1. `internal/store/read_masters.go`: `GetGeneralMasterItem` and `GetGeneralMasterItems` select `input_ids` and decode it into `model.GeneralMasterItem.InputIDs`, mirroring the existing `inputs` decode (`{}` -> empty non-nil map).
2. `internal/present/master.go` `GeneralMasterItem`:
   - `inputs[].id = m.InputIDs[fieldID]`.
   - If a field ID in `m.Inputs` has no entry in `InputIDs`, return an error (`present: general master item <id>: field <fieldId> has no input id`). Never fall back to the field ID.
   - Keep the `field` object and the `fieldOrder` sorting.
3. `internal/present/workflow_form.go` section shape (`SectionListItem`):
   - Slip sections add `code` (from `WorkflowSection.Code`).
   - Form sections add `conditional` (bool) and, when `conditional` is true, `combinationType`.
   - `sectionType` is the def value (always `form`/`slip` after PF-02/PF-07).
   - Keep the existing keys.
4. `internal/present/route.go` needs no code change: `t.GradeSymbol` is now `*string` and marshals as `null`. A test asserts that.
5. `internal/present/present_test.go`:
   - Update the master item seed at line ~47 to also insert `input_ids` (e.g. `{"fld":"in1"}`).
   - Add the test cases below. Keep PF-02's `pf08String("equal")` literals.

### Tests (input -> expected)

- `TestPresentMasterItemInputIDs`: an item seeded with `input_ids {"fld":"in1"}` -> `inputs[0].id == "in1"` and `inputs[0].field.id == "fld"`. An item seeded with an `inputs` value but `input_ids` `{}` -> error.
- `TestPresentSectionShapes`: a def with a slip section (`Code: "lines"`) and a conditional form section (`Conditional: true, CombinationType: "all"`) -> slip JSON has `code: "lines"`; form JSON has `conditional: true, combinationType: "all"`; a non-conditional form section has `conditional: false` and no `combinationType` key.
- `TestPresentRouteTargetGradeSymbolNull`: target with nil `GradeSymbol` -> JSON `"gradeSymbol": null`.
- Existing `TestPresentCoreKeySets` and `TestWorkflowPresenterContracts` still pass. Update their expected key sets only for the added section keys.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf08-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/present/... ./internal/store/... 2>&1 | tee /tmp/pf08-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v ./internal/present/... 2>&1 | tee /tmp/pf08-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf08-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf08-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf08-s462-test.log)"
gofmt -l internal/present internal/store/read_users.go internal/store/read_org.go internal/store/read_roles.go internal/store/read_classification.go internal/store/read_masters.go internal/store/read_definitions.go internal/store/read_proxies.go   # prints nothing
wc -l internal/present/*.go | sort -n | tail -2   # every file < 1000
```

Evidence: exit 0; FAIL=0; SKIP=0; the 3 new tests appear as `--- PASS`. (`internal/present` has only PF-08 tests in phase 2; PF-15 adds ticket tests in phase 3.)

### Done Criteria (session 462)

- [ ] CF-02 read/present half closed: `inputs[].id` is the persisted input ID; a missing ID is an error; tests prove both
- [ ] Section JSON carries `code` (slip) and `conditional`/`combinationType` (form); `gradeSymbol` null renders as `null`
- [ ] Uncached present tests pass with counts recorded; build/vet exit 0; gofmt clean; files < 1000 lines

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-08 implementation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-08/attempt-1/intent-001.md` through `intent-022.md`; final plan update intent: `tmp/punchflow-kickflow-dummy-s460/PF-08/attempt-1/intent-progress-final.md`.
**Tasks Completed**: Added schema-shaped non-ticket presenters and `store.Querier` read helpers for users, organization, roles, categories/folders, general masters, definitions/routes, proxies and audit logs. Added exact JSON key-set tests across every presenter, plus seeded SQLite behavior checks for relationships, null/empty values, ordering, timestamp format and not-found handling. No files outside PF-08 writePaths were changed.
**Verification**: `go build ./...` exit 0 (`/tmp/pf08-build.log`); `go vet ./internal/present/... ./internal/store/...` exit 0 (`/tmp/pf08-vet.log`); `go test ./internal/present/...` exit 0 (`/tmp/pf08-test.log`); `go test ./internal/present/... ./internal/store/...` exit 0 (`/tmp/pf08-store-present-test.log`); `gofmt -l internal/present internal/store` exit 0, no output (`/tmp/pf08-gofmt.log`). All PF-08 Go files are below 1000 lines.
**Reconciliation requests**: None. Formal independent review and combined-tree integration review remain downstream workflow steps.
