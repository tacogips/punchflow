# kickflow API and Webhook Coverage Matrix

Single source of truth for which kickflow operation and webhook event punchflow implements, where, under which permission, and which inferred behaviors apply. Implementation plans and tests must reference rows of this document.

## Overview

- Scope: all 96 HTTP operations on 57 paths of `design-docs/references/kickflow-openapi.yaml` and all 13 webhook events of `design-docs/references/kickflow-api.md` section 5.5.
- The spec contains 99 operationIds: the 96 path operations below plus the 3 `webhooks` entries (`webhookPing`, `webhookTicket`, `webhookComment`), which are payload schemas for outgoing webhooks, not routes. The contract test counts only the 96 path operations.
- `INF-xx` and `DIV-xx` IDs refer to `design-docs/specs/notes.md`. Behavior with no ID is Confirmed in the reference.
- Every operation is implemented (no "not implemented" rows). Where kickflow behavior is out of scope (DIV items), the endpoint still exists and returns schema-valid data.

## Technical Details

### Cross-cutting /v1 conventions

| Convention | punchflow behavior | Owner package |
|---|---|---|
| Base path | `/v1/...` exactly as in the spec; routes registered with Go 1.22+ `net/http.ServeMux` method patterns | `internal/api/v1` |
| Unknown path or wrong method | A catch-all `/v1/` handler returns `404 endpoint_not_found` (a more specific pattern with another method does not match, so ServeMux never emits 405) | `internal/api/v1` |
| Success status | Always `200`. Operations with "empty body" in the spec write status 200 with zero bytes and no `Content-Type` | `internal/httpx` |
| Request content type | Requests with a non-empty body must use `application/json` (parameters such as `; charset=utf-8` allowed), except `uploadFile` (`multipart/form-data`). Otherwise `400 invalid_content_type` | `internal/httpx` |
| 411 rule | HTTP/1.x `POST`/`PUT` without `Content-Length` and without chunked `Transfer-Encoding` returns `411` with an empty body | `internal/httpx` |
| JSON decoding | Malformed JSON `400 invalid_json`; wrong types `400 invalid_parameter` (INF-38); unknown fields ignored (INF-38); size limits INF-39 | `internal/httpx` |
| JSON encoding | INF-33 (HTML-escaped, no trailing newline) | `internal/httpx` |
| Error body | `{"code","message"}` plus `errors` (object of field -> string[]; `base` for non-field errors) only for 422 validation errors; `doc_url` is never emitted | `internal/kferr`, `internal/httpx` |
| Timestamps | JST with milliseconds `2006-01-02T15:04:05.000+09:00` using a fixed `+09:00` zone (no tzdata dependency). Date-only fields `YYYY-MM-DD`. Inputs accept the four documented formats | `internal/kftime` |
| IDs | UUID v4 from `crypto/rand`. Path parameters for users, grades, teams, folders, routes, workflows, general masters and items accept the UUID or the `code` | `internal/store` |
| Pagination | `page`/`perPage` (INF-03), headers `Link` (INF-01), `Page`, `Per-Page`, `Total`; body is a plain JSON array, `[]` when empty | `internal/httpx` |

Ticket list query behavior follows the public query meanings: `authorTeamFullName` matches the slash-joined team hierarchy; `stepTitle` and `assigneeStatus=current` are limited to the ticket's current actionable step group. Task lists default to outstanding assignments, while an explicit `status[]` selection retains historical assignments for those statuses. These query paths are regression-tested with populated tickets (API-01..API-03).

Keyword search includes issued custom numbering values (`formatted_number`) at SQL candidate selection and restricted-form visibility recheck (INF-NUM-03). Ticket CSV export uses the live author team name, falling back to the historical `authorTeamSnapshot.fullName` if the live team was deleted (INF-NUM-04).

Ticket action controls in the web UI check all steps in the current parallel group and include active proxy assignments, matching the backend's one-call handling for a user's assignments across that group (INF-API-UI-14).

The ticket editor restores reference fields from the cached-record arrays in ticket `value`, writes the corresponding IDs with the field's single/multiple cardinality, and restores uploaded files from `attachments[].signedId` (INF-UI-R4-10). It converts datetime-local control values to and from kickflow's `YYYY-MM-DD HH:mm` format (INF-UI-R4-11). Conditional sections evaluate custom A-J expressions with the same ordering, parentheses, and boolean precedence as the backend (INF-UI-R4-12). User-reference ticket snapshots include only public user custom fields, and a live contract scenario validates the populated `TicketDetail` response against the OpenAPI schema (INF-F5-05). Draft submission refreshes cached detail references and the applicant-team snapshot before route selection (INF-F5-02).

