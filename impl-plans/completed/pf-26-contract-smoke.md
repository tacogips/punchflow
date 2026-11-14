# PF-26 Contract Test and Serve Smoke Test Implementation Plan

**Status**: Completed (superseded by PF-26a..PF-26f)
**Wave**: 5 | **Depends On**: PF-17, PF-18, PF-19, PF-20, PF-21, PF-22, PF-23, PF-24 | **Plan ID**: PF-26

> **Session 466 (read first).** This file is now a superseded index, like `pf-14-engine.md`. Its delivered work is at `a2a82de` and stays green: `TestContractRoutes`, `TestContractScenarios`, `TestContractLiveReadScenarios` (22 operations), `TestContractWebhookSchemas`, `TestContractWebhookPayloads` (13 events, signatures) and `TestServeSmoke`. Those files are frozen for the s466 wave. The remaining 74 live scenarios are split as follows (design: `design-api-coverage.md` "Live scenario partition (session 466)"):
> - `pf-26a-contract-tickets.md` (13 operations)
> - `pf-26b-contract-comments-files.md` (7)
> - `pf-26c-contract-users-grades.md` (13; also owns the `liveReadScenarios` hoist in `scenarios_users_org_test.go`)
> - `pf-26d-contract-orgcharts-teams.md` (14)
> - `pf-26e-contract-masters.md` (14)
> - `pf-26f-contract-proxies-roles.md` (13)
>
> The planned files `scenarios_definitions_test.go` and `scenarios_misc_test.go` are dropped; they were never created. PF-27 adds the aggregate `TestContractOperationCoverage` and archives this file. The older continuation sections below are history.
**Design Reference**: design-docs/specs/design-api-coverage.md#contract-test; design-docs/specs/architecture.md#testing-and-verification (smoke row)
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Prove kickflow compatibility mechanically. (1) The route table equals the 96 path operations of `design-docs/references/kickflow-openapi.yaml`. (2) A representative 200 response of every operation validates against its response schema (JSON Schema 2020-12). (3) Every timestamp is JST with milliseconds. (4) Webhook bodies for all 13 events validate against the spec's webhook schemas. Plus a smoke test that runs the real `serve` command end to end.

## Non-goals

