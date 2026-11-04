# PF-18 /v1 Users and Grades Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-18
**Design Reference**: design-docs/specs/design-api-coverage.md rows 1-18; design-docs/references/kickflow-api.md section 3 "Users", "Grades" and section 4 schemas; notes.md INF-02, INF-03, INF-24, INF-29, INF-42
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

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

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the `not implemented` check exit 0; gofmt check exit 0
- [x] Pre/post hashes of `users.go` and `grades.go`; every source write followed by a logged compile gate with exit 0

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

### Session: 2026-10-21 (s465 attempt 1)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-1/edit-intent-001.txt`; source hashes before/after in `pre-edit-hashes.txt` and `post-edit-hashes.txt`.
**Tasks Completed**: Began PF-18. Implemented `getCurrentUser` and `getUser`; 15 remaining handlers, both store files, and required test files are not implemented. Plan remains In Progress; session-465 completion criteria are unchecked.
**Verification**:
- `go build ./...` after the first users.go write: exit 1 due to incorrect Principal field; complete log `build-after-users.log`. Corrected against fresh `internal/auth/principal.go` read.
- `go build ./...` after correction: exit 0; log `build-after-users-repair.log`.
- `go vet ./internal/api/v1/...`: exit 0; `vet.log`.
- PF10 guard: exit 1 during a concurrent PF-21 edit, `internal/store/roles.go:68: undefined: boolInt`; log `pf10guard.log`. The shared-tree build later passed, but no successful PF10 guard rerun was recorded.
- Scoped uncached race tests: exit 0 but `testing: warning: no tests to run`; positive behavioral evidence is absent. Log `test.log`.
- `gofmt -l` over declared PF-18 files: exit 0; three declared files do not exist yet (recorded in line-count log).
- Stub scan: exit 0 with matches; expected no-match proof fails, so handlers remain.
- Final `go build ./...`: exit 0; `final-build.log`.
**Reconciliation requests**: Transient shared-tree failure in `internal/store/roles.go:68` owned by PF-21; do not edit. A later build succeeded. No completed behavioral test suite exists.

**Verification follow-up**: Re-ran the PF10 guard after the shared PF-21 compile issue cleared: exit 0, PASS=2, FAIL=0, SKIP=0; `pf10guard-rerun.log`. Correction to the earlier gofmt note: five declared but not-yet-created paths were omitted (users_status.go, users_test.go, grades_test.go, store/users.go, store/grades.go); `file-line-counts.log` records their absence. No PF-18 behavior tests were created or matched.

### Session: 2026-10-21 (s465 attempt 2)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-2/plan-progress-intent.txt`; per-write intents `edit-intent-001.txt` through `edit-intent-014.txt`, plus store and formatting intent files in the same directory. Pre-node snapshot: `pre-node-snapshot.json`.
**Tasks Completed**: Implemented all 18 user and grade operations. Added user and grade store queries/mutations, status transitions, lookup, membership lists, grade defaults/deletion guards, audit records, and httptest coverage. The focused suite covers users, current-user, grades, status transitions, permissions, pagination, validation, and audit behavior. No PF-18 reconciliation request remains.
**Transient failures repaired**: The first compile gates caught duplicate stub declarations, an unused import, and a temporary missing `grades.go` during its replacement; each was corrected immediately and a subsequent compile gate passed. The first race suite had four failing assertions caused by test assumptions that did not match the seeded fixture (grade count, employee identity, and in-use grade); test setup/expectations were corrected against the current fixture and the final suite passes. These failed attempts are historical and are not treated as passing evidence.
**Final verification (current source)**:
- `go build ./...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-2/build-final.log`.
- `go vet ./internal/api/v1/...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-2/vet-final.log`.
- `go test -count=1 -v -race -run 'Test(User|Users|Grade|Grades|CurrentUser)' ./internal/api/v1/...`: exit 0, PASS=8, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-2/test-final-current.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-2/pf10guard-final-current.log`.
- `test -z "$(gofmt -l internal/api/v1/users.go internal/api/v1/users_status.go internal/api/v1/grades.go internal/api/v1/users_test.go internal/api/v1/grades_test.go internal/store/users.go internal/store/grades.go)"`: exit 0; no files listed.
- `rg -n 'not implemented' internal/api/v1/users.go internal/api/v1/users_status.go internal/api/v1/grades.go; rg_rc=$?; test $rg_rc -eq 1`: exit 0; no stub message remains.
- Each PF-18 Go file is below 1000 lines; line counts are 42, 539, 246, 167, 101, 275, and 156 respectively.
- Independent check-and-test rerun: build and vet exit 0; scoped race suite PASS=8/FAIL=0/SKIP=0; PF10 guard PASS=2/FAIL=0/SKIP=0. Logs `/tmp/pf18-check-build.log`, `/tmp/pf18-check-vet.log`, `/tmp/pf18-check-test.log`, `/tmp/pf18-check-pf10guard.log`.
**Source hashes**: before editing, `users.go` `97406943017563516fce0b78f8a5e6540dc5d482f8eb4ae0a25e3e6b6bd96584`, `grades.go` `68710a97dda86ee0d46ba8add9d63b4e6707e3553e918e5ae8848f0f65bc2f49` (`pre-source-hashes.json`); after editing, `users.go` `4297a5ae0f5cb6b53e85ef6171b0627ab2d70007a45b19147c6267dda6b70ff9`, `grades.go` `5dcb2503564509172d32cc8b82c6a2d9570a69796afa5a3a3ef72b6a16fd97a1f` (`post-source-hashes.txt`).
**Compile gates**: Every non-test source write was followed by `go build ./...`; each test write was followed by `go vet ./internal/api/v1/...`. Attempt logs are preserved in the attempt-2 directory; final gates above pass.
**Downstream**: Formal integration/adversarial review and final workflow commit are downstream steps; they are not PF-18 implementation gaps.

### Session: 2026-10-21 (s465 attempt 3, PF18-ADV-01)
**Intent snapshots**: `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/intent-users-status.txt`, `intent-users-test.txt`, and `plan-edit-intent.txt`.
**Finding addressed**: Removed the reinvite exclusion around `store.UserSetStatus` in `pf18TransitionUser`. Reinvite now persists `invited` and clears `deactivated_at` while retaining invitation creation, audit logging, and invalid-state checks.
**Regression**: `TestUserReinviteDeactivatedAndInvalidState` asserts delete response deactivated/non-null `deactivatedAt`, reinvite response invited/null `deactivatedAt`, persisted DB status invited with NULL deactivation timestamp, a `user.reinvite` audit row, and 422 `invalid_state` when reinviting the activated Admin fixture. Existing tests remain unchanged.
**Verification (current source)**:
- `go build ./...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/build.log`.
- `go vet ./internal/api/v1/...`: exit 0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/vet.log`.
- `go test -count=1 -v -race -run 'Test(User|Users|Grade|Grades|CurrentUser)' ./internal/api/v1/...`: exit 0, PASS=9, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/test.log`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/pf10guard.log`.
- `test -z "$(gofmt -l internal/api/v1/users.go internal/api/v1/users_status.go internal/api/v1/grades.go internal/api/v1/users_test.go internal/api/v1/grades_test.go internal/store/users.go internal/store/grades.go)"`: exit 0; complete result `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-3/gofmt.log`.
- Compile gate after the production-source edit: `go build ./...` exit 0; log `build-after-status.log`. Vet gate after the test edit: `go vet ./internal/api/v1/...` exit 0; log `vet-after-test.log`.
**Post-fix hashes**: `users_status.go` `cbfcb8e3d61ca4706eaa0d2f4baaa89b2c76acb0f38fa2706c9e3ea597feef7f`; `users_test.go` `da1f11ff56f840ff9e35bc446c47cd4f5368df8418d79e65bc6be01d19ce30d9`; both recorded in `post-fix-source-hashes.txt`. File sizes: users_status.go 537 lines, users_test.go 216 lines.
**Status**: PF18-ADV-01 is implemented and its regression passes; assigned PF-18 work remains Completed. Independent adversarial re-review is downstream and acceptance is not claimed in this implementation record.