User/API-authenticated mutation audit rows return `systemType: null`; only automated operations use `automation`. Route conditions resolve `GeneralMasterCode` and `GeneralMasterItemCode` and return full public metadata for both nested objects (API-04, API-15). General-master field removal deletes matching item inputs and stable input IDs in the same database transaction. Mutation validation enforces OpenAPI length limits for general masters, custom fields and items, teams and roles (API-12, API-13).
| Sorting | INF-02 | `internal/httpx` |
| Array query | `key[]=a&key[]=b`. For parameters typed "string or string[]" a bare `key=a` is also accepted. A bare repeated key for an array-only parameter returns `400 invalid_parameter` with message `Invalid request: value at root is not an array` | `internal/httpx` |
| Auth | Bearer PAT / SAT with `X-Caller-Id`, or UI session cookie; see `design-auth-admin.md` | `internal/auth` |
| Permissions | RBAC from admin roles; `403 missing_permission`. Restricted role entries (folders, general masters, teams) are enforced per resource and list endpoints filter to the covered resources (INF-84, `design-auth-admin.md`). Endpoints whose description names no permission are open to any authenticated active user (INF-42) | `internal/auth` |
| Tenant modes and IP allowlist | Middleware: `suspended` -> `403 tenant_suspended`, `maintenance` -> `403 tenant_maintenance` on every `/v1` request; IP allowlist -> `403 ip_address_denied`. Engine (ordering per INF-41): `admin_working` -> `403 admin_is_working` on ticket create/update/updateForm/approve/reject/deny/withdraw/archive | `internal/auth`, `internal/engine` |
| CORS | `Access-Control-Allow-Origin: *`, preflight `OPTIONS` answered with 200 before routing; allowed headers `Authorization, Content-Type, X-Caller-Id, X-Rate-Limit-Secret`; exposed headers `Link, Page, Per-Page, Total, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset, X-Request-Id`. Cookies are never honored on cross-origin requests because of the `*` origin | `internal/httpx` |
| Rate limit | INF-07 | `internal/httpx` |
| Request ID | Every response carries `X-Request-Id` (generated UUID); stored in audit logs | `internal/httpx` |

Middleware order for `/v1`: request id -> recover (500 `internal_server_error`) -> CORS/preflight -> fault injection -> 411 -> rate limit -> authentication -> tenant `suspended`/`maintenance` and IP checks -> `lastUsedOn` update -> ServeMux.

### Error code catalog

Every documented code is producible and has at least one test. "Natural" means produced by real behavior; "Fault" means only via the fault-injection admin API (INF-32).

| HTTP | code | Trigger in punchflow | Kind |
|---|---|---|---|
| 400 | `invalid_parameter` | Missing/invalid parameters, unknown field code, duplicate IDs, query string params over 300 chars | Natural |
| 400 | `invalid_content_type` | Wrong `Content-Type` | Natural |
| 400 | `invalid_json` | Unparsable body | Natural |
| 400 | `payload_too_large` | INF-39 | Natural |
| 400 | `invalid_header` | Malformed `X-Caller-Id` (INF-05) | Natural |
| 401 | `invalid_access_token` | Missing, unknown or revoked token / session | Natural |
| 401 | `user_not_registered` | INF-06 | Natural |
| 401 | `user_inactive` | INF-06 | Natural |
| 401 | `email_not_verified` | Fault | Fault |
| 401 | `invalid_caller_id` | INF-05 | Natural |
| 401 | `invitation_expired` | INF-06 | Natural |
| 403 | `missing_permission` | RBAC | Natural |
| 403 | `license_purchase_required` | Fault | Fault |
| 403 | `upgrade_required` | INF-27 | Natural |
| 403 | `option_purchase_required` | Fault | Fault |
| 403 | `invisible_ticket` | INF-36 | Natural |
| 403 | `user_not_assigned` | Approve/reject/deny by a non-assignee | Natural |
| 403 | `ticket_create_restricted` | INF-28 | Natural |
| 403 | `ip_address_denied` | Tenant IP allowlist | Natural |
| 403 | `admin_is_working` | Tenant mode `admin_working` | Natural |
| 403 | `tenant_suspended` | Tenant mode `suspended` | Natural |
| 403 | `tenant_maintenance` | Tenant mode `maintenance` | Natural |
| 403 | `resource_not_editable` | Root folder update/delete, super admin role update/delete | Natural |
| 404 | `resource_not_found` | Unknown id/code | Natural |
| 404 | `endpoint_not_found` | Unknown path or method | Natural |
| 411 | (empty body) | 411 rule | Natural |
| 422 | `validation_failed` | Business validation | Natural |
| 422 | `invalid_state` | INF-24 | Natural |
| 422 | `domain_not_allowed` | Tenant allowed e-mail domains on createUser/updateUser | Natural |
| 422 | `assignment_required_before_approve` | INF-20 | Natural |
| 422 | `input_required_before_approve` | Required approver field empty on approve | Natural |
| 429 | `rate_limited` | INF-07 | Natural |
| 500 | `internal_server_error` | Recovered panic; also Fault | Natural + Fault |
| 503 | `feature_disabled` | Fault | Fault |
| 504 | `gateway_timeout` | Fault | Fault |

### Operation matrix (96 operations, 57 paths)

Permission column uses the kickflow permission identifiers; "any" means any authenticated active user (INF-42); "visible" means ticket visibility (INF-36). Handler files live in `internal/api/v1/`; each handler file `X.go` has `X_test.go` with httptest cases for success, field names, pagination (list endpoints) and at least one error path. Every row is also exercised by the contract test.

