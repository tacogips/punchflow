# Architecture Design

This document describes system architecture and design decisions for punchflow.

## Overview

punchflow is a **local/dev-only** dummy of the kickflow approval-workflow SaaS. Other systems point their kickflow REST client and webhook receiver at punchflow to test their integration without a kickflow contract.

One Go binary, `punchflow serve`, provides:

1. A kickflow-compatible REST API under `/v1` covering all 96 operations of `design-docs/references/kickflow-openapi.yaml`.
2. kickflow-compatible outgoing webhooks for all 13 events, signed, persisted, retried, re-sendable.
3. An approval engine that drives realistic ticket lifecycles.
4. A SQLite store (pure-Go driver, schema migrations) seeded from JSON fixtures.
5. A punchflow-only dev admin API (`/admin/api`) for tokens, webhooks, tenant switches, fault injection, reset and fixtures.
6. An embedded Solid.js SPA (built with bun) that logs in through Keycloak OIDC (auth code + PKCE) or a flag-gated local login.

Non-goals: production hardening, multi-tenancy, billing, high availability, kickflow AI features, a visual workflow designer, notifications and third-party integrations (see DIV items in `notes.md`).

---

## Sections

### Document map

| Document | Content |
|---|---|
| `design-docs/specs/architecture.md` | This file: components, packages, storage, fixtures, UI, build, dependencies, testing, delivery waves |
| `design-docs/specs/command.md` | CLI subcommands, flags, environment variables, exit codes |
| `design-docs/specs/notes.md` | Terminology mapping, inferred-behavior register (INF-xx), intentional divergences (DIV-xx) |
| `design-docs/specs/design-api-coverage.md` | Coverage matrix: 96 operations, 13 events, error code catalog, contract test |
| `design-docs/specs/design-approval-engine.md` | Ticket state machine, route resolution, step progression, form validation |
| `design-docs/specs/design-webhooks.md` | Webhook outbox, payloads, signing, dispatcher, retries, manual operations |
| `design-docs/specs/design-auth-admin.md` | Tokens, sessions, OIDC, local login, CSRF, Keycloak deployment, dev admin API |
| `design-docs/specs/design-notifications-export-search.md` | Watch subscriptions, notification inbox, CSV export, keyword ticket search (`/ui/api/`) |
| `design-docs/user-qa/pending-punchflow-defaults.md` | Defaults chosen where the user may want a different value |

### Component view

```
 kickflow client ──Bearer──▶ /v1 ───────┐
 browser (SPA)  ──cookie──▶ /v1, /admin/api, /auth
 test harness   ──Bearer──▶ /admin/api ─┤
                                        ▼
            httpx middleware (request id, recover, CORS, faults, 411, rate limit)
                                        ▼
                         auth (token / session, tenant mode, RBAC)
                                        ▼
          api/v1 handlers      admin handlers      auth/oidc + local login
                 │                   │
                 ▼                   ▼
        engine (tickets)    fixtures, webhook admin ops
                 │                   │
                 ▼                   ▼
   store (SQLite tx) ◀── present (kickflow JSON shapes) ──▶ webhook outbox
                                                               │ commit
                                                               ▼
                                              webhook dispatcher ──POST──▶ receiver
```

### Package layout

| Path | Responsibility | May import |
|---|---|---|
| `cmd/punchflow/main.go` | Minimal: signal context, call `cli.Run`, exit code | `internal/cli` |
| `embed.go` (module root, package `punchflow`) | `//go:embed all:web/dist` (`WebDist`) and `//go:embed fixtures/default` (`DefaultFixtures`); go:embed cannot reach parent directories, so the embed must live at the root | - |
| `internal/cli` | Subcommand parsing (`flag`), config from flags + env, wiring | everything below |
| `internal/build` | Version string (existing `VERSION` file) | - |
| `internal/kftime` | `Clock` interface, JST millisecond formatting, accepted input formats, date-only type | - |
| `internal/kferr` | kickflow error type (`status`, `code`, `message`, field errors) and constructors per documented code | - |
| `internal/model` | Plain domain structs and enums (tenant, user, team, ticket, step, ...) | `kftime` |
| `internal/store` | SQLite open, pragmas, embedded migrations, `WithTx`, repositories per aggregate, id-or-code lookups | `model`, `kferr` |
| `internal/present` | Builds kickflow response shapes (User, TicketDetail, WorkflowDetail, ...) from store reads; emits every `required` property of the schema | `store`, `model`, `kftime` |
| `internal/engine` | Ticket lifecycle, route resolution, form validation, events | `store`, `model`, `kferr`, `kftime` |
| `internal/webhook` | Outbox enqueue, payload build, signing, dispatcher, retry, manual ops | `store`, `present`, `kftime` |
| `internal/auth` | Token and session auth, `X-Caller-Id`, RBAC, tenant checks, OIDC, local login, CSRF | `store`, `kferr` |
| `internal/httpx` | JSON encode/decode, error writing, pagination, sortBy, array queries, CORS, 411, rate limit, fault injection, request id, recover | `kferr` |
| `internal/files` | Upload storage under `<data-dir>/files`, signed ids, signed download URLs | `store` |
| `internal/api/v1` | `/v1` routes (`Operations()` table) and handlers per resource (files listed in the coverage matrix) | `httpx`, `auth`, `engine`, `present`, `store`, `files`, `webhook` |
| `internal/admin` | `/admin/api` handlers | `httpx`, `auth`, `webhook`, `fixtures`, `store` |
| `internal/fixtures` | Fixture parse/validate/load/reset/export | `store`, `model` |
| `internal/webui` | SPA static serving with `index.html` fallback and placeholder page | root `punchflow` embed |
| `internal/server` | Mux assembly, middleware chains, dispatcher lifecycle, graceful shutdown | all handler packages |
| `internal/testutil` | Test server with temp DB + default fixtures, persona tokens, webhook receiver, assertion helpers | test support only |
| `internal/contract` | Contract test (test files only) | `testutil`, `api/v1` |

Rules: dependencies point downward only; small interfaces at consumer side (`kftime.Clock`, `webhook.Notifier`); errors wrapped with `%w` and context; every Go file stays under 1000 lines (split handler and presenter files by resource; `present` is expected to need several files such as `ticket.go`, `workflow.go`, `user.go`).

### Request handling

- Router: standard `net/http.ServeMux` with method patterns. `/v1/` catch-all returns `404 endpoint_not_found`.
- Middleware order and conventions: see `design-api-coverage.md`.
- Every mutating request runs in exactly one `store.WithTx` (SQLite `BEGIN IMMEDIATE`). Engine changes, audit log rows and webhook outbox rows commit together; the dispatcher is notified after commit.
- Logging: `log/slog` text handler on stderr; request log line per request with method, path, status, duration, request id. Tokens, secrets and cookies are never logged.

### Storage

- Driver: `modernc.org/sqlite` (pure Go, no cgo), DSN pragmas `foreign_keys(1)`, `journal_mode(WAL)`, `busy_timeout(5000)`. One `*sql.DB` with `SetMaxOpenConns(1)`: all access is serialized, which removes `SQLITE_BUSY` handling at the cost of throughput that a dev tool does not need. No transaction is held across outbound HTTP.
- Migrations: forward-only SQL files `internal/store/migrations/NNNN_<name>.sql`, embedded, tracked in `schema_migrations(version, applied_at)`, applied in order inside a transaction by every command that opens the DB. A DB with a version newer than the binary knows fails with a clear error.
- Time: instants stored as INTEGER Unix milliseconds; date-only values as TEXT `YYYY-MM-DD`; formatting to JST happens only in `present`.
- Definitions: workflows (with sections, fields, route conditions) and routes (with steps) are read-only through `/v1`, so each is stored as one row with indexed columns (`id`, `code`, `status`, `folder_id`) plus `definition_json` in fixture format. Everything that is filtered, mutated or joined (users, teams, memberships, grades, roles, masters, items, proxies, categories, folders, tickets, steps, assignees, comments, viewers, files, audit logs, tokens, sessions, webhooks, events, attempts, tenant) is normalized.
- Secrets owned by the server (HMAC key for signed file URLs) are generated on first start and stored in `server_secrets`; they are never in the repo.

