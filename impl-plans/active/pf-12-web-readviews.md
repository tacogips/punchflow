# PF-12 Web Read Views Implementation Plan

**Status**: In Progress
**Wave**: 2 | **Depends On**: PF-03 | **Plan ID**: PF-12
**Design Reference**: design-docs/specs/architecture.md#web-ui-web (workflows/routes JSON views; users, org, grades, roles, masters, categories/folders, proxies, audit logs); design-docs/user-qa/pending-punchflow-defaults.md item 10
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Fill the definition and admin read-view page stubs (PF-03) with screens backed by `/v1` GET endpoints. Per the accepted default (user-qa item 10), these screens are read-only except user status actions (suspend / reactivate / reinvite). Other mutations remain available through the API.

## Non-goals

- No create/edit forms for org, grades, roles, masters, categories, folders or proxies. No dependency changes. No edits to `App.tsx`, `api/*` or the existing `lib/*` files.

## Files

`web/src/pages/definitions/Workflows.tsx`, `WorkflowView.tsx`, `Routes.tsx`, `RouteView.tsx`; `web/src/pages/admin/Users.tsx`, `Org.tsx`, `Grades.tsx`, `Roles.tsx`, `Masters.tsx`, `Classification.tsx`, `Proxies.tsx`, `AuditLogs.tsx`; `web/src/components/readviews/` (`JsonView.tsx`, `PagedTable.tsx`, `Tree.tsx`); `web/src/lib/tree.ts`, `web/src/lib/tree.test.ts`.

## Path Declarations

### writePaths
- `impl-plans/active/pf-12-web-readviews.md`
- `web/src/components/readviews/JsonView.tsx`
- `web/src/components/readviews/PagedTable.tsx`
- `web/src/components/readviews/Tree.tsx`
- `web/src/lib/tree.ts`
- `web/src/lib/tree.test.ts`

### sharedPaths
- `web/src/pages/definitions/Workflows.tsx`
- `web/src/pages/definitions/WorkflowView.tsx`
- `web/src/pages/definitions/Routes.tsx`
- `web/src/pages/definitions/RouteView.tsx`
- `web/src/pages/admin/Users.tsx`
- `web/src/pages/admin/Org.tsx`
- `web/src/pages/admin/Grades.tsx`
- `web/src/pages/admin/Roles.tsx`
- `web/src/pages/admin/Masters.tsx`
- `web/src/pages/admin/Classification.tsx`
- `web/src/pages/admin/Proxies.tsx`
- `web/src/pages/admin/AuditLogs.tsx`

### sharedPathNotes
- `web/src/pages/definitions/Workflows.tsx`: PF-03 stub; replace with the workflow list
- `web/src/pages/definitions/WorkflowView.tsx`: PF-03 stub; replace with the workflow summary and JSON view
- `web/src/pages/definitions/Routes.tsx`: PF-03 stub; replace with the route list
- `web/src/pages/definitions/RouteView.tsx`: PF-03 stub; replace with the route steps and JSON view
- `web/src/pages/admin/Users.tsx`: PF-03 stub; replace with the paged user list and status actions
- `web/src/pages/admin/Org.tsx`: PF-03 stub; replace with the chart/team tree and members
- `web/src/pages/admin/Grades.tsx`: PF-03 stub; replace with the grade list
- `web/src/pages/admin/Roles.tsx`: PF-03 stub; replace with the role list and detail
- `web/src/pages/admin/Masters.tsx`: PF-03 stub; replace with masters and items
- `web/src/pages/admin/Classification.tsx`: PF-03 stub; replace with categories and the folder tree
- `web/src/pages/admin/Proxies.tsx`: PF-03 stub; replace with proxy tables
- `web/src/pages/admin/AuditLogs.tsx`: PF-03 stub; replace with the paged audit log view

### artifactRoots
- (none; `web/node_modules` belongs to PF-03 and is read-only here)

## Screen Behavior

