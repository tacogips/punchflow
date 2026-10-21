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

### Source files

| Path | Responsibility |
|---|---|
| `internal/engine/engine.go` | Exported surface, `Actor`, input types |
| `create.go`, `update.go`, `submit.go` | Create, Update/UpdateForm, submission (`pf14FinishSubmission`: `openedAt`, numbering, `ticket_opened`/`ticket_completed`), application eligibility (INF-28) and applicant nominations (INF-70) |
| `routing.go`, `dynamic_refs.go`, `conditions.go` | Route branch selection, route building (ends in `pf14Advance`), assignee and fallback resolution, `$field_code` variables (INF-73), condition operators and section visibility conditions |
| `actions.go` | Approve, SendBack, Deny, Withdraw, Archive and the single advancement routine `pf14Advance` |
| `forms.go`, `forms_normalize.go`, `forms_patch.go`, `forms_slips.go`, `forms_calc.go`, `forms_update.go`, `forms_links.go` | Actor-aware normalization of form fields and detail rows, differential `/form` patches, detail-row merge (INF-75), calculation evaluation (INF-21), file attachments and `autoLink` links (INF-76, INF-78) |
| `visibility.go`, `scope.go` | The effective-visibility evaluator `CanView` (INF-36) and restricted `ticket_read` folder scope (INF-84) |
| `numbering.go` | Ticket number allocation at the configured timing (INF-35) |
| `comments.go` | Comments and mentions |
| `substatus.go`, `labels.go`, `controls.go`, `controls_custom_steps.go` | Sub-status sync, labels, cancel approval, raise and custom steps (PF-15, PF-19) |
| `user_cleanup.go` | User/team deletion cleanup and archival lifecycle (INF-79, INF-LIFE-API-05, INF-LIFE-API-06) |
| `internal/formula/` | Formula parser and evaluator used by `forms_calc.go` |
| `internal/access/section.go` | Form-section view restrictions shared with the presenter (INF-77) |

### Data owned by the engine

| Table | Purpose |
|---|---|
| `tickets` | status, currentStep, workflow id/version snapshot, author, authorTeam + `authorTeamSnapshot`, proxy client, title, ticketNumber, timestamps (`openedAt`, `completedAt`, `deniedAt`, `archivedAt`, `closedAt`), `forcedPublicType`, counters |
| `ticket_inputs` | One row per form field: value JSON plus referenced ids (master items, users, teams, tickets, file signed ids) |
| `ticket_slip_items` | Slip rows with their inputs JSON |
| `ticket_steps` | Built route: route step id, stepOrder (group), displayOrder, title, instruction, actionType, required number/percent, status, skip, fallback, fallbackResult, completedAt; team assignments refresh on step entry while manager assignments retain the application snapshot (INF-LIFE-R3-04); custom steps (PF-19) have a null route step id plus `author_id` and `instruction` |
| `labels`, `sub_statuses` | Fixture-defined definitions (`labels.json`, `sub-statuses.json`); read-only at runtime |
| `ticket_labels`, `tickets.sub_status_id` | Labels attached to a ticket and the sub-status attached while the ticket is in progress |
| `ticket_assignees` | Step assignees: user, completed, completedAt, pending hold state, actualApprover, assignedAt (`assignedToMeAt`) |
| `ticket_custom_approvers` | `approvers[]` per route step code (INF-20) |
| `ticket_viewers`, `ticket_links`, `comments`, `comment_attachments` | Viewers, related tickets, comments |

### State machine

