# PF-27 Reconciliation and Finalization Implementation Plan

**Status**: Ready
**Wave**: D (s463; serial, last) | **Depends On**: PF-01 to PF-26 | **Plan ID**: PF-27
**Design Reference**: design-docs/specs/architecture.md, "Verification-command rule (session 462)" rules 1-7 and "Path-ownership and fanout rule (session 463)" items 1-8 plus the `repo_invariants_test.go` paragraph; design-docs/specs/notes.md, "Carried review findings" (CF-01..CF-03, "Status at session 463"); impl-plans/README.md, Shared Execution Protocol rules 1-12
**Created**: 2026-10-20
**Last Updated**: 2026-10-21 (session 463 rewrite: narrowed path ownership; the session-460/462 versions are superseded and removed)

---

## Intent and Context

PF-27 runs once, after every other plan. It proves the whole repository is green and closes the carried findings with mechanical evidence. It records inferences the workers logged, and it archives the plans.

Session 462 failed at dispatch because this plan declared whole source trees (`internal`, `cmd`, `fixtures`, `deploy`, `web/src`, `impl-plans/active`). Those expanded to 457 files against the 512 fanout limit. In session 463, PF-27 owns only the files it edits, listed below (67 source entries).

PF-27 never edits another plan's file. A failing gate caused by a foreign file becomes a reconciliation request. Integration review routes it to the owning plan; afterwards PF-27 re-runs the whole gate.

## Non-goals

- No feature work.
- No edits under `internal/` or `cmd/`, except the new root file `repo_invariants_test.go`.
- No edits to `fixtures/`, `deploy/`, `web/src/`, `web/package.json`, `web/bun.lock`, `embed.go`, `embed_test.go`, `fixtures_default_test.go` or `.gitignore`.
- No identifier renames and no repo-wide `gofmt -w`.
- Do not move or edit `impl-plans/active/punchflow-kickflow-dummy-s463-dispatch.json`. It drives the running dispatch (design D-463-13).
- No git commands that change state. The workflow commits.

## Path Declarations

### writePaths
- `impl-plans/active/pf-27-finalize.md`
- `repo_invariants_test.go`
- `impl-plans/completed/pf-01-foundation-primitives.md`
- `impl-plans/completed/pf-02-data-contract.md`
- `impl-plans/completed/pf-03-web-scaffold.md`
- `impl-plans/completed/pf-04-httpx.md`
- `impl-plans/completed/pf-05-auth-core.md`
- `impl-plans/completed/pf-06-fixtures-loader.md`
- `impl-plans/completed/pf-07-fixtures-default-keycloak.md`
- `impl-plans/completed/pf-08-present-core.md`
- `impl-plans/completed/pf-09-webhook-delivery.md`
- `impl-plans/completed/pf-10-api-contracts.md`
- `impl-plans/completed/pf-11-web-tickets.md`
- `impl-plans/completed/pf-12-web-readviews.md`
- `impl-plans/completed/pf-13-web-integration.md`
- `impl-plans/completed/pf-14-engine.md`
- `impl-plans/completed/pf-15-present-ticket-payloads.md`
- `impl-plans/completed/pf-16-server-cli-testutil.md`
- `impl-plans/completed/pf-17-auth-oidc.md`
- `impl-plans/completed/pf-18-api-users-grades.md`
- `impl-plans/completed/pf-19-api-proxies-org.md`
- `impl-plans/completed/pf-20-api-definitions-masters.md`
- `impl-plans/completed/pf-21-api-roles-audit.md`
- `impl-plans/completed/pf-22-api-tickets.md`
- `impl-plans/completed/pf-23-api-comments-files.md`
- `impl-plans/completed/pf-24-admin-api.md`
- `impl-plans/completed/pf-25-readme.md`
- `impl-plans/completed/pf-26-contract-smoke.md`
- `impl-plans/completed/pf-27-finalize.md`
- `impl-plans/completed/punchflow-kickflow-dummy-s460-dispatch.json`
- `impl-plans/completed/punchflow-kickflow-dummy-s462-dispatch.json`
- `web/node_modules`
- `web/dist/app`
- `punchflow`