| # | operationId | Method | Path | Permission | Handler | Notes |
|---|---|---|---|---|---|---|
| 1 | getCurrentUser | GET | /v1/user | any | users.go | Returns the effective user (SAT: caller) |
| 2 | listUsers | GET | /v1/users | user | users.go | Filters per spec; `lastUsedOn` included |
| 3 | createUser | POST | /v1/users | user | users.go | Status `invited`, invitation expires in 7 days, no e-mail (DIV-02); `domain_not_allowed`; `customFields` only for defined codes (`user-custom-fields.json`), else 422 (INF-81); `email` must be an address only (`Name <a@b>`, comments, quoted parts and surrounding spaces are `400 invalid_parameter`) so the response satisfies `format: email` |
| 4 | getUser | GET | /v1/users/{userId} | user | users.go | id or code |
| 5 | deleteUser | DELETE | /v1/users/{userId} | user | users.go | Soft delete -> `deactivated`, returns UserDetail; archives the user's open tickets (`ticket_archived`) and removes unfinished assignments without skipping or completing affected approval steps (INF-79, INF-LIFE-API-06) |
| 6 | updateUser | PATCH | /v1/users/{userId} | user | users.go | Partial update; `email` is address-only as in createUser; `domain_not_allowed`; `customFields` validated against the definitions (INF-81) |
| 7 | lookupUserByEmail | GET | /v1/users/lookupByEmail | user | users.go | Exact, case-insensitive match |
| 8 | reinviteUser | POST | /v1/users/{userId}/reinvite | user | users.go | `deactivated`/`invited` -> `invited` with a new invitation; else INF-24 |
| 9 | suspendUser | POST | /v1/users/{userId}/suspend | user | users.go | `activated` -> `suspended`; else INF-24 |
| 10 | reactivateUser | POST | /v1/users/{userId}/reactivate | user | users.go | `suspended` -> `activated`; else INF-24 |
| 11 | listUserTeams | GET | /v1/users/{userId}/teams | team | users.go | Teams in the active org chart |
| 12 | listUserRoles | GET | /v1/users/{userId}/roles | role | users.go | |
| 13 | listGrades | GET | /v1/grades | user | grades.go | |
| 14 | createGrade | POST | /v1/grades | user | grades.go | Random code when omitted; level unique -> 422 |
| 15 | getGrade | GET | /v1/grades/{gradeId} | user | grades.go | id or code |
| 16 | deleteGrade | DELETE | /v1/grades/{gradeId} | user | grades.go | In use -> 422 `validation_failed` |
| 17 | updateGrade | PATCH | /v1/grades/{gradeId} | user | grades.go | |
| 18 | setDefaultGrade | POST | /v1/grades/{gradeId}/default | user | grades.go | Clears previous default |
| 19 | listProxyApplicants | GET | /v1/proxyApplicants | user | proxies.go | |
| 20 | createProxyApplicant | POST | /v1/proxyApplicants | user, or self when tenant `selfServiceProxy` | proxies.go | One per (user, proxyUser) pair -> 422 |
| 21 | updateProxyApplicant | PATCH | /v1/proxyApplicants/{proxyApplicantId} | user, or self | proxies.go | Partial (absent keeps, `null` clears `startsOn`/`endsOn`, INF-82); empty `workflowIds` = all workflows; de-duplicated |
| 22 | deleteProxyApplicant | DELETE | /v1/proxyApplicants/{proxyApplicantId} | user, or self | proxies.go | |
| 23 | listProxyApprovers | GET | /v1/proxyApprovers | user | proxies.go | |
| 24 | createProxyApprover | POST | /v1/proxyApprovers | user, or self | proxies.go | Effective immediately; future `startsOn` registrations are applied lazily on the next authenticated request at/after their JST date; automatic watches preserve explicit opt-outs (INF-16, INF-23, INF-SHR-01) |
| 25 | updateProxyApprover | PATCH | /v1/proxyApprovers/{proxyApproverId} | user, or self | proxies.go | De-duplicates `workflowIds`; `null` clears `startsOn`/`endsOn` (INF-82) |
| 26 | deleteProxyApprover | DELETE | /v1/proxyApprovers/{proxyApproverId} | user, or self | proxies.go | |
| 27 | listOrganizationCharts | GET | /v1/organizationCharts | team | orgcharts.go | Without teams |
| 28 | createOrganizationChart | POST | /v1/organizationCharts | team | orgcharts.go | Inactive on create |
| 29 | deleteOrganizationChart | DELETE | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | Active chart -> 422 `validation_failed`; synchronous (INF-23) |
| 30 | getOrganizationChart | GET | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | |
| 31 | updateOrganizationChart | PATCH | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | |
| 32 | getCurrentOrganizationChart | GET | /v1/organizationChart | team | orgcharts.go | |
| 33 | activateOrganizationChart | POST | /v1/organizationCharts/{organizationChartId}/activate | team | orgcharts.go | Optional `dueOn` (JST date) schedules the activation and sets `activationPlan`; due plans are applied lazily on the next authenticated request (INF-80); otherwise synchronous (INF-23); ticket-level team viewers remap by team code (INF-SHR-04) |
| 34 | listTeams | GET | /v1/organizationCharts/{organizationChartId}/teams | team | teams.go | `parentId` filter, roots when absent |
| 35 | createTeam | POST | /v1/organizationCharts/{organizationChartId}/teams | team | teams.go | |
| 36 | getTeam | GET | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | id or code |
| 37 | updateTeam | PATCH | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | Parent cycle -> 422 |
| 38 | deleteTeam | DELETE | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | Reparents direct child teams to the deleted team's parent and archives its unfinished authored tickets with lifecycle events/timestamps; completed snapshots remain (INF-LIFE-API-05) |
| 39 | listTeamMembers | GET | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | MemberUser without `userType` |
| 40 | createTeamMembers | POST | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | Max 10; team without a leader afterwards -> 422 |
| 41 | deleteTeamMembers | DELETE | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | JSON body on DELETE; max 10; leaderless remainder -> 422 |
| 42 | updateTeamMember | PATCH | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId} | team | teams.go | Leaderless result -> 422 |
| 43 | listCategories | GET | /v1/categories | any | categories.go | |
| 44 | createCategory | POST | /v1/categories | workflow_misc | categories.go | |
| 45 | deleteCategory | DELETE | /v1/categories/{categoryId} | workflow_misc | categories.go | |
| 46 | updateCategory | PATCH | /v1/categories/{categoryId} | workflow_misc | categories.go | |
| 47 | listFolders | GET | /v1/folders | any | folders.go | |
| 48 | createFolder | POST | /v1/folders | workflow_misc | folders.go | Omitted/null parent places the folder under the root folder (root exists from fixtures, INF-83) |
| 49 | deleteFolder | DELETE | /v1/folders/{folderId} | workflow_misc | folders.go | Root -> `resource_not_editable`; deletes the folder with its descendants in one transaction, retiring (status `deleted`) the workflows and routes inside; needs `workflow`/`route` coverage of each affected folder (INF-83, INF-84) |
| 50 | updateFolder | PATCH | /v1/folders/{folderId} | workflow_misc | folders.go | Root -> `resource_not_editable` |
| 51 | getFolder | GET | /v1/folders/{folderId} | any | folders.go | id or code |
| 52 | listRoutes | GET | /v1/routes | any | definitions.go | Fixture-defined, read-only (DIV-05) |
| 53 | getRoute | GET | /v1/routes/{routeId} | any | definitions.go | RouteDetail with steps |
| 54 | listWorkflows | GET | /v1/workflows | any | definitions.go | `status` filter per spec |
| 55 | getWorkflow | GET | /v1/workflows/{workflowId} | any | definitions.go | WorkflowDetail; integration fields null (DIV-03) |
| 56 | listTickets | GET | /v1/tickets | visible (all with ticket_read) | tickets.go | All documented filters; TicketWithStep without inputs/route; INF-02 |
| 57 | createTicket | POST | /v1/tickets | any (INF-28) | tickets.go | Engine `Create`; events `ticket_created` (+ `ticket_opened`, and `ticket_completed` when the route needs no user action, INF-11, INF-72); `403 ticket_create_restricted` per INF-28; formulas evaluated (INF-21); `viewers[].userId` and `watcherUserId` (INF-NES-16) |
| 58 | listTasks | GET | /v1/tickets/tasks | any | tickets.go | INF-34, INF-LIFE-WH13, INF-NUM-02; absent `pending` leaves results unchanged, `pending=true` filters held assignments, and `pending=false` filters non-held assignments; filtering precedes pagination and `Total`; `subStatusIds[]` filters tasks as it does tickets; explicit statuses retain historical assignments (API-03) |
| 59 | getTicket | GET | /v1/tickets/{ticketId} | visible | tickets.go | TicketDetail incl. `actualApprover` |
| 60 | updateTicket | PATCH | /v1/tickets/{ticketId} | author (draft/rejected) or current approver (in_progress, authorized form/detail fields) | tickets.go | Engine `Update`; `ticket_updated`, `ticket_opened` (+ `ticket_completed`, INF-11, INF-72); same-state `status` saves (INF-71); `approvers[]` by the author nominates `author_customizable` approvers (INF-70); `slipItems` replace all rows for applicants and preserve omitted sections for approvers (INF-75, INF-F5-01); `viewers[]` replaces the user viewers and `watcherUserId` subscribes a watcher (INF-NES-16) |
| 61 | updateTicketForm | PATCH | /v1/tickets/{ticketId}/form | same as #60 | tickets.go | Engine `UpdateForm`; empty body -> 400; duplicates -> 400; per-section differential detail rows (INF-75, INF-F5-01); each submitted applicant row requires every editable cell, while malformed row inputs are rejected without changing saved cells (INF-F5-03); unchanged historical references survive unrelated patches (INF-F5-06); `ticket_updated` |
| 62 | approveTicket | POST | /v1/tickets/{ticketId}/approve | current assignee or proxy | ticket_actions.go | Engine `Approve`; `ticket_approved`/`ticket_confirmed` for the step acted on even when it needs more approvals (INF-74) (+ `ticket_completed`); approver-required form and detail cells checked (INF-69); INF-22, INF-41 |
| 63 | rejectTicket | POST | /v1/tickets/{ticketId}/reject | current assignee or proxy | ticket_actions.go | Engine `SendBack`; `ticket_rejected`; INF-12 (skipped steps stay skipped); sub-status restriction INF-61 |
| 64 | denyTicket | POST | /v1/tickets/{ticketId}/deny | current assignee or proxy | ticket_actions.go | Engine `Deny`; `ticket_denied` |
| 65 | withdrawTicket | POST | /v1/tickets/{ticketId}/withdraw | author / proxy client | ticket_actions.go | Engine `Withdraw`; `ticket_withdrawn`; INF-25 |
| 66 | archiveTicket | POST | /v1/tickets/{ticketId}/archive | author (draft/rejected) or ticket_write | ticket_actions.go | Engine `Archive`; `ticket_archived`; INF-24 |
| 67 | listTicketLinks | GET | /v1/tickets/{ticketId}/links | visible | ticket_viewers.go | Links come from `ticket` fields with `autoLink`, maintained as reciprocal links on every edit (INF-78) |
| 68 | listViewers | GET | /v1/tickets/{ticketId}/viewers | visible | ticket_viewers.go | Default order team name, user email |
| 69 | createViewer | POST | /v1/tickets/{ticketId}/viewers | visible; workflow `allowEditingOfViewers` or ticket_write | ticket_viewers.go | Exactly one of userId/teamId -> else 400 |
| 70 | deleteViewer | DELETE | /v1/tickets/{ticketId}/viewers/{viewerId} | same as #69 | ticket_viewers.go | |
| 71 | uploadFile | POST | /v1/files | any | files.go | multipart field `file`; INF-26, INF-27, INF-39 |
| 72 | getFile | GET | /v1/files/{signedId} | visible ticket of the attachment | files.go | INF-26 |
| 73 | listGeneralMasters | GET | /v1/generalMasters | master_read or master | general_masters.go | |
| 74 | createGeneralMaster | POST | /v1/generalMasters | master | general_masters.go | Fields (custom fields) included |
| 75 | getGeneralMaster | GET | /v1/generalMasters/{generalMasterId} | master_read or master | general_masters.go | id or code |
| 76 | updateGeneralMaster | PATCH | /v1/generalMasters/{generalMasterId} | master | general_masters.go | `fields` merge by `code` (INF-82) |
| 77 | deleteGeneralMaster | DELETE | /v1/generalMasters/{generalMasterId} | master | general_masters.go | Deletes items |
| 78 | listGeneralMasterItems | GET | /v1/generalMasters/{generalMasterId}/items | master_read or master | general_masters.go | Default order `defaultSortBy` |
| 79 | createGeneralMasterItem | POST | /v1/generalMasters/{generalMasterId}/items | master | general_masters.go | Custom field values validated by field type |
| 80 | getGeneralMasterItem | GET | /v1/generalMasters/{generalMasterId}/items/{itemId} | master_read or master | general_masters.go | id or code |
| 81 | updateGeneralMasterItem | PATCH | /v1/generalMasters/{generalMasterId}/items/{itemId} | master | general_masters.go | |
| 82 | deleteGeneralMasterItem | DELETE | /v1/generalMasters/{generalMasterId}/items/{itemId} | master | general_masters.go | |
| 83 | listRoles | GET | /v1/roles | role | roles.go | |
| 84 | createRole | POST | /v1/roles | role | roles.go | Duplicate ids: exact -> 400, case-only -> 422 |
| 85 | getRole | GET | /v1/roles/{roleId} | role | roles.go | |
| 86 | updateRole | PATCH | /v1/roles/{roleId} | role | roles.go | Super admin -> `resource_not_editable`; caller's own role (non super admin) -> `403 missing_permission` (INF-43) |
| 87 | deleteRole | DELETE | /v1/roles/{roleId} | role | roles.go | Same rules as #86 |
| 88 | createRoleMembers | POST | /v1/roles/{roleId}/memberships | role | roles.go | Max 10; adding self (non super admin) -> 403 (INF-43) |
| 89 | listRoleMembers | GET | /v1/roles/{roleId}/memberships | role | roles.go | User without `customFields` |
| 90 | deleteRoleMember | DELETE | /v1/roles/{roleId}/memberships/{userId} | role | roles.go | Removing the last super admin -> 422; removing self (non super admin) -> 403 (INF-43) |
| 91 | listComments | GET | /v1/tickets/{ticketId}/comments | visible | comments.go | INF-30 |
| 92 | createComment | POST | /v1/tickets/{ticketId}/comments | visible | comments.go | `commentingEnabled: false` -> `422 validation_failed` with `errors.base` (engine `AddComment` returns it directly); mentions `<@uuid>`; up to 10 `files`; `comment_created` |
| 93 | getComment | GET | /v1/tickets/{ticketId}/comments/{commentId} | visible | comments.go | |
| 94 | updateComment | PATCH | /v1/tickets/{ticketId}/comments/{commentId} | visible + comment author (INF-37) | comments.go | Attachments unchanged; `comment_updated` |
| 95 | deleteComment | DELETE | /v1/tickets/{ticketId}/comments/{commentId} | visible + comment author (INF-37) | comments.go | Soft delete; `comment_updated` |
| 96 | listAuditLogs | GET | /v1/auditLogs | audit | audit_logs.go | INF-29; mutation `systemType` is null except for automation (API-04) |