### Fixtures

- Location: `fixtures/default/` (committed, embedded as the default set). `--fixtures DIR` selects another directory.
- Files: `tenant.json`, `grades.json`, `users.json`, `organization-charts.json` (charts with teams and memberships), `roles.json`, `categories.json`, `folders.json`, `general-masters.json` (fields and items), `routes.json`, `workflows.json`, `proxy-applicants.json`, `proxy-approvers.json`, `tokens.json`, `webhooks.json`.
- Format: kickflow field names (camelCase). Every API-visible entity and sub-entity (form fields, sections, route steps, conditions) has an explicit UUID `id` and, where kickflow has one, a `code`. Cross references use codes (`userCode`, `teamCode`, `gradeCode`, `routeCode`, `folderCode`, `formFieldCode`, ...), which keeps files readable and stable.
- Validation on load: JSON shape, enums, uniqueness of ids and codes, referential integrity, INF-14 step requirement rule, INF-18 unsupported features, exactly one active org chart, a super admin role with at least one member, a root folder. Errors name the file and JSON path; nothing is written when validation fails.
- Enum values in fixtures follow the enum contract in `design-api-coverage.md` (for example `userType` `normal|assistant`, route/workflow `status` `visible|...`, `gradeSymbol: null` when there is no grade filter, the 19 kickflow permissions).
- General master item input IDs (CF-02): each `general-masters.json` `items[].inputs[]` entry is `{id, fieldCode, value}`. Its `id` is a required UUID and must be globally unique like every other id. Data flow:
  - Load: `general_master_items.inputs` stores `{fieldId: value}` and `general_master_items.input_ids` stores `{fieldId: inputId}`. Both are written in the same insert.
  - Store read: `store.GetGeneralMasterItem` / `GetGeneralMasterItems` return `model.GeneralMasterItem.InputIDs`.
  - Present: `GeneralMasterItem.inputs[].id` is the persisted input ID. `inputs[].field.id` is the field ID. The two are never the same value. A field ID with a value but no input ID is a data error, not a fallback.
  - API (PF-20): `createGeneralMasterItem` assigns a new UUID per input. `updateGeneralMasterItem` keeps the existing input ID for every field already present, and assigns a new UUID only for a field that had no input before.
  - Export: writes `inputs[].id` from `input_ids`. Inputs are sorted by `fieldCode` (current export order).
  - Owners: PF-02 (column and model field, done), PF-06 (validate/load/export), PF-08 (store read + presenter), PF-20 (API), PF-07 (fixture data already carries the ids).
  - Edited migration: `0001_init.sql` was changed in place, which breaks the forward-only rule. That is acceptable only because nothing has been released (all commits are local). A database created before 410d3d8 must be deleted or `reset`.
- Operations: `load` (into an empty DB), `reset` (delete all rows in FK-safe order and uploaded files, then load), `export` (definitions and org data only; tickets, comments, files, audit logs, sessions and webhook history are state, not fixtures). Runtime-issued tokens are not exported (no plaintext); fixture-seeded tokens are (INF-04).
- Canonical form: export writes 2-space indented JSON with a fixed key order and arrays sorted by `code` (ordered collections such as sections, fields and steps keep their order). The committed default fixtures are stored in canonical form, so the round-trip test asserts `export(load(fixtures/default))` is byte-identical to the files, and `export(load(export(...)))` is stable.
- Canonical form, precise definition (session 464, CF-04): the canonical bytes are whatever `fixtures.Export` writes. The exporter's ordering is not changed. Grades, users, categories, general masters, routes and workflows are sorted by `code`. Org charts, roles, proxies, tokens and webhooks are sorted by `id`. Teams, memberships, folders, master fields and master items keep load order: fixture order, with a parent inserted before its children. Keys follow the field order of `internal/fixtures/types.go`, which is frozen. `omitempty` fields (in `internal/fixtures/types.go` and in the `internal/model` definition structs) that hold a Go zero value are left out: `null`, `false`, `""`, `[]` or `{}`. Examples: `startsOn`, `endsOn`, `description`, `parentCode`, `descendants` (`model.RouteTarget`, `false`), `gradeCodes`, `teamCode`. This bullet replaces the "arrays sorted by `code`" wording of the bullet above wherever the two differ. Two loader/exporter rules make the canonical form a fixed point:
  - Tenant `createdAt`/`updatedAt` are fixture data. `Load` and `Reset` store the values from `tenant.json` and use the load clock only when a value is absent. `Export` writes them in UTC RFC 3339 (`2026-10-20T00:00:00Z`), whatever the machine time zone.
    - Today the loader writes the clock time, and `store.Time` returns local time.
    - Both are fixed in `internal/fixtures/load.go` and `internal/fixtures/export.go` only: export converts with `.UTC()`. `internal/store/ids.go` (PF-02) is not changed.
    - No other row's timestamps are exported.
  - `fixtures/default/*.json` are rewritten to the exporter output.
    - The rewrite may only reorder arrays and drop `omitempty` keys whose value is a Go zero value (`null`, `false`, `""`, `[]`, `{}`).
    - It is accepted only when each of the 14 files is equal in its `44e40cc` version and its new version after this normalization, compared with `diff` and expected to print nothing: `jq -S 'walk(if type=="object" then with_entries(select(.value!=null and .value!=false and .value!="" and .value!=[] and .value!={})) else . end) | walk(if type=="array" then sort else . end)'`. The old version is read with `git show 44e40cc:fixtures/default/<file>`.
    - The filter removes those values recursively (bottom-up), then sorts every array.
    - No integer or interface `omitempty` field drops a zero today: `model` has none, and `any` is omitted only when nil. So `0` is not normalized.
    - Ids, codes, names, enum values and references never change, so the Keycloak realm users and the persona tokens stay valid.
    - `internal/model/definition.go` and every JSON tag stay unchanged (PF-02, not transferred).
    - `TestDefaultRoundTrip` keeps its byte-identical assertion.
- Default content (must exist): tenant `punchflow-dev` (plan enterprise); at least 5 grades; at least 8 users including one `suspended` and one `invited`; one active org chart with at least 6 teams over 3 levels and leaders, plus one inactive chart; roles Super Admin (all permissions), Ticket Admin (`ticket_read`, `ticket_write`), Master Editor (`master`, `master_read`), Integration Admin (`integration`); 3 categories; root folder plus 2 children; 2 general masters with custom fields and items (one expired); at least 4 routes; at least 3 workflows (`expense-claim`, `purchase-request`, `leave-request`) that together exercise every `stepType`, all `actionType`s, branch types `always`/`field`/`field_otherwise` plus a refusal branch, all step condition types, fallbacks `skip` and `higher_manager`, a parallel group with an "any" step and an "all" step, one slip section, one workflow with `commentingEnabled: false`, one with `availableToEveryone: false`; one proxy approver and one proxy applicant registration; dev tokens: PATs for the super admin, a manager, a finance approver and a plain employee, and one service account token. Token values are obviously fake, low-entropy dev strings (for example `pf_pat_dev_admin`) documented in the README.

