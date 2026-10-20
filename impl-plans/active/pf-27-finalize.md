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

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`, except that this plan runs `go mod tidy` and builds `web/dist/app` and `./punchflow`. Record pre/post `shasum -a 256` for every shared file you edit. Do not commit (the workflow commits).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests resolved/routed**: -
