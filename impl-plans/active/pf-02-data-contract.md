# PF-02 Data Contract Implementation Plan

**Status**: In Progress
**Wave**: 1 | **Depends On**: - | **Plan ID**: PF-02
**Design Reference**: design-docs/specs/architecture.md#storage, #fixtures, #dependencies; design-docs/specs/design-approval-engine.md#data-owned-by-the-engine; design-docs/specs/design-webhooks.md#data-model; design-docs/specs/design-auth-admin.md
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Pin the data shapes every later plan compiles against: Go module dependencies, domain model structs, the full SQLite schema (one migration), the store core (open, migrate, tx, querier, ids, secrets, audit insert), and the fixture/definition JSON structs. After this wave these files are frozen (README protocol rule 5), so they must be complete. The kickflow field names come from `design-docs/references/kickflow-api.md` section 4.

## Non-goals

- No repositories for specific resources (later plans own `internal/store/<aggregate>.go`).
- No fixture loading logic (PF-06), no presenters (PF-08).
- Do not edit `internal/kftime`, `internal/kferr` (PF-01, same wave; this plan must not import them).

## Files

| Path | Action |
|---|---|
| `go.mod`, `go.sum`, `go-toolchain.json` | `go get` dependencies and record the toolchain audit manifest (see below) |
| `internal/model/` files | create the 14 files listed in Path Declarations (workflow/route defs in `definition.go`) |
| `internal/store/db.go`, `tx.go`, `migrate.go`, `ids.go`, `secrets.go`, `audit.go`, `truncate.go`, `store_test.go` | create |
| `internal/store/migrations/0001_init.sql` | create |
| `internal/fixtures/types.go` | create (structs only, no logic) |

## Path Declarations

### writePaths
- `impl-plans/active/pf-02-data-contract.md`
- `go.mod`
- `go.sum`
- `go-toolchain.json`
- `internal/model/tenant.go`
- `internal/model/user.go`
- `internal/model/org.go`
- `internal/model/role.go`
- `internal/model/classification.go`
- `internal/model/master.go`
- `internal/model/definition.go`
- `internal/model/ticket.go`
- `internal/model/comment.go`
- `internal/model/file.go`
- `internal/model/audit.go`
- `internal/model/token.go`
- `internal/model/webhook.go`
- `internal/model/enums.go`
- `internal/store/db.go`
- `internal/store/tx.go`
- `internal/store/migrate.go`
- `internal/store/ids.go`
- `internal/store/secrets.go`
- `internal/store/audit.go`
- `internal/store/truncate.go`
- `internal/store/store_test.go`
- `internal/store/migrations/0001_init.sql`
- `internal/fixtures/types.go`
- `internal/model/enums_test.go`
- `internal/fixtures/enum_rules_test.go`

### sharedPaths
- `internal/fixtures/validate.go`
- `internal/present/present_test.go`

### sharedPathNotes
- `internal/fixtures/validate.go`: owned by PF-06; session 462 phase 1 only: replace enum literal checks with the `internal/model` Go lists and add the enum checks listed in Continuation task 5; nothing else
- `internal/present/present_test.go`: owned by PF-08; session 462 phase 1 only: change the two `RouteTarget` `GradeSymbol` literals to `pf08String("equal")` so the package compiles after the `*string` change

### artifactRoots
- (none; `go get` downloads into GOMODCACHE outside the repository, and `go.sum` is the in-repo digest record)

`go-toolchain.json` is the authored audit manifest: `{"goVersion": <output of go version>, "commands": [{"cmd": "go get <pkg>", "exitCode": 0, "resolved": "<module>@<version>"}...], "modVerify": {"cmd": "go mod verify", "exitCode": 0, "output": "all modules verified"}, "recordedAt": "<RFC3339>"}`. PF-27 appends its post-tidy record.

## Dependencies (exact procedure)

Run `go get` once per package path, at the newest tagged release, and record the resolved versions in the progress log:
`modernc.org/sqlite`, `github.com/coreos/go-oidc/v3/oidc`, `golang.org/x/oauth2`, `github.com/santhosh-tekuri/jsonschema/v6`, `gopkg.in/yaml.v3`. Then `go mod verify`. Write every command, its exit code and resolved version into `go-toolchain.json` (format above). Do NOT run `go mod tidy`: only sqlite is imported now, and tidy would drop the others that later plans import. Follow `.claude/skills/supply-chain-secure-install/SKILL.md` (no replace directives, no `GOFLAGS=-insecure`, keep `GONOSUMDB` unset).

## Contracts

```go
// internal/store
type Querier interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
type DB struct{ /* *sql.DB with MaxOpenConns(1) */ }
func Open(ctx context.Context, path string) (*DB, error) // mkdir parent, pragmas, migrate
func (d *DB) Close() error
func (d *DB) Q() Querier                                  // outside transactions only
func (d *DB) WithTx(ctx context.Context, fn func(tx *Tx) error) error
type Tx struct{ /* wraps *sql.Tx */ }                      // implements Querier
var ErrNotFound = errors.New("store: not found")
func NewID() string                                       // UUID v4 via crypto/rand, lowercase
func IsUUID(s string) bool
func Millis(t time.Time) int64; func Time(ms int64) time.Time
func NullMillis(t *time.Time) any; func TimePtr(ms sql.NullInt64) *time.Time
func ResolveID(ctx context.Context, q Querier, table, idOrCode string) (string, error) // whitelisted tables with a code column; ErrNotFound
func ServerSecret(ctx context.Context, q Querier, name string) ([]byte, error)           // create 32 random bytes on first use
func InsertAuditLog(ctx context.Context, q Querier, a model.AuditLog) error
func TruncateAll(ctx context.Context, q Querier) error // every table except schema_migrations and server_secrets, FK-safe order
func (d *DB) IsEmpty(ctx context.Context) (bool, error) // true when tenant table has no row
```