### Files

`POST /v1/files` stores the upload under `<data-dir>/files/<uuid>` and records filename, content type, byte size, checksum (base64 MD5, the Rails ActiveStorage convention; inferred, INF-26) and a random `signedId`. `GET /v1/files/{signedId}` returns metadata plus `url = <public-base-url>/files/download/<token>` where the token carries file id and expiry (5 min) signed with the server HMAC key (INF-26).

### Web UI (`web/`)

- Stack: Solid.js + TypeScript, `@solidjs/router`, Vite with `vite-plugin-solid` executed through bun (`bun run build` -> `vite build`), because Bun's own bundler has no Solid JSX transform. Package management, scripts and tests use bun only; `bun.lock` is committed; no npm/yarn/pnpm files.
- Output: `web/dist/app/` (git-ignored). `web/dist/.gitkeep` is committed so the root `//go:embed all:web/dist` always compiles; `internal/webui` serves `web/dist/app` when `index.html` exists and otherwise a placeholder page that says how to build the UI. So `go build ./...` and `go test ./...` work without bun.
- Serving: reserved prefixes `/v1/`, `/admin/api/`, `/auth/`, `/files/`, `/healthz`; any other GET serves a matching asset or falls back to `index.html`.
- Data access: the SPA calls `/v1` and `/admin/api` with the session cookie and the `X-Punchflow-CSRF` header; bootstrap from `GET /auth/session`.
- Screens: login (OIDC button; local form only when enabled); dashboard (my tickets, approval requests); ticket search; ticket create/edit (workflow and team selection, proxy client, form generated from `getWorkflow.sectionList`, slip rows, nominated approvers, save draft / submit); ticket detail (inputs, steps with assignees and proxies, approve / send back with step selection / deny / withdraw / archive, comments with attachments and edit/delete, viewers, related tickets); workflows and routes (list plus JSON view); users (list plus suspend/reactivate/reinvite), org charts and teams, grades, roles, general masters and items, categories and folders, proxies, audit logs (read views; other mutations go through the API); API tokens and service accounts; webhooks (CRUD, ping, delivery log with attempts, resend, fire event); dev tools (tenant plan/mode/IP/domains, fault rules, fixture export, reset when enabled).
- Tests: `bun test` over `web/src/**/*.test.ts` covering non-JSX modules: API client (error body parsing, CSRF header, pagination headers and `Link` parsing), form model (workflow sections -> form state -> kickflow `inputs`/`slipItems`), JST date formatting. `bun run typecheck` (`tsc --noEmit`) runs in lint.

### Developer tooling

| mise task | Action |
|---|---|
| `web:install` | `bun install --frozen-lockfile` in `web/` |
| `web:build` | depends on `web:install`; `bun run build` |
| `web:test` | depends on `web:install`; `bun test` |
| `web:typecheck` | depends on `web:install`; `bun run typecheck` |
| `build` (changed) | depends on `web:build`; `go build -o punchflow ./cmd/punchflow` |
| `dev` (new) | depends on `web:build`; `go run ./cmd/punchflow serve --enable-local-login --enable-admin-reset` |
| `run` (changed) | `mise run build && ./punchflow serve` |
| `test` (changed) | `go test -count=1 ./...` then `web:test` (uncached; session 463, PF-27 edits `mise.toml`) |
| `lint` (changed) | `golangci-lint run ./...`, `go vet ./...`, `web:typecheck` |
| `keycloak:up` / `keycloak:down` (new) | `docker compose -f deploy/keycloak/docker-compose.yml up -d` / `down` |

`.gitignore` additions: `/web/dist/app/`, `/.punchflow-data/`. Existing tasks (`vet`, `fmt`, `tidy`, `clean`, homebrew tasks) stay unchanged.

README (new `README.md`): what punchflow is (dev-only), quick start, the seeded personas and dev tokens, how to point a kickflow client at punchflow (base URL `http://localhost:8080` so requests go to `http://localhost:8080/v1/...`, PAT or SAT + `X-Caller-Id`), how to register a webhook with a secret and verify `X-Kickflow-Signature`, Keycloak manual check, the dev-only flags, and the INF/DIV summary link.

### Dependencies

| Dependency | Use | Scope |
|---|---|---|
| `modernc.org/sqlite` | SQLite driver without cgo | runtime |
| `github.com/coreos/go-oidc/v3`, `golang.org/x/oauth2` | OIDC discovery, code exchange, ID token verification | runtime |
| `github.com/santhosh-tekuri/jsonschema/v6` | JSON Schema 2020-12 validation of responses against the OpenAPI 3.1 schemas | tests only |
| `gopkg.in/yaml.v3` | Load the OpenAPI YAML in the contract test | tests only |
| `solid-js`, `@solidjs/router` | SPA | web runtime |
| `vite`, `vite-plugin-solid`, `typescript`, `@types/bun` | Build, typecheck, test types | web dev |

No router, UUID, CLI or logging libraries: the standard library covers them. Adding Go modules follows `.claude/skills/supply-chain-secure-install` (exact versions, `go.sum` committed, `go mod verify`). Bun packages are pinned by `bun.lock` and installed with `--frozen-lockfile`.

### Dev-only safety constraints

- Default listen address `127.0.0.1:8080`.
- Local login and admin reset are off unless their flags/env vars are set; both log a warning at startup when on.
- No real secrets anywhere in the repo: fixture tokens, webhook secrets, Keycloak passwords are documented dev values. If the gitleaks pre-commit hook flags a dev value, use a lower-entropy value rather than an allowlist where possible.
- `X-Caller-Id` impersonation only works with service-account tokens, which require the `integration` permission to create.

### Testing and verification

| Layer | Location | Notes |
|---|---|---|
| Unit | `internal/kftime`, `internal/httpx`, `internal/kferr`, `internal/store`, `internal/fixtures` | formats, middleware, migrations, fixture validation and round-trip |
| Engine | `internal/engine` | scenarios listed in `design-approval-engine.md` |
| Handlers | `internal/api/v1/*_test.go`, `internal/admin/*_test.go` | httptest per endpoint: status, field names, pagination headers, error body |
| Webhooks | `internal/webhook` | see `design-webhooks.md` |
| Auth | `internal/auth` | including fake OIDC issuer |
| Contract | `internal/contract` | 96 routes; a live, schema-validated HTTP scenario for each of the 96 operations (partition in `design-api-coverage.md`); webhook payload schemas |
| Smoke | `internal/cli/serve_smoke_test.go` | runs `cli.Run` with `serve --listen 127.0.0.1:0 --db <tmp>`, reads the `listening on` line, calls `/healthz`, `GET /v1/user` and `POST /v1/tickets` with a seeded token, then cancels and expects exit code 0 |
| Web | `web/src/**/*.test.ts` | `bun test` |
| Manual | README | Keycloak login; curl against a running server |

Final verification commands (PF-27 only): `go build ./...`, `go vet ./...`, `go test -count=1 -v ./...` (includes `repo_invariants_test.go`), `cd web && bun install --frozen-lockfile && bun run build && bun test`, `mise run build && mise run test && mise run lint`, the serve smoke test (`TestServeSmoke`: temp SQLite, seeded token, `/v1` calls) and the webhook receiver tests in PF-22/PF-23/PF-26 (`internal/testutil` receiver).

#### Verification-command rule (session 462)

