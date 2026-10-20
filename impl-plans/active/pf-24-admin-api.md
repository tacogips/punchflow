# PF-24 Dev Admin API Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-14, PF-15, PF-16 | **Plan ID**: PF-24
**Design Reference**: design-docs/specs/design-auth-admin.md#dev-admin-api-internaladmin; design-docs/specs/design-webhooks.md#manual-operations-admin-api-integration-permission; design-docs/specs/design-api-coverage.md (error catalog "Fault" rows); notes.md INF-04, INF-09, INF-29, INF-32
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

---

## Intent and Context

Implement the 23 punchflow-only `/admin/api` endpoints, whose route table and stubs PF-16 created: personal tokens, service accounts, webhooks (CRUD, ping, event log, resend, fire), tenant settings, fault rules, fixture export and flag-gated reset. They reuse the kickflow error body, the httpx pagination headers and the RBAC permissions.

## Non-goals

- No changes to `internal/admin/routes.go`/`admin.go` signatures, `internal/webhook`, `internal/fixtures`, `internal/auth`. No UI.

## Files

`internal/admin/tokens.go`, `service_accounts.go`, `webhooks.go`, `tenant.go`, `faults.go`, `fixtures.go` (shared: replace stubs); tests `tokens_test.go`, `webhooks_test.go`, `tenant_test.go`, `faults_test.go`, `fixtures_test.go` (`package admin_test`); `internal/store/admin.go` (new: token and tenant queries not already in `store/auth.go`).

## Path Declarations

### writePaths
- `impl-plans/active/pf-24-admin-api.md`
- `internal/admin/tokens_test.go`
- `internal/admin/webhooks_test.go`
- `internal/admin/tenant_test.go`
- `internal/admin/faults_test.go`
- `internal/admin/fixtures_test.go`
- `internal/store/admin.go`

### sharedPaths
- `internal/admin/tokens.go`
- `internal/admin/service_accounts.go`
- `internal/admin/webhooks.go`
- `internal/admin/tenant.go`
- `internal/admin/faults.go`
- `internal/admin/fixtures.go`

### sharedPathNotes
- `internal/admin/tokens.go`: PF-16 stub file; implement the personal token endpoints
- `internal/admin/service_accounts.go`: PF-16 stub file; implement the service account endpoints
- `internal/admin/webhooks.go`: PF-16 stub file; implement webhook CRUD, ping, events, resend and fire
- `internal/admin/tenant.go`: PF-16 stub file; implement tenant GET/PATCH
- `internal/admin/faults.go`: PF-16 stub file; implement the fault rule endpoints
- `internal/admin/fixtures.go`: PF-16 stub file; implement fixture export and flag-gated reset

### artifactRoots
- (none)

## JSON Shapes (camelCase, timestamps via `kftime.Format`)

- Token: `{id, name, createdAt, lastUsedAt}`; create response adds `token` (once). ServiceAccount: `{id, name, description, createdAt, lastUsedAt}` (+ `token` on create).
- Webhook: `{id, name, url, hasSecret, enabled, workflowIds, eventTypes, firstFailureAt, disabledAt, createdAt, updatedAt}`; create/update responses also include `secret`.
- WebhookEvent: `{id, webhookId, eventType, ticketId, commentId, status, attemptCount, nextAttemptAt, lastStatusCode, createdAt, body (string, only on GET by id), attempts[] (only on GET by id: {attemptNo, startedAt, durationMs, requestHeaders, statusCode, responseBody, error, manual})}`.
- Tenant: `{id, publicId, name, plan, mode, ipAllowlist, allowedEmailDomains, selfServiceProxy}`. FaultRule: `httpx.FaultRule` fields camelCased.

## Behavior

