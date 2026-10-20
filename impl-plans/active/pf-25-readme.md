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

## Verification (session 460, superseded by the Continuation section below)

```bash
for s in pf_pat_dev_admin pf_pat_dev_manager pf_pat_dev_finance pf_pat_dev_employee pf_sat_dev_integration X-Kickflow-Signature X-Caller-Id keycloak:up --enable-local-login --enable-admin-reset --oidc-issuer /admin/api/webhooks; do grep -q -- "$s" README.md && echo "ok $s" || echo "MISSING $s"; done 2>&1 | tee /tmp/pf25-grep.log   # no MISSING lines
grep -c 'pf_pat_dev_admin' fixtures/default/tokens.json   # >= 1 (README matches fixtures)
```

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 4. **Depends on**: PF-07, PF-16. **State at 410d3d8**: `README.md` does not exist. Write it as specified. Session-462 points:

- The personas table lists each role's permissions using the kickflow permission names exactly as in `fixtures/default/roles.json` after PF-07: Super Admin = all 19, written out by name; Ticket Admin = `ticket_read`, `ticket_write`; Master Editor = `master`, `master_read`; Integration Admin = `integration`.
- The webhooks section lists the 13 event types exactly as in `design-docs/specs/design-api-coverage.md` (webhook event matrix). Delivery status values are `pending`, `succeeded`, `failed`.
- The "Development" section uses the uncached test command `go test -count=1 -v ./...` and `cd web && bun test`.
- Run the verification as:

```bash
bash -o pipefail -c 'for s in pf_pat_dev_admin pf_pat_dev_manager pf_pat_dev_finance pf_pat_dev_employee pf_sat_dev_integration X-Kickflow-Signature X-Caller-Id keycloak:up --enable-local-login --enable-admin-reset --oidc-issuer /admin/api/webhooks ticket_opened comment_updated workflow_misc; do grep -q -- "$s" README.md && echo "ok $s" || echo "MISSING $s"; done 2>&1 | tee /tmp/pf25-s462-grep.log'; echo "exit=$?"
grep -c MISSING /tmp/pf25-s462-grep.log   # 0
rg -n -e 'ticket_submitted|comment_deleted|ticket_admin|audit_read' -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' README.md   # prints nothing (exit 1)
```

## Done Criteria

- [ ] README contains all 9 sections; grep check has no MISSING lines; no banned enum value appears in README (session 462)

## Continuation (session 463) - read first; overrides the session-462 phase, log name and Done Criteria

**Wave**: B (s463). **Depends on**: PF-07 (accepted at f82e374) and PF-16 (wave A; CLI flags and admin routes are fixed after wave A).
**State at f82e374**: `README.md` does not exist.
**Path Declarations**: unchanged. 2 declared files (< 300).

1. Write `README.md` as specified, with the session-462 points.
2. Development section: give the uncached evidence commands (`go test -count=1 -v ./...`; `cd web && bun install --frozen-lockfile && bun run build && bun test`). Describe `mise run test` as the convenience task; PF-27 makes it `go test -count=1 ./...`. Do not call it cached.
3. Take flag names from `design-docs/specs/command.md` and the PF-16 code (`internal/cli/config.go`); do not invent them. Take persona tokens and e-mails from `fixtures/default/tokens.json`, `users.json` and `deploy/keycloak/realm-punchflow.json`.
4. Verification: the session-462 commands with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy the log to `tmp/punchflow-kickflow-dummy-s463/PF-25/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] All 9 sections present; `grep -c MISSING /tmp/pf25-s463-grep.log` prints 0; the banned-enum rg over README.md prints nothing
- [ ] The progress log has exit status and log path for every command

## Continuation (session 464) - read first; overrides the session-462/463 sections

**Wave**: B. It starts only after PF-14, PF-15 and PF-16 are accepted. **Depends on**: PF-07 (accepted at `44e40cc`) and PF-16.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json`. Paths are unchanged: `README.md` and this plan file. Logs are `/tmp/pf25-s464-*.log`, copied to `tmp/punchflow-kickflow-dummy-s464/PF-25/attempt-<n>/`.

**Intent**: unchanged. Write `README.md` as specified, including the session-463 points: uncached evidence commands, flag names taken from `command.md` and `internal/cli/config.go`, persona tokens and e-mails from the fixtures and the realm.

**Session-464 notes**:
- Read persona tokens and e-mails from the canonical `fixtures/default/tokens.json` and `users.json`, as rewritten by PF-16 in wave A. Values are unchanged; only order and dropped zero-value keys may differ.
- Never edit any file other than `README.md` and this plan file.

### Done Criteria (session 464)

- [ ] All 9 sections present; `grep -c MISSING /tmp/pf25-s464-grep.log` prints 0; the banned-enum rg over README.md prints nothing
- [ ] The progress log has exit status and log path for every command

## Continuation (session 465) - read first; overrides the session-462/463/464 sections

**Wave**: A (with PF-14a, PF-14b, PF-14c, PF-15, PF-16). **Depends on**: PF-07 only (accepted at `ea385c7`). PF-16 is no longer a dependency (design `architecture.md` "Continuation order (session 465)").
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. Paths are unchanged (`README.md` and this plan file). Logs are `/tmp/pf25-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-<n>/`.

**Intent**: unchanged. Write `README.md` with the 9 required sections and the session-462/463 points. `README.md` does not exist at `ea385c7`.

**Sources of truth in s465**:
- CLI flags, environment variables, subcommands, defaults and exit codes: `design-docs/specs/command.md` only. `internal/cli/config.go` (PF-16) exists at `ea385c7` and may be read as a cross-check. If it disagrees with `command.md`, write what `command.md` says and record the mismatch as a reconciliation request to PF-16 (`file:line`). Never edit `internal/cli/*`.
- Persona tokens and e-mails: `fixtures/default/tokens.json`, `users.json` and `deploy/keycloak/realm-punchflow.json` (ids and values are stable even if PF-16 rewrites the fixture files in this wave).
- Admin API paths: `design-docs/specs/design-auth-admin.md` "Dev admin API".
- Webhook events, headers and retry schedule: `design-docs/specs/design-api-coverage.md` webhook event matrix and `design-docs/specs/design-webhooks.md`.

**Verification change**: the manifest commands are the s464 ones with `-s465-` log names. "No MISSING lines" and the banned-enum check are now must-pass (`grep -c MISSING <log>; test $? -eq 1` and `rg ... README.md; test $? -eq 1`), and each must show `exit=0`.

### Done Criteria (session 465)

- [ ] All 9 sections present; the MISSING check and the banned-enum check exit 0; every flag in README appears in `command.md`
- [ ] The progress log has exit status and log path for every command; any README/CLI mismatch is recorded as a reconciliation request to PF-16

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code or documents, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
