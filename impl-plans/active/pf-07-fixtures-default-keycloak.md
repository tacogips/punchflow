# PF-07 Default Fixtures and Keycloak Implementation Plan

**Status**: Completed
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-07
**Design Reference**: design-docs/specs/architecture.md#fixtures (default content); design-docs/specs/design-auth-admin.md#keycloak-dev-deployment-deploykeycloak; design-docs/specs/design-approval-engine.md (step types, branching)
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Author the realistic default data set embedded in the binary (`fixtures/default/*.json`), which every later test and the README use, plus the Keycloak docker-compose and realm import whose users mirror the fixture users. The JSON shapes are pinned by `internal/fixtures/types.go` and `internal/model/definition.go` (PF-02). The loader/validator (PF-06) is built in parallel, so this plan verifies by strict decoding; PF-16 later runs full validation and the byte-exact round-trip.

## Non-goals

- No Go logic outside `fixtures_default_test.go`. Do not edit `internal/fixtures/*` or `internal/model/*`.
- No tickets, comments or webhooks in the default set (`webhooks.json` = `[]`).

## Files

`fixtures/default/tenant.json`, `grades.json`, `users.json`, `organization-charts.json`, `roles.json`, `categories.json`, `folders.json`, `general-masters.json`, `routes.json`, `workflows.json`, `proxy-applicants.json`, `proxy-approvers.json`, `tokens.json`, `webhooks.json`; `deploy/keycloak/docker-compose.yml`, `deploy/keycloak/realm-punchflow.json`; `fixtures_default_test.go` (module root, `package punchflow`). Keep `fixtures/default/.gitkeep` (PF-01).

## Path Declarations

