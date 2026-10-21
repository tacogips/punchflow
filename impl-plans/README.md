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

All plans are completed and archived in session 467. `active/` holds only `punchflow-kickflow-dummy-s467-dispatch.json`, the final manifest. The s460 and s462..s466 manifests are in `completed/`.

## Completed Plans

| Plan | Completed | Design Reference |
|------|-----------|------------------|
| [pf-01-foundation-primitives](./completed/pf-01-foundation-primitives.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-02-data-contract](./completed/pf-02-data-contract.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-03-web-scaffold](./completed/pf-03-web-scaffold.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-04-httpx](./completed/pf-04-httpx.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-05-auth-core](./completed/pf-05-auth-core.md) | 2026-10-21 (s467) | `design-docs/specs/design-auth-admin.md` |
| [pf-06-fixtures-loader](./completed/pf-06-fixtures-loader.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-07-fixtures-default-keycloak](./completed/pf-07-fixtures-default-keycloak.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-08-present-core](./completed/pf-08-present-core.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-09-webhook-delivery](./completed/pf-09-webhook-delivery.md) | 2026-10-21 (s467) | `design-docs/specs/design-webhooks.md` |
| [pf-10-api-contracts](./completed/pf-10-api-contracts.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-11-web-tickets](./completed/pf-11-web-tickets.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-12-web-readviews](./completed/pf-12-web-readviews.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-13-web-integration](./completed/pf-13-web-integration.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-14-engine](./completed/pf-14-engine.md) | Superseded (archived s467) | `design-docs/specs/design-approval-engine.md` |
| [pf-14a-engine-lifecycle](./completed/pf-14a-engine-lifecycle.md) | 2026-10-21 (s467) | `design-docs/specs/design-approval-engine.md` |
| [pf-14b-engine-routing](./completed/pf-14b-engine-routing.md) | 2026-10-21 (s467) | `design-docs/specs/design-approval-engine.md` |
| [pf-14c-engine-forms-create](./completed/pf-14c-engine-forms-create.md) | 2026-10-21 (s467) | `design-docs/specs/design-approval-engine.md` |
| [pf-15-present-ticket-payloads](./completed/pf-15-present-ticket-payloads.md) | 2026-10-21 (s467) | `design-docs/specs/design-webhooks.md` |
| [pf-16-server-cli-testutil](./completed/pf-16-server-cli-testutil.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-17-auth-oidc](./completed/pf-17-auth-oidc.md) | 2026-10-21 (s467) | `design-docs/specs/design-auth-admin.md` |
| [pf-18-api-users-grades](./completed/pf-18-api-users-grades.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-19-api-proxies-org](./completed/pf-19-api-proxies-org.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-20-api-definitions-masters](./completed/pf-20-api-definitions-masters.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-21-api-roles-audit](./completed/pf-21-api-roles-audit.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-22-api-tickets](./completed/pf-22-api-tickets.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-23-api-comments-files](./completed/pf-23-api-comments-files.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-24-admin-api](./completed/pf-24-admin-api.md) | 2026-10-21 (s467) | `design-docs/specs/design-auth-admin.md` |
| [pf-25-readme](./completed/pf-25-readme.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [pf-26-contract-smoke](./completed/pf-26-contract-smoke.md) | Superseded (archived s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26a-contract-tickets](./completed/pf-26a-contract-tickets.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26b-contract-comments-files](./completed/pf-26b-contract-comments-files.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26c-contract-users-grades](./completed/pf-26c-contract-users-grades.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26d-contract-orgcharts-teams](./completed/pf-26d-contract-orgcharts-teams.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26e-contract-masters](./completed/pf-26e-contract-masters.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-26f-contract-proxies-roles](./completed/pf-26f-contract-proxies-roles.md) | 2026-10-21 (s467) | `design-docs/specs/design-api-coverage.md` |
| [pf-27-finalize](./completed/pf-27-finalize.md) | 2026-10-21 (s467) | `design-docs/specs/architecture.md` |
| [engine-lifecycle-review-fixes](./completed/engine-lifecycle-review-fixes.md) | 2026-10-21 | `design-docs/specs/design-approval-engine.md`, `design-webhooks.md` |

## Phase Dependencies (for impl-exec-auto)

**IMPORTANT**: This section is used by impl-exec-auto to determine which plans to load.
Only plans from eligible phases should be read to minimize context loading.

### Phase Status

| Phase | Status | Depends On |
|-------|--------|------------|
| A | COMPLETED | - |
| B | COMPLETED | A |

### Phase to Plans Mapping

```
PHASE_TO_PLANS = {}  # all plans completed (session 467); final manifest punchflow-kickflow-dummy-s467-dispatch.json
```

## Shared Execution Protocol (applies to every pf-* plan)

All plans of a wave run at the same time on the same branch and working directory.

1. **Ownership**: every plan has a `## Path Declarations` section with four lists (`writePaths`, `sharedPaths`, `sharedPathNotes`, `artifactRoots`) of concrete repository-relative paths. That section is authoritative over any prose or "Files" table. Edit only paths listed in `writePaths` (created or exclusively owned by this plan) and `sharedPaths` (created by an earlier-wave plan; the matching `sharedPathNotes` entry says what to change). Every other path in the repository is read-only for you. If anything else needs a change, do not edit it; add it under "Reconciliation requests" in your own progress log.
   `artifactRoots` (generated installs, build output, binaries such as `web/node_modules`, `web/dist/app`, `punchflow`) are never source snapshots: do not commit, diff or hand-edit them. Tool versions, install commands, exit codes and digests go into the tracked audit manifests `go-toolchain.json` (PF-02) and `web/toolchain.json` (PF-03).
2. **Fresh reads and hashes**: before editing a file that already exists, read it fresh and record `shasum -a 256 <file>` in your progress log. Record post-edit hashes when you finish.
3. **Intent snapshot**: before the first edit, write a short "Intent snapshot" in your progress log listing the files you will create or change and why. Do not change it afterwards; append corrections below it.
4. **Drift detection**: before each later edit to a shared path, re-hash it. If the hash differs from your last recorded value, re-read it and re-apply your intent on top. Never revert someone else's change.
5. **Frozen contracts (session 463)**:
   - `internal/model/*` and `internal/fixtures/types.go` change only through a PF-02 repair route.
   - `web/package.json` and `web/bun.lock` change only through a PF-03 repair route.
   - `go.mod` and `go.sum` change only through PF-27's `go mod tidy`, or through a PF-02 repair route.

   Later plans log missing fields or dependencies as reconciliation requests and use a local workaround. PF-27 does not edit model, fixture-type or web dependency files.
6. **No git state changes**: no commit, stash, checkout, reset, rebase or branch commands. No worktrees. No `go mod tidy` (except PF-27). No repo-wide formatting: run `gofmt -l` / `gofmt -w` only on your own Go files.
7. **Verification isolation**: never write the `./punchflow` binary or `web/dist/app` from a wave-2+ worker. Use `go build ./...` and build the web app into a temp `--outDir`.
8. **Progress log**: update only your plan's Progress Log. Record every verification command, its exit status and the log path (`/tmp/<plan-id>-<step>.log`). An incomplete log is not a passing check.
9. **Go standards**: follow `.claude/skills/go-coding-standards` (error wrapping with `%w`, small consumer-side interfaces, files under 1000 lines).
10. **Identifier collisions in shared packages**: parallel plans add files to the same Go packages (`internal/store`, `internal/present`, `internal/engine`, `internal/webhook`, `internal/auth`, `internal/admin`, `internal/api/v1`, and the external test packages `v1_test` and `admin_test`). Export only the names your plan's contract lists, and prefix every other package-level identifier you add (unexported helpers, test helpers, test-only types, constants) with your plan id, e.g. `pf22CreateTicket`, `pf05scanSession`. Test function names may stay `TestXxx` but must start with your resource noun (they are matched by your `-run` pattern). Never define `TestMain` or `init` in a shared package. Exported `internal/store` functions that a plan adds beyond its listed contract must start with that plan's resource noun (e.g. `ListUsers`, `InsertGrade`, `TicketQueryList`); PF-14 prefixes its store functions with `Eng` (e.g. `EngLoadTicket`). Before adding one, grep `internal/store` and reuse an existing function instead of duplicating it. A collision found at join time is repaired by the plan whose identifier breaks this prefix rule. If both identifiers follow the rule, the later-wave plan renames its identifier. Integration review routes the repair; PF-27 never renames identifiers.
11. **Verification evidence (sessions 462 and 463; design: `design-docs/specs/architecture.md#verification-command-rule-session-462`)**. Each rule exists because the previous run broke it:
    - Run every command that pipes into `tee` through `bash -o pipefail -c '<cmd> 2>&1 | tee /tmp/<plan-id>-s463-<step>.log'; echo "exit=$?"`. Record that `exit=` value. Session-462 logs (`-s462-`) are never evidence in session 463. Copy each log to `tmp/punchflow-kickflow-dummy-s463/<planId>/attempt-<n>/`. Do not use a shell variable named `status` (zsh treats it as read-only, which broke session-460 logs).
    - Go tests are always `go test -count=1 -v [-race] [-run '<pattern>'] <packages>`. Output marked `(cached)` is not evidence.
    - After each Go test log, record three counts: `grep -c -- '--- PASS' <log>`, `grep -c -- '--- FAIL' <log>` and `grep -c -- '--- SKIP' <log>`. A pass needs exit 0, FAIL 0 and PASS > 0. A test the plan requires to PASS must not SKIP. For `bun test`, record its `pass`/`fail` summary line.
    - Test only your own packages, or your own `-run` pattern inside a shared package. Only PF-27 runs `go test ./...`. `go build ./...` is allowed in every plan.
    - If a scoped command fails because of a file owned by another plan, record `file:line`, the owning plan and the log path under "Reconciliation requests". Do not edit that file. Re-run once that plan has landed (same wave). Otherwise integration review routes the repair to the owning plan (rule 12). PF-27 never repairs another plan's files.
    - A progress-log entry must state exit status, log path and counts for every command in the plan's Verification section. An entry missing any of these is incomplete, and the plan is not done.
12. **Ownership index and repair routing (session 463; design: architecture.md "Path-ownership and fanout rule (session 463)" items 4-5)**:
    - Each path has one owner. Accepted plans take their path declarations from `punchflow-kickflow-dummy-s460-dispatch.json` (PF-01, PF-03, PF-11, PF-12, PF-13) or `punchflow-kickflow-dummy-s462-dispatch.json` (PF-02, PF-04..PF-10, PF-17). Remaining plans take theirs from `punchflow-kickflow-dummy-s463-dispatch.json`.
    - When a path is in one plan's `writePaths` and another plan's `sharedPaths`, the `writePaths` plan owns it.
    - A reconciliation request names `file:line`, the owner from this index, the failing command and the log path.
    - Integration review routes the repair:
      - remaining plan: back to its implementer, within its declared paths;
      - accepted plan: a repair item limited to the named files, under that plan's declaring-manifest paths and scoped verification, followed by re-acceptance.
    - No plan's expansion (`writePaths` + `sharedPaths` minus `artifactRoots`) may reach 300 files.
    - **Session 464 override** (design: architecture.md "Shared-tree compile and ownership-transfer rule (session 464)" items 4-7): the active manifest is `punchflow-kickflow-dummy-s464-dispatch.json`. Its `ownershipTransfers[]` (43 files) are resolved first and override the s460/s462 declarations. Every file a remaining plan edits is in that plan's own `writePaths`. All remaining plans except PF-27 have empty `sharedPaths`. The tie-break above therefore matters only for PF-27's archival and doc files. Accepted commit: `44e40cc`. Logs: `/tmp/<plan-id>-s464-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s464/<planId>/attempt-<n>/`. Anything `-s462-`, `-s463-` or under `tmp/punchflow-kickflow-dummy-s463/` is history, not evidence.
13. **Shared-tree compile rule (session 464; design: architecture.md "Shared-tree compile and ownership-transfer rule (session 464)" items 1-3; CF-06)**. Session 463 failed because two plans left the shared tree uncompilable for their siblings.
    - Define before reference. A new type or function is written completely, in a file that compiles, before any file that references it is saved. Never save a file that names a symbol that does not exist yet.
    - Compile gate:
      - After every write of a non-test `.go` file, run `go build ./...`.
      - After every write of a `_test.go` file, run `go vet <that package>`.
      - Record each result (exit status) as one line in your progress log.
      - If the failure is in a file you just wrote, fix it before any other edit.
    - Never start a verification step, or end the turn, while a file of yours breaks the build.
    - Contract stubs keep every exported identifier and signature; only bodies change. That covers `internal/engine/engine.go`, `internal/webhook/payload.go`, `internal/files/files.go`, the `internal/api/v1` handler stubs and the `internal/admin` stubs.
    - A compile failure in a file owned by another in-flight plan is checked again at your next verification step. It becomes a reconciliation request (rule 11) only if it is still there at your final verification. Never edit that file.
14. **Evidence reporting (session 465; design: architecture.md "Plan split, evidence-reporting and wave rule (session 465)" item 3; CF-08)**. Every plan's Execution Protocol repeats this rule. Session 464 broke it, and the progress gate then ended the run.
    1. The `verification` array holds only commands that are required to pass, with exit 0, PASS > 0 and FAIL = 0 for test commands. An intentional negative or mutation proof (an expected non-zero exit: an `rg` ban grep that finds nothing and exits 1, a mutated test that must fail, a request that must be rejected) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path. In the s465 manifest, "prints nothing" checks are written in must-pass form (`<rg ...>; test $? -eq 1`, `test -z "$(gofmt -l ...)"`). Those forms may stay in `verification`, because their required result is exit 0.
    2. When assigned work remains, the report sets `implementationIncomplete: true` and lists the remaining items in `implPlanUpdates` and in the plan's Progress Log. Remaining work is never reported as a High/Medium finding, a risk or a residual risk.
    3. High and Medium labels are reserved for defects in delivered code, each with `file:line`. A sibling-breakage reconciliation request (rule 13) is reported as a blocker naming the owning plan, not as a finding against the reporting plan.
    4. Every attempt leaves compiling, incremental progress: `go build ./...` exits 0 at the end of the turn, and the Progress Log records what was completed.
15. **Session 465 override** (design: architecture.md verification-rule item 9 and "Plan split, evidence-reporting and wave rule (session 465)"):
    - The active manifest is `punchflow-kickflow-dummy-s465-dispatch.json`. It supersedes s464, and its `ownershipTransfers[]` (58 files) are resolved first.
    - Accepted commit: `ea385c7`. Logs: `/tmp/<plan-id>-s465-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s465/<planId>/attempt-<n>/`. Anything named `-s462-`, `-s463-` or `-s464-`, and anything under `tmp/punchflow-kickflow-dummy-s46{3,4}/`, is history, not evidence.
    - Each plan's "Continuation (session 465)" section overrides its earlier continuation sections for wave, dependencies, log names and Done Criteria.
    - PF-14 is replaced by PF-14a (`pf-14a-engine-lifecycle.md`), PF-14b (`pf-14b-engine-routing.md`) and PF-14c (`pf-14c-engine-forms-create.md`). They have disjoint files in `internal/engine` and `internal/store`. The cross-file helpers they share are frozen (design-approval-engine.md "Plan ownership (session 465)").
    - A plan is dispatched as soon as every plan in its `dependsOn` is accepted.
16. **Session 466 override (final run)** (design: architecture.md "PF-26 split and final-run rule (session 466)"; design-api-coverage.md "Live scenario partition (session 466)"):
    - The active manifest is `punchflow-kickflow-dummy-s466-dispatch.json`. It supersedes s465. Its `ownershipTransfers[]` carries the s465 list forward unchanged; integration review appends one entry per repair-eligible file a PF-26 sub-plan actually edits.
    - Accepted commit: `a2a82de`, for PF-01..PF-25 including PF-14a/b/c. Logs are `/tmp/<plan-id>-s466-<step>.log`, copied to `tmp/punchflow-kickflow-dummy-s466/<planId>/attempt-<n>/`. Older logs are history.
    - Wave A: PF-26a..PF-26f, in parallel, each with pairwise-disjoint `writePaths` and no `sharedPaths`. Wave B: PF-27.
    - Frozen during wave A (no plan edits them): `internal/contract/helpers_test.go`, `spec_test.go`, `routes_test.go`, `webhooks_test.go`, `internal/cli/serve_smoke_test.go`. `scenarios_users_org_test.go` may be edited only by PF-26c, and only for the `liveReadScenarios` hoist.
    - Repair-eligible files in a sub-plan's `writePaths` are edited only to fix a defect that one of its own scenarios exposes. Each edit needs a transfer-candidate record in the progress log and an uncached package guard. A defect in another file is a reconciliation request (README rule 11) plus `implementationIncomplete: true`.
    - Identifier prefixes in `package contract_test`: `pf26a`..`pf26f`, and `pf27` for PF-27's coverage test. The only unprefixed names are the six `TestContractLive<Group>` functions, the six `live<Group>Operations` variables and `liveReadScenarios`.

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
