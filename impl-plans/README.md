# Implementation Plans

This directory contains implementation plans that translate design documents into actionable implementation specifications.

## Purpose

Implementation plans bridge design documents (what to build) and actual code (how to build). They provide:
- Clear deliverables without code
- Interface and function specifications
- Dependency mapping for concurrent execution
- Progress tracking across sessions

## Directory Structure

```
impl-plans/
├── README.md              # This file
├── active/                # Currently active implementation plans
│   └── <feature>.md       # One file per feature being implemented
├── completed/             # Completed implementation plans (archive)
│   └── <feature>.md       # Completed plans for reference
└── templates/             # Plan templates
    └── plan-template.md   # Standard plan template
```

## File Size Limits

**IMPORTANT**: Implementation plan files must stay under 400 lines to prevent OOM errors.

| Metric | Limit |
|--------|-------|
| Line count | MAX 1000 lines |
| Modules per plan | MAX 8 modules |
| Tasks per plan | MAX 10 tasks |

Large features are split into multiple related plans with cross-references.

## Active Plans (punchflow dummy kickflow server)

Design source of truth: `design-docs/specs/architecture.md` and the documents it links (coverage matrix `design-docs/specs/design-api-coverage.md`, INF/DIV register `design-docs/specs/notes.md`).

Session 462 continuation (from checkpoint commit 410d3d8). PF-01, PF-03, PF-11, PF-12 and PF-13 were accepted in session 460. They are external dependencies: do not re-implement them, and keep them green. Every other plan carries a "Continuation (session 462)" section. That section overrides the plan's older Verification and Done Criteria text. For phase and dependencies, this table and the Continuation section are authoritative over the `**Wave**`/`**Depends On**` header line.

| Plan | Phase (s462) | Depends On (s462) | Status |
|------|------|------------|--------|
| [pf-01-foundation-primitives](./active/pf-01-foundation-primitives.md) | accepted | - | Accepted (410d3d8) |
| [pf-02-data-contract](./active/pf-02-data-contract.md) | 1 | - | In Progress |
| [pf-03-web-scaffold](./active/pf-03-web-scaffold.md) | accepted | - | Accepted (410d3d8) |
| [pf-04-httpx](./active/pf-04-httpx.md) | 2 | PF-02 | In Progress (re-verify) |
| [pf-05-auth-core](./active/pf-05-auth-core.md) | 2 | PF-02 | In Progress (re-verify, CF-03) |
| [pf-06-fixtures-loader](./active/pf-06-fixtures-loader.md) | 2 | PF-02 | In Progress |
| [pf-07-fixtures-default-keycloak](./active/pf-07-fixtures-default-keycloak.md) | 2 | PF-02 | In Progress |
| [pf-08-present-core](./active/pf-08-present-core.md) | 2 | PF-02 | In Progress |
| [pf-09-webhook-delivery](./active/pf-09-webhook-delivery.md) | 2 | PF-02 | In Progress (re-verify) |
| [pf-10-api-contracts](./active/pf-10-api-contracts.md) | 2 | PF-02 | In Progress (re-verify) |
| [pf-11-web-tickets](./active/pf-11-web-tickets.md) | accepted | - | Accepted (410d3d8) |
| [pf-12-web-readviews](./active/pf-12-web-readviews.md) | accepted | - | Accepted (410d3d8) |
| [pf-13-web-integration](./active/pf-13-web-integration.md) | accepted | - | Accepted (410d3d8) |
| [pf-14-engine](./active/pf-14-engine.md) | 3 | PF-06, PF-07, PF-08, PF-10 | Ready |
| [pf-15-present-ticket-payloads](./active/pf-15-present-ticket-payloads.md) | 3 | PF-06, PF-07, PF-08, PF-09 | Ready |
| [pf-16-server-cli-testutil](./active/pf-16-server-cli-testutil.md) | 3 | PF-04, PF-05, PF-06, PF-07, PF-08, PF-09, PF-10 | Ready |
| [pf-17-auth-oidc](./active/pf-17-auth-oidc.md) | 3 | PF-05 | Ready |
| [pf-18-api-users-grades](./active/pf-18-api-users-grades.md) | 4 | PF-08, PF-16 | Ready |
| [pf-19-api-proxies-org](./active/pf-19-api-proxies-org.md) | 4 | PF-08, PF-16 | Ready |
| [pf-20-api-definitions-masters](./active/pf-20-api-definitions-masters.md) | 4 | PF-06, PF-08, PF-16 | Ready |
| [pf-21-api-roles-audit](./active/pf-21-api-roles-audit.md) | 4 | PF-08, PF-16 | Ready |
| [pf-22-api-tickets](./active/pf-22-api-tickets.md) | 4 | PF-14, PF-15, PF-16 | Ready |
| [pf-23-api-comments-files](./active/pf-23-api-comments-files.md) | 4 | PF-14, PF-15, PF-16 | Ready |
| [pf-24-admin-api](./active/pf-24-admin-api.md) | 4 | PF-14, PF-15, PF-16 | Ready |
| [pf-25-readme](./active/pf-25-readme.md) | 4 | PF-07, PF-16 | Ready |
| [pf-26-contract-smoke](./active/pf-26-contract-smoke.md) | 5 | PF-17 to PF-24 | Ready |
| [pf-27-finalize](./active/pf-27-finalize.md) | 6 | all | Ready |

