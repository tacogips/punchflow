# Design Notes

This document contains research findings, terminology decisions, the inferred-behavior register and the list of intentional divergences for punchflow.

## Overview

punchflow is a local/dev-only dummy of the kickflow approval-workflow SaaS. The primary spec inputs are `design-docs/references/kickflow-api.md` (consolidated reference, "the reference") and `design-docs/references/kickflow-openapi.yaml` (official OpenAPI 3.1, "the spec"). Everything the reference marks **Confirmed** is implemented as documented. Everything else that punchflow has to decide is recorded below as an `INF-xx` entry so that code, tests and the coverage matrix can cite one stable ID.

Rule: when code implements an INF item, the Go doc comment or test name cites the ID (for example `// INF-12`). When an INF decision changes, update this file first.

---

## Sections

### Terminology mapping (issue wording vs kickflow API)

The issue uses generic approval words. kickflow uses different ones, and punchflow follows kickflow.

| Issue / generic word | kickflow API | Ticket status after | Webhook |
|---|---|---|---|
| submit | `POST /v1/tickets` with `status: in_progress`, or `PATCH /v1/tickets/{id}` `status: in_progress` from `draft` | `in_progress` | `ticket_opened` |
| approve | `POST /v1/tickets/{id}/approve` on an `approve` step | `in_progress` or `completed` | `ticket_approved` |
| confirm (circulation) | `POST /v1/tickets/{id}/approve` on a `confirm` step | `in_progress` or `completed` | `ticket_confirmed` |
| remand / send back | `POST /v1/tickets/{id}/reject` with `to` | `rejected` (`to: 0`) or `in_progress` (`to > 0`) | `ticket_rejected` |
| re-submit | `PATCH /v1/tickets/{id}` `status: in_progress` from `rejected` | `in_progress` | `ticket_opened` |
| reject (final refusal) | `POST /v1/tickets/{id}/deny` | `denied` (terminal) | `ticket_denied` |
| withdraw | `POST /v1/tickets/{id}/withdraw` | `rejected` | `ticket_withdrawn` |
| archive | `POST /v1/tickets/{id}/archive` | `archived` | `ticket_archived` |

So the issue's "reject" acceptance test is implemented with the `deny` endpoint, and "remand + re-submit" with `reject` followed by `PATCH status: in_progress`.

### Inferred-behavior register

Each entry is a design assumption. "Ref" points to the reference section that leaves the behavior open.