**Deadlock rule (reviewer finding)**: with one connection, calling `d.Q()` (or any `*DB` method) inside a `WithTx` callback blocks forever. Every repository, presenter, engine and webhook function therefore takes a `store.Querier` parameter and never captures a `*DB`. Document this rule in the `Q()` and `WithTx` doc comments, and test that `WithTx` rolls back when `fn` returns an error.

## Schema (`0001_init.sql`)

Conventions: ids `TEXT PRIMARY KEY` (UUID); instants `INTEGER` Unix ms; dates `TEXT` `YYYY-MM-DD`; booleans `INTEGER` 0/1; JSON `TEXT`; `FOREIGN KEY ... ON DELETE CASCADE` where the child cannot exist alone. Tables and columns (all have `created_at`, `updated_at` unless noted):

- `tenant`(id, public_id, name, plan `standard|enterprise`, mode `normal|admin_working|maintenance|suspended`, ip_allowlist json, allowed_email_domains json, self_service_proxy)
- `grades`(id, code UNIQUE, name, level UNIQUE, is_default)
- `users`(id, code UNIQUE, email UNIQUE COLLATE NOCASE, first_name, last_name, employee_id NULL, status, locale, user_type, custom_fields json, last_used_on NULL, deactivated_at NULL)
- `user_invitations`(id, user_id UNIQUE FK, user_email, user_locale, expires_at)
- `organization_charts`(id, name, current)
- `teams`(id, organization_chart_id FK, code, name, notes NULL, parent_id NULL FK teams, approve_only; UNIQUE(organization_chart_id, code))
- `team_memberships`(id, team_id FK, user_id FK, leader, grade_ids json; UNIQUE(team_id, user_id))
- `roles`(id, name, editable, permission_list json) ; `role_memberships`(role_id FK, user_id FK, created_at; PK both)
- `categories`(id, code UNIQUE, name); `folders`(id, code UNIQUE, name, description NULL, parent_id NULL FK, editable)
- `general_masters`(id, code UNIQUE, name, description NULL, default_sort_by, initial_display_code, initial_display_created_at, initial_display_description)
- `general_master_fields`(id, general_master_id FK, code, title, description NULL, field_type, required, field_order, visible, initial_display, options json NULL)
- `general_master_items`(id, general_master_id FK, code, name, description NULL, starts_on NULL, ends_on NULL, inputs json, input_ids json DEFAULT `{}` (field ID to input ID); UNIQUE(general_master_id, code))
- `routes`(id, code UNIQUE, status, folder_id FK, definition_json); `workflows`(id, code UNIQUE, status, folder_id FK, definition_json, ticket_number_counter)
- `proxy_applicants`, `proxy_approvers`(id, user_id FK, proxy_user_id FK, starts_on NULL, ends_on NULL, workflow_ids json; UNIQUE(user_id, proxy_user_id))
- `tickets`(id, workflow_id FK, status, current_step, title NULL, ticket_number NULL, author_id NULL, author_team_id NULL, author_team_snapshot json NULL, proxy_client_user_id NULL, forced_public_type, route_id NULL, opened_at, completed_at, denied_at, archived_at, closed_at all NULL)
- `ticket_inputs`(id, ticket_id FK, form_field_id, value json, ref_ids json; UNIQUE(ticket_id, form_field_id))
- `ticket_slip_items`(id, ticket_id FK, slip_section_id, position, inputs json)
- `ticket_steps`(id, ticket_id FK, route_step_id NULL, route_step_code NULL, step_order, display_order, title NULL, action_type, required_approvals_number, required_approvals_percent, status, skip, fallback, fallback_result NULL, completed_at NULL)
- `ticket_assignees`(id, ticket_step_id FK, ticket_id FK, user_id FK, completed, completed_at NULL, actual_approver_id NULL, assigned_at NULL)
- `ticket_custom_approvers`(ticket_id FK, route_step_code, user_ids json; PK both)
- `ticket_viewers`(id, ticket_id FK, user_id NULL, team_id NULL, grade_id NULL, descendants)
- `ticket_links`(ticket_id FK, linked_ticket_id FK, created_at; PK both; both directions stored)
- `comments`(id, ticket_id FK, user_id FK, body, mentioned_user_ids json, deleted_at NULL)
- `files`(id, signed_id UNIQUE, filename, content_type, byte_size, checksum, storage_name, uploaded_by NULL, created_at)
- `file_attachments`(file_id FK, ticket_id FK, comment_id NULL FK, form_field_id NULL, slip_item_id NULL, created_at)
- `audit_logs`(id, user_id NULL, action, data json NULL, remote_ip NULL, request_id NULL, system_type NULL, ticket_id NULL)
- `api_tokens`(id, kind `pat|sat`, name, description NULL, token_hash UNIQUE, user_id NULL, fixture_plaintext NULL, created_at, last_used_at NULL)
- `sessions`(id_hash PK, user_id FK, csrf_token, created_at, last_seen_at)
- `oidc_states`(state PK, nonce, code_verifier, return_to, expires_at)
- `webhooks`(id, name, url, secret NULL, enabled, workflow_ids json, event_types json, first_failure_at NULL, disabled_at NULL)
- `webhook_events`(id, webhook_id FK, event_type, ticket_id NULL, comment_id NULL, actor_user_id NULL, body BLOB, status, attempt_count, next_attempt_at, last_status_code NULL)
- `webhook_attempts`(id, event_id FK, attempt_no, started_at, duration_ms, request_headers json, status_code NULL, response_body BLOB NULL, error NULL, manual)
- `server_secrets`(name PK, value BLOB, created_at); `schema_migrations`(version PK, applied_at)