`punchflow-kickflow-dummy-s460-dispatch.json` is the historical session-460 dispatch manifest. It is not the source for the session-462 waves; PF-27 archives it with the plans.

## Completed Plans

| Plan | Completed | Design Reference |
|------|-----------|------------------|
| (No completed plans yet) | - | - |

## Phase Dependencies (for impl-exec-auto)

**IMPORTANT**: This section is used by impl-exec-auto to determine which plans to load.
Only plans from eligible phases should be read to minimize context loading.

### Phase Status

| Phase | Status | Depends On |
|-------|--------|------------|
| 1 | READY | - |
| 2 | BLOCKED | Phase 1 |
| 3 | BLOCKED | Phase 2 |
| 4 | BLOCKED | Phase 3 |
| 5 | BLOCKED | Phase 4 |
| 6 | BLOCKED | Phase 5 |

### Phase to Plans Mapping

```
PHASE_TO_PLANS = {   # session 462 continuation; accepted PF-01/03/11/12/13 are not dispatched
  1: ["pf-02-data-contract.md"],
  2: ["pf-04-httpx.md", "pf-05-auth-core.md", "pf-06-fixtures-loader.md", "pf-07-fixtures-default-keycloak.md",
      "pf-08-present-core.md", "pf-09-webhook-delivery.md", "pf-10-api-contracts.md"],
  3: ["pf-14-engine.md", "pf-15-present-ticket-payloads.md", "pf-16-server-cli-testutil.md", "pf-17-auth-oidc.md"],
  4: ["pf-18-api-users-grades.md", "pf-19-api-proxies-org.md", "pf-20-api-definitions-masters.md",
      "pf-21-api-roles-audit.md", "pf-22-api-tickets.md", "pf-23-api-comments-files.md",
      "pf-24-admin-api.md", "pf-25-readme.md"],
  5: ["pf-26-contract-smoke.md"],
  6: ["pf-27-finalize.md"]
}
```

## Shared Execution Protocol (applies to every pf-* plan)

All plans of a wave run at the same time on the same branch and working directory.

1. **Ownership**: every plan has a `## Path Declarations` section with four lists (`writePaths`, `sharedPaths`, `sharedPathNotes`, `artifactRoots`) of concrete repository-relative paths. That section is authoritative over any prose or "Files" table. Edit only paths listed in `writePaths` (created or exclusively owned by this plan) and `sharedPaths` (created by an earlier-wave plan; the matching `sharedPathNotes` entry says what to change). Every other path in the repository is read-only for you. If anything else needs a change, do not edit it; add it under "Reconciliation requests" in your own progress log.
   `artifactRoots` (generated installs, build output, binaries such as `web/node_modules`, `web/dist/app`, `punchflow`) are never source snapshots: do not commit, diff or hand-edit them. Tool versions, install commands, exit codes and digests go into the tracked audit manifests `go-toolchain.json` (PF-02) and `web/toolchain.json` (PF-03).
