# PF-27 Reconciliation and Finalization Implementation Plan

**Status**: Completed
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

## Continuation (session 466) - read first; overrides the session-463/464/465 dependencies, paths, archival list, log names and Done Criteria

**Wave**: B (serial, last). **Depends on**: PF-26a, PF-26b, PF-26c, PF-26d, PF-26e, PF-26f. Every other plan, PF-01..PF-25 including PF-14a/b/c, is accepted at `a2a82de`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s466-dispatch.json`. Its PF-27 `writePaths` and `sharedPaths` are authoritative over the "Path Declarations" list above.
**Design**: `design-docs/specs/architecture.md` "PF-26 split and final-run rule (session 466)" item 5; `design-docs/specs/design-api-coverage.md` "Aggregate coverage (PF-27, after the wave)".

Session-466 changes:

1. **Paths** (all files; no directory entries):
   - `writePaths` = the s465 list (39, including 3 `artifactRoots`) plus 8 entries:
     - `internal/contract/coverage_test.go`
     - `impl-plans/completed/pf-26a-contract-tickets.md`, `impl-plans/completed/pf-26b-contract-comments-files.md`, `impl-plans/completed/pf-26c-contract-users-grades.md`, `impl-plans/completed/pf-26d-contract-orgcharts-teams.md`, `impl-plans/completed/pf-26e-contract-masters.md`, `impl-plans/completed/pf-26f-contract-proxies-roles.md`
     - `impl-plans/completed/punchflow-kickflow-dummy-s465-dispatch.json`
   - `sharedPaths` = the s465 list (41) plus 7 entries: the six matching `impl-plans/active/pf-26?-*.md` files, and `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`.
   - Expansion: 47 - 3 + 48 = 92 files.
2. **Non-goal amendment.** PF-27 may create exactly one file under `internal/`: `internal/contract/coverage_test.go`. `repo_invariants_test.go` stays at the module root. Every other non-goal stands.
3. **New task 5b: `internal/contract/coverage_test.go`** (`package contract_test`; standard library plus `punchflow/internal/api/v1`).
   - Single test `TestContractOperationCoverage`; helpers prefixed `pf27`.
   - Build a list of operationIds from `liveReadScenarios[i].operationID` (22 entries, hoisted by PF-26c) followed by `liveTicketsOperations`, `liveCommentsFilesOperations`, `liveUsersGradesOperations`, `liveOrgChartsTeamsOperations`, `liveMastersOperations` and `liveProxiesRolesOperations`.
   - Assert: no id appears twice (report the duplicates); the list length is 96; the set equals the set of `apiv1.Operations()[i].OperationID` in both directions (report missing and extra ids).
   - Write it only after all six sub-plans are accepted, so every identifier exists. Then run `go vet ./internal/contract/...` (compile gate).
   - Test cases:
     - current tree -> PASS;
     - mutation proof, not in `verification`: temporarily drop one id from a copy of the list inside a scratch test and observe the failure, or reason it out in `authorSelfCheck`. Do not commit a mutated file.
4. **Log names**: `-s466-`, copied to `tmp/punchflow-kickflow-dummy-s466/PF-27/attempt-<n>/`. The 16 verification lines are the s465 must-pass forms with `-s465-` replaced by `-s466-`. Line 10 is replaced by:
   - `grep -E -- '--- PASS: (TestRepoInvariants|TestEmbeddedBuild|TestServeSmoke|TestDefaultRoundTrip|TestLoadExportRoundTripMinimal|TestContractOperationCoverage|TestContractWebhookPayloads|TestContractRoutes|TestContractLiveReadScenarios|TestContractLiveTickets|TestContractLiveCommentsFiles|TestContractLiveUsersGrades|TestContractLiveOrgChartsTeams|TestContractLiveMasters|TestContractLiveProxiesRoles) ' /tmp/pf27-s466-test.log | sort -u | wc -l`, which must print 15.
   - As must-pass form: `test "$(<that pipeline>)" -eq 15; echo "exit=$?"`.
5. **Smoke requirement** (architecture s466 item 5): satisfied by `TestServeSmoke` (temp SQLite, seeded tokens, `/v1/user`, `POST /v1/tickets`, `/healthz`, `/`) and `TestContractWebhookPayloads` (httptest receiver, signatures) in the uncached full gate (line 7). Do not start a background `punchflow serve`. The manual curl walkthrough stays in `README.md` "Point a kickflow client at punchflow".
6. **README.md**: it already documents the base URL, seeded tokens, webhook registration and signature verification (PF-25). Edit only the `mise run test` description, if the `mise.toml` change below makes it wrong.
7. **mise.toml** (unchanged intent): only the `test` task `run` becomes `go test -count=1 ./... && mise run web:test`. At `a2a82de` it is still `go test ./... && mise run web:test`.
8. **Toolchain records**: `go-toolchain.json` and `web/toolchain.json` are single JSON objects. Add one top-level key `"finalizeS466"`, an object with the commands, exit codes, `goVersion` / `bunVersion` and the `bun.lock` sha256. Keep every existing key unchanged. The files must stay valid JSON (`jq . <file>` exit 0).
9. **Reconciliation collection**: copy the "Transfer candidates" and "Reconciliation requests" from the PF-26a..PF-26f progress logs into this log. Resolve nothing outside PF-27's paths.
10. **notes.md** (append only): the CF-01..CF-09 closing notes as in s465, plus one line under the s464 wave-A findings table. That line records the PF-26 split outcome: the six `TestContractLive*` PASS lines and `TestContractOperationCoverage` PASS, with the gate log path.
11. **Archival**, only after the gate passes in one uninterrupted run:
    - 36 plan files: the s465 list of 30 plus the six PF-26 sub-plans. `pf-26-contract-smoke.md` (superseded index) is already in the 30.
    - Five historical manifests: s460, s462, s463, s464, s465. Move s465 last.
    - The s466 manifest stays in `impl-plans/active/`. Name it in `impl-plans/README.md` as the final manifest that the operator archives.
    - Last, set this plan to Completed and move it.

### Done Criteria (session 466)

- [ ] Verification lines 1-16 (`-s466-` logs; line 10 as rewritten above) passed in one final uninterrupted run, each with exit 0, log path and counts. The run stopped at line 12 (`mise run lint`) because of foreign-file diagnostics; lines 13-16 remain unrun.
- [x] `internal/contract/coverage_test.go` exists and passes (96 unique operations equal to `apiv1.Operations()`).
- [ ] CF closing notes and the PF-26 split note appended to `notes.md`. `mise.toml` `test` is uncached. Both toolchain files have a `finalizeS466` key and are valid JSON.
- [ ] 36 plan files and the s460..s465 manifests are in `impl-plans/completed/`. `impl-plans/README.md` is updated. The s466 manifest is untouched in `active/`.

## Continuation (session 467) - read first; overrides every earlier continuation's paths, tasks, archival list, log names and Done Criteria

**Wave**: single (serial, alone). **Depends on**: nothing to dispatch. Every other plan is accepted at `f6f4ba8`: PF-01..PF-13, PF-14a/b/c, PF-15..PF-25 and PF-26a..PF-26f.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s467-dispatch.json`. Its PF-27 `writePaths` are authoritative. It has no `sharedPaths`.
**Design**: `design-docs/specs/architecture.md`, "Final gate and archival rule (session 467)" items 1-7.

