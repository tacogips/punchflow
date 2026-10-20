# PF-19 /v1 Proxies, Organization Charts and Teams Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-08, PF-16 | **Plan ID**: PF-19
**Design Reference**: design-docs/specs/design-api-coverage.md rows 19-42; design-docs/references/kickflow-api.md section 3 "Proxy applicants", "Proxy approvers", "Organization charts", "Teams"; notes.md INF-16, INF-23, INF-29
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Replace the PF-10 stubs for the 24 proxy, org chart and team operations. Proxy approver registrations feed the engine's proxy approval (INF-16), and org data feeds route resolution, so the semantics here matter for later tickets.

## Non-goals

- No asynchronous jobs (INF-23: synchronous). No activation schedules (`activationPlan` stays null). No changes outside the listed files.

## Files

`internal/api/v1/proxies.go`, `orgcharts.go`, `teams.go` (shared: replace stubs); tests `proxies_test.go`, `orgcharts_test.go`, `teams_test.go` (`package v1_test`); `internal/store/proxies.go`, `internal/store/org.go` (new).

## Path Declarations

### writePaths
- `impl-plans/active/pf-19-api-proxies-org.md`
- `internal/api/v1/proxies_test.go`
- `internal/api/v1/orgcharts_test.go`
- `internal/api/v1/teams_test.go`
- `internal/store/proxies.go`
- `internal/store/org.go`

### sharedPaths
- `internal/api/v1/proxies.go`
- `internal/api/v1/orgcharts.go`
- `internal/api/v1/teams.go`

### sharedPathNotes
- `internal/api/v1/proxies.go`: PF-10 stub file; replace the bodies of operations 19-26
- `internal/api/v1/orgcharts.go`: PF-10 stub file; replace the bodies of operations 27-33
- `internal/api/v1/teams.go`: PF-10 stub file; replace the bodies of operations 34-42

### artifactRoots
- (none)

## Handler Conventions

Same as PF-18 "Handler Conventions" (permission first, id-or-code resolution, one tx for mutations with an audit log, presenters on the tx, pagination via httpx, `WriteEmpty` for empty-body responses).

## Operation Notes

| Op | Perm | Rules |
|---|---|---|
| listProxyApplicants / listProxyApprovers | user | paginated |
| create / update / delete proxy (applicant and approver) | `user`, or self when tenant `self_service_proxy` and body/target `userId` == principal | body `userId`, `proxyUserId`, `startsOn`, `endsOn` (dates), `workflowIds` (empty = all; de-duplicate case-insensitively); same pair exists -> 422; user == proxyUser -> 422; `startsOn > endsOn` -> 422; unknown workflow -> 422; audit `proxyApplicant.*` / `proxyApprover.*` |
| listOrganizationCharts | team | without teams; paginated |
| createOrganizationChart | team | `name`; `current` false; audit `organizationChart.create` |
| getOrganizationChart / getCurrentOrganizationChart / updateOrganizationChart | team | update `name` |
| deleteOrganizationChart | team | current chart -> 422 `validation_failed`; deletes teams and memberships synchronously; empty 200 |
| activateOrganizationChart | team | sets `current` on this chart and clears the others in one tx; returns OrganizationChartDetail |
| listTeams | team | `parentId` query -> children, absent -> roots; paginated |
| createTeam / updateTeam | team | body per TeamCreateBody/TeamUpdateBody; code unique per chart (random when omitted); `parentId` must be in the same chart; parent cycles -> 422 |
| deleteTeam | team | deletes memberships; child teams -> 422 `validation_failed` (inferred; log) |
| listTeamMembers | team | MemberUser (no `userType`), paginated |
| createTeamMembers | team | body per spec (`users[]` with userId, leader, gradeIds); max 10 -> 400; team ends without any leader -> 422; existing member -> 422 |
| deleteTeamMembers | team | JSON body on DELETE with user ids; max 10; leaderless remainder (if members remain) -> 422 |
| updateTeamMember | team | leader / grades; leaderless result -> 422 |

## Key Points and Pitfalls

- DELETE with a JSON body: `httpx.DecodeJSON` works for DELETE too; do not reject bodies on DELETE.
- Membership grade ids must be existing grades; `grades` must stay non-empty (MemberUser `grades` minItems 1) -> otherwise 422.
- Team `{teamId}` accepts id or code within the chart; use `store.TeamIDByCode(chartID, code)` from PF-08 for codes.

## Tests (`package v1_test`)

