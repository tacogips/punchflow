# PF-27 Reconciliation and Finalization Implementation Plan

**Status**: Ready
**Wave**: 6 (serial) | **Depends On**: PF-01 to PF-26 | **Plan ID**: PF-27
**Design Reference**: design-docs/specs/architecture.md#testing-and-verification; impl-plans/README.md (Shared Execution Protocol); all acceptance criteria in the workflow input
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Run serially after every parallel plan. Apply the reconciliation requests that workers logged, tidy modules, run the complete verification suite (including the real web build embedded into the binary), record inferred decisions that surfaced during implementation, and archive the plans. This is the only plan allowed to touch frozen contracts and shared indexes after their owners.

## Non-goals

- No new features. Fix only what verification or reconciliation requests require, each traced to a request or a failing check.

## Files

New: `impl-plans/completed/` (archived plans). Generated artifacts (never source snapshots): `web/node_modules`, `web/dist/app`, `punchflow` (binary from `mise run build`). Everything else is an earlier plan's file, edited only when a logged reconciliation request or a failing check requires it (record pre/post hashes). `design-docs/specs/notes.md` only gets new INF rows for inferences logged by workers (e.g. PF-08 `fullName` format, PF-19 team deletion rule, PF-20 category deletion rule).

## Path Declarations

### writePaths
- `impl-plans/completed`
- `web/node_modules`
- `web/dist/app`
- `punchflow`

### sharedPaths
- `impl-plans/active`
- `impl-plans/README.md`
- `go.mod`
- `go.sum`
- `go-toolchain.json`
- `web/toolchain.json`
- `web/package.json`
- `web/bun.lock`
- `design-docs/specs/notes.md`
- `internal`
- `cmd`
- `fixtures`
- `deploy`
- `web/src`
- `embed.go`
- `embed_test.go`
- `fixtures_default_test.go`
- `README.md`
- `mise.toml`
- `.gitignore`

### sharedPathNotes
- `impl-plans/active`: all plans (PF-01..PF-27); set Status Completed with a final progress entry, then move each file to `impl-plans/completed`
- `impl-plans/README.md`: created before wave 1; update the plan tables and phase status after archiving
- `go.mod`: PF-02; `go mod tidy` only
- `go.sum`: PF-02; updated by `go mod tidy`
- `go-toolchain.json`: PF-02; append the tidy and final verify record (command, exit code, output)
- `web/toolchain.json`: PF-03; append the final frozen install and build record (commands, exit codes, bun.lock sha256)
- `web/package.json`: PF-03; edit only if a logged reconciliation request requires a dependency change
- `web/bun.lock`: PF-03; regenerated only together with a `web/package.json` reconciliation change
- `design-docs/specs/notes.md`: accepted design; append new INF rows only
- `internal`: Go packages from PF-01..PF-26; repair only per reconciliation request or failing check
- `cmd`: PF-01; repair only
- `fixtures`: PF-01/PF-07/PF-16; repair only
- `deploy`: PF-07; repair only
- `web/src`: PF-03/PF-11/PF-12/PF-13; repair only
- `embed.go`: PF-01; repair only
- `embed_test.go`: PF-01; repair only
- `fixtures_default_test.go`: PF-07; repair only
- `README.md`: PF-25; repair only
- `mise.toml`: PF-01; repair only
- `.gitignore`: PF-01; repair only

### artifactRoots
- `web/node_modules`
- `web/dist/app`
- `punchflow`

## Tasks

1. Collect every "Reconciliation requests" entry from `impl-plans/active/pf-*.md` into this plan's log; resolve each (edit, or justify "no change").
2. `go mod tidy`, then `go mod verify`; confirm all 5 modules are still required (each is imported now). Append both commands with exit codes to `go-toolchain.json`, and append the final `bun install --frozen-lockfile` / `bun run build` record (exit codes, bun.lock sha256) to `web/toolchain.json`.
3. Repo-wide `gofmt -l .` (must be empty) and `golangci-lint run ./...`; fix findings in the owning files.
4. Check the file-size rule: `find . -name '*.go' -not -path './web/*' | xargs wc -l | sort -n | tail` -> no file >= 1000 lines; split if needed.
5. Full verification (below). On failure, fix and re-run the whole list.
6. Append new INF entries to `notes.md` for inferred behavior logged by workers.
7. Archive: set each plan's Status to Completed with a final progress entry, move `impl-plans/active/pf-*.md` to `impl-plans/completed/`, update the README tables and phase status.

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 6 (serial, last). **Depends on**: every other plan.
**Design**: `architecture.md#verification-command-rule-session-462` (this is the only plan that runs the full-repo gate); `notes.md` CF-01..CF-03; `design-api-coverage.md` enum contract rule 3.

Additional tasks for session 462 (on top of tasks 1-7):

