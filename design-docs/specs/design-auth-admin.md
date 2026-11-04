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
| POST | /admin/api/workflows | `workflow` covering the target folder | Add one workflow definition (shape of one fixture `workflows.json` element) to the running store; returns `201` with the created WorkflowDetail. See "Runtime workflow creation" |
| POST | /admin/api/fixtures/reset | tenant + `--enable-admin-reset` | Wipe all data and load the configured fixture set; returns a summary. Without the flag: `404 endpoint_not_found` |

Tenant settings defaults: plan `enterprise`, mode `normal`, empty IP allowlist (allow all), empty allowed domains (allow all), `selfServiceProxy: false`. Fault rules live in memory and are cleared by reset or restart.

Bootstrap after reset: fixture-seeded tokens (documented in README) or `punchflow token issue` on the CLI.

### Runtime workflow creation (`createWorkflow`, argus #523)

`POST /admin/api/workflows` adds one workflow to a running server, so tests no longer have to restart it with a modified fixture. It does not depend on `--enable-admin-reset`, because it only adds data and never wipes any.

Request body: a single JSON object with the shape of one element of fixture `workflows.json` (`model.WorkflowDef`: `code`, `name`, `status`, `folderCode`, `routeCode`, `categoryCodes`, `sectionList`, `routeConditions`, ...). It is decoded with unknown fields rejected, as `fixtures.Parse` does.
- `id` is optional. When it is empty the server generates a UUID v4; when it is present it must be a UUID.
- Nested IDs (sections, fields, custom numberings) must be present UUIDs, as in fixtures.
- The body does not carry dependent definitions. Folders, routes, categories, users, teams, grades and general masters must already exist in the live store, and are referenced by code exactly as in fixtures.

Processing order (every step after authentication runs inside one `store.DB.WithTx` transaction):

1. Authentication: a missing or invalid token gives `401 invalid_access_token`. A principal without the `workflow` permission in any scope gives `403 missing_permission`.
2. Decode: malformed JSON gives `400 invalid_json`. An unknown field or a wrong type gives `400 invalid_parameter`.
3. Duplicate check: a `workflows` row of any status (including `deleted`) with the same `id` or the same `code` gives `409`. The error is `kferr.New(409, "conflict", ...)` and the message names the clashing key. A unique-constraint failure on insert maps to the same 409, which covers concurrent creators.
4. Validation, reusing the fixture validator unchanged:
   - Build the current fixture `Set` from the transaction with `fixtures.Export` + `fixtures.Parse`. These are the same canonical bytes that `exportFixtures` returns.
   - Baseline run: call `fixtures.Validate` on the exported set as it is, and collect its problems (no problems when it returns nil).
   - Candidate run: append the candidate as the last element of `Set.Workflows` (index `n` = previous length) and call `fixtures.Validate` again.
   - Candidate problems are the candidate run's problems that the baseline run does not have. The comparison is a multiset difference on the full `(File, Path, Message)` triple. Problems are not attributed by path prefix, because the validator reports candidate problems in several forms:
     - `workflows.json` `[n]...` for structural and reference rules (validate.go:324-505);
     - `workflows.json` `$[n]...` for ID format and duplicate checks (`pf06WalkIDs`);
     - `workflows.json` `workflows[n]...` for custom validations and their 50 form / 10 detail limits (validate_custom.go:20-35, appended at validate.go:507);
     - other files, such as `tokens.json`, `proxy-*.json`, `webhooks.json`, `labels.json` or `sub-statuses.json` with message `duplicate id (also used at workflows.json:$[n]...)`, when a candidate ID collides with an ID that a later file registers (validate.go:28-50).

     The candidate is appended last, so no existing problem's path or message changes. Every new triple is therefore caused by the candidate. Baseline problems still present come from pre-existing runtime state and are ignored, so earlier runtime edits cannot block the request. Problems that the candidate removes (for example, a previously unknown workflow code that now resolves) are ignored too.
   - Any candidate problem gives `400 invalid_parameter`. The message lists every candidate problem as `file: path: message`, in the validator's stable order (`sortProblems`). The `errors` object is reserved for 422, so it is not used.
   - A rejection returns the error from the `WithTx` callback before step 6, so the transaction rolls back and neither the `workflows` row nor the audit log is written. `fixtures.Validate` itself is not changed.
   - Collision split: an `id` or `code` that matches a `workflows` row is the step 3 `409`. An `id` (top-level or nested) that collides with any other fixture-owned entity is a step 4 `400`.
