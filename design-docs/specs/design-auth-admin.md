# Authentication, Sessions and Dev Admin API Design

How callers authenticate to `/v1` and `/admin/api`, how the browser UI logs in through Keycloak OIDC (or the dev-only local login), and the dev admin API that kickflow itself does not have (tokens, service accounts, webhooks, tenant switches, fault injection, DB reset, fixtures).

## Overview

| Surface | Path prefix | Who uses it | Auth |
|---|---|---|---|
| kickflow-compatible API | `/v1/` | kickflow clients under test, the UI | Bearer PAT, Bearer SAT + `X-Caller-Id`, or UI session cookie (+ CSRF header) |
| Dev admin API | `/admin/api/` | UI admin screens, test harnesses (curl) | Same as `/v1` |
| UI helper API | `/ui/api/` | UI watch toggle, notification inbox, keyword search, CSV exports (see `design-notifications-export-search.md`), ticket controls and local workflow-admin helpers | Same as `/v1`; helpers enforce the applicable resource permission |
| Browser auth | `/auth/` | UI | OIDC redirect flow, local login (flag), session endpoints |
| Signed file downloads | `/files/download/` | Anyone holding a signed URL | HMAC signature + expiry in the URL (INF-26) |
| SPA | everything else | Browser | Static assets; `index.html` fallback |

`/admin/api` is never mounted under `/v1`, uses its own routes, and is documented as punchflow-only. It reuses the kickflow error body and the RBAC permission identifiers so the same personas work everywhere.

## Technical Details

### Token authentication (`internal/auth`)

1. Read `Authorization: Bearer <token>`. Missing/unknown/revoked -> `401 invalid_access_token`.
2. Look up `sha256(token)` in `api_tokens` (`kind` = `pat` | `sat`). Update `last_used_at`.
3. `pat`: effective user = token owner. `sat`: apply INF-05 to `X-Caller-Id`; effective user = caller.
4. Apply INF-06 to the effective user.
5. Permissions = union of `permissionList` of the effective user's admin roles, kept as per-permission grants (see "Restricted scopes"). `Principal.Has` means "held in any scope"; handlers that act on a resource additionally check that the grant covers it.
6. Tenant checks: mode `suspended` / `maintenance` and the IP allowlist (see coverage doc). `admin_working` is not checked here; the engine checks it per action (INF-41).
7. Update the user's `lastUsedOn` (JST date) at most once per day per user.
8. Apply a due scheduled organization chart activation (INF-80): the chart whose plan `dueOn` is today or earlier in JST becomes the active chart in one transaction before the grants are loaded, so no background job is needed.

### Restricted scopes (grant model)

A role's `permissionList` entry may be `restricted` and name the folders, general masters or teams it covers. Which dimension applies depends on the permission (`model.PermissionScopeKind`, as in kickflow): `workflow`, `route`, `pipeline` and `ticket_read` by folder; `master` and `master_read` by general master; `team` by team. Every other permission ignores the flag and is always unrestricted. The restriction is enforced (INF-84); there is no longer a divergence for it.

### User lifecycle ticket handover (INF-B8-04)

Kickflow documents applicant-ticket and approval-request handover as administrator UI operations and has no public REST endpoint for them. Punchflow exposes them under `/ui/api/admin/tickets/` and shows controls on the Users page to users with `ticket_write`. Applicant handover updates the direct applicant and selected target team for eligible unproxied drafts, in-progress and rejected tickets. Approval handover replaces incomplete assignees on active steps. Explicit ticket lists are limited to 1,000 and require ticket visibility; blank lists select all eligible tickets. Each changed ticket records an audit log and a `ticket_updated` webhook event in the same transaction.

- `store.UserGrants` merges the entries of all of a user's roles into one `PermissionGrant` per permission: any unrestricted entry makes the grant unrestricted; otherwise `Targets` is the union of the restricted entries' targets, with folder and team targets expanded to their descendants. A restricted entry with no targets matches nothing.

### Custom numbering counter reset (INF-D9-06)

