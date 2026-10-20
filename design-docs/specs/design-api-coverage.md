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
| Sorting | INF-02 | `internal/httpx` |
| Array query | `key[]=a&key[]=b`. For parameters typed "string or string[]" a bare `key=a` is also accepted. A bare repeated key for an array-only parameter returns `400 invalid_parameter` with message `Invalid request: value at root is not an array` | `internal/httpx` |
| Auth | Bearer PAT / SAT with `X-Caller-Id`, or UI session cookie; see `design-auth-admin.md` | `internal/auth` |
| Permissions | RBAC from admin roles; `403 missing_permission`. Endpoints whose description names no permission are open to any authenticated active user (INF-42) | `internal/auth` |
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
| 3 | createUser | POST | /v1/users | user | users.go | Status `invited`, invitation expires in 7 days, no e-mail (DIV-02); `domain_not_allowed` |
| 4 | getUser | GET | /v1/users/{userId} | user | users.go | id or code |
| 5 | deleteUser | DELETE | /v1/users/{userId} | user | users.go | Soft delete -> `deactivated`, returns UserDetail |
| 6 | updateUser | PATCH | /v1/users/{userId} | user | users.go | Partial update; `domain_not_allowed` |
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
| 21 | updateProxyApplicant | PATCH | /v1/proxyApplicants/{proxyApplicantId} | user, or self | proxies.go | Partial; empty `workflowIds` = all workflows; de-duplicated |
| 22 | deleteProxyApplicant | DELETE | /v1/proxyApplicants/{proxyApplicantId} | user, or self | proxies.go | |
| 23 | listProxyApprovers | GET | /v1/proxyApprovers | user | proxies.go | |
| 24 | createProxyApprover | POST | /v1/proxyApprovers | user, or self | proxies.go | Effective immediately (INF-16, INF-23) |
| 25 | updateProxyApprover | PATCH | /v1/proxyApprovers/{proxyApproverId} | user, or self | proxies.go | De-duplicates `workflowIds` |
| 26 | deleteProxyApprover | DELETE | /v1/proxyApprovers/{proxyApproverId} | user, or self | proxies.go | |
| 27 | listOrganizationCharts | GET | /v1/organizationCharts | team | orgcharts.go | Without teams |
| 28 | createOrganizationChart | POST | /v1/organizationCharts | team | orgcharts.go | Inactive on create |
| 29 | deleteOrganizationChart | DELETE | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | Active chart -> 422 `validation_failed`; synchronous (INF-23) |
| 30 | getOrganizationChart | GET | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | |
| 31 | updateOrganizationChart | PATCH | /v1/organizationCharts/{organizationChartId} | team | orgcharts.go | |
| 32 | getCurrentOrganizationChart | GET | /v1/organizationChart | team | orgcharts.go | |
| 33 | activateOrganizationChart | POST | /v1/organizationCharts/{organizationChartId}/activate | team | orgcharts.go | Synchronous (INF-23) |
| 34 | listTeams | GET | /v1/organizationCharts/{organizationChartId}/teams | team | teams.go | `parentId` filter, roots when absent |
| 35 | createTeam | POST | /v1/organizationCharts/{organizationChartId}/teams | team | teams.go | |
| 36 | getTeam | GET | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | id or code |
| 37 | updateTeam | PATCH | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | Parent cycle -> 422 |
| 38 | deleteTeam | DELETE | /v1/organizationCharts/{organizationChartId}/teams/{teamId} | team | teams.go | Deletes memberships |
| 39 | listTeamMembers | GET | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | MemberUser without `userType` |
| 40 | createTeamMembers | POST | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | Max 10; team without a leader afterwards -> 422 |
| 41 | deleteTeamMembers | DELETE | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships | team | teams.go | JSON body on DELETE; max 10; leaderless remainder -> 422 |
| 42 | updateTeamMember | PATCH | /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId} | team | teams.go | Leaderless result -> 422 |
| 43 | listCategories | GET | /v1/categories | any | categories.go | |
| 44 | createCategory | POST | /v1/categories | workflow_misc | categories.go | |
| 45 | deleteCategory | DELETE | /v1/categories/{categoryId} | workflow_misc | categories.go | |
| 46 | updateCategory | PATCH | /v1/categories/{categoryId} | workflow_misc | categories.go | |
| 47 | listFolders | GET | /v1/folders | any | folders.go | |
| 48 | createFolder | POST | /v1/folders | workflow_misc | folders.go | Parent required except root (root exists from fixtures) |
| 49 | deleteFolder | DELETE | /v1/folders/{folderId} | workflow_misc | folders.go | Root -> `resource_not_editable`; folder with workflows/routes -> 422 `validation_failed` (punchflow cannot delete fixture definitions, DIV-05) |
| 50 | updateFolder | PATCH | /v1/folders/{folderId} | workflow_misc | folders.go | Root -> `resource_not_editable` |
| 51 | getFolder | GET | /v1/folders/{folderId} | any | folders.go | id or code |
| 52 | listRoutes | GET | /v1/routes | any | definitions.go | Fixture-defined, read-only (DIV-05) |
| 53 | getRoute | GET | /v1/routes/{routeId} | any | definitions.go | RouteDetail with steps |
| 54 | listWorkflows | GET | /v1/workflows | any | definitions.go | `status` filter per spec |
| 55 | getWorkflow | GET | /v1/workflows/{workflowId} | any | definitions.go | WorkflowDetail; integration fields null (DIV-03) |
| 56 | listTickets | GET | /v1/tickets | visible (all with ticket_read) | tickets.go | All documented filters; TicketWithStep without inputs/route; INF-02 |
| 57 | createTicket | POST | /v1/tickets | any (INF-28) | tickets.go | Engine `Create`; events `ticket_created` (+ `ticket_opened`, INF-11) |
| 58 | listTasks | GET | /v1/tickets/tasks | any | tickets.go | INF-34, INF-40 |
| 59 | getTicket | GET | /v1/tickets/{ticketId} | visible | tickets.go | TicketDetail incl. `actualApprover` |
| 60 | updateTicket | PATCH | /v1/tickets/{ticketId} | author (draft/rejected) or current approver (in_progress, approver fields only) | tickets.go | Engine `Update`; `ticket_updated`, `ticket_opened` (INF-11) |
| 61 | updateTicketForm | PATCH | /v1/tickets/{ticketId}/form | same as #60 | tickets.go | Engine `UpdateForm`; empty body -> 400; duplicates -> 400; `ticket_updated` |
| 62 | approveTicket | POST | /v1/tickets/{ticketId}/approve | current assignee or proxy | ticket_actions.go | Engine `Approve`; `ticket_approved`/`ticket_confirmed` (+ `ticket_completed`); INF-22, INF-41 |
| 63 | rejectTicket | POST | /v1/tickets/{ticketId}/reject | current assignee or proxy | ticket_actions.go | Engine `SendBack`; `ticket_rejected`; INF-12 |
| 64 | denyTicket | POST | /v1/tickets/{ticketId}/deny | current assignee or proxy | ticket_actions.go | Engine `Deny`; `ticket_denied` |
| 65 | withdrawTicket | POST | /v1/tickets/{ticketId}/withdraw | author / proxy client | ticket_actions.go | Engine `Withdraw`; `ticket_withdrawn`; INF-25 |
| 66 | archiveTicket | POST | /v1/tickets/{ticketId}/archive | author (draft/rejected) or ticket_write | ticket_actions.go | Engine `Archive`; `ticket_archived`; INF-24 |
| 67 | listTicketLinks | GET | /v1/tickets/{ticketId}/links | visible | ticket_viewers.go | Links come from `ticket` fields with `autoLink` |
| 68 | listViewers | GET | /v1/tickets/{ticketId}/viewers | visible | ticket_viewers.go | Default order team name, user email |
| 69 | createViewer | POST | /v1/tickets/{ticketId}/viewers | visible; workflow `allowEditingOfViewers` or ticket_write | ticket_viewers.go | Exactly one of userId/teamId -> else 400 |
| 70 | deleteViewer | DELETE | /v1/tickets/{ticketId}/viewers/{viewerId} | same as #69 | ticket_viewers.go | |
| 71 | uploadFile | POST | /v1/files | any | files.go | multipart field `file`; INF-26, INF-27, INF-39 |
| 72 | getFile | GET | /v1/files/{signedId} | visible ticket of the attachment | files.go | INF-26 |
| 73 | listGeneralMasters | GET | /v1/generalMasters | master_read or master | general_masters.go | |
| 74 | createGeneralMaster | POST | /v1/generalMasters | master | general_masters.go | Fields (custom fields) included |
| 75 | getGeneralMaster | GET | /v1/generalMasters/{generalMasterId} | master_read or master | general_masters.go | id or code |
| 76 | updateGeneralMaster | PATCH | /v1/generalMasters/{generalMasterId} | master | general_masters.go | |
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
| 92 | createComment | POST | /v1/tickets/{ticketId}/comments | visible | comments.go | `commentingEnabled: false` -> 422 `errors.base`; mentions `<@uuid>`; up to 10 `files`; `comment_created` |
| 93 | getComment | GET | /v1/tickets/{ticketId}/comments/{commentId} | visible | comments.go | |
| 94 | updateComment | PATCH | /v1/tickets/{ticketId}/comments/{commentId} | visible + comment author (INF-37) | comments.go | Attachments unchanged; `comment_updated` |
| 95 | deleteComment | DELETE | /v1/tickets/{ticketId}/comments/{commentId} | visible + comment author (INF-37) | comments.go | Soft delete; `comment_updated` |
| 96 | listAuditLogs | GET | /v1/auditLogs | audit | audit_logs.go | INF-29 |

