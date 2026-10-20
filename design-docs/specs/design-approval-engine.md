# Approval Engine Design

Ticket lifecycle, route resolution, step evaluation, form validation and webhook event emission for punchflow.

## Overview

The engine (`internal/engine`) owns every ticket state change. HTTP handlers (`internal/api/v1/tickets.go`, `ticket_actions.go`, `comments.go`) decode requests, call one engine method inside one database transaction, and render the result with `internal/present`. The engine never performs HTTP or webhook I/O; it returns the list of events to enqueue, and the caller writes them to the webhook outbox in the same transaction.

Terminology follows kickflow (see "Terminology mapping" in `notes.md`): `reject` = send back (remand), `deny` = final refusal.

## Technical Details

### Engine surface

```go
// internal/engine (signatures only)
type Engine struct { /* clock kftime.Clock; ids IDGen */ }

func (e *Engine) Create(ctx context.Context, tx *store.Tx, actor Actor, in CreateInput) (ticketID string, evs []Event, err error)
func (e *Engine) Update(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, in UpdateInput) ([]Event, error)
func (e *Engine) UpdateForm(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, in FormPatch) ([]Event, error)
func (e *Engine) Approve(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, comment *string) ([]Event, error)
func (e *Engine) SendBack(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, to int, comment *string) ([]Event, error)
func (e *Engine) Deny(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, comment *string) ([]Event, error)
func (e *Engine) Withdraw(ctx context.Context, tx *store.Tx, actor Actor, ticketID string) ([]Event, error)
func (e *Engine) Archive(ctx context.Context, tx *store.Tx, actor Actor, ticketID string) ([]Event, error)
```

Every method first checks the tenant `admin_working` mode (after existence and visibility, INF-41).

- `Actor` = effective user (after `X-Caller-Id`) plus the permission set.
- `Event` = `{Type, TicketID, CommentID, ActorUserID}`; payload bytes are built by `internal/webhook` from the post-action state (INF-11).
- Errors are `*kferr.Error` values (`status`, `code`, `message`, `fieldErrors`) and map directly to the kickflow error body.
- Time comes from an injected `kftime.Clock` so tests control `openedAt`, `completedAt`, proxy date ranges and numbering.

### Data owned by the engine

| Table | Purpose |
|---|---|
| `tickets` | status, currentStep, workflow id/version snapshot, author, authorTeam + `authorTeamSnapshot`, proxy client, title, ticketNumber, timestamps (`openedAt`, `completedAt`, `deniedAt`, `archivedAt`, `closedAt`), `forcedPublicType`, counters |
| `ticket_inputs` | One row per form field: value JSON plus referenced ids (master items, users, teams, tickets, file signed ids) |
| `ticket_slip_items` | Slip rows with their inputs JSON |
| `ticket_steps` | Built route: route step id, stepOrder (group), displayOrder, title, actionType, required number/percent, status, skip, fallback, fallbackResult, completedAt |
| `ticket_assignees` | Step assignees: user, completed, completedAt, actualApprover, assignedAt (`assignedToMeAt`) |
| `ticket_custom_approvers` | `approvers[]` per route step code (INF-20) |
| `ticket_viewers`, `ticket_links`, `comments`, `comment_attachments` | Viewers, related tickets, comments |

### State machine

| From | Action | Guard | To | Events |
|---|---|---|---|---|
| (none) | Create `status: draft` | INF-28; inputs shape valid | `draft` | `ticket_created` |
| (none) | Create `status: in_progress` | as above + full validation + route resolution | `in_progress` | `ticket_created`, `ticket_opened` |
| `draft` | Update fields | author | `draft` | `ticket_updated` |
| `draft` | Update `status: in_progress` | author; full validation; route resolution | `in_progress` | (`ticket_updated`), `ticket_opened` |
| `rejected` | Update fields | author | `rejected` | `ticket_updated` |
| `rejected` | Update `status: in_progress` (re-submit) | author; full validation; route re-resolved | `in_progress` | (`ticket_updated`), `ticket_opened` |
| `rejected` | Update `status: draft` | - | error `422 invalid_state` (Confirmed rule) | - |
| `in_progress` | Update / UpdateForm by current approver | approver-editable fields only; `overwritable` rule | `in_progress` | `ticket_updated` |
| `in_progress` | Approve | INF-41 order; assignee or proxy | `in_progress` / `completed` | `ticket_approved` or `ticket_confirmed` (+ `ticket_completed`) |
| `in_progress` | SendBack `to: 0` | assignee or proxy; current step `actionType: approve` | `rejected`, currentStep 0 | `ticket_rejected` |
| `in_progress` | SendBack `to: N` | `1 <= N < currentStep` (INF-12) | `in_progress`, currentStep N | `ticket_rejected` |
| `in_progress` | Deny | assignee or proxy; current step `approve` | `denied` (terminal) | `ticket_denied` |
| `in_progress` | Withdraw | author / proxy client (INF-25) | `rejected`, currentStep 0 | `ticket_withdrawn` |
| `draft`, `rejected` | Archive | author, proxy client, or `ticket_write` | `archived` | `ticket_archived` |
| `in_progress`, `completed`, `denied` | Archive | `ticket_write` only | `archived` | `ticket_archived` |
| any other | any | - | `422 invalid_state` (INF-24) | - |

