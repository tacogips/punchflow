# PF-18 /v1 Users and Grades Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-18
**Design Reference**: design-docs/specs/design-api-coverage.md rows 1-18; design-docs/references/kickflow-api.md section 3 "Users", "Grades" and section 4 schemas; notes.md INF-02, INF-03, INF-24, INF-29, INF-42
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Replace the PF-10 stubs for the 18 user and grade operations with real handlers, backed by new store queries and the PF-08 presenters. Request/response fields, query parameters and constraints come from `kickflow-api.md` section 3 for each operationId.

## Non-goals

- No changes to `routes.go`/`api.go`, presenters, auth or other handler files. No invitation e-mail (DIV-02).

## Files

`internal/api/v1/users.go`, `internal/api/v1/grades.go` (shared: replace stub bodies), `internal/api/v1/users_test.go`, `internal/api/v1/grades_test.go` (new, `package v1_test`); `internal/store/users.go`, `internal/store/grades.go` (new: list/filter/insert/update/delete).

## Path Declarations

### writePaths
- `impl-plans/active/pf-18-api-users-grades.md`
- `internal/api/v1/users_status.go`
- `internal/api/v1/users_test.go`
- `internal/api/v1/grades_test.go`
- `internal/store/users.go`
- `internal/store/grades.go`

### sharedPaths
- `internal/api/v1/users.go`
- `internal/api/v1/grades.go`

### sharedPathNotes
- `internal/api/v1/users.go`: PF-10 stub file; replace the bodies of operations 1-12 (may move status actions into `users_status.go`)
- `internal/api/v1/grades.go`: PF-10 stub file; replace the bodies of operations 13-18

### artifactRoots
- (none)

## Handler Conventions (same in every wave-4 plan)

- Permission first: `auth.Require(r, <perm>)` -> `kferr.Write` on error. Then parse path values with `r.PathValue`, resolving id-or-code via `store.ResolveID` (404 `resource_not_found`).
- Reads use `a.d.DB.Q()`. Mutations use one `a.d.DB.WithTx`: validate -> write -> `store.InsertAuditLog` (action `<resource>.<verb>`, `data` `{"id":..., "fields":[changed names]}`, request id from `httpx.RequestIDFrom`, remote IP from `httpx.ClientIP`) -> build the response with presenters using `tx`. Write the response after commit.
- Lists: `httpx.ParsePage`, `httpx.ParseSort(r, <allowed fields from the spec pattern>)`, store returns `(ids []string, total int)`, presenters build items, `httpx.SetPagination(w, r, a.d.BaseURL, page, total)`, `httpx.WriteJSON(w, 200, items)` (empty -> `[]`).
- Bodies: `httpx.DecodeJSON`; required fields missing -> 400 `invalid_parameter`; business rule violations -> 422 `validation_failed` with field keys in camelCase as in the request; empty-body success -> `httpx.WriteEmpty`.

## Operation Notes

| Op | Perm | Rules |
|---|---|---|
| getCurrentUser | any | UserDetail of the principal (`LastUsedOn` and `UserType` on) |
| listUsers | user | filters and sortBy from the spec; `LastUsedOn` on; `CustomFields` on |
| createUser | user | body per spec (email, names, code optional -> random 16 alnum, employeeId, locale, memberships `InviteMembershipBody` -> active chart teams by code); e-mail domain must match tenant `allowed_email_domains` when non-empty -> 422 `domain_not_allowed`; duplicate e-mail/code -> 422; status `invited` + invitation 7 days; audit `user.create` |
| getUser / lookupUserByEmail | user | id or code; e-mail exact, case-insensitive; missing -> 404 |
| updateUser | user | partial; domain rule; audit `user.update` |
| deleteUser | user | -> `deactivated`, `deactivatedAt` now; returns UserDetail; already deactivated -> 422 `invalid_state`; audit `user.delete` |
| reinviteUser | user | from `deactivated`/`invited` -> `invited` + new invitation (replace row); else 422; audit `user.reinvite` |
| suspendUser | user | `activated` -> `suspended`; else 422; audit `user.suspend` |
| reactivateUser | user | `suspended` -> `activated`; else 422; audit `user.reactivate` |
| listUserTeams | team | teams of the user in the active chart, paginated |
| listUserRoles | role | roles of the user, paginated |
| listGrades, getGrade | user | sortBy per spec |
| createGrade | user | name, level 0-255 (out of range -> 400), unique level -> 422; code optional random; audit `grade.create` |
| updateGrade | user | partial; audit `grade.update` |
| deleteGrade | user | in use by a membership -> 422 `validation_failed`; default grade -> 422; empty 200; audit `grade.delete` |
| setDefaultGrade | user | unset previous default in the same tx; empty 200; audit `grade.setDefault` |

## Key Points and Pitfalls

- Suspending a user must not break their stored tokens: auth rejects them with `user_inactive` (PF-05). Do not delete tokens.
- `lookupByEmail` takes the `email` query parameter (URL-decoded by Go).
- Keep each file < 1000 lines; split `users.go` into `users.go` + `users_status.go` if needed (the new file is in this plan's write set).

## Tests (`package v1_test`, `testutil.New`)

- getCurrentUser Admin -> 200 keys include id, email, invitation; SAT with X-Caller-Id sales-rep -> sales-rep
- listUsers Admin -> 200, `Total: 13`, `Link` rel next when perPage=5; Employee -> 403 `missing_permission`; `perPage=0` -> 400
- createUser -> 200 status invited; same e-mail again -> 422; tenant allowed domain `example.com` (PATCH tenant row via SQL) + `x@punchflow.test` -> 422 `domain_not_allowed`
- suspend activated -> suspended; suspend again -> 422 `invalid_state`; reactivate -> activated; delete -> deactivated; reinvite -> invited
- a suspended user's token -> 401 `user_inactive` afterwards
- lookupByEmail `ADMIN@punchflow.test` -> admin; unknown -> 404 `resource_not_found`
- listUserTeams(sales-rep) with Admin -> sales, sales-east; listUserRoles(admin) -> Super Admin
- grades: create level 300 -> 400; duplicate level -> 422; delete `staff` (in use) -> 422; setDefault `chief` -> previous default cleared; delete unused grade -> 200 empty body
- audit log rows exist with actions `user.suspend`, `grade.create`
- unknown user id -> 404 `resource_not_found`; get by code `admin` works

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf18-build.log
go vet ./internal/api/... ./internal/store/... 2>&1 | tee /tmp/pf18-vet.log
go test -race -run 'Test(User|Users|Grade|Grades|CurrentUser)' ./internal/api/v1/... 2>&1 | tee /tmp/pf18-test.log
gofmt -l internal/api/v1/users*.go internal/api/v1/grades*.go internal/store/users.go internal/store/grades.go   # prints nothing
```

## Done Criteria

- [ ] 18 operations implemented (no stub message remains in `users.go`/`grades.go`); tests above pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. `users.go` and `grades.go` are shared with PF-10 (stub creator); record hashes before editing.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