Add indexes on every FK column used in WHERE clauses (`ticket_assignees.user_id`, `ticket_steps.ticket_id`, `webhook_events(status, next_attempt_at)`, `audit_logs.created_at`, `tickets.workflow_id`, `tickets.author_id`).

## Model (`internal/model`)

- Row structs mirror the tables with Go names (`User{ID, Code, Email, FirstName, LastName, EmployeeID *string, Status, Locale, UserType string, CustomFields json.RawMessage, LastUsedOn *string, DeactivatedAt *time.Time, CreatedAt, UpdatedAt time.Time}` and so on for every table). Times are `time.Time` / `*time.Time`. JSON columns are typed slices/structs where the shape is fixed (e.g. `GradeIDs []string`, `WorkflowIDs []string`, `PermissionList []RolePermission`).
- `enums.go`: string constants for ticket status (`draft|in_progress|completed|rejected|denied|archived`), step status (`pending|in_progress|completed|skipped`), action types, the eight kickflow step types, user status (`activated|invited|suspended|deactivated`), tenant plan/mode, token kind, webhook event types (all 13), webhook event status, permissions (all 19). Legacy enum identifiers remain deprecated until PF-27 removes their fixture-validator references.
- `webhook.go`: `type WebhookEvent struct{ Type, TicketID, CommentID, ActorUserID string }` (the engine/handler -> webhook contract) plus `Webhook`, `WebhookDelivery` (row of `webhook_events`), `WebhookAttempt`.
- `definition.go`: `WorkflowDef` and `RouteDef`, the JSON stored in `definition_json` and used verbatim as fixture entries. JSON tags equal the kickflow `WorkflowDetail` / `RouteDetail` property names, with these rules: server-computed properties are omitted (createdAt, updatedAt, counts, author, versionAuthor, nextTicketNumberValue*, publishable, integration objects); references to other entities use codes (`folderCode`, `categoryCodes`, `teamCode`, `gradeCodes`, `userCodes`, `routeCode`, `formFieldCode`, `generalMasterCode`, `generalMasterItemCode`); every API-visible sub-entity keeps an explicit `id`. Required members: workflow settings listed in `kickflow-api.md` "Workflow" schema; `ticketNumberingTiming` (string); `availableTeams[]`; `sectionList[]` with `formFields[]` (FormField properties) or `slipFields[]` (SlipField properties) and section `conditionFields[]`; `routeConditions[]` (`id, conditionType, combinationType, combinationExpression, routeCode|null, errorMessage|null, conditionFields[]`). `RouteDef`: `id, code, status, name, description, notes, folderCode, steps[]` where each step has RouteStep properties with `userCodes`, `targets[]{teamCode, descendants, gradeSymbol, gradeCodes, variable}`, `routeStepCondition{id, conditionType, combinationType, routeStepConditionFields[]{id, variable, fieldKey, symbol, value, gradeCode, teamCode, generalMasterCode, generalMasterItemCode}}`.

## Fixture types (`internal/fixtures/types.go`)

One struct per fixture file (`TenantFile`, `[]GradeFixture`, `[]UserFixture`, `[]OrgChartFixture{ID, Name, Current, Teams[]{ID, Code, Name, Notes, ParentCode, ApproveOnly, Memberships[]{UserCode, Leader, GradeCodes}}}`, `[]RoleFixture{ID, Name, Editable, PermissionList[], MemberCodes}`, `[]CategoryFixture`, `[]FolderFixture{ID, Code, Name, Description, ParentCode, Editable}`, `[]GeneralMasterFixture{..., Fields[], Items[]{ID, Code, Name, Description, StartsOn, EndsOn, Inputs[]{ID, FieldCode, Value}}}`, `[]model.RouteDef`, `[]model.WorkflowDef`, `[]ProxyFixture{ID, UserCode, ProxyUserCode, StartsOn, EndsOn, WorkflowCodes}`, `[]TokenFixture{ID, Kind, Name, Description, UserCode, Token}`, `[]WebhookFixture{ID, Name, URL, Secret, Enabled, WorkflowCodes, EventTypes}`). JSON tags camelCase. Struct field order defines the canonical key order of exported fixtures (PF-06), so order fields as listed. Also define `const` file names (`FileTenant = "tenant.json"` ... `FileWebhooks = "webhooks.json"`).

## Tests (`internal/store/store_test.go`)

- Open on a temp path -> all tables exist; `schema_migrations` has version 1; reopening does not re-apply
- `PRAGMA foreign_keys` = 1, `journal_mode` = wal
- WithTx with error -> rows rolled back; success -> committed
- NewID -> 36 chars, `IsUUID` true, 1000 calls unique
- ServerSecret twice -> same 32 bytes
- TruncateAll after inserting a user -> user count 0, schema_migrations intact
- ResolveID by id and by code on `users` -> same id; unknown -> ErrNotFound; non-whitelisted table -> error
- a `json.Unmarshal` round-trip test of a small `WorkflowDef`/`RouteDef` literal (keeps the JSON tags honest)

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 1 (runs alone; every other plan compiles against `internal/model`). **Depends on**: accepted PF-01 only.
**Design**: `design-docs/specs/design-api-coverage.md#enum-contract-frozen-in-pf-02` (table and rules 1-5), `design-docs/specs/notes.md` CF-01/CF-02/CF-03, `design-docs/specs/architecture.md#fixtures` (input IDs), `#verification-command-rule-session-462`.

### Why