| From | Action | Guard | To | Events |
|---|---|---|---|---|
| (none) | Create `status: draft` | INF-28 (the author team must be eligible); inputs shape valid | `draft` | `ticket_created` |
| (none) | Create `status: in_progress` | as above + full validation + route resolution | `in_progress`, or `completed` when no user action is needed (INF-72) | `ticket_created`, `ticket_opened` (+ `ticket_completed`) |
| `draft` | Update fields | author | `draft` | `ticket_updated` |
| `draft` | Update `status` equal to the current status | author | `draft` | `ticket_updated` only when something changed (INF-71) |
| `draft` | Update `status: in_progress` | author; INF-28 re-checked; full validation; route resolution | `in_progress`, or `completed` when no user action is needed | (`ticket_updated`), `ticket_opened` (+ `ticket_completed`) |
| `rejected` | Update fields | author | `rejected` | `ticket_updated` |
| `rejected` | Update `status` equal to the current status | author | `rejected` | `ticket_updated` only when something changed (INF-71) |
| `rejected` | Update `status: in_progress` (re-submit) | author; INF-28 re-checked; full validation; route re-resolved | `in_progress`, or `completed` when no user action is needed | (`ticket_updated`), `ticket_opened` (+ `ticket_completed`) |
| `rejected` | Update `status: draft` | - | error `422 invalid_state` (Confirmed rule) | - |
| `in_progress` | Update / UpdateForm by current approver | approver-editable fields only; `overwritable` rule | `in_progress` | `ticket_updated` |
| `in_progress` | Approve | INF-41 order; assignee or proxy | `in_progress` / `completed` | `ticket_approved` or `ticket_confirmed` for the step acted on, even when the step needs more approvals (INF-74) (+ `ticket_completed`) |
| `in_progress` | SendBack `to: 0` | assignee or proxy; current step `actionType: approve` | `rejected`, currentStep 0 | `ticket_rejected` |
| `in_progress` | SendBack `to: N` | `1 <= N < currentStep` (INF-12) | `in_progress`, currentStep N (the next unskipped group when all of group N was skipped) | `ticket_rejected` |
| `in_progress` | Deny | assignee or proxy; current step `approve` | `denied` (terminal) | `ticket_denied` |
| `in_progress` | Withdraw | author / proxy client (INF-25) | `rejected`, currentStep 0 | `ticket_withdrawn` |
| `draft`, `rejected` | Archive | author, proxy client, or `ticket_write` | `archived` | `ticket_archived` |
| `in_progress`, `completed`, `denied` | Archive | `ticket_write` only | `archived` | `ticket_archived` |
| any other | any | - | `422 invalid_state` (INF-24) | - |

The `status` value `permanently_deleted` is never produced (DIV-04). Step status `raised` is produced only by Raise (see Runtime lifecycle controls). In an `Update`, `status` and other fields are ignored for approvers (Confirmed note 2).

Counters: `commentsCount` = comments not soft-deleted; `linkedTicketsCount` = rows in `ticket_links`; `logsCount` = audit log rows that reference the ticket (INF-29).

Timestamps: `openedAt` set on every transition to `in_progress` from draft/rejected; `completedAt` and `closedAt` on completion; `deniedAt` on deny; `archivedAt` and `closedAt` on archive. `closedAt` is null for every other status, including denied, rejected, and withdrawn (WH-08). `updatedAt` changes on every engine action.

### Form validation

Applies to `inputs` and `slipItems` on Create, Update and UpdateForm. One normalizer (`forms_normalize.go`) serves form fields and detail fields; approvers are limited to `approver: true` fields (or all when the workflow is `overwritable`), applicants may enter every field.

1. Field resolution: `formFieldId` wins over `formFieldCode`; unknown id/code -> `400 invalid_parameter` ("invalid field code"). Duplicates -> `400 invalid_parameter`.
2. Completeness: Create and applicant Update must include every form field of the workflow version (calculation fields with `value: null`); missing -> `400 invalid_parameter`. `inputs` may be omitted only when the workflow has no form fields. Approver Update may include only fields with `approver: true` (or all fields when `overwritable` is true); violations -> `400 invalid_parameter` "invalid field code". UpdateForm accepts any non-empty subset of editable fields; both `inputs` and `slipItems` empty -> `400 invalid_parameter`.
3. Typing per `fieldType`: `text`/`text_long` string (min/max length, `regexpFormat`); `number`/`integer` numeric string or number (min/max value, `decimalDigit`); `checkbox` string array within `options`; `pull_down`/`radio_button` one of `options`; `date` `YYYY-MM-DD`; `datetime` `YYYY-MM-DD HH:mm`; `master` -> `generalMasterItemId` of the field's master; `user`/`team`/`ticket` -> existing ids (`ticket` also `allowedTicketStatus`); `file` -> uploaded signed ids; `calculation` -> evaluated server-side from its `formula`, never entered (INF-21); `button_api`/`button_kintone` -> stored as given, never executed (DIV-03). Type errors -> `422 validation_failed` with `errors.<fieldCode>`.
   Custom validations are evaluated after normalization against the complete resulting form. Each rule combines its conditions with `all` or `any`; when the combination matches, its configured `errorMessage` is added to the field's validation errors. Form conditions resolve workflow form field codes, and detail conditions resolve fields within the same detail row. Fixture validation requires known field codes, supported operators, non-empty conditions and error messages, and enforces the documented workflow limits of 50 form rules and 10 detail rules (INF-CFG-03).