- Do not fix handler bugs here. Log each failure as a reconciliation request naming the owning plan (by coverage-matrix handler file; ownership index in `impl-plans/README.md` rule 12). Integration review routes the repair to that plan (session 463; PF-27 no longer repairs other plans' files). You may fix only your own test code.

## Files

`internal/contract/spec_test.go` (load YAML -> JSON, schema compiler), `routes_test.go`, `scenarios_users_org_test.go`, `scenarios_definitions_test.go`, `scenarios_tickets_test.go`, `scenarios_misc_test.go`, `webhooks_test.go`, `helpers_test.go` (all `package contract_test`); `internal/cli/serve_smoke_test.go` (new).

## Path Declarations

### writePaths
- `impl-plans/active/pf-26-contract-smoke.md`
- `internal/contract/spec_test.go`
- `internal/contract/routes_test.go`
- `internal/contract/scenarios_users_org_test.go`
- `internal/contract/scenarios_definitions_test.go`
- `internal/contract/scenarios_tickets_test.go`
- `internal/contract/scenarios_misc_test.go`
- `internal/contract/webhooks_test.go`
- `internal/contract/helpers_test.go`
- `internal/cli/serve_smoke_test.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contract Test Details

- Locate the spec via `runtime.Caller` -> module root -> `design-docs/references/kickflow-openapi.yaml`. Parse with `gopkg.in/yaml.v3` into `map[string]any`, normalizing keys to strings, then add it as a resource to `jsonschema.NewCompiler()` (santhosh-tekuri v6) under the URL `file:///kickflow-openapi.json`. Compile `file:///kickflow-openapi.json#/paths/<escaped path>/<method>/responses/200/content/application~1json/schema` (JSON-pointer escaping: `~1` for `/`, `~0` for `~`). Enable format assertions (`compiler.AssertFormat()`).
- Route test: the set of (METHOD, path) under `paths` (exactly 96; fail with the count otherwise) equals `apiv1.Operations()` (Pattern compared after replacing `{x}` placeholders, which are identical) and the operationIds match.
- Scenario table: one entry per operationId (96, asserted by count and by name set equal to `Operations()`): persona, method, path builder (using ids from the default fixtures or created in setup), body, and expectations. Each must return 200; non-empty responses validate against the compiled schema; empty-body operations return 0 bytes. Mutating scenarios run in dependency order inside one `testutil.Env` (e.g. create ticket -> approve -> ...), or use separate envs where simpler.
- Timestamp walker: recursively visit decoded JSON; for any string value under a key ending in `At` that is non-empty, assert `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\+09:00$`.
- Webhooks: register a receiver webhook, trigger all 13 events (ping via webhook create; ticket events via the lifecycle; comment events via comments), `DeliverWebhooks`, validate each body against `#/webhooks/<ping|ticket|comment>/post/requestBody/content/application~1json/schema` and verify the signature.

## Smoke Test Details (`internal/cli/serve_smoke_test.go`)

Run `cli.Run(ctx, []string{"serve", "--listen", "127.0.0.1:0", "--db", tmp + "/pf.sqlite", "--data-dir", tmp}, stdout, stderrPipe)` in a goroutine. Read stderr until `listening on http://<addr>` (10 s timeout). Then `GET /healthz` 200; `GET /v1/user` with `pf_pat_dev_admin` -> 200 and `code` = `admin`; `POST /v1/tickets` as `pf_pat_dev_employee` (expense-claim, amount 5000, team sales, in_progress) -> 200 `status` in_progress; `GET /` -> 200 HTML. Cancel ctx -> `Run` returns 0 within 15 s. The goroutine is owned by the test and always joined.

## Key Points and Pitfalls

- OpenAPI 3.1 keywords such as `discriminator`, `example`, `xml` are ignored by the validator; that is fine. Do not pre-process the spec beyond YAML -> JSON key normalization (YAML may give `map[string]any` with non-string keys for numeric response codes like `200`; convert them to strings).
- Some schemas use `format: uuid` on ids: punchflow ids are lowercase UUID v4, which passes.
- Keep each test file < 1000 lines (hence the scenario split).

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 5 (alone). **Depends on**: PF-17..PF-24. **State at 410d3d8**: not started (`internal/contract` does not exist). Implement this plan as written. Session-462 points:

- The 13 webhook payloads are the events in `model.WebhookEventTypes`. The webhook test asserts the set of received `eventType` values equals that list exactly.
- Schema validation is the second line of defense for the enum contract: a response with `userType: member`, `sectionType: conditional`, `gradeSymbol: ""`, route `status: active` or an invented permission fails schema validation. Report such a failure as a reconciliation request against the owning plan (PF-07 data, PF-08/PF-15 presenter). Do not loosen the validator.
- Test names: `TestContract*` in `internal/contract`; the smoke test is `TestServeSmoke` in `internal/cli/serve_smoke_test.go`.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf26-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/contract/... ./internal/cli/... 2>&1 | tee /tmp/pf26-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/contract/... 2>&1 | tee /tmp/pf26-s462-contract.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'TestServeSmoke' ./internal/cli/... 2>&1 | tee /tmp/pf26-s462-smoke.log"; echo "exit=$?"
for f in /tmp/pf26-s462-contract.log /tmp/pf26-s462-smoke.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/contract internal/cli/serve_smoke_test.go   # prints nothing
```

Evidence: exit 0; FAIL=0; SKIP=0. The contract log shows the 96-route test, 96 scenario subtests and 13 webhook subtests passing. Count them with `grep -c -- '--- PASS: TestContract.*/' /tmp/pf26-s462-contract.log`, which must be >= 109. The smoke log shows `--- PASS: TestServeSmoke`.

### Done Criteria (session 462)

- [ ] Contract tests cover 96/96 routes, 96/96 scenarios and 13/13 webhook payloads (exact event set); smoke test passes; counts recorded
- [ ] Any non-test failure is logged as a reconciliation request with the owning plan id

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: C (s463; alone). **Depends on**: PF-17 (accepted at f82e374) and PF-18..PF-24 (wave B).
**State at f82e374**: not started. `internal/contract` and `internal/cli/serve_smoke_test.go` do not exist. The contract-test dependencies `gopkg.in/yaml.v3` and `github.com/santhosh-tekuri/jsonschema/v6` are already in `go.mod`, so no `go.mod` edit is needed. If one were, it would be a PF-02 repair route (README rule 5).
**Path Declarations**: unchanged. 10 declared files (< 300).

1. Implement the whole plan above, with the session-462 points.
2. Failure handling:
   - A schema, route or scenario failure caused by a handler, presenter or fixture becomes a reconciliation request: `file:line` or the operationId, the owning plan from README rule 12, the failing subtest and the log path.
   - The plan is then blocked on that repair. Re-run the full verification after the owner's repair lands.
   - Never loosen the validator, skip a scenario or special-case an operation.
3. The smoke test owns and joins its goroutine (`cli.Run` with a cancelable ctx). It must not leave a listening server or any background process behind.
4. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array), plus `grep -c -- '--- PASS: TestContract.*/' /tmp/pf26-s463-contract.log`, which must be >= 109. Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-26/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, SKIP=0, >= 109 `TestContract` subtests PASS, and `--- PASS: TestServeSmoke`
- [ ] Every non-test failure seen during the work is listed under "Reconciliation requests" with its owner and its resolution

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: C (alone). It starts after PF-18..PF-24 are accepted. **Depends on**: PF-17 (accepted at `44e40cc`) and PF-18..PF-24.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json`. Paths are unchanged (10 files). Logs are `/tmp/pf26-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-26/attempt-<n>/`.

**Intent**: unchanged. The contract test covers:
- 96/96 routes
- 96/96 schema-validated scenarios
- the exact 13 webhook payloads
- `TestServeSmoke`

**Session-464 changes**:
1. Repair routing uses the s464 ownership index: `ownershipTransfers[]` first. For example:
   - a `/v1` handler shape bug goes to the PF-18..PF-23 plan that now owns that handler file, not PF-10
   - a fixture data bug goes to PF-16
   - a payload bug goes to PF-15
2. Compile rule (README rule 13): after each `_test.go` write, run `go vet ./internal/contract/...` (or `./internal/cli/...` for the smoke test) and log it.
3. Failure handling as in session 463: never loosen the validator, skip a scenario or special-case an operation.

### Done Criteria (session 464)

- [ ] Every manifest verification line passes with exit status, log path and counts: exit 0, FAIL=0, SKIP=0, >= 109 `TestContract` subtests PASS, and `--- PASS: TestServeSmoke`
- [ ] Every non-test failure seen during the work is listed under "Reconciliation requests" with its s464 owner and its resolution

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: C. **Depends on**: PF-17 (accepted at `ea385c7`), PF-18, PF-19, PF-20, PF-21, PF-22, PF-23, PF-24 (unchanged; the engine plans PF-14a/b/c are reached through PF-22..PF-24).
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged from s464 (10 files). The manifest `verification` array is authoritative: the s464 commands with `-s465-` log names, and the gofmt check in must-pass form.

**Intent and tasks**: unchanged from the session-464 section: 96/96 routes, 96/96 schema-validated scenarios, 13/13 webhook payloads, and `TestServeSmoke`. Logs are `/tmp/pf26-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-<n>/`.

**Repair routing in s465**: a schema or behavior failure in a non-test file is a reconciliation request to the owner from the s465 `ownershipIndex`. Engine files go to PF-14a/PF-14b/PF-14c by file, as listed in `design-approval-engine.md` "Plan ownership (session 465)". Never edit them.

### Done Criteria (session 465)

- [ ] Every manifest verification line passes with exit status, log path and counts: build/vet exit 0; contract and smoke tests exit 0, FAIL=0, SKIP=0; >= 109 `TestContract` subtests PASS; `--- PASS: TestServeSmoke`; gofmt check exit 0
- [ ] Every non-test failure seen during the work is listed under "Reconciliation requests" with its s465 owner and its resolution

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -

### Session: 2026-10-21 (session 465, attempt 1)
**Intent snapshot**: Implement the OpenAPI contract inventory/schema compiler and end-to-end `serve` smoke test within PF-26's declared paths.
**Tasks Completed**:
- Added OpenAPI YAML loading, route/operationId comparison for all 96 operations, and response-schema compilation subtests for every operation.
- Added webhook-schema compilation subtests for the 13 canonical event names.
- Added recursive JST timestamp validation helper for scenario response assertions.
- Added `TestServeSmoke`, which starts and joins `cli.Run`, then checks `/healthz`, authenticated `/v1/user`, and `/`.
**Still Incomplete**:
- The 96 scenario subtests currently compile response schemas but do not execute representative HTTP requests or validate actual responses against those schemas; the timestamp helper is not yet applied to response bodies.
- The 13 webhook subtests compile shared schemas but do not emit events, validate delivered payloads, assert the exact received event set, or verify signatures.
- The smoke test does not yet exercise seeded employee ticket creation.
**Verification**:
- `go build ./...`: final exit 0, `/tmp/pf26-s465-build-final.log`; copied to `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/build-final.log`.
- `go vet ./internal/contract/... ./internal/cli/...`: final exit 0, `/tmp/pf26-s465-vet-final.log`; copied to `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/vet-final.log`.
- Contract test first attempt failed on the spec path; second failed on lowercase method matching and incorrect JSON Schema resource registration. These superseded logs are copied under `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/`.
- Final contract schema/inventory suite: exit 0, PASS=208, FAIL=0, SKIP=0; 205 `TestContract` subtests PASS. Log: `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/contract-final.log`.
- Final `TestServeSmoke`: exit 0, PASS=1, FAIL=0, SKIP=0. Log: `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/smoke-final.log`.
- `test -z "$(gofmt -l internal/contract internal/cli/serve_smoke_test.go)"`: exit 0; files are gofmt clean.
**Reconciliation requests**: None observed. The remaining gap is PF-26's own test implementation, not a production handler failure.

### Session: 2026-10-21 (session 465, continuation 1)
**Intent snapshot**: Continue from the preserved PF-26 tree; add live response validation for seeded read operations, actual webhook delivery/schema/signature checks, and employee ticket creation in the serve smoke test.
**Tasks Completed**:
- Added live schema-validated GET scenarios for 22 seeded operations, including status, response schema and timestamp assertions; list responses must include Page, Per-Page and Total headers.
- Added an end-to-end webhook contract test that fires, delivers and signature-verifies all 13 event types and checks their exact received eventType set.
- Extended `TestServeSmoke` to POST an in-progress expense-claim ticket using the seeded employee token and assert its returned status.
- Corrected the timestamp walker so boolean `initialDisplayCreatedAt` fields are not treated as timestamps.
**Reconciliation requests** (unresolved; integration review must route repairs to the owners, then PF-26 reruns its full contract suite):
- PF-07 owns fixture values in `fixtures/default/workflows.json`; PF-08 owns workflow presentation in `internal/present/workflow_form.go` and `workflow.go`. `GET /v1/workflows/{workflowId}` fails schema validation at `sectionList[].formFields[].size` (`medium`, schema allows `full|half`) and `routeConditions[].conditionFields[].value` (number, schema allows null/string); `formField` is null where the schema rejects null. Route a joint fixture/presenter disposition to PF-07/PF-08. Evidence: `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/contract-cont1-final.log`, `TestContractLiveReadScenarios/getWorkflow`.
- PF-15 owns ticket/comment presentation (`internal/present/ticket.go`, `ticket_detail.go`, `comment.go`): 12 non-ping events are delivered and signature-valid, but payload validation fails at `/data/ticket/forcedPublicType` (emitted value outside `follow_workflow|forced_public|forced_private`) and repeated `/data/ticket/.../formField/size` values (`medium`, schema allows `full|half`). Evidence: same log, `TestContractWebhookPayloads` subtests for ticket and comment events. The ping payload passes its schema.
- Resolution for both requests: owning plans must repair the fixture/presenter output within their declared paths, after which PF-26 reruns contract tests. PF-26 did not edit these foreign files.
**Still Incomplete**:
- Representative live response scenarios remain absent for 74 operations; only 22 seeded read operations currently execute through HTTP. The test suite also does not yet cover mutation request scenarios across all operations.
- Webhook delivery, event-set and signature assertions run, but 12 payload schemas remain failing pending PF-15 repair.
- `/v1/workflows/{workflowId}` response schema remains failing pending PF-07 fixture and/or PF-08 presenter disposition.
**Verification**:
- Final scoped contract command `go test -count=1 -v -race ./internal/contract/...`: exit 1, PASS=230, FAIL=15, SKIP=0; 227 `TestContract` subtests PASS. Failures are the workflow schema mismatch and 12 webhook payload schema mismatches plus parent test failures. Complete log: `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/contract-cont1-final.log`.
- Corrected `TestServeSmoke`: exit 0, PASS=1, FAIL=0, SKIP=0; log `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/smoke-teamfix.log`.
- Post-edit `go vet ./internal/contract/...` and `go vet ./internal/cli/...`: exit 0 (commands completed before the contract-cont1 final run).
- Final `go build ./...`: exit 0, `/tmp/pf26-s465-build-cont1-final.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/build-cont1-final.log`.
- Final `go vet ./internal/contract/... ./internal/cli/...`: exit 0, `/tmp/pf26-s465-vet-cont1-final.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-26/attempt-1/vet-cont1-final.log`.
- `test -z "$(gofmt -l internal/contract internal/cli/serve_smoke_test.go)"` and `git diff --check -- internal/contract internal/cli/serve_smoke_test.go impl-plans/active/pf-26-contract-smoke.md`: both exit 0.
**Review decision**: No formal test-integrity, adversarial or integration review has run. Current contract failures are owner-routed repair requests, not accepted results.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
