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

## Continuation (session 462) - read first; overrides the older Verification and Done Criteria

**Phase**: 3. **Depends on**: PF-04, PF-05, PF-06, PF-07, PF-08, PF-09, PF-10.
**State at 410d3d8**: not started. `internal/server`, `internal/webui`, `internal/admin` and `internal/testutil` do not exist; `internal/cli/cli.go` is the 31-line PF-01 skeleton (PF-01 is accepted; keep its `Run` signature and its existing `TestRunVersion`/`TestRunPlaceholdersAndUsage` cases green, adjusting only placeholder expectations that this plan replaces with real subcommands). Implement this plan as written, with these session-462 corrections:

- The default round trip (`internal/fixtures/default_roundtrip_test.go`, test name `TestDefaultRoundTrip`) now depends on PF-06 (input IDs, `inputs: []`) and PF-07 (canonical enum data, merged conditional section). If bytes differ, follow the regeneration procedure above. Regeneration must not reintroduce banned values: run the PF-07 rg check after regenerating.
- Persona permissions come from the corrected `roles.json`. Admin (Super Admin) holds all 19 kickflow permissions, so tests needing `user`, `team`, `role`, `audit`, `tenant` or `integration` use Admin. Do not rely on the removed invented permissions.
- The admin skeleton `Operations()` has 23 rows. Webhook event types are validated with `model.WebhookEventTypes`; the dispatcher statuses are `pending|succeeded|failed`.
- `Serve` owns the dispatcher goroutine. The CLI smoke test (PF-26) depends on graceful exit 0; never start a background process from tests.

### Verification

```bash
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf16-s462-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... ./internal/fixtures/... 2>&1 | tee /tmp/pf16-s462-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... 2>&1 | tee /tmp/pf16-s462-test.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'TestDefaultRoundTrip' ./internal/fixtures/... 2>&1 | tee /tmp/pf16-s462-roundtrip.log"; echo "exit=$?"
for f in /tmp/pf16-s462-test.log /tmp/pf16-s462-roundtrip.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/server internal/webui internal/admin internal/cli internal/testutil internal/fixtures/default_roundtrip_test.go   # prints nothing
bash -o pipefail -c 'go run ./cmd/punchflow version 2>&1 | tee /tmp/pf16-s462-version.log'; echo "exit=$?"   # "punchflow 0.1.0"
```

Evidence: exit 0 for each command; FAIL=0 in both logs. In the main log, SKIP may be 1, and only for `TestEmbeddedBuild` (the web build is absent until PF-27; that skip is expected). Any other SKIP fails the check. `TestDefaultRoundTrip` must PASS.

### Done Criteria (session 462)

- [ ] Contracts exist; server assembled as specified; CLI subcommands, flags and env vars match `command.md`
- [ ] Uncached `-race -v` tests of the five PF-16 packages pass; `TestDefaultRoundTrip` passes (Validate nil; byte-identical export); counts recorded
- [ ] build/vet exit 0; gofmt clean; files < 1000 lines

## Continuation (session 463) - read first; overrides the session-462 phase, log names and Done Criteria

**Wave**: A (s463; parallel with PF-14 and PF-15). **Depends on**: PF-04..PF-10, all accepted at f82e374. PF-17 (OIDC) is also accepted, so `auth.NewOIDC`, `Login` and `Callback` are real implementations: wire them as specified and do not stub them.
**State at f82e374**: not started. `internal/server`, `internal/webui`, `internal/admin` and `internal/testutil` do not exist. `internal/cli` holds only the PF-01 `cli.go` and `cli_test.go`.
**Path Declarations**: unchanged. 24 writePaths, plus the shared `internal/cli/cli.go`, `internal/cli/cli_test.go` and `fixtures/default` (15 files), give 41 entries (< 300). This is the only remaining plan with a directory entry.

1. Implement the whole plan above, with all session-462 corrections.
2. Shared accepted files:
   - `internal/cli/cli.go` and `internal/cli/cli_test.go` (PF-01, from the s460 manifest): keep the `Run` signature, `TestRunVersion` and `TestRunPlaceholdersAndUsage`.
   - `fixtures/default/*` (PF-07): regenerate only by the atomic procedure above, and only if `TestDefaultRoundTrip` fails after semantic equality is proven. After regenerating, run the CF-01 ban grep in `design-docs/specs/notes.md`; it must print nothing.

   Record pre/post hashes for each of these files.
3. Keep the contracts that wave B builds on stable, and record them in the progress log as you create them: the 23 admin `Operations()` rows in `internal/admin/routes.go`, the admin stub files PF-24 will fill (`tokens.go`, `service_accounts.go`, `webhooks.go`, `tenant.go`, `faults.go`, `fixtures.go`), the `testutil` API (`Env`, personas, `Do`, `CallerFor`, receiver, `DeliverWebhooks`) and `server.New`/`Serve`. Do not change them after wave A.
4. `TestEmbeddedBuild` may SKIP in your run, because `web/dist/app` is built only by PF-27. Never run `bun run build` into `web/dist/app` and never write `./punchflow` (README rule 7).
5. Accepted code is read-only: `internal/auth/*`, `internal/httpx/*`, `internal/api/v1/*`, `internal/webhook/*`, `internal/fixtures/*` (except your new `default_roundtrip_test.go`), `internal/store/*`, `embed.go`, `cmd/punchflow/main.go`. When blocked: record a reconciliation request (README rule 12).
6. Verification: the session-462 block with `-s462-` changed to `-s463-` (the manifest `verification` array). Copy each log to `tmp/punchflow-kickflow-dummy-s463/PF-16/attempt-<n>/`.

### Done Criteria (session 463)

- [ ] Every session-462 Done Criteria item is proven by `-s463-` logs: exit 0, FAIL=0, the only allowed SKIP is `TestEmbeddedBuild`, `TestDefaultRoundTrip` PASS, and the version log prints `punchflow 0.1.0`
- [ ] The progress log lists the wave-B contracts (admin Operations count 23, testutil exported names) and pre/post hashes of the shared files

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md`. Shared paths: `internal/cli/cli.go`, `internal/cli/cli_test.go` (from PF-01), `fixtures/default` (from PF-07; regenerate only as described).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