### Intent and context

Session 466 stopped at gate line 12 (`mise run lint`) because of 40 diagnostics in files owned by other plans. Commit `f6f4ba8` fixed them (v2 `.golangci.yml`, 0 issues). Every source deliverable of PF-27 already exists at HEAD:
- `repo_invariants_test.go`
- `internal/contract/coverage_test.go`
- the uncached `mise.toml` `test` task (`mise.toml:89`)
- `finalizeS466` in both toolchain JSON files
- the tidied `go.mod`/`go.sum`

Session 467 is docs and plans only. Three things remain: (1) the ordered gate, (2) the `notes.md` closing notes, (3) the README update and archival.

### Non-goals (s467)

- No edits to any `.go` file, `mise.toml`, `go.mod`, `go.sum`, `go-toolchain.json`, `web/toolchain.json`, `README.md`, `.golangci.yml`, `fixtures/`, `deploy/`, `web/` sources or `design-docs/specs/architecture.md`.
- No `gofmt -w`, no identifier renames.
- No git commands that change state. This includes `git mv`: use plain `mv`. The workflow commits.
- No background `punchflow serve`.
- Do not edit or move `impl-plans/active/punchflow-kickflow-dummy-s467-dispatch.json`. It stays in `active/` as the final manifest.
- Never edit an existing row or bullet of `notes.md`.

### Paths (s467; all files, no directory source entries)

- `writePaths` (86 source entries):
  - `design-docs/specs/notes.md`
  - `impl-plans/README.md`
  - `impl-plans/active/pf-27-finalize.md` and `impl-plans/completed/pf-27-finalize.md`
  - an `impl-plans/active/<f>` / `impl-plans/completed/<f>` pair for each of the 41 other files. These are the 35 plan files other than this one and the six manifests `punchflow-kickflow-dummy-s460/s462/s463/s464/s465/s466-dispatch.json`.
