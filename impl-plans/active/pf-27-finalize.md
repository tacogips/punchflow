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

## Verification (all must exit 0; record exit status and log path)

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
- [ ] Every verification command passed in one final run, with logs recorded
- [ ] `notes.md` updated with new inferences; all plans moved to `impl-plans/completed/`; README index updated

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`, except that this plan may edit frozen contracts and run `go mod tidy`. Do not commit (the workflow commits).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests resolved**: -
