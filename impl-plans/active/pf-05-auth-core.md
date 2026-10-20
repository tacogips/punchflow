# PF-05 Auth Core Implementation Plan

**Status**: Completed
**Wave**: 2 | **Depends On**: PF-01, PF-02 | **Plan ID**: PF-05
**Design Reference**: design-docs/specs/design-auth-admin.md#token-authentication, #browser-sessions, #local-login-fallback-dev-only; design-docs/specs/notes.md INF-04, INF-05, INF-06, INF-31, INF-42; design-docs/specs/design-api-coverage.md (middleware order, tenant modes)
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Authenticate every `/v1` and `/admin/api` request, either by Bearer token (PAT, or SAT plus `X-Caller-Id`) or by UI session cookie with a CSRF header. Expose the effective user and permissions to handlers, enforce the tenant `suspended`/`maintenance` modes and the IP allowlist, and update `lastUsedOn`. Also provide the `/auth/session`, `/auth/logout` and `/auth/local/login` endpoints, plus an OIDC stub whose API PF-17 implements.

## Non-goals

- No OIDC logic (PF-17 replaces `oidc.go`); no admin token endpoints (PF-24 uses `IssueToken`).
- `admin_working` is NOT checked here (engine, INF-41). Role `restricted` scoping is not enforced (DIV-08).
- Do not import `internal/httpx` (same wave). Use `kferr.Write` for errors and a small local JSON decode for local login.

## Files

`internal/auth/principal.go`, `token.go`, `middleware.go`, `session.go`, `handlers.go`, `oidc.go` (stub), and tests `token_test.go`, `middleware_test.go`, `session_test.go`, `handlers_test.go`; `internal/store/auth.go` (token, session, oidc_state, tenant and user-auth queries).

## Path Declarations

### writePaths
- `impl-plans/active/pf-05-auth-core.md`
- `internal/auth/principal.go`
- `internal/auth/token.go`
- `internal/auth/middleware.go`
- `internal/auth/session.go`
- `internal/auth/handlers.go`
- `internal/auth/oidc.go`
- `internal/auth/token_test.go`
- `internal/auth/middleware_test.go`
- `internal/auth/session_test.go`
- `internal/auth/handlers_test.go`
- `internal/store/auth.go`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts

```go
type Principal struct {
    User        model.User
    Permissions map[string]bool // union of role permission names
    Via         string          // "pat" | "sat" | "session"
    TokenID     string          // api_tokens.id when Via != session
    CSRFToken   string          // session only
}
func (p *Principal) Has(perms ...string) bool             // true if any is held
func FromContext(ctx context.Context) (*Principal, bool)
func WithPrincipal(ctx context.Context, p *Principal) context.Context
func Require(r *http.Request, perms ...string) error       // kferr.MissingPermission when none held; no principal -> InvalidAccessToken

type Authenticator struct{ /* db *store.DB, clock kftime.Clock */ }
func NewAuthenticator(db *store.DB, clock kftime.Clock) *Authenticator
func (a *Authenticator) Middleware(next http.Handler) http.Handler
func HashToken(token string) string                        // hex sha256
func NewTokenValue(kind string) string                     // "pf_pat_"/"pf_sat_" + 32 base62 (crypto/rand)
func (a *Authenticator) IssueToken(ctx context.Context, q store.Querier, kind, userID, name, description string) (plaintext, id string, err error)
func (a *Authenticator) CreateSession(ctx context.Context, q store.Querier, userID string) (cookieValue, csrf string, err error)
func (a *Authenticator) ActivateOnLogin(ctx context.Context, q store.Querier, user model.User) (model.User, error) // INF-31; inactive -> error
const SessionCookie = "punchflow_session"; const CSRFHeader = "X-Punchflow-CSRF"

type RoutesOptions struct { EnableLocalLogin bool; SecureCookie bool; OIDC *OIDC }
func (a *Authenticator) Register(mux *http.ServeMux, opt RoutesOptions)
// registers: GET /auth/session, POST /auth/logout, POST /auth/local/login,
//            GET /auth/oidc/login, GET /auth/oidc/callback (delegate to opt.OIDC; nil -> 404 endpoint_not_found)

// oidc.go (stub; PF-17 replaces bodies, keeps signatures)
type OIDCConfig struct{ Issuer, ClientID, ClientSecret, RedirectURL string }
type OIDC struct{ /* unexported */ }
func NewOIDC(ctx context.Context, cfg OIDCConfig, a *Authenticator, db *store.DB, clock kftime.Clock, secureCookie bool) (*OIDC, error) // Issuer "" -> (nil, nil)
func (o *OIDC) Login(w http.ResponseWriter, r *http.Request)
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request)
```