8. Close the carried findings:
   - CF-01: run the CF-01 ban grep in `design-docs/specs/notes.md` ("CF-01 ban grep" code block). It scans production `.go` files under `internal/`, `fixtures/` and `internal/fixtures/testdata/minimal`, and excludes `*_test.go` and `internal/fixtures/testdata/invalid/**`. It must print nothing. Then run the companion presence check from the same section: `rg -l 'audit_read' internal/fixtures/enum_rules_test.go internal/api/v1/roles_test.go` must list both files. Also run `rg -l 'ticket_submitted|comment_deleted' internal/admin` and confirm it lists the PF-24 webhook test file. Confirm that `internal/model/enums.go` defines none of the removed constants. Never delete, rename or obfuscate banned literals in negative tests to satisfy the grep; a hit in production code or fixtures is fixed in the owning file.
   - CF-02: confirm `TestExportMasterInputIDs` (PF-06), `TestPresentMasterItemInputIDs` (PF-08), the PF-20 input-ID tests and `TestDefaultRoundTrip` (PF-16) all PASS in the full run.
   - CF-03: confirm the PF-05 tests PASS in the full run.
   - Record the result per CF ID in the progress log. Update the "Carried review findings" table in `notes.md` only by appending a closing row or note per ID (sharedPath, append-only).
9. Accepted plans stay green: the full run includes PF-01's packages (`internal/kftime`, `internal/kferr`, `internal/build`, root `.`), and `bun test` covers PF-03/11/12/13. Record their PASS counts separately in the log.
10. Archive: all 27 plans plus `impl-plans/active/punchflow-kickflow-dummy-s460-dispatch.json` move to `impl-plans/completed/`. Update `impl-plans/README.md` (tables, phase status).

## Verification (all must exit 0; record exit status, log path and counts)

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf27-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./... 2>&1 | tee /tmp/pf27-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./... 2>&1 | tee /tmp/pf27-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf27-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf27-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf27-s462-test.log)"; grep -E '^(ok|FAIL|---)' /tmp/pf27-s462-test.log | grep -c '(cached)'   # last value must be 0
bash -o pipefail -c '(cd web && bun install --frozen-lockfile && bun run typecheck && bun test && bun run build) 2>&1 | tee /tmp/pf27-s462-web.log'; echo "exit=$?"   # writes web/dist/app; the whole chain is logged; record bun test's pass/fail summary line from this log
bash -o pipefail -c 'mise run build 2>&1 | tee /tmp/pf27-s462-mise-build.log'; echo "exit=$?"      # binary embeds web/dist/app
bash -o pipefail -c 'mise run test 2>&1 | tee /tmp/pf27-s462-mise-test.log'; echo "exit=$?"   # task-works check only (mise.toml runs cached `go test ./...`); NOT test evidence. The uncached go test line above is the evidence
bash -o pipefail -c 'mise run lint 2>&1 | tee /tmp/pf27-s462-mise-lint.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -race -run 'TestServeSmoke' ./internal/cli/... 2>&1 | tee /tmp/pf27-s462-smoke.log"; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'TestEmbeddedBuild' ./internal/webui/... 2>&1 | tee /tmp/pf27-s462-embed.log"; echo "exit=$?"   # after mise run build; must PASS, not SKIP
gofmt -l $(git ls-files '*.go')   # prints nothing
git status --porcelain web/dist/app | head -1   # prints nothing (output is git-ignored)
ls web/package-lock.json web/yarn.lock web/pnpm-lock.yaml 2>&1   # "No such file" for all three
```

Evidence for the full gate:
- `go test` exit 0, FAIL=0, zero `(cached)` lines.
- SKIP must be 0. Order: run the `web` line and the `mise run build` line first, then the remaining lines in the listed order. That way `web/dist/app` exists and `TestEmbeddedBuild` PASSes inside the full `go test` run as well.
- Every package of the module appears with `ok` in the log.

## Verification (session 460, superseded)

```bash
go build ./... 2>&1 | tee /tmp/pf27-build.log
go vet ./... 2>&1 | tee /tmp/pf27-vet.log
go test -race ./... 2>&1 | tee /tmp/pf27-test.log
cd web && bun install --frozen-lockfile && bun run typecheck && bun test && bun run build 2>&1 | tee /tmp/pf27-web.log   # writes web/dist/app
mise run build 2>&1 | tee /tmp/pf27-mise-build.log      # binary embeds web/dist/app
mise run test 2>&1 | tee /tmp/pf27-mise-test.log
mise run lint 2>&1 | tee /tmp/pf27-mise-lint.log
go test -race -run TestServeSmoke ./internal/cli/... 2>&1 | tee /tmp/pf27-smoke.log
git status --porcelain web/dist/app | head -1   # must print nothing (output is git-ignored)
ls web/package-lock.json web/yarn.lock web/pnpm-lock.yaml 2>&1   # "No such file" for all three
```

Embedded UI check (foreground, bounded, no background process): after `mise run build`, run `go test -count=1 -v -run TestEmbeddedBuild ./internal/webui/... 2>&1 | tee /tmp/pf27-embed.log`. The test is written by PF-16; it must report PASS, not SKIP.

Manual check left to the user (documented in README): Keycloak login via `mise run keycloak:up`.

## Done Criteria

- [ ] All reconciliation requests resolved and listed
- [ ] Every session-462 verification command passed in one final run, with exit status, log path and counts recorded (no cached output)
- [ ] CF-01, CF-02 and CF-03 closed with the evidence named in task 8; the CF-01 ban grep (production code + fixtures; tests excluded) prints nothing, and the negative-test presence checks list the PF-02/PF-21/PF-24 test files
- [ ] `notes.md` updated with new inferences; all plans moved to `impl-plans/completed/`; README index updated

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`, except that this plan may edit frozen contracts and run `go mod tidy`. Do not commit (the workflow commits).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests resolved**: -
