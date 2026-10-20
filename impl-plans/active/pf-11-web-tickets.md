# PF-11 Web Ticket Screens Implementation Plan

**Status**: Ready
**Wave**: 2 | **Depends On**: PF-03 | **Plan ID**: PF-11
**Design Reference**: design-docs/specs/architecture.md#web-ui-web (screens: dashboard, ticket search, create/edit, detail); design-docs/specs/notes.md "Terminology mapping"
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Fill the ticket-related page stubs created by PF-03 with working Solid.js screens that call the kickflow-compatible `/v1` API through `web/src/api/client.ts`. Users must be able to see their tickets and approval requests, search tickets, create/edit/submit a ticket from a workflow's form definition, and act on a ticket. Actions: approve, send back (`reject` with a `to` step), deny, withdraw, archive, plus comments, attachments, viewers and related tickets.

## Non-goals

- No new npm packages (`web/package.json`/`bun.lock` are frozen). No changes to `App.tsx`, `api/*`, `lib/form-model.ts`, `lib/format.ts`, `lib/link.ts` (log reconciliation requests instead).
- No component test framework; tests only for pure TS logic.

## Files

`web/src/pages/Dashboard.tsx`, `web/src/pages/tickets/TicketSearch.tsx`, `web/src/pages/tickets/TicketDetail.tsx`, `web/src/pages/tickets/TicketEdit.tsx`, `web/src/components/tickets/` (new components: `FormFields.tsx`, `SlipTable.tsx`, `StepList.tsx`, `ActionBar.tsx`, `Comments.tsx`, `Viewers.tsx`), `web/src/lib/ticket-view.ts`, `web/src/lib/ticket-view.test.ts`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-11-web-tickets.md`
- `web/src/components/tickets/FormFields.tsx`
- `web/src/components/tickets/SlipTable.tsx`
- `web/src/components/tickets/StepList.tsx`
- `web/src/components/tickets/ActionBar.tsx`
- `web/src/components/tickets/Comments.tsx`
- `web/src/components/tickets/Viewers.tsx`
- `web/src/lib/ticket-view.ts`
- `web/src/lib/ticket-view.test.ts`

### sharedPaths
- `web/src/pages/Dashboard.tsx`
- `web/src/pages/tickets/TicketSearch.tsx`
- `web/src/pages/tickets/TicketDetail.tsx`
- `web/src/pages/tickets/TicketEdit.tsx`

### sharedPathNotes
- `web/src/pages/Dashboard.tsx`: PF-03 stub; replace with the dashboard screen, keep the default export
- `web/src/pages/tickets/TicketSearch.tsx`: PF-03 stub; replace with the search screen, keep the default export
- `web/src/pages/tickets/TicketDetail.tsx`: PF-03 stub; replace with the detail screen, keep the default export
- `web/src/pages/tickets/TicketEdit.tsx`: PF-03 stub; replace with the create/edit screen, keep the default export

### artifactRoots
- (none; `web/node_modules` belongs to PF-03 and is read-only here)

## Screen Behavior

- Dashboard: "My tickets" = `GET /v1/tickets?authorId=<me>&perPage=10`; "Approval requests" = `GET /v1/tickets/tasks?perPage=10`. Links to detail.
- TicketSearch: filters `status[]` (multi), `workflowId` (from `GET /v1/workflows`), `ticketNumber`, `createdAtStart/End`; pagination using `apiList` (`Total`, next/prev links).
- TicketEdit (`/tickets/new?workflow=<id>` and `/tickets/:id/edit`): load `GET /v1/workflows/{id}` (WorkflowDetail) and the user's teams `GET /v1/users/{me}/teams`. Render `sectionList` form fields by `fieldType` (text, text_long, number, integer, checkbox, pull_down, radio_button, date, datetime, master -> select from `GET /v1/generalMasters/{id}/items`, user/team/ticket -> id text input with lookup, file -> `apiUpload('/v1/files')`, calculation -> read-only). Hide sections whose display condition fails (simple equal/not_equal/include evaluation; otherwise show). Slip sections as editable rows. Approver inputs for `author_customizable` steps (route step code + user ids). Optional proxy client user id. "Save draft" -> status `draft`; "Submit" -> `in_progress`. For edit, PATCH `/v1/tickets/{id}`. Show 4xx `KickflowError.errors` next to the fields (by field code) and `errors.base` at the top.
- TicketDetail: `GET /v1/tickets/{id}`; show header (number, title, status, workflow, author, timestamps), inputs, slip items, steps with assignees, `proxyUserIds`, `actualApprover`, and step status. Action bar: Approve (comment), Send back (select step 0..currentStep-1, comment), Deny (comment), Withdraw, Archive, Edit. Comments list with post (`POST .../comments` with files), edit and delete (own comments). Viewers list with add (user id or team id + grade + descendants) and remove. Related tickets (`GET .../links`). Refresh after each action.

## Contracts (`lib/ticket-view.ts`)

```ts
export type TicketAction = "approve" | "reject" | "deny" | "withdraw" | "archive" | "edit"
export function availableActions(t: TicketDetail, me: { id: string; permissions: string[] }): TicketAction[]
export function sendBackTargets(t: TicketDetail): { to: number; label: string }[] // 0 = applicant, then earlier steps
export function stepSummary(step: TicketStep): string                          // e.g. "1/2 approved (any)"
```

## Key Points and Pitfalls

- `availableActions` mirrors the engine rules, but the server is authoritative: always show server errors. approve/reject/deny need `status == in_progress` and me (or a user I proxy for) as a non-completed assignee of the current step; reject/deny only on `actionType == approve` steps. withdraw: author and in_progress. archive: author on draft/rejected, or `ticket_write` on in_progress/completed/denied. edit: author on draft/rejected, or current approver on in_progress.
- Use the kickflow wording "Send back" for `reject` and "Deny" for `deny` (notes.md terminology).
- Do not send a `comment` key when the textarea is empty (workflows with commenting disabled reject it).

## Tests (`ticket-view.test.ts`, input -> expected)

- in_progress ticket, me = current assignee on an approve step -> includes approve, reject, deny; not withdraw
- me = author of in_progress -> withdraw; draft -> edit, archive
- current step actionType confirm -> approve only
- `sendBackTargets` with currentStep 3 -> [0,1,2]
- `stepSummary` with requiredApprovalsNumber 1 of 2 assignees, 1 completed -> contains "1/2"

## Verification

```bash
test -d web/node_modules && echo node_modules-present   # installed by PF-03; do NOT run bun install here (PF-12/PF-13 run in parallel in the same directory)
cd web && bun run typecheck 2>&1 | tee /tmp/pf11-typecheck.log
cd web && bun test 2>&1 | tee /tmp/pf11-test.log
cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf11-dist" --emptyOutDir 2>&1 | tee /tmp/pf11-build.log
git status --porcelain web/package.json web/bun.lock   # must print nothing
```

## Done Criteria

- [ ] All four pages implemented; components and `ticket-view.ts` exist; tests, typecheck and temp build pass; dependency files unchanged

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. PF-12 and PF-13 edit other page files in parallel; never touch them.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