Store functions in `internal/store/auth.go` (all take `store.Querier`): `TokenByHash`, `TouchToken`, `InsertToken`, `UserByID`, `UserByEmail`, `UserPermissions(userID) (map[string]bool, error)`, `InvitationByUser`, `ActivateUser`, `TouchLastUsedOn`, `InsertSession`, `SessionByHash`, `TouchSession`, `DeleteSession`, `Tenant`, `InsertOIDCState`, `TakeOIDCState` (select and delete). Keep the names; PF-17 and PF-24 use them.

## Behavior (order inside `Middleware`)

1. `Authorization: Bearer x`: unknown hash -> 401 `invalid_access_token`. `kind=sat`: no `X-Caller-Id` -> 401 `invalid_caller_id`; not a UUID -> 400 `invalid_header`; unknown user -> 401 `invalid_caller_id`. `kind=pat`: ignore `X-Caller-Id` (INF-05). Update `last_used_at`.
2. No Authorization: session cookie -> look up `sha256(cookie)`. Expired (idle > 12 h or age > 24 h) -> delete it and treat as missing. For POST/PATCH/DELETE, `X-Punchflow-CSRF` must equal the session's CSRF token -> else 403 `missing_permission` "csrf token mismatch". Missing both -> 401 `invalid_access_token`.
3. Effective user state (INF-06): `invited` + invitation unexpired -> 401 `user_not_registered`; `invited` + expired/no invitation -> 401 `invitation_expired`; `suspended`/`deactivated` -> 401 `user_inactive`.
4. Tenant: mode `suspended` -> 403 `tenant_suspended`; `maintenance` -> 403 `tenant_maintenance`; non-empty `ip_allowlist` (CIDRs or plain IPs, parsed with `net/netip`) not containing the RemoteAddr host -> 403 `ip_address_denied`.
5. `last_used_on` = JST date of now, written only when different (at most once per day).
6. Put the Principal in the context and call next.

Handlers:
- `GET /auth/session` -> 200 `{user, permissions (sorted), csrfToken, authMethods:{oidc: OIDC!=nil, local: EnableLocalLogin}}`; `user` uses the kickflow User field names (build a minimal local JSON struct; do not import `internal/present`). No session -> 401 `invalid_access_token`.
- `POST /auth/logout` -> delete the session, expire the cookie, 200 empty.
- `POST /auth/local/login {email}` -> 404 `endpoint_not_found` when disabled. Unknown e-mail -> 401 `invalid_access_token`. Otherwise `ActivateOnLogin` (invited + valid -> activated; suspended/deactivated -> 401 `user_inactive`), create the session, set the cookie (`HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` per option), 200 with the same body as `/auth/session`.

## Key Points and Pitfalls

- Store only `sha256` of tokens and session ids; compare hashes, never raw values. Do not validate the token format (fixture tokens are low-entropy, INF-04).
- `UserPermissions` = union over the user's `role_memberships` -> `roles.permission_list[].permission`.
- All DB access for one request uses `db.Q()` (no tx is open in middleware). Never call `db.Q()` inside a `WithTx` callback.
- Never log tokens, cookies or CSRF values.

## Tests (input -> expected)

