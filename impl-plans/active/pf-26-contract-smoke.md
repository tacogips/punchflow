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

- Do not fix handler bugs here. Failures found here are logged as reconciliation requests naming the owning plan (by coverage-matrix handler file) and fixed in PF-27. You may fix only test code.

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

## Tests and Verification

```bash
go build ./... 2>&1 | tee /tmp/pf26-build.log
go vet ./internal/contract/... ./internal/cli/... 2>&1 | tee /tmp/pf26-vet.log
go test -race ./internal/contract/... 2>&1 | tee /tmp/pf26-contract.log     # 96 routes, 96 scenarios, 13 webhook payloads
go test -race -run TestServeSmoke ./internal/cli/... 2>&1 | tee /tmp/pf26-smoke.log
gofmt -l internal/contract internal/cli/serve_smoke_test.go   # prints nothing
```

## Done Criteria

- [ ] Contract tests cover 96/96 routes and 96/96 scenarios and 13/13 webhook payloads; smoke test passes; any non-test failures are logged as reconciliation requests with the owning plan id

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
