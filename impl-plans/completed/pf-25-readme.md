# PF-25 README Implementation Plan

**Status**: Completed
**Wave**: 4 | **Depends On**: PF-07, PF-16 | **Plan ID**: PF-25
**Design Reference**: design-docs/specs/architecture.md#developer-tooling (README contents); design-docs/specs/command.md; design-docs/specs/design-auth-admin.md; design-docs/specs/design-webhooks.md; design-docs/specs/notes.md
**Created**: 2026-10-20
**Last Updated**: 2026-10-21

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

- [x] All 9 sections present; the MISSING check and the banned-enum check exit 0; every flag in README appears in `command.md`
- [x] The progress log has exit status and log path for every command; any README/CLI mismatch is recorded as a reconciliation request to PF-16

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code or documents, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15).

## Progress Log

### Session: 2026-10-21 (s465 attempt 1)
**Intent snapshot**: Create `README.md` with the nine required sections, sourced from `command.md`, fixture files and accepted design documents. Update only this plan's Progress Log and the session-465 completion checkboxes.
**Tasks Completed**:
- Added the local/dev usage guide, seeded persona/token table, kickflow REST client examples, webhook setup/signature/retry guidance, Keycloak manual check, dev switches, CLI/fixture references and development commands in `README.md`.
- Documented all 13 webhook event types and `pending`, `succeeded`, `failed` statuses. Corrected the expense example to use the `expense-claim` workflow UUID and string-valued `formFieldCode` inputs from the OpenAPI contract.
- Cross-checked CLI subcommands, flags, environment-variable conventions and defaults against `internal/cli/config.go` and subcommand files. No mismatch with `design-docs/specs/command.md` found; no PF-16 reconciliation request.
- Updated both session-465 Done Criteria.
**Verification**:
- `bash -o pipefail -c 'for s in ...; do grep ...; done 2>&1 | tee /tmp/pf25-s465-grep.log'; echo "exit=$?"` — exit 0; all 15 markers found; log `/tmp/pf25-s465-grep.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/grep.log`.
- `bash -o pipefail -c 'grep -c MISSING /tmp/pf25-s465-grep.log; test $? -eq 1'` — exit 0 (count 0); log `/tmp/pf25-s465-no-missing.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/no-missing.log`.
- `bash -o pipefail -c 'rg -n ... README.md; test $? -eq 1'` — exit 0 (no banned values matched); log `/tmp/pf25-s465-banned.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/banned.log`.
- CLI flag reference check against `command.md` — exit 0 (all listed CLI flags documented there); log `/tmp/pf25-s465-flags-wrapper.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/flags.log`.
- README CLI flag extraction against `command.md` — exit 0 (all nine flags used in README are defined); log `/tmp/pf25-s465-readme-flags.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/readme-flags.log`.
- README criteria assertion (sections, 13 events, 3 statuses, uncached development commands) — exit 0; log `/tmp/pf25-s465-content.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-1/content.log`.
**Reconciliation requests**: None.

**README SHA-256**: `6f6514418e705414b5d1d88c43ffe0a5bdfc15b9d7d3ff4f74ef59238cc17d80`.

### Session: 2026-10-21 (s465 attempt 2, final-source check)
**Intent snapshot**: Add an explicit `What punchflow is` heading for required section 1; preserve all previously written README content.
**Tasks Completed**: Added the heading. The README now has all nine required section headings.
**Verification**:
- Marker loop — exit 0; all 15 markers found; `/tmp/pf25-s465-final-markers.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-2/markers.log`.
- `grep -c MISSING /tmp/pf25-s465-final-markers.log; test $? -eq 1` — exit 0 (count 0); `/tmp/pf25-s465-final-no-missing.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-2/no-missing.log`.
- Banned enum `rg` with expected no-match exit converted to pass — exit 0; `/tmp/pf25-s465-final-banned.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-2/banned.log`.
- Final README assertion — exit 0; nine required section headings, 13 events, three delivery statuses, both uncached development commands and all nine README flags matched; `/tmp/pf25-s465-final-content.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-2/content.log`.
**Reconciliation requests**: None.
**Final README SHA-256**: `1803f9ae4d2442fa01c9abfea09f5e097e451ebecf5ac46cc8c71a342a930ae7`.

