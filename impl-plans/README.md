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

Session 463 continuation (from checkpoint commit f82e374). The session-462 integration review accepted PF-01..PF-13 and PF-17. They are external dependencies (`acceptedDependencies` in `punchflow-kickflow-dummy-s463-dispatch.json`): do not re-implement or re-dispatch them, and keep them green. The 13 remaining plans each carry a "Continuation (session 463)" section. It overrides the session-462 log names, wave and Done Criteria. The tasks, tests, Path Declarations and session-462 corrections stay authoritative unless the s463 section says otherwise. For wave and dependencies, this table and the s463 section are authoritative over the `**Wave**`/`**Depends On**` header line. Design: `design-docs/specs/architecture.md`, "Path-ownership and fanout rule (session 463)" and "Continuation order (session 463)".

| Plan | Wave (s463) | Depends On | Status |
|------|------|------------|--------|
| [pf-01-foundation-primitives](./active/pf-01-foundation-primitives.md) | accepted | - | Accepted (f82e374) |
| [pf-02-data-contract](./active/pf-02-data-contract.md) | accepted | - | Accepted (f82e374) |
| [pf-03-web-scaffold](./active/pf-03-web-scaffold.md) | accepted | - | Accepted (f82e374) |
| [pf-04-httpx](./active/pf-04-httpx.md) | accepted | - | Accepted (f82e374) |
| [pf-05-auth-core](./active/pf-05-auth-core.md) | accepted | - | Accepted (f82e374) |
| [pf-06-fixtures-loader](./active/pf-06-fixtures-loader.md) | accepted | - | Accepted (f82e374) |
| [pf-07-fixtures-default-keycloak](./active/pf-07-fixtures-default-keycloak.md) | accepted | - | Accepted (f82e374) |
| [pf-08-present-core](./active/pf-08-present-core.md) | accepted | - | Accepted (f82e374) |
| [pf-09-webhook-delivery](./active/pf-09-webhook-delivery.md) | accepted | - | Accepted (f82e374) |
| [pf-10-api-contracts](./active/pf-10-api-contracts.md) | accepted | - | Accepted (f82e374) |
| [pf-11-web-tickets](./active/pf-11-web-tickets.md) | accepted | - | Accepted (f82e374) |
| [pf-12-web-readviews](./active/pf-12-web-readviews.md) | accepted | - | Accepted (f82e374) |
| [pf-13-web-integration](./active/pf-13-web-integration.md) | accepted | - | Accepted (f82e374) |
| [pf-14-engine](./active/pf-14-engine.md) | A | PF-06, PF-07, PF-08, PF-10 (accepted) | Ready |
| [pf-15-present-ticket-payloads](./active/pf-15-present-ticket-payloads.md) | A | PF-06, PF-07, PF-08, PF-09 (accepted) | Ready |
| [pf-16-server-cli-testutil](./active/pf-16-server-cli-testutil.md) | A | PF-04..PF-10 (accepted) | Ready |
| [pf-17-auth-oidc](./active/pf-17-auth-oidc.md) | accepted | - | Accepted (f82e374) |
| [pf-18-api-users-grades](./active/pf-18-api-users-grades.md) | B | PF-08, PF-16 | Ready |
| [pf-19-api-proxies-org](./active/pf-19-api-proxies-org.md) | B | PF-08, PF-16 | Ready |
| [pf-20-api-definitions-masters](./active/pf-20-api-definitions-masters.md) | B | PF-06, PF-08, PF-16 | Ready |
| [pf-21-api-roles-audit](./active/pf-21-api-roles-audit.md) | B | PF-08, PF-16 | Ready |
| [pf-22-api-tickets](./active/pf-22-api-tickets.md) | B | PF-14, PF-15, PF-16 | Ready |
| [pf-23-api-comments-files](./active/pf-23-api-comments-files.md) | B | PF-14, PF-15, PF-16 | Ready |
| [pf-24-admin-api](./active/pf-24-admin-api.md) | B | PF-14, PF-15, PF-16 | Ready |
| [pf-25-readme](./active/pf-25-readme.md) | B | PF-07, PF-16 | Ready |
| [pf-26-contract-smoke](./active/pf-26-contract-smoke.md) | C | PF-17 to PF-24 | Ready |
| [pf-27-finalize](./active/pf-27-finalize.md) | D (serial, last) | all | Ready |

Dispatch manifests:
- `punchflow-kickflow-dummy-s463-dispatch.json` is the current manifest. It stays in `active/` during the run (design D-463-13).
- `punchflow-kickflow-dummy-s460-dispatch.json` and `punchflow-kickflow-dummy-s462-dispatch.json` are historical. They remain the path-declaration sources for accepted plans (s460: PF-01, PF-03, PF-11, PF-12, PF-13; s462: PF-02, PF-04..PF-10, PF-17). PF-27 archives both.

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
| A | READY | accepted PF-01..PF-13, PF-17 at 44e40cc (session 464) |
| B | BLOCKED | Wave A accepted and `go build ./...` exit 0 |
| C | BLOCKED | Wave B accepted |
| D | BLOCKED | Wave C accepted |

### Phase to Plans Mapping

```
PHASE_TO_PLANS = {   # session 464 continuation (manifest punchflow-kickflow-dummy-s464-dispatch.json); accepted PF-01..PF-13 and PF-17 are not dispatched
  "A": ["pf-14-engine.md", "pf-15-present-ticket-payloads.md", "pf-16-server-cli-testutil.md"],
  "B": ["pf-18-api-users-grades.md", "pf-19-api-proxies-org.md", "pf-20-api-definitions-masters.md",
        "pf-21-api-roles-audit.md", "pf-22-api-tickets.md", "pf-23-api-comments-files.md",
        "pf-24-admin-api.md", "pf-25-readme.md"],
  "C": ["pf-26-contract-smoke.md"],
  "D": ["pf-27-finalize.md"]
}
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