Insert a tenant, users (activated, invited valid, invited expired, suspended), roles and tokens directly with SQL in test helpers (fixtures are not available in this wave).
- no header -> 401 invalid_access_token; bad token -> 401 invalid_access_token
- SAT without caller id -> 401 invalid_caller_id; `X-Caller-Id: abc` -> 400 invalid_header; unknown UUID -> 401 invalid_caller_id; valid -> principal is the caller, Via sat
- PAT with X-Caller-Id of another user -> principal is the token owner
- PAT of invited (valid) -> 401 user_not_registered; expired -> 401 invitation_expired; suspended -> 401 user_inactive
- tenant suspended -> 403 tenant_suspended; maintenance -> 403 tenant_maintenance; allowlist `10.0.0.0/8` with RemoteAddr 192.0.2.1 -> 403 ip_address_denied
- `Require(r, "user")` without the permission -> 403 missing_permission
- session cookie GET ok; session POST without CSRF -> 403; with CSRF -> ok; Bearer POST never needs CSRF
- fake clock +13 h idle -> 401; touched every hour for 25 h -> 401 (absolute limit)
- local login disabled -> 404; enabled -> Set-Cookie and session body; invited user becomes activated
- lastUsedOn written once per day (two requests same day -> one UPDATE)
- `NewTokenValue("pat")` matches `^pf_pat_[0-9A-Za-z]{32}$`

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 2. **Depends on**: PF-02 (canonical `model.Permissions`, `UserStatusActive = "activated"`, `UserTypeNormal`).
**Design**: `design-docs/specs/notes.md` CF-03 (mid) and CF-01; `design-api-coverage.md` enum contract.

### Why

- CF-03: the active-status correction is confirmed in code (`internal/store/auth.go:161` writes `model.UserStatusActive`; `internal/auth/session.go:17,30`; the tests use `activated`). It closes only with uncached PF-05 evidence.
- CF-01: `internal/auth/token_test.go:75,89` seeds and asserts the invented permission `user_read`.

### File-level changes

