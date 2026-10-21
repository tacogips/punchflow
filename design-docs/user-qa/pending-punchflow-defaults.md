# punchflow: Defaults Chosen Without User Input

**Status**: Pending Decision (non-blocking; implementation proceeds with the defaults below)

**Created**: 2026-10-20

**Category**: Architecture / Command Design

## Decision Needed

The design picked a default for each item below because the public kickflow material does not settle it or because it is a tooling preference. Implementation uses these defaults. The user may override any of them; the design docs name the place to change.

## Items

| # | Item | Default chosen | Alternatives | Where defined |
|---|---|---|---|---|
| 1 | Webhook retry backoff | 10 s base, doubling, 1 h cap, 1 + 10 attempts; `--webhook-retry-base` | Fixed interval; minutes-based base | `notes.md` INF-08 |
| 2 | Keycloak image version | Official Keycloak image pinned to an explicit current release tag chosen at implementation time (never `latest`); host port 8180 | Other ports or a specific version the user standardizes on | `design-auth-admin.md` |
| 3 | Docker availability | Keycloak login is a documented manual check; automated tests use a fake issuer and need no Docker | Add a Docker-based CI check | `design-auth-admin.md` |
| 4 | Frontend bundler | Vite + vite-plugin-solid, run through bun (Bun's bundler has no Solid JSX transform) | Bun.build with a custom Babel plugin | `architecture.md` Web UI |
| 5 | Frontend tests | `bun test` on non-JSX TypeScript modules only | Component tests with a Solid test renderer | `architecture.md` Web UI |
| 6 | Contract-test validator | `santhosh-tekuri/jsonschema/v6` + `gopkg.in/yaml.v3`, test-only | kin-openapi (limited OpenAPI 3.1 support) | `architecture.md` Dependencies |
| 7 | OIDC library | `coreos/go-oidc/v3` + `golang.org/x/oauth2` | Hand-written JWT/JWKS verification | `design-auth-admin.md` |
| 8 | Local login | E-mail only, no password, behind `--enable-local-login` | Shared dev password | `design-auth-admin.md` |
| 9 | Default listen address | `127.0.0.1:8080` | `0.0.0.0:8080` for container use | `command.md` |
| 10 | Admin UI mutations | Only tokens, service accounts, webhooks, user status actions, tenant switches, faults, reset are editable in the UI; other org/master data are read-only views (editable through `/v1`) | Full CRUD screens | `architecture.md` Web UI |

## Impact

Changing items 1, 8 or 9 affects only config defaults. Items 4-7 change dependencies and should be decided before wave 1/5 implementation if the user disagrees.

## Awaiting

Optional user confirmation. No item blocks implementation.
