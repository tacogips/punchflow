# PF-16 Server, CLI and Test Utilities Implementation Plan

**Status**: Ready
**Wave**: 3 | **Depends On**: PF-04, PF-05, PF-06, PF-07, PF-09, PF-10 | **Plan ID**: PF-16
**Design Reference**: design-docs/specs/architecture.md#request-handling, #web-ui-web (serving), #testing-and-verification; design-docs/specs/command.md (all); design-docs/specs/design-api-coverage.md (middleware order); design-docs/specs/design-auth-admin.md#dev-admin-api-internaladmin
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

Assemble the single binary: the HTTP server (mux, middleware chains, SPA, healthz, dispatcher lifecycle), the CLI subcommands from `command.md`, the `/admin/api` skeleton (route table + stubs that PF-24 fills), and `internal/testutil`, which every wave-4 handler test uses. Also run full validation and the byte-exact export round-trip on `fixtures/default`.

## Non-goals

- No /v1 handler logic (wave 4), no admin handler logic (PF-24), no OIDC logic (PF-17), no smoke test (PF-26), no README (PF-25).
- Do not change `internal/api/v1/*`, `internal/auth/*`, `internal/webhook/*`, `internal/httpx/*`, `internal/fixtures/{parse,validate,load,export}.go`.

## Files

Server, webui, admin skeleton, CLI subcommands, testutil and the default round-trip test. The exact paths are listed in "Path Declarations" below, which is authoritative.

## Path Declarations

### writePaths
- `impl-plans/active/pf-16-server-cli-testutil.md`
- `internal/server/server.go`
- `internal/server/server_test.go`
- `internal/webui/webui.go`
- `internal/webui/webui_test.go`
- `internal/admin/admin.go`
- `internal/admin/routes.go`
- `internal/admin/routes_test.go`
- `internal/admin/tokens.go`
- `internal/admin/service_accounts.go`
- `internal/admin/webhooks.go`
- `internal/admin/tenant.go`
- `internal/admin/faults.go`
- `internal/admin/fixtures.go`
- `internal/cli/config.go`
- `internal/cli/serve.go`
- `internal/cli/fixtures.go`
- `internal/cli/token.go`
- `internal/testutil/env.go`
- `internal/testutil/personas.go`
- `internal/testutil/receiver.go`
- `internal/testutil/assert.go`
- `internal/testutil/testutil_test.go`
- `internal/fixtures/default_roundtrip_test.go`

### sharedPaths
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `fixtures/default`

### sharedPathNotes
- `internal/cli/cli.go`: created by PF-01 as a skeleton; replace dispatch with the real subcommands, keep the `Run` signature
- `internal/cli/cli_test.go`: created by PF-01; extend with the CLI cases listed below, keep the existing version/usage cases
- `fixtures/default`: created by PF-01 (.gitkeep) and PF-07 (JSON files); regenerate atomically only if the byte round-trip fails, after proving semantic equality

### artifactRoots
- (none)

## Contracts