### sharedPaths
- `impl-plans/active/pf-01-foundation-primitives.md`
- `impl-plans/active/pf-02-data-contract.md`
- `impl-plans/active/pf-03-web-scaffold.md`
- `impl-plans/active/pf-04-httpx.md`
- `impl-plans/active/pf-05-auth-core.md`
- `impl-plans/active/pf-06-fixtures-loader.md`
- `impl-plans/active/pf-07-fixtures-default-keycloak.md`
- `impl-plans/active/pf-08-present-core.md`
- `impl-plans/active/pf-09-webhook-delivery.md`
- `impl-plans/active/pf-10-api-contracts.md`
- `impl-plans/active/pf-11-web-tickets.md`
- `impl-plans/active/pf-12-web-readviews.md`
- `impl-plans/active/pf-13-web-integration.md`
- `impl-plans/active/pf-14-engine.md`
- `impl-plans/active/pf-15-present-ticket-payloads.md`
- `impl-plans/active/pf-16-server-cli-testutil.md`
- `impl-plans/active/pf-17-auth-oidc.md`
- `impl-plans/active/pf-18-api-users-grades.md`
- `impl-plans/active/pf-19-api-proxies-org.md`
- `impl-plans/active/pf-20-api-definitions-masters.md`
- `impl-plans/active/pf-21-api-roles-audit.md`
- `impl-plans/active/pf-22-api-tickets.md`
- `impl-plans/active/pf-23-api-comments-files.md`
- `impl-plans/active/pf-24-admin-api.md`
- `impl-plans/active/pf-25-readme.md`
- `impl-plans/active/pf-26-contract-smoke.md`
- `impl-plans/active/punchflow-kickflow-dummy-s460-dispatch.json`
- `impl-plans/active/punchflow-kickflow-dummy-s462-dispatch.json`
- `impl-plans/README.md`
- `README.md`
- `mise.toml`
- `design-docs/specs/notes.md`
- `go.mod`
- `go.sum`
- `go-toolchain.json`
- `web/toolchain.json`

### sharedPathNotes
- `impl-plans/active/pf-NN-*.md` (26 files): set `**Status**: Completed` with a final progress entry, then move the file to the matching `impl-plans/completed/` path. This includes the stale headers of accepted plans (for example "In Progress").
- `impl-plans/active/punchflow-kickflow-dummy-s460-dispatch.json`, `...-s462-dispatch.json`: move them unchanged to `impl-plans/completed/`.
- `impl-plans/README.md`: after archiving, update the plan tables (all plans Completed), the phase status and the "Completed Plans" table. Name the s463 manifest as the final manifest, still in `active/` for the operator to archive.
- `README.md` (PF-25): edit only if PF-27's `mise.toml` change makes README text wrong (the description of `mise run test`). Nothing else.
- `mise.toml` (PF-01): change only the `test` task `run` to `go test -count=1 ./... && mise run web:test`. No other task changes.
- `design-docs/specs/notes.md`: append only. Add new INF rows for inferences that workers logged as INF candidates (for example the PF-20 category deletion rule), and one closing note per CF-01/CF-02/CF-03 under "Status at session 463". Never edit existing rows.
- `go.mod`, `go.sum` (PF-02): change only through `go mod tidy`. Record the diff in the progress log.
- `go-toolchain.json` (PF-02), `web/toolchain.json` (PF-03): append one record each. The Go record holds the tidy/verify commands, exit codes and the go version. The web record holds the frozen install and build commands, exit codes and the `bun.lock` sha256.

### artifactRoots
- `web/node_modules`
- `web/dist/app`
- `punchflow`

Expansion: 31 source writePaths + 36 sharedPaths = 67 files (< 300). The three artifact roots are excluded.

## Tasks

1. **Collect reconciliation requests.** Read the "Reconciliation requests" entries in every plan's progress log (PF-14..PF-26 and any routed accepted-plan repairs) and copy them into this plan's log.
   - Resolve a request only when the file is in this plan's paths above.
   - For any other file, write "routed to <owner per README rule 12>" and stop as blocked. Do not edit the file.
2. **Web build first.** It creates `web/dist/app`, so `TestEmbeddedBuild` can PASS later. Run the web line, then `mise run build` (Verification lines 1-2). Append the record to `web/toolchain.json`.
3. **Modules.** Run `go mod tidy` then `go mod verify` (Verification lines 3-4). Record the go.mod/go.sum diff (tidy may move `gopkg.in/yaml.v3` and `jsonschema/v6` from indirect to direct). Append the record to `go-toolchain.json`.
4. **mise.toml.** Change only the `test` task as noted above. Record pre/post hashes.
5. **Write `repo_invariants_test.go`** (spec below).
6. **Full gate.** Run Verification lines 5-16 in order.
   - On any failure outside PF-27's files: record a reconciliation request (`file:line`, owner, command, log path) and stop. Integration review routes it.
   - After the repair lands, re-run the whole Verification list from line 1.