Path count check: the 57 distinct paths are the distinct values of the Path column. Handler file count: 15 files, none expected near 1000 lines; `tickets.go` must be split further if it approaches the limit.

### Webhook event matrix (13 events)

All events use the envelope `{eventType, tenant, user, data}`. Ticket payloads use `TicketDetail` with a webhook-specific policy: restricted form values and resolved references are redacted while field definitions remain (INF-SEC-02). Comment payloads are `{comment: Comment, ticket: TicketWithStep}`. Emission happens inside the same database transaction as the state change (outbox, see `design-webhooks.md`). Tests live in `internal/engine/events_test.go` (which events, in which order) and `internal/webhook/*_test.go` (delivery, signature, retries).

| # | eventType | Emitted by | `user` | `data` | Notes |
|---|---|---|---|---|---|
| 1 | ping | Admin API webhook create; update that changes `url` or `secret`; admin "send test" | Acting admin | `{message: "ping"}` | Not filtered by workflow/event selection |
| 2 | ticket_created | createTicket (#57) | Actor | ticket | |
| 3 | ticket_updated | updateTicket (#60) with field changes, updateTicketForm (#61); punchflow UI control operations (cancel approval, raise, custom steps, labels; INF-66) | Actor | ticket | INF-11; same-state `status` saves emit it only when something changed (INF-71) |
| 4 | ticket_opened | createTicket with `in_progress`; updateTicket draft/rejected -> in_progress | Actor | ticket | INF-11; followed by `ticket_completed` when the route needs no user action (INF-72) |
| 5 | ticket_approved | approveTicket on an `approve` step, including a partial approval that leaves the step open | Actor (assignee or proxy) | ticket | One event per call even when the call completes several parallel steps (INF-74) |
| 6 | ticket_confirmed | approveTicket on a `confirm` step | Actor | ticket | If the call covers both approve and confirm steps of one group, `ticket_approved` is emitted once (approve wins) |
| 7 | ticket_rejected | rejectTicket | Actor | ticket | INF-12 |
| 8 | ticket_denied | denyTicket | Actor | ticket | |
| 9 | ticket_completed | approveTicket that completes the last group; a submission (createTicket / updateTicket) whose route needs no user action | Actor | ticket | After #5/#6 or after #4 (INF-11, INF-72) |
| 10 | ticket_withdrawn | withdrawTicket | Actor | ticket | |
| 11 | ticket_archived | archiveTicket; deleteUser (#5) and deleteTeam (#38) for each archived unfinished ticket | Actor | ticket | INF-79, INF-LIFE-API-05 |
| 12 | comment_created | createComment; comments attached to approve/reject/deny (INF-22) | Comment author | comment + ticket | |
| 13 | comment_updated | updateComment, deleteComment | Actor | comment + ticket | Delete payload has `deletedAt` set (INF-30) |

The admin API can also enqueue any of the 13 event types for an existing ticket/comment on demand ("fire event", see `design-webhooks.md`), which uses the same payload builders.

### Enum contract (frozen in PF-02)

Added in session 462 to close carried finding CF-01 (`notes.md`). The values come from the cited OpenAPI schema property (`#/components/schemas/<Schema>/properties/<prop>`); where this table lists values, they were checked against the spec. "Excluded" values exist in the spec but punchflow never produces or accepts them. `internal/model/enums.go` exports one canonical `[]string` per row, named in the "Go list" column, plus one constant per value.

| Enum | Values | Source | Excluded (reason) | Go list |
|---|---|---|---|---|
| Ticket status | `draft`, `in_progress`, `completed`, `rejected`, `denied`, `archived` | `TicketSummary.status` | `permanently_deleted` (DIV-04) | `TicketStatuses` |
| Ticket step status | `pending`, `in_progress`, `completed`, `skipped`, `raised` | `TicketStep.status` | - | `StepStatuses` |
| Action type | `approve`, `confirm`, `none` | `RouteStep.actionType`, `TicketStep.actionType` | - | `ActionTypes` |
| Step type | `author`, `manager`, `team`, `user`, `author_customizable`, `assignee_customizable`, `dynamic_team`, `dynamic_user` | `RouteStep.stepType` | - | `StepTypes` |
| Fallback | `direct_manager`, `higher_manager`, `skip`, `no_fallback`, `higher_team`, or `null` | `RouteStep.fallbackType`, `TicketStep.fallbackResult` | - | `FallbackTypes` |
| Manager selection | `closest`, `all` | `RouteStep.managerSelectionType` | - | `ManagerSelectionTypes` |
| Grade symbol | `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `any_of`, or `null` | `RouteStepTarget.gradeSymbol` | `""` is not a value; no grade filter is `null` | `GradeSymbols` |
| Step condition type | `always`, `conditional`, `conditional_skip` | `RouteStepCondition.conditionType` | - | `StepConditionTypes` |
| Step condition combination | `all`, `any` | `RouteStepCondition.combinationType` | - | `StepCombinationTypes` |
| Step condition field key | spec enum | `RouteStepConditionField.fieldKey` | `other_variable` (INF-18) | `StepConditionFieldKeys` |
| Route condition type | `always`, `field`, `field_otherwise` | `WorkflowRouteCondition.conditionType` | - | `RouteConditionTypes` |
| Route condition combination | `all`, `any`, `custom` | `WorkflowRouteCondition.combinationType` | `custom` uses the A-J expression grammar with `&&`, `||`, and parentheses (INF-CFG-02) | `RouteCombinationTypes` |
| Section type | `form`, `slip` | `SectionListItem.sectionType` | `conditional` is not a type. A conditional section is a `form` section with `conditional: true`, `combinationType` (`all`/`any`/`custom`), `combinationExpression` for `custom`, and its own `conditionFields`. Slip sections carry `code` and `allowApproverToAddItems` (default false; INF-CFG-01) | `SectionTypes` |
| Route status | `visible`, `deleted`, `error` | `Route.status` | - | `RouteStatuses` |
| Workflow status | `visible`, `invisible`, `deleted` | `Workflow.status` | - | `WorkflowStatuses` |
| User status | `invited`, `activated`, `suspended`, `deactivated` | `User.status` | - | `UserStatuses` (`UserStatusActive = "activated"`) |
| User type | `normal`, `assistant` | `User.userType` | - | `UserTypes` |
| Admin permission | `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` (19) | `RoleDetail.permissionList[].permission` | - | `Permissions` |
| Master field type | `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date` | `GeneralMasterField.fieldType` | - | `MasterFieldTypes` |
| Master default sort | `name`, `code` | `GeneralMaster.defaultSortBy` | - | `MasterSortKeys` |
| Webhook event type | `ping`, `ticket_created`, `ticket_updated`, `ticket_opened`, `ticket_approved`, `ticket_confirmed`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `ticket_withdrawn`, `ticket_archived`, `comment_created`, `comment_updated` (13) | `kickflow-api.md` section 5.5 (the spec has no enum here) | - | `WebhookEventTypes` |

Rules:

1. Fixture validation (PF-06), request validation (PF-18..PF-24), the engine (PF-14) and presenters (PF-08, PF-15) check and emit these values only through the Go lists and constants. No other package hard-codes these literals.
2. A value outside its row is a validation problem in fixtures and `400 invalid_parameter` / `422 validation_failed` in requests (as the coverage row says). It is never stored or emitted. Values that kickflow does not have, such as `ticket_submitted`, `ticket_remanded`, `ticket_resubmitted`, `comment_deleted`, the invented permissions `ticket_admin`, `audit_read`, `user_read`, `user_write`, `org_read`, `org_write`, `workflow_read`, `workflow_write`, `role_read`, `role_write`, `file_read`, `file_write`, `proxy_read`, `proxy_write` (the real `ticket_read`, `ticket_write`, `master_read` stay), step types `grade`/`parallel`, ticket status `withdrawn`, step statuses `approved`/`denied`/`remanded`, user type `member`, route/workflow status `active` and section type `conditional`, must not exist as model constants. "Parallel" is a set of steps that share one `stepOrder` (INF-13). "Grade" is a target filter (`gradeSymbol` + `gradeCodes`, INF-19). Withdraw yields `rejected`. Deny and send-back act on the ticket, not on the step.
3. PF-02 removes the constants that nothing else references. A deprecated constant that another package still references (currently `StepTypeGrade` and `StepTypeParallel` in `internal/fixtures/validate.go`) is removed once that package switches to the Go list (PF-06 in the same continuation wave). PF-27 confirms that no such identifier is left.
4. Conformance test `internal/model/enums_test.go` (PF-02; test-only use of `gopkg.in/yaml.v3`) loads `design-docs/references/kickflow-openapi.yaml`. For every row with an OpenAPI source, it asserts that the Go list equals the spec enum minus the "Excluded" values (set equality, `null` ignored). It compares `WebhookEventTypes` with the 13 literals above. This test replaces the hand-written expectations in `internal/store/store_test.go` `TestModelEnumValuesMatchDesign`.
5. Data conformance: `fixtures/default` (PF-07) passes `fixtures.Validate`, which a root test in `fixtures_default_test.go` asserts. The PF-06 invalid testdata covers at least step type `grade`, permission `audit_read`, event `ticket_submitted`, user type `member` and route status `active`.

### Contract test

Location: `internal/contract/` (test-only package; `contract_test.go` plus per-resource scenario files to stay under 1000 lines).

1. Load `design-docs/references/kickflow-openapi.yaml` (path computed relative to the module root) with a YAML parser and convert it to a JSON document.
2. Route coverage: collect every `(method, path)` under `paths` (exactly 96 expected; the test fails if the count changes). Convert `{param}` segments to ServeMux wildcards and compare with `apiv1.Operations()` (exported table of `{OperationID, Method, Pattern}` used for registration). Both directions must match exactly, and operationIds must match.
3. Schema validation: compile each operation's `200` `application/json` response schema with a JSON Schema 2020-12 validator, using the whole spec document as the resource so `#/components/schemas/...` references resolve. Unknown OpenAPI keywords are ignored. `format` assertions are enabled for `uuid`, `date`, `date-time` and `email`.
4. Scenarios: every one of the 96 operations has at least one live HTTP scenario running against `testutil.New(t)` (default fixtures, fresh SQLite), using a persona token with the needed permission. The response must be 200 and validate; empty-body operations must return zero bytes. Session 466 partitions the scenarios as described in "Live scenario partition (session 466)" below.
5. Timestamp check: every string value under a key ending in `At` must match `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+09:00$`.
6. Webhook payloads: one sample body of each of the 13 events (captured by the test receiver) is validated against `WebhookRequestBodyPing`, `WebhookRequestBodyTicket` or `WebhookRequestBodyComment`.

### Live scenario partition (session 466)

State at `a2a82de`: `internal/contract` passes with route coverage (`TestContractRoutes`), schema compilation for all 96 operations (`TestContractScenarios`), live GET scenarios for 22 read operations (`TestContractLiveReadScenarios`) and delivered, signed, schema-validated payloads for all 13 events (`TestContractWebhookPayloads`). `TestServeSmoke` passes. The remaining 74 operations get live scenarios from six sub-plans, PF-26a..PF-26f. All six are ready in one wave. `pf-26-contract-smoke.md` becomes a superseded index (not dispatched), like `pf-14-engine.md`.

Frozen during the wave (no sub-plan edits them): `helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go` and `internal/cli/serve_smoke_test.go`. A sub-plan that needs a helper defines it in its own file.

| Plan | Scenario file (new) | Test function | Operations (coverage-matrix rows) | Count |
|---|---|---|---|---|
| PF-26a tickets | `scenarios_tickets_test.go` | `TestContractLiveTickets` | #57, #59-#70 | 13 |
| PF-26b comments and files | `scenarios_comments_files_test.go` | `TestContractLiveCommentsFiles` | #71, #72, #91-#95 | 7 |
| PF-26c users and grades | `scenarios_users_grades_test.go` | `TestContractLiveUsersGrades` | #3, #5-#12, #14, #16-#18 | 13 |
| PF-26d org charts and teams | `scenarios_orgcharts_teams_test.go` | `TestContractLiveOrgChartsTeams` | #28-#31, #33-#42 | 14 |
| PF-26e categories, folders, general masters | `scenarios_masters_test.go` | `TestContractLiveMasters` | #44-#46, #48-#50, #74, #76-#82 | 14 |
| PF-26f proxies and roles | `scenarios_proxies_roles_test.go` | `TestContractLiveProxiesRoles` | #20-#22, #24-#26, #84-#90 | 13 |

The 74 rows plus the 22 operations of `TestContractLiveReadScenarios` are the 96 operations, each exactly once. Rows #52-#55 and #96 are read-only and already covered. PF-26c also owns `scenarios_users_org_test.go`. Its only allowed change is to move the 22 read cases into a package-level `liveReadScenarios` slice, without changing their behavior or subtest names.

Rules for every sub-plan file:

1. Inventory. The file declares `var live<Group>Operations = []string{...}` with exactly its rows' operationIds (`<Group>` as in the test function name). The test fails if an id is missing from `apiv1.Operations()`, or if a listed id has no passing subtest named `TestContractLive<Group>/<operationId>`. Extra subtests use `<operationId>/<case>`.
2. Isolation. Each test function creates its own `testutil.New(t)` env and runs its mutations in dependency order (create -> get -> update -> state change -> delete). It uses fixture codes or ids returned by earlier responses. It never depends on another sub-plan's data or test order.
3. Success checks for each operation: status 200. A non-empty body validates against the operation's 200 schema (`responseSchemaURI`) and passes `checkTimestamps`. An operation whose spec 200 response has no content returns zero bytes. Every operation in the group whose spec declares the `page`/`perPage` query parameters returns `Page`, `Per-Page` and `Total` headers. The test reads this from the spec operation, not from a hand-written list. If the group has such an operation, at least one call uses `perPage=1` with `Total` > 1 and asserts a `Link` header with `rel="next"` (INF-01).
4. Field names. The schema validation in check 3 is the field-name check: the response schemas list the required properties. Check 3 adds no hand-written key lists.
5. Error format. Each resource in the group has at least one error subtest, `<operationId>/error_<code>`, using a documented trigger from the operation matrix or the error catalog (for example `resource_not_found`, `validation_failed`, `invalid_state`, `missing_permission`, `resource_not_editable`). It asserts the status and that the body validates against `#/components/schemas/ErrorResponse`. Its keys must be a subset of `code`, `message` and `errors`. `errors` is present only on 422, and `doc_url` never appears (cross-cutting "Error body" row).
6. Webhooks. Scenarios do not assert webhook delivery. Payload shape and signatures are covered by `TestContractWebhookPayloads`, and emission order by the engine and handler tests. No scenario waits on asynchronous delivery.
7. Size and names. Each file stays under 1000 lines. Every package-level identifier, other than the test function and the inventory variable, starts with that plan's prefix `pf26a`..`pf26f`.
8. Never loosen. A failure is never fixed by loosening the validator, skipping an operation, special-casing an operation or editing a frozen file.

Defect repair and ownership transfer. Accepted plans are not redispatched in session 466. A schema or behavior defect that a scenario finds is fixed by the sub-plan that found it. Only the repair-eligible files below may be edited, and only for that defect. Each production file is eligible for exactly one sub-plan, so `writePaths` stay disjoint at dispatch. A file becomes a recorded `ownershipTransfers[]` entry (`fromPlan` = the accepted owner) only if the sub-plan actually changes it. Unchanged eligible files stay with their accepted owner.

| Plan | Repair-eligible production files (plus the matching existing `_test.go` file, edited only to keep it consistent with the fix) |
|---|---|
| PF-26a | `internal/api/v1/tickets.go`, `ticket_actions.go`, `ticket_viewers.go`; `internal/present/ticket.go`, `ticket_detail.go`, `viewer.go`; `internal/store/tickets.go`, `read_tickets.go`, `ticket_queries.go`; `internal/engine/engine.go`, `actions.go`, `create.go`, `update.go`, `forms.go`, `routing.go`, `conditions.go`, `visibility.go`, `numbering.go` |
| PF-26b | `internal/api/v1/comments.go`, `files.go`; `internal/present/comment.go`; `internal/store/files.go`; `internal/engine/comments.go`; `internal/files/files.go`, `download.go` |
| PF-26c | `internal/api/v1/users.go`, `users_status.go`, `grades.go`; `internal/present/user.go`; `internal/store/users.go`, `read_users.go`, `grades.go` |
| PF-26d | `internal/api/v1/orgcharts.go`, `teams.go`; `internal/present/org.go`; `internal/store/org.go`, `read_org.go` |
| PF-26e | `internal/api/v1/categories.go`, `folders.go`, `general_masters.go`; `internal/present/classification.go`, `master.go`; `internal/store/classification.go`, `read_classification.go`, `masters.go`, `read_masters.go` |
| PF-26f | `internal/api/v1/proxies.go`, `roles.go`; `internal/present/proxy.go`, `role.go`; `internal/store/proxies.go`, `read_proxies.go`, `roles.go`, `read_roles.go` |

- Guard verification for an edited file: the whole owning package, uncached, `go test -count=1 -v -race ./internal/<pkg>/...`. A failure there in a sibling's in-flight file follows the s464 sibling-breakage rule.
- Contract stubs keep their exported identifiers and signatures (s464 rule 2), and the `Operations()` table in `routes.go` stays fixed at 96.
- A defect in any other file is not eligible in the wave: `fixtures/default/*.json`, `internal/{httpx,auth,kferr,kftime,model,webhook,testutil,admin,server,cli}`, `internal/api/v1/{api,routes}.go`, `internal/present/{present,tenant,workflow,workflow_form,route,audit}.go` and the other store files. Such a defect is a reconciliation request with `file:line`, the failing subtest and the log path. The sub-plan reports `implementationIncomplete: true` with the request. Integration review then transfers exactly that file to the finding sub-plan, or to the first one to record it if several do, in that plan's correction instructions. Until then no sibling edits the file.
- A defect in a file that is repair-eligible for a sibling sub-plan is a reconciliation request addressed to that sibling. The sibling fixes it within its eligibility, and the finder never edits it.

Aggregate coverage (PF-27, after the wave). PF-27 adds `internal/contract/coverage_test.go` with `TestContractOperationCoverage`. It asserts that the operationIds of `liveReadScenarios` plus the six `live<Group>Operations` lists contain no duplicates and, as a set, equal the 96 operationIds of `apiv1.Operations()`. The six sub-plans are accepted before PF-27 starts, so every referenced identifier exists. The shared tree compiles at every step.

## References

- `design-docs/references/kickflow-api.md` sections 1-5
- `design-docs/references/kickflow-openapi.yaml`
- `design-docs/specs/notes.md` (INF/DIV register)
- `design-docs/specs/design-approval-engine.md`, `design-docs/specs/design-webhooks.md`, `design-docs/specs/design-auth-admin.md`