4. Required check (only when the ticket is or becomes `in_progress`): every visible section's `required` field (author-side fields on submit) must be non-empty -> `422 validation_failed`. Drafts skip required checks.
5. Section visibility: a form section with `conditional: true` is visible when its condition fields match `combinationType` `all`/`any`, or the A-J expression in `combinationExpression` when the type is `custom` (INF-CFG-02). Hidden sections are excluded from required checks and returned with `visible: false`.
   Sections with `viewRestriction` are viewable by the applicant or proxy client (`allowViewAuthor`), direct/actual approvers or active workflow/date-matched proxy approvers (`allowViewApprover`), designated `sectionViewers`, and `ticket_read` holders whose grant covers the ticket workflow folder (INF-77, INF-SEC-03, INF-SEC-04). Approver-side access is limited to `in_progress` and `completed`. Values entered for a section the actor cannot view are rejected (`400 invalid_parameter`), stored values are kept on edits and the presenter returns the section with `viewable: false` and no values.
6. Approve-time check: fields with `approver: true` and `required: true` that are empty -> `422 input_required_before_approve`. Detail (slip) cells are checked the same way (INF-69): every stored row of every slip section must have a non-empty value in each `approver: true`, `required: true` cell; hidden and `calculation` cells are exempt.
7. Title: `titleInputMode` `input` uses request `title` (title on other modes -> `400 invalid_parameter`); `calculate` -> the result of `titleFormula`, re-evaluated on every form edit (INF-21; a formula that yields nothing leaves the title unchanged); `none` -> null.
8. Calculation: after normalization the calculation fields, detail calculation cells and the title are evaluated in dependency order. Formula references support kickflow's `$code` and `${code}` forms; `$sum$code` and `$sum${code}` sum a detail column only when its `showTotal` option is enabled. The documented function/operator set and legacy syntax extensions are recorded in INF-21 and INF-CALC-01. Unsupported syntax, unknown references and cycles are `422 validation_failed`; runtime failures store `null`. The results feed route condition matching (INF-21).

Reference IDs are stored separately from the reference-record snapshots shown in `value` (INF-FRM-01); resolved reference arrays remain live. Master formula records expose custom values at their field-code keys at the record root (INF-FRM-02). Formula lookup resolves metadata, then workflow fields, then fixture-backed numeric globals (INF-FRM-03). Validation and conditional completeness/radio checks use actor-filtered form values for inaccessible sections, while carrying their stored values forward (INF-FRM-04, INF-FRM-05). Approver edits cannot remove persisted detail rows on either ticket or form patch (INF-FRM-06).
9. Detail rows: `PATCH /v1/tickets/{id}` replaces every row, `PATCH /v1/tickets/{id}/form` replaces only the supplied detail sections and preserves omitted sections and omitted cells the actor cannot enter (INF-75). Required detail cells are checked on submission, including stored rows, and approver cells on approval (INF-69). `file` values and `autoLink` ticket values maintain attachments and reciprocal links (INF-76, INF-78).

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

Condition operators (`equal`, `not_equal`, `greater_than(_or_equal)`, `less_than(_or_equal)`, `include`, `exclude`, `is_empty`, `is_not_empty`, `descendants_or_equal`, `not_descendants_or_equal`) compare: numbers numerically, strings exactly, checkbox arrays by membership (`include`/`exclude`), grades by `level`, teams by identity or ancestry (`descendants_or_equal`), master items by id. The pure variable lookup and comparison implementation is shared from `internal/conditions` by engine and ticket presenter, including numeric comparisons and normalized `$field` references (INF-SEC-06). Route step condition `fieldKey`: `author_grade`, `author_team`, `text_variable`, `number_variable`, `checkbox_variable`, `general_master_variable` read the author's membership or the form field whose code is `variable`.