7. **notes.md.** Append the INF candidates and the CF closing notes.
8. **Archive** (only after task 6 passes in one uninterrupted run).
   - For each of the 26 other plan files: set the Status, add the final progress entry, then `mv` it to `impl-plans/completed/`.
   - Move the s460 and s462 manifests the same way.
   - Last, set this plan Completed, finish its log, and move it.
   - Update `impl-plans/README.md`.

## repo_invariants_test.go (module root)

- Package clause: `package punchflow`, the same as `embed_test.go`. Standard library only (`os`, `io/fs`, `path/filepath`, `regexp`, `strings`, `bufio`, `testing`).
- Name the single test `TestRepoInvariants`. Prefix every helper with `pf27`. Do not define `TestMain` or `init`.
- The test runs with the working directory at the module root, because the package directory is the root.
- Walks skip the directories `.git`, `web/node_modules`, `web/dist`, `tmp` and any path segment named `tmp` (git-ignored evidence copies).
- Subtests (`t.Run` names exactly):
  - `cf01_ban`
    - Walk `internal/` and `fixtures/` and read every regular file.
    - Skip names ending in `_test.go` and every path under `internal/fixtures/testdata/invalid/`.
    - Apply the four regexes from the "CF-01 ban grep" block in `design-docs/specs/notes.md`, verbatim; Go RE2 supports `\b`.
    - Report every hit as `path:line: text` and fail if there is any.
  - `negative_tests_present`
    - `internal/fixtures/enum_rules_test.go` and `internal/api/v1/roles_test.go` each contain `audit_read`.
    - At least one `*_test.go` under `internal/admin/` contains `ticket_submitted` or `comment_deleted`.
    - A missing file is a failure.
  - `go_file_size`: every `.go` file in the repository (with the skips above) has fewer than 1000 lines. Report the offenders.
  - `no_foreign_lockfiles`: `web/package-lock.json`, `web/yarn.lock` and `web/pnpm-lock.yaml` do not exist (`os.Stat` returns `fs.ErrNotExist`).
- Pitfalls:
  - Do not shell out to `rg`.
  - Do not hard-code today's hit-free state by skipping directories that the notes.md grep scans.
  - Do not weaken a regex to pass. A real hit is routed to its owner.
  - Assemble the banned literals in this file as normal strings: it is a `_test.go` file, which the CF-01 scope excludes.
- Expected result: `--- PASS: TestRepoInvariants` with 4 passing subtests in the full gate log.

## Verification (all must exit 0; record exit status, log path and counts; copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-27/attempt-<n>/`)

```bash
bash -o pipefail -c '(cd web && bun install --frozen-lockfile && bun run typecheck && bun test && bun run build) 2>&1 | tee /tmp/pf27-s463-web.log'; echo "exit=$?"   # 1: record bun test pass/fail summary
bash -o pipefail -c 'mise run build 2>&1 | tee /tmp/pf27-s463-mise-build.log'; echo "exit=$?"   # 2: binary embeds web/dist/app
bash -o pipefail -c 'go mod tidy 2>&1 | tee /tmp/pf27-s463-tidy.log'; echo "exit=$?"   # 3
bash -o pipefail -c 'go mod verify 2>&1 | tee /tmp/pf27-s463-modverify.log'; echo "exit=$?"   # 4: "all modules verified"
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf27-s463-build.log'; echo "exit=$?"   # 5
bash -o pipefail -c 'go vet ./... 2>&1 | tee /tmp/pf27-s463-vet.log'; echo "exit=$?"   # 6
bash -o pipefail -c 'go test -count=1 -v -race ./... 2>&1 | tee /tmp/pf27-s463-test.log'; echo "exit=$?"   # 7: full gate
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf27-s463-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf27-s463-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf27-s463-test.log)"; grep -c '(cached)' /tmp/pf27-s463-test.log   # 8: FAIL=0 SKIP=0, last value 0
grep -E '^(ok|FAIL|\?)' /tmp/pf27-s463-test.log   # 9: every package with test files is "ok"; record the per-package lines (accepted PF-01..PF-13/PF-17 packages included)
grep -E -- '--- PASS: TestRepoInvariants|--- PASS: TestEmbeddedBuild|--- PASS: TestServeSmoke|--- PASS: TestDefaultRoundTrip' /tmp/pf27-s463-test.log   # 10: all four present
bash -o pipefail -c 'mise run test 2>&1 | tee /tmp/pf27-s463-mise-test.log'; echo "exit=$?"   # 11: task works; must show no "(cached)"
bash -o pipefail -c 'mise run lint 2>&1 | tee /tmp/pf27-s463-mise-lint.log'; echo "exit=$?"   # 12
gofmt -l cmd internal *.go   # 13: prints nothing
rg -n -e 'ticket_submitted|ticket_remanded|ticket_resubmitted|comment_deleted|ticket_admin|audit_read' -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' -e 'StepTypeGrade|StepTypeParallel|StepStatus(Approved|Denied|Remanded)|TicketStatusWithdrawn' -e '"userType": *"member"|"status": *"active"|"sectionType": *"conditional"|"gradeSymbol": *""' internal fixtures --glob '!**/*_test.go' --glob '!internal/fixtures/testdata/invalid/**'   # 14: CF-01 manual cross-check, prints nothing (exit 1)
git status --porcelain web/dist/app punchflow   # 15: prints nothing (git-ignored artifacts)
ls web/package-lock.json web/yarn.lock web/pnpm-lock.yaml 2>&1   # 16: "No such file" for all three
```