- `artifactRoots`, which are also in `writePaths` and git-ignored:
  - `tmp/punchflow-kickflow-dummy-s467`
  - `web/node_modules`
  - `web/dist/app`
  - `punchflow`

### Tasks (s467)

1. **Progress-log setup.**
   - Append an "Intent snapshot" to this plan's Progress Log that lists the files above.
   - Record `shasum -a 256` of `design-docs/specs/notes.md`, `impl-plans/README.md`, `go.mod` and `go.sum`.
2. **Gate, attempt n (start at 1).** Run lines 1-16 below in order, in the foreground, with no edits in between.
   - Copy every `/tmp/pf27-s467-*.log` to `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-<n>/`.
   - Record exit status, log path and counts for each line:
     - bun pass/fail summary (line 1)
     - PASS/FAIL/SKIP and the `(cached)` count (line 8)
     - the per-package `ok` lines (line 9)
     - the count 15 (line 10)
   - Run `git diff --quiet go.mod go.sum` right after line 3.
   - On any failure, including a tidy diff:
     - stop and record a reconciliation request: `file:line`, the owner, the command and the log path.
     - Find the owner in the s466 ownership index: `impl-plans/active/punchflow-kickflow-dummy-s466-dispatch.json` `ownershipIndex` and `ownershipTransfers[]` first, then the inherited s465 transfers, then the plans' `writePaths`.
     - A tidy diff goes to PF-02.
     - Set `implementationIncomplete: true`. Do not archive.
     - After the routed repair lands, restart from line 1 as attempt n+1.
3. **notes.md closing notes.** Do this only after one attempt passes all 16 lines.
   - Insert a new subsection `### Closing notes (session 467)`. It goes after the CF-07..CF-09 table, which ends with the `| CF-09 |` row, and before `### Intentional divergences from kickflow`.
   - Write exactly 10 bullets, in this order: `- CF-01:` .. `- CF-09:`, then `- PF-26 split:`.
   - Each bullet says "Closed in session 467" and cites the evidence from the architecture.md s467 item 7 table: the test names, the gate line, the log `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-<n>/pf27-s467-test.log` and its counts.
   - The PF-26 split bullet names `TestContractLiveTickets`, `TestContractLiveCommentsFiles`, `TestContractLiveUsersGrades`, `TestContractLiveOrgChartsTeams`, `TestContractLiveMasters`, `TestContractLiveProxiesRoles` and `TestContractOperationCoverage` (96 unique operations).
   - CF-08 cites the s466 PF-27 `verification` array and the s467 manifest `verification` array (must-pass forms only).
   - This is an insertion only: `git diff --numstat` must show 0 deleted lines for `notes.md`.
4. **Archive the 35 other plan files.** For each one:
   - Replace line 3, the first `**Status**:` line, with `**Status**: Completed`. For the two superseded indexes use these instead:
     - `pf-14-engine.md`: `**Status**: Completed (superseded by PF-14a/PF-14b/PF-14c)`
     - `pf-26-contract-smoke.md`: `**Status**: Completed (superseded by PF-26a..PF-26f)`
   - Leave any later `**Status**:` lines inside progress logs (pf-14b, pf-18, pf-24) untouched.
   - Append this one-line entry at the end of the file: `### Session: 2026-10-21 (session 467, PF-27 archival)` followed by `Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-<n>/).`
   - `mv` the file to `impl-plans/completed/` under the same basename.
5. **Archive the manifests** with `mv`, bytes unchanged. Order: s460, s462, s463, s464, s465, and s466 last.
6. **`impl-plans/README.md`** (fresh read and hash first):
   - Replace the body of `## Active Plans (punchflow dummy kickflow server)`, everything up to `## Completed Plans`, with a short paragraph:
     - every plan is completed and archived in session 467;
     - `active/` holds only `punchflow-kickflow-dummy-s467-dispatch.json`, the final manifest;
     - the s460 and s462..s466 manifests are in `completed/`.
   - Replace the `## Completed Plans` table with 36 rows, one per plan, PF-01..PF-27 in id order, with PF-14a/b/c after PF-14 and PF-26a..f after PF-26.
     - Plan: link `[<basename>](./completed/<basename>.md)`.
     - Completed: `2026-10-21 (s467)`. The two superseded indexes say `Superseded (archived s467)`.
     - Design Reference:
       - PF-14* -> `design-docs/specs/design-approval-engine.md`
       - PF-26* -> `design-docs/specs/design-api-coverage.md`
       - PF-09 and PF-15 -> `design-docs/specs/design-webhooks.md`
       - PF-05, PF-17 and PF-24 -> `design-docs/specs/design-auth-admin.md`
       - every other plan -> `design-docs/specs/architecture.md`
   - `### Phase Status`: set rows A and B to `COMPLETED`.
   - `PHASE_TO_PLANS`: replace the block with `PHASE_TO_PLANS = {}` and a comment saying all plans are completed (session 467) and the final manifest is `punchflow-kickflow-dummy-s467-dispatch.json`.
   - Leave the Shared Execution Protocol and the Workflow and Guidelines sections unchanged.
   - Afterwards, no `](./active/` link may remain.
