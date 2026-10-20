# Command Design

This document describes CLI command interface design specifications for the `punchflow` binary.

## Overview

`punchflow` uses standard-library `flag` sets per subcommand (`internal/cli`). `cmd/punchflow/main.go` only creates a signal-aware context (SIGINT/SIGTERM) and calls `cli.Run(ctx, args, stdout, stderr) int`. Configuration precedence: flag > environment variable > default. Every flag has an environment variable `PUNCHFLOW_<FLAG_NAME_IN_UPPER_SNAKE>` (for example `--oidc-issuer` -> `PUNCHFLOW_OIDC_ISSUER`). Boolean env vars accept `true`/`false`/`1`/`0`.

---

## Sections

### Subcommands

| Command | Purpose |
|---|---|
| `punchflow serve` | Migrate the DB, seed it from fixtures when it is empty, start the HTTP server (`/v1`, `/admin/api`, `/auth`, `/files`, SPA) and the webhook dispatcher. Prints `listening on http://<addr>` to stderr once the listener is open. Shuts down gracefully on signal |
| `punchflow fixtures load` | Load a fixture set into an empty DB. Fails (exit 1) if the DB already contains data |
| `punchflow fixtures reset` | Delete all data (including uploaded files) and load a fixture set. Local operator command, so it does not require `--enable-admin-reset`; stop `serve` first or expect the running server to see the new data on its next request |
| `punchflow fixtures export` | Write the current definitions and org data in canonical fixture form to `--out DIR` |
| `punchflow token issue` | Issue a token and print it once to stdout: `--user <id|code|email>` for a PAT, or `--service-account <name>` for a SAT |
| `punchflow version` | Print the version from `internal/build/VERSION` |

### Flags and Options

Shared by every DB-using subcommand:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data-dir` | path | `.punchflow-data` | Directory for the default DB file and uploaded files (`files/`) |
| `--db` | path | `<data-dir>/punchflow.sqlite` | SQLite database file |
| `--fixtures` | path | embedded `fixtures/default` | Fixture directory for seeding, `load` and `reset` |

`serve` only:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--listen` | host:port | `127.0.0.1:8080` | Listen address; port 0 picks a free port |
| `--public-base-url` | URL | `http://<listen address>` (`localhost` for loopback) | Base for `Link` headers, signed file URLs and the OIDC redirect URL |
| `--seed` | bool | `true` | Load fixtures automatically when the DB is empty |
| `--enable-local-login` | bool | `false` | Enable the dev-only e-mail login at `/auth/local/login` |
| `--enable-admin-reset` | bool | `false` | Enable `POST /admin/api/fixtures/reset` |
| `--oidc-issuer` | URL | empty (OIDC off) | Keycloak realm issuer, for example `http://localhost:8180/realms/punchflow` |
| `--oidc-client-id` | string | `punchflow-ui` | OIDC client id |
| `--oidc-client-secret` | string | empty (public client) | Only for confidential clients; prefer the env var |
| `--oidc-redirect-url` | URL | `<public-base-url>/auth/oidc/callback` | Must match the Keycloak client |
| `--rate-limit` | int | `0` (off) | Requests per minute per IP on `/v1` (INF-07) |
| `--rate-limit-secret` | string | empty | Value of `X-Rate-Limit-Secret` that raises the limit 10x |
| `--webhook-retry-base` | duration | `10s` | Base of the exponential retry backoff (INF-08) |
| `--max-upload-bytes` | int | `52428800` | Upload size limit (INF-39) |
| `--log-level` | enum | `info` | `debug`, `info`, `warn`, `error` |

`fixtures export`: `--out DIR` (required; created if missing; existing fixture files are overwritten).

`token issue`: `--user`, `--service-account` (exactly one), `--name` (default `cli`).

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PUNCHFLOW_<FLAG>` | No | flag default | Environment form of every flag above |
| `PUNCHFLOW_ENABLE_LOCAL_LOGIN` | No | `false` | Same as `--enable-local-login` (listed explicitly because it is a dev-only switch) |
| `PUNCHFLOW_ENABLE_ADMIN_RESET` | No | `false` | Same as `--enable-admin-reset` |
| `PUNCHFLOW_OIDC_CLIENT_SECRET` | No | empty | Preferred way to pass a client secret; never logged |

No variable is required to run with defaults. Secret-bearing values must not be committed; when a real secret is ever needed locally, run through `kinko exec`.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (for `serve`: clean shutdown after a signal) |
| 1 | General error (DB, migration, fixture validation, listen failure, `fixtures load` on a non-empty DB) |
| 2 | Usage error (unknown subcommand, invalid flag value, missing required flag) |

### Usage Examples

```bash
# start with a throwaway DB, seeded automatically from the embedded fixtures
punchflow serve --db /tmp/pf.sqlite --enable-local-login

# call the kickflow-compatible API with a seeded dev token
curl -H 'Authorization: Bearer pf_pat_dev_admin' http://localhost:8080/v1/tickets

# reset to the default fixtures
punchflow fixtures reset --db /tmp/pf.sqlite
```

---