Manual check left to the user (documented in README by PF-25): Keycloak login via `mise run keycloak:up`.

## Done Criteria

- [ ] Every reconciliation request is listed. Each one is resolved inside PF-27's paths or routed to its owner and re-verified after the repair.
- [ ] Verification lines 1-16 passed in one final uninterrupted run, each with exit status, log path and counts. Full gate: FAIL=0, SKIP=0, zero `(cached)`, every package `ok`, and `TestRepoInvariants` (4 subtests), `TestEmbeddedBuild`, `TestServeSmoke` and `TestDefaultRoundTrip` PASS.
- [ ] CF-01 closed by `cf01_ban` and `negative_tests_present` PASS; CF-02 closed by the PF-20 input-ID tests and `TestDefaultRoundTrip` PASS; CF-03 closed by the PF-05 `internal/auth` package `ok`. A closing note for each is appended to `notes.md`.
- [ ] `mise.toml` `test` task is uncached; `go-toolchain.json` and `web/toolchain.json` each have one appended record.
- [ ] 27 plans and the s460/s462 manifests are in `impl-plans/completed/`. `impl-plans/README.md` is updated. The s463 manifest is untouched in `active/`.

## Continuation (session 464) - read first; overrides the session-463 log names, archival list and Done Criteria above

**Wave**: D (serial, last). **Depends on**: every other plan; accepted PF-01..PF-13 and PF-17 are at `44e40cc`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json`. This plan's `writePaths` (35, of which 3 are `artifactRoots`) and `sharedPaths` (37) give 69 source entries with no directory entry. They are authoritative.

**Session-464 changes**:
1. Log names: replace every `-s463-` in the Verification block above with `-s464-`. Copy the logs to `tmp/punchflow-kickflow-dummy-s464/PF-27/attempt-<n>/`. The 16 commands are otherwise identical; the manifest `verification` array has them.
2. Archival:
   - Archive the 27 plans and three historical manifests: s460, s462 and s463. Move s463 last, after the gate passes, as `impl-plans/completed/punchflow-kickflow-dummy-s463-dispatch.json`.
   - The s464 manifest stays in `impl-plans/active/`. Name it in `impl-plans/README.md` as the final manifest that the operator archives.
3. `design-docs/specs/notes.md`: append closing notes for CF-01, CF-02 and CF-03 under "Status at session 463", and for CF-04, CF-05 and CF-06 below the session-463 wave-A findings table. Cite the evidence:
   - CF-04: PF-16 jqnorm and roundtrip logs plus `TestDefaultRoundTrip` in the full gate
   - CF-05: `TestDoRawResolvesRelativeURLs` in the full gate
   - CF-06: the wave progress logs' compile-gate lines
4. Repair routing: a failing gate in a foreign file goes to its owner from the s464 ownership index (`ownershipTransfers[]` first). PF-27 never edits it, and re-runs its whole gate after each routed repair.

### Done Criteria (session 464)

- [ ] Verification lines 1-16 (with `-s464-` logs) passed in one final uninterrupted run, each with exit status, log path and counts. Full gate: FAIL=0, SKIP=0, zero `(cached)`, every package `ok`, and `TestRepoInvariants` (4 subtests), `TestEmbeddedBuild`, `TestServeSmoke` and `TestDefaultRoundTrip` PASS.
- [ ] CF-01..CF-06 closing notes appended to `notes.md`; the `mise.toml` `test` task is uncached; `go-toolchain.json` and `web/toolchain.json` each have one appended record.
- [ ] 27 plans and the s460/s462/s463 manifests are in `impl-plans/completed/`; `impl-plans/README.md` is updated; the s464 manifest is untouched in `active/`.

## Continuation (session 465) - read first; overrides the session-463/464 log names, archival list, verification forms and Done Criteria

**Wave**: D (serial, last). **Depends on**: every other plan. PF-14 is replaced by PF-14a, PF-14b and PF-14c. The accepted PF-01..PF-13 and PF-17 are at `ea385c7`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. This plan's `writePaths` (39, of which 3 are `artifactRoots`) and `sharedPaths` (41) give 77 source entries with no directory entry. They are authoritative over the "Path Declarations" list above.

**Session-465 changes**:
1. Paths added (all files):
   - `writePaths`: `impl-plans/completed/pf-14a-engine-lifecycle.md`, `impl-plans/completed/pf-14b-engine-routing.md`, `impl-plans/completed/pf-14c-engine-forms-create.md`, `impl-plans/completed/punchflow-kickflow-dummy-s464-dispatch.json`
   - `sharedPaths`: the matching `impl-plans/active/` files of the three split plans, and `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json`
2. Log names: `-s465-`, copied to `tmp/punchflow-kickflow-dummy-s465/PF-27/attempt-<n>/`.
3. Verification forms (README rule 14). The manifest `verification` array is the 16 s464 commands with these must-pass rewrites. Each must show exit 0:
   - line 8: `...; grep -c '(cached)' <log>; test $? -eq 1`
   - line 13: `test -z "$(gofmt -l cmd internal *.go)"`
   - line 14: CF-01 cross-check `rg ...; test $? -eq 1`
   - line 15: `test -z "$(git status --porcelain web/dist/app punchflow)"`
   - line 16: `test ! -e web/package-lock.json && test ! -e web/yarn.lock && test ! -e web/pnpm-lock.yaml`
4. Archival, only after the gate passes in one uninterrupted run:
   - 30 plan files: PF-01..PF-27 including the superseded `pf-14-engine.md`, plus `pf-14a`/`pf-14b`/`pf-14c`.
   - Four historical manifests: s460, s462, s463, s464. Move s464 last.
   - The s465 manifest stays in `impl-plans/active/`. Name it in `impl-plans/README.md` as the final manifest that the operator archives.
5. `design-docs/specs/notes.md`: append closing notes for CF-01..CF-06 (as in s464), plus CF-07, CF-08 and CF-09 below the session-464 wave-A findings table:
   - CF-07: the PF-14a/b/c named-test PASS lines in the full gate
   - CF-08: s465 reports have no expected-non-zero command in `verification`
   - CF-09: `TestLoadExportRoundTripMinimal` PASS in the full gate
6. Repair routing: a failing gate in a foreign file goes to its owner from the s465 ownership index (`ownershipTransfers[]` first; engine files by the PF-14a/b/c split). PF-27 never edits it, and re-runs the whole gate after each routed repair.

### Done Criteria (session 465)

- [ ] Verification lines 1-16 (`-s465-` logs) passed in one final uninterrupted run, each with exit 0, log path and counts. Full gate: FAIL=0, SKIP=0, zero `(cached)`, every package `ok`. `TestRepoInvariants` (4 subtests), `TestEmbeddedBuild`, `TestServeSmoke`, `TestDefaultRoundTrip` and `TestLoadExportRoundTripMinimal` PASS.
- [ ] CF-01..CF-09 closing notes appended to `notes.md`; the `mise.toml` `test` task is uncached; `go-toolchain.json` and `web/toolchain.json` each have one appended record.
- [ ] 30 plan files and the s460/s462/s463/s464 manifests are in `impl-plans/completed/`; `impl-plans/README.md` is updated; the s465 manifest is untouched in `active/`.

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains (for example a routed repair that has not landed), report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. A foreign failure is a reconciliation request that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15), except that this plan runs `go mod tidy` and builds `web/dist/app` and `./punchflow`. Record pre/post `shasum -a 256` for every shared file you edit. Do not commit (the workflow commits).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests resolved/routed**: -