This rule fixes two failures from the previous run: test evidence came from the `go test` cache and had no counts, and plans deadlocked on a repo-wide gate that depended on other plans' pending files. Every implementation plan's Verification section and Done criteria follow it:

1. Uncached and counted. Every Go test command is `go test -count=1 -v [-race] [-run '<plan pattern>'] <plan packages>`. Web tests are `cd web && bun test`. Cached output (`(cached)`) or output without counts is not evidence.
2. Exit status survives logging. Commands that pipe into `tee` run under `bash -o pipefail -c '<cmd> 2>&1 | tee /tmp/<plan-id>-<step>.log'`, and the progress log records that exit status. Never use the status of `tee`.
3. Counts recorded. For Go, record `--- PASS`, `--- FAIL` and `--- SKIP` line counts from the log (`grep -c -- '--- PASS' <log>`, ...). For bun, record the pass/fail summary line. A passing check needs exit 0, FAIL count 0 and PASS count > 0. A test that the plan requires to PASS must not SKIP.
4. Scoped to the plan. A plan runs tests only for the packages it owns, plus its `-run` pattern inside shared packages (`internal/api/v1`, `internal/store`, `internal/present`, ...). `go build ./...` and `go vet <own packages>` are allowed. No plan other than PF-27 runs, or has a Done criterion that requires, `go test ./...` or "all packages pass".
5. Foreign breakage is a blocker, not a fix. If a scoped command fails because of a file owned by another plan, the plan records the file:line and the owning plan as a reconciliation blocker and does not edit that file. Wave partitioning (below) keeps plans that share a package in dependency order, so this should be rare.
6. Accepted plans stay green. Session 463 accepted set: PF-01..PF-13 and PF-17 at commit `f82e374`. They are not re-implemented or re-dispatched. Their packages are covered by the PF-27 full gate (`go test -count=1 -v ./...` includes `internal/kftime`, `kferr`, `build`, `model`, `store`, `httpx`, `auth`, `fixtures`, `present`, `webhook`, `api/v1` and root `.`; `cd web && bun test` covers PF-03/11/12/13). PF-27 records their PASS counts per package. A remaining plan whose scoped command fails inside an accepted package follows rule 5.
7. Fresh log names. Session 463 logs are `/tmp/<plan-id>-s463-<step>.log`, and per-attempt evidence copies go under `tmp/punchflow-kickflow-dummy-s463/<planId>/attempt-<n>/`. A session-462 log is never evidence for session 463.
8. Session 464 values (these override items 6 and 7):
   - Accepted set: PF-01..PF-13 and PF-17 at commit `44e40cc`. That commit adds the operator's PF-06 repair (`internal/fixtures/load.go` chart-local team lookup and the new `internal/fixtures/load_orgcharts_test.go`) and partial PF-14/PF-16 work.
   - Logs are `/tmp/<plan-id>-s464-<step>.log`. Evidence copies go under `tmp/punchflow-kickflow-dummy-s464/<planId>/attempt-<n>/`.
   - Logs named `-s462-` or `-s463-`, and anything under `tmp/punchflow-kickflow-dummy-s463/`, are history, not evidence.
9. Session 465 values (these override item 8):
   - Accepted set: PF-01..PF-13 and PF-17 at commit `ea385c7`. That commit holds the s464 partial PF-14/PF-15/PF-16 work, including all 17 named PF-14 engine tests.
   - Logs are `/tmp/<plan-id>-s465-<step>.log`. Evidence copies go under `tmp/punchflow-kickflow-dummy-s465/<planId>/attempt-<n>/`.
   - Logs named `-s462-`, `-s463-` or `-s464-`, and anything under `tmp/punchflow-kickflow-dummy-s46{3,4}/`, are history, not evidence.
   - Known baseline failure at `ea385c7`: `go test -count=1 ./...` fails only in `internal/fixtures` (`TestLoadExportRoundTripMinimal`, `fixtures_test.go:56`, "export tenant.json differs from canonical fixture"). It is PF-16's to fix (CF-09). A remaining plan whose scoped command does not include `./internal/fixtures/...` is not affected by it.

#### Path-ownership and fanout rule (session 463)

Session 462 stopped at dispatch. PF-27 declared whole source trees (`internal`, `cmd`, `fixtures`, `deploy`, `web/src`, `impl-plans/active`, ...) as `sharedPaths`, and those expanded to 457 of the 560 tracked files at `f82e374`. The native fanout limit is 512. Every dispatched plan follows these rules:

1. Expansion measure. A plan's expansion is the number of files matched by `writePaths ∪ sharedPaths`, minus anything under `artifactRoots`. A file entry counts 1, even if it does not exist yet. A directory entry counts every file under it in the projected final tree: the files tracked at `f82e374`, plus the files that any remaining plan's `writePaths` create under that directory. The limit is < 300 per plan. 512 is the hard limit and must never be approached.
2. Concrete paths. Remaining plans declare individual files. A directory entry is allowed only for a directory the plan itself populates, and only if its projected expansion is < 300. Example: PF-16 `fixtures/default` has 15 files. No remaining plan may declare `internal/fixtures/testdata`, which belongs to accepted PF-06 and has about 231 files.
3. PF-27 owns only what it edits. It owns:
   - `writePaths`: `repo_invariants_test.go`, plus one `impl-plans/completed/<file>` entry for every file it archives.
   - `sharedPaths`: the matching `impl-plans/active/<file>` entries, `impl-plans/README.md`, `README.md`, `mise.toml`, `design-docs/specs/notes.md`, `go.mod`, `go.sum`, `go-toolchain.json` and `web/toolchain.json`.
   - `artifactRoots`: `web/node_modules`, `web/dist/app` and `punchflow`.

   It owns no directory entry. In particular, none of `internal`, `cmd`, `fixtures`, `deploy`, `web/src`, `impl-plans/active` or `impl-plans/completed` appears as an entry. It archives 27 plan files plus the s460 and s462 manifests, so it declares about 67 entries, all of them files.
4. Repairs go to the owner, not to PF-27. When a PF-27 gate fails because of a file PF-27 does not own, PF-27 does not edit that file. It records a reconciliation request: `file:line`, the owning plan, the log path and the failing command. Integration review then routes the repair:
   - Remaining plans (PF-14..PF-16, PF-18..PF-26): the repair goes back to the owning plan's implementer, within that plan's declared paths.
   - Accepted plans (PF-01..PF-13, PF-17): integration review reopens the plan as a repair item. The repair is limited to the named files and uses that plan's path declarations and scoped verification from the manifest that declares it (item 5: s460 or s462). Acceptance is then re-recorded at the new commit. This is the only way accepted code changes in session 463.

   After every routed repair, PF-27 re-runs its whole gate.
5. Ownership index. Integration review uses one map from path to owning plan, built from the manifest that last dispatched each plan:
   - `impl-plans/active/punchflow-kickflow-dummy-s460-dispatch.json` for PF-01, PF-03, PF-11, PF-12 and PF-13. Those five were accepted before session 462 and are not in the s462 `plans[]`. For example, `embed.go`, `.gitignore`, `cmd/punchflow/main.go` and `mise.toml` are PF-01 `writePaths`, and `web/package.json`, `web/bun.lock` and `web/src/**` are PF-03.
   - `impl-plans/active/punchflow-kickflow-dummy-s462-dispatch.json` for PF-02, PF-04..PF-10 and PF-17.
   - The s463 manifest for the remaining plans.

   Tie-break: when one plan declares a path in `writePaths` and another declares it in `sharedPaths`, the `writePaths` plan owns it. For example, `web/src/pages/Dashboard.tsx` belongs to PF-03, not PF-11. A path that none of these declarations names belongs to the plan whose plan file names it.