7. **Finish this plan, the last move.**
   1. Run the pre-move checks: lines 17, 21, 22, 23 and 24 below. If any fails, fix it inside PF-27's own paths and re-run it. A failure caused by a foreign file is a reconciliation request.
   2. Append the final progress entry to this plan. It holds all gate and pre-move evidence: exit status, log paths and counts.
   3. Tick the s467 Done Criteria that the evidence supports.
   4. Set line 3 to `**Status**: Completed`.
   5. `mv` this file to `impl-plans/completed/pf-27-finalize.md`. Never edit it after the move.
   6. Run the post-move checks: lines 18, 19, 20 and 25. Their evidence goes into the step report (`verification` and `authorSelfCheck`), not into the moved file.

### Pitfalls

- Archiving before the gate passes, or editing a file between gate lines, invalidates the attempt.
- `go mod tidy` must not change `go.mod`/`go.sum`. Never "fix" them by hand.
- `grep -c '(cached)'` exits 1 when nothing is found. That exit is the required result of line 8's `test $? -eq 1`. Do not treat it as a failure.
- Do not count `--- PASS` lines of subtests for line 10. The pipeline anchors on the trailing space and uses `sort -u`.
- Lines 7 and 11 each run the full suite. Give each command a long enough timeout (about 10 minutes) and never background it.
- Do not use a shell variable named `status`.

### Verification (lines 1-16: the gate, before archival; lines 17-25: after archival; every line must print exit=0 or exit 0)

