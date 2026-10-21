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
// Labels and runtime lifecycle controls (PF-15, PF-19); UI API only, see below.
func (e *Engine) AttachLabel(ctx context.Context, tx *store.Tx, actor Actor, ticketID, labelID string) ([]Event, error)
func (e *Engine) DetachLabel(ctx context.Context, tx *store.Tx, actor Actor, ticketID, labelID string) ([]Event, error)
func (e *Engine) CancelApproval(ctx context.Context, tx *store.Tx, actor Actor, ticketID string) ([]Event, error)
func (e *Engine) Raise(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, to *int) ([]Event, error)
func (e *Engine) AddCustomStep(ctx context.Context, tx *store.Tx, actor Actor, ticketID string, in CustomStepInput) ([]Event, error)
func (e *Engine) RemoveCustomStep(ctx context.Context, tx *store.Tx, actor Actor, ticketID, stepID string) ([]Event, error)
func (e *Engine) Controls(ctx context.Context, q store.Querier, actor Actor, ticketID string) (TicketControls, error)
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
| `ticket_steps` | Built route: route step id, stepOrder (group), displayOrder, title, actionType, required number/percent, status, skip, fallback, fallbackResult, completedAt; custom steps (PF-19) have a null route step id plus `author_id` and `instruction` |
| `labels`, `sub_statuses` | Fixture-defined definitions (`labels.json`, `sub-statuses.json`); read-only at runtime |
| `ticket_labels`, `tickets.sub_status_id` | Labels attached to a ticket and the sub-status attached while the ticket is in progress |
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

The `status` value `permanently_deleted` is never produced (DIV-04). Step status `raised` is produced only by Raise (see Runtime lifecycle controls). In an `Update`, `status` and other fields are ignored for approvers (Confirmed note 2).

Counters: `commentsCount` = comments not soft-deleted; `linkedTicketsCount` = rows in `ticket_links`; `logsCount` = audit log rows that reference the ticket (INF-29).

Timestamps: `openedAt` set on every transition to `in_progress` from draft/rejected; `completedAt` and `closedAt` on completion; `deniedAt` on deny; `archivedAt` and `closedAt` on archive. `updatedAt` changes on every engine action.

### Form validation

Applies to `inputs` and `slipItems` on Create, Update and UpdateForm.

1. Field resolution: `formFieldId` wins over `formFieldCode`; unknown id/code -> `400 invalid_parameter` ("invalid field code"). Duplicates -> `400 invalid_parameter`.
2. Completeness: Create and applicant Update must include every form field of the workflow version (calculation fields with `value: null`); missing -> `400 invalid_parameter`. `inputs` may be omitted only when the workflow has no form fields. Approver Update may include only fields with `approver: true` (or all fields when `overwritable` is true); violations -> `400 invalid_parameter` "invalid field code". UpdateForm accepts any non-empty subset of editable fields; both `inputs` and `slipItems` empty -> `400 invalid_parameter`.
3. Typing per `fieldType`: `text`/`text_long` string (min/max length, `regexpFormat`); `number`/`integer` numeric string or number (min/max value, `decimalDigit`); `checkbox` string array within `options`; `pull_down`/`radio_button` one of `options`; `date` `YYYY-MM-DD`; `datetime` `YYYY-MM-DD HH:mm`; `master` -> `generalMasterItemId` of the field's master; `user`/`team`/`ticket` -> existing ids (`ticket` also `allowedTicketStatus`); `file` -> uploaded signed ids; `calculation` -> stored null (INF-21); `button_api`/`button_kintone` -> stored as given, never executed (DIV-03). Type errors -> `422 validation_failed` with `errors.<fieldCode>`.
4. Required check (only when the ticket is or becomes `in_progress`): every visible section's `required` field (author-side fields on submit) must be non-empty -> `422 validation_failed`. Drafts skip required checks.
5. Section visibility: a form section with `conditional: true` is visible when its `conditionFields` hold under `combinationType` `all`/`any` (`custom` rejected at fixture load, INF-18). Hidden sections are excluded from required checks and returned with `visible: false`.
6. Approve-time check: fields with `approver: true` and `required: true` that are empty -> `422 input_required_before_approve`. Detail (slip) cells are checked the same way (INF-69): every stored row of every slip section must have a non-empty value in each `approver: true`, `required: true` cell; hidden and `calculation` cells are exempt.
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