Carried finding CF-01 (high) is still open. The previous correction fixed only step types and `activated`. In the tree at 410d3d8:
- `internal/model/enums.go:49-62` defines invented events (`ticket_submitted`, `ticket_remanded`, `ticket_resubmitted`, `comment_deleted`) and lacks `ticket_updated`, `ticket_opened`, `ticket_confirmed` and `ticket_rejected`.
- `enums.go:66-84` defines 19 invented permissions instead of the kickflow 19.
- It keeps the non-kickflow constants `StepTypeGrade`, `StepTypeParallel`, `TicketStatusWithdrawn`, `TicketStatusPending`, `StepStatusApproved`, `StepStatusDenied` and `StepStatusRemanded`. `WebhookStatusDelivered = "delivered"` contradicts `design-webhooks.md` (`succeeded`).
- `internal/fixtures/validate.go:222` accepts only `user|team|grade|parallel`.
- `model.RouteTarget.GradeSymbol` is a non-nullable `string` (so routes.json must write `""`).
- `model.WorkflowSection` cannot express a conditional form section or a slip section code (fixtures fake it with `sectionType: "conditional"`).

PF-02 owns the frozen model. In this phase no other plan runs, so PF-02 also makes the few consumer edits that keep the tree compiling and the validator canonical. Fixture JSON data is fixed later (PF-07 default set, PF-06 testdata).

### Non-goals (this session)

- Do not edit fixture JSON (`fixtures/default/*`, `internal/fixtures/testdata/*`). Do not edit `load.go`/`export.go` (PF-06), presenters (PF-08), auth (PF-05) or the web app.
- Do not add a migration. `0001_init.sql` stays as is (D-06: the in-place `input_ids` edit is accepted pre-release).
- No `go mod tidy`. Do not edit `go.mod`/`go.sum`: `gopkg.in/yaml.v3` is already required (indirect), and importing it from a test compiles as is.

### File-level changes

Path additions for this session: `internal/model/enums_test.go` and `internal/fixtures/enum_rules_test.go` (writePaths, new). `internal/fixtures/validate.go` and `internal/present/present_test.go` (sharedPaths, see notes below).

1. `internal/model/enums.go`: rewrite the constants and add one exported `[]string` list per row of the design enum table, with exactly these names: `TicketStatuses`, `StepStatuses`, `ActionTypes`, `StepTypes`, `FallbackTypes`, `ManagerSelectionTypes`, `GradeSymbols`, `StepConditionTypes`, `StepCombinationTypes`, `StepConditionFieldKeys`, `RouteConditionTypes`, `RouteCombinationTypes`, `SectionTypes`, `RouteStatuses`, `WorkflowStatuses`, `UserStatuses`, `UserTypes`, `Permissions`, `MasterFieldTypes`, `MasterSortKeys`, `WebhookEventTypes`.
   - Add one constant per value. Naming: `<Enum><Value>` in Go camel case, e.g. `PermissionWorkflowMisc = "workflow_misc"`, `WebhookEventTicketOpened = "ticket_opened"`, `UserTypeNormal = "normal"`, `SectionTypeForm = "form"`, `RouteStatusVisible = "visible"`, `WorkflowStatusInvisible = "invisible"`, `GradeSymbolAnyOf = "any_of"`, `FallbackHigherTeam = "higher_team"`.
   - Keep `UserStatusActive = "activated"` (name unchanged, used by PF-05). Keep the existing `TicketStatus*`, `StepStatus*`, `ActionType*` and `StepType*` names for the canonical values.
   - Delete every invented or non-kickflow constant: the 4 invented events, the 19 invented permissions, `StepTypeGrade`, `StepTypeParallel`, `TicketStatusWithdrawn`, `TicketStatusPending`, `StepStatusApproved`, `StepStatusDenied`, `StepStatusRemanded`. `StepTypeGrade`/`StepTypeParallel` are referenced only by `validate.go:222`, which task 5 fixes in the same session.
   - Punchflow-only enums stay: tenant plan/mode, token kind, webhook delivery status. Rename `WebhookStatusDelivered` to `WebhookStatusSucceeded = "succeeded"`; nothing else references it.
   - Excluded values (`permanently_deleted`, `raised`, `other_variable`, `custom`) are NOT in the lists.
2. `internal/model/definition.go`:
   - `RouteTarget.GradeSymbol` becomes `*string` with tag `json:"gradeSymbol"` (no omitempty), so "no filter" is `null`. Leave the other `RouteTarget` tags unchanged.
   - `WorkflowSection` gets, in this order: `ID`, `Code string json:"code,omitempty"` (slip sections only), `Title`, `SectionType`, `Conditional bool json:"conditional,omitempty"`, `CombinationType string json:"combinationType,omitempty"`, then the existing `FormFields`, `SlipFields`, `ConditionFields`. Struct field order is the canonical fixture key order (PF-06/PF-07 depend on it).
