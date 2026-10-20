# PF-11 Web Ticket Screens Implementation Plan

**Status**: In Progress
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

- [x] All four pages implemented; components and `ticket-view.ts` exist; tests, typecheck and temp build pass; dependency files unchanged against PF-03 accepted snapshot

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. PF-12 and PF-13 edit other page files in parallel; never touch them.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -

### Session: 2026-10-20 (Step 6, PF-11 implementation)
**Intent snapshot**: Replaced the four ticket page stubs and added ticket field/slip/step/action/comment/viewer components plus the pure ticket action helper and tests. See `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-1/edit-intents.md` for per-edit intent records and pre-edit hashes.
**Tasks Completed**:
- [x] Dashboard loads the current user's tickets and approval tasks, with links and loading/error states.
- [x] Ticket search supports status[], workflow, ticket number, created date bounds, API pagination and ticket detail links.
- [x] Ticket create/edit loads workflow/team data, renders conditional form/slip fields, uploads files, supports nominated approvers/proxy client, saves drafts/submits and maps field/base errors. In-progress approver edits submit only approver-editable inputs and slipItems.
- [x] Ticket detail renders metadata, form/slip values, approval steps/proxies, related tickets, comments and viewers; mutations refresh data and show server errors.
- [x] Added tests for assigned actions, author actions, confirm steps, send-back targets and approval summary.
- [x] Kept `web/src/api/*`, `web/src/lib/form-model.ts`, `web/src/lib/format.ts`, `web/src/lib/link.ts`, `web/src/App.tsx`, `web/package.json`, `web/bun.lock`, and PF-12/PF-13 page files outside this plan's edits.
**Verification**:
- `test -d web/node_modules && printf '%s\\n' node_modules-present | tee /tmp/pf11-node_modules.log` — exit 0.
- `cd web && bun run typecheck 2>&1 | tee /tmp/pf11-final-typecheck.log` — exit 0.
- `cd web && bun test 2>&1 | tee /tmp/pf11-final-test.log` — exit 0; 17 passed, 0 failed, 37 assertions across 7 files (shared tree includes concurrent tests).
- `cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf11-dist-final" --emptyOutDir 2>&1 | tee /tmp/pf11-final-build.log` — exit 0; Vite built successfully in a temporary output directory.
- `python3` hash comparison of `web/package.json` and `web/bun.lock` against PF-03's accepted snapshot `tmp/riela-fanout/40C5086C-DF26-43B5-86A2-D15F25DEC90E/A1B7DA65-3686-4761-A91E-B8C34E69FBFD.json` — exit 0; both hashes match exactly.
- The plan's `git status --porcelain web/package.json web/bun.lock` printed both files as untracked and exited 1 because PF-03 created them earlier in this no-git-state-change branch. This is baseline workspace state; the accepted snapshot hash comparison confirms neither frozen dependency file changed during PF-11.
- Initial checks and corrections are retained at `/tmp/pf11-typecheck.log`, `/tmp/pf11-test.log`, `/tmp/pf11-build.log`, `/tmp/pf11-attempt2-*`, `/tmp/pf11-attempt3-typecheck.log`, and `/tmp/pf11-attempt4-typecheck.log`; final-source passing logs are the paths above.
**Reconciliation requests**: None. Formal test-integrity/adversarial review and serial integration are downstream workflow steps.

### Source-stable rerun after approver-field payload refinement
- In-progress approver PATCH now includes only `approver: true` form/slip values; empty form arrays are omitted. UI also hides author-only metadata and fields in that edit mode.
- `cd web && bun run typecheck 2>&1 | tee /tmp/pf11-attempt6-typecheck.log` — exit 0.
- `cd web && bun test 2>&1 | tee /tmp/pf11-attempt6-test.log` — exit 0; 17 passed, 0 failed, 37 assertions across 7 files.
- `cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf11-dist-attempt6" --emptyOutDir 2>&1 | tee /tmp/pf11-attempt6-build.log` — exit 0; Vite production build passed in the temporary output directory.
- Final assigned-source hashes: `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-1/final-source-sha256.txt`.