### Session: 2026-10-21 (s465 attempt 4, current-tree re-verification)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-4/plan-edit-intent.txt`. No Go source was edited in this attempt.
**Tasks Completed**: Re-verified the PF-18 implementation and PF18-ADV-01 regression on the current shared tree. A focused read-only review found no defect in reinvite persistence or invalid-state handling. No implementation work remains; formal review, combined-tree integration acceptance and finalization remain downstream.
**Verification (current source; complete logs and exit records under `tmp/punchflow-kickflow-dummy-s465/PF-18/attempt-4/`)**:
- `go build ./...`: exit 0; `build.log`, `build.exit`.
- `go vet ./internal/api/v1/...`: exit 0; `vet.log`, `vet.exit`.
- `go test -count=1 -v -race -run 'Test(User|Users|Grade|Grades|CurrentUser)' ./internal/api/v1/...`: exit 0, PASS=9, FAIL=0, SKIP=0; `race.log`, `race.exit`, `race.counts`.
- `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2, FAIL=0, SKIP=0; `pf10.log`, `pf10.exit`, `pf10.counts`.
- Declared PF-18 Go-file `gofmt -l` check: exit 0, no output; `gofmt.log`, `gofmt.exit`.
- `not implemented` scan over users.go, users_status.go and grades.go: no matches; expected no-match wrapper exit 0; `no-stub.log`, `no-stub.exit`.
- Current SHA-256 values: users.go `4297a5ae0f5cb6b53e85ef6171b0627ab2d70007a45b19147c6267dda6b70ff9`; grades.go `5dcb2503564509172d32cc8b82c6a2d9570a69796afa5a3a3ef72b6a16fd97a1`; users_status.go `cbfcb8e3d61ca4706eaa0d2f4baaa89b2c76acb0f38fa2706c9e3ea597feef7f`; users_test.go `da1f11ff56f840ff9e35bc446c47cd4f5368df8418d79e65bc6be01d19ce30d9`; grades_test.go `29fc5e9b214b38279de9ecb658dfaf414540d28df7effa74bc44c339237c6f1d`; store/users.go `02f018526fbfd632f9f358c459387fd12810b9b403223a47432cd75f8a605f12`; store/grades.go `64c06921a1b485535d45d8aa810e210b000ed4bea581bb83ec42407fb21c6bc4`.
**Evidence capture note**: The first build wrapper attempt could not record `$?` because zsh reserves `status`; the Go build itself completed, then the wrapper failed. The command was rerun using a writable variable; the final complete log and exit record report build exit 0. This was not a source or Go build failure.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
