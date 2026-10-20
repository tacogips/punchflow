# PF-05 Auth Core Implementation Plan

**Status**: Ready
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

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf05-build.log
go vet ./internal/auth/... ./internal/store/... 2>&1 | tee /tmp/pf05-vet.log
go test -race ./internal/auth/... 2>&1 | tee /tmp/pf05-test.log
gofmt -l internal/auth internal/store/auth.go   # prints nothing
```

## Done Criteria

- [ ] Contract symbols and store functions exist with the exact names; `oidc.go` stub compiles and returns nil OIDC for an empty issuer
- [ ] All tests pass with `-race`; gofmt clean

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`.

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
