# PF-21 /v1 Admin Roles and Audit Logs Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-21
**Design Reference**: design-docs/specs/design-api-coverage.md rows 83-90 and 96; design-docs/references/kickflow-api.md section 3 "Admin roles", "Audit logs", section 1.8; notes.md INF-29, INF-43, DIV-08
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Replace the PF-10 stubs for the 8 admin-role operations and `listAuditLogs`. Roles define the RBAC permissions that `internal/auth` unions per user, so changes here immediately change what tokens may do.

## Non-goals

- No enforcement of `restricted` scoping (DIV-08). No audit log export. No changes outside the listed files.

## Files

`internal/api/v1/roles.go`, `internal/api/v1/audit_logs.go` (shared: replace stubs); tests `roles_test.go`, `audit_logs_test.go` (`package v1_test`); `internal/store/roles.go`, `internal/store/audit_queries.go` (new).

## Path Declarations

### writePaths
- `impl-plans/active/pf-21-api-roles-audit.md`
- `internal/api/v1/roles_test.go`
- `internal/api/v1/audit_logs_test.go`
- `internal/store/roles.go`
- `internal/store/audit_queries.go`

### sharedPaths
- `internal/api/v1/roles.go`
- `internal/api/v1/audit_logs.go`

### sharedPathNotes
- `internal/api/v1/roles.go`: PF-10 stub file; replace the bodies of operations 83-90
- `internal/api/v1/audit_logs.go`: PF-10 stub file; replace the body of operation 96

### artifactRoots
- (none)

## Handler Conventions

Same as PF-18 "Handler Conventions".

## Operation Notes

| Op | Perm | Rules |
|---|---|---|
| listRoles / getRole | role | Role / RoleDetail |
| createRole | role | `name`, `permissionList[]` (permission enum; `folderIds`/`generalMasterIds`/`teamIds` exact duplicates -> 400 `invalid_parameter`, case-only duplicates -> 422 `validation_failed`; unknown ids -> 422); `editable` true; audit `role.create` |
| updateRole / deleteRole | role | super admin role (`editable` false) -> 403 `resource_not_editable`; caller is a member and not a super admin member -> 403 `missing_permission` (INF-43); delete returns empty 200 |
| createRoleMembers | role | `userIds` max 10 -> 400; adding self while not super admin -> 403 (INF-43); already member -> ignored; empty 200 |
| listRoleMembers | role | User without `customFields`, paginated |
| deleteRoleMember | role | removing the last super admin member -> 422; removing self while not super admin -> 403; empty 200 |
| listAuditLogs | audit | filters `createdAtStart`, `createdAtEnd` (`httpx.QueryTime`), `userId`; default `createdAt` desc (INF-02); paginated; AuditLog presenter |

## Key Points and Pitfalls

- The "self" checks use the principal's user id and the caller's super admin membership (membership in the role with `editable` false).
- Audit logs written by the role mutations themselves must appear in `listAuditLogs` (same DB), which is a cheap end-to-end check.

## Tests (`package v1_test`)

