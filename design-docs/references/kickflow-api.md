# kickflow REST API and Webhook Reference

Reference input for the punchflow dummy kickflow server (local/dev only). This document consolidates the public kickflow developer documentation, the official OpenAPI 3.1 definition, the developer changelog and the public help center into one English reference.

- Research date: 2026-10-20
- No kickflow account was used. Everything here comes from public material.
- Marking convention: every statement is **Confirmed** (directly stated in an official public source listed in [Sources](#8-sources)) unless it is tagged **(inferred)**. Inferred items are our best reading of the behavior and must be treated as design assumptions.
- The full machine-readable OpenAPI definition is stored next to this file as [`kickflow-openapi.yaml`](./kickflow-openapi.yaml) (downloaded from `https://developer.kickflow.com/rest/schema.yaml`, OpenAPI 3.1.0, `info.version` 1.0.0, license MIT per `info.license`). It is the single most authoritative input: sections 3 and 4 of this document are generated from it (field names, types, required flags, enums and constraints are verbatim; descriptions are translated from Japanese).

## Table of contents

1. [General API conventions](#1-general-api-conventions)
2. [Endpoint index](#2-endpoint-index)
3. [Endpoint details](#3-endpoint-details)
4. [Resource schemas](#4-resource-schemas)
5. [Webhooks](#5-webhooks)
6. [Product features (for the UI)](#6-product-features-for-the-ui)
7. [Confirmed vs inferred summary](#7-confirmed-vs-inferred-summary)
8. [Sources](#8-sources)

---

## 1. General API conventions

Source: https://developer.kickflow.com/rest/ , https://developer.kickflow.com/troubleshooting/ , https://developer.kickflow.com/changelog/ , `kickflow-openapi.yaml`.

### 1.1 Endpoint and transport

| Item | Value |
|---|---|
| Base URL | `https://api.kickflow.com/v1/` (OpenAPI `servers[0].url` is `https://api.kickflow.com`; every path is prefixed with `/v1`) |
| Versioning | Path prefix `/v1`. Only v1 exists. Breaking changes are announced in the changelog ("planned breaking changes" section) rather than via a new version. |
| Request content type | Only `application/json` is accepted. Exception: `POST /v1/files` uses `multipart/form-data` (field name `file`). A wrong content type returns `400 invalid_content_type`. |
| Content-Length | For HTTP/1.1 POST/PUT requests either `Content-Length` or `Transfer-Encoding: chunked` is mandatory, otherwise `411 Length Required`. Body-less POSTs (e.g. `POST /v1/tickets/{id}/approve` without comment) must send `Content-Length: 0`. Not required on HTTP/2. (Behavior since 2026-10-12.) |
| HTTP methods | GET, POST, PATCH, DELETE. PUT is not used by any documented endpoint. A wrong method on an existing path returns `404 endpoint_not_found`. |
| Success status | Every documented operation declares only `200` as its success response, including create (`POST`) and `DELETE` (DELETE returns 200 with an empty body). No 201/204 are used. |
| CORS | Supported; all APIs can be called from browser JavaScript. |
| Unknown fields | Responses may contain undocumented fields; clients must not rely on them. (For the dummy server: emitting only documented fields is acceptable.) |

### 1.2 Authentication

Two token kinds, both sent as `Authorization: Bearer <token>` (OpenAPI security schemes `PersonalAccessTokenAuth` and `ServiceAccountTokenAuth`, both `type: http, scheme: bearer`).

| Token | Header(s) | Semantics |
|---|---|---|
| Personal Access Token (PAT) | `Authorization: Bearer <token>` | Acts as the user who issued the token. Issued by each user from the access token settings screen. |
| Service Account Token (SAT) | `Authorization: Bearer <token>` and `X-Caller-Id: <user UUID>` | Tenant-wide token that impersonates any user given by `X-Caller-Id`. Missing caller id returns `401 invalid_caller_id`. kickflow recommends PAT unless impersonation is truly needed. |

- Authentication failure: `401`. Error codes: `invalid_access_token`, `user_not_registered`, `user_inactive`, `email_not_verified`, `invalid_caller_id`, `invitation_expired`.
- Authorization is RBAC: the token has exactly the admin permissions of the user (for SAT, of the impersonated user). Missing permission returns `403 missing_permission`. The permission identifiers are the admin role `permissionList[].permission` enum (see [RoleDetail](#schema-roledetail)): `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation`.
- Per-endpoint required permissions are written in each endpoint description in section 3 (e.g. "requires user management permission").
- Token format: not documented (inferred: opaque string; the dummy server may use any random string).

### 1.3 Pagination

- Query parameters `page` (1-based, default 1, min 1) and `perPage` (default 25, min 1, max 100).
- Response headers (RFC 8288 `Link` plus counters):

```
Link: <https://api.kickflow.com/v1/users?page=2&perPage=100>; rel="next",
  <https://api.kickflow.com/v1/users?page=50&perPage=100>; rel="last"
Page: 1
Per-Page: 100
Total: 4950
```

- The documentation example shows only `next` and `last`. Whether `prev` and `first` are also emitted is not documented (inferred: they are emitted when applicable, as with the common Rails `api-pagination` gem whose header names `Total` / `Per-Page` / `Page` this matches).
- Paginated list responses are plain JSON arrays (no envelope).
- Which endpoints paginate: every list endpoint whose parameter table in section 3 contains `page`/`perPage`.

### 1.4 Sorting and array query parameters

- `sortBy=<field>-asc` or `sortBy=<field>-desc`, for example `/v1/users?sortBy=email-asc`. The OpenAPI patterns make the direction suffix optional (e.g. `^(createdAt|updatedAt)(-asc|-desc)?$`); default direction when omitted is not documented (inferred: ascending).
- Unsupported sort fields are silently ignored (documented).
- Array query parameters use the `key[]=v1&key[]=v2` form, e.g. `/v1/tickets?status[]=in_progress&status[]=completed`. Comma-separated values or repeated keys without `[]` are wrong and may yield `400 invalid_parameter` with message `Invalid request: value at root is not an array`.
- Type mismatches yield messages of the form `expected XXX, but received YYY` (e.g. `expected array, but received String`).
- Query values must be URL-encoded (e.g. `email=foo%40example.com`, `createdAtStart=2023-10-01T00%3A00%3A00%2B09%3A00`).

### 1.5 Rate limiting

| Item | Value |
|---|---|
| Limit | 30 requests per minute per source IP address |
| Exceeded | `429 Too Many Requests`, error code `rate_limited` |
| Response headers | `RateLimit-Limit` (requests per window), `RateLimit-Remaining` (remaining in current window), `RateLimit-Reset` (UNIX time when the window resets) |
| Paid option | "REST API quota option": sending `X-Rate-Limit-Secret: <secret issued with the paid option>` raises the limit to 300 requests per minute |
| Stability | Thresholds may change without notice |

### 1.6 Date, time and ID formats

- All response timestamps are JST, RFC 3339 / ISO 8601 extended format with millisecond precision: `"createdAt": "2020-05-01T12:34:56.789+09:00"`.
- Accepted input formats: `2020-05-01T12:34:56.789+09:00`, `2020-05-01T12:34:56+09:00`, `2020-05-01T12:34:56.789Z`, `2020-05-01` (date only).
- Date-only fields (OpenAPI `format: date`) use `YYYY-MM-DD` (e.g. user `lastUsedOn`, general master item `startsOn`/`endsOn`).
- Form field value formats: `date` fields hold `YYYY-MM-DD` strings (inferred from the type name), `datetime` fields hold `YYYY-MM-DD HH:mm` strings (confirmed by changelog 2026).
- IDs: all resource IDs are UUID strings (`format: uuid`). The legacy `publicId` fields were deprecated and removed for all resources except the tenant (`WebhookTenant.publicId` is the tenant subdomain string).
- Codes: many resources also have a human-defined `code`. Path parameters for users, grades, teams, folders, routes, workflows, general masters and general master items accept either the UUID or the code (pattern `^[a-zA-Z0-9_-]+$` where specified).
- Workflow versioning: workflows are versioned internally; each time an admin edits a workflow the internal IDs of its form fields change. The `code` of a form field / slip field / slip section / route step is stable across versions, so integrations should key on `code` (official guide "ticketInput"). Ticket create/update accepts either `formFieldId` or `formFieldCode` (and `slipFieldId` / `slipFieldCode`, `slipSectionId` / `slipSectionCode`).

### 1.7 Error response format

Error bodies (except during maintenance or unexpected failures) look like:

```json
{
  "code": "validation_failed",
  "message": "hoge must not be empty",
  "errors": {
    "hoge": ["must not be empty"]
  }
}
```

| Field | Required | Meaning |
|---|---|---|
| `code` | yes | Error code (string) |
| `message` | yes | Human readable message |
| `errors` | no | Object mapping field name to array of messages (validation errors only). `errors.base` is used for non-field errors (e.g. comments disabled). |
| `doc_url` | no | URL of related documentation (some errors only). Note the snake_case name. |

Error codes (documented list; others may exist and codes/statuses can change without notice):

| HTTP | code | Meaning |
|---|---|---|
| 400 | `invalid_parameter` | Missing required parameter, wrong type, invalid field code, duplicate IDs, too long query (e.g. `authorTeamFullName`/`stepTitle` > 300 chars) |
| 400 | `invalid_content_type` | Content-Type is not allowed |
| 400 | `invalid_json` | JSON parse failure |
| 400 | `payload_too_large` | Payload or file size exceeds the limit |
| 400 | `invalid_header` | Invalid request header |
| 401 | `invalid_access_token` | Invalid access token |
| 401 | `user_not_registered` | User not invited |
| 401 | `user_inactive` | User not active |
| 401 | `email_not_verified` | User email not verified |
| 401 | `invalid_caller_id` | SAT used without `X-Caller-Id` |
| 401 | `invitation_expired` | Invitation expired |
| 403 | `missing_permission` | Missing admin permission |
| 403 | `license_purchase_required` | Additional licenses required |
| 403 | `upgrade_required` | Plan upgrade required |
| 403 | `option_purchase_required` | Paid option required |
| 403 | `invisible_ticket` | Ticket not visible to the caller |
| 403 | `user_not_assigned` | Caller is not an assigned approver of the ticket |
| 403 | `ticket_create_restricted` | Ticket creation is restricted |
| 403 | `ip_address_denied` | Blocked by IP address restriction |
| 403 | `admin_is_working` | Tenant is in "admin working" mode |
| 403 | `tenant_suspended` | Tenant suspended |
| 403 | `tenant_maintenance` | Tenant under maintenance (applies to all APIs, including login and guest APIs) |
| 403 | `resource_not_editable` | Non-editable resource (super admin role, root folder, etc.) |
| 404 | `resource_not_found` | Resource does not exist |
| 404 | `endpoint_not_found` | Endpoint does not exist (also wrong HTTP method) |
| 411 | (no JSON body documented) | Missing Content-Length / Transfer-Encoding on HTTP/1.1 POST/PUT |
| 422 | `validation_failed` | Business validation failed |
| 422 | `invalid_state` | Resource state does not allow the operation |
| 422 | `domain_not_allowed` | User email domain not allowed |
| 422 | `assignment_required_before_approve` | Next approver must be assigned before approving |
| 422 | `input_required_before_approve` | Form input required before approving |
| 429 | `rate_limited` | Rate limit exceeded |
| 500 | `internal_server_error` | Internal server error |
| 503 | `feature_disabled` | Feature temporarily unavailable |
| 504 | `gateway_timeout` | Request timed out (processing may have completed) |

OpenAPI declares on every operation the error responses 400, 401, 403, 404, 422, 500, 504 (`ErrorResponse` schema; 422 adds `errors` as `additionalProperties: string[]`).

### 1.8 Behavioral notes collected from the guides and changelog

These are confirmed behaviors that a faithful dummy server should reproduce.

- Ticket create (`POST /v1/tickets`): `status` may only be `draft` or `in_progress`. `inputs` must contain every form field of the workflow, including auto-calculation fields (send `value: null`; the server computes them). `inputs` may be omitted when the workflow has no form fields (slip sections only). `title` is accepted only when the workflow `titleInputMode` is `input`. `proxyClientUserId` is for proxy applications only; a suspended or deleted proxy client yields `422 validation_failed`. Non-existent or deleted user IDs in `viewers[].userId` / `watcherUserId` yield `422 validation_failed` (suspended users are allowed).
- Ticket update (`PATCH /v1/tickets/{ticketId}`): only fields present in the body are updated (`status`, `authorTeamId`, `inputs`, `slipItems` optional). Status may move to `draft`, `in_progress` or `rejected` per the enum, but a ticket in `rejected` cannot be moved back to `draft`. When an approver updates, the workflow `overwritable` setting governs which fields must be sent: "approver fields only (default)" means only fields whose editors include approvers; "all fields" means every field. Violations return `400 invalid_parameter` "invalid field code". `proxyClientUserId` accepts `null`.
- Partial form update (`PATCH /v1/tickets/{ticketId}/form`): only submitted form fields / slip items are overwritten; the rest is preserved.
- Approve / reject / deny (`POST .../approve|reject|deny`): caller must be a currently assigned approver, otherwise `403 user_not_assigned`. Optional `comment` (required when the workflow's `commentingRequiredOnApproval` / `commentingRequiredOnRejection` / `commentingRequiredOnDenial` is true; forbidden when `commentingEnabled` is false). `reject` requires `to` (step number to send back to; 0 = applicant, 1 = first approval step). `approve` also performs "confirm" on circulation steps.
- Comments: `POST /v1/tickets/{id}/comments` returns `422 validation_failed` with `errors.base` when the workflow `commentingEnabled` is false. Mentions are written in the body as `<@USER-UUID>` and rendered as `@Full Name`; mentioned users appear in `mentionedUsers` and receive notifications. Deleting a comment is a soft delete (`deletedAt`); the webhook `comment_updated` fires for both edit and delete.
- Files: upload is Enterprise-plan only. Max 50 MB (was 2 MB). Missing/invalid `file` returns `400 invalid_parameter`. Response `{ "signedId": "..." }`. Use the signed ID in ticket `inputs[].files` (or slip inputs / comment attachments, inferred for comments). Unreferenced uploads are deleted after at least 24 hours. `GET /v1/files/{signedId}` returns metadata plus a pre-signed S3 URL valid for 5 minutes and only works for files already attached to a ticket.
- Proxy approver update: re-assignment of approval rights from old to new proxy user is asynchronous. Duplicate `workflowIds` (including case-only differences) are de-duplicated.
- Admin roles: users with role-management permission (other than super admins) cannot update/delete roles they belong to, nor add/remove themselves. Duplicate IDs in `folderIds` / `teamIds` / `generalMasterIds` give `400 invalid_parameter` (exact duplicates) or `422 validation_failed` (case-only duplicates).
- Organization chart deletion is processed asynchronously.
- User `customFields` (Enterprise plan only): fields not flagged "visible to everyone" are returned only to callers with user-management permission.
- Ticket viewers list default order: team name asc, then user email asc; `sortBy=createdAt` supported.
- General master item list default order follows the master's `defaultSortBy` (`name` or `code`).
- User `lastUsedOn`: last date the user operated kickflow (UI, API or chat), `null` if never.
- Planned breaking change (date TBD): `user` may become `null` in comment list/get responses and in the payloads of `ticket_approved`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `comment_created` webhooks (for `comment_created`: top-level `user` and `data.comment.user`).

### 1.9 Ticket lifecycle (state machine)

Ticket `status` enum: `draft`, `in_progress`, `completed`, `rejected`, `archived`, `denied`, `permanently_deleted`.

| From | Action (API) | To | Webhook |
|---|---|---|---|
| (none) | create with `status: draft` | `draft` | `ticket_created` |
| (none) | create with `status: in_progress` | `in_progress` | `ticket_created` then `ticket_opened` (inferred that both fire) |
| `draft` | update with `status: in_progress` | `in_progress` | `ticket_opened` (and possibly `ticket_updated`, inferred) |
| `draft` / `in_progress` / `rejected` | update fields | unchanged | `ticket_updated` |
| `in_progress` | approve on approval step (not last) | `in_progress` (currentStep advances) | `ticket_approved` |
| `in_progress` | approve on circulation step (`actionType: confirm`) | `in_progress` or `completed` | `ticket_confirmed` |
| `in_progress` | approve on last step | `completed` | `ticket_approved` / `ticket_confirmed` then `ticket_completed` (inferred ordering) |
| `in_progress` | reject with `to: 0` | `rejected` (sent back to applicant) | `ticket_rejected` |
| `in_progress` | reject with `to: N>0` | `in_progress` (back to step N) (inferred) | `ticket_rejected` |
| `rejected` | update with `status: in_progress` (re-submit) | `in_progress` | `ticket_opened` (inferred) |
| `in_progress` | deny | `denied` (terminal) | `ticket_denied` |
| `in_progress` | withdraw (applicant only, own ticket in progress) | `rejected` (Confirmed by help center: a withdrawn ticket becomes "sent back" and is editable again) | `ticket_withdrawn` |
| `draft` / `rejected` | archive (applicant, or admin with `ticket_write`) | `archived` (irreversible; still viewable, not editable or re-submittable) | `ticket_archived` |
| `in_progress` / `completed` / `denied` | archive (admin with `ticket_write` only) | `archived` | `ticket_archived` |
| `archived` | archive again | not allowed (inferred error: `422 invalid_state`) | none |
| `archived` | (purge, UI only) | `permanently_deleted` (inferred meaning) | none documented |

`currentStep`: 0 = applicant, 1 = first approval step. Ticket step `status`: `pending`, `in_progress`, `completed`, `skipped`, `raised` (escalated). Assignee status for `listTasks` / `pending` flags: see [TicketAssignee](#schema-ticketassignee).

### 1.10 Key enumerations (quick reference)

| Enum | Values | Where |
|---|---|---|
| Ticket status | `draft`, `in_progress`, `completed`, `rejected`, `archived`, `denied`, `permanently_deleted` | [TicketSummary](#schema-ticketsummary).status |
| Ticket list `status[]` filter | `draft`, `in_progress`, `rejected`, `completed`, `denied`, `archived` | `GET /v1/tickets` |
| Task list `status[]` filter | `draft`, `in_progress`, `rejected`, `completed`, `deleted` | `GET /v1/tickets/tasks` |
| Ticket public override | `follow_workflow`, `forced_public`, `forced_private` | TicketSummary.forcedPublicType |
| Ticket step status | `pending`, `in_progress`, `completed`, `skipped`, `raised` | [TicketStep](#schema-ticketstep).status |
| Step action type | `approve` (approve / send back), `confirm` (circulation with confirmation), `none` (circulation without confirmation) | RouteStep / TicketStep.actionType |
| Route step (approver) type | `author`, `manager`, `team`, `user`, `author_customizable`, `assignee_customizable`, `dynamic_team`, `dynamic_user` | [RouteStep](#schema-routestep).stepType |
| Approver fallback | `direct_manager`, `higher_manager`, `skip`, `no_fallback`, `higher_team` | RouteStep.fallbackType / TicketStep.fallbackResult |
| Manager selection | `closest`, `all` | RouteStep.managerSelectionType |
| Grade comparison | `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `any_of` | RouteStepTarget.gradeSymbol, WorkflowAvailableTeam.gradeSymbol |
| Route step execution | `always`, `conditional`, `conditional_skip` | RouteStepCondition.conditionType |
| Route step condition variable | `author_grade`, `author_team`, `text_variable`, `number_variable`, `checkbox_variable`, `general_master_variable`, `other_variable` | RouteStepConditionField.fieldKey |
| Condition operators | `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal`, `not_descendants_or_equal` (last one only in route step conditions) | *ConditionField.symbol |
| Condition combination | `all`, `any`, `custom` (advanced expression in `combinationExpression`) | SectionListItem / WorkflowRouteCondition / validations |
| Workflow route branch type | `always`, `field`, `field_otherwise` | [WorkflowRouteCondition](#schema-workflowroutecondition).conditionType |
| Form field type | `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date`, `file`, `master`, `user`, `team`, `ticket`, `calculation`, `button_api`, `button_kintone`, `datetime`, `radio_button` | [FormField](#schema-formfield).fieldType |
| Slip (line item) field type | `text`, `number`, `integer`, `calculation`, `pull_down`, `checkbox`, `date`, `file`, `master`, `user`, `team`, `ticket`, `datetime`, `radio_button` | [SlipField](#schema-slipfield).fieldType |
| Section type | `form`, `slip` | [SectionListItem](#schema-sectionlistitem).sectionType |
| Form field size | `full`, `half` | FormField.size |
| Option orientation | `vertical`, `horizontal` | FormField.orientation |
| Approver edit permission scope | `all`, `user`, `team_grade` | *ApproverEditSetting.permissionType |
| General master field type | `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date` | [GeneralMasterField](#schema-generalmasterfield).fieldType |
| General master search filter | `free_word`, `name`, `code`, `description`, `custom_field` | GeneralMasterSearchFilter.fieldType |
| General master default sort | `name`, `code` | GeneralMaster.defaultSortBy |
| Workflow status | `visible`, `invisible`, `deleted` | [Workflow](#schema-workflow).status |
| Route status | `visible`, `deleted`, `error` | [Route](#schema-route).status |
| Workflow title mode | `none`, `input`, `calculate` | Workflow.titleInputMode |
| Applicant manager as viewer | `none`, `direct`, `all` | Workflow.visibleToManager |
| Report formats | `pdf`, `excel` | Workflow.reportFormats |
| Watcher type | `user`, `team` | WorkflowWatcher.watcherType |
| Numbering timing | `created`, `opened`, `completed`, `sub_status_attached` | TicketNumberingTiming / CustomNumberingSetting.timingType |
| Ticket copy mapping | `ticket_title`, `ticket_number`, `form_field` | FormFieldTicketCopySetting.mappings.fieldType |
| External API HTTP method | `get`, `post`, `put`, `delete`, `patch` | ExternalApiSetting.httpMethod |
| CloudSign document status | `draft`, `in_progress`, `rejected`, `template`, `imported`, `completed` | TicketDetail.cloudSignDocument.status |
| User status | `invited`, `activated`, `suspended`, `deactivated` | [User](#schema-user).status |
| User type | `normal`, `assistant` | User.userType |
| User custom field type | `text`, `textLong`, `number`, `integer`, `checkbox`, `pullDown`, `date` (note camelCase here) | User.customFields.fieldType |

| Admin role permission | `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` | RoleDetail.permissionList.permission |
| Audit log system type | `automation`, `null` | [AuditLog](#schema-auditlog).systemType |
| Audit log `action` | free string; values not documented (inferred: dotted or snake_case action names) | AuditLog.action |

---

### General master limits

Kickflow's help center specifies up to 30 general masters and 10 custom fields per master for standard plans, or 100 masters and 30 fields per master for enterprise. Each master supports up to 100,000 items. General-master short-text custom fields accept 300 characters; long-text fields accept 10,000 characters. These limits are recorded as inference `INF-V9-API-02` because the public REST schema does not encode tenant-plan quotas.

## 2. Endpoint index

96 operations across 57 paths, all from the OpenAPI definition (Confirmed). Grouping follows the OpenAPI tags. There are **no** public REST endpoints for webhooks, access tokens, labels, sub-statuses (read via tickets only), SCIM, tenant settings or workflow/route editing; those are managed only in the web UI (see sections 5 and 6).

| Resource | Method | Path | operationId | Purpose | Success response |
|---|---|---|---|---|---|
| Users | GET | `/v1/user` | getCurrentUser | Get the current user | 200 [UserDetail](#schema-userdetail) |
| Users | GET | `/v1/users` | listUsers | Get the list of users | 200 [User](#schema-user)[] |
| Users | POST | `/v1/users` | createUser | Create (invite) a user | 200 [UserDetail](#schema-userdetail) |
| Users | GET | `/v1/users/{userId}` | getUser | Get a user | 200 [UserDetail](#schema-userdetail) |
| Users | DELETE | `/v1/users/{userId}` | deleteUser | Delete a user | 200 [UserDetail](#schema-userdetail) |
| Users | PATCH | `/v1/users/{userId}` | updateUser | Update a user | 200 [UserDetail](#schema-userdetail) |
| Users | GET | `/v1/users/lookupByEmail` | lookupUserByEmail | Get a user by email address | 200 [UserDetail](#schema-userdetail) |
| Users | POST | `/v1/users/{userId}/reinvite` | reinviteUser | Re-invite a user | 200 [UserDetail](#schema-userdetail) |
| Users | POST | `/v1/users/{userId}/suspend` | suspendUser | Suspend a user | 200 [UserDetail](#schema-userdetail) |
| Users | POST | `/v1/users/{userId}/reactivate` | reactivateUser | Reactivate a user | 200 [UserDetail](#schema-userdetail) |
| Users | GET | `/v1/users/{userId}/teams` | listUserTeams | Get the list of teams the user belongs to | 200 [Team](#schema-team)[] |
| Users | GET | `/v1/users/{userId}/roles` | listUserRoles | Get the list of admin roles of the user | 200 [Role](#schema-role)[] |
| Grades (job titles) | GET | `/v1/grades` | listGrades | Get grade list | 200 [Grade](#schema-grade)[] |
| Grades (job titles) | POST | `/v1/grades` | createGrade | Create grade | 200 [Grade](#schema-grade) |
| Grades (job titles) | GET | `/v1/grades/{gradeId}` | getGrade | Get grade | 200 [Grade](#schema-grade) |
| Grades (job titles) | DELETE | `/v1/grades/{gradeId}` | deleteGrade | Delete grade | 200 (empty body) |
| Grades (job titles) | PATCH | `/v1/grades/{gradeId}` | updateGrade | Update grade | 200 [Grade](#schema-grade) |
| Grades (job titles) | POST | `/v1/grades/{gradeId}/default` | setDefaultGrade | Change default grade | 200 (empty body) |
| Proxy applicants | GET | `/v1/proxyApplicants` | listProxyApplicants | Get the list of proxy applicants | 200 [ProxyApplicant](#schema-proxyapplicant)[] |
| Proxy applicants | POST | `/v1/proxyApplicants` | createProxyApplicant | Create a proxy applicant | 200 [ProxyApplicant](#schema-proxyapplicant) |
| Proxy applicants | PATCH | `/v1/proxyApplicants/{proxyApplicantId}` | updateProxyApplicant | Update a proxy applicant | 200 [ProxyApplicant](#schema-proxyapplicant) |
| Proxy applicants | DELETE | `/v1/proxyApplicants/{proxyApplicantId}` | deleteProxyApplicant | Delete a proxy applicant | 200 (empty body) |
| Proxy approvers | GET | `/v1/proxyApprovers` | listProxyApprovers | Get the list of proxy approvers | 200 [ProxyApprover](#schema-proxyapprover)[] |
| Proxy approvers | POST | `/v1/proxyApprovers` | createProxyApprover | Create a proxy approver | 200 [ProxyApprover](#schema-proxyapprover) |
| Proxy approvers | PATCH | `/v1/proxyApprovers/{proxyApproverId}` | updateProxyApprover | Update a proxy approver | 200 [ProxyApprover](#schema-proxyapprover) |
| Proxy approvers | DELETE | `/v1/proxyApprovers/{proxyApproverId}` | deleteProxyApprover | Delete a proxy approver | 200 (empty body) |
| Organization charts | GET | `/v1/organizationCharts` | listOrganizationCharts | Get organization chart list | 200 [OrganizationChart](#schema-organizationchart)[] |
| Organization charts | POST | `/v1/organizationCharts` | createOrganizationChart | Create organization chart | 200 [OrganizationChartDetail](#schema-organizationchartdetail) |
| Organization charts | DELETE | `/v1/organizationCharts/{organizationChartId}` | deleteOrganizationChart | Delete organization chart | 200 (empty body) |
| Organization charts | GET | `/v1/organizationCharts/{organizationChartId}` | getOrganizationChart | Get organization chart | 200 [OrganizationChartDetail](#schema-organizationchartdetail) |
| Organization charts | PATCH | `/v1/organizationCharts/{organizationChartId}` | updateOrganizationChart | Update organization chart | 200 [OrganizationChartDetail](#schema-organizationchartdetail) |
| Organization charts | GET | `/v1/organizationChart` | getCurrentOrganizationChart | Get current organization chart | 200 [OrganizationChartDetail](#schema-organizationchartdetail) |
| Organization charts | POST | `/v1/organizationCharts/{organizationChartId}/activate` | activateOrganizationChart | Activate organization chart | 200 [OrganizationChartDetail](#schema-organizationchartdetail) |
| Teams | GET | `/v1/organizationCharts/{organizationChartId}/teams` | listTeams | Get team list | 200 [Team](#schema-team)[] |
| Teams | POST | `/v1/organizationCharts/{organizationChartId}/teams` | createTeam | Create team | 200 [TeamDetail](#schema-teamdetail) |
| Teams | GET | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}` | getTeam | Get team | 200 [TeamDetail](#schema-teamdetail) |
| Teams | PATCH | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}` | updateTeam | Edit team | 200 [TeamDetail](#schema-teamdetail) |
| Teams | DELETE | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}` | deleteTeam | Delete team | 200 (empty body) |
| Teams | GET | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships` | listTeamMembers | Get team member list | 200 [MemberUser](#schema-memberuser)[] |
| Teams | POST | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships` | createTeamMembers | Add members to team | 200 (empty body) |
| Teams | DELETE | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships` | deleteTeamMembers | Remove members from team | 200 (empty body) |
| Teams | PATCH | `/v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId}` | updateTeamMember | Update team member | 200 (empty body) |
| Categories | GET | `/v1/categories` | listCategories | Get category list | 200 [Category](#schema-category)[] |
| Categories | POST | `/v1/categories` | createCategory | Create category | 200 [Category](#schema-category) |
| Categories | DELETE | `/v1/categories/{categoryId}` | deleteCategory | Delete category | 200 (empty body) |
| Categories | PATCH | `/v1/categories/{categoryId}` | updateCategory | Update category | 200 [Category](#schema-category) |
| Folders | GET | `/v1/folders` | listFolders | Get folder list | 200 [Folder](#schema-folder)[] |
| Folders | POST | `/v1/folders` | createFolder | Create folder | 200 [FolderDetail](#schema-folderdetail) |
| Folders | DELETE | `/v1/folders/{folderId}` | deleteFolder | Delete folder | 200 (empty body) |
| Folders | PATCH | `/v1/folders/{folderId}` | updateFolder | Update folder | 200 [FolderDetail](#schema-folderdetail) |
| Folders | GET | `/v1/folders/{folderId}` | getFolder | Get folder | 200 [FolderDetail](#schema-folderdetail) |
| Routes | GET | `/v1/routes` | listRoutes | Get the list of routes | 200 [Route](#schema-route)[] |
| Routes | GET | `/v1/routes/{routeId}` | getRoute | Get a route | 200 [RouteDetail](#schema-routedetail) |
| Workflows | GET | `/v1/workflows` | listWorkflows | Get the list of workflows | 200 [Workflow](#schema-workflow)[] |
| Workflows | GET | `/v1/workflows/{workflowId}` | getWorkflow | Get a workflow | 200 [WorkflowDetail](#schema-workflowdetail) |
| Tickets | GET | `/v1/tickets` | listTickets | Get ticket list | 200 [TicketWithStep](#schema-ticketwithstep)[] |
| Tickets | POST | `/v1/tickets` | createTicket | Create ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | GET | `/v1/tickets/tasks` | listTasks | Get approval request list | 200 [TicketWithStep](#schema-ticketwithstep)[] |
| Tickets | GET | `/v1/tickets/{ticketId}` | getTicket | Get a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | PATCH | `/v1/tickets/{ticketId}` | updateTicket | Update a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | PATCH | `/v1/tickets/{ticketId}/form` | updateTicketForm | Partially update ticket form data | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | POST | `/v1/tickets/{ticketId}/approve` | approveTicket | Approve or confirm a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | POST | `/v1/tickets/{ticketId}/reject` | rejectTicket | Reject (send back) a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | POST | `/v1/tickets/{ticketId}/deny` | denyTicket | Deny a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | POST | `/v1/tickets/{ticketId}/withdraw` | withdrawTicket | Withdraw a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | POST | `/v1/tickets/{ticketId}/archive` | archiveTicket | Archive a ticket | 200 [TicketDetail](#schema-ticketdetail) |
| Tickets | GET | `/v1/tickets/{ticketId}/links` | listTicketLinks | Get related tickets of a ticket | 200 [Ticket](#schema-ticket)[] |
| Tickets | GET | `/v1/tickets/{ticketId}/viewers` | listViewers | Get the list of viewers | 200 [TicketViewer](#schema-ticketviewer)[] |
| Tickets | POST | `/v1/tickets/{ticketId}/viewers` | createViewer | Add a viewer | 200 (empty body) |
| Tickets | DELETE | `/v1/tickets/{ticketId}/viewers/{viewerId}` | deleteViewer | Remove a viewer | 200 (empty body) |
| Files | POST | `/v1/files` | uploadFile | Upload an attachment | 200 object |
| Files | GET | `/v1/files/{signedId}` | getFile | Get attachment information | 200 object |
| General masters | GET | `/v1/generalMasters` | listGeneralMasters | Get general master list | 200 [GeneralMaster](#schema-generalmaster)[] |
| General masters | POST | `/v1/generalMasters` | createGeneralMaster | Create general master | 200 [GeneralMaster](#schema-generalmaster) |
| General masters | GET | `/v1/generalMasters/{generalMasterId}` | getGeneralMaster | Get general master | 200 [GeneralMaster](#schema-generalmaster) |
| General masters | PATCH | `/v1/generalMasters/{generalMasterId}` | updateGeneralMaster | Update general master | 200 [GeneralMaster](#schema-generalmaster) |
| General masters | DELETE | `/v1/generalMasters/{generalMasterId}` | deleteGeneralMaster | Delete general master | 200 (empty body) |
| General masters | GET | `/v1/generalMasters/{generalMasterId}/items` | listGeneralMasterItems | Get general master item list | 200 [GeneralMasterItem](#schema-generalmasteritem)[] |
| General masters | POST | `/v1/generalMasters/{generalMasterId}/items` | createGeneralMasterItem | Create general master item | 200 [GeneralMasterItem](#schema-generalmasteritem) |
| General masters | GET | `/v1/generalMasters/{generalMasterId}/items/{itemId}` | getGeneralMasterItem | Get general master item | 200 [GeneralMasterItem](#schema-generalmasteritem) |
| General masters | PATCH | `/v1/generalMasters/{generalMasterId}/items/{itemId}` | updateGeneralMasterItem | Update general master item | 200 [GeneralMasterItem](#schema-generalmasteritem) |
| General masters | DELETE | `/v1/generalMasters/{generalMasterId}/items/{itemId}` | deleteGeneralMasterItem | Delete general master item | 200 (empty body) |
| Admin roles | GET | `/v1/roles` | listRoles | Get admin role list | 200 [Role](#schema-role)[] |
| Admin roles | POST | `/v1/roles` | createRole | Create admin role | 200 [RoleDetail](#schema-roledetail) |
| Admin roles | GET | `/v1/roles/{roleId}` | getRole | Get admin role | 200 [RoleDetail](#schema-roledetail) |
| Admin roles | PATCH | `/v1/roles/{roleId}` | updateRole | Update admin role | 200 [RoleDetail](#schema-roledetail) |
| Admin roles | DELETE | `/v1/roles/{roleId}` | deleteRole | Delete admin role | 200 (empty body) |
| Admin roles | POST | `/v1/roles/{roleId}/memberships` | createRoleMembers | Add members to admin role | 200 (empty body) |
| Admin roles | GET | `/v1/roles/{roleId}/memberships` | listRoleMembers | Get admin role member list | 200 [User](#schema-user)[] |
| Admin roles | DELETE | `/v1/roles/{roleId}/memberships/{userId}` | deleteRoleMember | Remove members from admin role | 200 (empty body) |
| Comments | GET | `/v1/tickets/{ticketId}/comments` | listComments | Get the list of comments | 200 [Comment](#schema-comment)[] |
| Comments | POST | `/v1/tickets/{ticketId}/comments` | createComment | Post a comment | 200 [Comment](#schema-comment) |
| Comments | GET | `/v1/tickets/{ticketId}/comments/{commentId}` | getComment | Get a comment | 200 [Comment](#schema-comment) |
| Comments | PATCH | `/v1/tickets/{ticketId}/comments/{commentId}` | updateComment | Update a comment | 200 [Comment](#schema-comment) |
| Comments | DELETE | `/v1/tickets/{ticketId}/comments/{commentId}` | deleteComment | Delete a comment | 200 (empty body) |
| Audit logs | GET | `/v1/auditLogs` | listAuditLogs | Get the list of audit logs | 200 [AuditLog](#schema-auditlog)[] |

---

## 3. Endpoint details

Generated from `kickflow-openapi.yaml` (Confirmed). For each operation: parameters (path, query, header), request body fields flattened with dotted paths (`a.b`, `a[].b`), and the 200 response schema. Type notation: `string(uuid)`, `string|null` (nullable), `X[]` (array), `A or B` (oneOf/anyOf), links point to section 4. Every operation also declares the error responses listed per endpoint, all with the [ErrorResponse](#schema-errorresponse) body. Descriptions are English translations of the official Japanese text, including the required admin permission where kickflow documents it.

### Users

#### `GET /v1/user`

- operationId: `getCurrentUser`
- Summary: Get the current user
- Description: Retrieves the current user.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `GET /v1/users`

- operationId: `listUsers`
- Summary: Get the list of users
- Description: Retrieves the list of users.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `status` | query | string[] |  | enum: `invited`, `activated`, `suspended`, `deactivated` | Filter by status |
| `sortBy` | query | string |  | pattern: `^(email\|code\|lastUsedOn)(-asc\|-desc)?$` | Field and order to sort by. Available fields: email, code, lastUsedOn |

Response 200: Schema: array of [User](#schema-user)
Response headers: Link

#### `POST /v1/users`

- operationId: `createUser`
- Summary: Create (invite) a user
- Description: Creates a user.<br>The created user will have the invited status, and an invitation email will be sent.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `email` | string(email) | yes |  | Email address |
| `code` | string | yes | maxLength: 100 | Code. If not specified, a random alphanumeric string is set automatically. |
| `firstName` | string | yes | maxLength: 255 | First name |
| `lastName` | string | yes | maxLength: 255 | Last name |
| `sendEmail` | boolean |  |  | true to send an invitation email (default), false to not send it |
| `employeeId` | string\|null |  | maxLength: 30 | Employee number |
| `memberships` | [InviteMembershipBody](#schema-invitemembershipbody)[] |  | maxItems: 5 | Array of affiliations to create upon invitation (up to 5) |
| `customFields` | object[] |  |  | List of user custom field values. Each element is { code, value }.<br>code specifies UserCustomField#code.<br>value is a value according to fieldType (string / number / string[] / null).<br>Specifying null for value clears the value of that field.<br>This parameter is valid only for enterprise/trial contract tenants.<br> |
| `customFields[].code` | string | yes |  | UserCustomField#code |
| `customFields[].value` | string or number or string[] or null | yes |  | Value according to fieldType. For number / integer, send the value as a string<br>(a JSON number is also accepted for compatibility, but it is normalized to a string when saved).<br>For number / integer, thousands-separator commas cannot be used; only plain numeric strings are accepted (422 if specified).<br>Specify null to clear.<br> |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `GET /v1/users/{userId}`

- operationId: `getUser`
- Summary: Get a user
- Description: Retrieves a single user.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `DELETE /v1/users/{userId}`

- operationId: `deleteUser`
- Summary: Delete a user
- Description: Deletes a user (soft delete).<br>The deleted user gets the deleted status, but the user information remains accessible.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `PATCH /v1/users/{userId}`

- operationId: `updateUser`
- Summary: Update a user
- Description: Updates a user.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `email` | string(email) |  | maxLength: 254 | Email address |
| `code` | string |  | maxLength: 100 | Code |
| `firstName` | string |  | maxLength: 255 | First name |
| `lastName` | string |  | maxLength: 255 | Last name |
| `employeeId` | string\|null |  | maxLength: 30 | Employee number |
| `customFields` | object[] |  |  | List of user custom field values. Each element is { code, value }.<br>code specifies UserCustomField#code.<br>value is a value according to fieldType (string / number / string[] / null).<br>Specifying null for value clears the value of that field.<br>This parameter is valid only for enterprise/trial contract tenants.<br> |
| `customFields[].code` | string | yes |  | UserCustomField#code |
| `customFields[].value` | string or number or string[] or null | yes |  | Value according to fieldType. For number / integer, send the value as a string<br>(a JSON number is also accepted for compatibility, but it is normalized to a string when saved).<br>For number / integer, thousands-separator commas cannot be used; only plain numeric strings are accepted (422 if specified).<br>Specify null to clear.<br> |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `GET /v1/users/lookupByEmail`

- operationId: `lookupUserByEmail`
- Summary: Get a user by email address
- Description: Retrieves a user by email address (exact match).<br>Note: Send the email address URL-encoded.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `email` | query | string | yes |  | Email address (URL-encoded) |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `POST /v1/users/{userId}/reinvite`

- operationId: `reinviteUser`
- Summary: Re-invite a user
- Description: Re-invites a deleted user.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `POST /v1/users/{userId}/suspend`

- operationId: `suspendUser`
- Summary: Suspend a user
- Description: Suspends an active user.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `POST /v1/users/{userId}/reactivate`

- operationId: `reactivateUser`
- Summary: Reactivate a user
- Description: Activates a suspended user.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: [UserDetail](#schema-userdetail)

#### `GET /v1/users/{userId}/teams`

- operationId: `listUserTeams`
- Summary: Get the list of teams the user belongs to
- Description: Retrieves the list of teams the user belongs to.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `submittable` | query | boolean |  |  | If true, returns only teams that can be applied to. Defaults to false (returns all teams) |
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: array of [Team](#schema-team)

#### `GET /v1/users/{userId}/roles`

- operationId: `listUserRoles`
- Summary: Get the list of admin roles of the user
- Description: Retrieves the list of admin roles of the user.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `userId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | User UUID or code |

Response 200: Schema: array of [Role](#schema-role)

### Grades (job titles)

#### `GET /v1/grades`

- operationId: `listGrades`
- Summary: Get grade list
- Description: Retrieves a list of grades.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(level\|code)(-asc\|-desc)?$` | Field and order to sort by. Available fields: level, code |

Response 200: Schema: array of [Grade](#schema-grade)

#### `POST /v1/grades`

- operationId: `createGrade`
- Summary: Create grade
- Description: Creates a grade.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: [GradeCreateBody](#schema-gradecreatebody)

Response 200: Schema: [Grade](#schema-grade)

#### `GET /v1/grades/{gradeId}`

- operationId: `getGrade`
- Summary: Get grade
- Description: Retrieves a single grade.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `gradeId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Grade UUID or code |

Response 200: Schema: [Grade](#schema-grade)

#### `DELETE /v1/grades/{gradeId}`

- operationId: `deleteGrade`
- Summary: Delete grade
- Description: Deletes a grade.<br>Executing this API requires user management permission.<br>Note: If any users are using this grade, an error occurs. Remove the grade from those users first.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `gradeId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Grade UUID or code |

Response 200: empty body

#### `PATCH /v1/grades/{gradeId}`

- operationId: `updateGrade`
- Summary: Update grade
- Description: Updates a grade.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `gradeId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Grade UUID or code |

Request body (`application/json`): Schema: [GradeUpdateBody](#schema-gradeupdatebody)

Response 200: Schema: [Grade](#schema-grade)

#### `POST /v1/grades/{gradeId}/default`

- operationId: `setDefaultGrade`
- Summary: Change default grade
- Description: Sets the specified grade as the default.<br>At the same time, the grade that was previously the default is automatically no longer the default.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `gradeId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Grade UUID or code |

Response 200: empty body

### Proxy applicants

#### `GET /v1/proxyApplicants`

- operationId: `listProxyApplicants`
- Summary: Get the list of proxy applicants
- Description: Retrieves the list of proxy applicants in the tenant.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |

Response 200: Schema: array of [ProxyApplicant](#schema-proxyapplicant)

#### `POST /v1/proxyApplicants`

- operationId: `createProxyApplicant`
- Summary: Create a proxy applicant
- Description: Creates a new proxy applicant.<br>Executing this API requires user management permission. However, if setting one's own proxy applicants is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userId` | string(uuid) | yes |  | ID of the user being represented |
| `proxyUserId` | string(uuid) | yes |  | ID of the user acting as proxy |
| `startsOn` | string(date)\|null |  |  | Start date. If null, it is treated as already started. |
| `endsOn` | string(date)\|null |  |  | End date. If null, it is treated as indefinite. |
| `workflowIds` | string(uuid)[] |  |  | IDs of target workflows |

Response 200: Schema: [ProxyApplicant](#schema-proxyapplicant)

#### `PATCH /v1/proxyApplicants/{proxyApplicantId}`

- operationId: `updateProxyApplicant`
- Summary: Update a proxy applicant
- Description: Updates the specified proxy applicant. Only the specified fields are updated.<br>Executing this API requires user management permission. However, if setting one's own proxy applicants is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `proxyApplicantId` | path | string(uuid) | yes |  | Proxy applicant UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userId` | string(uuid) |  |  | ID of the user being represented |
| `proxyUserId` | string(uuid) |  |  | ID of the user acting as proxy |
| `startsOn` | string(date)\|null |  |  | Start date. If null, it is treated as already started. |
| `endsOn` | string(date)\|null |  |  | End date. If null, it is treated as indefinite. |
| `workflowIds` | string(uuid)[] |  |  | IDs of target workflows. Specifying an empty array removes the restriction on target workflows, and all workflows become subject to the proxy applicant. |

Response 200: Schema: [ProxyApplicant](#schema-proxyapplicant)

#### `DELETE /v1/proxyApplicants/{proxyApplicantId}`

- operationId: `deleteProxyApplicant`
- Summary: Delete a proxy applicant
- Description: Deletes the specified proxy applicant.<br>Executing this API requires user management permission. However, if setting one's own proxy applicants is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `proxyApplicantId` | path | string(uuid) | yes |  | Proxy applicant UUID |

Response 200: empty body

### Proxy approvers

#### `GET /v1/proxyApprovers`

- operationId: `listProxyApprovers`
- Summary: Get the list of proxy approvers
- Description: Retrieves the list of proxy approvers in the tenant.<br>Executing this API requires user management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |

Response 200: Schema: array of [ProxyApprover](#schema-proxyapprover)

#### `POST /v1/proxyApprovers`

- operationId: `createProxyApprover`
- Summary: Create a proxy approver
- Description: Creates a new proxy approver.<br>Executing this API requires user management permission. However, if setting one's own proxy approvers is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userId` | string(uuid) | yes |  | ID of the user being represented |
| `proxyUserId` | string(uuid) | yes |  | ID of the user acting as proxy |
| `startsOn` | string(date)\|null |  |  | Start date. If null, it is treated as already started. |
| `endsOn` | string(date)\|null |  |  | End date. If null, it is treated as indefinite. |
| `workflowIds` | string(uuid)[] |  |  | IDs of target workflows |

Response 200: Schema: [ProxyApprover](#schema-proxyapprover)

#### `PATCH /v1/proxyApprovers/{proxyApproverId}`

- operationId: `updateProxyApprover`
- Summary: Update a proxy approver
- Description: Updates the specified proxy approver. Only the specified fields are updated.<br>Executing this API requires user management permission. However, if setting one's own proxy approvers is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `proxyApproverId` | path | string(uuid) | yes |  | Proxy approver UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userId` | string(uuid) |  |  | ID of the user being represented |
| `proxyUserId` | string(uuid) |  |  | ID of the user acting as proxy |
| `startsOn` | string(date)\|null |  |  | Start date. If null, it is treated as already started. |
| `endsOn` | string(date)\|null |  |  | End date. If null, it is treated as indefinite. |
| `workflowIds` | string(uuid)[] |  |  | IDs of target workflows. Specifying an empty array removes the restriction on target workflows, and all workflows become subject to the proxy approver. |

Response 200: Schema: [ProxyApprover](#schema-proxyapprover)

#### `DELETE /v1/proxyApprovers/{proxyApproverId}`

- operationId: `deleteProxyApprover`
- Summary: Delete a proxy approver
- Description: Deletes the specified proxy approver.<br>Executing this API requires user management permission. However, if setting one's own proxy approvers is permitted for all users, management permission is not required.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `proxyApproverId` | path | string(uuid) | yes |  | Proxy approver UUID |

Response 200: empty body

### Organization charts

#### `GET /v1/organizationCharts`

- operationId: `listOrganizationCharts`
- Summary: Get organization chart list
- Description: Retrieves a list of organization charts.<br>Executing this API requires team management permission.<br>Note: For performance reasons, the list of teams within each organization chart is not included in the response when retrieving the list of organization charts.<br>To get all the information of an organization chart, use the API that retrieves a single organization chart.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|name)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, name |

Response 200: Schema: array of [OrganizationChart](#schema-organizationchart)

#### `POST /v1/organizationCharts`

- operationId: `createOrganizationChart`
- Summary: Create organization chart
- Description: Creates an organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: [OrganizationChartBody](#schema-organizationchartbody)

Response 200: Schema: [OrganizationChartDetail](#schema-organizationchartdetail)

#### `DELETE /v1/organizationCharts/{organizationChartId}`

- operationId: `deleteOrganizationChart`
- Summary: Delete organization chart
- Description: Deletes an organization chart. All teams and memberships within the organization chart are also deleted at the same time.<br>Executing this API requires team management permission.<br>Note: The currently active organization chart cannot be deleted.<br>Note: Deleting an organization chart can take time, so deletion is executed asynchronously. Be aware that the response is returned before the deletion completes.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Response 200: empty body

#### `GET /v1/organizationCharts/{organizationChartId}`

- operationId: `getOrganizationChart`
- Summary: Get organization chart
- Description: Retrieves a single organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Response 200: Schema: [OrganizationChartDetail](#schema-organizationchartdetail)

#### `PATCH /v1/organizationCharts/{organizationChartId}`

- operationId: `updateOrganizationChart`
- Summary: Update organization chart
- Description: Updates an organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Request body (`application/json`): Schema: [OrganizationChartBody](#schema-organizationchartbody)

Response 200: Schema: [OrganizationChartDetail](#schema-organizationchartdetail)

#### `GET /v1/organizationChart`

- operationId: `getCurrentOrganizationChart`
- Summary: Get current organization chart
- Description: Retrieves the currently active organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Response 200: Schema: [OrganizationChartDetail](#schema-organizationchartdetail)

#### `POST /v1/organizationCharts/{organizationChartId}/activate`

- operationId: `activateOrganizationChart`
- Summary: Activate organization chart
- Description: Activates the specified organization chart and switches the organization chart used by workflows.<br>If other organization charts are scheduled to be activated, those schedules are deleted.<br>Executing this API requires team management permission.<br>Note: Activating an organization chart can take time, so activation is executed asynchronously. Be aware that the response is returned before the activation completes.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `dueOn` | string(date) |  |  | Date to activate. If null, it is activated immediately. |

Response 200: Schema: [OrganizationChartDetail](#schema-organizationchartdetail)

### Teams

#### `GET /v1/organizationCharts/{organizationChartId}/teams`

- operationId: `listTeams`
- Summary: Get team list
- Description: Retrieves a list of teams within the specified organization chart.<br>If parentId is specified, returns the list of teams under the specified parent team; if parentId is not specified, returns the list of root teams in the organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `parentId` | query | string |  |  | Parent team UUID |
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Response 200: Schema: array of [Team](#schema-team)

#### `POST /v1/organizationCharts/{organizationChartId}/teams`

- operationId: `createTeam`
- Summary: Create team
- Description: Creates a team within the specified organization chart.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |

Request body (`application/json`): Schema: [TeamCreateBody](#schema-teamcreatebody)

Response 200: Schema: [TeamDetail](#schema-teamdetail)

#### `GET /v1/organizationCharts/{organizationChartId}/teams/{teamId}`

- operationId: `getTeam`
- Summary: Get team
- Description: Retrieves a single team.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Response 200: Schema: [TeamDetail](#schema-teamdetail)

#### `PATCH /v1/organizationCharts/{organizationChartId}/teams/{teamId}`

- operationId: `updateTeam`
- Summary: Edit team
- Description: Updates a team.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Request body (`application/json`): Schema: [TeamUpdateBody](#schema-teamupdatebody)

Response 200: Schema: [TeamDetail](#schema-teamdetail)

#### `DELETE /v1/organizationCharts/{organizationChartId}/teams/{teamId}`

- operationId: `deleteTeam`
- Summary: Delete team
- Description: Deletes a team. All memberships of this team are also deleted at the same time.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Response 200: empty body

#### `GET /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships`

- operationId: `listTeamMembers`
- Summary: Get team member list
- Description: Retrieves a list of team members.<br>Executing this API requires team management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page. 1 is the first page. |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Response 200: Schema: array of [MemberUser](#schema-memberuser)

#### `POST /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships`

- operationId: `createTeamMembers`
- Summary: Add members to team
- Description: Adds members to the specified team. Up to 10 members can be added at once.<br>Executing this API requires team management permission.<br>Note: A team requires at least one manager. If there is no manager, the API returns 422 Unprocessable Content.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `users` | object[] | yes | minItems: 1; maxItems: 10 | Array of information on users to add as members |
| `users[].id` | string(uuid) | yes |  | User UUID |
| `users[].leader` | boolean | yes |  | true if the user is a manager |
| `users[].gradeIds` | string(uuid)[] |  | minItems: 1 | Grade UUID |

Response 200: empty body

#### `DELETE /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships`

- operationId: `deleteTeamMembers`
- Summary: Remove members from team
- Description: Removes members from the specified team. Up to 10 members can be removed at once.<br>Executing this API requires team management permission.<br>Note: If members remain after removal, the team requires at least one manager. If removing members would leave the team without a manager, the API returns 422 Unprocessable Content.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userIds` | string(uuid)[] | yes | minItems: 1; maxItems: 10 | Array of user UUIDs |

Response 200: empty body

#### `PATCH /v1/organizationCharts/{organizationChartId}/teams/{teamId}/memberships/{userId}`

- operationId: `updateTeamMember`
- Summary: Update team member
- Description: Updates the specified member.<br>Executing this API requires team management permission.<br>Note: A team requires at least one manager. If there is no manager, the API returns 422 Unprocessable Content.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `organizationChartId` | path | string(uuid) | yes |  | Organization chart UUID |
| `teamId` | path | string | yes |  | Team UUID or code |
| `userId` | path | string | yes |  | User UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `leader` | boolean | yes |  | true if the user is a manager |
| `gradeIds` | string(uuid)[] | yes | minItems: 1 | Grade UUID |

Response 200: empty body

### Categories

#### `GET /v1/categories`

- operationId: `listCategories`
- Summary: Get category list
- Description: Retrieves a list of categories.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(name\|createdAt\|updatedAt)(-asc\|-desc)?$` | Field and order to sort by. Available fields: name, createdAt, updatedAt |

Response 200: Schema: array of [Category](#schema-category)

#### `POST /v1/categories`

- operationId: `createCategory`
- Summary: Create category
- Description: Creates a category.<br>Executing this API requires permission to manage workflow-related settings.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |

Response 200: Schema: [Category](#schema-category)

#### `DELETE /v1/categories/{categoryId}`

- operationId: `deleteCategory`
- Summary: Delete category
- Description: Deletes a category.<br>Executing this API requires permission to manage workflow-related settings.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `categoryId` | path | string(uuid) | yes |  | Category UUID |

Response 200: empty body

#### `PATCH /v1/categories/{categoryId}`

- operationId: `updateCategory`
- Summary: Update category
- Description: Updates a category.<br>Executing this API requires permission to manage workflow-related settings.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `categoryId` | path | string(uuid) | yes |  | Category UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |

Response 200: Schema: [Category](#schema-category)

### Folders

#### `GET /v1/folders`

- operationId: `listFolders`
- Summary: Get folder list
- Description: Retrieves a list of folders.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|name\|fullName)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, name, fullName |

Response 200: Schema: array of [Folder](#schema-folder)

#### `POST /v1/folders`

- operationId: `createFolder`
- Summary: Create folder
- Description: Creates a folder.<br>Executing this API requires permission to manage workflow-related settings.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `description` | string |  |  | Folder description |
| `parentFolderId` | string(UUID)\|null |  |  | Parent folder ID |

Response 200: Schema: [FolderDetail](#schema-folderdetail)

#### `DELETE /v1/folders/{folderId}`

- operationId: `deleteFolder`
- Summary: Delete folder
- Description: Deletes a folder.<br>Executing this API requires permission to manage workflow-related settings.<br>Note: All folders, workflows, routes, and pipelines under this folder are also deleted.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `folderId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Folder UUID or code |

Response 200: empty body

#### `PATCH /v1/folders/{folderId}`

- operationId: `updateFolder`
- Summary: Update folder
- Description: Updates a folder.<br>Executing this API requires permission to manage workflow-related settings.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `folderId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Folder UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string |  |  | Name |
| `code` | string |  |  | Code |
| `description` | string |  |  | Folder description |
| `parentFolderId` | string(UUID)\|null |  |  | Parent folder ID |

Response 200: Schema: [FolderDetail](#schema-folderdetail)

#### `GET /v1/folders/{folderId}`

- operationId: `getFolder`
- Summary: Get folder
- Description: Retrieves a single folder.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `folderId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Folder UUID or code |

Response 200: Schema: [FolderDetail](#schema-folderdetail)

### Routes

#### `GET /v1/routes`

- operationId: `listRoutes`
- Summary: Get the list of routes
- Description: Retrieves the list of routes. Can be filtered by status and folder.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page. 1 is the first page. |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|folderId)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, folderId |
| `status` | query | string[] |  | enum: `visible`, `error` | Status |
| `folderId` | query | string(uuid) |  |  | Folder UUID |

Response 200: Schema: array of [Route](#schema-route)

#### `GET /v1/routes/{routeId}`

- operationId: `getRoute`
- Summary: Get a route
- Description: Retrieves the specified route.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `routeId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Route UUID or code |

Response 200: Schema: [RouteDetail](#schema-routedetail)

### Workflows

#### `GET /v1/workflows`

- operationId: `listWorkflows`
- Summary: Get the list of workflows
- Description: Retrieves the list of workflows. Can be filtered by status.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page. 1 is the first page. |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|updatedAt\|name\|status)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, updatedAt, name, status |
| `status` | query | string[] |  | enum: `visible`, `invisible` | Status |

Response 200: Schema: array of [Workflow](#schema-workflow)

#### `GET /v1/workflows/{workflowId}`

- operationId: `getWorkflow`
- Summary: Get a workflow
- Description: Retrieves the workflow with the specified ID.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `workflowId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Workflow UUID or code |

Response 200: Schema: [WorkflowDetail](#schema-workflowdetail)

### Tickets

#### `GET /v1/tickets`

- operationId: `listTickets`
- Summary: Get ticket list
- Description: Retrieves a list of tickets. Multiple conditions can be combined to filter.<br>Note 1: For performance reasons, form inputs, approval routes, and the like are not included in the response when retrieving the ticket list.<br>To get more detailed ticket information, use the API that retrieves a single ticket.<br>Note 2: Only tickets viewable by the user executing the API can be retrieved.<br>To target all tickets in the tenant, the user executing the API must have ticket management permission (view).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|updatedAt)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, updatedAt |
| `status` | query | string or string[] |  | enum: `draft`, `in_progress`, `rejected`, `completed`, `denied`, `archived` | Array of statuses |
| `subStatusIds` | query | string(uuid)[] |  |  | Array of sub-status UUIDs |
| `workflowId` | query | string |  |  | Workflow UUID |
| `authorId` | query | string |  |  | Applicant UUID |
| `authorTeamFullName` | query | string |  | maxLength: 300 | Name, including parent organizations, of the team selected at the time of application |
| `ticketNumber` | query | string |  |  | Ticket number |
| `createdAtStart` | query | string(date) or string(date-time) |  |  | Start of creation date and time range |
| `createdAtEnd` | query | string(date) or string(date-time) |  |  | End of creation date and time range |
| `updatedAtStart` | query | string(date) or string(date-time) |  |  | Start of update date and time range |
| `updatedAtEnd` | query | string(date) or string(date-time) |  |  | End of update date and time range |
| `openedAtStart` | query | string(date) or string(date-time) |  |  | Start of application date and time range |
| `openedAtEnd` | query | string(date) or string(date-time) |  |  | End of application date and time range |
| `completedAtStart` | query | string(date) or string(date-time) |  |  | Start of completion date and time range |
| `completedAtEnd` | query | string(date) or string(date-time) |  |  | End of completion date and time range |
| `archivedAtStart` | query | string(date) or string(date-time) |  |  | Start of archive date and time range |
| `archivedAtEnd` | query | string(date) or string(date-time) |  |  | End of archive date and time range |
| `assigneeUserId` | query | string(uuid) |  |  | Approver UUID. Specify together with assigneeStatus. |
| `assigneeStatus` | query | string[] |  | enum: `all`, `current`, `approved` | Approver status. Specify together with assigneeUserId. |
| `stepTitle` | query | string |  | maxLength: 300 | Current approval step name |

Response 200: Schema: array of [TicketWithStep](#schema-ticketwithstep)

#### `POST /v1/tickets`

- operationId: `createTicket`
- Summary: Create ticket
- Description: Creates a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `status` | string | yes | enum: `draft`, `in_progress` | Status. Only draft or in_progress can be selected on creation. |
| `workflowId` | string(uuid) | yes |  | Workflow UUID |
| `authorTeamId` | string(uuid) | yes |  | Applying team UUID |
| `proxyClientUserId` | string(uuid)\|null |  |  | UUID of the user who is the requester. Specify only for proxy applications. |
| `title` | string\|null |  |  | Ticket title. Can be set only when titleInputMode is input in the workflow. |
| `slipItems` | object[] |  |  | Array of slip items. This field is required for slip workflows. |
| `slipItems[].slipSectionId` | string(uuid)\|null |  |  | Slip section UUID. Specify either slipSectionId or slipSectionCode. If omitted, the first slip section is used. |
| `slipItems[].slipSectionCode` | string\|null |  |  | Slip section code. Specify either slipSectionId or slipSectionCode. If omitted, the first slip section is used. |
| `slipItems[].inputs` | object[] | yes |  | Array of slip item inputs |
| `slipItems[].inputs[].slipFieldId` | string(uuid)\|null |  |  | Slip field UUID. Only one of slipFieldId or slipFieldCode is required. |
| `slipItems[].inputs[].slipFieldCode` | string\|null |  |  | Slip field code. Only one of slipFieldId or slipFieldCode is required. |
| `slipItems[].inputs[].value` | string\|null or string[] |  |  | Input value. Specify as an array when the field is of checkbox type. |
| `slipItems[].inputs[].generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `slipItems[].inputs[].userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `slipItems[].inputs[].teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `slipItems[].inputs[].ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `slipItems[].inputs[].files` | string[]\|null |  |  | Signed ID of the attached file.<br>Specify only when the field is of file type. |
| `inputs` | object[] |  |  | Array of form inputs. Include inputs for all form fields of the workflow. Can be omitted for workflows with no form fields (slip sections only). Required for all other workflows. |
| `inputs[].formFieldId` | string(uuid)\|null |  |  | Form field UUID. Only one of formFieldId or formFieldCode is required. |
| `inputs[].formFieldCode` | string\|null |  |  | Form field code. Only one of formFieldId or formFieldCode is required. |
| `inputs[].value` | string\|null or string[] |  |  | Input value. Specify as an array when the field is of checkbox type. |
| `inputs[].generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `inputs[].userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `inputs[].teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `inputs[].ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `inputs[].files` | string[]\|null |  |  | Signed ID of the attached file.<br>Specify only when the field is of file type. |
| `cloudSignDocument` | object |  |  | CloudSign document. Specify only when CloudSign integration is enabled in the workflow. |
| `cloudSignDocument.id` | string | yes |  | CloudSign document ID |
| `cloudSignDocument.title` | string | yes |  | CloudSign document title |
| `cloudSignDocument.sandbox` | string | yes |  | true if using the CloudSign sandbox environment |
| `approvers` | object[]\|null |  |  | Array specifying the approvers of route steps whose approval type is "designated by applicant". |
| `approvers[].routeStepCode` | string | yes |  | Code of the route step for which approvers are specified |
| `approvers[].userId` | string(uuid)[] | yes |  | UUID of the user to designate as approver |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `GET /v1/tickets/tasks`

- operationId: `listTasks`
- Summary: Get approval request list
- Description: Retrieves a list of approval requests assigned to the current user. Multiple conditions can be combined to filter.<br>Note: For performance reasons, form inputs, approval routes, and the like are not included in the response when retrieving the ticket list.<br>To get more detailed ticket information, use the API that retrieves a single ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|updatedAt\|openedAt\|assignedToMeAt)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, updatedAt, openedAt, assignedToMeAt |
| `status` | query | string[] |  | enum: `draft`, `in_progress`, `rejected`, `completed`, `deleted` | Array of statuses |
| `subStatusIds` | query | string(uuid)[] |  |  | Array of sub-status UUIDs |
| `workflowId` | query | string |  |  | Workflow UUID |
| `authorId` | query | string |  |  | Applicant UUID |
| `authorTeamFullName` | query | string |  | maxLength: 300 | Name, including parent organizations, of the team selected at the time of application |
| `ticketNumber` | query | string |  |  | Ticket number |
| `createdAtStart` | query | string(date-time) |  |  | Start of creation date and time range |
| `createdAtEnd` | query | string(date-time) |  |  | End of creation date and time range |
| `updatedAtStart` | query | string(date-time) |  |  | Start of update date and time range |
| `updatedAtEnd` | query | string(date-time) |  |  | End of update date and time range |
| `openedAtStart` | query | string(date-time) |  |  | Start of application date and time range |
| `openedAtEnd` | query | string(date-time) |  |  | End of application date and time range |
| `assignedToMeAtStart` | query | string(date-time) |  |  | Start of approval request date and time range |
| `assignedToMeAtEnd` | query | string(date-time) |  |  | End of approval request date and time range |
| `completedAtStart` | query | string(date-time) |  |  | Start of completion date and time range |
| `completedAtEnd` | query | string(date-time) |  |  | End of completion date and time range |
| `archivedAtStart` | query | string(date-time) |  |  | Start of archive date and time range |
| `archivedAtEnd` | query | string(date-time) |  |  | End of archive date and time range |
| `stepTitle` | query | string |  | maxLength: 300 | Current approval step name |
| `pending` | query | string |  | enum: `true`, `false` | Filters tickets by approval hold status.<br>- true: Retrieve only tickets on hold<br>- false: Retrieve only tickets not on hold |

Response 200: Schema: array of [TicketWithStep](#schema-ticketwithstep)

#### `GET /v1/tickets/{ticketId}`

- operationId: `getTicket`
- Summary: Get a ticket
- Description: Retrieves a single ticket. Returns detailed data including form inputs and the approval route.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `PATCH /v1/tickets/{ticketId}`

- operationId: `updateTicket`
- Summary: Update a ticket
- Description: Updates a ticket.<br>Note 1: If the ticket status is draft or rejected, the applicant can update it.<br>Note 2: If the ticket status is in progress, an approver can update only the approver fields. Set only slipItems or inputs in the request body (other parameters are ignored).<br>Note 3: When specified, inputs and slipItems replace the entire form. When specifying them, send all values contained in the form being updated. To partially update only some form fields or slip items, use `PATCH /tickets/{ticketId}/form` (updateTicketForm).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `status` | string |  | enum: `draft`, `in_progress`, `rejected` | Status. For updates, only draft, in_progress, and rejected can be selected. |
| `authorTeamId` | string(uuid) |  |  | Applying team UUID |
| `proxyClientUserId` | string(uuid)\|null |  |  | UUID of the user who is the requester. Specify only for proxy applications. |
| `title` | string |  |  | Ticket title. Can be set only when allow_title is true in the workflow. |
| `slipItems` | object[] |  |  | Array of slip items.<br>Note: If omitted, the existing values are used. |
| `slipItems[].slipSectionId` | string(uuid)\|null |  |  | Slip section UUID. Specify either slipSectionId or slipSectionCode. If omitted, the first slip section is used. |
| `slipItems[].slipSectionCode` | string\|null |  |  | Slip section code. Specify either slipSectionId or slipSectionCode. If omitted, the first slip section is used. |
| `slipItems[].inputs` | object[] | yes |  | Array of slip item inputs |
| `slipItems[].inputs[].slipFieldId` | string\|null |  |  | Slip field UUID. Only one of slipFieldId or slipFieldCode is required. |
| `slipItems[].inputs[].slipFieldCode` | string\|null |  |  | Slip field code. Only one of slipFieldId or slipFieldCode is required. |
| `slipItems[].inputs[].value` | string\|null or string[] |  |  | Input value. Specify as an array when the field is of checkbox type. |
| `slipItems[].inputs[].generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `slipItems[].inputs[].userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `slipItems[].inputs[].teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `slipItems[].inputs[].ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `slipItems[].inputs[].files` | string[]\|null |  |  | Signed ID of the attached file.<br>Specify only when the field is of file type. |
| `inputs` | object[] |  |  | Array of form inputs.<br>Note: When updated by the applicant, include inputs corresponding to all form fields of the workflow.<br>Note: When updated by an approver, include only inputs corresponding to the approver fields.<br>Note: If omitted, the existing values are used. |
| `inputs[].formFieldId` | string(uuid)\|null |  |  | Form field UUID. Only one of formFieldId or formFieldCode is required. |
| `inputs[].formFieldCode` | string\|null |  |  | Form field code. Only one of formFieldId or formFieldCode is required. |
| `inputs[].value` | string\|null or string[] |  |  | Input value. Specify as an array when the field is of checkbox type. |
| `inputs[].generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `inputs[].userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `inputs[].teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `inputs[].ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `inputs[].files` | string[]\|null |  |  | Signed ID of the attached file.<br>Specify only when the field is of file type. |
| `cloudSignDocument` | object |  |  | CloudSign document. Specify only when CloudSign integration is enabled in the workflow. |
| `cloudSignDocument.id` | string | yes |  | CloudSign document ID |
| `cloudSignDocument.title` | string | yes |  | CloudSign document title |
| `cloudSignDocument.sandbox` | boolean | yes |  | true if using the CloudSign sandbox environment |
| `approvers` | object[]\|null |  |  | Array specifying the approvers of route steps whose approval type is "designated by applicant". |
| `approvers[].routeStepCode` | string | yes |  | Code of the route step for which approvers are specified |
| `approvers[].userId` | string(uuid)[] | yes |  | UUID of the user to designate as approver |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `PATCH /v1/tickets/{ticketId}/form`

- operationId: `updateTicketForm`
- Summary: Partially update ticket form data
- Description: Partially updates the ticket form data (form input values `inputs` and slip items `slipItems`). Only the submitted parts are overwritten, and unsent parts keep their existing values (`inputs` are updated per field, `slipItems` per slip section).<br>Note 1: If the ticket status is draft or rejected, the applicant can update it.<br>Note 2: If the ticket status is in progress, an approver can update only the approver fields.<br>Note 3: The `inputs` of each row in a slip (`slipItems`) must include all editable fields. Omit fields that cannot be edited (e.g., approver fields when the applicant is updating); their existing values are kept.<br>Note 4: Specify either `formFieldId` or `formFieldCode` for `inputs[]`, either `slipSectionId` or `slipSectionCode` for `slipItems[]`, and either `slipFieldId` or `slipFieldCode` for `slipItems[].inputs[]` (if both are specified, the ID takes precedence).<br>Note 5: It is an error if both `inputs` and `slipItems` are unspecified or empty arrays.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`, required): Schema: [TicketFormPartialUpdateBody](#schema-ticketformpartialupdatebody)

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `POST /v1/tickets/{ticketId}/approve`

- operationId: `approveTicket`
- Summary: Approve or confirm a ticket
- Description: Approves or confirms the specified ticket.<br>Returns 403 Forbidden if the user executing the API is not assigned to the ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `comment` | string |  |  | Comment body. Required if the workflow's commentingRequiredOnApproval is true. Cannot be specified if the workflow's commentingEnabled is false. |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `POST /v1/tickets/{ticketId}/reject`

- operationId: `rejectTicket`
- Summary: Reject (send back) a ticket
- Description: Rejects (sends back) the specified ticket.<br>Returns 403 Forbidden if the user executing the API is not assigned to the ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `to` | integer | yes |  | Step number to send back to (0 is the ticket creator, 1 is the first approval step) |
| `comment` | string |  |  | Comment body. Required if the workflow's commentingRequiredOnRejection is true. Cannot be specified if the workflow's commentingEnabled is false. |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `POST /v1/tickets/{ticketId}/deny`

- operationId: `denyTicket`
- Summary: Deny a ticket
- Description: Denies the specified ticket.<br>Returns 403 Forbidden if the user executing the API is not assigned to the ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `comment` | string |  |  | Comment body. Required if the workflow's commentingRequiredOnDenial is true. Cannot be specified if the workflow's commentingEnabled is false. |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `POST /v1/tickets/{ticketId}/withdraw`

- operationId: `withdrawTicket`
- Summary: Withdraw a ticket
- Description: Withdraws a ticket you created.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `POST /v1/tickets/{ticketId}/archive`

- operationId: `archiveTicket`
- Summary: Archive a ticket
- Description: Archives a ticket. The ticket status changes to the archived status, but the ticket remains accessible through the UI and API.<br>Note: The users who can archive a ticket differ depending on the ticket status. See the [help page](https://support.kickflow.com/hc/ja/articles/360058324973) for details.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: [TicketDetail](#schema-ticketdetail)

#### `GET /v1/tickets/{ticketId}/links`

- operationId: `listTicketLinks`
- Summary: Get related tickets of a ticket
- Description: Retrieves the related tickets of the specified ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: array of [Ticket](#schema-ticket)

#### `GET /v1/tickets/{ticketId}/viewers`

- operationId: `listViewers`
- Summary: Get the list of viewers
- Description: Retrieves the list of viewers of a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt |
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: array of [TicketViewer](#schema-ticketviewer)

#### `POST /v1/tickets/{ticketId}/viewers`

- operationId: `createViewer`
- Summary: Add a viewer
- Description: Adds a viewer to a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userId` | string(uuid) or string(uuid)[] or null |  |  | User UUID. Exactly one of userId and teamId is required. |
| `teamId` | string(uuid) or string(uuid)[] or null |  |  | Team UUID. Exactly one of userId and teamId is required. |
| `gradeId` | string(uuid)\|null |  |  | Grade UUID. Optional, and can be specified only when teamId is specified. |
| `descendants` | boolean\|null |  |  | Whether to include lower-level teams. Can be specified only when teamId is specified. Treated as false when unspecified. |

Response 200: empty body

#### `DELETE /v1/tickets/{ticketId}/viewers/{viewerId}`

- operationId: `deleteViewer`
- Summary: Remove a viewer
- Description: Removes a viewer from a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |
| `viewerId` | path | string(uuid) | yes |  | Viewer UUID |

Response 200: empty body

### Files

#### `POST /v1/files`

- operationId: `uploadFile`
- Summary: Upload an attachment
- Description: Uploads an attachment. Files up to 50MB can be uploaded.<br>Note: This API is available only to customers on the enterprise plan.<br>Note: Use uploaded files promptly, for example when creating a ticket. Files not referenced by any ticket or the like are automatically deleted after at least 24 hours.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`multipart/form-data`, required): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `file` | string(binary) | yes |  | Attachment |

Response 200: Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `signedId` | string | yes |  | Signed IDs of attachments |

#### `GET /v1/files/{signedId}`

- operationId: `getFile`
- Summary: Get attachment information
- Description: Retrieves information about an attachment, including its URL on Amazon S3.<br>Note: The URL returned by this API expires after 5 minutes.<br>Note: The URL cannot be obtained for files not attached to a ticket. Attach the file to a ticket first.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `signedId` | path | string | yes |  | Signed IDs of attachments |

Response 200: Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `url` | string(uri) | yes |  | Amazon S3 URL |
| `filename` | string | yes |  | File name |
| `checksum` | string | yes |  | Checksum |
| `byteSize` | integer | yes |  | Number of bytes |
| `contentType` | string | yes |  | Content-Type |
| `createdAt` | string(date-time) | yes |  | Created at |

### General masters

#### `GET /v1/generalMasters`

- operationId: `listGeneralMasters`
- Summary: Get general master list
- Description: Retrieves a list of general masters.<br>Executing this API requires general master management permission (view or edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|code\|name)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, code, name |

Response 200: Schema: array of [GeneralMaster](#schema-generalmaster)

#### `POST /v1/generalMasters`

- operationId: `createGeneralMaster`
- Summary: Create general master
- Description: Creates a general master.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |
| `code` | string\|null |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `description` | string\|null |  |  | Description |
| `defaultSortBy` | string |  | enum: `name`, `code` | Default sort order of the item list |
| `fields` | object[] |  |  | Array of custom fields |
| `fields[].title` | string | yes |  | Field name |
| `fields[].description` | string\|null |  |  | Field description |
| `fields[].code` | string | yes |  | Field code |
| `fields[].required` | boolean | yes |  | Whether input is required |
| `fields[].fieldType` | string | yes | enum: `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date` | Field type |
| `fields[].options` | string[]\|null |  |  | Choices. Required when fieldType is checkbox or pull_down. |
| `fields[].visible` | boolean |  |  | true if non-admins can also view it |

Response 200: Schema: [GeneralMaster](#schema-generalmaster)

#### `GET /v1/generalMasters/{generalMasterId}`

- operationId: `getGeneralMaster`
- Summary: Get general master
- Description: Retrieves a single general master.<br>Executing this API requires general master management permission (view or edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |

Response 200: Schema: [GeneralMaster](#schema-generalmaster)

#### `PATCH /v1/generalMasters/{generalMasterId}`

- operationId: `updateGeneralMaster`
- Summary: Update general master
- Description: Updates a general master.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `name` | string |  |  | Name |
| `description` | string |  |  | Description |
| `defaultSortBy` | string |  | enum: `name`, `code` | Default sort order of the item list |
| `fields` | object[] |  |  | Array of custom fields |
| `fields[].title` | string |  |  | Field name |
| `fields[].description` | string\|null |  |  | Field description |
| `fields[].code` | string | yes |  | Field code |
| `fields[].required` | boolean |  |  | Whether input is required |
| `fields[].fieldType` | string |  |  | Field type |
| `fields[].options` | string[]\|null |  |  | Choices. Required only when fieldType is checkbox or pull_down. |
| `fields[].visible` | boolean |  |  | true if non-admins can also view it |

Response 200: Schema: [GeneralMaster](#schema-generalmaster)

#### `DELETE /v1/generalMasters/{generalMasterId}`

- operationId: `deleteGeneralMaster`
- Summary: Delete general master
- Description: Deletes a general master. All items of this general master are also deleted at the same time.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |

Response 200: empty body

#### `GET /v1/generalMasters/{generalMasterId}/items`

- operationId: `listGeneralMasterItems`
- Summary: Get general master item list
- Description: Retrieves a list of general master items.<br>Executing this API requires general master management permission (view or edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|code\|name)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, code, name |
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |

Response 200: Schema: array of [GeneralMasterItem](#schema-generalmasteritem)

#### `POST /v1/generalMasters/{generalMasterId}/items`

- operationId: `createGeneralMasterItem`
- Summary: Create general master item
- Description: Creates a general master item.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `name` | string | yes |  | Name |
| `description` | string |  |  | Description |
| `startsOn` | string(date)\|null |  |  | Start date of validity period |
| `endsOn` | string(date)\|null |  |  | End date of validity period |
| `inputs` | object[] | yes |  | Custom field inputs. Inputs must be provided for all custom fields, including those that are not required. |
| `inputs[].code` | string | yes |  | Field code |
| `inputs[].value` | string\|null or string[] | yes |  | Input value. An array of strings if the custom field is checkbox; otherwise a string. |

Response 200: Schema: [GeneralMasterItem](#schema-generalmasteritem)

#### `GET /v1/generalMasters/{generalMasterId}/items/{itemId}`

- operationId: `getGeneralMasterItem`
- Summary: Get general master item
- Description: Retrieves a single general master item.<br>Executing this API requires general master management permission (view or edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |
| `itemId` | path | string | yes |  | General master item UUID or code |

Response 200: Schema: [GeneralMasterItem](#schema-generalmasteritem)

#### `PATCH /v1/generalMasters/{generalMasterId}/items/{itemId}`

- operationId: `updateGeneralMasterItem`
- Summary: Update general master item
- Description: Updates a general master item.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |
| `itemId` | path | string | yes |  | General master item UUID or code |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `name` | string |  |  | Name |
| `description` | string |  |  | Description |
| `startsOn` | string(date)\|null |  |  | Start date of validity period |
| `endsOn` | string(date)\|null |  |  | End date of validity period |
| `inputs` | object[] |  |  | Custom field inputs. Inputs must be provided for all custom fields, including those that are not required. |
| `inputs[].code` | string | yes |  | Field code |
| `inputs[].value` | string\|null or string[] | yes |  | Input value. An array of strings if the custom field is checkbox; otherwise a string. |

Response 200: Schema: [GeneralMasterItem](#schema-generalmasteritem)

#### `DELETE /v1/generalMasters/{generalMasterId}/items/{itemId}`

- operationId: `deleteGeneralMasterItem`
- Summary: Delete general master item
- Description: Deletes a general master item.<br>Executing this API requires general master management permission (edit).
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `generalMasterId` | path | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | General master UUID or code |
| `itemId` | path | string | yes |  | General master item UUID or code |

Response 200: empty body

### Admin roles

#### `GET /v1/roles`

- operationId: `listRoles`
- Summary: Get admin role list
- Description: Retrieves a list of admin roles.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt\|name)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt, name |

Response 200: Schema: array of [Role](#schema-role)

#### `POST /v1/roles`

- operationId: `createRole`
- Summary: Create admin role
- Description: Creates an admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Request body (`application/json`): Schema: [RoleCreateBody](#schema-rolecreatebody)

Response 200: Schema: [RoleDetail](#schema-roledetail)

#### `GET /v1/roles/{roleId}`

- operationId: `getRole`
- Summary: Get admin role
- Description: Retrieves a single admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |

Response 200: Schema: [RoleDetail](#schema-roledetail)

#### `PATCH /v1/roles/{roleId}`

- operationId: `updateRole`
- Summary: Update admin role
- Description: Updates an admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |

Request body (`application/json`): Schema: [RoleUpdateBody](#schema-roleupdatebody)

Response 200: Schema: [RoleDetail](#schema-roledetail)

#### `DELETE /v1/roles/{roleId}`

- operationId: `deleteRole`
- Summary: Delete admin role
- Description: Deletes an admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |

Response 200: empty body

#### `POST /v1/roles/{roleId}/memberships`

- operationId: `createRoleMembers`
- Summary: Add members to admin role
- Description: Adds members to an admin role. Up to 10 members can be added at once.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userIds` | string(uuid)[] | yes | minItems: 1; maxItems: 10 | Array of user UUIDs |

Response 200: empty body

#### `GET /v1/roles/{roleId}/memberships`

- operationId: `listRoleMembers`
- Summary: Get admin role member list
- Description: Retrieves a list of members of an admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |

Response 200: Schema: array of [User](#schema-user)

#### `DELETE /v1/roles/{roleId}/memberships/{userId}`

- operationId: `deleteRoleMember`
- Summary: Remove members from admin role
- Description: Removes members from an admin role.<br>Executing this API requires role management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `roleId` | path | string(uuid) | yes |  | Admin role UUID |
| `userId` | path | string | yes |  | User UUID or code |

Response 200: empty body

### Comments

#### `GET /v1/tickets/{ticketId}/comments`

- operationId: `listComments`
- Summary: Get the list of comments
- Description: Retrieves the list of comments on a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt)(-asc\|-desc)?$` | Field and order to sort by. Available fields: createdAt |
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Response 200: Schema: array of [Comment](#schema-comment)

#### `POST /v1/tickets/{ticketId}/comments`

- operationId: `createComment`
- Summary: Post a comment
- Description: Posts a comment on a ticket.<br>If the workflow's `commentingEnabled` is `false`, returns `422` (`code: "validation_failed"`). The response's `errors.base` contains an array of error messages.<br>
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `body` | string | yes |  | Body |
| `files` | string[]\|null |  |  | Signed IDs of attachments |

Response 200: Schema: [Comment](#schema-comment)

#### `GET /v1/tickets/{ticketId}/comments/{commentId}`

- operationId: `getComment`
- Summary: Get a comment
- Description: Retrieves a comment on a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |
| `commentId` | path | string(uuid) | yes |  | Comment UUID |

Response 200: Schema: [Comment](#schema-comment)

#### `PATCH /v1/tickets/{ticketId}/comments/{commentId}`

- operationId: `updateComment`
- Summary: Update a comment
- Description: Updates a comment on a ticket. Attachments cannot be updated.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |
| `commentId` | path | string(uuid) | yes |  | Comment UUID |

Request body (`application/json`): Schema: object

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `body` | string | yes |  | Body |

Response 200: Schema: [Comment](#schema-comment)

#### `DELETE /v1/tickets/{ticketId}/comments/{commentId}`

- operationId: `deleteComment`
- Summary: Delete a comment
- Description: Deletes a comment on a ticket.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `ticketId` | path | string(uuid) | yes |  | Ticket UUID |
| `commentId` | path | string(uuid) | yes |  | Comment UUID |

Response 200: empty body

### Audit logs

#### `GET /v1/auditLogs`

- operationId: `listAuditLogs`
- Summary: Get the list of audit logs
- Description: Retrieves the list of audit logs.<br>Executing this API requires audit log management permission.
- Error statuses: 400, 401, 403, 404, 422, 500, 504

Parameters:

| Name | In | Type | Required | Constraints | Description |
|---|---|---|---|---|---|
| `page` | query | integer |  | default: `1`; minimum: 1 | Page |
| `perPage` | query | integer |  | default: `25`; minimum: 1; maximum: 100 | Number of items per page |
| `sortBy` | query | string |  | pattern: `^(createdAt)(-asc\|-desc)?$` | Field and order to sort by. Only createdAt can be specified as the field. |
| `createdAtStart` | query | string |  |  | Start of creation date and time range |
| `createdAtEnd` | query | string |  |  | End of creation date and time range |
| `userId` | query | string(uuid) |  |  | User ID |

Response 200: Schema: array of [AuditLog](#schema-auditlog)


---

## 4. Resource schemas

Generated from `components.schemas` of `kickflow-openapi.yaml` (Confirmed). "Extends" means OpenAPI `allOf` with the base schema; only the additional fields are listed. Nested inline objects are flattened with dotted paths. "Required" reflects the OpenAPI `required` list (in responses this means "always present", possibly `null` when the type includes `null`).

Schema families at a glance:

| Family | Schemas |
|---|---|
| Users and organization | User, UserDetail, UserInvitation, MemberUser, Grade, GradeCreateBody, GradeUpdateBody, OrganizationChart, OrganizationChartDetail, OrganizationChartBody, Team, TeamDetail, TeamCreateBody, TeamUpdateBody, InviteMembershipBody |
| Admin roles | Role, RoleDetail, RoleCreateBody, RoleUpdateBody |
| Proxies | ProxyApplicant, ProxyApprover |
| Classification | Category, Folder, FolderDetail, Label, SubStatus |
| General masters | GeneralMaster, GeneralMasterItem, GeneralMasterField, FieldGeneralMaster, GeneralMasterSearchFilter, CustomValidationGeneralMasterItem |
| Workflows and forms | Workflow, WorkflowInTicket, WorkflowDetail, WorkflowExternalPublish, TicketNumberKey, TicketNumberingTiming, ExcelTemplate, ExcelTemplateAllowedTicketStatus, SectionListItem, SectionConditionField, SectionViewer, FormField, FormFieldDetail, FormFieldInTicket, FormFieldApproverEditRestriction, FormFieldApproverEditSetting, FormCustomValidation, FormCustomValidationField, FormFieldTicketCopySetting, SlipField, SlipFieldDetail, SlipFieldInTicket, SlipFieldApproverEditRestriction, SlipFieldApproverEditSetting, SlipFieldGeneralMasterSearchFilter, SlipCustomValidation, SlipCustomValidationField, WorkflowRouteCondition, WorkflowRouteConditionField, WorkflowTicketViewer, WorkflowAvailableTeam, WorkflowWatcher, WorkflowFilterVisibility, CustomNumbering, CustomNumberingSetting |
| Integrations (workflow settings) | ExternalApiSetting, ClimberCloudSetting, KintoneApp, KintoneAppSetting, WorkflowSlackChannel, SlackAccessToken, WorkflowChatworkRoom, WorkflowMicrosoftTeamsChannel, WorkflowGoogleChatSetting, WorkflowReportUploadGoogleDrive, WorkflowReportUploadBox |
| Routes | Route, RouteDetail, RouteStep, RouteStepTarget, RouteStepCondition, RouteStepConditionField, RouteStepConditionGeneralMaster |
| Tickets | TicketSummary, Ticket, TicketWithStep, TicketDetail, TicketFormPartialUpdateBody, TicketFormPartialUpdateInput, TicketFormPartialUpdateSlipFieldInput, TicketFormPartialUpdateSlipItem, TicketInput, TicketSection, TicketSectionViewer, TicketGuest, TicketStep, TicketAssignee, CustomNumberingValue, Attachment, SlipItem, SlipItemInput, TicketViewer |
| Comments and logs | Comment, AuditLog |
| Webhooks | WebhookRequestBodyPing, WebhookRequestBodyTicket, WebhookRequestBodyComment, WebhookTenant |
| Errors | ErrorResponse |

<a id="schema-errorresponse"></a>

#### ErrorResponse

Error response

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `code` | string | yes |  | Error code |
| `message` | string | yes |  | Error message |

<a id="schema-grade"></a>

#### Grade

Grade

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes | maxLength: 300 | Name |
| `level` | integer | yes | minimum: 0; maximum: 255 | Level |
| `code` | string\|null | yes | maxLength: 100 | Code |
| `isDefault` | boolean | yes | default: `false` | Whether this is the default grade |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-gradecreatebody"></a>

#### GradeCreateBody

Request body when creating a grade

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes | maxLength: 300 | Name |
| `level` | integer | yes | minimum: 0; maximum: 255 | Level |
| `code` | string |  | maxLength: 100 | Code. If not specified, a random alphanumeric string is set automatically. |

<a id="schema-gradeupdatebody"></a>

#### GradeUpdateBody

Request body when updating a grade

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string |  | maxLength: 300 | Name |
| `level` | integer |  | minimum: 0; maximum: 255 | Level |
| `code` | string |  | maxLength: 100 | Code. If not specified, a random alphanumeric string is set automatically. |

<a id="schema-user"></a>

#### User

User

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `email` | string(email) | yes | maxLength: 254 | Email address |
| `code` | string | yes | maxLength: 100 | Code |
| `firstName` | string | yes | maxLength: 255 | First name |
| `lastName` | string | yes | maxLength: 255 | Last name |
| `fullName` | string | yes | maxLength: 255 | Full name |
| `employeeId` | string\|null | yes | maxLength: 30 | Employee number |
| `image` | object | yes |  | URL of the user image. Multiple URLs are returned, one per size. |
| `image.100x100` | string\|null | yes |  |  |
| `image.64x64` | string\|null | yes |  |  |
| `image.32x32` | string\|null | yes |  |  |
| `status` | string | yes | enum: `invited`, `activated`, `suspended`, `deactivated` | Status |
| `locale` | string | yes |  | Locale (ja or en) |
| `userType` | string |  | enum: `normal`, `assistant` | User type. Not included in the response of the team member list API. |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `deactivatedAt` | string(date-time)\|null |  |  | Deleted at |
| `lastUsedOn` | string(date)\|null |  |  | Last used date (the date of the last operation performed in kickflow, including operations via the UI as well as via the API or chat). Returned when user information under /v1/users is retrieved with a token that has user management permission. |
| `customFields` | object[] |  |  | List of user custom field values. Each element is { code, value, fieldType }.<br>code holds UserCustomField#code as is, without conversion.<br>fieldType is the input type of the user custom field definition.<br>value has a type according to fieldType (string / number / string[] / null).<br>For number / integer, newly saved values are returned as strings, while existing data saved under the old specification may be returned as numbers (clients must accept both string and number).<br>Only fields that have a value set are included.<br>Included only for enterprise/trial contract tenants that meet the following public boundary.<br>In regular user lists and details, fields with "viewable by anyone" enabled are returned,<br>and requests with user management permission also receive fields where it is disabled.<br>Nested users in responses outside the public boundary, such as role members,<br>do not include customFields at all.<br>A code whose definition does not exist is not included, regardless of user management permission.<br> |
| `customFields[].code` | string | yes |  | UserCustomField#code (as is, without conversion) |
| `customFields[].fieldType` | string\|null | yes | enum: `text`, `textLong`, `number`, `integer`, `checkbox`, `pullDown`, `date`, `null` | Input type of the user custom field |
| `customFields[].value` | string or number or string[] or null | yes |  | Value according to fieldType. For number / integer, newly saved values are returned as strings<br>(existing data saved under the old specification may be returned as numbers, so clients must accept both string and number).<br> |

<a id="schema-userdetail"></a>

#### UserDetail

User details

Extends: [User](#schema-user) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `userLineWorksAccount` | object or null | yes |  | LINE WORKS account. null if not linked. |
| `userLineWorksAccount.lineWorksAccountId` | string | yes |  | LINE WORKS account ID |
| `invitation` | [UserInvitation](#schema-userinvitation) or null | yes |  | Invitation information. null if no invitation information exists. |

<a id="schema-userinvitation"></a>

#### UserInvitation

User invitation

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `expiresAt` | string(date-time) | yes |  | Invitation expiration |
| `userEmail` | string(email) | yes | maxLength: 254 | Email address invited to |
| `userLocale` | string | yes |  | Language of the invitee |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-proxyapplicant"></a>

#### ProxyApplicant

Proxy applicant

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `user` | [User](#schema-user) | yes |  |  |
| `proxyUser` | [User](#schema-user) | yes |  |  |
| `startsOn` | string(date)\|null | yes |  | Start date |
| `endsOn` | string(date)\|null | yes |  | End date |
| `workflows` | [Workflow](#schema-workflow)[] | yes |  | Target workflows |

<a id="schema-proxyapprover"></a>

#### ProxyApprover

Proxy approver

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `user` | [User](#schema-user) | yes |  |  |
| `proxyUser` | [User](#schema-user) | yes |  |  |
| `startsOn` | string(date)\|null | yes |  | Start date |
| `endsOn` | string(date)\|null | yes |  | End date |
| `workflows` | [Workflow](#schema-workflow)[] | yes |  | Target workflows |

<a id="schema-team"></a>

#### Team

Team

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes | maxLength: 300 | Name |
| `fullName` | string | yes |  | Name including parent organizations |
| `code` | string | yes | maxLength: 100 | Code |
| `notes` | string\|null | yes | maxLength: 10000 | Administrative note |
| `approveOnly` | boolean | yes |  | Whether this is an approval-only team |
| `usersCount` | integer | yes | minimum: 0 | Number of users |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-teamdetail"></a>

#### TeamDetail

Team details

Extends: [Team](#schema-team) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `parent` | [Team](#schema-team) or null | yes |  | Parent team |
| `children` | [Team](#schema-team)[] | yes |  | Child teams |
| `users` | [MemberUser](#schema-memberuser)[] | yes |  | Array of members.<br>Note: For performance reasons, more than 100 members cannot be returned. To retrieve all members when there are 101 or more, call the member retrieval API separately. |

<a id="schema-teamcreatebody"></a>

#### TeamCreateBody

Request body when creating a team

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `notes` | string\|null |  |  | Administrative note |
| `parentId` | string(uuid) |  |  | UUID of the parent team. If null, the created team becomes a root. |
| `approveOnly` | boolean |  |  | Whether this is an approval-only team |

<a id="schema-teamupdatebody"></a>

#### TeamUpdateBody

Request body when updating a team

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string |  |  | Name |
| `code` | string |  |  | Code. If not specified, a random alphanumeric string is set automatically. |
| `notes` | string |  |  | Administrative note |
| `parentId` | string(uuid) |  |  | ID of the parent team |
| `approveOnly` | boolean |  |  | Whether this is an approval-only team |

<a id="schema-organizationchart"></a>

#### OrganizationChart

Organization chart

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes | maxLength: 300 | Name |
| `current` | boolean | yes |  | Whether this is the currently active organization chart |
| `teamsCount` | integer | yes |  | Number of teams |
| `membershipsCount` | integer | yes |  | Number of affiliations |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `activationPlan` | object\|null | yes |  | Scheduled activation |
| `activationPlan.id` | string | yes |  | UUID |
| `activationPlan.dueOn` | string(date) | yes |  | Scheduled activation date |
| `activationPlan.createdAt` | string(date-time) | yes |  | Created at |
| `activationPlan.updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-organizationchartdetail"></a>

#### OrganizationChartDetail

Organization chart details

Extends: [OrganizationChart](#schema-organizationchart) (all base fields are included).

_(no fields)_

<a id="schema-organizationchartbody"></a>

#### OrganizationChartBody

Request body when creating or updating an organization chart

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |

<a id="schema-memberuser"></a>

#### MemberUser

Members belonging to the team

Extends: [User](#schema-user) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `grades` | [Grade](#schema-grade)[] | yes | minItems: 1 | Grade |
| `leader` | boolean | yes |  | Whether this is a supervisor |

<a id="schema-role"></a>

#### Role

Admin role

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes | maxLength: 300 | Name |
| `editable` | boolean | yes |  | Whether it is editable. Only false for "all administrators". |
| `usersCount` | integer | yes | minimum: 0 | Number of users belonging to this admin role |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-roledetail"></a>

#### RoleDetail

Admin role details

Extends: [Role](#schema-role) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `permissionList` | object[] | yes |  | List of permissions |
| `permissionList[].permission` | string | yes | enum: `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` | Permission type |
| `permissionList[].restricted` | boolean | yes |  | true if the managed targets are restricted |
| `permissionList[].folders` | [Folder](#schema-folder)[] | yes |  | Managed folders |
| `permissionList[].generalMasters` | [GeneralMaster](#schema-generalmaster)[] | yes |  | Managed general masters |
| `permissionList[].teams` | [Team](#schema-team)[] | yes |  | Managed teams |

<a id="schema-rolecreatebody"></a>

#### RoleCreateBody

Request body when creating an admin role

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string | yes |  | Name |
| `permissionList` | object[] | yes |  | Permission list |
| `permissionList[].permission` | string | yes | enum: `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` | Permission type |
| `permissionList[].restricted` | boolean | yes |  | true to restrict the managed targets |
| `permissionList[].folderIds` | string(uuid)[] |  |  | IDs of managed folders. The same ID cannot be specified more than once (IDs differing only in letter case are treated as the same ID). |
| `permissionList[].generalMasterIds` | string(uuid)[] |  |  | ID of the general master to manage. Duplicate IDs are not allowed (IDs differing only in letter case are treated as the same ID). |
| `permissionList[].teamIds` | string(uuid)[] |  |  | ID of the team to manage. Duplicate IDs are not allowed (IDs differing only in letter case are treated as the same ID). |

<a id="schema-roleupdatebody"></a>

#### RoleUpdateBody

Request body for updating an admin role

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `name` | string |  |  | Name |
| `permissionList` | object[] |  |  | Permission list |
| `permissionList[].permission` | string | yes | enum: `tenant`, `billing`, `integration`, `security`, `audit`, `stats`, `workflow`, `route`, `pipeline`, `workflow_misc`, `user`, `team`, `role`, `master`, `master_read`, `ticket_read`, `ticket_write`, `label`, `automation` | Permission type |
| `permissionList[].restricted` | boolean | yes |  | true to restrict the managed targets |
| `permissionList[].folderIds` | string(uuid)[] |  |  | IDs of managed folders. The same ID cannot be specified more than once (IDs differing only in letter case are treated as the same ID). |
| `permissionList[].generalMasterIds` | string(uuid)[] |  |  | ID of the general master to manage. Duplicate IDs are not allowed (IDs differing only in letter case are treated as the same ID). |
| `permissionList[].teamIds` | string(uuid)[] |  |  | ID of the team to manage. Duplicate IDs are not allowed (IDs differing only in letter case are treated as the same ID). |

<a id="schema-category"></a>

#### Category

Category

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 100 | Name |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-folder"></a>

#### Folder

Folder

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes | maxLength: 300 | Name |
| `fullName` | string | yes |  | Full name (path from the root folder) |
| `code` | string | yes | maxLength: 100 | Code |
| `description` | string\|null | yes |  | Description |
| `workflowsCount` | integer | yes | minimum: 0 | Number of workflows in the folder |
| `routesCount` | integer | yes | minimum: 0 | Number of routes in the folder |
| `pipelinesCount` | integer | yes | minimum: 0 | Number of pipelines in the folder |
| `editable` | boolean | yes |  | Whether it is editable |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-folderdetail"></a>

#### FolderDetail

Folder details

Extends: [Folder](#schema-folder) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `ancestors` | [Folder](#schema-folder)[] | yes |  | Array from the parent folder up to the root folder |
| `children` | [Folder](#schema-folder)[] | yes |  | Child folders |

<a id="schema-invitemembershipbody"></a>

#### InviteMembershipBody

Affiliation to create upon invitation

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `teamCode` | string | yes |  | Code of the team to belong to |
| `gradeCodes` | string[] |  |  | Array of grade codes |
| `leader` | boolean |  |  | Manager flag (default: false) |

<a id="schema-generalmaster"></a>

#### GeneralMaster

General master

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 300 | Name |
| `description` | string\|null | yes | maxLength: 10000 | Description |
| `defaultSortBy` | string | yes | enum: `name`, `code` | Default sort order of the item list |
| `itemsCount` | integer | yes | minimum: 0 | Number of items |
| `initialDisplayCode` | boolean | yes |  | Whether to display the code by default |
| `initialDisplayCreatedAt` | boolean | yes |  | Whether to display the creation date and time by default |
| `initialDisplayDescription` | boolean | yes |  | Whether to display the description by default |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `fields` | [GeneralMasterField](#schema-generalmasterfield)[] | yes |  | Array of custom fields |

<a id="schema-generalmasteritem"></a>

#### GeneralMasterItem

General master item

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 100 | Name |
| `description` | string\|null | yes |  | Description |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `startsOn` | string(date)\|null | yes |  | Start date of validity period |
| `endsOn` | string(date)\|null | yes |  | End date of validity period |
| `inputs` | object[] | yes |  | Array of custom field inputs |
| `inputs[].id` | string(uuid) | yes |  | UUID |
| `inputs[].value` | string\|null or string[] | yes |  | Input value |
| `inputs[].createdAt` | string(date-time) | yes |  | Created at |
| `inputs[].updatedAt` | string(date-time) | yes |  | Updated at |
| `inputs[].field` | [GeneralMasterField](#schema-generalmasterfield) | yes |  |  |

<a id="schema-generalmasterfield"></a>

#### GeneralMasterField

Custom field of the general master

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `title` | string | yes | maxLength: 300 | Field name |
| `description` | string\|null | yes | maxLength: 10000 | Field description |
| `code` | string | yes | maxLength: 100 | Field code |
| `fieldType` | string | yes | enum: `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date` | Field type |
| `required` | boolean | yes |  | Whether it is a required field |
| `fieldOrder` | integer | yes |  | Display order of the field |
| `visible` | boolean | yes |  | true if non-admins can also view it |
| `initialDisplay` | boolean | yes |  | Whether to display by default |
| `options` | string[]\|null | yes |  | Options. Only when fieldType is checkbox or pull_down. |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-workflow"></a>

#### Workflow

Workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `code` | string | yes | pattern: `^[a-zA-Z0-9_-]+$` | Code |
| `versionId` | string | yes |  | UUID of the version |
| `versionNumber` | integer | yes |  | Version number |
| `name` | string | yes |  | Name |
| `description` | string | yes |  | Description |
| `status` | string | yes | enum: `visible`, `invisible`, `deleted` | Status. visible means enabled, invisible means disabled, deleted means deleted. |
| `publicTicket` | boolean | yes | default: `false` | true if the ticket is shared with the entire tenant |
| `visibleToManager` | string | yes | enum: `none`, `direct`, `all` | Whether to add the applicant's supervisor to the viewers. none means not added, direct means only the direct supervisor, all means all supervisors. |
| `visibleToTeamMembers` | boolean | yes | default: `false` | true if members of the applicant team are added to the viewers |
| `titleDescription` | string\|null | yes |  | Description of the title |
| `ticketNumberFormat` | string\|null | yes |  | Format of the ticket number |
| `overwritable` | boolean | yes |  | true if the approver can override |
| `createdAt` | string | yes |  | Created at |
| `updatedAt` | string | yes |  | Updated at |
| `titleInputMode` | string | yes | enum: `none`, `input`, `calculate` | Title input mode |
| `titleFormula` | string\|null | yes |  | Formula for the title |
| `allowEditingOfViewers` | boolean | yes | default: `true` | true if viewers can edit |
| `commentingEnabled` | boolean | yes | default: `true` | true if posting new comments is allowed. If false, posting new comments is prohibited on all tickets under the workflow. |
| `commentingRequiredOnApproval` | boolean | yes | default: `false` | true if a comment is required when approving (including confirming a circulation). If true, approval or confirmation is not possible without a comment. |
| `commentingRequiredOnRejection` | boolean | yes | default: `false` | true if a comment is required when rejecting (sending back). If true, rejecting is not possible without a comment. |
| `commentingRequiredOnDenial` | boolean | yes | default: `false` | true if a comment is required when denying. If true, denying is not possible without a comment. |
| `author` | [User](#schema-user) or null |  |  | Author |
| `versionAuthor` | [User](#schema-user) or null |  |  | Version author |
| `folder` | [Folder](#schema-folder) |  |  | Folder |
| `categories` | [Category](#schema-category)[] |  |  | Array of categories |
| `availableToEveryone` | boolean | yes |  | true if all users can submit applications |
| `reportFormats` | string[] | yes | enum: `pdf`, `excel` | Format of the form document |
| `hiddenOnWorkflowFilterForTicket` | boolean | yes |  | true if it is not shown in the workflow filter of ticket search |
| `hiddenOnWorkflowSelectionScreen` | boolean | yes |  | true if it is not shown on the workflow selection screen |
| `approvalCancellable` | boolean | yes |  | true if canceling an approval is allowed |
| `reportFileNameFormat` | string\|null | yes |  | File name format of the form document |
| `current` | boolean | yes |  | true if it is the current version |
| `notes` | string\|null | yes |  | Administrative note |
| `versionCreatedAt` | string(date-time) | yes |  | Creation date and time of the version |
| `collectEmailOnExternalPublish` | boolean | yes |  | true if email addresses are collected when published externally |
| `notifyGuestOnCompletion` | boolean | yes |  | true if external guests are notified of the application result (completed/denied) |
| `allowCustomSteps` | boolean | yes |  | true if adding custom steps is allowed |
| `nextTicketNumberValue` | integer\|null | yes |  | Sequential number of the next ticket number to be issued. null if numbering has never been issued. |
| `nextTicketNumberValueByKey` | integer\|null | yes |  | Sequential number to be issued next in the numbering group. null if no numbering group is set. |
| `externalPublish` | [WorkflowExternalPublish](#schema-workflowexternalpublish) or null |  |  | External publication settings. null if not published externally. |
| `publishable` | boolean |  |  | true if this version can be published. Included only when retrieved via a ticket. |

<a id="schema-workflowinticket"></a>

#### WorkflowInTicket

Workflow included in the ticket. Includes section information and viewer information.

Extends: [Workflow](#schema-workflow) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `sectionList` | [SectionListItem](#schema-sectionlistitem)[] | yes |  | Array of objects representing sections and slips (line items), stored in the order displayed on the screen. |
| `ticketViewers` | [WorkflowTicketViewer](#schema-workflowticketviewer)[] | yes |  | Ticket viewers per workflow |
| `cloudSignSetting` | object or null | yes |  | CloudSign integration settings |
| `cloudSignSetting.required` | boolean | yes |  | true if attaching documents is required |
| `ticketNumberKey` | [TicketNumberKey](#schema-ticketnumberkey) or null | yes |  | Numbering group. null if not set. |
| `excelTemplate` | [ExcelTemplate](#schema-exceltemplate) or null | yes |  | Excel form template. null if not set. |
| `customNumberings` | [CustomNumbering](#schema-customnumbering)[] | yes |  | Array of custom numberings. Stored in ascending order of code. |
| `customNumberingSettings` | [CustomNumberingSetting](#schema-customnumberingsetting)[] | yes |  | Array of custom numbering settings. Stored in ascending order of display order. |

<a id="schema-workflowdetail"></a>

#### WorkflowDetail

Workflow details

Extends: [WorkflowInTicket](#schema-workflowinticket) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `routeConditions` | [WorkflowRouteCondition](#schema-workflowroutecondition)[] | yes |  | Route branch |
| `availableTeams` | [WorkflowAvailableTeam](#schema-workflowavailableteam)[] | yes |  | Array of teams that can submit applications |
| `watchers` | [WorkflowWatcher](#schema-workflowwatcher)[] | yes |  | Array of auto-watch settings |
| `workflowFilterVisibilities` | [WorkflowFilterVisibility](#schema-workflowfiltervisibility)[] | yes |  | Array of targets that can select the workflow as a filter condition. If the array is empty, everyone can select it. |
| `slackChannel` | [WorkflowSlackChannel](#schema-workflowslackchannel) or null | yes |  | Slack notification settings. null if not set. |
| `chatworkRoom` | [WorkflowChatworkRoom](#schema-workflowchatworkroom) or null | yes |  | Chatwork notification settings. null if not set. |
| `microsoftTeamsChannel` | [WorkflowMicrosoftTeamsChannel](#schema-workflowmicrosoftteamschannel) or null | yes |  | Microsoft Teams notification settings. null if not set. |
| `googleChatSetting` | [WorkflowGoogleChatSetting](#schema-workflowgooglechatsetting) or null | yes |  | Google Chat notification settings. null if not set. |
| `reportUploadGoogleDrive` | [WorkflowReportUploadGoogleDrive](#schema-workflowreportuploadgoogledrive) or null | yes |  | Google Drive upload settings for Excel form documents. null if not set. |
| `reportUploadBox` | [WorkflowReportUploadBox](#schema-workflowreportuploadbox) or null | yes |  | Box upload settings for Excel form documents. null if not set. |
| `ticketNumberingTiming` | [TicketNumberingTiming](#schema-ticketnumberingtiming) | yes |  |  |

<a id="schema-workflowexternalpublish"></a>

#### WorkflowExternalPublish

External publication settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `externalPublishHash` | string | yes |  | Hash value included in the external publication URL |

<a id="schema-ticketnumberkey"></a>

#### TicketNumberKey

Numbering group

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes |  | Code |
| `name` | string | yes |  | Name |
| `notes` | string\|null | yes |  | Description |
| `default` | boolean | yes |  | true if it is the default numbering group |

<a id="schema-exceltemplate"></a>

#### ExcelTemplate

Excel form template

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `url` | string | yes |  | Download URL of the template file |
| `restrictedExport` | boolean | yes |  | true if the ticket statuses that can be output are restricted |
| `allowedTicketStatuses` | [ExcelTemplateAllowedTicketStatus](#schema-exceltemplateallowedticketstatus)[] | yes |  | Array of ticket statuses for which output is allowed |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-exceltemplateallowedticketstatus"></a>

#### ExcelTemplateAllowedTicketStatus

Ticket statuses for which output of the Excel form template is allowed

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `ticketStatus` | string | yes | enum: `draft`, `in_progress`, `completed`, `archived`, `rejected`, `denied`, `permanently_deleted` | Ticket status for which output is allowed |
| `subStatuses` | [SubStatus](#schema-substatus)[] | yes |  | Array of substatuses for which output is allowed |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-sectionlistitem"></a>

#### SectionListItem

Slip section or form section

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `sectionType` | string | yes | enum: `form`, `slip` |  |
| `title` | string\|null | yes |  | Title |
| `description` | string\|null | yes |  | Description |
| `id` | string(uuid) | yes |  | ID of the section (UUID) |
| `code` | string |  |  | Code of the slip section. Not included for form sections. |
| `formFields` | [FormFieldDetail](#schema-formfielddetail)[] |  |  | Form fields. Not included for slip sections. |
| `conditional` | boolean |  |  | Whether there is a display condition. Not included for slip sections. |
| `combinationType` | string |  | enum: `all`, `any`, `custom` | Combination type of conditions. all=all, any=any, custom=advanced conditional expression. Not included for slip sections. |
| `combinationExpression` | string\|null |  |  | Advanced conditional expression |
| `conditionFields` | [SectionConditionField](#schema-sectionconditionfield)[] |  |  | Display condition of the form section. Not included for slip sections. |
| `slipFields` | [SlipFieldDetail](#schema-slipfielddetail)[] |  |  | Slip fields. Not included for form sections. |
| `allowApproverToAddItems` | boolean |  |  | Whether approvers can add rows to the slip section. Included only for slip sections. |
| `viewRestriction` | boolean |  |  | true if the targets who can view are restricted. Not included for slip sections. |
| `allowViewAuthor` | boolean |  |  | true if viewing by the applicant is allowed when viewRestriction is true. Not included for slip sections. |
| `allowViewApprover` | boolean |  |  | true if viewing by approvers is allowed when viewRestriction is true. Not included for slip sections. |
| `sectionViewers` | [SectionViewer](#schema-sectionviewer)[] |  |  | Targets allowed to view. Not included for slip sections. |

<a id="schema-sectionconditionfield"></a>

#### SectionConditionField

Display condition of the form section

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `symbol` | string | yes | enum: `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` | Operator |
| `value` | string\|null | yes |  | Threshold |
| `formField` | [FormField](#schema-formfield) or null | yes |  | Form field targeted by the condition |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade used as the threshold |
| `team` | [Team](#schema-team) or null | yes |  | Team used as the threshold |
| `generalMasterItem` | [GeneralMasterItem](#schema-generalmasteritem) or null | yes |  | General master item used as the threshold |

<a id="schema-formfield"></a>

#### FormField

Form field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `title` | string | yes | maxLength: 300 | Description |
| `description` | string\|null | yes |  | Description |
| `fieldType` | string | yes | enum: `text`, `text_long`, `number`, `integer`, `checkbox`, `pull_down`, `date`, `file`, `master`, `user`, `team`, `ticket`, `calculation`, `button_api`, `button_kintone`, `datetime`, `radio_button` | Field type |
| `required` | boolean | yes |  | Whether it is a required field. This is the value of the required option in the field definition. For fields whose fieldType is radio_button, it is normalized to false when approver is true, and to true otherwise. Whether the radio button type allows no selection is not determined by this value alone. Type-specific unselected checks are performed on application and when an approver updates the form, and no selection can be temporarily kept when saving as a draft. |
| `approver` | boolean | yes |  | Whether approvers can edit it |
| `author` | boolean | yes |  | Whether the applicant can edit it |
| `options` | string[]\|null | yes |  | List of options. Has a value only when the type is checkbox, pull_down, or radio_button. |
| `code` | string | yes | maxLength: 100 | Code |
| `size` | string | yes | enum: `full`, `half` | Form size. full means full width, half means half width. |
| `regexpFormat` | string\|null | yes |  | Regular expression format |
| `formula` | string\|null | yes |  | Formula.<br>Has a value only when the type is calculation. |
| `defaultValue` | string\|null | yes |  | Initial value |
| `minValue` | number\|null | yes |  | Minimum value |
| `maxValue` | number\|null | yes |  | Maximum value |
| `minLength` | integer\|null | yes | minimum: 0 | Minimum number of characters |
| `maxLength` | integer\|null | yes | minimum: 0 | Maximum number of characters |
| `decimalDigit` | integer\|null | yes | minimum: 0 | Number of decimal places |
| `delimited` | boolean\|null | yes |  | true to display with comma separators.<br>null for fields other than integer, number, and auto-calculation fields. |
| `prefix` | string\|null | yes |  | Unit (prefix) |
| `suffix` | string\|null | yes |  | Unit (suffix) |
| `hidden` | boolean\|null | yes |  | true if it is a hidden field |
| `readonlyOnUi` | boolean\|null | yes |  | When true, applicants and approvers are prohibited from entering a value on the screen.<br>Assignment via external API integration (button) and input via the REST API are not subject to this option. |
| `multiple` | boolean | yes |  | Whether multiple selection is allowed |
| `orientation` | string | yes | enum: `vertical`, `horizontal` | Layout direction of options such as checkboxes.<br>vertical=arranged vertically, horizontal=arranged horizontally with wrapping.<br>A value is always returned even for field types without options (default is vertical). |
| `allowedExtensions` | string[]\|null | yes |  | List of attachable file extensions. Has a value only when fieldType is file.<br>May return null for older fields. |
| `autoLink` | boolean | yes |  | For ticket-type fields, whether the selected ticket is automatically made a related ticket |
| `useTodayForDefaultValue` | boolean | yes |  | For date-type fields, whether to use the current date as the initial value |
| `allowedTicketStatus` | string[] | yes | enum: `draft`, `in_progress`, `completed`, `rejected`, `archived`, `denied` | For ticket-type fields, the statuses of tickets that can be selected |

<a id="schema-formfielddetail"></a>

#### FormFieldDetail

Form field details

Extends: [FormField](#schema-formfield) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `generalMaster` | [GeneralMaster](#schema-generalmaster) or null | yes |  | General master (for general master fields) |
| `defaultGeneralMasterItem` | [GeneralMasterItem](#schema-generalmasteritem) or null | yes |  | Initial value (for general master fields) |
| `externalApiSetting` | [ExternalApiSetting](#schema-externalapisetting) or null | yes |  | External API settings. Has a value only when fieldType is button_api. |
| `kintoneAppSetting` | [KintoneAppSetting](#schema-kintoneappsetting) or null | yes |  | External API settings. Has a value only when fieldType is button_kintone. |
| `climberCloudSetting` | [ClimberCloudSetting](#schema-climbercloudsetting) or null |  |  | ClimberCloud integration settings. Has a value only when fieldType is file. |
| `generalMasterSearchFilters` | [GeneralMasterSearchFilter](#schema-generalmastersearchfilter)[]\|null | yes |  | Auto-filtering settings for general master type fields |
| `approverEditRestriction` | [FormFieldApproverEditRestriction](#schema-formfieldapprovereditrestriction) or null | yes |  | Input restrictions by approvers. null if not set. |
| `approverEditSetting` | [FormFieldApproverEditSetting](#schema-formfieldapprovereditsetting) |  |  | Settings for approvers who can edit. If not set, permissionType=all is used. |
| `customValidations` | [FormCustomValidation](#schema-formcustomvalidation)[] | yes |  | Custom validation |
| `ticketCopySetting` | [FormFieldTicketCopySetting](#schema-formfieldticketcopysetting) or null | yes |  | Copy settings for ticket-type fields. null if not set. |
| `selectableTicketWorkflows` | [Workflow](#schema-workflow)[] | yes |  | Workflows that can be selected in a ticket-type field.<br>Each element does not include author / versionAuthor / folder / categories. |

<a id="schema-externalapisetting"></a>

#### ExternalApiSetting

External API settings

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `httpMethod` | string | yes | enum: `get`, `post`, `put`, `delete`, `patch` | HTTP method |
| `url` | string(uri) | yes |  | URL |
| `body` | string\|null | yes |  | Request body |
| `responseArray` | boolean | yes |  | true if the response contains multiple records |
| `arrayJsonPath` | string\|null | yes |  | JSONPath to the array when multiple records are contained |
| `mappings` | object[] | yes |  | Mapping settings to fields |
| `mappings[].formField` | [FormField](#schema-formfield) | yes |  |  |
| `mappings[].jsonPath` | string | yes |  | JSONPath for extracting values |
| `mappings[].displayInTable` | boolean | yes |  | true to display in the selection table |
| `mappings[].title` | string\|null | yes |  | Title in the selection table |

<a id="schema-climbercloudsetting"></a>

#### ClimberCloudSetting

ClimberCloud integration settings

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `tenantId` | string(uuid) | yes |  | Tenant ID |
| `contentsId` | string | yes |  | List ID with files |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `formField` | [FormField](#schema-formfield) | yes |  |  |
| `mappings` | object[] | yes |  | Mapping settings to ClimberCloud columns |
| `mappings[].formField` | [FormField](#schema-formfield) | yes |  |  |
| `mappings[].order` | integer | yes |  | Display order (starts from 1) |

<a id="schema-slipfield"></a>

#### SlipField

Slip field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `fieldType` | string | yes | enum: `text`, `number`, `integer`, `calculation`, `pull_down`, `checkbox`, `date`, `file`, `master`, `user`, `team`, `ticket`, `datetime`, `radio_button` | Field type |
| `code` | string | yes |  | Field code |
| `title` | string | yes |  | Title |
| `required` | boolean | yes |  | true if input is required. This is the value of the required option in the field definition. For fields whose fieldType is radio_button, it is normalized to false when approver is true, and to true otherwise. Whether the radio button type allows no selection is not determined by this value alone. Type-specific unselected checks are performed on application and when an approver updates the form, and no selection can be temporarily kept when saving as a draft. |
| `showTotal` | boolean | yes |  | true to display the column total |
| `options` | string[] | yes |  | Options. Has a value only for pull-down, checkbox, or radio button types. |
| `regexpFormat` | string\|null | yes |  | Regular expression format |
| `formula` | string\|null | yes |  | Formula. Has a value only when the type is auto-calculation. |
| `maxValue` | number\|null | yes |  | Maximum value |
| `minValue` | number\|null | yes |  | Minimum value |
| `defaultValue` | string\|null | yes |  | Initial value |
| `decimalDigit` | integer\|null | yes |  | Number of decimal places |
| `delimited` | boolean\|null | yes |  | true to display with comma separators.<br>null for fields other than integer, number, and auto-calculation fields. |
| `allowedExtensions` | string[] | yes |  | List of attachable file extensions |
| `prefix` | string\|null | yes |  | Unit (prefix) |
| `suffix` | string\|null | yes |  | Unit (suffix) |
| `approver` | boolean | yes |  | Whether approvers can edit it |
| `author` | boolean | yes |  | Whether the applicant can edit it |
| `hidden` | boolean\|null | yes |  | true if it is a hidden field |
| `readonlyOnUi` | boolean\|null | yes |  | When true, applicants and approvers are prohibited from entering a value on the screen.<br>Assignment via external API integration (button) and input via the REST API are not subject to this option. |
| `minLength` | integer\|null | yes | minimum: 0 | Minimum number of characters |
| `maxLength` | integer\|null | yes | minimum: 0 | Maximum number of characters |
| `multiple` | boolean | yes |  | Whether multiple selection is allowed |
| `autoLink` | boolean | yes |  | For ticket-type fields, whether the selected ticket is automatically made a related ticket |
| `useTodayForDefaultValue` | boolean | yes |  | For date-type fields, whether to use the current date as the initial value |
| `allowedTicketStatus` | string[] | yes | enum: `draft`, `in_progress`, `completed`, `rejected`, `archived`, `denied` | For ticket-type fields, the statuses of tickets that can be selected |

<a id="schema-slipfielddetail"></a>

#### SlipFieldDetail

Slip field details

Extends: [SlipField](#schema-slipfield) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `generalMaster` | [GeneralMaster](#schema-generalmaster) or null | yes |  | General master. Has a value only when the type is general master. |
| `defaultGeneralMasterItem` | [GeneralMasterItem](#schema-generalmasteritem) or null | yes |  | Initial value of the general master item |
| `generalMasterSearchFilters` | [SlipFieldGeneralMasterSearchFilter](#schema-slipfieldgeneralmastersearchfilter)[] | yes |  | Auto-filtering settings for general master type fields. Stored in ascending order of display order. |
| `approverEditRestriction` | [SlipFieldApproverEditRestriction](#schema-slipfieldapprovereditrestriction) or null | yes |  | Input restrictions by approvers. null if not set. |
| `approverEditSetting` | [SlipFieldApproverEditSetting](#schema-slipfieldapprovereditsetting) |  |  | Settings for approvers who can edit. If not set, permissionType=all is used. |
| `customValidations` | [SlipCustomValidation](#schema-slipcustomvalidation)[] | yes |  | Custom validation |
| `selectableTicketWorkflows` | [Workflow](#schema-workflow)[] | yes |  | Workflows that can be selected in a ticket-type field.<br>Each element does not include author / versionAuthor / folder / categories. |

<a id="schema-slipfieldapprovereditrestriction"></a>

#### SlipFieldApproverEditRestriction

Input restrictions by approvers for slip fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `routeStepCodes` | string[] | yes |  | Array of codes of route steps for which input is required |

<a id="schema-slipfieldapprovereditsetting"></a>

#### SlipFieldApproverEditSetting

Settings for approvers who can edit the slip field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `permissionType` | string | yes | enum: `all`, `user`, `team_grade` | Scope of allowed editing.<br>all=all approvers, user=specified users, team_grade=specified teams and grades. |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |
| `gradeSymbol` | string\|null | yes | enum: `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `null` | Comparison condition for grades. Has a value only when permissionType is team_grade and a grade is specified. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when permissionType is team_grade. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Has a value only when permissionType is team_grade. |
| `users` | [User](#schema-user)[] | yes |  | Array of users. Elements are present only when permissionType is user. |

<a id="schema-slipfieldgeneralmastersearchfilter"></a>

#### SlipFieldGeneralMasterSearchFilter

Auto-filtering settings for the general master of a slip field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `filterSlipFieldId` | string(uuid) | yes |  | ID (UUID) of the slip field used for filtering |
| `fieldType` | string | yes | enum: `free_word`, `name`, `code`, `description`, `custom_field` | Type of the field to be filtered |
| `generalMasterFieldId` | string(uuid)\|null | yes |  | When fieldType=custom_field, ID (UUID) of the custom field of the general master to be filtered |

<a id="schema-slipcustomvalidation"></a>

#### SlipCustomValidation

Custom validation for slip fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `combinationType` | string | yes | enum: `all`, `any` | How to combine conditions. all=all conditions are met, any=any condition is met. |
| `errorMessage` | string | yes |  | Error message to display when the conditions are not met |
| `fields` | [SlipCustomValidationField](#schema-slipcustomvalidationfield)[] | yes |  | Array of conditions. Stored in ascending order of display order. |

<a id="schema-slipcustomvalidationfield"></a>

#### SlipCustomValidationField

Condition of the custom validation for slip fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `symbol` | string | yes | enum: `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` | Comparison condition |
| `value` | string\|null | yes |  | Value to compare |
| `slipField` | [SlipField](#schema-slipfield) | yes |  | Slip field targeted by the condition |
| `generalMasterItem` | [CustomValidationGeneralMasterItem](#schema-customvalidationgeneralmasteritem) or null | yes |  | General master item to compare. Has a value only when the target is a general master type field. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade to compare |
| `team` | [Team](#schema-team) or null | yes |  | Team to compare |

<a id="schema-slipfieldinticket"></a>

#### SlipFieldInTicket

Slip field included in the slip item input of the ticket

Extends: [SlipField](#schema-slipfield) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `generalMaster` | [FieldGeneralMaster](#schema-fieldgeneralmaster) or null | yes |  | General master. Has a value only when the type is general master. |

<a id="schema-fieldgeneralmaster"></a>

#### FieldGeneralMaster

General master linked to form fields and slip fields.<br>Because the ticket response does not return the array of custom fields (fields),<br>it is defined as a component separate from the GeneralMaster returned by the general master API.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 300 | Name |
| `description` | string\|null | yes | maxLength: 10000 | Description |
| `defaultSortBy` | string | yes | enum: `name`, `code` | Default sort order of the item list |
| `itemsCount` | integer | yes | minimum: 0 | Number of items |
| `initialDisplayCode` | boolean | yes |  | Whether to display the code by default |
| `initialDisplayCreatedAt` | boolean | yes |  | Whether to display the creation date and time by default |
| `initialDisplayDescription` | boolean | yes |  | Whether to display the description by default |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-formfieldapprovereditrestriction"></a>

#### FormFieldApproverEditRestriction

Input restrictions by approvers for form fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `routeStepCodes` | string[] | yes |  | Array of codes of route steps for which input is required |

<a id="schema-formfieldapprovereditsetting"></a>

#### FormFieldApproverEditSetting

Settings for approvers who can edit the form field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `permissionType` | string | yes | enum: `all`, `user`, `team_grade` | Scope of allowed editing.<br>all=all approvers, user=specified users, team_grade=specified teams and grades. |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |
| `gradeSymbol` | string\|null | yes | enum: `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `null` | Comparison condition for grades. Has a value only when permissionType is team_grade and a grade is specified. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when permissionType is team_grade. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Has a value only when permissionType is team_grade. |
| `users` | [User](#schema-user)[] | yes |  | Array of users. Elements are present only when permissionType is user. |

<a id="schema-formcustomvalidation"></a>

#### FormCustomValidation

Custom validation for form fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `combinationType` | string | yes | enum: `all`, `any` | How to combine conditions. all=all conditions are met, any=any condition is met. |
| `errorMessage` | string | yes |  | Error message to display when the conditions are not met |
| `fields` | [FormCustomValidationField](#schema-formcustomvalidationfield)[] | yes |  | Array of conditions. Stored in ascending order of display order. |

<a id="schema-formcustomvalidationfield"></a>

#### FormCustomValidationField

Condition of the custom validation for form fields

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `symbol` | string | yes | enum: `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` | Comparison condition |
| `value` | string\|null | yes |  | Value to compare |
| `formField` | [FormField](#schema-formfield) | yes |  | Form field targeted by the condition |
| `generalMasterItem` | [CustomValidationGeneralMasterItem](#schema-customvalidationgeneralmasteritem) or null | yes |  | General master item to compare. Has a value only when the target is a general master type field. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade to compare |
| `team` | [Team](#schema-team) or null | yes |  | Team to compare |

<a id="schema-formfieldticketcopysetting"></a>

#### FormFieldTicketCopySetting

Settings for copying the value of the selected ticket in a ticket-type field

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `workflow` | [Workflow](#schema-workflow) or null | yes |  | Workflow of the copy source |
| `mappings` | object[] | yes |  | Array of items to copy. Stored in ascending order of display order. |
| `mappings[].fieldType` | string | yes | enum: `ticket_title`, `ticket_number`, `form_field` | Type of the copy source. ticket_title=ticket title, ticket_number=ticket number, form_field=form field. |
| `mappings[].fieldCode` | string\|null | yes |  | Code of the form field of the copy source. Has a value only when fieldType is form_field. |
| `mappings[].targetFieldCode` | string\|null | yes |  | Code of the form field of the copy destination |

<a id="schema-formfieldinticket"></a>

#### FormFieldInTicket

Form field included in the input values of the ticket

Extends: [FormField](#schema-formfield) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `generalMaster` | [FieldGeneralMaster](#schema-fieldgeneralmaster) or null | yes |  | General master. Has a value only when the type is general master. |

<a id="schema-sectionviewer"></a>

#### SectionViewer

Targets allowed to view the form section

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |
| `user` | [User](#schema-user) or null | yes |  | User. Only one of user and team has a value. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Only one of user and team has a value. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when a team is specified and a grade is also specified. |

<a id="schema-customvalidationgeneralmasteritem"></a>

#### CustomValidationGeneralMasterItem

General master item compared in a custom validation condition.<br>Because the custom validation response does not return the array of custom field inputs (inputs),<br>it is defined as a component separate from the GeneralMasterItem returned by the general master API.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 100 | Name |
| `description` | string\|null | yes |  | Description |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `startsOn` | string(date)\|null | yes |  | Start date of validity period |
| `endsOn` | string(date)\|null | yes |  | End date of validity period |

<a id="schema-workflowroutecondition"></a>

#### WorkflowRouteCondition

Route branch of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `conditionType` | string | yes | enum: `always`, `field`, `field_otherwise` | Route branch type |
| `combinationType` | string | yes | enum: `all`, `any`, `custom` | Combination type of conditions |
| `combinationExpression` | string | yes |  | Advanced conditional expression |
| `route` | [Route](#schema-route) or null | yes |  | Route. Only one of route and errorMessage has a value. |
| `conditionFields` | [WorkflowRouteConditionField](#schema-workflowrouteconditionfield)[] | yes |  | Condition |
| `errorMessage` | string\|null | yes |  | Error message when the application is refused. Only one of route and errorMessage has a value. |

<a id="schema-workflowrouteconditionfield"></a>

#### WorkflowRouteConditionField

Condition for a workflow route branch

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `symbol` | string | yes | enum: `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal` | Operator |
| `value` | string\|null | yes |  | Threshold |
| `formField` | [FormField](#schema-formfield) | yes |  | Target form field |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade used as the threshold |
| `team` | [Team](#schema-team) or null | yes |  | Team used as the threshold |
| `generalMasterItem` | [GeneralMasterItem](#schema-generalmasteritem) or null | yes |  | General master item used as the threshold |

<a id="schema-workflowticketviewer"></a>

#### WorkflowTicketViewer

Viewers configured at the workflow level

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `user` | [User](#schema-user) or null | yes |  | User. Only one of user and team has a value. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Only one of user and team has a value. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when a team is specified and a grade is also specified. |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |

<a id="schema-workflowavailableteam"></a>

#### WorkflowAvailableTeam

Teams that can submit applications with the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `descendants` | boolean | yes |  | Whether to include lower-level teams |
| `gradeSymbol` | string\|null | yes | enum: `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `any_of`, `null` | Grade condition. equal means the same grade as the specified grade, greater_than means higher than the specified grade, greater_than_or_equal means the specified grade or higher, less_than means lower than the specified grade, less_than_or_equal means the specified grade or lower, and any_of means any of the specified grades. null if grade is not used as a condition. |
| `team` | [Team](#schema-team) | yes |  |  |
| `grades` | [Grade](#schema-grade)[] | yes |  | Array of grades, sorted in descending order of grade level. An empty array if gradeSymbol is null. |

<a id="schema-workflowwatcher"></a>

#### WorkflowWatcher

Auto-watch settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `watcherType` | string | yes | enum: `user`, `team` | Watcher type. user means a user is specified, team means a team is specified. |
| `user` | [User](#schema-user) or null | yes |  | User. Only one of user and team has a value. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Only one of user and team has a value. |

<a id="schema-workflowfiltervisibility"></a>

#### WorkflowFilterVisibility

Targets that can be selected as workflow filter conditions

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `user` | [User](#schema-user) or null | yes |  | User. Only one of user and team has a value. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Only one of user and team has a value. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when a team is specified and a grade is also specified. |

<a id="schema-workflowslackchannel"></a>

#### WorkflowSlackChannel

Slack notification settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | ID of the Slack channel to notify |
| `notifyOpened` | boolean | yes |  | true to notify when a ticket is submitted |
| `notifyApproved` | boolean | yes |  | true to notify when a ticket is approved |
| `notifyRejected` | boolean | yes |  | true to notify when a ticket is rejected (sent back) |
| `notifyCompleted` | boolean | yes |  | true to notify when a ticket is completed |
| `notifyArchived` | boolean | yes |  | true to notify when a ticket is archived |
| `notifyWithdrawn` | boolean | yes |  | true to notify when a ticket is withdrawn |
| `notifyCommented` | boolean | yes |  | true to notify when a comment is posted on a ticket |
| `notifyConfirmed` | boolean | yes |  | true to notify when a ticket is confirmed |
| `notifyDenied` | boolean | yes |  | true to notify when a ticket is denied |
| `notifyApprovalCanceled` | boolean | yes |  | true to notify when approval of a ticket is canceled |
| `notifyConfirmCanceled` | boolean | yes |  | true to notify when confirmation of a ticket is canceled |
| `slackAccessToken` | [SlackAccessToken](#schema-slackaccesstoken) | yes |  |  |

<a id="schema-slackaccesstoken"></a>

#### SlackAccessToken

Access token for the Slack integration

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `slackTeamId` | string\|null | yes |  | ID of the Slack workspace. null if integrated across the entire Enterprise Grid. |
| `slackTeamName` | string\|null | yes |  | Name of the Slack workspace. null if integrated across the entire Enterprise Grid. |
| `slackEnterpriseId` | string\|null | yes |  | ID of the Slack Enterprise Grid. null if integrated per workspace. |
| `slackEnterpriseName` | string\|null | yes |  | Name of the Slack Enterprise Grid. null if integrated per workspace. |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-workflowchatworkroom"></a>

#### WorkflowChatworkRoom

Chatwork notification settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `roomId` | string | yes |  | ID of the Chatwork room to notify |
| `name` | string | yes |  | Name of the Chatwork room to notify |
| `notifyOpened` | boolean | yes |  | true to notify when a ticket is submitted |
| `notifyApproved` | boolean | yes |  | true to notify when a ticket is approved |
| `notifyRejected` | boolean | yes |  | true to notify when a ticket is rejected (sent back) |
| `notifyCompleted` | boolean | yes |  | true to notify when a ticket is completed |
| `notifyArchived` | boolean | yes |  | true to notify when a ticket is archived |
| `notifyWithdrawn` | boolean | yes |  | true to notify when a ticket is withdrawn |
| `notifyCommented` | boolean | yes |  | true to notify when a comment is posted on a ticket |
| `notifyConfirmed` | boolean | yes |  | true to notify when a ticket is confirmed |
| `notifyDenied` | boolean | yes |  | true to notify when a ticket is denied |
| `notifyApprovalCanceled` | boolean | yes |  | true to notify when approval of a ticket is canceled |
| `notifyConfirmCanceled` | boolean | yes |  | true to notify when confirmation of a ticket is canceled |

<a id="schema-workflowmicrosoftteamschannel"></a>

#### WorkflowMicrosoftTeamsChannel

Microsoft Teams notification settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `webhookUrl` | string | yes |  | Webhook URL of the Microsoft Teams channel to notify |
| `notifyOpened` | boolean | yes |  | true to notify when a ticket is submitted |
| `notifyApproved` | boolean | yes |  | true to notify when a ticket is approved |
| `notifyRejected` | boolean | yes |  | true to notify when a ticket is rejected (sent back) |
| `notifyCompleted` | boolean | yes |  | true to notify when a ticket is completed |
| `notifyArchived` | boolean | yes |  | true to notify when a ticket is archived |
| `notifyWithdrawn` | boolean | yes |  | true to notify when a ticket is withdrawn |
| `notifyCommented` | boolean | yes |  | true to notify when a comment is posted on a ticket |
| `notifyConfirmed` | boolean | yes |  | true to notify when a ticket is confirmed |
| `notifyDenied` | boolean | yes |  | true to notify when a ticket is denied |
| `notifyApprovalCanceled` | boolean | yes |  | true to notify when approval of a ticket is canceled |
| `notifyConfirmCanceled` | boolean | yes |  | true to notify when confirmation of a ticket is canceled |

<a id="schema-workflowgooglechatsetting"></a>

#### WorkflowGoogleChatSetting

Google Chat notification settings of the workflow

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `webhookUrl` | string | yes |  | Webhook URL of the Google Chat space to notify |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `notifyApproved` | boolean | yes |  | true to notify when a ticket is approved |
| `notifyArchived` | boolean | yes |  | true to notify when a ticket is archived |
| `notifyCommented` | boolean | yes |  | true to notify when a comment is posted on a ticket |
| `notifyCompleted` | boolean | yes |  | true to notify when a ticket is completed |
| `notifyConfirmed` | boolean | yes |  | true to notify when a ticket is confirmed |
| `notifyDenied` | boolean | yes |  | true to notify when a ticket is denied |
| `notifyOpened` | boolean | yes |  | true to notify when a ticket is submitted |
| `notifyRejected` | boolean | yes |  | true to notify when a ticket is rejected (sent back) |
| `notifyWithdrawn` | boolean | yes |  | true to notify when a ticket is withdrawn |
| `notifyApprovalCanceled` | boolean | yes |  | true to notify when approval of a ticket is canceled |
| `notifyConfirmCanceled` | boolean | yes |  | true to notify when confirmation of a ticket is canceled |

<a id="schema-workflowreportuploadgoogledrive"></a>

#### WorkflowReportUploadGoogleDrive

Google Drive upload settings for the workflow's Excel forms

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `googleDriveFolderId` | string | yes |  | ID of the Google Drive folder to upload to |

<a id="schema-workflowreportuploadbox"></a>

#### WorkflowReportUploadBox

Box upload settings for the workflow's Excel forms

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `boxFolderId` | string | yes |  | ID of the Box folder to upload to |

<a id="schema-ticketnumberingtiming"></a>

#### TicketNumberingTiming

Timing at which the ticket number is assigned

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid)\|null | yes |  | UUID. null if the numbering timing is not configured and the default value is returned. |
| `timingType` | string | yes | enum: `created`, `opened`, `completed`, `sub_status_attached` | Timing of numbering. created means when the ticket is created, opened means when it is submitted, completed means when it is completed, and sub_status_attached means when a sub-status is attached. |
| `subStatus` | [SubStatus](#schema-substatus) or null | yes |  | Sub-status used for numbering. null if timingType is not sub_status_attached. |

<a id="schema-route"></a>

#### Route

Route

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `code` | string | yes |  | Code |
| `status` | string | yes | enum: `visible`, `deleted`, `error` | Status |
| `versionId` | string | yes |  | ID of the version |
| `versionNumber` | integer | yes |  | Version number |
| `name` | string | yes |  | Name |
| `description` | string | yes |  | Description |
| `notes` | string\|null | yes |  | Administrative note |
| `current` | boolean | yes |  | Whether this is the current version |
| `createdAt` | string | yes |  | Created at |
| `updatedAt` | string | yes |  | Updated at |
| `versionCreatedAt` | string(date-time) | yes |  | Creation date and time of the version |
| `author` | [User](#schema-user) or null | yes |  | Author |
| `versionAuthor` | [User](#schema-user) or null |  |  |  |
| `folder` | [Folder](#schema-folder) | yes |  | Folder |

<a id="schema-routedetail"></a>

#### RouteDetail

Detailed information of the route

Extends: [Route](#schema-route) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `versionAuthor` | [User](#schema-user) or null | yes |  | Creator of the version |
| `steps` | [RouteStep](#schema-routestep)[] | yes |  | Route step |

<a id="schema-routestep"></a>

#### RouteStep

Route step

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `stepOrder` | integer | yes |  | Step order (starts from 1) |
| `stepType` | string | yes | enum: `author`, `manager`, `team`, `user`, `author_customizable`, `assignee_customizable`, `dynamic_team`, `dynamic_user` | Type of the step |
| `title` | string | yes |  | Title |
| `actionType` | string | yes | enum: `approve`, `confirm`, `none` | Action type. approve for approve/reject (send back), confirm for circulation (with confirmation), and none for circulation (without confirmation). |
| `instruction` | string\|null | yes |  | Instructions for approvers |
| `requiredApprovalsNumber` | integer | yes |  | Number of approvers required |
| `requiredApprovalsPercent` | integer | yes |  | Approval ratio required (%) |
| `fallbackType` | string or null | yes | enum: `direct_manager`, `higher_manager`, `skip`, `no_fallback`, `higher_team` | Type of fallback |
| `allowSelfApproval` | boolean | yes |  | Whether self-approval is allowed |
| `managerSelectionType` | string | yes | enum: `closest`, `all` | How to select superiors. closest targets only the direct superior, all targets all superiors. |
| `allowRaising` | boolean | yes |  | Whether to allow approvers of this step to be escalated |
| `minCustomAssignees` | integer\|null | yes | minimum: 0 | Minimum number of nominees. Can be set only for the "nominated by applicant" step. |
| `approverAssignmentInstruction` | string\|null | yes |  | How approvers are selected |
| `users` | [User](#schema-user)[] | yes |  | Array of users used to specify approvers |
| `targets` | [RouteStepTarget](#schema-routesteptarget)[] | yes |  | Team and grade conditions used to specify approvers |
| `routeStepCondition` | [RouteStepCondition](#schema-routestepcondition) or null | yes |  |  |
| `code` | string | yes |  | Code |
| `subStatus` | [SubStatus](#schema-substatus) or null | yes |  | Sub-status set at this step |

<a id="schema-routesteptarget"></a>

#### RouteStepTarget

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `team` | [Team](#schema-team) or null | yes |  | Team used to specify approvers. null for steps that do not specify a team. |
| `descendants` | boolean | yes |  | When stepType=author_customizable or stepType=assignee_customizable, whether members of teams below the specified team are also included as approver candidates (true: include, false: do not include) |
| `gradeSymbol` | string or null | yes | enum: `equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `any_of` | Grade comparison condition. Set only when a grade is specified. |
| `grades` | [Grade](#schema-grade)[] | yes |  | Array of grades used to specify approvers |
| `variable` | string\|null | yes |  | Variable name specified when the approver type is "dynamically specify team" or "dynamically specify user". |

<a id="schema-routestepcondition"></a>

#### RouteStepCondition

Execution condition that can be set for each step

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `conditionType` | string | yes | enum: `always`, `conditional`, `conditional_skip` | Execution type |
| `combinationType` | string | yes | enum: `all`, `any` | Combination type of conditions |
| `routeStepConditionFields` | [RouteStepConditionField](#schema-routestepconditionfield)[] | yes |  |  |

<a id="schema-routestepconditionfield"></a>

#### RouteStepConditionField

Details of the execution condition that can be set for each step

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `variable` | string\|null | yes |  | Variable. null if fieldKey is author_grade / author_team. |
| `fieldKey` | string | yes | enum: `author_grade`, `author_team`, `text_variable`, `number_variable`, `checkbox_variable`, `general_master_variable`, `other_variable` | Field of the variable |
| `symbol` | string | yes | enum: `equal`, `not_equal`, `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal`, `not_descendants_or_equal` | Operator |
| `value` | string\|null | yes |  | Threshold. null if fieldKey is author_grade / author_team / general_master_variable, or if symbol is is_empty / is_not_empty. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade used as the threshold |
| `team` | [Team](#schema-team) or null | yes |  | Team used as the threshold |
| `generalMaster` | [RouteStepConditionGeneralMaster](#schema-routestepconditiongeneralmaster) or null | yes |  | General master to which the general master item used as the threshold belongs |
| `generalMasterItem` | [GeneralMasterItem](#schema-generalmasteritem) or null | yes |  | General master item used as the threshold |

<a id="schema-routestepconditiongeneralmaster"></a>

#### RouteStepConditionGeneralMaster

General master to which the general master item used as the threshold of the execution condition belongs.<br>The route response does not return the array of custom fields (fields),<br>so this is defined as a component separate from the GeneralMaster returned by the general master API.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes | maxLength: 100 | Code |
| `name` | string | yes | maxLength: 300 | Name |
| `description` | string\|null | yes | maxLength: 10000 | Description |
| `defaultSortBy` | string | yes | enum: `name`, `code` | Default sort order of the item list |
| `itemsCount` | integer | yes | minimum: 0 | Number of items |
| `initialDisplayCode` | boolean | yes |  | Whether to display the code by default |
| `initialDisplayCreatedAt` | boolean | yes |  | Whether to display the creation date and time by default |
| `initialDisplayDescription` | boolean | yes |  | Whether to display the description by default |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-ticketsummary"></a>

#### TicketSummary

Basic information of the ticket. Does not include relations such as the applicant, affiliated team, and labels.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `ticketNumber` | string\|null | yes |  | Ticket number |
| `title` | string\|null | yes |  | Title |
| `status` | string | yes | enum: `draft`, `in_progress`, `completed`, `rejected`, `archived`, `denied`, `permanently_deleted` | Status |
| `currentStep` | number | yes | minimum: 0 | Current step. 0 is the creator, 1 is the first approval step. |
| `createdAt` | string(date-time) | yes |  | Created at |
| `openedAt` | string(date-time)\|null | yes |  | Date and time of application |
| `completedAt` | string(date-time)\|null | yes |  | Date and time of completion |
| `archivedAt` | string(date-time)\|null | yes |  | Date and time of archiving |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `publicStatus` | boolean | yes |  | true if the ticket is shared with the entire tenant |
| `forcedPublicType` | string | yes | enum: `follow_workflow`, `forced_public`, `forced_private` | Override setting for the sharing scope of the ticket |
| `workflow` | [Workflow](#schema-workflow) or [WorkflowInTicket](#schema-workflowinticket) | yes |  | Workflow information of this ticket. Only when a single ticket is retrieved does this contain more detailed workflow information, including sections and viewers. |
| `authorTeamId` | string(uuid)\|null | yes |  | ID of the applicant's affiliated team. null for external guests or when there is no affiliated team. |
| `authorTeamSnapshot` | object\|null | yes |  | Information on the applicant's affiliated team at the time of application. null for external guests or when there is no affiliated team. |
| `authorTeamSnapshot.id` | string(uuid) | yes |  | UUID |
| `authorTeamSnapshot.name` | string | yes |  | Team name |
| `authorTeamSnapshot.fullName` | string | yes |  | Team name including parent teams |
| `authorTeamSnapshot.code` | string\|null | yes |  | Team code |
| `commentsCount` | integer | yes |  | Number of comments |
| `logsCount` | integer | yes |  | Number of logs |
| `linkedTicketsCount` | integer | yes |  | Number of related tickets |
| `closedAt` | string(date-time)\|null | yes |  | Date and time of closing. Contains the completion date and time or the archive date and time. null if neither exists. |
| `deniedAt` | string(date-time)\|null | yes |  | Date and time the application was refused |
| `useRouteStepCondition` | boolean | yes |  | true to build the steps by evaluating the conditions of in-route branches |

<a id="schema-ticket"></a>

#### Ticket

Ticket

Extends: [TicketSummary](#schema-ticketsummary) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `author` | [User](#schema-user) or null | yes |  | Applicant. For a proxy application, this is the proxy applicant. null for external guests. |
| `authorTeam` | [Team](#schema-team) or null | yes |  | Applicant's affiliated team. null for external guests or when there is no affiliated team. |
| `proxyClientUser` | [User](#schema-user) or null | yes |  | User who requested the proxy application. Set only for proxy applications. |
| `subStatus` | [SubStatus](#schema-substatus) or null | yes |  | Sub-status. Set only while in processing. |
| `labels` | [Label](#schema-label)[] | yes |  | Array of labels of the ticket |

<a id="schema-ticketwithstep"></a>

#### TicketWithStep

Extends: [Ticket](#schema-ticket) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `steps` | [TicketStep](#schema-ticketstep)[] | yes |  | Array of steps |
| `ticketGuest` | [TicketGuest](#schema-ticketguest) or null | yes |  | Applicant of an external guest application. null if not an external guest application. |
| `triggerTicket` | [TicketSummary](#schema-ticketsummary) or null | yes |  | Source ticket (set only when created by a pipeline). Does not include relations such as the applicant and labels. |

<a id="schema-ticketdetail"></a>

#### TicketDetail

Details of the ticket

Extends: [Ticket](#schema-ticket) (all base fields are included).

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `route` | [RouteDetail](#schema-routedetail) or null | yes |  | Approval route of this ticket. null if the application is refused. |
| `ticketGuest` | [TicketGuest](#schema-ticketguest) or null | yes |  | Applicant of an external guest application. null if not an external guest application. |
| `triggerTicket` | [Ticket](#schema-ticket) or null | yes |  | Source ticket (set only when created by a pipeline) |
| `nextTickets` | [Ticket](#schema-ticket)[] | yes |  | Next ticket (set only when the next ticket was created by a pipeline) |
| `slipItems` | [SlipItem](#schema-slipitem)[] | yes |  | Input of slips (line items) |
| `ticketSections` | [TicketSection](#schema-ticketsection)[] | yes |  | Array of sections |
| `inputs` | [TicketInput](#schema-ticketinput)[] | yes |  | Form input |
| `cloudSignDocument` | null or object | yes |  | Attached CloudSign document |
| `cloudSignDocument.id` | string(uuid) | yes |  | UUID |
| `cloudSignDocument.documentId` | string | yes |  | CloudSign document ID |
| `cloudSignDocument.documentTitle` | string | yes |  | CloudSign document title |
| `cloudSignDocument.status` | string | yes | enum: `draft`, `in_progress`, `rejected`, `template`, `imported`, `completed` | Status of the CloudSign document |
| `cloudSignDocument.sandbox` | boolean | yes |  | true if CloudSign is a sandbox environment |
| `cloudSignDocument.createdAt` | string(date-time) | yes |  | Created at |
| `cloudSignDocument.updatedAt` | string(date-time) | yes |  | Updated at |
| `cloudSignDocument.url` | string(uri) | yes |  | URL of the CloudSign document |
| `steps` | [TicketStep](#schema-ticketstep)[] | yes |  | Step of the ticket |
| `customNumberingValues` | [CustomNumberingValue](#schema-customnumberingvalue)[] | yes |  | Array of custom numbering values assigned to the ticket |

<a id="schema-ticketformpartialupdatebody"></a>

#### TicketFormPartialUpdateBody

Request body of `PATCH /tickets/{ticketId}/form`.<br>Specify at least one of `inputs` and `slipItems`. If both are unspecified or both are empty arrays, `400` is returned.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `inputs` | [TicketFormPartialUpdateInput](#schema-ticketformpartialupdateinput)[] |  |  | Array of form inputs to update. Specify only the form fields to be updated incrementally.<br>Unspecified form fields retain their existing values.<br>The same form field cannot be specified in multiple elements (`400` on duplicates). |
| `slipItems` | [TicketFormPartialUpdateSlipItem](#schema-ticketformpartialupdateslipitem)[] |  |  | Array of slip items (line items) to update. Each element represents one row of a slip.<br>By specifying multiple elements with the same `slipSectionId` / `slipSectionCode`, you can submit multiple rows to one slip section.<br>The update rules (adding and deleting rows, etc.) for explicitly submitted slip sections are the same as the existing `PATCH /tickets/{ticketId}`.<br>Unspecified slip sections retain their existing values. |

<a id="schema-ticketformpartialupdateinput"></a>

#### TicketFormPartialUpdateInput

Top-level `inputs[]` element of `PATCH /tickets/{ticketId}/form`.<br>Specify either `formFieldId` or `formFieldCode`. If both are specified, `formFieldId` takes precedence.<br>For inputs under a slip section, use `TicketFormPartialUpdateSlipFieldInput`.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `formFieldId` | string(uuid) |  |  | UUID of the form field. Specify either `formFieldId` or `formFieldCode`. |
| `formFieldCode` | string |  |  | Code of the form field. Specify either `formFieldId` or `formFieldCode`. |
| `value` | string\|null or number\|null or integer\|null or boolean\|null or any[]\|null |  |  | Input value. Specify a value appropriate to the field type (string / number / boolean / array, etc.).<br>If the field is of checkbox type, specify an array. |
| `generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `files` | string[]\|null |  |  | Signed ID of the attached file. Specify only when the field is of file type. |

<a id="schema-ticketformpartialupdateslipfieldinput"></a>

#### TicketFormPartialUpdateSlipFieldInput

Input element under a slip section (`slipItems[].inputs[]`) of `PATCH /tickets/{ticketId}/form`.<br>Specify either `slipFieldId` or `slipFieldCode`. If both are specified, `slipFieldId` takes precedence.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `slipFieldId` | string(uuid) |  |  | UUID of the slip field. Specify either `slipFieldId` or `slipFieldCode`. |
| `slipFieldCode` | string |  |  | Code of the slip field. Specify either `slipFieldId` or `slipFieldCode`. |
| `value` | string\|null or number\|null or integer\|null or boolean\|null or any[]\|null |  |  | Input value. Specify a value appropriate to the field type (string / number / boolean / array, etc.).<br>If the field is of checkbox type, specify an array. |
| `generalMasterItemId` | string(uuid)\|null or string(uuid)[] |  |  | General master item UUID. Specify only when the field is of general master type. |
| `userId` | string(uuid)\|null or string(uuid)[] |  |  | User UUID. Specify only when the field is of user type. |
| `teamId` | string(uuid)\|null or string(uuid)[] |  |  | Team UUID. Specify only when the field is of team type. |
| `ticketId` | string(uuid)\|null or string(uuid)[] |  |  | Ticket UUID. Specify only when the field is of ticket type. |
| `files` | string[]\|null |  |  | Signed ID of the attached file. Specify only when the field is of file type. |

<a id="schema-ticketformpartialupdateslipitem"></a>

#### TicketFormPartialUpdateSlipItem

Slip item (one row) of `PATCH /tickets/{ticketId}/form`.<br>Specify either `slipSectionId` or `slipSectionCode`. If both are specified, `slipSectionId` takes precedence.<br>If multiple elements with the same `slipSectionId` / `slipSectionCode` are specified, they are treated as multiple rows under the same slip section.

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `slipSectionId` | string(uuid) |  |  | UUID of the slip section. Specify either `slipSectionId` or `slipSectionCode`. |
| `slipSectionCode` | string |  |  | Code of the slip section. Specify either `slipSectionId` or `slipSectionCode`. |
| `inputs` | [TicketFormPartialUpdateSlipFieldInput](#schema-ticketformpartialupdateslipfieldinput)[] | yes |  | Array of field input values for that row. Each element specifies a slip field by `slipFieldId` or `slipFieldCode`.<br>Include all editable fields. Omit fields that cannot be edited (e.g., fields for approvers when the applicant is updating) without specifying them (existing values are retained). |

<a id="schema-ticketinput"></a>

#### TicketInput

Form input of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `formFieldId` | string(uuid) | yes |  | UUID of the form field |
| `value` | string\|null or any[]\|null or number\|null or integer\|null | yes |  | Input value<br>If the field type is general master item, user, team, or ticket, a JSON Array is stored as a cache. |
| `formField` | [FormFieldInTicket](#schema-formfieldinticket) | yes |  |  |
| `generalMasterItems` | [GeneralMasterItem](#schema-generalmasteritem)[] | yes |  | Input value: general master item |
| `users` | [User](#schema-user)[] | yes |  | Input value: user |
| `teams` | [Team](#schema-team)[] | yes |  | Input value: team |
| `inputTickets` | [Ticket](#schema-ticket)[] | yes |  | Input value: ticket |
| `attachments` | [Attachment](#schema-attachment)[] | yes |  | Attachment |

<a id="schema-ticketsection"></a>

#### TicketSection

Section of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | Ticket section ID |
| `sectionId` | string(uuid) | yes |  | Section ID of the workflow |
| `visible` | boolean | yes |  | Display state of the section |
| `viewRestriction` | boolean | yes |  | Whether to restrict viewing of the section |
| `allowViewAuthor` | boolean | yes |  | Whether to allow the applicant to view |
| `allowViewApprover` | boolean | yes |  | Whether to allow approvers to view |
| `viewable` | boolean | yes |  | Whether it is viewable |
| `inputs` | [TicketInput](#schema-ticketinput)[] | yes |  | Array of inputs of this section |
| `sectionViewers` | [TicketSectionViewer](#schema-ticketsectionviewer)[] | yes |  | Targets allowed to view the section |

<a id="schema-ticketsectionviewer"></a>

#### TicketSectionViewer

Targets allowed to view the section of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |
| `user` | [User](#schema-user) or null | yes |  | User. Only one of user and team has a value. |
| `team` | [Team](#schema-team) or null | yes |  | Team. Only one of user and team has a value. |
| `grade` | [Grade](#schema-grade) or null | yes |  | Grade. Has a value only when a team is specified and a grade is also specified. |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-ticketguest"></a>

#### TicketGuest

Applicant of an external guest application

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `email` | string\|null | yes |  | Email address |

<a id="schema-ticketstep"></a>

#### TicketStep

Approval step of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `routeStepId` | string\|null | yes |  | UUID of the route step. null for custom steps. |
| `title` | string\|null | yes |  | Title of the step |
| `actionType` | string | yes | enum: `approve`, `confirm`, `none` | Action type. approve for approve/reject (send back), confirm for circulation (with confirmation), and none for circulation (without confirmation). |
| `requiredApprovalsNumber` | integer | yes |  | Number of approvers required |
| `requiredApprovalsPercent` | integer | yes |  | Approval ratio required (%) |
| `stepOrder` | integer | yes |  | Order of the step. The first approval step is 1. |
| `skip` | boolean | yes |  | true if it can be skipped |
| `fallback` | boolean | yes |  | true if fallback occurred |
| `fallbackResult` | string or null | yes | enum: `direct_manager`, `higher_manager`, `skip`, `no_fallback`, `higher_team` | Result of the fallback |
| `author` | [User](#schema-user) or null | yes |  | Creator of the approval step. Set only for custom steps. |
| `assignees` | [TicketAssignee](#schema-ticketassignee)[] | yes |  | Approver |
| `completed` | boolean | yes |  | true if the step is completed |
| `completedAt` | string(date-time)\|null | yes |  | Date and time the step was completed. Returns null for past data. |
| `instruction` | string\|null | yes |  | Instruction message for approvers |
| `displayOrder` | integer | yes |  | Display order of the step |
| `status` | string | yes | enum: `pending`, `in_progress`, `completed`, `skipped`, `raised` | Status of the step. pending if before approval, in_progress if in progress, completed if completed, skipped if skipped, and raised if escalated. |

<a id="schema-ticketassignee"></a>

#### TicketAssignee

Approver of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string | yes |  | UUID |
| `stepOrder` | integer | yes |  | Step number. 1 is the first approval step. |
| `current` | boolean | yes |  | true if it is the current approval step |
| `completed` | boolean | yes |  | true if approved |
| `completedAt` | string(date-time)\|null | yes |  | Date and time of approval. Returns null for old data. |
| `pending` | boolean | yes |  | true if approval is on hold |
| `user` | [User](#schema-user) | yes |  |  |
| `proxyUserIds` | string(uuid)[] | yes |  | Array of user IDs (UUID) of proxy approvers. Returns an empty array if there are no proxy approvers. |
| `actualApprover` | [User](#schema-user) or null |  |  | User who actually performed the approval operation. For proxy approval, this is the proxy approver.<br>Set only when approval is completed; otherwise null is returned.<br>Included only in APIs that return ticket details; not included in the responses of list APIs (ticket list, task list). |

<a id="schema-customnumberingvalue"></a>

#### CustomNumberingValue

Custom numbering value assigned to the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `formattedNumber` | string | yes |  | Custom numbering value with the format applied |
| `customNumbering` | [CustomNumbering](#schema-customnumbering) | yes |  | Custom numbering |

<a id="schema-customnumbering"></a>

#### CustomNumbering

Custom numbering

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes |  | Code of the custom numbering |
| `name` | string | yes |  | Name of the custom numbering |
| `value` | integer | yes |  | Next number to be assigned |
| `ticketNumberKeyId` | string(uuid)\|null | yes |  | ID of the numbering group (ticket number key). null if numbering groups are not used. |

<a id="schema-customnumberingsetting"></a>

#### CustomNumberingSetting

Custom numbering settings for each workflow version

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `customNumberingId` | string(uuid) | yes |  | UUID of the custom numbering |
| `ticketNumberFormat` | string\|null | yes |  | Numbering format. null if not set. |
| `timingType` | string | yes | enum: `created`, `opened`, `completed`, `sub_status_attached` | Timing of numbering. created means when the ticket is created, opened means when it is submitted, completed means when it is completed, and sub_status_attached means when a sub-status is attached. |
| `subStatusId` | string(uuid)\|null | yes |  | UUID of the sub-status used for numbering. null if timingType is not sub_status_attached. |
| `subStatus` | [SubStatus](#schema-substatus) or null | yes |  | Sub-status used for numbering. null if timingType is not sub_status_attached. |
| `sortOrder` | integer | yes |  | Display order |

<a id="schema-attachment"></a>

#### Attachment

Attachment

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `signedId` | string | yes |  | Signed ID |
| `filename` | string | yes |  | File name |
| `url` | string(uri) | yes |  | File URL |
| `byteSize` | integer | yes |  | Number of bytes |
| `contentType` | string | yes |  | Content-Type |

<a id="schema-slipitem"></a>

#### SlipItem

Slip item

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `inputs` | [SlipItemInput](#schema-slipiteminput)[] | yes |  | Array of inputs of the slip item (line item) |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `slipSectionId` | string(uuid) | yes |  | UUID of the slip section |

<a id="schema-slipiteminput"></a>

#### SlipItemInput

Slip item input

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `slipFieldId` | string(uuid) | yes |  | UUID of the slip field |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `value` | string or any[] or number or integer or null | yes |  | Input value<br>If the field type is general master item, a JSON Array is stored as a cache. |
| `generalMasterItems` | [GeneralMasterItem](#schema-generalmasteritem)[] | yes |  | Input value: general master item |
| `users` | [User](#schema-user)[] | yes |  | Input value: user |
| `teams` | [Team](#schema-team)[] | yes |  | Input value: team |
| `inputTickets` | [Ticket](#schema-ticket)[] | yes |  | Input value: ticket |
| `attachments` | [Attachment](#schema-attachment)[] | yes |  | Attachment |
| `slipField` | [SlipFieldInTicket](#schema-slipfieldinticket) | yes |  | Slip field |

<a id="schema-label"></a>

#### Label

Label

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes |  | Name |
| `description` | string\|null | yes |  | Description |
| `color` | string | yes | minLength: 6; maxLength: 6; pattern: `^[a-f0-9]{6}$` | Color of the label. HEX code without # (e.g., ff0000) |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-substatus"></a>

#### SubStatus

Sub-status

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `code` | string | yes |  | Code |
| `name` | string | yes |  | Name |
| `notes` | string\|null | yes |  | Description |
| `allowWithdrawal` | boolean | yes |  | Allow withdrawal |
| `allowRejection` | boolean | yes |  | Allow reject (send back) |
| `allowDenial` | boolean | yes |  | Allow denial |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-ticketviewer"></a>

#### TicketViewer

Viewer of the ticket

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `user` | [User](#schema-user) or null | yes |  |  |
| `team` | [Team](#schema-team) or null | yes |  |  |
| `grade` | [Grade](#schema-grade) or null | yes |  |  |
| `descendants` | boolean | yes |  | Whether to include lower-level teams |

<a id="schema-comment"></a>

#### Comment

Comment

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `body` | string | yes |  | Body |
| `attachments` | [Attachment](#schema-attachment)[] | yes |  | Attachment |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `deletedAt` | string(date-time)\|null | yes |  | Deleted at |
| `user` | [User](#schema-user) | yes |  |  |
| `mentionedUsers` | [User](#schema-user)[] | yes |  | Array of mentioned users |

<a id="schema-auditlog"></a>

#### AuditLog

Audit log

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `user` | [User](#schema-user) or null | yes |  | User who performed the operation. null for operations by the system. |
| `action` | string | yes |  | Operation type |
| `data` | object\|null | yes |  | Operation data |
| `remoteIp` | string\|null | yes |  | Remote IP address |
| `requestId` | string\|null | yes |  | Request ID |
| `systemType` | string\|null | yes | enum: `automation`, `null` | Operation type by the system |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-webhookrequestbodyping"></a>

#### WebhookRequestBodyPing

Webhook request body (ping)

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `eventType` | string |  |  | Type of event |
| `tenant` | [WebhookTenant](#schema-webhooktenant) |  |  |  |
| `user` | [User](#schema-user) |  |  |  |
| `data` | object |  |  |  |
| `data.message` | string |  |  | Message. Used when eventType is ping. |

<a id="schema-webhookrequestbodyticket"></a>

#### WebhookRequestBodyTicket

Webhook request body (ticket_*)

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `eventType` | string |  |  | Type of event |
| `tenant` | [WebhookTenant](#schema-webhooktenant) |  |  |  |
| `user` | [User](#schema-user) |  |  |  |
| `data` | object |  |  |  |
| `data.ticket` | [TicketDetail](#schema-ticketdetail) |  |  |  |

<a id="schema-webhookrequestbodycomment"></a>

#### WebhookRequestBodyComment

Webhook request body (comment_*)

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `eventType` | string |  |  | Type of event |
| `tenant` | [WebhookTenant](#schema-webhooktenant) |  |  |  |
| `user` | [User](#schema-user) |  |  |  |
| `data` | object |  |  |  |
| `data.ticket` | [TicketWithStep](#schema-ticketwithstep) |  |  |  |
| `data.comment` | [Comment](#schema-comment) |  |  |  |

<a id="schema-webhooktenant"></a>

#### WebhookTenant

Tenant information of the webhook

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) |  |  | UUID of the tenant |
| `publicId` | string |  |  | Public ID (subdomain) of the tenant |
| `name` | string |  |  | Tenant name |

<a id="schema-kintoneapp"></a>

#### KintoneApp

kintone integration

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `name` | string | yes |  | kintone app name |
| `domain` | string | yes |  | kintone domain |
| `appId` | string | yes |  | kintone app ID |
| `code` | string\|null | yes |  | Code |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |

<a id="schema-kintoneappsetting"></a>

#### KintoneAppSetting

kintone integration settings

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `formField` | [FormField](#schema-formfield) | yes |  |  |
| `kintoneApp` | [KintoneApp](#schema-kintoneapp) | yes |  |  |
| `createdAt` | string(date-time) | yes |  | Created at |
| `updatedAt` | string(date-time) | yes |  | Updated at |
| `mappings` | object[] | yes |  | Mapping settings to fields |
| `mappings[].id` | string(uuid) | yes |  | UUID |
| `mappings[].displayInTable` | boolean | yes |  | true to display in the selection table |
| `mappings[].kintoneFieldCode` | string | yes |  | kintone field code |
| `mappings[].kintoneFieldName` | string | yes |  | kintone field code |
| `mappings[].kintoneFieldType` | string | yes |  | kintone field code |
| `mappings[].formField` | [FormField](#schema-formfield) | yes |  |  |

<a id="schema-generalmastersearchfilter"></a>

#### GeneralMasterSearchFilter

| Field | Type | Required | Constraints | Description |
|---|---|---|---|---|
| `id` | string(uuid) | yes |  | UUID |
| `filterFormFieldId` | string(uuid) | yes |  | ID (UUID) of the general field used for filtering |
| `fieldType` | string | yes | enum: `free_word`, `name`, `code`, `description`, `custom_field` | Type of the field to be filtered |
| `generalMasterFieldId` | string(uuid)\|null | yes |  | When fieldType=custom_field, ID (UUID) of the custom field of the general master to be filtered |


---

## 5. Webhooks

Source: https://developer.kickflow.com/webhook/ , https://developer.kickflow.com/troubleshooting/ , https://developer.kickflow.com/changelog/ , `kickflow-openapi.yaml` (`webhooks` section and `WebhookRequestBody*` schemas). Registration UI details: see section 6.

### 5.1 Delivery

| Item | Value |
|---|---|
| Method | `POST` to the registered URL |
| Timeout | 10 seconds |
| Retries | Up to 10 retries (was 5 before a 2023 change) with exponential backoff. Exact intervals not documented (inferred: e.g. 2^n seconds or minutes). |
| Success criterion | Receiver must return HTTP `200`. Any 3xx (redirects are not followed), 4xx or 5xx is a failure. Response body is ignored. |
| Duplicates | A timed-out delivery may be sent twice; duplicates share the same `X-Kickflow-Delivery` value. |
| Ordering | Not guaranteed (deliveries are processed by multiple servers in parallel). Receivers should compare `data.ticket.updatedAt` and only apply newer state. |
| Auto-disable | A webhook that keeps failing for one week is disabled automatically and receives no further requests. |
| Source IPs | `52.5.38.201`, `52.7.247.138`, `173.44.50.7`, `34.84.16.25`, `34.104.147.98`, `34.146.67.210` (may change with notice) |

### 5.2 Headers

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `User-Agent` | `kickflow-Hookshot/v1` |
| `X-Kickflow-Delivery` | UUID unique per event (identical across retries of the same event) |
| `X-Kickflow-Signature` | `sha256=<hex>`; present only when a secret is configured on the webhook (the OpenAPI marks it required, the prose docs say it is conditional) |

### 5.3 Signature verification

- `X-Kickflow-Signature = "sha256=" + hex(HMAC_SHA256(key = webhook secret, message = raw JSON request body))`, lowercase hex (example: `sha256=709e8de4bbf818bbbd9ee7f52866322dffcf7efea31d7aec5f397468cc11df18`).
- Receivers compute HMAC over the received bytes as-is.
- Escaping quirk that the dummy server must reproduce: in the JSON payload the characters `&`, `<`, `>` are emitted as `&`, `<`, `>` (Rails-style JSON escaping), and the signature is computed over this escaped string. In Go, `encoding/json` with `SetEscapeHTML(true)` (the default for `json.Marshal`) produces exactly these escapes; note Go also escapes U+2028/U+2029, which kickflow's Rails encoder also does (inferred).
- Constant-time comparison is recommended for receivers (inferred best practice; not stated).

### 5.4 Envelope

Every event body has the same envelope (OpenAPI schemas [WebhookRequestBodyPing](#schema-webhookrequestbodyping), [WebhookRequestBodyTicket](#schema-webhookrequestbodyticket), [WebhookRequestBodyComment](#schema-webhookrequestbodycomment)):

```json
{
  "eventType": "ticket_approved",
  "tenant": { "id": "uuid", "publicId": "subdomain", "name": "Tenant name" },
  "user":   { "id": "uuid", "firstName": "...", "lastName": "...", "fullName": "...", "email": "...", "...": "User schema" },
  "data":   { "ticket": { "...": "TicketDetail" } }
}
```

- `eventType`: event name (table below).
- `tenant`: [WebhookTenant](#schema-webhooktenant) (`id`, `publicId` = subdomain, `name`).
- `user`: [User](#schema-user) who caused the event. Planned breaking change: may be `null` for `ticket_approved`, `ticket_rejected`, `ticket_denied`, `ticket_completed`, `comment_created` (e.g. system-driven actions; inferred reason).
- `data`: event-specific payload.
  - `ping`: `{ "message": "ping" }`
  - `ticket_*`: `{ "ticket": TicketDetail }` (same as `GET /v1/tickets/{ticketId}` response; external API `headers` are stripped)
  - `comment_*`: `{ "comment": Comment, "ticket": TicketWithStep }` (comment as in `GET /v1/tickets/{ticketId}/comments/{commentId}`, ticket as one element of `GET /v1/tickets`)
- The documentation example shows a numeric `user.publicId`; `publicId` was removed for all resources but the tenant, so the dummy server may omit it (inferred).

### 5.5 Event types (complete list)

| eventType | Trigger | `data` |
|---|---|---|
| `ping` | Webhook created, or its URL or secret edited (other edits no longer send ping since 2025) | `message` |
| `ticket_created` | Ticket created | `ticket` |
| `ticket_updated` | Ticket edited | `ticket` |
| `ticket_opened` | Ticket submitted (applied) | `ticket` |
| `ticket_approved` | Ticket approved (approve / send-back type step) | `ticket` |
| `ticket_confirmed` | Ticket confirmed (circulation step) | `ticket` |
| `ticket_rejected` | Ticket sent back | `ticket` |
| `ticket_denied` | Ticket denied | `ticket` |
| `ticket_completed` | Ticket completed (final approval) | `ticket` |
| `ticket_withdrawn` | Ticket withdrawn by applicant | `ticket` |
| `ticket_archived` | Ticket archived (renamed from `ticket_deleted` in 2022) | `ticket` |
| `comment_created` | Comment posted | `comment`, `ticket` |
| `comment_updated` | Comment edited or deleted | `comment`, `ticket` |

No other events (users, workflows, masters, org charts) are documented.

### 5.6 Registration and management

Source: https://support.kickflow.com/hc/ja/articles/360048308493 ("Webhook APIを設定する").

- Webhooks are registered **only via the admin web UI**; the public REST API has no webhook endpoints (Confirmed by absence in the OpenAPI definition).
- UI location: Admin Center (管理センター) > External integrations (外部連携) > Webhook > "New" (新規作成).
- Fields (Confirmed): webhook name, request URL, secret (optional; HMAC key). Optional scope: target workflows ("all workflows" by default, or "specify workflows" with multiple selection) and the events to send (default: all events). Workflow selection limit: 10 (Standard plan and below) / 50 (Enterprise).
- Count limit per tenant: 10 webhooks (Small / Standard plans), 50 (Enterprise plan).
- Permission: only users with the external-integration admin permission (`integration`) can manage webhooks. A webhook receives events for **all** tickets in the tenant regardless of ticket visibility restrictions.
- URL requirements: must be reachable from the internet; HTTPS endpoints need a valid (not self-signed) certificate.
- Delivery history UI: per webhook, "Check execution results" (実行結果を確認) lists events; each event has "Request result" showing request ID, HTTP status code, response body, created-at timestamp. (Note: the developer doc says kickflow ignores the response body for success determination, but the UI stores and shows it.)
- Auto-disable: requests failing continuously for one week disable the webhook.
- Enabled/disabled toggle, manual redelivery and editing UX beyond the above are not documented (inferred: an enabled flag exists because auto-disable exists; manual re-enable is presumably done by editing the webhook).
- Dummy server recommendation (inferred): support multiple webhooks per tenant, name, URL, secret, enabled flag, workflow filter, event filter, delivery log (request ID = `X-Kickflow-Delivery`, status code, response body, timestamp, attempt count), retry with backoff, and automatic disabling.

### 5.7 OpenAPI webhook operations

#### Webhook `ping` (operationId `webhookPing`)

- ping event: Sends a webhook ping event.
- Header `X-Kickflow-Delivery` (string(uuid), required in spec): Event-specific UUID
- Header `X-Kickflow-Signature` (string, required in spec): Signature for payload verification
- Body: Schema: [WebhookRequestBodyPing](#schema-webhookrequestbodyping)

#### Webhook `ticket` (operationId `webhookTicket`)

- ticket_* events: Sends a webhook ticket event.
- Header `X-Kickflow-Delivery` (string(uuid), required in spec): Event-specific UUID
- Header `X-Kickflow-Signature` (string, required in spec): Signature for payload verification
- Body: Schema: [WebhookRequestBodyTicket](#schema-webhookrequestbodyticket)

#### Webhook `comment` (operationId `webhookComment`)

- comment_* events: Sends a webhook comment event.
- Header `X-Kickflow-Delivery` (string(uuid), required in spec): Event-specific UUID
- Header `X-Kickflow-Signature` (string, required in spec): Signature for payload verification
- Body: Schema: [WebhookRequestBodyComment](#schema-webhookrequestbodycomment)


---

## 6. Product features (for the UI)

Source: the public kickflow help center (all 319 Japanese articles, retrieved through the public Zendesk Help Center API because the HTML pages return 403 to non-browser clients), `https://kickflow.com/llms.txt` (mirror of /features and /pricing), and release notes under `https://kickflow.com/update/<slug>`. Notation: `HC:<id>` = `https://support.kickflow.com/hc/ja/articles/<id>`, `UPD:<slug>` = `https://kickflow.com/update/<slug>`, `LLMS` = `https://kickflow.com/llms.txt`. Items are Confirmed unless tagged (inferred). Help-center UI wording is not visible without an account, so layouts are not described.

### 6.0 Terminology mapping (UI Japanese / API / this document)

| UI (JA) | UI (EN rendering here) | API value |
|---|---|---|
| 下書き | Draft | `draft` |
| 処理中 / 申請中 | In progress | `in_progress` |
| 完了 | Completed | `completed` |
| 差し戻し | Sent back | `rejected` (API name), webhook `ticket_rejected` |
| 却下 | Denied (refused) | `denied`, webhook `ticket_denied` |
| 取り下げ | Withdraw | `POST .../withdraw`, webhook `ticket_withdrawn`; resulting status `rejected` |
| アーカイブ | Archived | `archived` |
| 承認 / 確認 | Approve / Confirm (circulation) | `approve` endpoint; `ticket_approved` / `ticket_confirmed` |
| 回覧 | Circulation step | `actionType: confirm` or `none` |
| 経路 | Route | Route |
| 明細 | Slip (detail rows) | slip section / `slipItems` |
| 汎用マスタ | General master | GeneralMaster |
| 役職 | Grade | Grade |
| 共有ユーザー (formerly 閲覧者) | Viewer / shared user | `viewers` |
| ウォッチ | Watcher | `watcherUserId`, WorkflowWatcher |
| 管理センター | Admin Center | (UI only) |

### 6.1 Plans (affects which features punchflow may expose)

- Current plans: Standard and Enterprise; many articles still refer to a legacy Small plan ("Standard or below") (LLMS).
- Enterprise-only (relevant subset): attachment upload API, SAML SSO, IP restriction, dynamic team / dynamic user approver types, route conditions on form values, document output (Excel/PDF templates) and auto-upload to Google Drive / Box, custom validation, sub-status, section view restriction, ticket-field copy, conditional required approver fields, test environment sync, Slack Enterprise Grid, multiple Teams tenants, ClimberCloud, FUJIFILM IWpro, kintone, admin scope limited to specific masters or team subtree, analytics reports, shared views, user custom fields, custom numbering.
- Standard and above: service accounts, external API button, future org charts, folder-scoped admin roles, pipelines, work-in-progress (admin working) mode, advanced condition expressions.
- Paid add-ons: REST API quota option, public guest form option.

| Limit | Standard | Enterprise |
|---|---|---|
| Webhooks | 10 | 50 |
| Workflows per webhook | 10 | 50 |
| Audit log retention | 60 days (Small: 30) | 1 year |
| Admin roles | 10 | 100 |
| Pipelines | 20 | 100 |
| Automation executions | 100 / month | 10,000 / month |
| General masters | 30 | 100 |
| Proxy target workflows | 10 | 30 |
| Backup history | 3 days | 7 days |

Recommendation for punchflow (inferred): model the plan as a tenant setting (`standard` / `enterprise`) so tests can exercise `403 upgrade_required` / `option_purchase_required` paths; default to Enterprise to unlock everything.

### 6.2 Authentication, tokens and security (Admin Center > Security, External integrations)

- Authentication methods, each toggled on/off: email + password, Google sign-in, SAML SSO (Enterprise). Disabling a method returns users who could only use it to `invited` status (HC:4407493936025, UPD:20250407_authentication_renewal).
- SAML 2.0: NameID must equal the kickflow email; SP-initiated via `/signin`, IdP-initiated via `/startSso`; guides exist for Okta, OneLogin, Google Workspace, Microsoft Entra ID, HENNGE One, GMO Trust Login. Microsoft login is only via SAML with Entra ID (HC:360054056214, UPD:20240325_idp_initiated).
- JIT provisioning: on by default; when off, only invited users can sign in via SSO (UPD:20230620_jit_provisioning).
- SCIM: not mentioned anywhere (inferred: not supported). Provisioning alternatives: Google Workspace directory sync, CSV import, REST API.
- 2FA (TOTP) per user and per authentication method, recovery codes; admins with Users permission can reset 2FA (HC:18438396825113, HC:62721463585049). No enforced-2FA policy found.
- IP address restriction (Enterprise): up to 300 entries plus enable toggle; also applies to guest forms; violations give `403 ip_address_denied` (HC:360056089173).
- Invitable email domain allowlist (`422 domain_not_allowed`) (HC:4411887717273); account lockout after repeated failed passwords (HC:24426682064153); fixed session limits 12 h idle / 24 h absolute (HC:40267997935769).
- Personal Access Tokens: profile menu > "Access Token Settings" > New; only field is the token name; token value shown once; no expiry or scopes documented (HC:360047613534).
- Access token usage policy (Admin Center > External integrations > Access token usage permission): allow all users (default) / only specified users / nobody; list shows per-user token count and last-used date; revoking permission deletes that user's tokens; does not affect service accounts (HC:59394836332825, UPD:20260713_access_token_permission_control).
- Service accounts (Standard+): Admin Center > External integrations > Service Accounts > New with name and description; token shown once; impersonates any user via `X-Caller-Id`; requires the `integration` permission (HC:4402035342617).
- punchflow mapping (inferred): Keycloak replaces password/Google/SAML login; PAT and service-account screens must exist in the UI so test systems can mint tokens.

### 6.3 Tickets (end-user screens)

- Dashboard: workflow search, recently used workflows, My Tickets, Approval Requests (tasks), Ticket Search, Views, admin notice banner (HC:360044547694, HC:360053680653).
- Apply flow: choose workflow > choose applicant (proxy) / team (if multiple) > fill form (sections, slip rows) > review route, optionally add custom steps > details (comment, shared users, watchers, labels, related tickets) > submit or save draft (HC:360044547694).
- Approve / send back (to applicant or any earlier step) / deny with optional comment; comments can be mandatory or disabled per workflow; circulation steps show "Confirm" (HC:360044547334, UPD:20230329_reject_to_any_step, HC:60306120645145, HC:59120377175833).
- Bulk approval from the Approval Requests list (tenant can disable) and "continuous approval" mode (HC:27688739836057, HC:48473829367833).
- Withdraw: applicant withdraws own in-progress ticket; it becomes "sent back" (`rejected`) and editable (HC:15066948003353).
- Revoke own approval (per-workflow opt-in; only while in progress and before the next step acts) (HC:45346362606617). Not in the public API.
- Archive: draft/sent back by applicant or ticket-edit admin; in progress/completed/denied by ticket-edit admin only; irreversible (HC:360058324973). Archived tickets can be permanently deleted by ticket-edit admins (HC:11805411813017) - corresponds to `permanently_deleted`. Bulk cleanup (archive/delete by workflow + status, up to 10,000) (HC:16029464994329).
- Duplicate ticket (re-apply) from detail page and My Tickets; can be blocked per workflow (HC:360045273553).
- Hold (保留): per-assignee switch that stops reminders; filterable in Approval Requests; API exposes `TicketAssignee.pending` and `GET /v1/tickets/tasks?pending=` (HC:40445314200857).
- Pull-up (引き上げ): a later approver skips a stalled earlier step if allowed on that step (HC:33362410564633).
- Custom steps: added by applicant or approver; name, action type, assignee rule (user/team), approvers, advance condition (count or percentage), instructions; per-workflow opt-out (HC:360056135854). API: `approvers` in ticket create/update, `author_customizable` / `assignee_customizable` step types.
- Comments: Markdown body, up to 10 attachments, `@user` and `@all` mentions, edit/delete; can be posted together with approve/send back/deny (HC:39425902443545, HC:360056189234).
- Attachments: file fields, comment attachments, preview for PDF / Office / images; 50 MB per file; extension allowlist configurable per file field (HC:360053857093).
- Sharing scope: "all users" or "specified users only" per workflow; default viewers are applicant, proxy applicant, approvers and proxy approvers; extra viewers by user or team (+ minimum grade, + descendants); per-ticket override (`forcedPublicType`); restriction of who can edit viewers (`allowEditingOfViewers`); admin bulk add/remove viewers (HC:360045462794, HC:59493283644313, HC:35783440856857).
- Watch toggle per user; applicants and approvers auto-watch; workflow-level forced watchers (HC:360045069513).
- Related tickets (two-way links; created manually or by pipelines) (HC:360044547674). API: `GET /v1/tickets/{id}/links`.
- Sub-status (Enterprise): admin-defined name, code, memo, allow withdraw / send back / deny switches; attached when a ticket reaches a route step (HC:31421911043353).
- Labels: name, description, color (6-hex), managed by `label` permission; any viewer can attach/detach (HC:30025426510745).
- Print / PDF via browser; Excel/PDF document output from uploaded `.xlsx`/`.xlsm` templates with `${...}` variables (Enterprise) (HC:1500005916061, HC:4404540031257).
- Numbering: ticket number format tokens `[year] [month] [day] [number] [fyear]`; timing on create / submit / complete / sub-status; numbering groups shared across workflows; custom numbering up to 5 per workflow (Enterprise) (HC:360047757013, HC:58896171530521).
- Operation history tab per ticket (create/edit, comments, viewer changes, labels, step moves), kept forever (HC:360056189234). API: `logsCount` only.
- Approver input fields (fields editable by approvers, optionally restricted by team/grade/user) and per-workflow "approvers may overwrite all fields" (`overwritable`) (HC:360044556134, HC:4409145693977).
- Admin ticket tools: hand over a user's tickets / approval requests to another user; "tickets with no approver" list with add-approver action (HC:6663669942553, HC:28410770323737).
- Search: Ticket Search with filters (mirrors `GET /v1/tickets` query parameters), up to 5 saved searches per screen, Views with custom columns (10/50 per user), CSV export of tickets or slip rows (max 5 workflows, 10,000 rows, encoding choice) (HC:29499250310169, HC:18324290337049, HC:1500007064762).

### 6.4 Workflow builder (Admin Center > Workflow > Workflows)

- 4-step wizard: Basic info > Form > Route > Detail settings > "Save and activate" (HC:360044982993).
- Basic info: name, description, per-version admin memo, numbering timing and format, numbering group, folder, categories, code.
- Title mode: applicant input / formula / none (`titleInputMode`, `titleFormula`).
- Sections: form sections (display conditions from the 2nd section on; view restriction is Enterprise), slip sections up to 5 with section code.
- Field types (see 1.10 enum): short/long text, integer, number, auto-calc, checkbox, pull-down, radio, date, datetime, file, general master, user, team, ticket, external API button (Standard+), kintone button (Enterprise); hidden fields; read-only-on-UI fields.
- Validation: required, min/max length, regex, min/max value; custom validation (Enterprise, AND/OR, max 50 form / 10 slip) (HC:23898886993433).
- Calculation formulas (e.g. `json_lookup`) and tenant global variables (alphanumeric names up to 10 chars, numeric values) (HC:900007118003).
- Route branching: up to 100 branches, conditions with all / any / advanced expression, first match wins, or reject the application with an error message (HC:900005812006). API: `WorkflowRouteCondition`.
- Versioning: new version, save as draft, switch now or schedule a switch (00:30 on a chosen date) (HC:32789227217689). Form field IDs change per version; codes are stable.
- Other settings: applicable teams and grades, ticket creation restriction, filter visibility, auto viewers and watchers, Slack / Chatwork / Google Chat / Teams channel notifications, document templates, CloudSign, allow approval revoke, comment required / disabled, allow duplicate.
- Templates (no route included), copy form from another workflow, JSON import/export (HC:4407180502425, HC:25724391432217).
- Limits: 10,000 workflows; sections 20/50; fields 100/200; slip columns 10/30; slip rows 50; short text 300 chars; long text 10,000; 100 options; numbers keep 14 significant digits (HC:18778979045017).
- Public REST API is read-only for workflows (`GET /v1/workflows`, `GET /v1/workflows/{id}`).

### 6.5 Routes (Admin Center > Workflow > Routes)

- Routes are standalone objects shared by workflows (HC:360044982933).
- Step fields: name, action type (approve/send back, circulation with confirm, circulation without confirm), instructions, approver type, advance condition (count or percentage), allow self-approval, execution condition, step code (default 16 random chars, editable, `[A-Za-z0-9_-]` up to 100), sub-status, allow pull-up.
- Approver types: manager (direct or by grade; fallback direct / higher / skip), team + grade (up to 10; fallback stay / walk up / skip), fixed user, applicant, applicant nominates (min count may be 0, restrictable), previous approver nominates, dynamic team and dynamic user via `$fieldcode` (Enterprise). API enum: `manager`, `team`, `user`, `author`, `author_customizable`, `assignee_customizable`, `dynamic_team`, `dynamic_user`.
- Execution condition: always / only when / except when (`always` / `conditional` / `conditional_skip`), up to 5 conditions; applicant team/grade on all plans, form values Enterprise only.
- Parallel steps: all must finish; a user in several parallel steps approves them all at once (HC:360046043034).
- Route versions with scheduled switch; simulation tool (pick applicant, org chart, team; shows resolved approvers) (HC:40135380794137, HC:360044511134).
- Limits: 50,000 routes; 50 steps + 10 custom steps; 100 approvers per step and per route (HC:18779253569305).
- Public REST API is read-only for routes.

### 6.6 Organization (Admin Center > Organization / Users)

- Organization charts: multiple charts (future charts, Standard+), activate now or on a schedule; CSV import into the current chart blocked by default (HC:360045217634, HC:59490699308185). API: org chart CRUD + activate.
- Teams: name, code, admin memo (`notes`), parent team, "approve-only (cannot apply)" flag; CSV `code,name,notes,parent_code,approve_only` (full replace, blocks of 1,000); limits 20,000 teams, 100 children, 10 levels (HC:360044982853, HC:4411564295449).
- Memberships: user, team, grades, "manager of this team" (`leader`) flag; CSV `user_code,team_code,leader,grade_code`; one user may be in several teams; 300,000 memberships (HC:360045070333).
- Grades: name, level 0-255 (unique), code, default grade; up to 100; CSV `code,name,level,is_default` (HC:360044455474).
- Users: invite form (last name, first name, email, code, employee ID, language, up to 5 memberships, send-email checkbox); invitation lifetime 1-7 days; statuses invited / activated / suspended / deactivated; reinvite, suspend, reactivate, delete; CSV import/export; last-used date (HC:360044972593, HC:360044511814, UPD:20260730_user_last_used_on).
- User custom fields (Enterprise): up to 10; types short text, long text, number, integer, pull-down, checkbox, date; options required, visible to everyone, default column in user picker; reserved codes `id, last_name, first_name, email, code, employee_id, status, created_at, send_email, line_works_account_id, locale` (HC:58880204881945).
- Google Workspace directory sync (invite / suspend / delete by Workspace status) (HC:360058701014).

### 6.7 General masters (Admin Center > General Masters)

- Master: name, code, description, default picker columns, default sort (name/code), custom fields (each with required, visible-to-all, default column) (HC:360044511774).
- Items: code, name, description, start/end dates (expired items hidden in pickers), custom field values; CSV import/export.
- Form usage: single or multiple selection, default item, auto-filter by up to 10 other fields (`generalMasterSearchFilters`).
- Limits: 30/100 masters, 100,000 items each, 10/30 custom fields (HC:18780503080601).
- Permissions: `master_read` (view/export) and `master` (edit); Enterprise can scope to specific masters.

### 6.8 Categories and folders

- Categories: for finding workflows; a workflow may have several; CSV import (HC:22394058512921).
- Folders: hierarchy for workflows, routes and pipelines; max 300 folders, 100 children, 10 levels; root folder is not editable (`resource_not_editable`) (HC:360044510954, HC:18809204278297).
- Folder-scoped permissions are expressed through admin roles (workflow, route, pipeline, ticket view scoped to folders; Standard+) (HC:360044559254).

### 6.9 Proxy application and proxy approval

- Admin screens: Users > Proxy Application and Users > Proxy Approval; fields: requesting user, proxy user, optional date range, optional target workflows (10, or 30 on Enterprise); one registration per requester/proxy pair (HC:360045038013, HC:360045038033).
- Self-service mode: admins may let users manage their own proxies (HC:1500003077661, HC:1500003227322).
- Proxy approvers are auto-added as viewers and watchers on in-progress tickets; proxy applicants are not.
- CSV import for both (UPD:20250121_proxy_applicants_approvers_csv).

### 6.10 Admin roles (Admin Center > Security > Admin Roles)

- Role: name, permission set (optionally scoped), members. A Super Admin role always exists with all permissions and at least one member; non-super admins cannot change roles they belong to (HC:360045038093, HC:8938166943513).

| UI permission | API `permission` value (mapping inferred from names) | Covers | Scopable to |
|---|---|---|---|
| Tenant | `tenant` | Tenant name, subdomain, notices, notification defaults, work-in-progress mode, remote assistant, backup | |
| Billing | `billing` | Licenses, billing, invoices | |
| Users | `user` | Users, grades, proxies | |
| Teams | `team` | Org charts, teams, memberships | Team subtree (Enterprise) |
| Workflows | `workflow` | Workflow CRUD | Folders |
| Routes | `route` | Route CRUD | Folders |
| Pipelines | `pipeline` | Pipeline CRUD | Folders |
| Workflow-related settings | `workflow_misc` | Categories, numbering groups, sub-statuses, folders, global variables | |
| General master (view) | `master_read` | View/export | Specific masters (Enterprise) |
| General master (edit) | `master` | Edit, CSV | Specific masters (Enterprise) |
| Labels | `label` | Labels | |
| Automations | `automation` | Automation rules | |
| Tickets (view) | `ticket_read` | All tickets incl. restricted sections | Folders |
| Tickets (edit) | `ticket_write` | Delete, add approver, override sharing, hand over, bulk viewers, cleanup | |
| Analytics reports | `stats` | Reports | |
| External integrations | `integration` | Integrations, webhooks, service accounts | |
| Admin roles | `role` | Role management | |
| Security | `security` | Auth methods, SSO, IP restriction, invite domains | |
| Audit logs | `audit` | View/export audit logs | |

(HC:8938311325209)

### 6.11 Audit logs (Admin Center > Security > Audit Logs)

- List with CSV export; retention 30 days (Small) / 60 days (Standard) / 1 year (Enterprise); records source IP and before/after values in "operation details" (HC:360045335414, UPD:20260715_audit_log_detail_expansion). REST: `GET /v1/auditLogs` (all plans, UPD:20250311_audit_logs_rest_api). Webhook edits are audited.

### 6.12 Notifications and integrations

- Channels: email, browser push, Slack, Microsoft Teams, Chatwork, Google Chat, LINE WORKS; per-user event x channel matrix; tenant defaults with optional overwrite; extra notification email addresses (HC:4415137587993, HC:4411304984985, HC:360044547874).
- Event catalog (HC:43704281380761): My Tickets (approved, sent back, denied, confirmed, completed, commented, edited, archived, auto-created by pipeline, approval/confirm revoked); Approval Requests (assigned, reminder, approved/sent back/denied/confirmed by a parallel approver, commented, edited, archived, withdrawn, revoked); watched tickets (submitted, approved, sent back, denied, confirmed, completed, commented, edited, archived, withdrawn, revoked).
- Reminders: default 08:00 JST weekdays excluding Japanese holidays, configurable (HC:8194449929369).
- Chat DMs with action buttons (approve / send back; Slack also deny and comment); channel notifications per workflow (`WorkflowSlackChannel`, `WorkflowChatworkRoom`, `WorkflowMicrosoftTeamsChannel`, `WorkflowGoogleChatSetting`); Slack `/kickflow` slash command (HC:52011774552089, HC:360050697313).
- CloudSign (send document on completion; `cloudSignDocument` on tickets) (HC:4405391501465); kintone (Enterprise; button field + automation write) (HC:49514035446041); ClimberCloud (Enterprise; upload on approval) (HC:31421867811097); Google Drive / Box auto-upload of generated PDF (Enterprise) (HC:29759164940185); FUJIFILM IWpro via automation (Enterprise) (HC:54690456288153).
- External API button (Standard+): method, URL, headers with variables, always sends `User-Agent: kickflow`; response must be UTF-8 JSON mapped into fields; up to 100 records (HC:4402691009049). API: `ExternalApiSetting` (request headers are not exposed).
- Automations: triggers on status change or sub-status change (Enterprise); actions add/remove labels, add viewers, upsert/delete general master item, add admin role member, write to kintone, upload to IWpro; limits 100 rules, 5 actions, 10 conditions; execution log; auto-disable on error with email (HC:36919697053465). Audit `systemType: automation`.
- Pipelines (Standard+): on completion of workflow A create (draft or auto-submitted) ticket in workflow B with field copy; auto or manual run modes; 20/100 limit (HC:360044455654).
- Guest / public forms (paid option): enabled per workflow at creation; share link re-issuable; optional guest email with completion/denial notice; restrictions on approver types and features (HC:44900646683417). API: `TicketGuest`, `WorkflowExternalPublish`, nullable `author`.
- Analytics reports (Enterprise), aggregated daily at 06:00 (HC:4403248466329).

### 6.13 Tenant settings (Admin Center > Tenant)

- Tenant name, subdomain (tenant ID, `WebhookTenant.publicId`), plan, admin notice, notification defaults, reminder schedule (HC:360044983073).
- Work-in-progress mode (Standard+): blocks create/update/copy/approve/send back/deny/withdraw/archive/custom steps, including via API and chat (`403 admin_is_working`) (HC:4491537058201).
- Remote assistant (support staff access up to 180 days), daily configuration backup (CSV/JSON), production-to-test tenant sync (Enterprise) (HC:16785122523545, HC:28166738289689, HC:42910605283993).
- Other settings: bulk approve on/off, CSV import into current org chart, fiscal-year start month.
- Languages: Japanese, English, Chinese, Korean; responsive PWA, no native mobile app (HC:8680406270617, HC:4404302573465).

### 6.14 Suggested punchflow UI scope (inferred)

Minimum UI to make integration testing meaningful: login via Keycloak; dashboard; apply (create/draft/submit) with dynamic forms from workflow definitions; ticket detail with approve / send back / deny / withdraw / archive, comments with mentions and attachments, viewers, related tickets; My Tickets / Approval Requests / Ticket Search; admin screens for users, grades, org charts, teams and memberships, general masters and items, categories, folders, workflows and routes (editor or JSON import), proxy settings, admin roles, audit logs, access tokens (PAT), service accounts, webhooks (with delivery log and redeliver); tenant settings for plan and maintenance / admin-working / suspended toggles to trigger error paths.

### 6.15 Out of scope: AI features

Not to be implemented; names only: AI option and AI credits, AI input assist, AI pre-submission review (AI check), AI ticket summary, AI form creation assist, AI formula suggestion, AI regex suggestion, AI workflow search, admin AI helpdesk, kickflow MCP Server (`@kickflow/mcp-server`, a generic `discover_apis` / `get_api_info` / `call_api` wrapper over the REST API; its source reads `KICKFLOW_API_BASE_URL` (default `https://api.kickflow.com`), so it can be pointed at punchflow for smoke testing without implementing anything AI-related).

---

## 7. Confirmed vs inferred summary

### 7.1 Confirmed (official public sources)

- Base URL, `/v1` prefix, JSON-only requests, 411 rule, CORS, JST RFC 3339 timestamps with milliseconds, accepted input date formats.
- Authentication with PAT and SAT (`X-Caller-Id`), 401/403 semantics, RBAC permission identifiers.
- Pagination parameters and headers (`Link`, `Page`, `Per-Page`, `Total`), `sortBy` syntax, `key[]` array syntax.
- Rate limit 30/min per IP, `RateLimit-*` headers, `X-Rate-Limit-Secret` paid option (300/min).
- Error body format and the documented error code table.
- All 96 operations, their parameters, request bodies, response schemas, and all component schemas with enums (from the official OpenAPI file dated by the 2026-10-20 changelog entry).
- Webhook transport (POST, 10 s timeout, 10 retries with exponential backoff, 200-only success, duplicate deliveries, no ordering guarantee, one-week auto-disable), headers, HMAC-SHA256 signature format and escaping quirk, source IPs, all 13 event types and payload envelopes.
- Webhook admin UI location, fields (name, URL, optional secret, workflow scope, event selection), plan limits, required permission, execution-result UI.
- Withdraw moves a ticket to `rejected`; archive rules per status and irreversibility.
- Changelog history, including deprecated/removed fields (`publicId`, `conditionAllOf`, `slip*` workflow fields, `deleted` ticket status renamed to `archived`, `ticket_deleted` webhook renamed to `ticket_archived`, removal of ticket delete API).

### 7.2 Inferred (design assumptions for punchflow)

- Presence of `prev` / `first` relations in the `Link` header.
- Default sort direction when the `-asc`/`-desc` suffix is omitted (ascending).
- Exact webhook retry intervals and backoff base.
- Exact firing combinations of webhook events for compound actions (e.g. create with `in_progress` emitting both `ticket_created` and `ticket_opened`; last approval emitting `ticket_approved` then `ticket_completed`).
- Behavior of `reject` with `to > 0` (ticket stays `in_progress` and returns to step N).
- Meaning of `permanently_deleted` (purged after archive; UI-only operation).
- Error code for archiving an already archived ticket (`422 invalid_state`).
- Token string format (opaque).
- The `AuditLog.action` vocabulary (not enumerated publicly).
- Sub-status management API: the 2024-05-14 changelog says a sub-status API was added, but no sub-status endpoint exists in the current OpenAPI file; sub-statuses are only visible through tickets (`subStatus`) and the `subStatusIds` filters. Treat sub-status CRUD as UI-only.
- Everything in section 6 marked "(inferred)".

### 7.3 Not publicly available (cannot be confirmed without an account)

- Exact JSON of real responses beyond the OpenAPI schemas (example payloads are not published except a few in guides).
- Undocumented internal API used by the web UI.
- Exact audit log action names, exact webhook backoff schedule, exact admin UI layouts.

### 7.4 Out of scope: AI features

All kickflow AI features are explicitly excluded from punchflow. They are listed here only for completeness (names as found in public material, see section 6.15); none of them affect the REST API or webhooks.


---

## 8. Sources

All retrieved on 2026-10-20.

| Section | Source |
|---|---|
| 1 Conventions | https://developer.kickflow.com/rest/ (endpoint, auth, RBAC, pagination, sorting, arrays, rate limits, CORS, time zone, errors, notes) |
| 1 Conventions | https://developer.kickflow.com/troubleshooting/ (URL encoding, array syntax, type errors, field code errors, endpoint_not_found, 411, signature escaping) |
| 1 Conventions, 7 | https://developer.kickflow.com/changelog/ (behavior changes and deprecations 2020-2026, planned breaking changes) |
| 1.8 Guides | https://developer.kickflow.com/guide/ticketInput/ , https://developer.kickflow.com/guide/calculation/ , https://developer.kickflow.com/guide/attchment/ , https://developer.kickflow.com/guide/comment/ , https://developer.kickflow.com/guide/serviceAccount/ |
| 2, 3, 4, 5.7 | https://developer.kickflow.com/rest/schema.yaml (OpenAPI 3.1.0, copied to `kickflow-openapi.yaml`); human-readable rendering at https://developer.kickflow.com/rest/spec |
| 2, 3, 4 (cross-check) | https://github.com/kickflow/kickflow-mcp-server (official MCP server; bundles an older copy of `schema.yaml` lacking only the 2026-10 `radio_button` additions) and https://www.npmjs.com/package/@kickflow/mcp-server |
| 5 Webhooks | https://developer.kickflow.com/webhook/ |
| 5.6 Webhook UI | https://support.kickflow.com/hc/ja/articles/360048308493 , https://kickflow.com/update/20221124_webhook_event , https://kickflow.com/update/20230605_webhook_disabled , https://kickflow.com/update/20230608_webhook_workflow , https://kickflow.com/update/20241031_webhook_results_view , https://kickflow.com/update/20241114_webhook_comment_events , https://kickflow.com/update/20250929_webhook_workflow_limit_50 , https://kickflow.com/update/20260603_webhook_limit_50 |
| 6 Features | https://support.kickflow.com/hc/ja (319 articles via the public Help Center API `https://support.kickflow.com/api/v2/help_center/ja/articles.json`; article IDs cited inline as `HC:<id>`), https://kickflow.com/llms.txt (features and pricing), https://kickflow.com/update/ release notes (cited inline as `UPD:<slug>`) |
| 6.2 Tokens / security | HC:360047613534 (PAT), HC:59394836332825 (token usage policy), HC:4402035342617 (service accounts), HC:44091117959449 (rate limit option), HC:4407493936025 (auth methods), HC:360054056214 (SAML), HC:360056089173 (IP restriction), HC:18438396825113 (2FA) |
| 1.9, 6.3 Ticket status | HC:15066948003353 (withdraw), HC:360058324973 (archive), HC:11805411813017 (delete archived) |
| GitHub search | No third-party OpenAPI mirrors, SDKs or Terraform providers with real content were found (`japan4415/terraform-provider-kickflow` is an unmodified scaffold; `hbn-kumi-ohashi/kickflow-teams-notifier` is empty). |