3. `internal/model/enums_test.go` (new, `package model`): `TestEnumListsMatchOpenAPI`. It is table-driven, one row per Go list with an OpenAPI source: `{list, schema, propertyPath, excluded}`. Sources:

   | List | Schema and property | Excluded |
   |---|---|---|
   | `TicketStatuses` | `TicketSummary.status` | `permanently_deleted` |
   | `StepStatuses` | `TicketStep.status` | `raised` |
   | `ActionTypes` | `RouteStep.actionType` | - |
   | `StepTypes` | `RouteStep.stepType` | - |
   | `FallbackTypes` | `RouteStep.fallbackType` | - |
   | `ManagerSelectionTypes` | `RouteStep.managerSelectionType` | - |
   | `GradeSymbols` | `RouteStepTarget.gradeSymbol` | - |
   | `StepConditionTypes` | `RouteStepCondition.conditionType` | - |
   | `StepCombinationTypes` | `RouteStepCondition.combinationType` | - |
   | `StepConditionFieldKeys` | `RouteStepConditionField.fieldKey` | `other_variable` |
   | `RouteConditionTypes` | `WorkflowRouteCondition.conditionType` | - |
   | `RouteCombinationTypes` | `WorkflowRouteCondition.combinationType` | `custom` |
   | `SectionTypes` | `SectionListItem.sectionType` | - |
   | `RouteStatuses` | `Route.status` | - |
   | `WorkflowStatuses` | `Workflow.status` | - |
   | `UserStatuses` | `User.status` | - |
   | `UserTypes` | `User.userType` | - |
   | `Permissions` | `RoleDetail.permissionList.permission` | - |
   | `MasterFieldTypes` | `GeneralMasterField.fieldType` | - |
   | `MasterSortKeys` | `GeneralMaster.defaultSortBy` | - |

   - Load `../../design-docs/references/kickflow-openapi.yaml` (the test working directory is the package dir) with `gopkg.in/yaml.v3` into `map[string]any`.
   - Add a helper `pf02Enum(doc, schema string, path ...string) []string`. It starts at `components.schemas.<schema>`. For each path element it descends into `properties.<name>`, searching through `allOf`/`anyOf`/`oneOf` members and `items` when the name is not found directly (`RoleDetail` uses `allOf` + `items`). At the leaf it collects the string values of `enum`, including the `enum` of each `anyOf` member (fallbackType, gradeSymbol). It ignores `null`.
   - Assert set equality (sort both). Failure message: missing and extra values.
   - `TestWebhookEventTypesMatchReference`: `WebhookEventTypes` equals the 13 literals of the design table, in that order.
   - `TestEnumListsHaveNoDuplicates`.
4. `internal/store/store_test.go`: delete `TestModelEnumValuesMatchDesign` (superseded by task 3). Change the seed literals at lines ~188 and ~281 from `"active"`/`"member"` to `model.UserStatusActive`/`model.UserTypeNormal`.
5. `internal/fixtures/validate.go` (shared; PF-06 owns the file): replace every hard-coded enum literal list with the Go lists. Change only enum checks. Keep problem paths and messages the same; where a new check is added, use the message `invalid <field>`.
   - Already checked, switch to Go lists: user `status`, role `permission`, `stepType`, `actionType`, step condition `conditionType`/`combinationType`/`fieldKey` (keep the distinct `other_variable is unsupported` message for INF-18 wherever `other_variable` is checked), route condition `conditionType`, master `fieldType`, webhook `eventTypes`.
   - Add checks:
     - `userType` (`UserTypes`)
     - route `status` (`RouteStatuses`) and workflow `status` (`WorkflowStatuses`)
     - `fallbackType` (nil allowed, else `FallbackTypes`) and `managerSelectionType` (`ManagerSelectionTypes`)
     - route target and workflow `availableTeams` `gradeSymbol` (nil allowed, else `GradeSymbols`)
     - section `sectionType` (`SectionTypes`); a section with `conditional: true` must be `form` and have `combinationType` in `StepCombinationTypes`; `custom` -> `combinationType custom is unsupported` (INF-18)
     - route condition `combinationType` (empty allowed only for non-`field` types, else `RouteCombinationTypes`; `custom` keeps the INF-18 message)
   - Tighten master `defaultSortBy` to `MasterSortKeys` (today it also accepts `""` and `createdAt`).
   - Do not touch the ID walk, references, trees, tokens or anything else.
6. `internal/present/present_test.go` (shared; PF-08 owns the file): a compile fix only. The two `model.RouteTarget{... GradeSymbol: "equal" ...}` literals at line ~52 become `GradeSymbol: pf08String("equal")`. No other change.

### Invariants

- After this session `go build ./...` and `go vet` on every touched package succeed. The build may not be left broken for phase 2.
- Exported names that other plans already use stay unchanged: `UserStatusActive`, `UserStatusInvited`, `UserStatusSuspended`, `UserStatusDeactivated`, `TokenKind*`, `TenantPlan*`, `TenantMode*`, `ActionType*`, canonical `StepType*`/`StepStatus*`/`TicketStatus*`, `WebhookEventTypes`, `Permissions`.
- No package other than `internal/model` defines the 21 enums; the Go lists are the only source.

### Tests (input -> expected)

- `TestEnumListsMatchOpenAPI`: each of the 20 rows -> PASS. Temporarily adding `"grade"` to `StepTypes` makes it fail (check this manually once; do not commit the change).
- `TestWebhookEventTypesMatchReference` -> PASS with exactly 13 values.
- `internal/fixtures/enum_rules_test.go` (new, `package fixtures`; prefix helpers `pf02`): `TestEnumRule*`, table-driven. Each case does `Parse(os.DirFS("../../fixtures/default"))`, mutates one field, calls `Validate`, and checks for a `Problem` at a specific `File`+`Path`. It asserts presence or absence at that path only, never the full problem list, because the default data is fixed later by PF-07. Cases:
  - step type `grade` on `routes[0].steps[0]` -> problem at `[0].steps[0].stepType`; `manager` -> none at that path
  - permission `audit_read` on `roles[0].permissionList[0]` -> problem; `workflow_misc` -> none
  - `users[0].userType` `member` -> problem; `normal` -> none
  - `routes[0].status` `active` -> problem; `visible` -> none
  - `routes[0].steps[0].targets[0].gradeSymbol` pointer to `""` -> problem; nil -> none (pick a step that has targets)
  - a section with `sectionType` `conditional` -> problem
  - an appended webhook fixture with event `ticket_submitted` -> problem; `ticket_opened` -> none
  - master `defaultSortBy` `createdAt` -> problem
- Store tests still pass (11 tests at attempt 5 minus the deleted one).