6. Frozen contracts after PF-27's narrowing:
   - `internal/model/*` and `internal/fixtures/types.go` change only through a PF-02 repair route.
   - `web/package.json` and `web/bun.lock` change only through a PF-03 repair route.
   - `go.mod` and `go.sum` change only through PF-27's `go mod tidy`, or through a PF-02 repair route when a dependency must be added.
7. Identifier collisions in shared packages (README rule 10). The plan whose identifier breaks the prefix rule renames it. If both identifiers follow the rule, the later-wave plan renames. PF-27 never renames identifiers.
8. The dispatch manifest is not archived by its own run. The s463 manifest stays in `impl-plans/active/` while it drives dispatch. PF-27 lists it in `impl-plans/README.md` as the final manifest. Moving it to `impl-plans/completed/` is a post-run step for the operator.

`repo_invariants_test.go` (module root, package `punchflow`, standard library only) is PF-27's single aggregate test file. It turns PF-27's former ad-hoc shell checks into counted, uncached test evidence that runs inside the full `go test -count=1 -v ./...`. One subtest per invariant:

- `cf01_ban`: the CF-01 ban grep regexes from `notes.md`, applied to production `.go` files under `internal/`, the files under `fixtures/` and `internal/fixtures/testdata/minimal`. It excludes `*_test.go` and `internal/fixtures/testdata/invalid/**`.
- `negative_tests_present`: `audit_read` appears in `internal/fixtures/enum_rules_test.go` and `internal/api/v1/roles_test.go`, and `ticket_submitted` or `comment_deleted` appears in a test file under `internal/admin/`.
- `go_file_size`: every `.go` file outside `web/` has fewer than 1000 lines.
- `no_foreign_lockfiles`: none of `web/package-lock.json`, `web/yarn.lock` or `web/pnpm-lock.yaml` exists.

The `rg` commands in `notes.md` remain manual cross-checks.

#### Shared-tree compile and ownership-transfer rule (session 464)

Session 463's wave A ended with PF-14, PF-15 and PF-16 all blocked. The evidence is under `tmp/punchflow-kickflow-dummy-s463/PF-1{4,5,6}/attempt-1/`. The causes, recorded as CF-04..CF-06 in `notes.md`:

- PF-15 saved `internal/webhook/payload.go` with references to `present.WebhookTenant`, `present.Comment`, `present.TicketWithStep` and `present.TicketDetail` before it defined them. `go build ./...` then failed for every sibling. The file was restored to `f82e374` in `44e40cc`.
- PF-14 briefly left `internal/store/tickets.go:56` with `undefined: time`, which broke PF-15's vet and test runs in the shared package.
- Accepted PF-06's loader failed every fixture-loading test with `UNIQUE constraint failed: team_memberships`. The operator fixed it in `44e40cc`.
- PF-16's `TestDefaultRoundTrip` needed edits to files it did not own. The s463 tie-break also contradicted the working practice: by the tie-break `payload.go` belongs to PF-09, while PF-16's reconciliation request routed it to PF-15.

Every s464 plan follows these rules:

1. Define before reference. A new type or function is written completely, in a file that compiles, before any file that references it is saved. PF-15 runs in this order:
   1. Write its `internal/present` files and `internal/store/read_tickets.go`.
   2. Check that `go build ./...` exits 0.
   3. Edit `internal/webhook/payload.go` in a single write that references only symbols that already exist.
2. Compile gate after every write:
   - After writing a non-test `.go` file, run `go build ./...`.
   - After writing a `_test.go` file, run `go vet <that package>`. This compiles the tests that siblings will run.
   - If the failure is in a file you just wrote, fix it before any other edit. Never start a verification step, or end the turn, while one of your files breaks the build.
   - Contract stubs (`engine.go`, `payload.go`, `files.go`, the `internal/api/v1` handler stubs, the admin stubs) keep every exported identifier and signature. Only bodies change.
3. Sibling breakage. A failure in a file owned by another in-flight plan is checked again at your next verification step. It becomes a reconciliation request (verification rule item 5) only if it is still there at your final verification. You never edit that file.
4. One owner per path, by transfer.
   - When a remaining plan must edit a file that an accepted plan declares in `writePaths`, the s464 manifest moves that file into the remaining plan's `writePaths`. It also records the move in a top-level `ownershipTransfers[]` list as `{path, fromPlan, toPlan, declaringManifest, reason, guardVerification}`.
   - A transfer overrides the s460/s462 declaration in the ownership index. A repair of a transferred file therefore goes to the receiving plan.
   - The receiving plan's verification runs the giving plan's tests for that package, uncached (`guardVerification`). That keeps accepted behavior green.
   - The same applies to stub files created by an earlier-wave remaining plan and filled by a later one: the PF-16 admin stubs go to PF-24.
   - After the transfers, `sharedPaths` is only for a file that two plans in the same wave both edit. It needs a `sharedPathNote`, and the plan author expects none in s464.
