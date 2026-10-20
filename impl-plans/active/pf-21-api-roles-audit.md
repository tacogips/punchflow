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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf21-build.log
go vet ./internal/api/... ./internal/store/... 2>&1 | tee /tmp/pf21-vet.log
go test -race -run 'Test(Role|Roles|AuditLog)' ./internal/api/v1/... 2>&1 | tee /tmp/pf21-test.log
gofmt -l internal/api/v1/roles*.go internal/api/v1/audit_logs*.go internal/store/roles.go internal/store/audit_queries.go   # prints nothing
```

## Done Criteria

- [ ] 9 operations implemented; tests pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. Handler files are shared with PF-10 (stub creator).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