### Verification (run in order; rules in `impl-plans/README.md` rule 11)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf02-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/model/... ./internal/store/... ./internal/fixtures/... ./internal/present/... 2>&1 | tee /tmp/pf02-s462-vet.log'; echo "exit=$?"   # vet compiles present_test.go
bash -o pipefail -c 'go test -count=1 -v ./internal/model/... ./internal/store/... 2>&1 | tee /tmp/pf02-s462-test.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'TestEnumRule' ./internal/fixtures/... 2>&1 | tee /tmp/pf02-s462-fixtures-test.log"; echo "exit=$?"
for f in /tmp/pf02-s462-test.log /tmp/pf02-s462-fixtures-test.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/model internal/store internal/fixtures/validate.go internal/fixtures/enum_rules_test.go internal/present/present_test.go   # prints nothing
rg -n -e 'ticket_submitted|ticket_remanded|ticket_resubmitted|comment_deleted|ticket_admin|audit_read|StepTypeGrade|StepTypeParallel|StepStatus(Approved|Denied|Remanded)|TicketStatus(Withdrawn|Pending)|WebhookStatusDelivered' -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' internal/model internal/fixtures/validate.go internal/store   # prints nothing, exit 1
go mod verify 2>&1 | tee /tmp/pf02-s462-modverify.log   # all modules verified
```

Evidence: every `exit=` is 0 (except the `rg` line, which must print nothing). Both test logs have FAIL=0, SKIP=0 and PASS>0. The model log shows `TestEnumListsMatchOpenAPI` with its 20 subtests passing.

### Done Criteria (session 462)

- [x] `enums.go` has the 21 Go lists and constants named above; no invented or non-kickflow constant remains (rg check empty)
- [x] `RouteTarget.GradeSymbol` is `*string` `json:"gradeSymbol"`; `WorkflowSection` has `Code`, `Conditional`, `CombinationType` in the stated order
- [x] `TestEnumListsMatchOpenAPI` (20 rows), `TestWebhookEventTypesMatchReference` and `TestEnumListsHaveNoDuplicates` pass uncached
- [x] `validate.go` uses the Go lists for every enum check listed in task 5; the `TestEnumRule*` cases pass
- [x] Store test seeds use canonical values; `TestModelEnumValuesMatchDesign` removed
- [x] build/vet exit 0; gofmt clean; every touched Go file < 1000 lines; progress log has exit status, log path and counts for each command

Earlier criteria (modules, schema, idempotent migration) were satisfied in session 460. They stay satisfied: do not touch them.

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-02 implementation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-1/baseline.json`; per-edit intents `edit-001` through `edit-023` in the same directory.
**Tasks Completed**: Added five required module dependencies, Go toolchain audit manifest, full idempotent SQLite schema, model row and definition types, fixture JSON types, store database/migration/transaction/id/secret/audit/truncate helpers, and focused store tests. Expanded workflow, form, slip, and route step definitions (including external publish settings) to preserve API property names, and completed timestamp fields on assignee/webhook rows after independent verification identified omissions. Preserved Go 1.25.4 by selecting latest compatible SQLite v1.59.0 and x/oauth2 v0.36.0; incompatible latest tags require Go 1.26 and are noted in `go-toolchain.json`.
**Verification**: `go mod verify` exit 0 (`/tmp/pf02-modverify.log`); `go list -m all` exit 0 (`/tmp/pf02-modules.log`); Final-source `go build ./...` exit 0 (`/tmp/pf02-final-build.log`); `go vet ./internal/store/... ./internal/model/... ./internal/fixtures/...` exit 0 (`/tmp/pf02-final-vet.log`); uncached focused tests exit 0 with 8 tests passing (`/tmp/pf02-final-test.log`); `gofmt -l internal/store internal/model internal/fixtures` clean (`/tmp/pf02-final-gofmt.log`). Module integrity/list checks also pass at `/tmp/pf02-modverify.log` and `/tmp/pf02-modules.log`.
**Prior attempts**: Unconstrained `go get modernc.org/sqlite` selected v1.60.1 and temporarily raised the go directive to 1.26.0; its output is `/tmp/pf02-get-sqlite.log`. An unconstrained x/oauth2 resolution selected v0.37.0 and required Go 1.26; output is `/tmp/pf02-get-oauth2.log`. Both were corrected to the latest versions compatible with Go 1.25.4. `go mod tidy` was not run per plan.
**Reconciliation requests**: None. Independent PF-02 verification found missing Workflow/FormField/SlipField/RouteStep API properties and three row timestamp fields; these were fixed and verified on final source. Formal implementation review and plan archival/index updates remain downstream workflow steps.

