# PF-02 Data Contract Implementation Plan

**Status**: Ready
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

### sharedPaths
- (none)

### sharedPathNotes
- (none)

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
- `general_master_items`(id, general_master_id FK, code, name, description NULL, starts_on NULL, ends_on NULL, inputs json; UNIQUE(general_master_id, code))
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
- `enums.go`: string constants for ticket status, step status, action types, step types, user status, tenant plan/mode, token kind, webhook event types (all 13), webhook event status, permissions (all 19).
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

## Verification

```bash
go mod verify 2>&1 | tee /tmp/pf02-modverify.log         # "all modules verified"
go list -m all 2>&1 | tee /tmp/pf02-modules.log          # must list the 5 modules
go build ./... 2>&1 | tee /tmp/pf02-build.log
go vet ./internal/store/... ./internal/model/... ./internal/fixtures/... 2>&1 | tee /tmp/pf02-vet.log
go test ./internal/store/... ./internal/model/... ./internal/fixtures/... 2>&1 | tee /tmp/pf02-test.log
gofmt -l internal/store internal/model internal/fixtures   # prints nothing
```

## Done Criteria

- [ ] 5 modules in go.mod with go.sum entries; `go mod verify` passes; `go-toolchain.json` records go version, each `go get` with exit code 0 and resolved version, and the verify result
- [ ] Schema contains every table above; migration idempotent
- [ ] Model and fixture types compile with documented JSON tags; every file < 1000 lines
- [ ] Tests pass; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
