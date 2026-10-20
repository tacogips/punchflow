# PF-12 Web Read Views Implementation Plan

**Status**: Ready
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

- [ ] 12 pages implemented; `tree.ts` with tests; typecheck, tests and temp build pass; dependency files unchanged

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. PF-11 and PF-13 edit other page files in parallel.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