1. `internal/auth/token_test.go`: replace `user_read` with the kickflow permission `user` (both the seed JSON at ~75 and the expected list at ~89). Expected union stays 3 values: `user`, `ticket_write`, `integration`.
2. Re-read `internal/auth/*_test.go` and `internal/store/auth.go` for any other user-status, user-type or permission literal. Every one must be a value from the design enum table; use `model.*` constants where the file already imports `model`. No production behavior change.
3. Do not edit `internal/auth/oidc.go` (PF-17 replaces it in phase 3).

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf05-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/auth/... 2>&1 | tee /tmp/pf05-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/auth/... 2>&1 | tee /tmp/pf05-s462-test.log'; echo "exit=$?"
echo "PASS=$(grep -c -- '--- PASS' /tmp/pf05-s462-test.log) FAIL=$(grep -c -- '--- FAIL' /tmp/pf05-s462-test.log) SKIP=$(grep -c -- '--- SKIP' /tmp/pf05-s462-test.log)"
gofmt -l internal/auth internal/store/auth.go   # prints nothing
rg -n '"active"|user_read|"member"' internal/auth internal/store/auth.go   # prints nothing (exit 1)
```

Evidence: exit 0; FAIL=0; SKIP=0; PASS >= 22 (16 tests + 6 subtests in session 460). The log shows `TestSessionActivationRequiresValidInvitation` and `TestHandlersLocalLoginAndOIDCDisabled` passing.

### Done Criteria (session 462)

- [ ] CF-03 closed: activation writes `activated` and the uncached auth tests pass with counts recorded
- [ ] No invented permission, user-status or user-type literal remains in PF-05 files (rg check empty)
- [ ] build/vet exit 0; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: 2026-10-20 PF-05 implementation
**Intent snapshot**: Added `internal/store/auth.go` with the declared token, user, permission, invitation, session, tenant and OIDC-state store contracts. Added PF-05 auth principal, token/session creation, middleware, browser session/local-login handlers, and OIDC stubs under `internal/auth/`, plus focused SQLite-backed tests. No paths outside PF-05 `writePaths` were edited; `go.mod` and `go.sum` remain owned by PF-02.
**Tasks Completed**: Implemented Bearer PAT/SAT resolution, SAT caller impersonation, session-cookie auth and CSRF checks, account/tenant/IP restrictions, daily last-used updates, role-permission union, local login/logout/session handlers, invited-user activation, and the empty-issuer OIDC seam. Tests cover token format/hash-only persistence, principal permission checks, PAT/SAT behavior, account and tenant errors, allowlist, CSRF, idle/absolute expiry, daily update behavior, local login/cookies/logout, invitation activation, permission union, and disabled OIDC.
**Verification**:
- `go build ./...` exit 0; complete log `/tmp/pf05-build.log`.
- `go vet ./internal/auth/... ./internal/store/...` exit 0; complete log `/tmp/pf05-vet.log`.
- `go test -race ./internal/auth/...` exit 0; complete log `/tmp/pf05-test.log`.
- `go test -race -count=1 -v ./internal/auth/...` exit 0; 16 PASS records including subtests, 0 failures; complete log `/tmp/pf05-test-uncached.log`.
- `gofmt -l internal/auth internal/store/auth.go` exit 0 with no output; complete log `/tmp/pf05-gofmt.log`.
- `TakeOIDCState` consumes state with one SQLite `DELETE ... RETURNING` statement; its regression test confirms the second consumption returns `store.ErrNotFound`.
- Required post-modification check agent reran all four commands after the atomic-consumption correction; all exit statuses were 0, logs `/tmp/pf05-postmodify-attempt2-{build,vet,test,gofmt}.log` (test result cached; the uncached verbose run above independently passed on current source).
- Pre-compact-JSON source hashes for all 11 PF-05 Go files are preserved in `tmp/punchflow-kickflow-dummy-s460/PF-05/attempt-1/source-final.sha256`; final source identity is in the later post-correction manifest below.
- Earlier shared-tree build errors in `internal/fixtures/load.go:295`, `internal/present/master.go:6`, and `internal/present/role.go:5` cleared before final successful build. Earlier test-fixture expectation failures were corrected; reproduced outputs are preserved in `tmp/punchflow-kickflow-dummy-s460/PF-05/attempt-1/prior-build-shared-tree-failure.log` and `prior-race-test-correction.log`.
- Final self-check found that the session handler used `json.Encoder.Encode`, adding a trailing newline contrary to INF-33. `writeSessionResponse` now uses `json.Marshal` and writes the compact body; `TestHandlersLocalLoginAndOIDCDisabled` asserts no trailing newline.
- After that correction, required post-modification checks passed: `go build ./...` exit 0 (`/tmp/pf05-final-postmod-build.log`); `go vet ./internal/auth/... ./internal/store/...` exit 0 (`/tmp/pf05-final-postmod-vet.log`); `go test -race -count=1 -v ./internal/auth/...` exit 0, 16 tests plus 6 subtests passed and 0 failures (`/tmp/pf05-final-postmod-test.log`); `gofmt -l internal/auth internal/store/auth.go` exit 0 with no output (`/tmp/pf05-final-postmod-gofmt.log`).
- Final source hashes, including the compact-JSON correction, are recorded in `tmp/punchflow-kickflow-dummy-s460/PF-05/attempt-1/source-final-compact-json.sha256`; `shasum -a 256 -c` verified all 11 entries.
**Reconciliation requests**: None. Independent test-integrity/adversarial review and final shared-tree integration review remain downstream workflow gates.

### Session: 2026-10-20 PF05-TIC-001 test-integrity repair
**Intent snapshot**: Added JSON `code` assertions for missing/bad bearer tokens and all three SAT caller cases in `internal/auth/middleware_test.go`; no production files changed.
**Tasks Completed**: Closed PF05-TIC-001 by decoding each error response and asserting `invalid_access_token`, `invalid_caller_id`, or `invalid_header` while retaining the original HTTP status assertions and cases.
**Verification**:
- `go test -race -count=1 -v ./internal/auth/...` exit 0; 16 tests and 6 subtests passed, 0 failures; complete log `/tmp/pf05-tic001-test.log`.
- `go vet ./internal/auth/... ./internal/store/...` exit 0; complete log `/tmp/pf05-tic001-vet.log`.
- `gofmt -l internal/auth internal/store/auth.go` exit 0 with no output; complete log `/tmp/pf05-tic001-gofmt.log`.
- `go build ./...` exit 0; complete log `/tmp/pf05-tic001-build.log`.
- Final 11-file source identity is recorded and verified in `tmp/punchflow-kickflow-dummy-s460/PF-05/attempt-2/source-final.sha256`.
**Reconciliation requests**: None. Independent re-review of PF05-TIC-001 and PF-27 combined-tree integration remain downstream workflow gates.