The `status` values `permanently_deleted` and step status `raised` are never produced (DIV-04). In an `Update`, `status` and other fields are ignored for approvers (Confirmed note 2).

Counters: `commentsCount` = comments not soft-deleted; `linkedTicketsCount` = rows in `ticket_links`; `logsCount` = audit log rows that reference the ticket (INF-29).

Timestamps: `openedAt` set on every transition to `in_progress` from draft/rejected; `completedAt` and `closedAt` on completion; `deniedAt` on deny; `archivedAt` and `closedAt` on archive. `updatedAt` changes on every engine action.

### Form validation

Applies to `inputs` and `slipItems` on Create, Update and UpdateForm.

1. Field resolution: `formFieldId` wins over `formFieldCode`; unknown id/code -> `400 invalid_parameter` ("invalid field code"). Duplicates -> `400 invalid_parameter`.
2. Completeness: Create and applicant Update must include every form field of the workflow version (calculation fields with `value: null`); missing -> `400 invalid_parameter`. `inputs` may be omitted only when the workflow has no form fields. Approver Update may include only fields with `approver: true` (or all fields when `overwritable` is true); violations -> `400 invalid_parameter` "invalid field code". UpdateForm accepts any non-empty subset of editable fields; both `inputs` and `slipItems` empty -> `400 invalid_parameter`.
3. Typing per `fieldType`: `text`/`text_long` string (min/max length, `regexpFormat`); `number`/`integer` numeric string or number (min/max value, `decimalDigit`); `checkbox` string array within `options`; `pull_down`/`radio_button` one of `options`; `date` `YYYY-MM-DD`; `datetime` `YYYY-MM-DD HH:mm`; `master` -> `generalMasterItemId` of the field's master; `user`/`team`/`ticket` -> existing ids (`ticket` also `allowedTicketStatus`); `file` -> uploaded signed ids; `calculation` -> stored null (INF-21); `button_api`/`button_kintone` -> stored as given, never executed (DIV-03). Type errors -> `422 validation_failed` with `errors.<fieldCode>`.
4. Required check (only when the ticket is or becomes `in_progress`): every visible section's `required` field (author-side fields on submit) must be non-empty -> `422 validation_failed`. Drafts skip required checks.
5. Section visibility: a form section with `conditional: true` is visible when its `conditionFields` hold under `combinationType` `all`/`any` (`custom` rejected at fixture load, INF-18). Hidden sections are excluded from required checks and returned with `visible: false`.
6. Approve-time check: fields with `approver: true` and `required: true` that are empty -> `422 input_required_before_approve`.
7. Title: `titleInputMode` `input` uses request `title` (title on other modes -> `400 invalid_parameter`); `calculate` -> workflow name (INF-21); `none` -> null.

### Route resolution (on submit and re-submit)

```
branch   := first routeConditions entry whose condition holds   // always | field | field_otherwise
if branch.errorMessage != nil -> 422 validation_failed errors.base (INF-17)
steps    := branch.route.steps ordered by (stepOrder, position)
for each route step s:
    include := evalStepCondition(s.routeStepCondition)          // always | conditional | conditional_skip
    if !include -> ticket step with status skipped, skip=true
    assignees := resolve(s)                                     // see table
    apply allowSelfApproval=false (drop the author), then fallback (INF-19)
group steps by stepOrder; group 1 becomes current
```