Kickflow documents manual reset as a workflow-management UI action rather than a public REST operation. Punchflow exposes `POST /ui/api/admin/workflows/{workflowId}/custom-numberings/{customNumberingId}/reset`, checks the caller's `workflow` permission against the workflow folder, and resets only the selected custom numbering counter (or its configured shared numbering-key counter) to zero. The next allocation is 1; previously issued ticket values are retained. The Workflow detail UI shows this action to workflow managers.
- `auth.Principal` carries the grants: `Has` (held in any scope), `Unrestricted`, `Allows(permission, target)`, `AllowsAny` and `Targets`.
- Handlers use `requireTarget` (the grant covers a given folder, master or team) and `requireUnrestricted` (operations without a single target). Out-of-scope access returns `403 missing_permission`; list endpoints filter to the covered resources. Deleting a folder needs `workflow`/`route` coverage of every folder it retires (INF-83).
- `ticket_read` is the one restricted permission the engine consults: principal-to-actor conversion does not pass a scoped grant as global; it hands the engine the granted folder IDs (`Actor.ReadFolderIDs`) and `Engine.CanView` lets the actor read tickets of workflows in those folders (INF-36). Section presenters resolve the same grant against the actual ticket workflow folder before applying its section override (INF-SEC-03); ticket UI, exports, watch access and notification visibility use the scoped actor conversion too.
- Roles loaded from fixtures use the same persisted entry shape (`permission`, `restricted`, `folderIds`, `generalMasterIds`, `teamIds`), so a restricted fixture role is enforced like one created through the API. The default fixtures define no restricted role.

Team membership creation accepts omitted `gradeIds` and assigns the configured default grade; an explicit empty array is invalid. Membership updates require both `leader` and `gradeIds` as in the OpenAPI request schema. When a user or team is deleted, directly dependent routes are marked `error` and workflows selecting those routes become `invisible`. Team deletion also removes the team from role permission target arrays so existing roles remain readable. These configuration changes run in the same transaction as deletion, while ticket and audit history is retained according to the lifecycle cleanup rules (INF-SHR-05 to INF-SHR-07).

Token issuing: PATs belong to the creating user; SATs (service accounts) are tenant-wide and need `integration`. The plaintext token is returned exactly once. Fixture-seeded tokens keep their plaintext in `fixture_plaintext` so fixture export can reproduce them; runtime-issued tokens never store plaintext (INF-04).

### Browser sessions

- `sessions` table: id hash, user id, csrf token, created_at, last_seen_at. Idle timeout 12 h, absolute 24 h (kickflow values, reference 6.2).
- Cookie `punchflow_session`: random 32 bytes, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` only when `--public-base-url` is https.
- Cookie auth is used only when no `Authorization` header is present. For `POST`/`PATCH`/`DELETE` with cookie auth, the header `X-Punchflow-CSRF` must equal the session's csrf token, otherwise `403 missing_permission` with message `csrf token mismatch`. Bearer requests never need it.
- `GET /auth/session` -> `{user: UserDetail, permissions: string[], csrfToken, authMethods: {oidc: bool, local: bool}}` or `401 invalid_access_token`. `POST /auth/logout` deletes the session (local logout only; no RP-initiated Keycloak logout).
- Login of an `invited` user with a valid invitation activates the user (INF-31); `suspended`/`deactivated` users are refused.

### Keycloak OIDC (authorization code + PKCE)

Backend-for-frontend flow: the SPA never handles tokens.

| Step | Endpoint | Behavior |
|---|---|---|
| 1 | `GET /auth/oidc/login?returnTo=/path` | Discovery from `--oidc-issuer`. Create `oidc_states` row (state, nonce, PKCE verifier, returnTo, expires in 10 min). Redirect to the authorization endpoint with `response_type=code`, `scope=openid email profile`, `code_challenge_method=S256`. `returnTo` must be a same-origin relative path |
| 2 | `GET /auth/oidc/callback?code&state` | State must exist, be unexpired, and is deleted on use. Exchange the code with the verifier (public client by default; client secret if configured). Verify the ID token: signature via JWKS, `iss`, `aud` = client id, `exp`, `nonce`. Require `email` claim; match a punchflow user by e-mail (case-insensitive). Unknown e-mail or inactive user -> HTML error page with status 403 (no JIT provisioning). Create a session and redirect to `returnTo` |

- OIDC is enabled only when `--oidc-issuer` is set. Library choice: `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` (no hand-written JWT verification).
- Unit test `internal/auth/oidc_test.go` runs a fake issuer on httptest (discovery, JWKS with a test RSA key, token endpoint that checks `code_verifier` against the stored challenge). Cases: success; bad state; expired state; replayed state; nonce mismatch; wrong audience; expired ID token; unknown e-mail; suspended user; invited user activated.
- Manual check (documented in README): start Keycloak via `mise run keycloak:up`, run `punchflow serve --oidc-issuer http://localhost:8180/realms/punchflow`, log in as a realm test user, see the dashboard.

### Keycloak dev deployment (`deploy/keycloak/`)