```bash
bash -o pipefail -c '(cd web && bun install --frozen-lockfile && bun run typecheck && bun test && bun run build) 2>&1 | tee /tmp/pf27-s467-web.log'; echo "exit=$?"   # 1
bash -o pipefail -c 'mise run build 2>&1 | tee /tmp/pf27-s467-mise-build.log'; echo "exit=$?"   # 2
bash -o pipefail -c 'go mod tidy 2>&1 | tee /tmp/pf27-s467-tidy.log'; echo "exit=$?"; git diff --quiet go.mod go.sum; echo "tidy-diff-exit=$?"   # 3 (both 0)
bash -o pipefail -c 'go mod verify 2>&1 | tee /tmp/pf27-s467-modverify.log'; echo "exit=$?"   # 4: all modules verified
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf27-s467-build.log'; echo "exit=$?"   # 5
bash -o pipefail -c 'go vet ./... 2>&1 | tee /tmp/pf27-s467-vet.log'; echo "exit=$?"   # 6
bash -o pipefail -c 'go test -count=1 -v -race ./... 2>&1 | tee /tmp/pf27-s467-test.log'; echo "exit=$?"   # 7
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf27-s467-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf27-s467-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf27-s467-test.log)"; grep -c '(cached)' /tmp/pf27-s467-test.log; test $? -eq 1; echo "exit=$?"   # 8: FAIL=0 SKIP=0
grep -E '^(ok|FAIL|\?)' /tmp/pf27-s467-test.log; test "$(grep -cE '^FAIL' /tmp/pf27-s467-test.log)" -eq 0; echo "exit=$?"   # 9
test "$(grep -E -- '--- PASS: (TestRepoInvariants|TestEmbeddedBuild|TestServeSmoke|TestDefaultRoundTrip|TestLoadExportRoundTripMinimal|TestContractOperationCoverage|TestContractWebhookPayloads|TestContractRoutes|TestContractLiveReadScenarios|TestContractLiveTickets|TestContractLiveCommentsFiles|TestContractLiveUsersGrades|TestContractLiveOrgChartsTeams|TestContractLiveMasters|TestContractLiveProxiesRoles) ' /tmp/pf27-s467-test.log | sort -u | wc -l)" -eq 15; echo "exit=$?"   # 10
bash -o pipefail -c 'mise run test 2>&1 | tee /tmp/pf27-s467-mise-test.log'; echo "exit=$?"; grep -c '(cached)' /tmp/pf27-s467-mise-test.log; test $? -eq 1; echo "nocache-exit=$?"   # 11
bash -o pipefail -c 'mise run lint 2>&1 | tee /tmp/pf27-s467-mise-lint.log'; echo "exit=$?"   # 12
test -z "$(gofmt -l cmd internal *.go)"; echo "exit=$?"   # 13
rg -n -e 'ticket_submitted|ticket_remanded|ticket_resubmitted|comment_deleted|ticket_admin|audit_read' -e '\b(user|org|workflow|role|file|proxy)_(read|write)\b' -e 'StepTypeGrade|StepTypeParallel|StepStatus(Approved|Denied|Remanded)|TicketStatusWithdrawn' -e '"userType": *"member"|"status": *"active"|"sectionType": *"conditional"|"gradeSymbol": *""' internal fixtures --glob '!**/*_test.go' --glob '!internal/fixtures/testdata/invalid/**'; test $? -eq 1; echo "exit=$?"   # 14
test -z "$(git status --porcelain web/dist/app punchflow)"; echo "exit=$?"   # 15
test ! -e web/package-lock.json && test ! -e web/yarn.lock && test ! -e web/pnpm-lock.yaml; echo "exit=$?"   # 16
go build ./...; echo "exit=$?"   # 17: tree still compiles after archival
test "$(ls impl-plans/completed | wc -l)" -eq 42; echo "exit=$?"   # 18: 36 plans + 6 manifests
test "$(ls impl-plans/active)" = "punchflow-kickflow-dummy-s467-dispatch.json"; echo "exit=$?"   # 19
bad=0; n=0; for f in impl-plans/completed/pf-*.md; do n=$((n+1)); sed -n 3p "$f" | grep -q '^\*\*Status\*\*: Completed' || { echo "BAD $f"; bad=1; }; done; test "$bad" -eq 0 && test "$n" -eq 36; echo "exit=$?"   # 20
bad=0; n=0; for f in impl-plans/completed/punchflow-kickflow-dummy-s46*-dispatch.json; do n=$((n+1)); git show "HEAD:impl-plans/active/$(basename "$f")" | cmp -s - "$f" || { echo "CHANGED $f"; bad=1; }; done; test "$bad" -eq 0 && test "$n" -eq 6; echo "exit=$?"   # 21: six manifests byte-identical
test "$(git diff --numstat -- design-docs/specs/notes.md | awk '{print $2}')" = 0 && test "$(sed -n '/^### Closing notes (session 467)/,/^### Intentional divergences/p' design-docs/specs/notes.md | grep -cE '^- (CF-0[1-9]|PF-26 split):')" -eq 10; echo "exit=$?"   # 22
rg -q 'punchflow-kickflow-dummy-s467-dispatch.json' impl-plans/README.md && ! rg -q '\]\(\./active/' impl-plans/README.md; echo "exit=$?"   # 23
test -z "$(git status --porcelain -- cmd internal fixtures deploy web/src web/package.json web/bun.lock mise.toml go.mod go.sum go-toolchain.json web/toolchain.json README.md embed.go embed_test.go repo_invariants_test.go .golangci.yml design-docs/specs/architecture.md)"; echo "exit=$?"   # 24: no source/foreign edits
test -e impl-plans/completed/pf-27-finalize.md && test ! -e impl-plans/active/pf-27-finalize.md; echo "exit=$?"   # 25
```

### Done Criteria (session 467)

- [x] Lines 1-16 passed in one uninterrupted attempt. Each line has exit 0, a log path and counts. Full gate: FAIL=0, SKIP=0, no `(cached)`, every package `ok`, the 15 named tests PASS. Logs are under `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-<n>/`.
- [x] `notes.md` has the `### Closing notes (session 467)` subsection with 10 bullets, CF-01..CF-09 and the PF-26 split, citing gate evidence. Line 22 passes (0 deletions).
- [x] The 35 other plan files (line 3 Completed, final entry appended) and the six manifests (byte-identical, line 21) are in `impl-plans/completed/`. Line 17 passes. This plan's own move and the post-move lines 18, 19, 20 and 25 are evidenced in the step report.
- [x] `impl-plans/README.md` shows every plan Completed, links point to `./completed/`, and it names the s467 final manifest. Line 23 passes.
- [x] No source or foreign file edited (line 24).

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains (for example a routed repair that has not landed), report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. A foreign failure is a reconciliation request that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15), except that this plan runs `go mod tidy` and builds `web/dist/app` and `./punchflow`. Record pre/post `shasum -a 256` for every shared file you edit. Do not commit (the workflow commits).