- listRoles Admin -> 4 roles; Employee -> 403
- create role with `folderIds` [x, x] -> 400; [x, X] -> 422; valid -> 200 RoleDetail with `permissionList[].folders` resolved
- update Super Admin -> 403 `resource_not_editable`; Integration Admin member (SAT caller `eng-manager`) holding a new role with `role` permission updates own role -> 403 `missing_permission`
- add 11 members -> 400; remove the only Super Admin member -> 422
- Employee `GET /v1/auditLogs` -> 403; Admin creates role `Auditors` (`audit`) and adds sales-rep -> Employee `GET /v1/auditLogs` -> 200 (permission union works end to end, using only this plan's endpoints)
- listAuditLogs Admin after a role create -> contains action `role.create` with `user` = admin; `createdAtStart` in the future -> `[]`; Employee -> 403

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-08, PF-16.
**State at 410d3d8**: not started (PF-10 stubs). Implement this plan as written. Session-462 points:

- `createRole`/`updateRole` validate each `permissionList[].permission` against `model.Permissions` (the 19 kickflow values). Unknown -> `400 invalid_parameter`. The invented names (`audit_read`, `ticket_admin`, `user_read`, ...) are rejected; add the test case `permission: audit_read -> 400`.
- Endpoint permissions: `model.PermissionRole` (roles), `model.PermissionAudit` (audit logs).
- Name tests `TestRole*`, `TestRoles*`, `TestAuditLog*`; prefix helpers with `pf21`.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf21-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf21-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Role|Roles|AuditLog)' ./internal/api/v1/... 2>&1 | tee /tmp/pf21-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf21-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf21-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf21-s462-test.log)"
gofmt -l internal/api/v1/roles.go internal/api/v1/audit_logs.go internal/api/v1/roles_test.go internal/api/v1/audit_logs_test.go internal/store/roles.go internal/store/audit_queries.go   # prints nothing
rg -n 'not implemented' internal/api/v1/roles.go internal/api/v1/audit_logs.go   # prints nothing (exit 1)
```

### Done Criteria (session 462)

- [ ] 9 operations implemented; permission enum validated against `model.Permissions`; listed tests pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-08 (accepted at f82e374) and PF-16 (wave A).
**State at f82e374**: not started. `roles.go` and `audit_logs.go` are PF-10 stubs. `internal/store/roles.go` and `internal/store/audit_queries.go` do not exist.
**Path Declarations**: unchanged. 7 declared files (< 300).

1. Implement the whole plan above, with the session-462 points.
2. `internal/api/v1/roles_test.go` must contain the negative case `permission: audit_read -> 400 invalid_parameter` using that literal string. PF-27's `repo_invariants_test.go` subtest `negative_tests_present` checks that this file contains `audit_read`. Never remove or obfuscate it.
3. Never edit `internal/store/audit.go` (accepted PF-02) or `read_roles.go` (accepted PF-08), or `internal/api/v1/api.go`, `routes.go` or `routes_test.go` (PF-10). Shared-package compile failures caused by another plan's file go to a reconciliation request (README rules 11 and 12).
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-21/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0, and the `not implemented` rg prints nothing
- [ ] `rg -l 'audit_read' internal/api/v1/roles_test.go` lists the file
- [ ] The progress log has the intent snapshot, pre/post hashes of the shared files, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-08 (accepted at `44e40cc`) and PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 83-90 and 96 and the enum contract; INF-29 and INF-43 in `notes.md`; CF-01.

**Intent**: unchanged. Implement 9 operations (roles, audit logs). Keep the literal `audit_read` -> 400 negative case in `roles_test.go`: PF-27's `negative_tests_present` invariant requires it.

**Session-464 changes**:
1. `internal/api/v1/roles.go` and `audit_logs.go` are in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty. Replace bodies only. Never edit `api.go`, `routes.go`, `routes_test.go`, `internal/testutil/*` or `internal/server/*`.
2. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/api/v1/...`.
   - Log each result.
   - Prefixes: unexported identifiers start with `pf21`. New exported store functions start with `Role` or `AuditLog`.
3. Do not put a banned enum literal in production code. `audit_read` may appear only in `roles_test.go`.
4. Guard: the manifest `-run 'TestPF10'` line must PASS. Logs are `/tmp/pf21-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-21/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0; PF10 guard PASS; the `not implemented` rg empty; `rg -l 'audit_read' internal/api/v1/roles_test.go` lists the file
- [ ] Pre/post hashes of the transferred files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B1 (with PF-18, PF-19, PF-20). It is dispatched as soon as PF-16 is accepted. It may run while PF-14a/b/c are still running. **Depends on**: PF-08 (accepted at `ea385c7`), PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (7 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: implement the 9 operations, validate the permission enum against `model.Permissions`, and keep the `audit_read` negative test in `roles_test.go` (PF-27's `negative_tests_present` depends on it). Logs are `/tmp/pf21-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-21/attempt-<n>/`.

**Pitfalls added in s465**:
- `internal/store` may still be edited by PF-14a/b/c (`Eng*` names). Use your `Role*`/`Audit*` store names.
- Never touch `internal/engine/*`.

### Done Criteria (session 465)

- [ ] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the `not implemented` check exit 0; `rg -l 'audit_read' internal/api/v1/roles_test.go` lists the file; gofmt check exit 0
- [ ] Pre/post hashes of the transferred files; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). Handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