Restricted fields remain available to route-condition selection under the applicant context. In the separate route-step context used for execution conditions, dynamic user/team assignment and fallback resolution they are present as known-empty values, so `is_empty` and `is_not_empty` conditions evaluate consistently (INF-SEC-05, INF-RTE-01). Applicant-team condition codes resolve against the active organization chart; master conditions resolve selected item IDs to the configured master/item codes and compare multiple selections by membership. `other_variable` conditions use the same variable lookup as other form-backed route conditions (INF-RTE-02).

Applicant and previous-approver nominations reject inactive users and users outside configured team/grade targets. Explicit previous-approver nominations enforce `minCustomAssignees`; zero allows an empty nomination and skips that step, while a positive minimum requires that many users. An omitted previous-step nomination remains available for runtime approver customization (INF-RTE-03, INF-RTE-04). Manager steps with self-approval enabled assign the applicant after manager search and configured fallbacks yield no candidate (INF-RTE-05). Advancement and send-back share step entry, including team assignment refresh and sharing synchronization (INF-RTE-06). Approval-only teams cannot be selected as the application team at creation or submission (INF-RTE-07).

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

A resolved non-skipped step with zero assignees takes its `fallbackType` assignees (`direct_manager`, `higher_manager`, `higher_team`); when that yields nobody, fallback `skip` skips the step, while `no_fallback` and no usable fallback materialize an empty waiting step. That step remains `in_progress`, leaves the ticket unassigned, and is never auto-completed, including in a parallel group (INF-19). Exceptions: `assignee_customizable` steps are filled before they become current, and an `author_customizable` step with no nominations is skipped when its minimum is 0 (otherwise `422`, "custom approver is required"). `dynamic_user`/`dynamic_team` targets read the form field named by `variable` (`$code`, `${code}` and `code` are equivalent, INF-73) and accept user or team codes or IDs.

### Step progression

- One routine, `pf14Advance`, moves a ticket forward. It is used by route building (submission), Approve, Raise, cancel approval, send back over skipped groups and user deletion cleanup. Current group = lowest `stepOrder` with a step that is not finished (`completed`, `skipped` or `raised`). `ticket.currentStep` = that `stepOrder`. When a group becomes current, its steps get status `in_progress` and their assignees get `assignedAt`; `actionType: none` steps complete immediately (INF-15) and the routine continues to the next group. When no unfinished group remains the ticket becomes `completed`.
- Auto-advancement on build: `pf14BuildRoute` materializes all ticket steps (with skipped steps already marked) and then calls `pf14Advance` from `currentStep` 0, so submission lands on the first group that needs a user, or completes the ticket at once when none does (INF-72). `pf14FinishSubmission` then records `openedAt`, allocates the number at the `opened` (and, if completed, `completed`) timing and returns `ticket_opened` (+ `ticket_completed`).
- Approve: the actor acts for every assignee row of every in-progress step of the current group where `assignee.user == actor` or the actor is an active proxy for `assignee.user` (INF-16). No such row -> `403 user_not_assigned`. Each such row becomes `completed` with `actualApprover = actor`.
- A step completes per INF-14; the group completes when all its non-skipped steps complete; then the next group becomes current, or the ticket becomes `completed`.
- Event type for Approve: `ticket_approved` if any acted row belongs to an `approve` step, otherwise `ticket_confirmed`, whether or not the step reached its threshold (INF-74); `ticket_completed` is appended when the ticket completed.
- Before approving a step that is followed by an `assignee_customizable` step with no approvers -> `422 assignment_required_before_approve`.
- SendBack and Deny are allowed only when the actor holds a row in an `approve` step of the current group (otherwise `403 user_not_assigned`, or `422 invalid_state` when only on `confirm`/`none` steps).
- SendBack resets: `to: 0` resets all steps to `pending` and clears all assignee completions; `to: N` resets steps with `stepOrder >= N`. Steps that were `skipped` stay skipped (they are never approvable again), and when every step of group `N` was skipped the ticket advances to the next group that needs an approval. Re-submit rebuilds steps from scratch (route re-resolved).

### Labels and sub-status (PF-15)

