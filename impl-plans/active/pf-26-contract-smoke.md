# PF-26 Contract Test and Serve Smoke Test Implementation Plan

**Status**: Ready
**Wave**: 5 | **Depends On**: PF-17, PF-18, PF-19, PF-20, PF-21, PF-22, PF-23, PF-24 | **Plan ID**: PF-26
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

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