### Session: 2026-10-20 PF02-ADV-001 repair
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-2/baseline.json`; per-edit intents `attempt-2-edit-001` through `attempt-2-edit-003` in `tmp/punchflow-kickflow-dummy-s460/PF-02/`.
**Finding addressed**: PF02-ADV-001 (high): relative SQLite paths produced a URI authority error; canonical `:memory:` was also malformed.
**Tasks Completed**: `Open` now resolves file paths with `filepath.Abs`, creates the absolute parent directory, and constructs a slash-normalized file URL. Canonical `:memory:` uses `file::memory:` with the existing pragmas; legacy memory aliases were removed. Added relative path, spaced path, migration/pragmas/reopen, and in-memory regression coverage. Exported signatures, pragmas and pool settings are unchanged.
**Verification**: `go build ./...` exit 0 (`/tmp/pf02-attempt2-build.log`); `go vet ./internal/store/... ./internal/model/... ./internal/fixtures/...` exit 0 (`/tmp/pf02-attempt2-vet.log`); uncached focused tests exit 0 with 10 tests passing, including `TestOpenRelativePath` and `TestOpenMemoryPath` (`/tmp/pf02-attempt2-test.log`); `gofmt -l internal/store internal/model internal/fixtures` printed nothing, exit 0 (`/tmp/pf02-attempt2-gofmt.log`); `go mod verify` reported all modules verified, exit 0 (`/tmp/pf02-attempt2-modverify.log`).
**Reproduction**: Before the Open fix, `go test -count=1 -run 'TestOpen(RelativePath|MemoryPath)' ./internal/store` failed as expected with `invalid uri authority: rel` and an invalid memory authority (`/tmp/pf02-relative-path-repro.log`, exit 1). The same regressions pass on final source.
**Evidence**: Final source hashes and verification log identities are recorded under `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-2/final-source.json`.
**Reconciliation requests**: None. Adversarial re-review of PF02-ADV-001 remains downstream.

### Session: 2026-10-20 PF-02 Step 6 final verification
**Intent snapshots**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-3/edit-intent-001.md` and `edit-intent-002.md`; verification source hashes: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-3/final-source-files.json` (27 tracked implementation source paths; progress-plan hash recorded separately in this log).
**Tasks Completed**: Re-read the accepted PF-02 data contract and current review state. The previous PF02-ADV-001 correction and relative-path, spaced-path, migration/reopen, pragma, and `:memory:` tests are present. No new source change was needed. Independent read-only audit `/root/pf02_readonly_review` found no current high or medium implementation findings or unmet assigned criteria.
**Verification**: `go mod verify` exit 0, all modules verified (`/tmp/pf02-step6-modverify.log`); `go list -m all` exit 0 and lists the five required direct module families alongside their transitive dependencies (`/tmp/pf02-step6-modules.log`); `go build ./...` exit 0 (`/tmp/pf02-step6-build.log`); `go vet ./internal/store/... ./internal/model/... ./internal/fixtures/...` exit 0 (`/tmp/pf02-step6-vet.log`); uncached `go test -count=1 -v ./internal/store/... ./internal/model/... ./internal/fixtures/...` exit 0, 10 tests passed and 0 failed (`/tmp/pf02-step6-test.log`); `gofmt -l internal/store internal/model internal/fixtures` exit 0 with no formatted-file output (`/tmp/pf02-step6-gofmt.log`); `git diff --check` exit 0. All Go files in the assigned packages are below 1000 lines.
**Review boundary**: Formal independent adversarial re-review of PF02-ADV-001 and later workflow integration/finalization remain downstream. Existing low-severity notes are recorded in the prior repair entry; no high or medium issue remains on this source.
**Reconciliation requests**: None.

### Session: 2026-10-20 PF-02 review repair attempt 4
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-4/edit-intent.md`; progress edit intent: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-4/edit-intent-progress.md`. Pre-edit hashes: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-4/pre-edit-sha256.txt` and `pre-progress-edit-sha256.txt`.
**Findings addressed**: PF02-ENUM-DESIGN-DRIFT (high) and PF02-MASTER-INPUT-ID (high). `internal/model/enums.go` now has the accepted eight step types, `activated`, canonical ticket/step status values, and deprecated legacy identifiers retained for PF-06 compatibility. `internal/model/master.go` adds JSON-tagged `InputIDs`; `0001_init.sql` adds `input_ids TEXT NOT NULL DEFAULT '{}'`. `store_test.go` checks canonical enum sets, input_ids default, all 38 tables, and migration idempotence. PF-06 handoff: fixture load persists each input ID in `InputIDs` keyed by field ID; export restores it to `MasterInputFixture.ID`.
**Schema note**: Existing local `.punchflow-data` databases predate this pre-release column and must be recreated or reset; the initial migration is intentionally unchanged/idempotent for fresh development databases.
**Verification**: `go mod verify` exit 0 (`/tmp/pf02-a4-modverify.log`); `go list -m all` exit 0, includes all five required module families (`/tmp/pf02-a4-modules.log`); `go build ./...` exit 0 (`/tmp/pf02-a4-build.log`); `go vet ./...` exit 0 (`/tmp/pf02-a4-vet.log`); focused uncached `go test -count=1 -v ./internal/model/... ./internal/store/... ./internal/fixtures/...` exit 0, 11 store tests passed (`/tmp/pf02-a4-test.log`); migration regression `go test -count=1 -v ./internal/store/...` exit 0, 11 store tests passed (`/tmp/pf02-a4-store-test.log`); `gofmt -l internal/model internal/store internal/fixtures` exit 0 with no output (`/tmp/pf02-a4-gofmt.log`); enum grep exit 0 (`/tmp/pf02-a4-enumgrep.log`). Required full-suite `go test -count=1 ./...` exited 1 (`/tmp/pf02-a4-all-test.log`): `TestSessionActivationRequiresValidInvitation` and `TestHandlersLocalLoginAndOIDCDisabled` fail because auth still expects the old `active` value. Read-only `rg -n 'active' internal/store internal/auth --glob '*.go'` also found `internal/store/auth.go:161` writing literal `active`, plus PF-05 tests seeding/asserting it. This is an additional production mismatch: subsequent login for an invited user will see stored `active` and reject it after PF-02 makes `activated` canonical. All logs are copied to the attempt-4 evidence directory.
**Reconciliation request / blocker**: PF-05 owner must replace the literal activation write in `internal/store/auth.go` and migrate auth test fixture/status expectations to `model.UserStatusActive` / `activated`, then rerun auth and full-suite tests. Those files are forbidden by this repair brief. Implementation remains In Progress until `go test -count=1 ./...` passes. Source contract intentionally uses `activated` per accepted design/OpenAPI.
**Source hashes**: See `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-4/final-source-sha256.txt`; full test outputs are copied into that attempt directory.