- Tokens: list own PATs only; create `{name}` via `Auth.IssueToken(kind pat, principal user)`; delete own or with `integration` (others -> 404 to avoid leaking). Audit `token.create|delete`.
- Service accounts (`integration`): `kind sat`, `user_id` NULL; audit `serviceAccount.*`.
- Webhooks (`integration`): validate `url` (http/https, absolute), `eventTypes` subset of the 12 non-ping types, `workflowIds` existing. Create -> insert + `Webhooks.EnqueueTo(ping, [id])`; PATCH changing url or secret -> ping; PATCH `enabled:true` clears `disabledAt`/`firstFailureAt`. Ping endpoint -> `EnqueueTo` ping and return the event. Resend -> `Webhooks.Resend`. Fire `{eventType, ticketId?, commentId?, webhookIds?}` -> validate presence by type (ticket_* need ticketId; comment_* need both, and the comment must belong to the ticket); with `webhookIds` -> `EnqueueTo`, otherwise `Enqueue` (normal filters); actor = principal. Call `Webhooks.Notify()` after every commit that enqueued.
- Tenant (`tenant`): PATCH validates enums, CIDRs/IPs (`net/netip`), domains (no `@`).
- Faults (`tenant`): validate `status` 400-599, non-empty `code` and `pathPrefix` starting with `/v1/`; stored in `Deps.Faults`.
- Fixtures (`tenant`): export -> `fixtures.Export` -> JSON object `{fileName: <parsed JSON>}`; reset -> `Deps.EnableReset` false -> 404 `endpoint_not_found`; else `fixtures.Reset(ctx, db, FixturesFS, FilesDir, clock)`, clear `Deps.Faults`, return `{counts}`. A successful reset invalidates all sessions and runtime tokens, because the tables are truncated; the response tells the caller to use seeded tokens.

## Key Points and Pitfalls

- Never return a token or webhook secret in list responses.
- Reset runs its own transaction inside `fixtures.Reset`: do not wrap it in another `WithTx` (single connection deadlock).
- Event types for fire must be one of the 13 (ping allowed with webhookIds only).

## Tests (`package admin_test`, `testutil`)

- tokens: Employee creates PAT -> token works on `GET /v1/user` (stub or real; assert not 401); list shows no `token` key; delete -> token now 401
- service account: Employee -> 403; Admin create -> SAT + `X-Caller-Id` works
- webhooks: create with receiver URL -> receiver gets `ping` after `DeliverWebhooks`; list has `hasSecret` true and no `secret`; PATCH url -> second ping; invalid eventType -> 422
- resend succeeded event -> new attempt with the same delivery id; GET event -> attempts with `manual` true on the last
- fire `ticket_approved` for a ticket created in the test with `engine.New(env.Clock).Create` inside `env.DB.WithTx` (do not depend on `POST /v1/tickets`, which PF-22 builds in parallel) -> receiver gets it, signature verifies; `comment_created` without commentId -> 400; all 13 event types can be fired (table test)
- tenant PATCH mode `maintenance` -> `/v1/user` 403 `tenant_maintenance`; invalid CIDR -> 422
- faults: add `{pathPrefix:"/v1/tickets", status:503, code:"feature_disabled", remaining:1}` -> next `/v1/tickets` 503 `feature_disabled`, then normal; repeat for `gateway_timeout` 504, `email_not_verified` 401, `license_purchase_required` 403, `option_purchase_required` 403, `internal_server_error` 500 (INF-32 catalog)
- fixtures: export has 14 files; reset without flag -> 404; with `WithAdminReset()` -> 200 and a ticket created earlier is gone, `pf_pat_dev_admin` still works

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-14, PF-15, PF-16.
**State at 410d3d8**: not started (`internal/admin` is created by PF-16 in phase 3). Implement this plan as written. Session-462 points:

- Webhook `eventTypes` are validated against the 12 non-ping values of `model.WebhookEventTypes`. The fire endpoint accepts all 13 (`ping` only with `webhookIds`). The table test iterates `model.WebhookEventTypes`, so it covers exactly the kickflow 13. `ticket_submitted` or `comment_deleted` -> 422.
- Webhook event `status` in responses is `pending|succeeded|failed` (`model.WebhookStatus*`).
- Tenant PATCH validates `plan` and `mode` against the `model.TenantPlan*`/`model.TenantMode*` constants. Permissions are `model.PermissionIntegration` (service accounts, webhooks) and `model.PermissionTenant` (tenant, faults, fixtures).
- Fixtures export/reset now include the CF-02 input IDs. The test "export has 14 files" also asserts that `general-masters.json` items have non-empty `inputs[].id`.
- Name tests `TestAdmin*` or by resource (`TestTokens*`, `TestServiceAccount*`, `TestWebhooks*`, `TestTenant*`, `TestFaults*`, `TestFixtures*`); prefix helpers with `pf24`. PF-16's `routes_test.go` lives in the same package; it must stay green.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf24-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/admin/... 2>&1 | tee /tmp/pf24-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/admin/... 2>&1 | tee /tmp/pf24-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf24-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf24-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf24-s462-test.log)"
gofmt -l internal/admin internal/store/admin.go   # prints nothing
```

Evidence: exit 0; FAIL=0; SKIP=0; the fault-code table test shows 6 passing subtests; the fire-event table test shows 13.

### Done Criteria (session 462)

- [ ] 23 endpoints implemented; every "Fault" catalog code proven; fire covers exactly the 13 kickflow events
- [ ] Uncached `-race -v` admin tests pass with counts recorded; build/vet exit 0; gofmt clean

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: B (s463). **Depends on**: PF-14, PF-15 and PF-16 (all wave A).
**State at f82e374**: not started. `internal/admin` does not exist yet; PF-16 creates it with the stub files you fill. `internal/store/admin.go` does not exist.
**Path Declarations**: unchanged. 13 declared files (< 300).

1. Implement the whole plan above, with the session-462 points.
2. `internal/admin/webhooks_test.go` must keep the negative cases `ticket_submitted -> 422` and `comment_deleted -> 422` using those literal strings. PF-27's `repo_invariants_test.go` subtest `negative_tests_present` checks that a test file under `internal/admin/` contains one of them.
3. Never edit `internal/admin/admin.go`, `routes.go` or `routes_test.go` (PF-16; 23 rows fixed), `internal/webhook/*` (accepted PF-09/PF-15), `internal/fixtures/*` (accepted PF-06) or `internal/testutil/*` (PF-16). Defects in those become reconciliation requests (README rule 12).
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-24/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, 6 fault subtests and 13 fire subtests PASS
- [ ] `rg -l 'ticket_submitted|comment_deleted' internal/admin` lists `internal/admin/webhooks_test.go`
- [ ] The progress log has the intent snapshot, pre/post hashes of the six shared stub files, and exit status, log path and counts for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted and `go build ./...` exits 0. **Depends on**: PF-14, PF-15, PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` (`writePaths`, `transferredFileNotes` and `verification` are authoritative).
**Design**: `design-docs/specs/design-auth-admin.md`, the dev admin API table (23 operations); `design-webhooks.md` manual operations; INF-04, INF-09, INF-32 in `notes.md`; CF-01.

**Intent**: unchanged. Implement the 23 `/admin/api` endpoints:
- the fault catalog (6 subtests)
- `fire` covering exactly the 13 kickflow events (13 subtests)
- export with non-empty `inputs[].id`
- the literal `ticket_submitted` and `comment_deleted` -> 422 negative cases in `webhooks_test.go`, required by PF-27's `negative_tests_present`

**Session-464 changes**:
1. `internal/admin/{tokens,service_accounts,webhooks,tenant,faults,fixtures}.go` are in this plan's `writePaths`, transferred from PF-16. They are compiling stubs at `44e40cc`. `sharedPaths` is empty. Keep every function name that `internal/admin/routes.go` (PF-16) references; `routes_test.go` must keep reporting 23 operations.
2. Never edit:
   - `internal/admin/admin.go`, `routes.go`, `routes_test.go` (PF-16)
   - `internal/testutil/*`, `internal/server/*`
   - `internal/webhook/*`, `internal/fixtures/*`, `fixtures/default/*`

   Missing behavior there is a reconciliation request to its owner.
3. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet ./internal/admin/...`.
   - Log each result.
   - Prefixes: unexported identifiers start with `pf24`. New exported store functions start with `Admin`.
4. The reset endpoint stays flag-gated: 404 `endpoint_not_found` when `EnableAdminReset` is false. Tests use `testutil.WithAdminReset()`.
5. Logs are `/tmp/pf24-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-24/attempt-<n>/`.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0; 6 fault subtests and 13 fire subtests PASS; PF-16's `routes_test.go` PASS
- [ ] `rg -l 'ticket_submitted|comment_deleted' internal/admin` lists `internal/admin/webhooks_test.go`
- [ ] Pre/post hashes of the six transferred stub files; every source write followed by a logged compile gate with exit 0

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: B2 (with PF-22, PF-23). **Depends on**: PF-14a, PF-14b, PF-14c, PF-15, PF-16. The single PF-14 dependency is replaced by its three split plans. Your fire tests create tickets through `Engine.Create`, which uses all three.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (13 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the gofmt check in must-pass form (`test -z "$(gofmt -l ...)"`).

**Intent and tasks**: unchanged from the session-464 section: the 23 admin endpoints, the fault catalog, `fire` for exactly the 13 kickflow events, and the negative `ticket_submitted`/`comment_deleted` test in `internal/admin/webhooks_test.go` (PF-27's `negative_tests_present` depends on it). Logs are `/tmp/pf24-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-<n>/`.

**Pitfalls added in s465**: never edit `internal/engine/*` or PF-16's `internal/admin/admin.go`, `routes.go` or `routes_test.go`. An engine defect is a reconciliation request to the owner by file, as listed in PF-22's s465 section.

### Done Criteria (session 465)

- [x] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; admin tests exit 0, FAIL=0, SKIP=0, with 6 fault subtests and 13 fire subtests PASS and PF-16's `routes_test.go` PASS; `rg -l 'ticket_submitted|comment_deleted' internal/admin` lists `internal/admin/webhooks_test.go`; gofmt check exit 0
- [x] Pre/post hashes of the six transferred stub files; every source write followed by a logged compile gate with exit 0

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). The six handler files were transferred from PF-16 in session 464.

## Progress Log

### Session: 2026-10-21 (attempt 1)
**Status**: Completed implementation; downstream serial integration review and PF-27 full gate remain pending.
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/edit-intent.md`.
**Tasks Completed**: Implemented all 23 `/admin/api` handlers in the six transferred files; added admin store queries in `internal/store/admin.go`; added token/SAT, webhook, tenant, fault and fixture endpoint tests. Token and secret values are omitted from list responses; fixture reset remains flag-gated. Fire test iterates all 13 model webhook events and preserves the literal negative cases `ticket_submitted` and `comment_deleted`.
**Changed paths**: `internal/admin/{tokens,service_accounts,webhooks,tenant,faults,fixtures}.go`, `internal/store/admin.go`, `internal/admin/{tokens,webhooks,tenant,faults,fixtures}_test.go`.
**Verification**:
- `go build ./...`: exit 0, `/tmp/pf24-s465-build-final2.log` (copied to `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/build-final.log`).
- `go vet ./internal/admin/...`: exit 0, `/tmp/pf24-s465-vet-final3.log` (copied to `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/vet-final.log`).
- `go test -count=1 -v -race ./internal/admin/...`: exit 0, PASS=31, FAIL=0, SKIP=0, cached=0, `/tmp/pf24-s465-test-final3.log` (copied to `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/test-final.log`). `TestFaultCatalog` has six passing subtests; `TestWebhookFireEventTypesAndNegativeEvents` has 13 valid event cases plus both negative cases passing; `TestOperationsContractAndUnknownRoute` passes.
- `test -z "$(gofmt -l internal/admin internal/store/admin.go)"`: exit 0 (clean).
- `rg -l 'ticket_submitted|comment_deleted' internal/admin`: exit 0 and lists `internal/admin/webhooks_test.go`.
- Six transferred handler pre/post SHA-256 manifests: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/{pre-stub-hashes,post-stub-hashes}.txt`.
- Per-write build/vet history and complete intermediate logs: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-1/compile-gate-history.txt` and corresponding `build-after-*`, `vet-after-*` files.
**Prior verification**: Early shared-tree build failures were foreign transient PF-23 (`internal/files/files.go`) breakage, and early vet failures were transient PF-22 (`internal/api/v1/tickets.go`, `ticket_viewers.go`) breakage. PF-24 made no edits to those paths. Current final build/vet/test gates pass; intermediate diagnostics are retained under the attempt directory.
**Reconciliation requests**: None outstanding for PF-24. Integration review and PF-27's full repository gate own downstream combined-tree acceptance.

### Session: 2026-10-21 (attempt 2, test-integrity repairs)
**Status**: PF24-TI-01 and PF24-TI-02 implementation repairs completed; independent review and the combined-tree/PF-27 gates remain downstream.
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-2/edit-intent.md`; plan update intent: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-2/plan-edit-intent.md`.
**Tasks Completed**:
- PF24-TI-01: `TestWebhookFireEventTypesAndNegativeEvents` now registers `env.Receiver`, checks exactly one added persisted row with the matching event type and ticket/comment IDs, requires `DeliverWebhooks` to attempt exactly one delivery, verifies the HMAC signature, and checks the receiver payload `eventType`. Ping also asserts its response row. The literal `ticket_submitted` and `comment_deleted` rejection cases remain unchanged.
- PF24-TI-02: added `TestWebhookResendManualAttempt`, checking successful first delivery, same event ID on resend, identical receiver bodies, two attempts with the latest marked manual, succeeded status, valid resent signature, and 404 `resource_not_found` for an unknown event.
**Verification**:
- `go vet ./internal/admin/...` after the test write: exit 0; `/tmp/pf24-s465-attempt2-vet-after-test.log` (complete copy: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-2/vet-after-test.log`).
- `go test -count=1 -v -race -run '^TestWebhookFireEventTypesAndNegativeEvents$' ./internal/admin/...`: exit 0; 16 PASS, 0 FAIL, 0 SKIP; `/tmp/pf24-s465-attempt2-test-fire.log` (copy: `test-fire.log`).
- `go test -count=1 -v -race -run '^TestWebhookResendManualAttempt$' ./internal/admin/...`: exit 0; 1 PASS, 0 FAIL, 0 SKIP; `/tmp/pf24-s465-attempt2-test-resend.log` (copy: `test-resend.log`).
- `go test -count=1 -v -race ./internal/admin/...`: exit 0; 32 PASS, 0 FAIL, 0 SKIP, cached=0; `/tmp/pf24-s465-attempt2-test-full.log` (copy: `test-full.log`). This includes PF-16 `TestOperationsContractAndUnknownRoute`, six Fault subtests, all 13 supported fire events and both literal negative cases.
- `go build ./...`: exit 0; `/tmp/pf24-s465-attempt2-build.log` (copy: `build.log`).
- `test -z "$(gofmt -l internal/admin internal/store/admin.go)"`: exit 0; clean. `rg -l 'ticket_submitted|comment_deleted' internal/admin`: exit 0 and lists `internal/admin/webhooks_test.go`.
- Source SHA-256: `pre-edit-sha256.txt` and `post-edit-sha256.txt`; `git diff --check` and per-write vet result are recorded in attempt-2 evidence.
**Reconciliation requests**: None. Earlier attempt-1 build/vet/test evidence remains in its own attempt directory and is superseded for these changed tests by attempt-2 current-source verification.

**Final-source confirmation after fire-row identity strengthening**: The fire table now identifies the newly created delivery by ID before asserting eventType and associated IDs, preventing an older row of the requested type from masking an incorrectly enqueued event. Intent: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-2/fire-row-intent.md`; this progress update intent: `tmp/punchflow-kickflow-dummy-s465/PF-24/attempt-2/final-progress-intent.md`.
- `go vet ./internal/admin/...`: exit 0; `/tmp/pf24-s465-attempt2-vet-final.log` (copy: `vet-final.log`).
- Focused fire test: exit 0, PASS=16, FAIL=0, SKIP=0; `/tmp/pf24-s465-attempt2-test-fire-final.log` (copy: `test-fire-final.log`).
- Focused resend test: exit 0, PASS=1, FAIL=0, SKIP=0; `/tmp/pf24-s465-attempt2-test-resend-final.log` (copy: `test-resend-final.log`).
- Full uncached admin race suite: exit 0, PASS=32, FAIL=0, SKIP=0, cached=0; `/tmp/pf24-s465-attempt2-test-full-final.log` (copy: `test-full-final.log`).
- `go build ./...`: exit 0; `/tmp/pf24-s465-attempt2-build-final.log` (copy: `build-final.log`).
- Gofmt check is clean; literal negative-event search lists `internal/admin/webhooks_test.go`. Current source hashes are in `post-edit-sha256.txt`.