- proxyApprovers list Admin -> 1 row (sales-manager -> sales-deputy); create duplicate pair -> 422; create with `workflowIds` [a, A-case-variant] -> stored once; Employee create for self with self-service off -> 403; tenant `self_service_proxy=1` -> 200
- orgCharts: current = `2026 Org`; activate `2027 Plan` -> current switches and getCurrent returns it; delete current -> 422; delete non-current -> 200 and its teams gone
- teams: listTeams roots -> [company]; `parentId=<sales>` -> [sales-east]; create team with parent in other chart -> 422; set parent to own child -> 422
- memberships: add 11 users -> 400; remove the only leader of `finance` while accountant remains -> 422; update member to leader -> 200
- pagination headers on listTeamMembers; Employee on any org endpoint -> 403 `missing_permission`
- audit actions `organizationChart.activate`, `proxyApprover.create` recorded

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-08, PF-16.
**State at 410d3d8**: not started (PF-10 stubs). Implement this plan as written. Session-462 points:

- Permissions are `model.PermissionUser` (proxies) and `model.PermissionTeam` (org charts, teams, memberships). No literal permission strings.
- Name tests `TestProxy*`, `TestProxies*`, `TestOrgChart*`, `TestOrganizationChart*`, `TestTeam*`; prefix helpers with `pf19`. The shared-package compile rule is README rule 11.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf19-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/api/v1/... 2>&1 | tee /tmp/pf19-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'Test(Proxy|Proxies|OrgChart|OrganizationChart|Team)' ./internal/api/v1/... 2>&1 | tee /tmp/pf19-s462-test.log"; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf19-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf19-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf19-s462-test.log)"
gofmt -l internal/api/v1/proxies.go internal/api/v1/orgcharts.go internal/api/v1/teams.go internal/api/v1/proxies_test.go internal/api/v1/orgcharts_test.go internal/api/v1/teams_test.go internal/store/proxies.go internal/store/org.go   # prints nothing
rg -n 'not implemented' internal/api/v1/proxies.go internal/api/v1/orgcharts.go internal/api/v1/teams.go   # prints nothing (exit 1)
```

### Done Criteria (session 462)

- [ ] 24 operations implemented (no stub message left); listed tests pass uncached with `-race`; counts recorded; build/vet exit 0; gofmt clean

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-08 (accepted at f82e374) and PF-16 (wave A).
**State at f82e374**: not started. `proxies.go`, `orgcharts.go` and `teams.go` are PF-10 stubs. `internal/store/proxies.go` and `internal/store/org.go` do not exist.
**Path Declarations**: unchanged. 9 declared files (< 300).

1. Implement the whole plan above, with the session-462 points.
2. Edit only the handler bodies in your three shared stub files. Never edit `internal/api/v1/api.go`, `routes.go` or `routes_test.go` (accepted PF-10; the 96 routes are fixed), `internal/testutil/*` or `internal/server/*` (PF-16), or the accepted `internal/store/read_org.go` and `read_proxies.go` (PF-08). Reuse those read helpers rather than duplicating them.
3. Shared-package compile failures caused by another plan's file: record a reconciliation request (README rules 11 and 12) and do not edit that file.
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-19/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, PASS > 0, and the `not implemented` rg prints nothing
- [ ] The progress log has the intent snapshot, pre/post hashes of the three shared files, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B (parallel with PF-18, PF-20..PF-25). It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-08 (accepted at `44e40cc`) and PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-api-coverage.md` operation rows 19-42; INF-16 and INF-23 in `notes.md`.

**Intent**: unchanged. Implement operations 19-42 (proxies, org charts, teams).

**Session-464 changes**:
1. `internal/api/v1/proxies.go`, `orgcharts.go` and `teams.go` are in this plan's `writePaths`, transferred from PF-10. `sharedPaths` is empty. Replace handler bodies only and keep the handler names that `routes.go` references. Never edit `api.go`, `routes.go`, `routes_test.go`, `internal/testutil/*`, `internal/server/*` or `internal/fixtures/*`.
2. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/api/v1/...`.
   - Log each result.
3. Prefixes: unexported identifiers start with `pf19`. New exported store functions start with `Proxy`, `OrgChart` or `Team`.
4. Team codes are unique per organization chart, not globally: the PF-06 repair at `44e40cc`. Every team lookup by code must be scoped to a chart, normally the active one.
5. Guard: the manifest `-run 'TestPF10'` line must PASS. Logs are `/tmp/pf19-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-19/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, PASS>0; PF10 guard PASS; the `not implemented` rg empty
- [ ] Pre/post hashes of the three transferred files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B1 (with PF-18, PF-20, PF-21). It is dispatched as soon as PF-16 is accepted. It may run while PF-14a/b/c are still running. **Depends on**: PF-08 (accepted at `ea385c7`), PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (9 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the "prints nothing" checks in must-pass form (`...; test $? -eq 1`, `test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: implement the 24 operations, run the listed tests, and keep the PF10 guard green. Logs are `/tmp/pf19-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-<n>/`.

**Pitfalls added in s465**:
- `internal/store` may still be edited by PF-14a/b/c (wave A, `Eng*` names; PF-14c adds `engine_refs.go` with proxy-applicant lookups) and by the other B1 plans. Use your `Proxy*`/`Org*`/`Team*` store names and do not duplicate the `Eng*` lookups.
- Never touch `internal/engine/*`.

### Done Criteria (session 465)

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; PF10 guard and plan tests exit 0, FAIL=0, SKIP=0, PASS>0; the `not implemented` check exit 0; gofmt check exit 0
- [x] Pre/post hashes of the three transferred files; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). The three handler files were transferred from PF-10 in session 464.

## Progress Log

### Session: 2026-10-21 (s465 attempt 1)
**Intent snapshot**: Implemented operations 19-42 from the accepted design: proxy applicant/approver CRUD and self-service, org chart CRUD/activation, team CRUD and memberships. Used only PF-19 declared paths. Evidence directory: `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/`.
**Tasks Completed**:
- Added transactional proxy, organization-chart, team and membership persistence in `internal/store/proxies.go` and `internal/store/org.go`.
- Replaced all eight proxy, seven organization-chart and nine team handler stubs. Added pagination/presenters, permission checks, validations, audit records, code-scoped team lookup, parent-cycle checks, membership leader invariants and empty DELETE responses.
- Added `TestProxyApplicantsCRUDAndValidation`, `TestProxyApproversAndSelfServicePermission`, `TestOrganizationChartListPermissionsAndCRUD`, `TestTeamTreeCreateUpdateAndPermissions`, and `TestTeamMembershipLeaderAndPaginationRules`.
- Resolved implementation test failures during this attempt: use the actual `sales-rep` principal, allow case-insensitive workflow code lookup, require UUID parent references to belong to the requested chart, and persist team code updates.
**Verification**:
- Final `go build ./...`: exit 0, complete log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/build-final.log`.
- Final `go vet ./internal/api/v1/...`: exit 0, complete log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/vet-final.log`.
- PF10 guard `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2 FAIL=0 SKIP=0, log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/pf10guard-final.log`.
- Plan tests `go test -count=1 -v -race -run 'Test(Proxy|Proxies|OrgChart|OrganizationChart|Team)' ./internal/api/v1/...`: exit 0, PASS=5 FAIL=0 SKIP=0, log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/test-final2.log`.
- `gofmt -l` on the eight owned Go files: exit 0, no paths; log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/gofmt-final.log`.
- `rg -n 'not implemented'` over the three handler files followed by `test $? -eq 1`: exit 0 (no stub strings); log `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-1/stub-check-final.log`.
- Shared handler file SHA-256 before -> after: `proxies.go` `d2b7299307274758e49b5ab789cad0ab8d60641a418cb30f1f3389b4620e7034` -> `285f8c1ccaf03b4c8360358121e80eb01c447257828a918ba46a2b91af510cb0`; `orgcharts.go` `41dee20770f8f332021196f5e2d967a62b97cf3cc5c4b7df102eaf8b2f1e0389` -> `69fc62b6166efbb94eed0f41c7011c95adeaa67f4b4cca9e8840340383e21e7f`; `teams.go` `43e6c9703fe009382cb4576a2f4800b7179920b926e3d107a0f9b3db11b45b44` -> `a3a53c84623197313cebbb9fd021e270674a3af0e2740f7f155079cea99606f5`. Hash files: `pre-edit-hashes.txt`, `post-edit-hashes.txt`.
- Compile gate after each non-test Go write was run and logged. Early failures were resolved: PF-18 temporarily had undefined `Principal.UserID`; proxy handler edits had local compile errors corrected by subsequent writes; PF-20 temporarily had `general_masters.go` call/signature and `strconv` import errors. The later source-matched build and vet both pass. Compile logs are `compile-after-*.log`, `gate-*.log` in the attempt directory.
- Earlier behavioral runs failed before fixes and are retained as `test-attempt1.log` (implementation test defects and later-sibling compile overlap) and `test-attempt2`/`test-attempt1` in the attempt directory; final-source rerun above passes.
**Reconciliation requests**: None unresolved. Concurrent PF-18/PF-20 compile errors were not edited; the current shared tree now builds and vets successfully.

### Session: 2026-10-21 (s465 attempt 2, PF19-TI-01 repair)
**Intent snapshot**: Corrected `TestTeamMembershipLeaderAndPaginationRules` so its 11-entry `users` payload uses well-formed `{id, leader, gradeIds}` objects and asserts the specific 400 `invalid_parameter` size-limit response. All other assertions remain. Evidence directory: `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-2/`.
**Changed paths**: `internal/api/v1/teams_test.go` only among source files; no production source changed. Current SHA-256: `teams_test.go` `16aaf1a6cb7040b784aa740007c7ab9a70c53d088559b9d2ce9915f9a7da8d5c`; production hashes unchanged: `teams.go` `a3a53c84623197313cebbb9fd021e270674a3af0e2740f7f155079cea99606f5`, `proxies.go` `285f8c1ccaf03b4c8360358121e80eb01c447257828a918ba46a2b91af510cb0`, `orgcharts.go` `69fc62b6166efbb94eed0f41c7011c95adeaa67f4b4cca9e8840340383e21e7f`. Hash record: `attempt-2/current-source-hashes.txt`.
**Verification**:
- Post-test-write compile gate `go vet ./internal/api/v1/...`: exit 0; complete log `attempt-2/pf19-s465-vet.log`.
- `go build ./...`: the attempt-2 log recorded exit 0, but that result is stale and not valid for the completed attempt: the overlay source was added afterward and made the shared-tree build fail. Corrected current-tree result is recorded under attempt-3 below.
- Scoped uncached race suite `go test -count=1 -v -race -run 'Test(Proxy|Proxies|OrgChart|OrganizationChart|Team)' ./internal/api/v1/...`: exit 0, PASS=5 FAIL=0 SKIP=0; complete log `attempt-2/pf19-s465-test.log`.
- PF10 guard `go test -count=1 -v -run 'TestPF10' ./internal/api/v1/...`: exit 0, PASS=2 FAIL=0 SKIP=0; complete log `attempt-2/pf19-s465-pf10guard.log`.
- `gofmt -l internal/api/v1/teams_test.go` empty; exit 0. `git diff --check -- internal/api/v1/teams_test.go` exit 0.
- Negative mutation proof used `go test -overlay=.../overlay.json -count=1 -v -run '^TestTeamMembershipLeaderAndPaginationRules$' ./internal/api/v1/...` with a plan-local copy of `teams.go` changing only `len(b.Users) > 10` to `> 11`; expected nonzero, observed exit 1 because the corrected test rejected the 422 duplicate-member response instead of the required 400 size-limit error. Complete log `attempt-2/mutation-proof-rerun.log`; the tracked production file was not modified.

### Session: 2026-10-21 (s465 attempt 3, PF19-ADV-01 repair)
**Intent snapshot**: Renamed the plan-local overlay source to `_teams_mutated.go` so Go package discovery excludes it, then updated `attempt-2/overlay.json` to point to the renamed file. No production or test files changed. Evidence directory for fresh checks: `tmp/punchflow-kickflow-dummy-s465/PF-19/attempt-3/`.
**Correction to attempt 2**: The attempt-2 build log's exit 0 preceded creation of `teams_mutated.go`; it did not verify the final attempt-2 tree. The reviewer reproduced the resulting package build failure. The file is now renamed, and current-tree build evidence below supersedes that stale claim.
**Verification**:
- `go build ./...`: exit 0; complete log `attempt-3/build.log`.
- `go vet ./internal/api/v1/...`: exit 0; complete log `attempt-3/vet.log`.
- Scoped uncached race suite: exit 0, PASS=5 FAIL=0 SKIP=0; complete log `attempt-3/scoped-race-test.log`.
- PF10 guard: exit 0, PASS=2 FAIL=0 SKIP=0; complete log `attempt-3/pf10guard.log`.
- `go list ./...` contains no package under `tmp/punchflow-kickflow-dummy-s465/PF-19`; `find tmp/punchflow-kickflow-dummy-s465/PF-19 -name '*.go' ! -name '_*'` is empty.
- `gofmt -l` on the eight PF-19 Go files is empty; the `not implemented` check found no matches (rg exit 1, accepted no-match result).
- Production and test hashes remain unchanged at the accepted values; full list in `attempt-3/source-hashes.txt`.
- Overlay mutation proof with the updated mapping still fails as expected (observed exit 1); log `attempt-3/mutation-proof.log`. This negative proof is progress/self-check evidence, not a passing verification gate.