5. Complete transfer list for s464:

   | Paths | From -> to | Guard verification |
   |---|---|---|
   | The 14 `fixtures/default/*.json` files, which replace PF-16's `fixtures/default` directory entry (`.gitkeep` stays PF-01) | PF-07 -> PF-16 | whole `./internal/fixtures/...`; the CF-01 ban grep; the `jq` normalization equality from `#fixtures` (drops `null`, `false`, `""`, `[]` and `{}`) |
   | `internal/fixtures/load.go`, `internal/fixtures/export.go` | PF-06 -> PF-16 | whole `./internal/fixtures/...`, including `load_orgcharts_test.go`, which stays PF-06's |
   | `internal/cli/cli.go`, `internal/cli/cli_test.go` | PF-01 -> PF-16 | whole `./internal/cli/...` |
   | `internal/engine/engine.go` | PF-10 -> PF-14 | whole `./internal/engine/...` |
   | `internal/webhook/payload.go` | PF-09 -> PF-15 | whole `./internal/webhook/...` |
   | `internal/files/files.go` | PF-10 -> PF-23 | whole `./internal/files/...` |
   | The `internal/api/v1` handler stub files each wave-B plan fills (today's `sharedPaths` of PF-18..PF-23) | PF-10 -> that plan | the plan's `-run` pattern, plus `-run 'TestPF10'` (PF-10's `TestPF10OperationsMatchCoverageMatrix` and `TestPF10DispatchPatternsAndCatchAll`) |
   | `internal/admin/{tokens,service_accounts,webhooks,tenant,faults,fixtures}.go` (present at `44e40cc`) | PF-16 -> PF-24 | whole `./internal/admin/...` |

6. Fanout. The transfers move entries between lists and do not add files. PF-16 drops `fixtures/default` (15 entries) and the six admin stubs, and gains 16 file entries. PF-27 now also archives the s463 manifest: about 71 entries, all of them files. Every plan stays well under 300.
7. Manifests. `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json` supersedes the s463 manifest. Its `ownershipIndex` reads the accepted plans from the s460/s462 manifests, the remaining plans from s464, and then applies `ownershipTransfers[]`. `acceptedDependencies[].commit` is `44e40cc`. The s464 manifest stays in `impl-plans/active/`. PF-27 archives the s460, s462 and s463 manifests as its last step, after its gate passes.
8. Resume, don't restart. PF-14 and PF-16 continue from their partial files at `44e40cc`. PF-14 has 8 of its 17 tests. PF-16 has its server, CLI, admin skeleton and testutil. For both, the remaining work is what their Done Criteria still leave unchecked. PF-16 must also fix `testutil.Env.DoRaw` (CF-05). It sends a clone of the request with `RequestURI` cleared and a relative URL resolved against `Env.URL`. No testutil helper ever sets `Request.RequestURI` on a client request, because `net/http` rejects it.

#### Plan split, evidence-reporting and wave rule (session 465)

Session 464's wave A (PF-14, PF-15, PF-16) made real progress, but every branch ended blocked, and the progress gate ends a run whose whole wave is blocked (CF-07, CF-08 in `notes.md`). The s464 rules above stay in force. This section adds four changes. `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json` supersedes the s464 manifest, and `acceptedDependencies[].commit` is `ea385c7`.

1. PF-14 is split into three plans with disjoint `writePaths`. Each one can be finished and verified in one attempt. The boundaries follow the existing files at `ea385c7` (see "Plan ownership (session 465)" in `design-approval-engine.md` for behavior, frozen helpers and tests):

   | Plan | Plan file | `writePaths` (besides its plan file) | Remaining work at `ea385c7` |
   |---|---|---|---|
   | PF-14a lifecycle and actions | `impl-plans/active/pf-14a-engine-lifecycle.md` | `internal/engine/engine.go` (transfer PF-10 -> PF-14a, previously -> PF-14), `actions.go`, `comments.go`, `visibility.go`, `numbering.go`, `setup_test.go`, `lifecycle_test.go`, `events_test.go`, `comments_test.go`, `internal/store/tickets.go` | Keep its 14 tests green, and record the per-write compile evidence that s464 left unchecked. Its behavior is complete. |
   | PF-14b routing and conditions | `impl-plans/active/pf-14b-engine-routing.md` | `internal/engine/routing.go`, `conditions.go`, `routing_test.go`, `conditions_test.go` (new), `internal/store/engine_lookup.go` | The full condition operator set (team ancestry, grade `level`, checkbox membership, master item ids); grade-aware `team`/`manager` target resolution; the `higher_manager`/`higher_team` fallbacks; `TestStepConditionAndFallback`, `TestConditionOperators` |
   | PF-14c forms, create and update | `impl-plans/active/pf-14c-engine-forms-create.md` | `internal/engine/forms.go`, `create.go`, `update.go`, `forms_test.go`, `create_update_test.go` (new), `internal/store/engine_refs.go` (new) | Type and reference validation for every field kind; proxy-applicant checks on Create/Update; `UpdateForm` merging that keeps untouched fields; `TestFormFieldTypes`, `TestProxyApplicantCreate`, `TestUpdateFormMerge` |

   - `internal/engine/progression.go` is dropped from the declarations. It was never created; step progression (`pf14Advance`) lives in `actions.go` (PF-14a).
   - `impl-plans/active/pf-14-engine.md` stays as a superseded index. Its status is "Superseded by PF-14a/PF-14b/PF-14c", it keeps its progress log, and it is not dispatched. PF-27 archives it with the other plans.
   - The three plans share the `internal/engine` package, so the s464 compile gate applies to each write. They also share these rules: a cross-file helper's signature and its existing results are frozen (the list is in `design-approval-engine.md`); each plan's scoped test command is the whole package, `go test -count=1 -v -race ./internal/engine/...`, plus `go vet ./internal/engine/... ./internal/store/...`; a failure in a sibling's file follows s464 rule 3 (sibling breakage).
   - Accepted-plan guard: each split plan owns one file in `internal/store` (PF-02's package), so each also runs `go test -count=1 -v ./internal/store/...`. Exported store functions added by the split plans keep the `Eng` prefix (README rule 10).
   - `dependsOn`: PF-14a, PF-14b and PF-14c each depend on PF-06, PF-07, PF-08, PF-10 (all accepted). None depends on another split plan.
   - Downstream rewiring (PF-14 is replaced everywhere it appears in `dependsOn`):

     | Plan | New `dependsOn` |
     |---|---|
     | PF-22 tickets API | PF-14a, PF-14b, PF-14c, PF-15, PF-16 |
     | PF-23 comments and files API | PF-14a, PF-14b, PF-14c, PF-15, PF-16 (its tests create tickets through `Engine.Create`, which validates file references in PF-14c and builds the route in PF-14b) |
     | PF-24 admin API | PF-14a, PF-14b, PF-14c, PF-15, PF-16 (its tests create tickets through `Engine.Create`) |
     | PF-26 contract and smoke | unchanged (PF-17..PF-24); it reaches the engine through PF-22..PF-24 |
     | PF-27 finalize | PF-14 is replaced by PF-14a, PF-14b, PF-14c |

2. Remaining fixture transfers to PF-16 (CF-09). The s464 list already moves the 14 `fixtures/default/*.json` files and `internal/fixtures/load.go`/`export.go`. The s465 manifest adds these, as individual file entries:
   - `internal/fixtures/fixtures_test.go` (PF-06 -> PF-16).
   - The 14 `internal/fixtures/testdata/minimal/*.json` files (PF-06 -> PF-16). They are listed by name; the `internal/fixtures/testdata` and `.../minimal` directories never appear as entries (s463 rule 2).
   - Guard verification for both: the whole `go test -count=1 -v ./internal/fixtures/...`, including `TestLoadExportRoundTripMinimal`, `TestDefaultRoundTrip` and PF-06's `load_orgcharts_test.go`, `validate_test.go` and `enum_rules_test.go`.
   - Allowed change to the minimal testdata: rewrite it to the exporter output. Tenant `createdAt`/`updatedAt` become UTC RFC 3339 for the same instant (`2026-10-20T21:00:00+09:00` -> `2026-10-20T12:00:00Z`). Any other change must pass the same `jq` normalization equality as `fixtures/default` (diff against `git show ea385c7:internal/fixtures/testdata/minimal/<file>`). Ids, codes and enum values never change.
   - Allowed change to `fixtures_test.go`: only what the UTC export needs. The byte-identical assertion and the second-round fixed-point check stay. No test is skipped, deleted or weakened.
3. Evidence-reporting rule. Every plan's Shared Execution Protocol states it word for word, and Step 6 reports follow it:
   1. The `verification` array holds only commands that are required to pass, with exit 0, PASS > 0 and FAIL = 0 for test commands. An intentional negative or mutation proof (an expected non-zero exit: an `rg` ban grep that finds nothing and exits 1, a mutated test that must fail, a request that must be rejected) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
   2. When assigned work remains, the report sets `implementationIncomplete: true` and lists the remaining items in `implPlanUpdates` and in the plan's Progress Log. Remaining work is never reported as a High/Medium finding, a risk or a residual risk.
   3. High and Medium labels are reserved for defects in delivered code, each with `file:line`. A sibling-breakage reconciliation request (s464 rule 3) is reported as a blocker naming the owning plan, not as a finding against the reporting plan.
   4. Every attempt leaves compiling, incremental progress: the shared tree builds (`go build ./...` exit 0) at the end of the turn, and the Progress Log records what was completed.
4. Fanout after the split, measured against the projected final tree with the s463 rule-1 measure:
   - PF-14a: 11 entries. PF-14b: 6. PF-14c: 7.
   - PF-16: 36 + 1 + 14 = 51.
   - PF-27: about 80 entries, all files. The s464 total of 72 grows by the three new plan files (active `sharedPaths` + completed `writePaths`) and the s464 manifest (its archive pair). It still declares no directory entry.
   - Every other plan keeps its s464 count (PF-15: 11, PF-18: 8, PF-19: 9, PF-20: 12, PF-21: 7, PF-22: 9, PF-23: 9, PF-24: 13, PF-25: 2, PF-26: 10).
   - Every plan is far under the 300 target. The s465 manifest records each plan's computed count and the disjointness of all `writePaths`.

#### PF-26 split and final-run rule (session 466)

Session 466 is the final continuation. All s464/s465 rules stay in force. `impl-plans/active/punchflow-kickflow-dummy-s466-dispatch.json` supersedes the s465 manifest.

1. Accepted set: PF-01..PF-13, PF-14a, PF-14b, PF-14c and PF-15..PF-25 at commit `a2a82de` (`acceptedDependencies[].commit`). They are not dispatched. Logs are `/tmp/<plan-id>-s466-<step>.log`, with evidence copies under `tmp/punchflow-kickflow-dummy-s466/<planId>/attempt-<n>/`. Older logs are history, not evidence. The s465 baseline failure note (verification rule item 9) no longer applies, because PF-16 (the CF-09 owner) is accepted. PF-27's full gate re-proves `TestLoadExportRoundTripMinimal`.
2. PF-26 is split into PF-26a..PF-26f. Their files, operations, scenario rules and repair-eligible files are defined in `design-api-coverage.md` "Live scenario partition (session 466)". `impl-plans/active/pf-26-contract-smoke.md` stays as a superseded index: status "Superseded by PF-26a..PF-26f", with its progress log kept. It is not dispatched, and PF-27 archives it. Each sub-plan's `dependsOn` is empty, because every dependency is in the accepted set.
3. Sub-plan paths:
   - `writePaths`: its plan file `impl-plans/active/pf-26<x>-<slug>.md`, its scenario file and its repair-eligible files with their same-basename `_test.go` files. PF-26c also has `scenarios_users_org_test.go`.
   - No `sharedPaths`. The `writePaths` of the six sub-plans are pairwise disjoint.
   - Each plan has about 10-30 entries, all files.
4. Sub-plan verification, all must pass:
   - `go build ./...`
   - `go vet ./internal/contract/...`
   - `go test -count=1 -v -race -run '^TestContractLive<Group>$' ./internal/contract/...` with PASS/FAIL/SKIP counts
   - `test -z "$(gofmt -l <own files>)"`
   - for each edited repair-eligible file, its package guard `go test -count=1 -v -race ./internal/<pkg>/...`

   The whole-package contract run belongs to PF-27's full gate.
5. PF-27 depends on PF-26a..PF-26f. Additions:
   - It owns the new file `internal/contract/coverage_test.go` (`TestContractOperationCoverage`), its only file under `internal/`. `repo_invariants_test.go` stays at the module root.
   - It archives the six sub-plan files, the PF-26 index and the s465 manifest. The s466 manifest stays in `active/`.
   - Its full gate must also show `--- PASS: TestContractOperationCoverage`, the six `--- PASS: TestContractLive<Group>` lines, `--- PASS: TestContractWebhookPayloads` and `--- PASS: TestServeSmoke`.
   - The smoke requirement ("serve against a temp SQLite file, call `/v1` with a seeded token, run the webhook receiver") is met by `TestServeSmoke` and the receiver-based tests in the same uncached gate. Nodes may not leave detached processes, so a manual `curl` against a foreground `punchflow serve` stays a README-documented manual check (`README.md` "Point a kickflow client at punchflow").
   - `README.md` already documents the base URL, the seeded tokens and webhook secret verification (PF-25, accepted). PF-27 edits it only if its `mise.toml` `test` change makes the text wrong.

| Wave | Plans | Ready because |
|---|---|---|
| A | PF-26a, PF-26b, PF-26c, PF-26d, PF-26e, PF-26f (parallel) | Every dependency is accepted at `a2a82de` |
| B | PF-27 (serial, last) | PF-26a..PF-26f accepted |

#### Final gate and archival rule (session 467)

Session 466 stopped PF-27 at gate line 12 (`mise run lint`): 40 golangci-lint diagnostics in files that other plans own. Commit `f6f4ba8` fixed them and added a v2 `.golangci.yml` that excludes the git-ignored `tmp/` and `web/node_modules`. Session 467 finishes PF-27. The rules from s463 to s466 stay in force, except where the items below override them.

1. Accepted set. PF-01..PF-13, PF-14a, PF-14b, PF-14c, PF-15..PF-25 and PF-26a..PF-26f are all accepted at `f6f4ba8`. They are not dispatched. The superseded indexes `pf-14-engine.md` and `pf-26-contract-smoke.md` are not dispatched either. Logs are named `/tmp/pf27-s467-<step>.log`, with evidence copies under `tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-<n>/`. Logs tagged `-s466-` or earlier are history, not evidence.
2. Docs and plans only. Everything PF-27 changes in source was finished at `f6f4ba8`: `repo_invariants_test.go`, `internal/contract/coverage_test.go`, the uncached `mise.toml` `test` task, the `finalizeS466` keys in `go-toolchain.json` and `web/toolchain.json`, and the tidied `go.mod`/`go.sum`. Session 467 edits none of these files. PF-27 paths, all of them file entries:
   - `writePaths`:
     - `design-docs/specs/notes.md`
     - `impl-plans/README.md`
     - `impl-plans/active/pf-27-finalize.md` and `impl-plans/completed/pf-27-finalize.md`
     - for each of the 41 other archived files, its `impl-plans/active/<f>` and `impl-plans/completed/<f>` paths. The 41 files are the 35 plan files other than pf-27-finalize.md and the s460, s462, s463, s464, s465 and s466 manifests.
   - `artifactRoots`: `tmp/punchflow-kickflow-dummy-s467`, `web/node_modules`, `web/dist/app` and `punchflow`, all git-ignored.
   - Total: 2 + 2 + 82 = 86 entries, no directory entry, far under 300.
3. Gate. The gate is the 16 must-pass lines of s466 item 4, with `-s466-` replaced by `-s467-`. Line 10 must count 15. The lines run in order, in the foreground, as one uninterrupted run. Line 3 (`go mod tidy`) must leave `git diff --quiet go.mod go.sum` true. If tidy changes either file, that is a reconciliation request routed to PF-02, not a PF-27 edit. Any failing line is routed through the s466 ownership index (`ownershipTransfers[]` first). After the repair lands, the gate restarts from line 1 as attempt n+1. Archival starts only after one attempt passes all 16 lines.
4. Manifest archival. This item replaces s463 item 8 and s466 item 5 for this run. Those items kept the then-current manifest in `active/` because it was driving that run's dispatch. The s466 manifest no longer drives dispatch, so PF-27 archives it with the others: s460, s462, s463, s464 and s465 first, then s466 last. Then PF-27 sets its own plan to Completed and moves it, as the very last move.
   - If the workflow writes a session-467 manifest, it stays in `active/`, and `impl-plans/README.md` names it as the final manifest.
   - If no s467 manifest exists, `impl-plans/README.md` names `completed/punchflow-kickflow-dummy-s466-dispatch.json` as the final manifest, and `active/` ends up empty. No placeholder file is added.
5. Plan status on archival. Each archived plan file gets `**Status**: Completed` and one final progress entry, a single line that cites the s467 gate log. The two superseded indexes get `**Status**: Completed (superseded by PF-14a/PF-14b/PF-14c)` and `**Status**: Completed (superseded by PF-26a..PF-26f)`. No other plan content changes. Manifests move with their bytes unchanged.
6. Stale paths. Only `impl-plans/README.md` is rewritten to point at `completed/`. Path mentions of `impl-plans/active/<f>` in this document, in `notes.md` and inside the archived plans are records of earlier sessions, so they stay as written. After archival, each such path refers to `impl-plans/completed/<f>`. No design doc or plan has a Markdown link into `active/`, so no link breaks.
7. Closing notes in `notes.md`. They are appended as one new subsection, "Closing notes (session 467)". It goes directly after the CF-07..CF-09 table and before "Intentional divergences". No existing row or bullet is edited. The subsection has one bullet per finding, each citing evidence from `/tmp/pf27-s467-test.log` (copied to the attempt directory) and its gate line, plus the PF-26 split note:

   | ID | Closing evidence |
   |---|---|
   | CF-01 | `TestRepoInvariants/cf01_ban` and `/negative_tests_present` PASS (line 7); line 14 `rg` exits 1 |
   | CF-02 | `TestGeneralMasterItemsAndInputIDHop` and `TestDefaultRoundTrip` PASS |
   | CF-03 | `internal/auth` reports `ok` (line 9) |
   | CF-04 | `TestDefaultRoundTrip` PASS |
   | CF-05 | `TestDoRawResolvesRelativeURLs` PASS |
   | CF-06 | lines 5 and 6 exit 0, and every package is `ok` (line 9) |
   | CF-07 | `TestSubmitMultiStepApproveCompletes`, `TestStepConditionAndFallback`, `TestConditionOperators`, `TestFormFieldTypes` and `TestUpdateFormMerge` PASS, and `internal/engine` reports `ok` |
   | CF-08 | the s466 PF-27 `verification` array, and the s467 one if an s467 manifest exists, hold only must-pass forms (lines 8 and 13-16 end in `test ...`) |
   | CF-09 | `TestLoadExportRoundTripMinimal` PASS |
   | PF-26 split | the six `TestContractLive<Group>` PASS lines and `TestContractOperationCoverage` PASS (96 unique operations) |

Session 467 dispatch:

| Wave | Plans | Ready because |
|---|---|---|
| - | PF-27 (serial, alone) | every other plan accepted at `f6f4ba8` |

### Delivery waves (input for implementation plans)

| Wave | Work | Depends on |
|---|---|---|
| 1 | `kftime`, `kferr`, `model`, `store` + migrations, `httpx`, root `embed.go` + `web/dist/.gitkeep`, CLI skeleton (`version`), mise tasks, `web/` scaffold with bun | - |
| 2 | `fixtures` (load/reset/export) + default fixture files; `auth` token/session core; `present` | 1 |
| 3 | `engine`; `/v1` non-ticket handlers (users, grades, proxies, org charts, teams, categories, folders, definitions, general masters, roles, audit logs) in parallel | 2 |
| 4 | Ticket, comment, viewer and file handlers; `webhook` package; `admin` API; `serve` wiring and smoke test | 3 |
| 5 | OIDC + local login + `deploy/keycloak`; SPA screens; contract test; README | 4 (SPA can start after 2 against the API shapes) |

The coverage matrix is the checklist for waves 3-5: a row is done when its handler, handler test and contract scenario exist and pass.

Continuation order (session 462, from checkpoint 410d3d8). The plan-to-phase mapping in `impl-plans/README.md` is unchanged except for one point: the PF-02 corrections for CF-01 and CF-02 (enum lists, removal of invented constants, conformance test, store-test seed values) run first and alone, because every other plan compiles against `internal/model`. After that the phases are: PF-04..PF-10 -> PF-14..PF-17 -> PF-18..PF-25 -> PF-26 -> PF-27. Within a phase, PF-06 (validator switch to the Go lists, input ID load/export) and PF-07 (fixture enum values) are the first consumers of the corrected enums. PF-08 handles the input ID read/present.

Continuation order (session 463, from checkpoint `f82e374`). PF-01..PF-13 and PF-17 are accepted dependencies. They are listed in the s463 manifest's `acceptedDependencies` with commit `f82e374`, which corrects the s462 list that named only PF-01/03/11/12/13. They are not dispatched. The remaining 13 plans keep their tasks, paths and `dependsOn`. Any `dependsOn` that points at an accepted plan is satisfied by `acceptedDependencies`. Waves:

| Wave | Plans | Ready because |
|---|---|---|
| A | PF-14, PF-15, PF-16 | their dependencies (PF-04..PF-10) are accepted |
| B | PF-18, PF-19, PF-20, PF-21, PF-22, PF-23, PF-24, PF-25 | PF-14/15/16 done; PF-08 and PF-07 accepted |
| C | PF-26 | PF-17 accepted, PF-18..PF-24 done |
| D | PF-27 (serial, last) | every other plan done |

Wave B plans share `internal/api/v1` and `internal/store` only through files that the PF-10 or PF-02 stubs already created, or through new files that each plan creates for itself. No plan in wave B edits a router or registration file: the PF-10 `Operations()` table (`routes.go`) is accepted and fixed at 96 entries. Each plan replaces handler bodies in its own declared stub files.

Continuation order (session 464, from checkpoint `44e40cc`). The waves are the s463 waves. Session 463 did not fail because of dependency order. It failed on the shared-tree, ownership and loader problems listed in the session-464 rule section above, so the waves stay and the rules change.

| Wave | Plans | Ready because | Gate before the next wave |
|---|---|---|---|
| A | PF-14, PF-15, PF-16 (parallel, under the compile rule; PF-15 uses the define-before-reference order) | PF-01..PF-13 and PF-17 accepted at `44e40cc`; the PF-06 loader repair has landed | every wave-A plan accepted; `go build ./...` exits 0 |
| B | PF-18, PF-19, PF-20, PF-21, PF-22, PF-23, PF-24, PF-25 (parallel) | PF-14/15/16 accepted: engine, ticket presenters, payloads, testutil, server and admin skeleton | every wave-B plan accepted |
| C | PF-26 | PF-17 accepted, PF-18..PF-24 accepted | PF-26 accepted |
| D | PF-27 (serial, last) | every other plan accepted | - |

Continuation order (session 465, from checkpoint `ea385c7`). A plan is dispatched as soon as every plan in its `dependsOn` is accepted. The waves below are the groups that this produces; they are not extra gates. The s464 wave A had three plans, and all of them were blocked. The s465 wave A has six independent plans, so one blocked branch no longer blocks the whole wave.

| Wave | Plans | Ready because |
|---|---|---|
| A | PF-14a, PF-14b, PF-14c, PF-15, PF-16, PF-25 (parallel; PF-15 keeps its define-before-reference order) | Every dependency is accepted at `ea385c7`. PF-25 now depends only on PF-07 (see below). |
| B1 | PF-18, PF-19, PF-20, PF-21 (parallel) | PF-16 accepted (testutil, server); PF-08 accepted |
| B2 | PF-22, PF-23, PF-24 (parallel) | PF-14a, PF-14b, PF-14c, PF-15 and PF-16 accepted |
| C | PF-26 | PF-17 accepted, PF-18..PF-24 accepted |
| D | PF-27 (serial, last) | every other plan accepted |

- B1 may start while A's engine plans are still running. B1 plans do not touch `internal/engine`.
- PF-25 (README) dependency change: `dependsOn` is now `[PF-07]`. The README documents the CLI flags, environment variables and `serve` defaults as `command.md` specifies them, and the base URL, tokens and webhook secrets as the fixtures (PF-07) seed them. The README needs nothing that PF-16 adds beyond what `command.md` already fixes. If PF-27's gate finds a mismatch between the README and the PF-16 CLI, it routes the repair to the plan whose file diverges from `command.md` (s463 rule 4).

---