### Session: 2026-10-20 PF-02 integration review redispatch attempt 5
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-5/intent.md`; plan edit intent: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-5/edit-intent-progress.md`; pre-verification hashes: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-5/pre-verification-sha256.txt`.
**Findings addressed**: Re-verified PF02-ENUM-DESIGN-DRIFT and PF02-MASTER-INPUT-ID from fresh source. The accepted enum values and deprecated compatibility constants are present in `internal/model/enums.go`; `GeneralMasterItem.InputIDs` and the `general_master_items.input_ids` default are present, with migration/default and idempotence checks in `internal/store/store_test.go`. PF-06 handoff remains: load stores input IDs keyed by field ID and export restores `MasterInputFixture.ID`.
**Verification**: `go mod verify` exit 0, all modules verified (`/tmp/pf02-a5-modverify.log`); `go list -m all` exit 0 and includes all five required direct module families (`/tmp/pf02-a5-modules.log`); `go build ./...` exit 0 (`/tmp/pf02-a5-build.log`); `go vet ./...` exit 0 (`/tmp/pf02-a5-vet.log`); focused `go test -count=1 -v ./internal/model/... ./internal/store/... ./internal/fixtures/...` exit 0, 11 store tests passed (`/tmp/pf02-a5-model-store-test.log`); migration regression `go test -count=1 -v ./internal/store/...` exit 0, 11 store tests passed (`/tmp/pf02-a5-store-test.log`); full `go test -count=1 ./...` exit 0 across all packages (`/tmp/pf02-a5-all-test.log`); `gofmt -l internal/model internal/store internal/fixtures` exit 0 with no output (`/tmp/pf02-a5-gofmt.log`); enum grep exit 0 (`/tmp/pf02-a5-enumgrep.log`).
**Current source hashes**: `tmp/punchflow-kickflow-dummy-s460/PF-02/attempt-5/final-source-sha256.txt`; verification outputs and exit codes are copied into the same attempt directory.
**Reconciliation requests**: None. Formal independent re-review and workflow finalization remain downstream.


### Session: 2026-10-20 PF-02 session 462 implementation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s462/PF-02/attempt-1/pre-edit-intent.md`; per-edit intent records `edit-intent-001.md` through `edit-intent-010.md`; progress intent `edit-intent-progress.md` in the same attempt directory.
**Tasks completed**: Replaced invented enum constants with the 21 canonical model lists and constants; added OpenAPI enum conformance, webhook reference and duplicate tests; made RouteTarget gradeSymbol nullable without omitting the JSON property; added conditional/slip section model fields; switched PF-02 fixture enum checks to model lists and added the requested route/workflow/user/fallback/grade/section/combination checks; updated store seeds and the two PF-08 shared test literals. Fixture JSON remains assigned to PF-06/PF-07.
**Verification**:
- `bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf02-s462-build.log'; echo "exit=$?"` exit 0, log copied to `tmp/punchflow-kickflow-dummy-s462/PF-02/attempt-1/logs/pf02-s462-build.log`.
- `bash -o pipefail -c 'go vet ./internal/model/... ./internal/store/... ./internal/fixtures/... ./internal/present/... 2>&1 | tee /tmp/pf02-s462-vet.log'; echo "exit=$?"` exit 0, log copied to `tmp/punchflow-kickflow-dummy-s462/PF-02/attempt-1/logs/pf02-s462-vet.log`.
- `bash -o pipefail -c 'go test -count=1 -v ./internal/model/... ./internal/store/... 2>&1 | tee /tmp/pf02-s462-test-rerun.log'; echo "exit=$?"` exit 0; PASS=54 FAIL=0 SKIP=0; log copied to `tmp/punchflow-kickflow-dummy-s462/PF-02/attempt-1/logs/pf02-s462-test-rerun.log`.
- `bash -o pipefail -c "go test -count=1 -v -run 'TestEnumRule' ./internal/fixtures/... 2>&1 | tee /tmp/pf02-s462-fixtures-test-rerun.log"; echo "exit=$?"` exit 0; PASS=9 FAIL=0 SKIP=0; log copied to `tmp/punchflow-kickflow-dummy-s462/PF-02/attempt-1/logs/pf02-s462-fixtures-test-rerun.log`.
- `bash -o pipefail -c 'go mod verify 2>&1 | tee /tmp/pf02-s462-modverify.log'; echo "exit=$?"` exit 0 (`all modules verified`), log copied under attempt logs.
- `gofmt -l internal/model internal/store internal/fixtures/validate.go internal/fixtures/enum_rules_test.go internal/present/present_test.go` exit 0, no output; log `/tmp/pf02-s462-gofmt.log` copied under attempt logs.
- Forbidden enum `rg -n` check over `internal/model internal/fixtures/validate.go internal/store` exit 1 with no matches (expected); log `/tmp/pf02-s462-enumgrep.log` copied under attempt logs.
- `git diff --check` exit 0. Every touched Go file is under 1000 lines (max 517).
**Prior verification attempt**: Initial uncached model/store command exited 1 (PASS=49 FAIL=5 SKIP=0; parent test plus four excluded-value subtests) because the first conformance assertion included OpenAPI values excluded by the accepted design (`permanently_deleted`, `raised`, `other_variable`, `custom`); complete log `/tmp/pf02-s462-test.log`, copied under attempt logs. The test now subtracts those four documented exclusions and the final-source rerun passes all 20 rows. Initial fixture-rule run exited 0 (PASS=9 FAIL=0 SKIP=0); log `/tmp/pf02-s462-fixtures-test.log`, copied under attempt logs.
**Final source hashes**: `internal/model/enums.go` 676bfe21; `definition.go` 22030847; `enums_test.go` 28b80a61; `internal/fixtures/validate.go` 6c8acee8; `enum_rules_test.go` 3f93fc6d; `internal/present/present_test.go` ca1d7a36; `internal/store/store_test.go` f9208da8.
**Review boundary**: PF-02 implementation criteria are complete on this source. Independent review, review-dependent documentation/index updates, commit, and push remain downstream workflow work. No reconciliation requests.