### Labels and sub-status (PF-15)

Definitions come from the fixtures `labels.json` (`id`, `name`, `description`, `color` as six lowercase hex digits) and `sub-statuses.json` (`id`, `code`, `name`, `notes`, `allowWithdrawal`, `allowRejection`, `allowDenial`). A route step references a sub-status by `subStatus: "<code>"`; a workflow with `ticketNumberingTiming: "sub_status_attached"` may name the numbering sub-status in `ticketNumberingSubStatus`. The loader rejects duplicate names/codes, malformed colors and unknown codes, and `fixtures export` round-trips both files. Kickflow has no label or sub-status CRUD in the public API, so neither has one here.

- Attachment (INF-60): after every engine action the sub-status is recomputed from the ticket state by `pf15SyncSubStatus`. While the ticket is `in_progress` it is the sub-status of the first step (by display order) of the group that is currently waiting; a group whose steps configure none clears it. Any other ticket status carries no sub-status (`Ticket.subStatus` is "set only while in processing"). Send back to step N re-applies the sub-status of step N; resubmission applies the first group's.
- Restrictions (INF-61): withdraw, send back (`reject`, any `to`) and deny are refused with `422 invalid_state` ("... is not allowed while the ticket has the sub-status <name>") when the current sub-status sets the matching `allow*` flag to false. The check sits after the actor checks and before comment validation, and applies to UI and API callers alike. Archive and approve are never restricted.
- Numbering (INF-68): `sub_status_attached` allocates the number the first time a sub-status is attached (the configured `ticketNumberingSubStatus` when set, any sub-status otherwise). A ticket that never receives such a sub-status is never numbered.
- Presentation: `Ticket.subStatus` is a `SubStatus` object or null and `Ticket.labels` the attached `Label` objects ordered by name, in `getTicket`, `listTickets`, `TicketWithStep`, `TicketDetail` and webhook payloads. `RouteStep.subStatus` and `TicketNumberingTiming.subStatus` are objects, too. `listTickets` filters by `subStatusIds[]`; kickflow documents no label filter, so none exists.
- Labels (INF-64): any user who can view a ticket may attach or detach labels through the UI API; a real change emits `ticket_updated`, a repeated attach/detach emits nothing.

### Runtime lifecycle controls (PF-19)

`approvalCancellable`, step `allowRaising` and `allowCustomSteps` drive four engine operations. kickflow documents no REST endpoint for them, so they are exposed only through the punchflow UI API under `/ui/api/` (INF-67), never under `/v1`:

| Operation | UI API | Eligibility | Effect | Event |
|---|---|---|---|---|
| `CancelApproval` | `POST /ui/api/tickets/{id}/cancel-approval` | Ticket `in_progress`, workflow `approvalCancellable`, the caller is the actual approver of a step, and no step after it has a completed assignee (INF-62) | The caller's assignee rows of their latest approved group are reopened and that group becomes current; later in-progress or auto-completed steps return to `pending` | `ticket_updated` |
| `Raise` | `POST /ui/api/tickets/{id}/raise` `{to?}` | Ticket `in_progress`; the caller (or a user they are proxy for) is an incomplete assignee of a later `pending` step that has `allowRaising` and needs action (INF-65) | Unfinished steps from the current group up to the target are marked `raised`; the target group becomes current through the shared advancement routine | `ticket_updated` |
| `AddCustomStep` | `POST /ui/api/tickets/{id}/custom-steps` `{title, actionType, userIds, instruction?, afterStepOrder?}` | Ticket `in_progress`, workflow `allowCustomSteps`, caller is the applicant or an incomplete assignee of the current group (INF-63) | A new group is inserted after `afterStepOrder` (default: the current group); later groups shift by one; `displayOrder` is renumbered; the step has a null `routeStepId`, `author` = caller, all assignees must act | `ticket_updated` |
| `RemoveCustomStep` | `DELETE /ui/api/tickets/{id}/custom-steps/{stepId}` | The author of a still `pending` custom step | The step is deleted and the order gap closed | `ticket_updated` |