```go
// internal/server
type Config struct {
    Listen, PublicBaseURL, DataDir, DBPath string
    FixturesFS fs.FS; Seed, EnableLocalLogin, EnableAdminReset bool
    OIDC auth.OIDCConfig; RateLimit int; RateLimitSecret string
    WebhookRetryBase time.Duration; MaxUploadBytes int64
}
type Server struct{ DB *store.DB; Webhooks *webhook.Service; Faults *httpx.FaultSet /* ... */ }
func New(ctx context.Context, cfg Config, clock kftime.Clock) (*Server, error) // open DB, seed if empty && Seed, build services and handler
func (s *Server) Handler() http.Handler
func (s *Server) Serve(ctx context.Context, ln net.Listener) error             // runs the HTTP server and webhooks.Run; graceful shutdown on ctx done
func (s *Server) Close() error

// internal/admin
type Deps struct {
    DB *store.DB; Auth *auth.Authenticator; Webhooks *webhook.Service; Faults *httpx.FaultSet
    Clock kftime.Clock; FixturesFS fs.FS; FilesDir string; EnableReset bool; BaseURL string
}
type API struct{ d Deps }
func New(d Deps) *API
type Operation struct{ Name, Method, Pattern string; Handle func(a *API, w http.ResponseWriter, r *http.Request) }
func Operations() []Operation // the 23 rows of the admin table in design-auth-admin.md, Name = lowerCamel e.g. "listTokens", "createWebhook", "resetFixtures"
func (a *API) Register(mux *http.ServeMux) // + "/admin/api/" catch-all -> 404 endpoint_not_found

// internal/testutil
type Env struct{ URL string; Server *server.Server; DB *store.DB; Clock *kftime.FakeClock; Receiver *Receiver }
func New(t *testing.T, opts ...Option) *Env         // temp dir, default fixtures, httptest server, dispatcher NOT running
func WithLocalLogin() Option; func WithAdminReset() Option; func WithRateLimit(n int, secret string) Option
func (e *Env) Do(t *testing.T, persona Persona, method, path string, body any) *Response // JSON body; persona "" = no auth
func (e *Env) DoRaw(t *testing.T, req *http.Request) *Response
func (e *Env) DeliverWebhooks(t *testing.T) int      // Server.Webhooks.RunOnce
func (e *Env) RegisterReceiverWebhook(t *testing.T, secret string, eventTypes ...string) string // inserts a webhook pointing at e.Receiver
type Persona string // Admin, Manager, Finance, Employee, Integration (SAT; Do adds X-Caller-Id of CallerFor)
func (e *Env) UserID(t *testing.T, code string) string
type Response struct{ Status int; Header http.Header; Body []byte }
func (r *Response) JSON(t *testing.T, v any)
func AssertError(t *testing.T, r *Response, status int, code string)
func AssertPagination(t *testing.T, r *Response, page, perPage, total int)
func AssertKeys(t *testing.T, obj map[string]any, keys ...string) // exact key set
type Receiver struct{ /* httptest server; mutex-guarded []Received; configurable status sequence */ }
type Received struct{ Header http.Header; Body []byte }
func (r *Receiver) Requests() []Received; func (r *Receiver) RespondWith(statuses ...int)
```

Persona tokens and users are pinned in PF-07: Admin `pf_pat_dev_admin` (admin), Manager `pf_pat_dev_manager` (sales-manager), Finance `pf_pat_dev_finance` (finance-manager), Employee `pf_pat_dev_employee` (sales-rep), Integration `pf_sat_dev_integration` with `X-Caller-Id` = the `admin` user unless `Env.CallerFor` is changed.

## Server Assembly

- `/v1/` -> `httpx.Chain(v1mux, RequestID, Recover, CORS, faults.Middleware, LengthRequired, rateLimiter.Middleware, authn.Middleware)`; `v1.New(Deps{...}).Register(v1mux, identity)`.
- `/admin/api/` -> Chain(adminmux, RequestID, Recover, LengthRequired, authn.Middleware).
- `/auth/` -> `authn.Register(authmux, RoutesOptions{EnableLocalLogin, SecureCookie: https base URL, OIDC: auth.NewOIDC(...)})` wrapped with RequestID and Recover.
- `/files/download/` -> `files.New(...).DownloadHandler(db)`; `files.New` key = `store.ServerSecret(ctx, db.Q(), "files")`.
- `GET /healthz` -> `{"status":"ok"}`; everything else -> `webui.Handler(punchflow.WebDist)`.
- Engine = `engine.New(clock)`; Webhooks = `webhook.New(db, webhook.NewPayloads(), clock, nil, Config{RetryBase})`; v1 `Events` = the webhook service.
- Seeding: when `cfg.Seed` and `db.IsEmpty` -> `fixtures.Load(ctx, db, cfg.FixturesFS, clock)`; default `FixturesFS` = `fixtures.DefaultFS(punchflow.DefaultFixtures)`.
- Log warnings at startup when local login or admin reset is enabled.

## CLI (`command.md` is the spec)

- `serve`: parse flags, apply env `PUNCHFLOW_<UPPER_SNAKE>` when the flag was not set, build `server.Config`, listen (`--listen`, port 0 allowed), print `listening on http://<actual addr>` to stderr, `Serve` until ctx done -> exit 0. Defaults: listen `127.0.0.1:8080`; data dir `.punchflow-data`; db `<data-dir>/punchflow.sqlite`; public base URL `http://localhost:<port>` for loopback listens, else `http://<listen>`.
- `fixtures load|reset|export` (`--out DIR` required for export), `token issue --user <id|code|email> | --service-account <name> [--name]`: prints only the token + newline to stdout; `version`.
- Exit codes: 0 ok; 1 runtime error (message to stderr); 2 usage (unknown subcommand/flag, invalid value, missing required flag, both/neither of `--user`/`--service-account`).

## webui