## Progress Log

### Session: 2026-10-21 (session 466, PF-27 implementation)
**Intent snapshot**: Add `internal/contract/coverage_test.go` to assert the 22 `liveReadScenarios` and six PF-26 operation inventories are duplicate-free and exactly equal `apiv1.Operations()`; update this plan's progress log with implementation and final-gate evidence. The downstream review-dependent docs, archive moves, and README index updates remain for their workflow steps.
**Tasks Completed**: All six PF-26 predecessors were accepted. Added the aggregate 96-operation inventory test and `repo_invariants_test.go`; changed only the `mise.toml` test run to `go test -count=1 ./... && mise run web:test`; ran `go mod tidy` and `go mod verify`; appended `finalizeS466` records to both toolchain manifests. Go module tidy promoted `go-oidc`, `jsonschema/v6`, `oauth2`, `yaml.v3` and `modernc/sqlite` to direct requirements and refreshed transitive go.sum entries. The final full Go gate and uncached mise test pass, but the required lint command failed on foreign files; work stops at that reconciliation boundary.
**Verification**:
- Focused coverage guard: `go test -count=1 -v -race -run '^TestContractOperationCoverage$' ./internal/contract/...` exit 0; PASS=1, FAIL=0, SKIP=0; `/tmp/pf27-s466-step6-coverage.log` and `tmp/punchflow-kickflow-dummy-s466/PF-27/attempt-1/coverage.log`.
- Coverage vet: `go vet ./internal/contract/...` exit 0; `/tmp/pf27-s466-step6-vet-contract.log`.
- `go vet .` exit 0; `/tmp/pf27-s466-step6-vet-root.log` and `.../vet-root-r2.log`.
- First `TestRepoInvariants` run exposed wrapped `os.Stat` not-exist errors; its log is `.../attempt-1/invariants.log` (exit 1, PASS=3, FAIL=2, SKIP=0). Replaced direct `fs.ErrNotExist` equality with `os.IsNotExist`; retry `go test -count=1 -v -run '^TestRepoInvariants$' .` exited 0, PASS=5, FAIL=0, SKIP=0 at `/tmp/pf27-s466-step6-invariants-r2.log` and `.../attempt-1/invariants-r2.log`.
- Attempt 2 ordered checks: web install/typecheck/test/build exit 0 (Bun 25 pass, 0 fail; `/tmp/pf27-s466-attempt-2-web.log`); `mise run build` exit 0 (`...-mise-build.log`); `go mod tidy` exit 0 (`...-tidy.log`); `go mod verify` exit 0, “all modules verified” (`...-modverify.log`); `go build ./...` exit 0 (`...-build.log`); `go vet ./...` exit 0 (`...-vet.log`). Full `go test -count=1 -v -race ./...` exit 0, PASS=793, FAIL=0, SKIP=0, cached=0 (`...-test.log`). All 15 required named top-level tests passed, including `TestRepoInvariants`, `TestContractOperationCoverage` and six `TestContractLive*`; package summaries are in `.../packages.log`.
- `mise run test` exit 0 and uncached; Bun summary 25 pass, 0 fail (`/tmp/pf27-s466-attempt-2-mise-test.log`).
- `mise run lint` exit 1; complete log `/tmp/pf27-s466-attempt-2-mise-lint.log`. `golangci-lint run ./...` found 40 diagnostics (24 errcheck, 1 ineffassign, 10 staticcheck, 5 unused) and the task stopped before `go vet ./...` / web typecheck. The ordered gate stopped here as required; verification lines 13-16 were not run. Full attempt-2 logs are copied under `tmp/punchflow-kickflow-dummy-s466/PF-27/attempt-2/`.
- Toolchain JSON validation: `jq -e '.finalizeS466' go-toolchain.json` and same for `web/toolchain.json` both exited 0. Go version `go1.25.4 darwin/arm64`; Bun `1.4.3`; bun.lock SHA-256 `eee300137f6b77ce73ab6491efd1cda132c13fd3548c394bf5ba7270788beee0`.
- Hashes: mise.toml before `ae06bfa549ad47fb22631fac69b4ce27a2ac717e8054ed98f6f964cfb2d31eef`, after `452223bf9701d6bd4ef3d7a4aedf9040c2963440fe27003cdeaf82bbf8ae7e00`; go.mod after tidy `ee9df89830fb0ac16f7a7e121b3a71e93567fb5746ff161e2038bfe4d1e7de6f`, go.sum `b8a47c6606ec32aaa02f025d68e6494097cb2c9020ab114248307e402869030f`; go-toolchain.json `c5fbcbcabc0ac888b7c5db46ca5cd4740004f8401723a1a83ec047dd49611692`; web/toolchain.json `24255d992503c95871b083d14e4e9689a726c06c3591f3f75029ff4e11aaef10`.
**Reconciliation requests** (required resume boundary; ownership resolved using s466/s465/s462/s460 indexes):
- `mise run lint` `/tmp/pf27-s466-attempt-2-mise-lint.log` reports sibling-owned diagnostics: PF-26b (`internal/api/v1/files.go:45,61`, `internal/files/download.go:61`, `internal/api/v1/comments_test.go:173`); PF-26a (`internal/engine/update.go:89`, `internal/engine/conditions.go:88,118,122`, `internal/engine/forms.go:21`, `internal/engine/routing.go:95`, `internal/api/v1/tickets.go:332`, `internal/engine/actions.go:554`, `internal/engine/create.go:160`); PF-26f (`internal/store/roles.go:16`). These are outside PF-27 writePaths.
- Accepted-plan diagnostics from the same lint command: PF-02 (`internal/store/db.go:61,66`, `internal/store/migrate.go:48,52`, `internal/store/store_test.go:89,116,149`); PF-04 (`internal/httpx/middleware_test.go:48`); PF-05 (`internal/store/auth.go:117`); PF-06 (`internal/fixtures/validate.go:387`); PF-08 (`internal/present/workflow.go:108`); PF-09 (`internal/webhook/dispatcher.go:99`); PF-16 (`internal/fixtures/default_roundtrip_test.go:78`, `internal/fixtures/export.go:506-516`, `internal/testutil/env.go:146`, `internal/testutil/testutil_test.go:50`, `internal/admin/admin.go:41`); PF-17 (`internal/auth/oidc_fake_test.go:67`); PF-21 (`internal/store/audit_queries.go:41`); PF-24 (`internal/store/admin.go:27`). `internal/cli/serve_smoke_test.go:105` is frozen by the s466 rule. The historical generated `tmp/punchflow-kickflow-dummy-s462/PF-06/attempt-1/generate/main.go:21` has no listed plan owner. These paths require integration routing; PF-27 did not edit them.
- PF-26b transfer candidates to record at integration review: `internal/api/v1/files.go`, `internal/api/v1/files_test.go`; PF-26e transfer candidate: `internal/store/read_classification.go`. No outstanding reconciliation request was reported by PF-26a..PF-26f before this PF-27 lint run.
**Reconciliation requests resolved/routed**: None resolved in PF-27 because all lint diagnostics are outside PF-27-owned files. Integration review must route the listed lint findings, then resume the ordered final verification from line 1. Review-dependent CF closing notes, PF-26 split note, README index update, plan archival and formal review remain downstream and were not edited here.