`GET /ui/api/tickets/{id}/controls` returns the same eligibility as booleans (`cancelApproval`, `raise` + `raiseTargetStepOrders`, `addCustomStep` + `customStepInsertAfter`, `removableCustomStepIds`) so the UI shows only valid buttons. Raised steps count as finished for progression (`pf15StepDone`), and the assignment check before approving matches the next group to route steps by step code because custom steps shift ticket step orders away from route orders. Every UI API mutation writes an audit log (`ticket.label_attach`, `ticket.label_detach`, `ticket.cancel_approval`, `ticket.raise`, `ticket.add_custom_step`, `ticket.remove_custom_step`).

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

The table above lists scenarios. The PF-14 plan names the tests that cover them with the default fixtures. `TestCircularStepAutoComplete` is covered by `TestLeaveAssigneeCustomizableAndAuthorConfirm` (step 3, `none`); `TestArchiveRules` is `TestArchive`; `TestStepConditionAndFallback` is added by PF-14b (below).

### Plan ownership (session 465)

PF-14 is split into PF-14a, PF-14b and PF-14c, and each owns whole files (`architecture.md`, "Plan split, evidence-reporting and wave rule (session 465)"). The engine surface in `engine.go` (PF-14a) is unchanged: every exported identifier and signature stays as it is at `ea385c7`.