### Session: 2026-10-20 (Step 6 redispatch, PF11-ADV-001)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-2/edit-intents.md`; pre-edit hashes are in `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-2/pre-edit-sha256.txt`.
**Tasks Completed**:
- [x] Ticket edit initializes existing author team and proxy client from `authorTeam.id` and `proxyClientUser.id`; new-ticket team defaults still work if teams load after the workflow without resetting form state.
- [x] Added `buildApprovers` to group deduplicated user IDs per route step. Create requests always include its result; author PATCH omits `approvers` when the user entered no nominee. The in-progress approver-only PATCH remains unchanged.
- [x] Added pure helper tests for grouping, duplicate removal and empty input; all existing ticket-view tests remain.
**Verification**:
- `cd web && bun run typecheck 2>&1 | tee /tmp/pf11-adv1-typecheck.log` — exit 0.
- `cd web && bun test 2>&1 | tee /tmp/pf11-adv1-test.log` — exit 0; 19 passed, 0 failed, 40 assertions across 7 files.
- `cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf11-dist-adv1" --emptyOutDir 2>&1 | tee /tmp/pf11-adv1-build.log` — exit 0; Vite built 69 modules successfully in a temporary output directory.
- `python3 -I` comparison of `web/package.json` and `web/bun.lock` against PF-03 snapshot `tmp/riela-fanout/40C5086C-DF26-43B5-86A2-D15F25DEC90E/A1B7DA65-3686-4761-A91E-B8C34E69FBFD.json` — exit 0; both hashes match (`/tmp/pf11-adv1-deps-hash-final.log`). The initial comparison script and shell wrapper attempts are preserved in `/tmp/pf11-adv1-deps-hash.log` and `/tmp/pf11-adv1-deps-hash-retry.log`; both showed the matching digests but exited nonzero due to script/schema and shell-status handling errors.
- Attempt-2 source manifest: `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-2/final-source-sha256.txt`.
**Review**: PF11-ADV-001 correction is implemented and verified; independent adversarial re-review remains pending. No files outside the authorized repair paths were edited.

### Session: 2026-10-20 (Step 6 redispatch, PF11-KF-FIELDS)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-3/edit-intents.md`; pre-edit hashes are captured in the branch-evidence snapshot and the prior attempt-2 final-source manifest.
**Tasks Completed**:
- [x] Comments render `comment.user.fullName` and attachment filename links from `attachments`; edit/delete and `comment.user.id` ownership behavior remain intact.
- [x] Ticket and slip input labels use `formField.title`/`slipField.title` with code/ID fallback; resolved values show master, user, team, and ticket labels before raw value fallback; input attachments render filename links.
- [x] Ticket author, step assignees, actual approvers, and viewers use full-name-based labels; team viewers include grade names when available and fall back to IDs only without a display name.
- [x] Added four helper tests without removing or weakening existing assertions.
**Verification**:
- `cd web && set -o pipefail && bun run typecheck 2>&1 | tee /tmp/pf11-a3-typecheck.log` — exit 0.
- `cd web && set -o pipefail && bun test 2>&1 | tee /tmp/pf11-a3-test.log` — exit 0; 23 passed, 0 failed, 49 assertions across 7 files.
- `cd web && set -o pipefail && bun run build --outDir "${TMPDIR:-/tmp}/pf11-a3-dist" --emptyOutDir 2>&1 | tee /tmp/pf11-a3-build.log` — exit 0; 69 modules transformed and temporary Vite build completed.
- `python3` dependency digest check — exit 0; `web/package.json` = `021bffcb3f794a75011ebcaaaa36c0e18ea11c420ac745f52a94314fd14760ef`, `web/bun.lock` = `eee300137f6b77ce73ab6491efd1cda132c13fd3548c394bf5ba7270788beee0`, matching PF-03's accepted snapshot.
- Schema-read grep across Comments, StepList, Viewers, and TicketDetail — exit 0; no obsolete `comment.files`, `user.name`, `author.name`, or old input field reads.
- Detailed logs: `/tmp/pf11-a3-typecheck.log`, `/tmp/pf11-a3-test.log`, `/tmp/pf11-a3-build.log`, `/tmp/pf11-a3-deps-hash.log`, `/tmp/pf11-a3-deps-identity.log`, `/tmp/pf11-a3-field-grep.log`. Attempt-local final source hashes are in `tmp/punchflow-kickflow-dummy-s460/PF-11/attempt-3/final-source-sha256.txt`.
**Review**: PF11-KF-FIELDS repair is implemented and current-source checks pass. Independent review must re-review the correction; PF-27 retains the separate combined-tree verification and approver PATCH semantics follow-up.