### Session: 2026-10-21 (session 466, PF-27 resume)
**Tasks Completed**: Rechecked the existing attempt-2 lint report and confirmed its diagnostics remain outside PF-27's writePaths; no routed repairs or new review findings were supplied in this step. Ran the required final shared-tree compile check after resuming; `go build ./...` exited 0. The implementation handoff remains incomplete until integration routes and resolves the listed foreign-file lint findings, after which the ordered verification must restart from line 1.
**Verification**: `go build ./...` exit 0; `/tmp/pf27-s466-step6-resume-build.log`; copied to `tmp/punchflow-kickflow-dummy-s466/PF-27/attempt-3/build.log`.
**Reconciliation requests**: Existing attempt-2 `mise run lint` requests remain outstanding as detailed above; no additional request was discovered or resolved.


### Session: 2026-10-21 (session 467, PF-27 finalization)
**Intent snapshot**: Insert the CF-01..CF-09 and PF-26 split closing subsection in `design-docs/specs/notes.md`; replace the README plan index with 36 completed links, completed phase statuses, empty `PHASE_TO_PLANS`, and the s467 final manifest; archive 35 other plans and the s460/s462..s466 manifests; complete this plan and move it last. No Go, frontend source, module, toolchain, architecture, or other foreign file was edited.
**Pre-edit hashes**: `design-docs/specs/notes.md` `19c6cd06169cfb4a257b19249baa281d340759143d1956aaeea8ef2c1470e663`; `impl-plans/README.md` `33223b60ff38d3d0937ca83aaa4ac1e35e5ca05768e44d759bb836b751f75757`; `go.mod` `ee9df89830fb0ac16f7a7e121b3a71e93567fb5746ff161e2038bfe4d1e7de6f`; `go.sum` `b8a47c6606ec32aaa02f025d68e6494097cb2c9020ab114248307e402869030f`.
**Tasks Completed**: The ordered gate lines 1-16 passed in one uninterrupted foreground attempt (attempt 1). Archived 35 other plan files with Completed status and session entry; the superseded PF-14 and PF-26 indexes have their required wording. Moved s460, s462, s463, s464, s465, and s466 manifests byte-identically. Added the ten closing notes without deleting existing lines. Updated the README with all 36 completed rows; only the s467 manifest remains active. Pre-move line 17 passed, then this plan was completed and moved last.
**Verification**:
- Line 1 `bash -o pipefail -c '(cd web && bun install --frozen-lockfile && bun run typecheck && bun test && bun run build) 2>&1 | tee /tmp/pf27-s467-web.log'`: exit 0; Bun tests 25 pass, 0 fail; complete log `/tmp/pf27-s467-web.log`, copy `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/pf27-s467-web.log`.
- Line 2 `mise run build`: exit 0; complete log `/tmp/pf27-s467-mise-build.log`, copied to attempt-1.
- Line 3 `go mod tidy` and `git diff --quiet go.mod go.sum`: both exit 0; module files unchanged; `/tmp/pf27-s467-tidy.log`, copied to attempt-1.
- Line 4 `go mod verify`: exit 0, all modules verified; `/tmp/pf27-s467-modverify.log`, copied to attempt-1.
- Line 5 `go build ./...`: exit 0; `/tmp/pf27-s467-build.log`, copied to attempt-1.
- Line 6 `go vet ./...`: exit 0; `/tmp/pf27-s467-vet.log`, copied to attempt-1.
- Line 7 `go test -count=1 -v -race ./...`: exit 0; 793 PASS, 0 FAIL, 0 SKIP; complete `/tmp/pf27-s467-test.log`, copied to attempt-1.
- Line 8 counts: PASS=793, FAIL=0, SKIP=0, cached=0; exit 0; evidence `/tmp/pf27-s467-test.log` and `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/pf27-s467-gate-summary.log`.
- Line 9: every package with tests reports `ok`, no package `FAIL`; exit 0; package lines in `/tmp/pf27-s467-test.log` and gate-summary.log.
- Line 10: all 15 named top-level tests PASS; exit 0; `/tmp/pf27-s467-test.log` and gate-summary.log.
- Line 11 `mise run test` and cache check: exit 0, zero cached results; Bun tests 25 pass, 0 fail; `/tmp/pf27-s467-mise-test.log`, copied to attempt-1.
- Line 12 `mise run lint`: exit 0, golangci-lint reports 0 issues and typecheck completes; `/tmp/pf27-s467-mise-lint.log`, copied to attempt-1.
- Line 13 gofmt inventory: exit 0, no files listed; recorded in gate-summary.log.
- Line 14 CF-01 ban grep: underlying `rg` exit 1 as required (no matches), wrapper exit 0; recorded in gate-summary.log and the uninterrupted gate output.
- Lines 15-16 ignored build artifacts and foreign lockfiles: exit 0; recorded in gate-summary.log and the uninterrupted gate output.
- Pre-move lines 17, 21, 22, 23, 24: each exit 0; `go build ./...`; six manifests byte-identical; notes 0 deleted lines/10 bullets; README final manifest/no active links; no foreign edits. Complete `/tmp/pf27-s467-pre-move.log`, copied to attempt-1.
- Post-move lines 18, 19, 20, 25: run after the final plan move and recorded in the Step 6 report; 42 visible completed files, active contains only s467 manifest, 36 plan statuses Completed, final plan exists only in completed.
**Post-edit hashes**: `design-docs/specs/notes.md` `b738a1930909b0ab7a0d17855aee819609ba056eb35748a677fc625d3aaae6f7`; `impl-plans/README.md` `47ea4337a0f171f9f1718bcc62b9f4d655c97918ebec35c6492b070ba0f3a6c4`; `go.mod` and `go.sum` unchanged at the pre-edit hashes above.
**Reconciliation requests**: None. All gate lines passed. Formal integration/adversarial review, workflow-owned exact-file staging and commit remain downstream.