5. Folder scope: resolve `folderCode` to the folder ID. A restricted `workflow` grant that does not cover that folder gives `403 missing_permission`.
6. Persist: insert the row with the fixture loader's column mapping (`id, code, status, folder_id, definition_json, created_at, updated_at`), taking timestamps from the injected clock. Write the audit log `workflow.create` `{workflowId}` in the same transaction, as the other admin mutations do.
7. Respond `201` with the WorkflowDetail presenter output that `GET /v1/workflows/{workflowId}` returns.

The insert writes only the `workflows` row, exactly as the fixture loader does. Custom numbering counters, category membership (`categoryCodes` inside the definition JSON) and route resolution are all derived from that row at read or ticket-creation time. As a result:
- The created workflow appears immediately in `GET /v1/workflows` and `GET /v1/workflows/{workflowId}`.
- A `visible` workflow can be used with `POST /v1/tickets` by a user it is available to.
- Fixture export includes the created workflow, and fixture reset removes it like any other runtime data.

Out of scope (#523 does not need them): `GET /admin/api/workflows` would duplicate `GET /v1/workflows`, and `DELETE /admin/api/workflows/{id}` would need rules for tickets that reference the workflow. Neither is added, so both paths keep the reserved-namespace 404.

Required tests (`internal/admin/workflows_test.go`):
- 201 with the default fixture admin token. Afterwards the workflow is listed and fetchable through `/v1/workflows`, and a ticket can be created on it.
- 401 without a token.
- 403 with a token that lacks `workflow` (fixture `pf_pat_dev_employee`).
- 400 for malformed JSON, an unknown field, and an unknown `routeCode` or `folderCode`.
- 400 for an invalid form custom validation (for example `combinationType: "bogus"`), reported as `workflows[n]...combinationType`, and for an invalid detail (slip) custom validation.
- 400 when a candidate exceeds the custom validation limit (51 form validations, or 11 detail validations).
- 400 when the candidate workflow `id` or a nested ID equals the ID of an entity registered in a later fixture file (for example a fixture token ID, reported against `tokens.json`), and when it equals the ID of an earlier entity (for example a user ID).
- After every 400 and 409 case: `GET /v1/workflows` shows no new workflow (count and code unchanged), and the audit log has no `workflow.create` entry.
- Unrelated-state tolerance: the test first writes to the store directly to create a baseline validation problem unrelated to the candidate. For example, it inserts a webhook whose `workflow_ids` names a nonexistent workflow ID, so the export yields an empty code and `webhooks.json` reports `unknown workflow code`. The test asserts the baseline `fixtures.Validate` fails, then checks that a valid candidate still gets 201.
- 409 for a duplicate `code` and for a duplicate `id`.
- `routes_test.go` lists `createWorkflow`.

### Required tests

| Package | Tests |
|---|---|
| `internal/auth` | PAT/SAT resolution; every INF-05/INF-06 case; permission denial; tenant modes; IP allowlist; CSRF required for cookie mutations and not for bearer; session idle/absolute expiry with fake clock; OIDC cases above; local login disabled/enabled |
| `internal/httpx` | 411, content type, invalid JSON, size limit, pagination headers and Link relations, sortBy parsing, array query parsing, rate limit headers/429/secret multiplier, CORS preflight, fault injection matching and `remaining` countdown |
| `internal/admin` | Every admin endpoint: permission checks, token shown once, reset requires the flag, export/reset round-trip, createWorkflow 201/400/401/403/409 and `/v1` visibility |

## References

- `design-docs/references/kickflow-api.md` 1.2, 1.5, 1.7, 5.6, 6.2
- `design-docs/specs/notes.md` INF-04 to INF-07, INF-26 to INF-32
- `design-docs/specs/design-webhooks.md`
