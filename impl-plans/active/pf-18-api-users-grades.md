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

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4 (parallel with PF-19..PF-25). **Depends on**: PF-08, PF-16.
**State at 410d3d8**: not started. `users.go`/`grades.go` hold the PF-10 stubs (`kferr` 500 "not implemented"). Implement this plan as written, with these session-462 points:

- Permission checks use the kickflow permission names from `model.Permission*` (`model.PermissionUser`, `model.PermissionTeam`, `model.PermissionRole`). Never use a string literal or an invented name such as `user_read`.
- User status transitions use `model.UserStatus*`. `userType` values come from `model.UserTypes`; `createUser` accepts `userType` only from that list, and anything else is `400 invalid_parameter`. The `status` query filter on `listUsers` accepts only `model.UserStatuses` (otherwise `400 invalid_parameter`).
- Shared package: `internal/api/v1` is compiled with PF-19..PF-23 tests at the same time. Name tests `TestUser*`, `TestUsers*`, `TestCurrentUser*`, `TestGrade*`. Prefix helpers with `pf18`. If the package fails to compile because of another plan's file, follow README rule 11 (reconciliation entry, no foreign edit, re-run).

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf18-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf18-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(User|Users|Grade|Grades|CurrentUser)' ./internal/api/v1/... 2>&1 | tee /tmp/pf18-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf18-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf18-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf18-s462-test.log)"
gofmt -l internal/api/v1/users.go internal/api/v1/users_status.go internal/api/v1/grades.go internal/api/v1/users_test.go internal/api/v1/grades_test.go internal/store/users.go internal/store/grades.go   # prints nothing
rg -n 'not implemented' internal/api/v1/users.go internal/api/v1/grades.go   # prints nothing (exit 1)
```

Evidence: exit 0; FAIL=0; SKIP=0; PASS covers every case in the Tests section.

### Done Criteria (session 462)

- [ ] 18 operations implemented (no stub message left); listed tests pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean; files < 1000 lines

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463; parallel with PF-19..PF-25). **Depends on**: PF-08 (accepted at f82e374) and PF-16 (wave A).
**State at f82e374**: not started. `users.go` and `grades.go` are PF-10 stubs. `internal/store/users.go` and `internal/store/grades.go` do not exist.
**Path Declarations**: unchanged. 8 declared files (< 300).

1. Implement the whole plan above, with the session-462 points.
2. Edit only the handler bodies in the two shared stub files. Never edit `internal/api/v1/api.go`, `routes.go` or `routes_test.go`: the accepted PF-10 `Operations()` table is fixed at 96 rows and already routes your operations. Never edit `internal/testutil/*` or `internal/server/*` (PF-16).
3. Wave B shares `internal/api/v1` and `internal/store` with PF-19..PF-24. Use `pf18` prefixes and your `Test*` nouns. If a compile failure is caused by another plan's file, record a reconciliation request (README rules 11 and 12) and re-run after that plan lands.
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-18/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0, and the `not implemented` rg prints nothing
- [ ] The progress log has the intent snapshot, pre/post hashes of `users.go` and `grades.go`, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B (parallel with PF-19..PF-25). It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-08 (accepted at `44e40cc`) and PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (this plan's `writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 1-18 and the cross-cutting conventions; `design-docs/specs/architecture.md` "Shared-tree compile and ownership-transfer rule (session 464)".

**Intent**: unchanged. Implement operations 1-18 (users, grades) with the httptest coverage described above.

**Session-464 changes**:
1. `internal/api/v1/users.go` and `grades.go` are now in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty. Replace handler bodies only. Keep every handler name that `internal/api/v1/routes.go` references. Never edit `api.go`, `routes.go`, `routes_test.go`, `internal/testutil/*` or `internal/server/*`.
2. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/api/v1/...`.
   - Log each result.
   - Seven wave-B plans write into `internal/api/v1` and `internal/store` at the same time, and a broken file blocks all of them.
3. Prefixes: unexported identifiers start with `pf18`. New exported store functions start with `User` or `Grade`. Grep `internal/store` before adding one.
4. Guard: the manifest line `-run 'TestPF10'` must PASS. It proves the 96-row table and dispatch still match after your edits.
5. Logs are `/tmp/pf18-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-18/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0; the PF10 guard log PASS; the `not implemented` rg prints nothing
- [ ] Pre/post hashes of `users.go` and `grades.go`; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B1 (with PF-19, PF-20, PF-21). It is dispatched as soon as PF-16 is accepted. It may run while the wave-A engine plans (PF-14a/b/c) are still running. **Depends on**: PF-08 (accepted at `ea385c7`), PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (8 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: implement the 18 operations, run the listed tests, and keep the PF10 guard green. Logs are `/tmp/pf18-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-<n>/`.

**Pitfalls added in s465**:
- `internal/store` may still be edited by PF-14a/b/c (wave A, `Eng*` names) and by the other B1 plans. Use only the `User*`/`Grade*` store names your plan lists, and grep `internal/store` before adding one.
- Never touch `internal/engine/*`.

### Done Criteria (session 465)

- [ ] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the `not implemented` check exit 0; gofmt check exit 0
- [ ] Pre/post hashes of `users.go` and `grades.go`; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). `users.go` and `grades.go` were transferred from PF-10 in session 464; record hashes before editing.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