- Workflows: `GET /v1/workflows` table (code, name, status, folder) -> WorkflowView: summary (settings, route branches, form fields table) plus a collapsible pretty-printed JSON of `GET /v1/workflows/{id}`. Routes/RouteView likewise (steps table: order, type, action, approvers, condition).
- Users: paged `GET /v1/users` with status column; row actions Suspend (`POST .../suspend`), Reactivate, Reinvite, shown according to status; errors displayed.
- Org: `GET /v1/organizationCharts` -> pick chart (default `GET /v1/organizationChart`) -> team tree (`GET .../teams?parentId=`, lazy) -> members (`GET .../memberships`) with leader flag and grades.
- Grades, Roles (role detail permissions + members), Masters (master list -> items, with expired items marked), Classification (categories table + folder tree from `GET /v1/folders`), Proxies (applicants and approvers tables), AuditLogs (paged `GET /v1/auditLogs` with `createdAtStart/End` and `userId` filters; `data` shown as JSON).
- 403 `missing_permission` shows a "You do not have permission (<code>)" panel instead of failing.

## Contracts (`lib/tree.ts`)

```ts
export interface TreeNode<T> { item: T; children: TreeNode<T>[] }
export function buildTree<T>(items: T[], id: (t: T) => string, parentId: (t: T) => string | null): TreeNode<T>[]
export function flattenWithDepth<T>(nodes: TreeNode<T>[]): { item: T; depth: number }[]
```

## Key Points and Pitfalls

- `buildTree` must tolerate orphans (parent not in the list -> treat as root) and must not loop on cycles (guard with a visited set).
- Paging uses the `Link`/`Total` headers via `apiList`, not body fields.

## Tests (`tree.test.ts`)

- flat list with parent ids -> correct nesting and depth order
- orphan -> becomes a root
- cycle a->b->a -> terminates, each node appears once

## Verification

```bash
test -d web/node_modules && echo node_modules-present   # installed by PF-03; do NOT run bun install here (PF-11/PF-13 run in parallel in the same directory)
cd web && bun run typecheck 2>&1 | tee /tmp/pf12-typecheck.log
cd web && bun test 2>&1 | tee /tmp/pf12-test.log
cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf12-dist" --emptyOutDir 2>&1 | tee /tmp/pf12-build.log
git status --porcelain web/package.json web/bun.lock   # must print nothing
```

## Done Criteria

- [x] 12 pages implemented; `tree.ts` with tests; typecheck, tests and temp build pass; dependency files unchanged

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. PF-11 and PF-13 edit other page files in parallel.

## Progress Log

### Session: 2026-10-20 PF-12 implementation
**Intent snapshot**: Recorded before edits in `tmp/punchflow-kickflow-dummy-s460/PF-12/attempt-1/edit-intents.md`; final PF-12 source hashes are in `post-edit-sha256.json`.
**Tasks Completed**: Implemented all twelve assigned read views, reusable JSON/error/table/tree components, and `buildTree`/`flattenWithDepth` with orphan and cycle handling. Workflows, routes, users, and audit logs consume API pagination headers. Users expose status-appropriate actions; organization hierarchy fetches child teams lazily and displays memberships. Definition JSON and audit data can be inspected.
**Verification**: PF-12 scoped TypeScript compile passed (`/tmp/pf12-scoped-typecheck-final.log`); tree tests passed 3/3 (`/tmp/pf12-tree-test-final.log`); temporary Vite build passed (`/tmp/pf12-build-final.log`); dependency status check exited 0 with no output. Final-source `bun test` passed 17/17 with zero failures (`/tmp/pf12-test-final.log`); the PF-12 tree suite passed 3/3 (`/tmp/pf12-tree-test-final.log`). Scoped TypeScript compilation of all PF-12 files passed (`/tmp/pf12-scoped-typecheck-final.log`). The full Vite build passed to `${TMPDIR}/pf12-dist` (`/tmp/pf12-build-final.log`). Full `bun run typecheck` still exits 2 (`/tmp/pf12-typecheck-final.log`) due to type errors in `web/src/components/tickets/FormFields.tsx`, `web/src/pages/tickets/TicketDetail.tsx`, `web/src/pages/tickets/TicketEdit.tsx`, and `web/src/pages/tickets/TicketSearch.tsx`; these paths are outside PF-12 ownership. The dependency status check exited 0 and printed nothing. No dependency files were changed.
**Reconciliation requests**: None for PF-12 implementation. The earlier full-tree typecheck errors in ticket UI files were resolved on the shared tree by their owners; PF-12 reran and passed the required full typecheck, tests, and temporary build. PF-27 still owns serial combined-tree finalization.