| File | Content |
|---|---|
| `docker-compose.yml` | One service, official Keycloak image pinned to an explicit version tag (never `latest`), `start-dev --import-realm`, host port 8180, realm file mounted into `/opt/keycloak/data/import`. Bootstrap admin credentials come from env vars with dev defaults documented as dev-only |
| `realm-punchflow.json` | Realm `punchflow`; public client `punchflow-ui` with standard flow only, PKCE method `S256` required, redirect URI `http://localhost:8080/auth/oidc/callback`, web origin `http://localhost:8080`; one Keycloak user per fixture user (same e-mail, `emailVerified: true`), all with the documented dev password |

Keycloak user e-mails must equal `fixtures/default/users.json` e-mails; a Go test asserts that every realm user e-mail exists in the fixtures (the realm JSON is plain data, so this test needs no Docker).

### Local login fallback (dev only)

- Disabled unless `--enable-local-login` or `PUNCHFLOW_ENABLE_LOCAL_LOGIN=true`. When disabled, `/auth/local/*` returns `404 endpoint_not_found`, and `authMethods.local` is false.
- `POST /auth/local/login {email}` creates a session for that user (no password: it is an explicit dev switch). The server logs a warning at startup when enabled.

### Dev admin API (`internal/admin`)

All endpoints use JSON in camelCase, kickflow error bodies, JST timestamps, and the `/v1` pagination headers on list endpoints. Mutations write audit logs (INF-29).

| Method | Path | Permission | Purpose |
|---|---|---|---|
| GET | /admin/api/tokens | any | List own PATs (name, createdAt, lastUsedAt; never the token) |
| POST | /admin/api/tokens | any | Create own PAT `{name}` -> `{token, ...}` once |
| DELETE | /admin/api/tokens/{tokenId} | owner or `integration` | Revoke |
| GET | /admin/api/service-accounts | integration | List service accounts |
| POST | /admin/api/service-accounts | integration | Create `{name, description}` -> SAT once |
| DELETE | /admin/api/service-accounts/{id} | integration | Revoke |
| GET | /admin/api/webhooks | integration | List webhooks |
| POST | /admin/api/webhooks | integration | Create `{name, url, secret?, enabled, workflowIds, eventTypes}`; sends ping |
| GET | /admin/api/webhooks/{id} | integration | Get |
| PATCH | /admin/api/webhooks/{id} | integration | Update; ping when url/secret change |
| DELETE | /admin/api/webhooks/{id} | integration | Delete with its history |
| POST | /admin/api/webhooks/{id}/ping | integration | Test delivery |
| GET | /admin/api/webhooks/{id}/events | integration | Delivery log (paginated) |
| GET | /admin/api/webhook-events/{eventId} | integration | Event with attempts |
| POST | /admin/api/webhook-events/{eventId}/resend | integration | Manual resend |
| POST | /admin/api/webhook-events | integration | Fire an arbitrary event |
| GET | /admin/api/tenant | tenant | Tenant settings |
| PATCH | /admin/api/tenant | tenant | `{name, publicId, plan, mode, ipAllowlist, allowedEmailDomains, selfServiceProxy}` |
| GET | /admin/api/faults | tenant | List fault rules |
| POST | /admin/api/faults | tenant | `{method?, pathPrefix, status, code, message?, remaining?}`; applies to `/v1` only; `remaining` 0 = until deleted |
| DELETE | /admin/api/faults/{id} | tenant | Remove a rule |
| GET | /admin/api/fixtures/export | tenant | Download the current definitions as a fixture bundle (JSON object of file name -> content) |
| POST | /admin/api/fixtures/reset | tenant + `--enable-admin-reset` | Wipe all data and load the configured fixture set; returns a summary. Without the flag: `404 endpoint_not_found` |

Tenant settings defaults: plan `enterprise`, mode `normal`, empty IP allowlist (allow all), empty allowed domains (allow all), `selfServiceProxy: false`. Fault rules live in memory and are cleared by reset or restart.

Bootstrap after reset: fixture-seeded tokens (documented in README) or `punchflow token issue` on the CLI.

### Required tests

| Package | Tests |
|---|---|
| `internal/auth` | PAT/SAT resolution; every INF-05/INF-06 case; permission denial; tenant modes; IP allowlist; CSRF required for cookie mutations and not for bearer; session idle/absolute expiry with fake clock; OIDC cases above; local login disabled/enabled |
| `internal/httpx` | 411, content type, invalid JSON, size limit, pagination headers and Link relations, sortBy parsing, array query parsing, rate limit headers/429/secret multiplier, CORS preflight, fault injection matching and `remaining` countdown |
| `internal/admin` | Every admin endpoint: permission checks, token shown once, reset requires the flag, export/reset round-trip |

## References

- `design-docs/references/kickflow-api.md` 1.2, 1.5, 1.7, 5.6, 6.2
- `design-docs/specs/notes.md` INF-04 to INF-07, INF-26 to INF-32
- `design-docs/specs/design-webhooks.md`