`func Handler(dist fs.FS) http.Handler`: serve from `fs.Sub(dist, "web/dist/app")` when `index.html` exists; GET/HEAD of an existing file -> the file (`Cache-Control: public, max-age=31536000, immutable` under `/assets/`); any other GET -> `index.html` (`Cache-Control: no-cache`); when not built -> 200 placeholder HTML "punchflow UI is not built. Run `mise run web:build`." Non-GET -> 404 `endpoint_not_found`.

## Key Points and Pitfalls

- Reserved prefixes must never fall through to the SPA: `/v1/x` unknown -> JSON 404 from the v1 catch-all (after auth), never `index.html`.
- `Serve` owns the dispatcher goroutine: start `webhooks.Run(ctx)` and wait for it on shutdown (`http.Server.Shutdown` with a 10 s timeout). No orphan goroutines.
- testutil must not start the dispatcher; tests call `DeliverWebhooks`. Use `t.TempDir()` and `t.Cleanup` to close the server and DB.
- testutil handler tests are external (`package v1_test`, `package admin_test`) to avoid import cycles (testutil -> server -> api/v1).

## Tests (input -> expected)

- server (stub-independent; these tests must stay green after wave 4 replaces the handler bodies): `GET /healthz` 200; `GET /v1/user` without token -> 401 `invalid_access_token`; with the Admin token -> status is neither 401 nor 404 (proves routing plus auth, whatever the handler returns); `GET /v1/nope` with Admin -> 404 `endpoint_not_found`; `OPTIONS /v1/users` preflight 200; `GET /tickets/123` -> placeholder or index HTML (never JSON); `POST /admin/api/fixtures/reset` with Admin and `EnableAdminReset` true -> status neither 401 nor 404; with `EnableAdminReset` false -> 404 `endpoint_not_found` is NOT asserted here (PF-24 owns that behavior). Never assert a 500 status or a "not implemented" message
- admin `routes_test`: 23 operations, unique patterns, unknown `/admin/api/x` -> 404
- webui with `fstest.MapFS` containing `web/dist/app/index.html` and `web/dist/app/assets/a.js` -> asset served with immutable cache header; `/deep/link` -> index.html; without index -> placeholder
- `TestEmbeddedBuild` in `internal/webui/webui_test.go`: `t.Skip` unless `web/dist/app/index.html` exists on disk (path from `runtime.Caller` -> module root); otherwise assert `Handler(punchflow.WebDist)` serves `/` with a `<script` tag. PF-27 runs it after the real web build
- cli: `version` 0; `serve --listen bad` -> 1; `token issue` without target -> 2; `token issue --user admin --db tmp` -> stdout matches `^pf_pat_[0-9A-Za-z]{32}\n$`; `fixtures load` twice on the same DB -> second exits 1; `fixtures export --out dir` writes 14 files; env `PUNCHFLOW_LISTEN` respected when the flag is absent and overridden when present
- default round-trip (`internal/fixtures/default_roundtrip_test.go`): `Validate(Parse(default))` nil; `Export(Load(default))` byte-identical to every `fixtures/default/*.json`. If not identical, regenerate: run `go run ./cmd/punchflow fixtures export --db <tmp db> --out <tmp dir outside the repo>`, confirm that `Parse` of the old and new sets gives equal `Set` values (reflect.DeepEqual), then replace each `fixtures/default/<name>.json` atomically (`cp` to `fixtures/default/<name>.json.tmp`, then `mv` over the target) so that PF-14, which loads these files in the same wave, never reads a partial file. Log the regeneration and re-run.
- testutil self-test (`internal/testutil/testutil_test.go`): for each persona, `Do(p, GET, /v1/user)` returns a status other than 401 (handler-body independent); with persona "" it returns 401. `Env.CallerFor` (user code, default `admin`) selects the `X-Caller-Id` for the Integration persona.

## Verification

```bash
go build ./... 2>&1 | tee /tmp/pf16-build.log
go vet ./... 2>&1 | tee /tmp/pf16-vet.log
go test -race ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... ./internal/fixtures/... 2>&1 | tee /tmp/pf16-test.log
gofmt -l internal/server internal/webui internal/admin internal/cli internal/testutil internal/fixtures/default_roundtrip_test.go   # prints nothing
go run ./cmd/punchflow version 2>&1 | tee /tmp/pf16-version.log   # "punchflow 0.1.0"
```

## Done Criteria

- [ ] Contracts exist; server assembled exactly as above; CLI subcommands, flags and env vars match `command.md`; all tests pass with `-race`; default fixtures validate and round-trip byte-identically

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. Shared paths: `internal/cli/cli.go`, `internal/cli/cli_test.go` (from PF-01), `fixtures/default` (from PF-07; regenerate only as described).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