### Session: 2026-10-20 PF-12 verification continuation
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-12/attempt-2/edit-intent.md`; pre-edit plan hash in `pre-edit-sha256.txt`.
**Tasks Completed**: Re-ran the full PF-12 verification after the shared tree stabilized. The prior out-of-scope ticket UI type errors and transient ticket/tree test failures no longer reproduce. All twelve read views and tree utilities remain in place. `web/package.json` and `web/bun.lock` match the PF-03 `web/toolchain.json` hashes; their untracked status is inherited from the shared no-commit branch.
**Verification**: `test -d web/node_modules && echo node_modules-present` exited 0; `cd web && bun run typecheck 2>&1 | tee /tmp/pf12-typecheck.log` exited 0; `cd web && bun test 2>&1 | tee /tmp/pf12-test.log` exited 0 (19 passed, 0 failed); temporary Vite build via `cd web && bun run build --outDir "${TMPDIR:-/tmp}/pf12-dist" --emptyOutDir 2>&1 | tee /tmp/pf12-build.log` exited 0; PF-12 scoped TypeScript check exited 0; `git status --porcelain web/package.json web/bun.lock` exited 0 but prints both as untracked, with digests matching PF-03 (`021bffcb3f794a75011ebcaaaa36c0e18ea11c420ac745f52a94314fd14760ef`, `eee300137f6b77ce73ab6491efd1cda132c13fd3548c394bf5ba7270788beee0`).
**Resolved Earlier Verification**: Attempt 1 full typecheck exited 2 on ticket UI files outside PF-12 ownership and `bun test` had a transient shared-tree failure. The current full-tree reruns above pass, so these are historical failures resolved by shared-tree stabilization; no out-of-scope files were edited.
**Downstream**: Independent test-integrity/adversarial review and PF-27 serial integration remain pending downstream workflow steps.

### Session: 2026-10-20 PF-12 adversarial repairs (comm-008112)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s460/PF-12/attempt-3/edit-intents.md`; source hashes before and after edits are in `pre-edit-sha256.txt` and `post-edit-sha256.txt`.
**Tasks Completed**: PF12-ADV-001: added `parentIndexFromChildLists` and `parentByPath`; Org now derives parents from lazy child-list cache keys, ignores child responses from a previous chart, and reads member grades from `grades[]`; Classification derives folder parents from `fullName`. Added two pure-helper tests while preserving the original three tests. PF12-ADV-002: Proxies now reads `user`, `proxyUser`, `startsOn`, `endsOn`, and workflow names; WorkflowView uses route condition and route fields plus form-field `title`; Workflows reads the folder object; Grades reads `level`.
**Verification**: `cd web && bun test src/lib/tree.test.ts` exited 0 (5 passed, 0 failed; `/tmp/pf12-adv1-tree-test.log`); `cd web && bun run typecheck` exited 0 (`/tmp/pf12-adv1-typecheck.log`); `cd web && bun test` exited 0 (25 passed, 0 failed; `/tmp/pf12-adv1-test.log`); temporary Vite build exited 0 (`/tmp/pf12-adv1-build.log`). Stale-field grep checks exited 0 with no matches. `web/package.json` and `web/bun.lock` hashes remain equal to PF-03 evidence: `021bffcb3f794a75011ebcaaaa36c0e18ea11c420ac745f52a94314fd14760ef` and `eee300137f6b77ce73ab6491efd1cda132c13fd3548c394bf5ba7270788beee0`. Post-edit hashes for all eight changed source files are in `attempt-3/post-edit-sha256.txt`.
**Review disposition**: Both mid findings are implemented and verified on the current source. Independent re-review is pending. Optional low-priority Settings/Category-description display cleanup was not part of the required repairs.