| Plan | Behavior owned | Tests that must PASS by name |
|---|---|---|
| PF-14a | State machine, Approve/SendBack/Deny/Withdraw/Archive, step progression (`pf14Advance`), action comments, comments with mentions, visibility, numbering, the INF-41 check order | `TestSubmitMultiStepApproveCompletes`, `TestDenyIsTerminal`, `TestSendBackToApplicantAndResubmit`, `TestSendBackToEarlierStep`, `TestWithdraw`, `TestParallelAllAndAny`, `TestProxyApproval`, `TestRouteBranching` (`lifecycle_test.go`); `TestCreationRestricted`, `TestAdminWorking`, `TestVisibility`, `TestArchive` (`events_test.go`); `TestCommentRules`, `TestComments` (`comments_test.go`) |
| PF-14b | "Route resolution": branch selection, step conditions, the condition operators, assignee resolution for every `stepType`, fallbacks, and section-visibility condition matching | `TestPurchaseConditionsAndNomination`, `TestLeaveAssigneeCustomizableAndAuthorConfirm` (`routing_test.go`); new `TestStepConditionAndFallback` (a `conditional_skip` step is skipped; a `manager` step whose author team has no leader with a matching grade falls back to `higher_manager` and walks up the ancestry; fallback `skip` marks the step skipped; a `team` target with `descendants: true` includes child-team members) and new `TestConditionOperators` (table test for every operator in "Route resolution" over number, string, checkbox, grade `level`, team ancestry and master item values) (`conditions_test.go`) |
| PF-14c | "Form validation" rules 1-7, Create, Update, UpdateForm, the proxy application rule | `TestFormValidation` (`forms_test.go`); new `TestFormFieldTypes` (one valid and one invalid value per `fieldType`, including unknown `user`/`team`/`ticket`/master item ids and unknown file signed ids -> `422 validation_failed` with `errors.<fieldCode>`), `TestProxyApplicantCreate` (valid registration -> author = actor, `proxyClientUser` = client; expired, other-workflow or missing registration and a suspended client -> `422 validation_failed`) and `TestUpdateFormMerge` (`UpdateForm` with a subset keeps the other fields' values and referenced ids) (`create_update_test.go`) |

Frozen cross-file helpers. One plan's file calls them and another plan owns them. Their signatures stay fixed, and so do their results for inputs that already work at `ea385c7`. A plan that needs different behavior adds a new helper in its own file. Each helper is listed under the plan that owns the file that defines it. The list comes from a grep at `ea385c7` of every `pf14*`/`mustActiveChart`/`assertPF14*` call and every `store.Eng*` call in `internal/engine/*.go`, matched to the file that defines it. Every function that is defined in one split plan's file and called from another split plan's file is listed. A few same-plan helpers are listed too, which does no harm.

- PF-14a owns:
  - `actions.go`: `pf14Guard` (called by comments, update), `pf14Event` (comments, create, update), `pf14CanAct` (update), `pf14Advance`.
  - `numbering.go`: `pf14Number` (create, update).
  - `engine.go`: the whole exported surface. It calls PF-14c's `pf14Create`/`pf14Update`/`pf14UpdateForm`, which are frozen below.
  - `internal/store/tickets.go`, which routing, create and update call: `EngLoadTicket`, `EngInsertTicket`, `EngUpdateTicket`, `EngTicketInputs`, `EngReplaceInputs`, `EngReplaceSlipItems`, `EngTicketSteps`, `EngDeleteTicketSteps`, `EngInsertStep`, `EngUpdateStep`, `EngStepAssignees`, `EngInsertAssignee`, `EngUpdateAssignee`, `EngInsertCustomApprovers`, `EngCustomApprovers`, plus the comment and link functions `EngAddComment`, `EngLoadComment`, `EngUpdateComment`, `EngAttachCommentFiles`, `EngLinkTickets`.
  - Test helpers: `pf14Setup`, `pf14TestEnv` and its methods (`setup_test.go`), `pf14CreateWorkflowTicket`, `pf14EventTypes`, `assertPF14FieldError` (`comments_test.go`), `assertPF14ErrorCode` (`events_test.go`).
- PF-14b owns:
  - `routing.go`: `pf14BuildRoute` (create, update), `pf14Contains` (comments, visibility).
  - `conditions.go`: `pf14InputValues`, `pf14SectionVisible`, `pf14FieldValue`, `pf14Float` (all called by forms), `pf14ConditionMatch`.
  - `internal/store/engine_lookup.go`: `EngTenantMode`, `EngActiveProxyApprovers` (actions), `EngUserIsActiveMember` (create, update, visibility), `EngUsersByTeam` (visibility), `EngActiveMemberships`, `EngTeamAncestors`, `EngLeadersOfTeam`, `EngSignedFileExists`.
  - New operators extend `pf14ConditionMatch`; an operator that works today keeps its result. `pf14Float` keeps its numeric parsing. Grade-level comparison goes in a new PF-14b helper.
- PF-14c owns:
  - `create.go`/`update.go`: `pf14Create`, `pf14Update`, `pf14UpdateForm` (all called by `engine.go`), `mustActiveChart`, `pf14ErrorContext`, `pf14InputsFromRows` (`update.go`; called by forms).
  - `forms.go`: `pf14ValidateApprovalInputs` (actions), `pf14NormalizeForm`.
  - Test helpers: `pf14CompleteInputs` (also called by `setup_test.go`, `lifecycle_test.go`, `routing_test.go`), `pf14FormDefinition` (also called by `routing_test.go`), `pf14Input` (`forms_test.go`).
  - Its new reference and proxy-applicant lookups go in the new `internal/store/engine_refs.go` with the `Eng` prefix.
- New test helpers use a plan prefix (`pf14b...`, `pf14c...`) and live in the plan's own test files. No split plan edits `setup_test.go` except PF-14a.

## References

- `design-docs/references/kickflow-api.md` 1.8, 1.9, 3 (Tickets), 4 (Route*, Ticket*, Workflow*), 6.3-6.5, 6.9
- `design-docs/specs/notes.md` INF-11 to INF-25, INF-34 to INF-36
- `design-docs/specs/design-api-coverage.md` rows 56-70, event matrix