Condition operators (`equal`, `not_equal`, `greater_than(_or_equal)`, `less_than(_or_equal)`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal`, `not_descendants_or_equal`) compare: numbers numerically, strings exactly, checkbox arrays by membership (`include`/`exclude`), grades by `level`, teams by identity or ancestry (`descendants_or_equal`), master items by id. Route step condition `fieldKey`: `author_grade`, `author_team`, `text_variable`, `number_variable`, `checkbox_variable`, `general_master_variable` read the author's membership or the form field whose code is `variable`.

| stepType | Assignees |
|---|---|
| `user` | `users` of the route step |
| `author` | The proxy client if present, otherwise the author |
| `manager` | INF-19 with `managerSelectionType` and grade targets |
| `team` | Members of each target team in the active org chart whose membership grade satisfies `gradeSymbol`/`grades`; fallback `higher_team`/`skip` |
| `author_customizable` | `approvers[]` for the step code from the request (INF-20); members must satisfy targets when targets are set |
| `assignee_customizable` | `approvers[]` set later by the previous step's approver (INF-20) |
| `dynamic_user` | Users selected in the form field whose code is the target `variable` |
| `dynamic_team` | Members of the teams selected in the form field `variable`, filtered by grade targets |

A resolved non-skipped step with zero assignees fails submit with `422 validation_failed` (`errors.base`), except `assignee_customizable` steps, which are filled before they become current.

### Step progression

- Current group = lowest `stepOrder` with a step that is not `completed` or `skipped`. `ticket.currentStep` = that `stepOrder`. When a group becomes current, its steps get status `in_progress` and their assignees get `assignedAt`; `actionType: none` steps complete immediately (INF-15).
- Approve: the actor acts for every assignee row of every in-progress step of the current group where `assignee.user == actor` or the actor is an active proxy for `assignee.user` (INF-16). No such row -> `403 user_not_assigned`. Each such row becomes `completed` with `actualApprover = actor`.
- A step completes per INF-14; the group completes when all its non-skipped steps complete; then the next group becomes current, or the ticket becomes `completed`.
- Event type for Approve: `ticket_approved` if any acted row belongs to an `approve` step, otherwise `ticket_confirmed`; `ticket_completed` is appended when the ticket completed.
- Before approving a step that is followed by an `assignee_customizable` step with no approvers -> `422 assignment_required_before_approve`.
- SendBack and Deny are allowed only when the actor holds a row in an `approve` step of the current group (otherwise `403 user_not_assigned`, or `422 invalid_state` when only on `confirm`/`none` steps).
- SendBack resets: `to: 0` resets all steps to `pending` and clears all assignee completions; `to: N` resets steps with `stepOrder >= N`. Re-submit rebuilds steps from scratch (route re-resolved).

### Comments with actions

`comment` on approve/reject/deny: forbidden when `commentingEnabled` is false (`422 validation_failed`, `errors.comment`), required when the matching `commentingRequiredOn*` is true (`422 validation_failed`, `errors.comment`). Stored as a normal comment authored by the actor (INF-22).

### Proxy application

`proxyClientUserId` on Create/Update requires an active `ProxyApplicant` registration with `user = proxy client` and `proxyUser = actor`, valid today (JST) and for the workflow; otherwise `422 validation_failed`. A suspended/deactivated proxy client -> `422 validation_failed` (Confirmed). `author` is the actor and `proxyClientUser` the client.

### Required engine tests (`internal/engine/*_test.go`)

| Test | Scenario |
|---|---|
| TestSubmitMultiStepApproveCompletes | Create in_progress -> approve step 1 -> approve step 2 -> `completed`; events `created, opened, approved, approved, completed` |
| TestDenyIsTerminal | in_progress -> deny -> `denied`; further actions `422 invalid_state` |
| TestSendBackToApplicantAndResubmit | approve step 1 -> reject `to: 0` -> `rejected` -> PATCH fields + `in_progress` -> steps rebuilt; events `rejected, updated, opened` |
| TestSendBackToEarlierStep | reject `to: 1` from step 2 keeps `in_progress`, step 1 re-opened (INF-12) |
| TestWithdraw | author withdraws -> `rejected`; non-author `403` (INF-25) |
| TestParallelAllAndAny | group with an "any" step and an "all" step; group completes only when both complete; one user in both steps approves both with one call |
| TestProxyApproval | proxy approver of the assignee approves; `actualApprover` = proxy; expired proxy registration -> `403 user_not_assigned` |
| TestRouteBranching | amount below/above threshold selects different routes; refusal branch -> `422` (INF-17) |
| TestStepConditionAndFallback | `conditional_skip` step skipped; manager fallback `skip`/`higher_manager` |
| TestCommentRules | required/forbidden action comments; action comment emits `comment_created` |
| TestArchiveRules | per-status archive permissions; archive twice -> `422 invalid_state` |
| TestFormValidation | unknown field code `400`, missing required on submit `422`, draft skips required, approver field rules |
| TestCircularStepAutoComplete | `actionType: none` completes on activation (INF-15) |

## References

- `design-docs/references/kickflow-api.md` 1.8, 1.9, 3 (Tickets), 4 (Route*, Ticket*, Workflow*), 6.3-6.5, 6.9
- `design-docs/specs/notes.md` INF-11 to INF-25, INF-34 to INF-36
- `design-docs/specs/design-api-coverage.md` rows 56-70, event matrix