Definitions come from the fixtures `labels.json` (`id`, `name`, `description`, `color` as six lowercase hex digits) and `sub-statuses.json` (`id`, `code`, `name`, `notes`, `allowWithdrawal`, `allowRejection`, `allowDenial`). A route step references a sub-status by `subStatus: "<code>"`; a workflow with `ticketNumberingTiming: "sub_status_attached"` may name the numbering sub-status in `ticketNumberingSubStatus`. The loader rejects duplicate names/codes, malformed colors and unknown codes, and `fixtures export` round-trips both files. Kickflow has no label or sub-status CRUD in the public API, so neither has one here.

- Attachment (INF-60): when the ticket reaches a step group with a configured sub-status it attaches that value; a group without one preserves the current value. Any terminal transition (completed, denied, withdrawn/rejected, archived, including user-deletion archive) clears it. Mid-route send back restores the target group's configured sub-status, retaining the current value if the target has none; resubmission applies the first group's value when configured. `Ticket.subStatus` is otherwise null.
- Restrictions (INF-61): withdraw, send back (`reject`, any `to`) and deny are refused with `422 invalid_state` ("... is not allowed while the ticket has the sub-status <name>") when the current sub-status sets the matching `allow*` flag to false. The check sits after the actor checks and before comment validation, and applies to UI and API callers alike. Archive and approve are never restricted.
- Numbering (INF-68): `sub_status_attached` allocates the number the first time a sub-status is attached (the configured `ticketNumberingSubStatus` when set, any sub-status otherwise). A ticket that never receives such a sub-status is never numbered.
- Presentation: `Ticket.subStatus` is a `SubStatus` object or null and `Ticket.labels` the attached `Label` objects ordered by name, in `getTicket`, `listTickets`, `TicketWithStep`, `TicketDetail` and webhook payloads. `RouteStep.subStatus` and `TicketNumberingTiming.subStatus` are objects, too. `listTickets` filters by `subStatusIds[]`; kickflow documents no label filter, so none exists.
- Labels (INF-64): any user who can view a ticket may attach or detach labels through the UI API; a real change emits `ticket_updated`, a repeated attach/detach emits nothing.

### Numbering groups and custom numbering (INF-CFG-04, INF-CFG-05)

Standard ticket numbers use the workflow counter unless `ticketNumberKey` is configured, in which case allocation increments the shared group counter inside the ticket transaction. Custom numbering definitions and per-workflow settings are fixture-backed. Each setting selects its own issuance timing and format; values persist by ticket and custom-numbering ID, so REST ticket details and webhook bodies share the same presentation. Creation-timed values are allocated after inserting the ticket parent row, in the same transaction, to satisfy the ticket foreign key. A custom numbering's optional `ticketNumberKeyId` shares its counter; otherwise the numbering ID scopes the counter. The workflow response includes the group object, group next value and populated custom numbering/settings arrays (INF-NUM-01).

### Runtime lifecycle controls (PF-19)

`approvalCancellable`, step `allowRaising` and `allowCustomSteps` drive four engine operations. kickflow documents no REST endpoint for them, so they are exposed only through the punchflow UI API under `/ui/api/` (INF-67), never under `/v1`:

| Operation | UI API | Eligibility | Effect | Event |
|---|---|---|---|---|
| `CancelApproval` | `POST /ui/api/tickets/{id}/cancel-approval` | Ticket `in_progress`, workflow `approvalCancellable`; caller is the assignee or an active proxy for a completed assignment in the current group or immediately previous group, and nobody in the following group has approved (INF-62) | The authorized assignment is reopened; the canceled group becomes current and later in-progress or auto-completed steps return to `pending` | `ticket_updated` |
| `Raise` | `POST /ui/api/tickets/{id}/raise` `{to?}` | Ticket `in_progress`; the caller is assigned (directly or as active proxy) to a later pending group with `allowRaising`, regardless of action type (INF-65) | Later assignment authorizes the operation; exactly the current group is marked `raised` and the next unfinished group becomes current. Repeated calls are required to raise more groups. | No webhook event (INF-66); audit log is retained |
| `AddCustomStep` | `POST /ui/api/tickets/{id}/custom-steps` `{title, actionType, userIds, instruction?, afterStepOrder?}` | Ticket `in_progress`, workflow `allowCustomSteps`, caller is the applicant or an incomplete assignee of the current group (INF-63) | A new group is inserted after `afterStepOrder` (default: the current group); later groups shift by one; `displayOrder` is renumbered; the step has a null `routeStepId`, `author` = caller, all assignees must act | `ticket_updated` |
| `RemoveCustomStep` | `DELETE /ui/api/tickets/{id}/custom-steps/{stepId}` | The author of a still `pending` custom step | The step is deleted and the order gap closed | `ticket_updated` |