| ID | Topic | Decision | Ref |
|---|---|---|---|
| INF-01 | Pagination `Link` | Emit `first`, `prev`, `next`, `last` relations when applicable (no `prev`/`first` on page 1, no `next` on the last page). URLs are absolute, built from `--public-base-url`, keeping all other query parameters. `Total` counts all matching rows. | 1.3 |
| INF-02 | Sorting | `sortBy` without suffix sorts ascending. An unsupported field or a value that fails the operation's pattern is ignored (Confirmed: ignored). Default order without `sortBy`: `createdAt` descending for `listTickets`, `listTasks`, `listAuditLogs`; documented defaults for viewers (team name, then email) and master items (`defaultSortBy`); `createdAt` ascending for every other list. Ties break on `id`. | 1.4, 1.8 |
| INF-03 | Paging parameters | `page < 1`, `perPage < 1`, `perPage > 100` or non-integer values return `400 invalid_parameter` (no clamping). An out-of-range page returns `200 []`. | 1.3 |
| INF-04 | Token format | Opaque strings `pf_pat_<32 base62>` (personal) and `pf_sat_<32 base62>` (service account). Only the SHA-256 hash is stored, except fixture-seeded tokens (see design-auth-admin.md). Fixture-seeded dev tokens may use any low-entropy string (for example `pf_pat_dev_admin`); authentication never validates the token format, it only looks up the hash. | 1.2 |
| INF-05 | `X-Caller-Id` | Service token without the header: `401 invalid_caller_id` (Confirmed). Header present but not a UUID: `400 invalid_header`. UUID of an unknown user: `401 invalid_caller_id`. A personal token ignores the header. | 1.2, 1.7 |
| INF-06 | Token user state | The effective user (PAT owner or `X-Caller-Id` user) is checked after the token: `invited` with a valid invitation -> `401 user_not_registered`; `invited` with an expired invitation -> `401 invitation_expired`; `suspended` or `deactivated` -> `401 user_inactive`. | 1.7 |
| INF-07 | Rate limiting | Disabled by default. When `--rate-limit N` (N > 0) is set: fixed 60 s window per client IP (`RemoteAddr` host only, no proxy headers), `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` (UNIX seconds) on every `/v1` response, `429 rate_limited` when exceeded. A request whose `X-Rate-Limit-Secret` equals `--rate-limit-secret` gets a limit of `10 * N` (kickflow: 30 -> 300). Headers are omitted when disabled. | 1.5 |
| INF-08 | Webhook retry schedule | 1 initial attempt plus up to 10 retries. Delay before retry n (1-based) is `base * 2^(n-1)`, capped at 1 h; `base` defaults to 10 s (`--webhook-retry-base`). | 5.1 |
| INF-09 | Delivery identity | One `X-Kickflow-Delivery` UUID per (event, webhook) pair. Automatic retries and manual resends reuse the UUID and the stored body bytes; the signature is recomputed with the webhook's current secret. | 5.1, 5.2 |
| INF-10 | Webhook auto-disable | A webhook records `firstFailureAt` on the first failed attempt after a success. When an attempt fails and `firstFailureAt` is at least 7 days old, the webhook is disabled (`enabled=false`, `disabledAt` set) and its pending events become `failed`. Any success clears `firstFailureAt`. | 5.1 |
| INF-11 | Compound event order | Create with `in_progress`: `ticket_created` then `ticket_opened`. Final approval: `ticket_approved` (or `ticket_confirmed`) then `ticket_completed`. `PATCH` that changes fields and submits: `ticket_updated` then `ticket_opened`. `PATCH` that only changes the status emits only the status event. Every payload in one action is the ticket state after the whole action. | 1.9 |
| INF-12 | Send back to step N | `reject` with `to: 0` sets status `rejected`, `currentStep 0`. With `1 <= to < currentStep` the ticket stays `in_progress`, `currentStep = to`, and approvals of steps `>= to` are cleared. `to >= currentStep` or `to < 0`: `422 validation_failed` (`errors.to`). | 1.9 |
| INF-13 | Parallel steps | Route steps that share the same `stepOrder` form one parallel group. The group is current as a whole and completes when every non-skipped step in it completes. `TicketStep.displayOrder` is the sequential index inside the route. A user assigned to several steps of the current group approves all of them with one call (Confirmed). | 6.5 |
| INF-14 | Step completion (all/any) | A step completes when its approval count reaches `requiredApprovalsNumber` if that is > 0, otherwise `ceil(requiredApprovalsPercent / 100 * assigneeCount)`. "any" = `requiredApprovalsNumber: 1`; "all" = `requiredApprovalsNumber: 0, requiredApprovalsPercent: 100`. Fixtures must set exactly one of the two to a positive value. | 3 RouteStep |
| INF-15 | Circulation without confirmation | A step with `actionType: none` completes as soon as it becomes current, emits no event, and does not block. | 1.10 |
| INF-16 | Proxy approval | Active proxy approver registrations are evaluated at read and action time (date range in JST today, workflow filter empty or containing the ticket's workflow). `TicketAssignee.proxyUserIds` lists them. A proxy may approve, send back or deny for the assignee; `actualApprover` is the proxy. | 3 ProxyApprover, 4 TicketAssignee |
| INF-17 | Route branch refusal | When the first matching `routeConditions` entry has `errorMessage` instead of a route, submitting returns `422 validation_failed` with `errors.base: [errorMessage]` and nothing is persisted. | 4 WorkflowRouteCondition |
| INF-18 | Unsupported condition features | `combinationType: custom` and `fieldKey: other_variable` are rejected by the fixture loader with a clear error. They are never evaluated. | 1.10 |
| INF-19 | Manager resolution | Managers are members with `leader: true` of a team in the active org chart. `closest`: leaders of the author team; if the only leader is the author, use the parent team's leaders, walking up. `all`: leaders of the author team and every ancestor. Grade targets filter the result with `gradeSymbol`. Fallbacks: `direct_manager`/`higher_manager` walk up; `higher_team` walks to the parent team; `skip` marks the step `skipped`; `no_fallback` with no approver makes submit fail with `422 validation_failed` (`errors.base`). | 6.5 |
| INF-20 | Nominated approvers | `author_customizable` steps take approvers from `approvers[]` on create/update (count must be `>= minCustomAssignees`; 0 approvers with min 0 skips the step). `assignee_customizable` steps take approvers from `approvers[]` sent by a current approver via `PATCH /v1/tickets/{id}`; approving the step before it while the next step is `assignee_customizable` and empty returns `422 assignment_required_before_approve`. | 1.7, 6.5 |
| INF-21 | Formulas | `calculation` field values and `titleFormula` are not evaluated. Calculation values are stored as `null`; `titleInputMode: calculate` sets the title to the workflow name. | 6.4 |
| INF-22 | Action comments | A `comment` sent with approve/reject/deny is stored as a ticket comment and emits `comment_created` after the ticket event. | 6.3 |
| INF-23 | Asynchronous operations | Org chart delete/activate and proxy reassignment complete before the response returns. | 1.8 |
| INF-24 | Invalid transitions | Any action not allowed by the state machine (including archiving an archived ticket and `rejected -> draft`) returns `422 invalid_state`. | 1.9 |
| INF-25 | Withdraw permission | Only the author (or the proxy client of a proxy application) may withdraw; anyone else gets `403 missing_permission`. | 1.9 |
| INF-26 | Files | Uploads are stored under `<data-dir>/files`. `signedId` is an opaque random string. `getFile.url` is a punchflow URL signed with HMAC and valid for 5 minutes. `getFile` on a file not attached to a ticket input or comment returns `404 resource_not_found`. Unreferenced uploads are kept until reset. `checksum` is the base64 MD5 of the content (Rails ActiveStorage convention). | 1.8 |
| INF-27 | Plan gate | Tenant plan `standard` makes `uploadFile` return `403 upgrade_required`. Default plan is `enterprise`. | 6.1 |
| INF-28 | Creation restriction | A workflow with `availableToEveryone: false` whose `availableTeams` does not match the author team (and grade) returns `403 ticket_create_restricted` on create. | 1.7 |
| INF-29 | Audit log actions | Every successful mutating `/v1` and `/admin/api` request writes one audit log with `action` = `<resource>.<verb>` (for example `ticket.approve`, `user.suspend`, `webhook.update`), `data` = ids and changed field names, `remoteIp`, `requestId` (`X-Request-Id`), `systemType: null`. | 4 AuditLog |
| INF-30 | Deleted comments | Soft-deleted comments stay in list/get responses with `deletedAt` set and `body` cleared to `""`. | 1.8 |
| INF-31 | Invitation acceptance | The first successful UI login (OIDC or local) of an `invited` user with a valid invitation activates the user. | 6.6 |
| INF-32 | Codes without a natural trigger | `email_not_verified`, `license_purchase_required`, `option_purchase_required`, `feature_disabled`, `gateway_timeout` (and forced `internal_server_error`) are produced only by the dev fault-injection admin API. | 1.7 |
| INF-33 | JSON escaping | All JSON written by punchflow (API responses and webhook bodies) uses Go `json.Marshal` default HTML escaping (`<`, `>`, `&`, ` `, ` `) with no trailing newline. | 5.3 |
| INF-34 | Task list semantics | `listTasks` returns tickets where the caller, or a user the caller is an active proxy approver for, is an assignee. Without `status[]` only tickets where that assignee is current and not completed are returned. With `status[]`, any ticket where the caller ever was an assignee is filtered by ticket status (`deleted` matches `archived`). `assignedToMeAt` is when the assignee's step became current. | 3 listTasks |
| INF-35 | Ticket numbering | `ticketNumberFormat` tokens `[year]`, `[month]`, `[day]`, `[number]`, `[fyear]` (fiscal year starts in April) are expanded at the workflow's `ticketNumberingTiming` (`created`, `opened`, `completed`; `sub_status_attached` behaves like `opened`). `[number]` is a per-workflow counter starting at 1. A null format leaves `ticketNumber` null. | 6.3 |
| INF-36 | Ticket visibility | A ticket is visible to: its author and proxy client, any assignee or proxy approver of any step, explicit viewers (user, or team member with grade filter and optional descendants), everyone when `publicStatus` is true, and holders of `ticket_read` or `ticket_write`. Otherwise `403 invisible_ticket`. Drafts are visible only to the author, the proxy client and `ticket_read` holders. | 1.7, 6.3 |
| INF-37 | Comment ownership | Only the comment author may update or delete a comment; others get `403 missing_permission`. | 3 Comments |
| INF-38 | Unknown request fields | Unknown JSON request fields are ignored. Wrong JSON types return `400 invalid_parameter` with message `expected <type>, but received <type>`. | 1.4 |
| INF-39 | Request body limits | JSON bodies over 10 MiB and uploads over 50 MiB (`--max-upload-bytes`) return `400 payload_too_large`. | 1.7 |
| INF-40 | Hold flag | `TicketAssignee.pending` is always false (no API sets a hold); `listTasks?pending=true` returns `[]`. | 6.3 |
| INF-42 | Undocumented permissions | Operations whose description names no permission (`getCurrentUser`, categories/folders reads, routes, workflows, tickets, tasks, files, comments) need only an authenticated active user; ticket-scoped ones additionally apply INF-36. | 1.2 |
| INF-43 | Own-role protection | A caller without super admin membership who updates/deletes a role they belong to, or adds/removes themselves, gets `403 missing_permission`. The super admin role itself is `403 resource_not_editable`; removing its last member is `422 validation_failed`. | 1.8 |
| INF-41 | Default order inside action checks | Tenant `suspended`/`maintenance` and IP checks run in middleware before routing. For approve/reject/deny the engine checks run in this order: ticket exists (404) -> visible (403 `invisible_ticket`) -> tenant mode (403 `admin_is_working`) -> status `in_progress` (422 `invalid_state`) -> caller assigned (403 `user_not_assigned`) -> comment rules (422 `validation_failed`) -> input and assignment checks (422). | 1.7 |

### Carried review findings (session 462 continuation)

Wave-3 integration-review findings carried over from session 460. This session re-reviewed them against the tree at 410d3d8 by static inspection. "Closed when" is the acceptance evidence that the owning plans must produce.

| ID | Sev | Finding and current state at 410d3d8 | Design resolution | Owners | Closed when |
|---|---|---|---|---|---|
| CF-01 | high | **Still open: the previous correction was partial.** `internal/model/enums.go` now has the eight kickflow step types and `activated`, but it still defines invented webhook events (`ticket_submitted`, `ticket_remanded`, `ticket_resubmitted`, `comment_deleted`) and lacks `ticket_updated`, `ticket_opened`, `ticket_confirmed` and `ticket_rejected`. It defines 19 invented permissions (`ticket_admin`, `user_read`, `audit_read`, ...) in place of the kickflow 19. It keeps the non-kickflow constants `StepTypeGrade`/`StepTypeParallel`. `internal/fixtures/validate.go:222` accepts only `user\|team\|grade\|parallel`, so it rejects six valid step types that `fixtures/default/routes.json` uses. `fixtures/default` uses `userType: member`, route/workflow `status: active`, `gradeSymbol: ""` and the invented permissions in `roles.json`. `internal/fixtures/testdata/minimal` seeds user, route and workflow `status: active`. `fixtures/default/workflows.json:519` models a section condition as a separate `sectionType: "conditional"` section, and `model.WorkflowSection` has no `code`, `conditional` or `combinationType` (found while planning; added as the "Section type" row). `model.RouteTarget.GradeSymbol` is a non-nullable `string`, so `routes.json` has to write `""` for "no grade filter". `internal/auth/token_test.go:75,89` uses `user_read`. `TestModelEnumValuesMatchDesign` checks only four enums, using hand-written expectations. Baseline ban-grep hit counts: enums.go 24, roles.json 14, users.json 12, routes.json 4, workflows.json 3, minimal testdata 4, token_test.go 2, validate.go 1. | Enum contract in `design-api-coverage.md` (Go lists, banned values, OpenAPI conformance test, default-fixture validation test) | PF-02, PF-06, PF-07, PF-05 (test seeds) (PF-08/PF-15 consume) | `internal/model/enums_test.go` passes against the OpenAPI; `fixtures.Validate(default)` returns nil; the invalid-testdata cases are rejected; the CF-01 ban grep below prints nothing. It scans production `.go` files and fixture data and excludes `*_test.go` and the PF-06 invalid testdata, because negative tests may name banned values; only production code and fixtures must be clean. The companion presence check confirms that the negative tests still exist |
| CF-02 | high | **Still open: only the PF-02 half is done.** `model.GeneralMasterItem.InputIDs` and the `general_master_items.input_ids` column exist. `internal/fixtures/load.go:261` does not write `input_ids`. `internal/fixtures/export.go:274` drops `MasterInputFixture.ID`. `internal/store/read_masters.go` does not read `input_ids`. `internal/present/master.go:47` emits the field ID as `inputs[].id`. | Input ID data flow in `architecture.md#fixtures` | PF-06, PF-08, PF-20 | `export(load(fixtures/default))` is byte-identical including `inputs[].id`; `getGeneralMasterItem` returns the fixture input ID (not the field ID); `updateGeneralMasterItem` keeps the IDs |
| CF-03 | mid | **Confirmed correct; closes on PF-05 verification.** `internal/store/auth.go:161` writes `model.UserStatusActive` (`activated`). `internal/auth/session.go:17,30` use the constant. The auth tests seed and assert `activated` (`handlers_test.go:81`, `middleware_test.go:28`, `session_test.go:41`). Residual drift outside PF-05: `internal/store/store_test.go:188,281` seed `status "active"` and `user_type "member"` (PF-02-owned test data). | INF-31 activation writes `activated`; the PF-02 store tests use canonical values | PF-05 (verify), PF-02 (seed values) | PF-05 scoped `go test -count=1 -v -race ./internal/auth/...` passes with counts; `rg -n '"active"' internal --glob '*.go'` finds no user-status literal |

CF-01 ban grep:

```bash
rg -n -e 'ticket_submitted|ticket_remanded|ticket_resubmitted|comment_deleted|ticket_admin|audit_read' \
  -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' \
  -e 'StepTypeGrade|StepTypeParallel|StepStatus(Approved|Denied|Remanded)|TicketStatusWithdrawn' \
  -e '"userType": *"member"|"status": *"active"|"sectionType": *"conditional"|"gradeSymbol": *""' \
  internal fixtures --glob '!**/*_test.go' --glob '!internal/fixtures/testdata/invalid/**'
```

Scope: production Go files under `internal/`, the `fixtures/` data, and `internal/fixtures/testdata/minimal`. Test files (`*_test.go`) and the PF-06 invalid-case testdata are excluded on purpose. Negative tests must name banned values in order to prove they are rejected (PF-02 `enum_rules_test.go`, PF-15 payload tests, PF-21 `roles_test.go`, PF-23 comment tests, PF-24 admin webhook tests). Never delete or obfuscate those literals to make this grep pass.

```bash
# companion presence check (PF-27; encoded as repo_invariants_test.go/negative_tests_present in session 463): the negative tests still exist
rg -l 'audit_read' internal/fixtures/enum_rules_test.go internal/api/v1/roles_test.go   # lists both files
```

Status at session 463 (checkpoint `f82e374`; the session-462 integration review accepted PF-01..PF-13 and PF-17):

- CF-01: the PF-02, PF-05, PF-06 and PF-07 halves are accepted. The CF-01 ban grep above was re-run on 2026-10-21 against `f82e374` and printed nothing (rg exit 1, no match). It is still open only for its final closing evidence: PF-27's `repo_invariants_test.go` subtests `cf01_ban` and `negative_tests_present`. The second subtest needs `internal/api/v1/roles_test.go` (PF-21) and the PF-24 admin webhook test, which do not exist yet.
- CF-02: the PF-02, PF-06 and PF-08 halves are accepted. Still open: the PF-20 API hop (`createGeneralMasterItem` / `updateGeneralMasterItem` input IDs) and the PF-16 `TestDefaultRoundTrip`.
- CF-03: closed by the accepted PF-05 at `f82e374`. PF-27 records the PF-05 PASS counts from the full gate.

### Intentional divergences from kickflow

These are scope cuts, not inferences. Each is visible to API clients only as a constant value.

| ID | Divergence | Visible effect |
|---|---|---|
| DIV-01 | Single tenant, no billing, no plans beyond the `standard`/`enterprise` switch | `WebhookTenant` comes from the tenant row |
| DIV-02 | No e-mail, chat, push or reminder notifications | None; invitations are only stored |
| DIV-03 | No integrations (Slack, Teams, Chatwork, Google Chat, CloudSign, kintone, ClimberCloud, Drive, Box, external API buttons, Excel templates) | Their workflow/ticket fields are `null` or empty arrays |
| DIV-04 | No labels, sub-statuses, pipelines, automations, guest forms, custom-step editor, pull-up (raise), approval revoke, bulk approval, saved views | `labels: []`, `subStatus: null`, `triggerTicket: null`, `nextTickets: []`, `ticketGuest: null`; step status `raised` and ticket status `permanently_deleted` are never produced |
| DIV-05 | No workflow/route editor and no versioning UI; each workflow and route has exactly one version from fixtures | `versionNumber: 1`, `current: true` |
| DIV-06 | No section view restrictions and no approver edit restrictions beyond the field `approver`/`author` flags | `viewable: true` for every section |
| DIV-07 | AI features and the kickflow MCP server are out of scope | None |
| DIV-08 | Admin role `restricted` scoping (folders, general masters, teams) is stored and returned but not enforced | A restricted permission behaves like the unrestricted one |
