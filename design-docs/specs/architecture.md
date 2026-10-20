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
- Operations: `load` (into an empty DB), `reset` (delete all rows in FK-safe order and uploaded files, then load), `export` (definitions and org data only; tickets, comments, files, audit logs, sessions and webhook history are state, not fixtures). Runtime-issued tokens are not exported (no plaintext); fixture-seeded tokens are (INF-04).
- Canonical form: export writes 2-space indented JSON with a fixed key order and arrays sorted by `code` (ordered collections such as sections, fields and steps keep their order). The committed default fixtures are stored in canonical form, so the round-trip test asserts `export(load(fixtures/default))` is byte-identical to the files, and `export(load(export(...)))` is stable.
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
| `test` (changed) | `go test ./...` then `web:test` |
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
| Contract | `internal/contract` | 96 routes, schema validation, webhook payload schemas |
| Smoke | `internal/cli/serve_smoke_test.go` | runs `cli.Run` with `serve --listen 127.0.0.1:0 --db <tmp>`, reads the `listening on` line, calls `/healthz`, `GET /v1/user` and `POST /v1/tickets` with a seeded token, then cancels and expects exit code 0 |
| Web | `web/src/**/*.test.ts` | `bun test` |
| Manual | README | Keycloak login; curl against a running server |

Verification commands: `go build ./...`, `go vet ./...`, `go test ./...`, `cd web && bun install && bun run build && bun test`, `mise run build && mise run test && mise run lint`.

### Delivery waves (input for implementation plans)

| Wave | Work | Depends on |
|---|---|---|
| 1 | `kftime`, `kferr`, `model`, `store` + migrations, `httpx`, root `embed.go` + `web/dist/.gitkeep`, CLI skeleton (`version`), mise tasks, `web/` scaffold with bun | - |
| 2 | `fixtures` (load/reset/export) + default fixture files; `auth` token/session core; `present` | 1 |
| 3 | `engine`; `/v1` non-ticket handlers (users, grades, proxies, org charts, teams, categories, folders, definitions, general masters, roles, audit logs) in parallel | 2 |
| 4 | Ticket, comment, viewer and file handlers; `webhook` package; `admin` API; `serve` wiring and smoke test | 3 |
| 5 | OIDC + local login + `deploy/keycloak`; SPA screens; contract test; README | 4 (SPA can start after 2 against the API shapes) |

The coverage matrix is the checklist for waves 3-5: a row is done when its handler, handler test and contract scenario exist and pass.

---