Path count check: the 57 distinct paths are the distinct values of the Path column. Handler file count: 15 files, none expected near 1000 lines; `tickets.go` must be split further if it approaches the limit.

### Webhook event matrix (13 events)

All events use the envelope `{eventType, tenant, user, data}`. Ticket payloads are `TicketDetail` (same presenter as `getTicket`), comment payloads are `{comment: Comment, ticket: TicketWithStep}`. Emission happens inside the same database transaction as the state change (outbox, see `design-webhooks.md`). Tests live in `internal/engine/events_test.go` (which events, in which order) and `internal/webhook/*_test.go` (delivery, signature, retries).

| # | eventType | Emitted by | `user` | `data` | Notes |
|---|---|---|---|---|---|
| 1 | ping | Admin API webhook create; update that changes `url` or `secret`; admin "send test" | Acting admin | `{message: "ping"}` | Not filtered by workflow/event selection |
| 2 | ticket_created | createTicket (#57) | Actor | ticket | |
| 3 | ticket_updated | updateTicket (#60) with field changes, updateTicketForm (#61) | Actor | ticket | INF-11 |
| 4 | ticket_opened | createTicket with `in_progress`; updateTicket draft/rejected -> in_progress | Actor | ticket | INF-11 |
| 5 | ticket_approved | approveTicket on an `approve` step | Actor (assignee or proxy) | ticket | One event per call even when the call completes several parallel steps |
| 6 | ticket_confirmed | approveTicket on a `confirm` step | Actor | ticket | If the call covers both approve and confirm steps of one group, `ticket_approved` is emitted once (approve wins) |
| 7 | ticket_rejected | rejectTicket | Actor | ticket | INF-12 |
| 8 | ticket_denied | denyTicket | Actor | ticket | |
| 9 | ticket_completed | approveTicket that completes the last group | Actor | ticket | After #5/#6 (INF-11) |
| 10 | ticket_withdrawn | withdrawTicket | Actor | ticket | |
| 11 | ticket_archived | archiveTicket | Actor | ticket | |
| 12 | comment_created | createComment; comments attached to approve/reject/deny (INF-22) | Comment author | comment + ticket | |
| 13 | comment_updated | updateComment, deleteComment | Actor | comment + ticket | Delete payload has `deletedAt` set (INF-30) |

The admin API can also enqueue any of the 13 event types for an existing ticket/comment on demand ("fire event", see `design-webhooks.md`), which uses the same payload builders.

### Enum contract (frozen in PF-02)

Added in session 462 to close carried finding CF-01 (`notes.md`). The values come from the cited OpenAPI schema property (`#/components/schemas/<Schema>/properties/<prop>`); where this table lists values, they were checked against the spec. "Excluded" values exist in the spec but punchflow never produces or accepts them. `internal/model/enums.go` exports one canonical `[]string` per row, named in the "Go list" column, plus one constant per value.

| Enum | Values | Source | Excluded (reason) | Go list |
|---|---|---|---|---|
| Ticket status | `draft`, `in_progress`, `completed`, `rejected`, `denied`, `archived` | `TicketSummary.status` | `permanently_deleted` (DIV-04) | `TicketStatuses` |
| Ticket step status | `pending`, `in_progress`, `completed`, `skipped` | `TicketStep.status` | `raised` (DIV-04) | `StepStatuses` |
| Action type | `approve`, `confirm`, `none` | `RouteStep.actionType`, `TicketStep.actionType` | - | `ActionTypes` |
| Step type | `author`, `manager`, `team`, `user`, `author_customizable`, `assignee_customizable`, `dynamic_team`, `dynamic_user` | `RouteStep.stepType` | - | `StepTypes` |
| Fallback | `direct_manager`, `higher_manager`, `skip`, `no_fallback`, `higher_team`, or `null` | `RouteStep.fallbackType`, `TicketStep.fallbackResult` | - | `FallbackTypes` |
| Manager selection | `closest`, `all` | `RouteStep.managerSelectionType` | - | `ManagerSelectionTypes` |
| Grade symbol | `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `any_of`, or `null` | `RouteStepTarget.gradeSymbol` | `""` is not a value; no grade filter is `null` | `GradeSymbols` |
| Step condition type | `always`, `conditional`, `conditional_skip` | `RouteStepCondition.conditionType` | - | `StepConditionTypes` |
| Step condition combination | `all`, `any` | `RouteStepCondition.combinationType` | - | `StepCombinationTypes` |
| Step condition field key | spec enum | `RouteStepConditionField.fieldKey` | `other_variable` (INF-18) | `StepConditionFieldKeys` |
| Route condition type | `always`, `field`, `field_otherwise` | `WorkflowRouteCondition.conditionType` | - | `RouteConditionTypes` |
| Route condition combination | `all`, `any` | `WorkflowRouteCondition.combinationType` | `custom` (INF-18) | `RouteCombinationTypes` |
| Section type | `form`, `slip` | `SectionListItem.sectionType` | `conditional` is not a type. A conditional section is a `form` section with `conditional: true`, `combinationType` (`all`/`any`; `custom` rejected, INF-18) and its own `conditionFields`. Slip sections carry `code` | `SectionTypes` |
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
4. Scenarios: one scenario per operation (96) running against a server built with `testutil.NewServer` and the default fixtures, each using a persona token with the needed permission. The response must be 200 and validate; empty-body operations must return zero bytes.
5. Timestamp check: every string value under a key ending in `At` must match `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+09:00$`.
6. Webhook payloads: one sample body of each of the 13 events (captured by the test receiver) is validated against `WebhookRequestBodyPing`, `WebhookRequestBodyTicket` or `WebhookRequestBodyComment`.

## References

- `design-docs/references/kickflow-api.md` sections 1-5
- `design-docs/references/kickflow-openapi.yaml`
- `design-docs/specs/notes.md` (INF/DIV register)
- `design-docs/specs/design-approval-engine.md`, `design-docs/specs/design-webhooks.md`, `design-docs/specs/design-auth-admin.md`