`GET /ui/api/tickets/{id}/controls` returns the same eligibility as booleans (`cancelApproval`, `raise` + `raiseTargetStepOrders`, `addCustomStep` + `customStepInsertAfter`, `removableCustomStepIds`) so the UI shows only valid buttons. Raised steps count as finished for progression (`pf15StepDone`), and the assignment check before approving matches the next group to route steps by step code because custom steps shift ticket step orders away from route orders. Every UI API mutation writes an audit log (`ticket.label_attach`, `ticket.label_detach`, `ticket.cancel_approval`, `ticket.raise`, `ticket.add_custom_step`, `ticket.remove_custom_step`).

`POST /ui/api/tickets/{id}/assignees` lets a `ticket_write` administrator add a distinct active user to an active step in the current group, even when the step already has assignments. Existing suspended assignments are retained, and duplicate assignments are rejected (INF-NUM-06). Raising still has no webhook event, but local approval-request notifications are generated independently for the newly current approvers (INF-LIFE-R3-16).

### Assignee holds

`TicketAssignee.pending` is persisted and included in REST and webhook ticket-step presentations. The current unfinished assignee or an active proxy can set or release it with `POST /ui/api/tickets/{ticketId}/assignees/{assigneeId}/pending` and `{pending: boolean}`. The assignment may belong to any active step in the current parallel group; a proxy changes the primary assignment's state (INF-NUM-05). Holds do not block approval or send-back; the state clears when the ticket leaves `in_progress` or the active step changes. `GET /v1/tickets/tasks?pending=true` returns current unfinished held assignments. Kickflow says a hold suppresses reminders, but punchflow does not send reminder notifications (INF-LIFE-WH13).

### User deletion cleanup

`Engine.CleanupDeletedUser` runs in the transaction of `DELETE /v1/users/{id}` (INF-79). It archives the user's `draft`, `in_progress` and `rejected` tickets (`ticket_archived` each) and removes the user's unfinished assignments on other in-progress tickets. Deletion never applies fallback, skips a step, or completes a ticket because the approval obligation remains unresolved; administrators repair an empty current step with the UI helper above (INF-LIFE-API-06). Suspension leaves assignments intact. `Engine.CleanupDeletedTeam` archives unfinished tickets authored from a deleted team through the same lifecycle state updates and event outbox, while team deletion reparents direct child teams to the deleted team's parent (INF-LIFE-API-05).

### Comments with actions

`comment` on approve/reject/deny: forbidden when `commentingEnabled` is false (`422 validation_failed`, `errors.comment`), required when the matching `commentingRequiredOn*` is true (`422 validation_failed`, `errors.comment`). Stored as a normal comment authored by the actor (INF-22).

### Proxy application

`proxyClientUserId` on Create/Update requires an active `ProxyApplicant` registration with `user = proxy client` and `proxyUser = actor`, valid today (JST) and for the workflow; an empty `workflowIds` list allows every workflow (INF-RTE-08); otherwise invalid registration produces `422 validation_failed`. The effective client is revalidated on every applicant draft/rejected save and submission, even when the request omits the field; suspended/deactivated clients produce `422 validation_failed`. Explicit `null` removes the client. `author` is the actor and `proxyClientUser` the client.

Proxy approver action authority is checked at action time and is available only while the original assignee is activated. Proxy settings add the proxy as a persisted ticket viewer and watcher for matching processing tickets; submission and subsequent step progression also apply active settings to the ticket's assigned users. Removing or expiring a proxy setting leaves the already persisted sharing and watch state in place. Completed historical tickets are not backfilled when a proxy setting is added. See INF-ACC-02.

`visibleToManager: direct` snapshots active leaders from the applicant's team as ticket viewers at submission. `visibleToManager: all` also snapshots leaders from each ancestor team. The manager audience is re-added when a later approval step becomes active; historical viewers remain on the ticket. See INF-ACC-03.

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
