# PF-25 README Implementation Plan

**Status**: Ready
**Wave**: 4 | **Depends On**: PF-07, PF-16 | **Plan ID**: PF-25
**Design Reference**: design-docs/specs/architecture.md#developer-tooling (README contents); design-docs/specs/command.md; design-docs/specs/design-auth-admin.md; design-docs/specs/design-webhooks.md; design-docs/specs/notes.md
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Write the user-facing `README.md`: how to run punchflow locally and how to point an existing kickflow client (REST and webhook receiver) at it. Acceptance criterion: "README or docs explain how to configure an existing kickflow client (base URL, token, webhook secret) to use punchflow".

## Non-goals

- No design-doc edits; no code. Do not duplicate the coverage matrix: link to it.

## Files

`README.md` (new).

## Path Declarations

### writePaths
- `impl-plans/active/pf-25-readme.md`
- `README.md`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Required Sections

1. What punchflow is: a local/dev-only kickflow dummy; never expose it publicly; AI features out of scope.
2. Quick start: `mise install`, `mise run build`, `./punchflow serve` (or `mise run dev`), default `http://localhost:8080`; `punchflow serve --db /tmp/pf.sqlite` seeds automatically.
3. Seeded personas table: user code, e-mail, role/permissions, dev token (`pf_pat_dev_admin`, `pf_pat_dev_manager`, `pf_pat_dev_finance`, `pf_pat_dev_employee`, `pf_sat_dev_integration`), plus the warning that these are dev-only values.
4. Pointing a kickflow REST client at punchflow: base URL `http://localhost:8080` (paths stay `/v1/...`); PAT `Authorization: Bearer ...`; SAT plus `X-Caller-Id: <user UUID>`; curl examples (`GET /v1/user`, `GET /v1/tickets`, `POST /v1/tickets` for `expense-claim` with `formFieldCode` inputs, `POST /v1/tickets/{id}/approve`); pagination headers; error body format; JST timestamps.
5. Webhooks: register via UI (Admin > Webhooks) or `POST /admin/api/webhooks` with `url` and `secret`; headers (`X-Kickflow-Delivery`, `X-Kickflow-Signature: sha256=<hex>`, `User-Agent: kickflow-Hookshot/v1`); verifying the signature over the raw body; retry schedule (INF-08); resend and fire-event endpoints; delivery log.
6. Keycloak SSO (manual check): `mise run keycloak:up`, `punchflow serve --oidc-issuer http://localhost:8180/realms/punchflow`, log in at `http://localhost:8080` as `sales-rep@punchflow.test` / `punchflow`, expect the dashboard; `mise run keycloak:down`.
7. Dev-only switches: `--enable-local-login`, `--enable-admin-reset`, rate limiting (`--rate-limit`, `--rate-limit-secret`), tenant modes and fault injection for error testing.
8. CLI reference summary (link to `design-docs/specs/command.md`), fixtures (`fixtures/default`, `punchflow fixtures export|reset`), and the inferred-behavior notice linking to `design-docs/specs/notes.md` and `design-docs/specs/design-api-coverage.md`.
9. Development: `mise run test`, `mise run lint`, `cd web && bun install && bun run build && bun test` (bun only).

## Key Points and Pitfalls

- Every flag, env var, path and token value must match `command.md`, PF-07 and PF-16 exactly; check with grep (verification below).
- Use plain ASCII; no emojis.

## Verification

```bash
for s in pf_pat_dev_admin pf_pat_dev_manager pf_pat_dev_finance pf_pat_dev_employee pf_sat_dev_integration X-Kickflow-Signature X-Caller-Id keycloak:up --enable-local-login --enable-admin-reset --oidc-issuer /admin/api/webhooks; do grep -q -- "$s" README.md && echo "ok $s" || echo "MISSING $s"; done 2>&1 | tee /tmp/pf25-grep.log   # no MISSING lines
grep -c 'pf_pat_dev_admin' fixtures/default/tokens.json   # >= 1 (README matches fixtures)
```

## Done Criteria

- [ ] README contains all 9 sections; grep check has no MISSING lines

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
