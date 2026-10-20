# PF-19 /v1 Proxies, Organization Charts and Teams Implementation Plan

**Status**: Ready
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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf19-build.log
go vet ./internal/api/... ./internal/store/... 2>&1 | tee /tmp/pf19-vet.log
go test -race -run 'Test(Proxy|Proxies|OrgChart|OrganizationChart|Team)' ./internal/api/v1/... 2>&1 | tee /tmp/pf19-test.log
gofmt -l internal/api/v1/proxies*.go internal/api/v1/orgcharts*.go internal/api/v1/teams*.go internal/store/proxies.go internal/store/org.go   # prints nothing
```

## Done Criteria

- [ ] 24 operations implemented (no stub message left in the three files); tests pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. The three handler files are shared with PF-10 (stub creator).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