### writePaths
- `impl-plans/active/pf-07-fixtures-default-keycloak.md`
- `fixtures/default/tenant.json`
- `fixtures/default/grades.json`
- `fixtures/default/users.json`
- `fixtures/default/organization-charts.json`
- `fixtures/default/roles.json`
- `fixtures/default/categories.json`
- `fixtures/default/folders.json`
- `fixtures/default/general-masters.json`
- `fixtures/default/routes.json`
- `fixtures/default/workflows.json`
- `fixtures/default/proxy-applicants.json`
- `fixtures/default/proxy-approvers.json`
- `fixtures/default/tokens.json`
- `fixtures/default/webhooks.json`
- `deploy/keycloak/docker-compose.yml`
- `deploy/keycloak/realm-punchflow.json`
- `fixtures_default_test.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Pinned Content (other plans and README rely on these codes)

- Tenant: `publicId` `punchflow-dev`, name `punchflow dev tenant`, plan `enterprise`, mode `normal`, empty allowlists, `selfServiceProxy` false.
- Grades (code/level): `executive` 50, `director` 40, `manager` 30, `chief` 20, `staff` 10 (default).
- Users (code; e-mail `<code>@punchflow.test`; status `activated` unless noted): `admin`, `ceo`, `sales-manager`, `sales-deputy`, `sales-rep`, `eng-manager`, `engineer`, `finance-manager`, `accountant`, `hr-manager`, `assistant` (userType `assistant`), `suspended-user` (`suspended`), `invited-user` (`invited`).
- Org chart `2026 Org` (current) teams (code, parent, leader, members with grade): `company` (root; leader ceo/executive; admin/staff), `sales` (company; leader sales-manager/manager; sales-deputy/chief; sales-rep/staff), `sales-east` (sales; leader sales-deputy/chief; sales-rep/staff), `engineering` (company; leader eng-manager/manager; engineer/staff), `platform` (engineering; leader eng-manager/manager; engineer/staff), `finance` (company; leader finance-manager/director; accountant/staff), `hr` (company; leader hr-manager/manager; assistant/staff; suspended-user/staff; invited-user/staff). Second chart `2027 Plan` (not current) with team `company` led by ceo.
- Roles: `Super Admin` (editable false, all 19 permissions, member admin); `Ticket Admin` (`ticket_read`, `ticket_write`; member hr-manager); `Master Editor` (`master`, `master_read`; member accountant); `Integration Admin` (`integration`; member eng-manager).
- Categories: `expense`, `purchasing`, `hr`. Folders: `root` (no parent, editable false), `finance`, `general`.
- General masters: `cost-centers` (field `budget-owner` text; items `cc-sales`, `cc-eng`, `cc-legacy` with `endsOn` `2020-12-31`), `vendors` (field `country` pull_down [JP, US]; items `vendor-acme`, `vendor-globex`).
- Routes:
  - `expense-standard`: 1 manager closest approve (number 1, fallback `higher_manager`); 2 team `finance` approve (number 1).
  - `expense-large`: 1 manager closest approve; 2a team `finance` approve number 1 ("any") and 2b user `ceo` approve percent 100 ("all"), both `stepOrder` 2 (parallel group); 3 user `accountant` actionType `confirm`.
  - `purchase`: 1 `author_customizable` code `nominee`, minCustomAssignees 1; 2 `dynamic_user` variable `reviewer`; 3 team `engineering` gradeSymbol `greater_than_or_equal` [manager] with condition `conditional_skip` all [checkbox_variable `urgent` include `yes`]; 4 `dynamic_team` variable `approving-team`; 5 user `finance-manager` with condition `conditional` all [number_variable `amount` greater_than_or_equal `50000`].
  - `leave`: 1 manager closest fallback `skip`; 2 `assignee_customizable` code `delegate`; 3 team `hr` actionType `none`; 4 `author` actionType `confirm`.
- Workflows:
  - `expense-claim` (folder finance, category expense): titleInputMode `input`; ticketNumberFormat `EXP-[year]-[number]`, timing `opened`; form section fields `amount` number required, `purpose` text required, `expense-date` date, `cost-center` master (cost-centers), `receipt` file, `approver-note` text_long (approver true, author false, required false); slip section code `lines` with `item` text, `qty` integer, `unit-price` number. routeConditions: amount greater_than `10000000` -> errorMessage `Amount exceeds the limit`; amount greater_than_or_equal `100000` -> `expense-large`; `field_otherwise` -> `expense-standard`. availableToEveryone true.
  - `purchase-request` (folder general, category purchasing): titleInputMode `calculate`; fields `amount` number required, `item` text required, `urgent` checkbox [yes], `reviewer` user, `approving-team` team, `vendor` master (vendors), `related` ticket (autoLink true, allowedTicketStatus all), `budget-code` text (approver true, author false, required true). routeConditions: `always` -> `purchase`. commentingRequiredOnRejection true. availableToEveryone false, availableTeams `sales` and `engineering` (descendants true).
  - `leave-request` (folder general, category hr): titleInputMode `none`; commentingEnabled false; section 1 fields `start-date` date required, `end-date` date, `reason` pull_down [vacation, sick, other]; section 2 `conditional` all [reason equal `other`] with field `detail` text required. routeConditions: `always` -> `leave`.
- Proxies: approver user `sales-manager` -> proxy `sales-deputy`, no dates, all workflows. Applicant user `ceo` -> proxy `assistant`, workflows [`expense-claim`].
- Tokens (dev-only plaintext): `pf_pat_dev_admin` (pat, admin), `pf_pat_dev_manager` (sales-manager), `pf_pat_dev_finance` (finance-manager), `pf_pat_dev_employee` (sales-rep), `pf_sat_dev_integration` (sat, name `integration-test`).
- Every entity and sub-entity has a fixed, hand-chosen UUID v4 (generate once and keep).

## Keycloak

- `docker-compose.yml`: service `keycloak`, image `quay.io/keycloak/keycloak:<explicit current release tag>` (record the tag and why in the progress log; never `latest`), command `start-dev --import-realm`, ports `8180:8080`, env `KC_BOOTSTRAP_ADMIN_USERNAME=${KC_BOOTSTRAP_ADMIN_USERNAME:-admin}`, `KC_BOOTSTRAP_ADMIN_PASSWORD=${KC_BOOTSTRAP_ADMIN_PASSWORD:-admin}` (dev-only default, comment says so), volume `./realm-punchflow.json:/opt/keycloak/data/import/realm-punchflow.json:ro`.
- `realm-punchflow.json`: realm `punchflow`, enabled; client `punchflow-ui` with `publicClient: true`, `standardFlowEnabled: true`, `directAccessGrantsEnabled: false`, `implicitFlowEnabled: false`, attribute `pkce.code.challenge.method: S256`, redirect URI `http://localhost:8080/auth/oidc/callback`, web origin `http://localhost:8080`; one user per fixture user with the same e-mail, `username` = code, first/last name, `emailVerified: true`, `enabled: true`, credential password `punchflow` (`temporary: false`, dev-only).

## Key Points and Pitfalls

- Write JSON in canonical form: 2-space indent, keys in the struct field order of `types.go` / `definition.go`, arrays sorted by `code` (by `id` where there is no code), ordered collections in logical order, trailing newline. PF-16 may regenerate the files with `fixtures export` if bytes differ; semantic content must not change then.
- Every route step sets exactly one of `requiredApprovalsNumber` > 0 or `requiredApprovalsPercent` > 0. No `custom` combinations, no `other_variable`.
- Route step condition variables (`urgent`, `amount`, `reviewer`, `approving-team`) must be form field codes of the workflow using the route.
- Plaintext dev tokens are intentionally low-entropy (gitleaks). Do not use random-looking secrets.

## Tests (`fixtures_default_test.go`)