### Session: 2026-10-21 (s465 attempt 3, behavioral verification)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/edit-intent.md`. Progress-only update; README content is unchanged.
**Tasks Completed**: Set plan status to Completed after current-source assertions and positive-count scoped CLI behavioral tests passed. No mismatch with `command.md`; no PF-16 reconciliation request.
**Verification**:
- Marker loop — exit 0; all 15 markers found; complete log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/pf25-s465-attempt3-markers.log`.
- `grep -c MISSING ...; test $? -eq 1` — exit 0; zero missing markers; complete log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/pf25-s465-attempt3-no-missing.log`.
- Banned enum `rg` with expected no-match converted to pass — exit 0; no banned values; complete log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/pf25-s465-attempt3-banned.log`.
- README contract assertion against `command.md` — exit 0; 9 sections, 13 events, 3 statuses, uncached development commands and 9 flags found; complete log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/pf25-s465-attempt3-content.log`.
- `go test -count=1 -v ./internal/cli/...` — exit 0; PASS=6, FAIL=0, SKIP=0; complete log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-3/cli-test.log`.
**Reconciliation requests**: None.

### Session: 2026-10-21 (s465 attempt 4, verification-gap closure)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/edit-intent.md`. Progress-only update; `README.md` is unchanged (SHA-256 remains `1803f9ae4d2442fa01c9abfea09f5e097e451ebecf5ac46cc8c71a342a930ae7`).
**Tasks Completed**: Re-ran required README contract checks and uncached CLI behavioral tests on the current shared tree. Kept status Completed. No `command.md` mismatch; no PF-16 reconciliation request. Independent review remains downstream.
**Verification**:
- Marker loop — exit 0; all 15 markers found; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/markers.log`.
- `grep -c MISSING ...; test $? -eq 1` — exit 0; zero missing markers; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/no-missing.log`.
- Banned-enum `rg` wrapper — exit 0; expected no-match exit was converted to pass; log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/banned-enums.log`.
- README contract assertion against `command.md` — exit 0; 9 sections, 13 events, 3 delivery statuses, uncached development commands and all 9 punchflow CLI flags matched; log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/content-corrected.log`. An initial exploratory scan also picked up Bun's `--frozen-lockfile`; this is not a punchflow CLI flag and was excluded from the corrected assertion.
- `go test -count=1 -v ./internal/cli/...` — exit 0; PASS=6, FAIL=0, SKIP=0; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-4/cli-test.log`.
**Reconciliation requests**: None.

### Session: 2026-10-21 (s465 attempt 5, PF25-ADV-01)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/edit-intent.md`. Correct only the create-ticket walkthrough and document the create response ID source; keep the nine headings and existing README contracts.
**Tasks Completed**: Added the required `status` and seeded `authorTeamId` to the create body. Documented the allowed statuses, the applicant team requirement (sales for sales-rep), and that `TICKET_ID` comes from the create response `id`. No live API execution is claimed because PF-22 still owns the ticket handlers.
**Verification**:
- Required marker loop — exit 0; 15/15 markers found; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/markers.log`.
- `grep -c MISSING ...; test $? -eq 1` — exit 0; zero missing markers; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/no-missing.log`.
- Banned-enum `rg` wrapper — exit 0; no banned enums matched; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/banned-enums.log`.
- Static create-body and fixture check — exit 0; required fields, allowed status, workflow/team IDs, and documented `TICKET_ID` source passed; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/create-body.log`.
- `grep -c '^## ' README.md` assertion — exit 0; exactly 9 headings; complete log `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/headings.log`.
**Reconciliation requests**: None.
**README SHA-256**: `c95c7f47a187008c8f95644aa9c86bd23cd12887605daa2c09a0c4fa7aef0a14`.

### Session: 2026-10-21 (s465 attempt 6, behavioral evidence for PF25-ADV-01)
**Intent snapshot**: `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/edit-intent.md`. Preserve `README.md` unchanged; rerun current-source contract checks and positive-count uncached CLI behavioral tests, then record complete evidence paths.
**Tasks Completed**: Re-verified the corrected create walkthrough against fixture IDs and reran the uncached CLI behavioral suite. `README.md` remains at SHA-256 `c95c7f47a187008c8f95644aa9c86bd23cd12887605daa2c09a0c4fa7aef0a14`. The live ticket-create walkthrough is not claimed as executed; PF-22 owns ticket handlers and their integration coverage.
**Verification**:
- `go test -count=1 -v ./internal/cli/...` — exit 0; PASS=6, FAIL=0, SKIP=0; `/tmp/pf25-s465-attempt6-cli-test-rerun.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/cli-test-rerun.log`.
- Required marker loop — exit 0; 15/15 markers found; `/tmp/pf25-s465-attempt6-markers.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/markers.log`.
- `bash -o pipefail -c 'grep -c MISSING tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/markers.log | tee /tmp/pf25-s465-attempt6-no-missing-rerun.log; test ${PIPESTATUS[0]} -eq 1'` — exit 0; zero missing markers; log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/no-missing-rerun.log`.
- `rg -n -e 'ticket_submitted|comment_deleted|ticket_admin|audit_read' -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' README.md` — expected no-match exit 1; wrapper converted it to exit 0; no banned values; log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/banned-enums.log`.
- `python3 -I tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-5/check-create-body.py` — exit 0; required fields, status, fixture IDs and `TICKET_ID` source pass; `/tmp/pf25-s465-attempt6-create-body.log`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/create-body.log`.
- `bash -o pipefail -c 'grep -c "^## " README.md | tee /tmp/pf25-s465-attempt6-headings.log; test "$(cat /tmp/pf25-s465-attempt6-headings.log)" -eq 9'` — exit 0; nine headings; log copied to `tmp/punchflow-kickflow-dummy-s465/PF-25/attempt-6/headings.log`.
- Two harness setup errors were recovered: the first `zsh` wrapper used read-only variable `status` after the test process passed, and an initial no-missing check raced the marker-log copy (missing file, exit 1). The correct shell rerun passed, and the no-missing check was rerun after the marker log existed. Their original outputs are preserved in the tool transcript; no implementation gate remains failed.
**Reconciliation requests**: None. No mismatch with `command.md`; no PF-16 reconciliation request.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