2. **Fresh reads and hashes**: before editing a file that already exists, read it fresh and record `shasum -a 256 <file>` in your progress log. Record post-edit hashes when you finish.
3. **Intent snapshot**: before the first edit, write a short "Intent snapshot" in your progress log listing the files you will create or change and why. Do not change it afterwards; append corrections below it.
4. **Drift detection**: before each later edit to a shared path, re-hash it. If the hash differs from your last recorded value, re-read it and re-apply your intent on top. Never revert someone else's change.
5. **Frozen contracts**: `go.mod`, `go.sum`, `internal/model/*`, `internal/fixtures/types.go`, `web/package.json` and `web/bun.lock` change only in their owning plan (PF-02 or PF-03) and in PF-27. Later plans log missing fields or dependencies as reconciliation requests and use a local workaround.
6. **No git state changes**: no commit, stash, checkout, reset, rebase or branch commands. No worktrees. No `go mod tidy` (except PF-27). No repo-wide formatting: run `gofmt -l` / `gofmt -w` only on your own Go files.
7. **Verification isolation**: never write the `./punchflow` binary or `web/dist/app` from a wave-2+ worker. Use `go build ./...` and build the web app into a temp `--outDir`.
8. **Progress log**: update only your plan's Progress Log. Record every verification command, its exit status and the log path (`/tmp/<plan-id>-<step>.log`). An incomplete log is not a passing check.
9. **Go standards**: follow `.claude/skills/go-coding-standards` (error wrapping with `%w`, small consumer-side interfaces, files under 1000 lines).
10. **Identifier collisions in shared packages**: parallel plans add files to the same Go packages (`internal/store`, `internal/present`, `internal/engine`, `internal/webhook`, `internal/auth`, `internal/admin`, `internal/api/v1`, and the external test packages `v1_test` and `admin_test`). Export only the names your plan's contract lists, and prefix every other package-level identifier you add (unexported helpers, test helpers, test-only types, constants) with your plan id, e.g. `pf22CreateTicket`, `pf05scanSession`. Test function names may stay `TestXxx` but must start with your resource noun (they are matched by your `-run` pattern). Never define `TestMain` or `init` in a shared package. Exported `internal/store` functions that a plan adds beyond its listed contract must start with that plan's resource noun (e.g. `ListUsers`, `InsertGrade`, `TicketQueryList`); PF-14 prefixes its store functions with `Eng` (e.g. `EngLoadTicket`). Before adding one, grep `internal/store` and reuse an existing function instead of duplicating it. A collision found at join time is repaired serially in PF-27.
11. **Verification evidence (session 462; design: `design-docs/specs/architecture.md#verification-command-rule-session-462`)**. Each rule exists because the previous run broke it:
    - Run every command that pipes into `tee` through `bash -o pipefail -c '<cmd> 2>&1 | tee /tmp/<plan-id>-s462-<step>.log'; echo "exit=$?"`. Record that `exit=` value. Do not use a shell variable named `status` (zsh treats it as read-only, which broke session-460 logs).
    - Go tests are always `go test -count=1 -v [-race] [-run '<pattern>'] <packages>`. Output marked `(cached)` is not evidence.
    - After each Go test log, record three counts: `grep -c -- '--- PASS' <log>`, `grep -c -- '--- FAIL' <log>` and `grep -c -- '--- SKIP' <log>`. A pass needs exit 0, FAIL 0 and PASS > 0. A test the plan requires to PASS must not SKIP. For `bun test`, record its `pass`/`fail` summary line.
    - Test only your own packages, or your own `-run` pattern inside a shared package. Only PF-27 runs `go test ./...`. `go build ./...` is allowed in every plan.
    - If a scoped command fails because of a file owned by another plan, record `file:line`, the owning plan and the log path under "Reconciliation requests". Do not edit that file. Re-run once that plan has landed (same wave) or leave it for PF-27.
    - A progress-log entry must state exit status, log path and counts for every command in the plan's Verification section. An entry missing any of these is incomplete, and the plan is not done.

## Workflow

### Creating a New Plan

1. Use the `/impl-plan` command with a design document reference
2. Or manually create a plan using `templates/plan-template.md`
3. Save to `active/<feature-name>.md`
4. Update this README with the new plan entry
5. **IMPORTANT**: If plan exceeds 400 lines, split into multiple files

### Working on a Plan

1. Read the active plan
2. Select a subtask to work on (consider parallelization)
3. Implement following the deliverable specifications
4. Update task status and progress log
5. Mark completion criteria as done

### Completing a Plan

1. Verify all completion criteria are met
2. Update status to "Completed"
3. Move file from `active/` to `completed/`
4. Update this README

## Guidelines

- Plans contain NO implementation code
- Plans specify interfaces, functions, and file structures
- Subtasks should be as independent as possible for parallel execution
- Always update progress log after each session
- **Keep each plan file under 400 lines** - split if necessary