- each `fixtures/default/*.json` decodes from `DefaultFixtures` with `DisallowUnknownFields` into its type -> no error
- the 5 persona tokens exist with the pinned values and owners
- realm users' e-mails == fixture users' e-mails (set equality); realm client has PKCE S256 and the pinned redirect URI
- workflows `expense-claim`, `purchase-request`, `leave-request` and routes `expense-standard`, `expense-large`, `purchase`, `leave` exist
- every route step has exactly one positive required-approval value

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf07-build.log
go vet . 2>&1 | tee /tmp/pf07-vet.log
go test . 2>&1 | tee /tmp/pf07-test.log
gofmt -l fixtures_default_test.go   # prints nothing
python3 -I -m json.tool deploy/keycloak/realm-punchflow.json > /dev/null && echo realm-json-ok   # or: bun -e "JSON.parse(await Bun.file('deploy/keycloak/realm-punchflow.json').text())"
docker compose -f deploy/keycloak/docker-compose.yml config -q 2>&1 | tee /tmp/pf07-compose.log  # if docker is unavailable, record "skipped: no docker" (not a failure)
```

## Done Criteria

- [x] All 14 fixture files and both Keycloak files exist with the pinned content
- [x] Root package tests pass; JSON valid; compose config valid or explicitly skipped (no docker)

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20
**Intent snapshot**: Created the 14 pinned `fixtures/default/*.json` files, Keycloak compose and realm import, and `fixtures_default_test.go` for strict fixture decoding, persona/definition invariants, and realm-to-fixture/PKCE agreement. No files outside PF-07 writePaths were changed.
**Tasks Completed**: Authored the fixture set and Keycloak setup. Sorted fixture collections canonically while preserving ordered workflow sections and route steps. Pinned Keycloak image `quay.io/keycloak/keycloak:26.8.1`, the upstream release current on 2026-10-20 (official 26.8.1 release, 2026-10-13).
**Verification**:
- `go vet .` exit 0, `/tmp/pf07-vet.log`.
- `gofmt -l fixtures_default_test.go` exit 0, no output, `/tmp/pf07-gofmt.log`.
- `python3 -I -m json.tool deploy/keycloak/realm-punchflow.json` exit 0, `realm-json-ok`, `/tmp/pf07-json.log`.
- `docker compose -f deploy/keycloak/docker-compose.yml config -q` exit 0, `/tmp/pf07-compose.log`.
- `go build ./...` exit 1, `/tmp/pf07-build.log`; current shared tree has compile errors in PF-06-owned `internal/store/read_masters.go`, `internal/store/read_proxies.go`, and `internal/store/webhooks.go`.
- `go test .` exit 1, `/tmp/pf07-test.log`; compiling root tests now reaches the same PF-06 `internal/store` errors through `internal/fixtures`. An earlier `go test .` passed before those PF-06 files appeared and is not treated as current-source evidence.
**Reconciliation requests**: PF-06/PF-27: repair the current `internal/store` compile errors, then rerun `go test .` and `go build ./...` against the final shared tree. PF-07-owned source is complete; no out-of-scope files were edited.

### Session: 2026-10-20 (verification resolution)
**Tasks Completed**: Corrected duplicate UUIDs for the 2027 company team and the expense/purchase amount fields. All 121 fixture IDs are now unique. The PF-06 compile errors observed during a moving-tree run did not reproduce once that shared work stabilized; no PF-07 repair was needed. The plan is implementation-complete; PF-27 owns later combined-tree reconciliation and active-to-completed plan archiving/index maintenance.
**Verification**:
- `go build ./...` exit 0, `/tmp/pf07-build.log`.
- `go vet .` exit 0, `/tmp/pf07-vet.log`.
- `go test -count=1 -v .` exit 0; 4 top-level tests and 14 strict-decode subtests passed (18 total, 0 failures), `/tmp/pf07-test.log`.
- `gofmt -l fixtures_default_test.go` exit 0, no output, `/tmp/pf07-gofmt.log`.
- `python3 -I -m json.tool deploy/keycloak/realm-punchflow.json` exit 0, `/tmp/pf07-json.log`.
- `docker compose -f deploy/keycloak/docker-compose.yml config -q` exit 0, `/tmp/pf07-compose.log`.
- Fixture content check: 14 JSON files valid, 13 realm user e-mails match fixture users, 121 unique IDs, and route approval constraints hold, `/tmp/pf07-content-check.log`.
**Earlier attempts**: transient shared-tree `go build ./...` and `go test .` failures and the duplicate-ID assertion are preserved under `tmp/punchflow-kickflow-dummy-s460/PF-07/attempt-1/prior-*-failure.log`; all were resolved on current source by shared-tree stabilization or the UUID correction above.
**Reconciliation requests**: None for PF-07. Final plan archiving and shared index updates remain with PF-27.
