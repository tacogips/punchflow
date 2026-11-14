# PF-16 Server, CLI and Test Utilities Implementation Plan

**Status**: Completed
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
bash -o pipefail -c 'go build ./... 2>&1 | tee /tmp/pf16-s463-build.log'; echo "exit=$?"
bash -o pipefail -c 'go vet ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... ./internal/fixtures/... 2>&1 | tee /tmp/pf16-s463-vet.log'; echo "exit=$?"
bash -o pipefail -c 'go test -count=1 -v -race ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... 2>&1 | tee /tmp/pf16-s463-test.log'; echo "exit=$?"
bash -o pipefail -c "go test -count=1 -v -run 'TestDefaultRoundTrip' ./internal/fixtures/... 2>&1 | tee /tmp/pf16-s463-roundtrip.log"; echo "exit=$?"
for f in /tmp/pf16-s463-test.log /tmp/pf16-s463-roundtrip.log; do echo "$f PASS=$(grep -c -- '--- PASS' $f) FAIL=$(grep -c -- '--- FAIL' $f) SKIP=$(grep -c -- '--- SKIP' $f)"; done
gofmt -l internal/server internal/webui internal/admin internal/cli internal/testutil internal/fixtures/default_roundtrip_test.go   # prints nothing
bash -o pipefail -c 'go run ./cmd/punchflow version 2>&1 | tee /tmp/pf16-s463-version.log'; echo "exit=$?"   # "punchflow 0.1.0"
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
- [x] The progress log lists the wave-B contracts (admin Operations count 23, testutil exported names) and pre/post hashes of the shared files

## Continuation (session 464) - read first; overrides the session-462/463 sections and the "Path Declarations" above

**Wave**: A (parallel with PF-14 and PF-15). **Depends on**: PF-04..PF-10, all accepted at `44e40cc`. PF-17 is accepted too.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s464-dispatch.json`. This plan's `writePaths` (36 files, `sharedPaths` empty) and `verification` arrays are authoritative.
**Design**: `design-docs/specs/architecture.md` "#fixtures" (canonical form, precise definition), "Shared-tree compile and ownership-transfer rule (session 464)" items 4-8; `design-docs/specs/command.md`; `design-docs/specs/notes.md` CF-04, CF-05, CF-06.

**Intent**: finish the server, CLI and testutil. Close CF-04 (default fixture round trip) and CF-05 (`DoRaw` RequestURI) so wave B can build on stable contracts. Resume from the partial files at `44e40cc`.

**Path changes vs s463**:
- Gained, transferred from accepted plans:
  - the 14 `fixtures/default/*.json` files (PF-07; replaces the old `fixtures/default` directory entry; `fixtures/default/.gitkeep` stays PF-01)
  - `internal/fixtures/load.go` and `internal/fixtures/export.go` (PF-06)
  - `internal/cli/cli.go` and `internal/cli/cli_test.go` (PF-01; now `writePaths`, no longer shared)
- Lost: `internal/admin/{tokens,service_accounts,webhooks,tenant,faults,fixtures}.go` now belong to PF-24. They exist as compiling stubs at `44e40cc`; do not edit them. PF-16 keeps `admin.go`, `routes.go` and `routes_test.go`.

**State at 44e40cc**:
- server, webui, admin skeleton (23 `Operations()` rows), CLI subcommands and testutil exist.
- s463 left two failures: `TestDefaultRoundTrip` fails on folders, proxy-applicants, proxy-approvers, routes and tenant (reviewer run, step 3), and `TestDoRawResolvesRelativeURLs` fails.
- The full race suite, build/vet and the version command were never verified.

**Tasks**:
1. Intent snapshot; pre-edit hashes of every existing file you will touch, including all 14 fixture files, `load.go` and `export.go`.
2. Baseline: run the manifest commands once (logs `-s464-baseline-*`).
3. CF-05, in `internal/testutil/env.go`:
   - `DoRaw` sends a clone of the request (`req.Clone(ctx)`), sets `RequestURI = ""` on the clone, and resolves a relative URL against `Env.URL`.
   - No testutil helper ever sets `Request.RequestURI` on a client request.
   - The existing test may keep using `httptest.NewRequest`.
4. CF-04 code, in `internal/fixtures/load.go` and `export.go` only:
   - `Load` and `Reset` write `tenant.created_at`/`updated_at` from `tenant.json` (`store.Millis(t.CreatedAt)`), using the load clock only when the value is zero.
   - `Export` returns those timestamps with `.UTC()` applied before marshaling.
   - Do not change export ordering, key order, other rows' timestamps, `internal/store/ids.go`, `internal/model/*` or `internal/fixtures/types.go`.
   - Imitate the existing insert/scan code at `load.go:100` and `export.go:24-27`.
5. CF-04 data:
   - Regenerate the 14 files with `go run ./cmd/punchflow fixtures export --db <tmp db> --out <tmp dir outside the repo>`, using a DB loaded from the current defaults.
   - Replace each `fixtures/default/<name>.json` atomically: `cp` to `<name>.json.tmp` in the same directory, then `mv` over the target. PF-14 loads these files in the same wave.
   - Then run the jq normalization check (manifest line with `/tmp/pf16-s464-jqnorm.log`). It must print `same` for all 14 files and give DIFF count 0. A DIFF means the change is not just reordering or dropping a zero value: stop and record a reconciliation request; never hand-edit to force equality.
   - Run the CF-01 ban grep (manifest).
6. Finish the remaining session-462 tests and contracts. Keep the wave-B contracts stable: `server.Config/New/Handler/Serve/Close`, the testutil exports listed in the s463 progress log, and the 23 admin `Operations()` rows.
7. Compile rule (README rule 13):
   - After every non-test `.go` write, run `go build ./...`.
   - After every `_test.go` write, run `go vet <pkg>`.
   - Log each result.

**Pitfalls**:
- Do not weaken `TestDefaultRoundTrip`. It stays byte-identical. Add a second-round assertion: `export(load(export))` equals the first export.
- `TZ=Asia/Tokyo` must not change the export bytes (manifest line `/tmp/pf16-s464-roundtrip-tz.log`).
- The whole `./internal/fixtures/...` package is your guard. PF-06's `load_orgcharts_test.go` (chart-local team ids), `fixtures_test.go`, `validate_test.go` and `enum_rules_test.go` must stay PASS. Do not edit those test files or `internal/fixtures/testdata/**`.
- Never write `./punchflow` or `web/dist/app`. `TestEmbeddedBuild` may SKIP; it is the only allowed SKIP.
- Unexported identifiers you add in shared packages start with `pf16`.

**Test cases (input -> expected)**: the "Tests" bullets above, plus:
- `DoRaw(httptest.NewRequest(GET, "/healthz", nil))` -> 200, no client error
- default round trip -> byte-identical on rounds 1 and 2
- `tenant.json` `createdAt` `2026-10-20T00:00:00Z` -> exported unchanged under `TZ=Asia/Tokyo`
- tenant fixture without timestamps -> load succeeds using the clock (unit test in `default_roundtrip_test.go` or `load.go`'s existing tests; add it to `default_roundtrip_test.go` as a pf16 subtest)

### Verification (session 464)

The manifest `verification` array for PF-16 (logs `/tmp/pf16-s464-*.log`), copied to `tmp/punchflow-kickflow-dummy-s464/PF-16/attempt-<n>/`. Evidence (final-source logs copied to `tmp/punchflow-kickflow-dummy-s464/PF-16/attempt-1/final/`):
- `go build ./...` exit 0; scoped `go vet` exit 0.
- Scoped server/webui/admin/cli/testutil race suite exit 0: PASS=22, FAIL=0, SKIP=0; includes `TestRunVersion`, `TestRunPlaceholdersAndUsage` and `TestDoRawResolvesRelativeURLs`.
- Whole fixtures race suite exit 1: PASS=35, FAIL=1, SKIP=0. `TestDefaultRoundTrip` and `TestTenantTimestampFallback` PASS. `TestLoadExportRoundTripMinimal` fails because `fixtures_test.go` still expects the fixture’s non-UTC `+09:00` tenant bytes, while CF-04 requires UTC export.
- Focused default round-trip suite exit 0: PASS=2, FAIL=0, SKIP=0; also passes under `TZ=Asia/Tokyo` (PASS=1).
- jq normalization prints 14 `same`, DIFF=0; CF-01 grep and frozen-file diff are clean; gofmt output is empty; version prints `punchflow 0.1.0`.
- Baseline and intermediate failed attempts, including the transient shared-tree `internal/store/read_tickets.go` undefined `pf08StringPtr` failure and initial evidence-copy contamination, are preserved in the same attempt directory; final build/vet and scoped tests pass after the shared compile recovered.

### Done Criteria (session 464)

- [x] CF-04 closed: `TestDefaultRoundTrip` PASS, also under `TZ=Asia/Tokyo`, with the second-round fixed point; jq normalization 14/14 `same`; CF-01 grep empty; `internal/model`, `internal/fixtures/types.go` and `internal/store/ids.go` unchanged versus `44e40cc`
- [x] CF-05 closed: `TestDoRawResolvesRelativeURLs` PASS
- [ ] Every manifest verification line passes with exit status, log path and counts; the only SKIP is `TestEmbeddedBuild`
- [ ] Wave-B contracts listed in the progress log are unchanged from `44e40cc`, or the change is recorded; the six admin stubs are untouched (hashes equal to `44e40cc`)
- [ ] Every source write followed by a logged compile gate with exit 0; gofmt clean; files < 1000 lines

## Continuation (session 465) - read first; overrides the session-462/463/464 sections and Path Declarations

**Wave**: A (with PF-14a, PF-14b, PF-14c, PF-15, PF-25). **Depends on**: PF-04..PF-10, all accepted at `ea385c7`.
**Manifest**: `impl-plans/active/punchflow-kickflow-dummy-s465-dispatch.json`. This plan has 51 `writePaths`, empty `sharedPaths`, and the manifest `verification` array is authoritative.
**Design**: `architecture.md` "#fixtures" (canonical form) and "Plan split, evidence-reporting and wave rule (session 465)" item 2; `notes.md` CF-04, CF-05, CF-09.

**Path changes vs s464** (transferred PF-06 -> PF-16, manifest `ownershipTransfers[]`):
- `internal/fixtures/fixtures_test.go`
- the 14 files `internal/fixtures/testdata/minimal/{categories,folders,general-masters,grades,organization-charts,proxy-applicants,proxy-approvers,roles,routes,tenant,tokens,users,webhooks,workflows}.json`, declared one by one. The `testdata` and `testdata/minimal` directories are never entries. `internal/fixtures/testdata/invalid/**` and `load_orgcharts_test.go`, `validate_test.go`, `enum_rules_test.go` stay PF-06 and are read-only.

**State at `ea385c7`**:
- CF-04 and CF-05 are closed (s464 log): `TestDefaultRoundTrip` PASS, also under `TZ=Asia/Tokyo`; jq normalization 14/14 `same`; `TestDoRawResolvesRelativeURLs` PASS; scoped server/webui/admin/cli/testutil race suite PASS=22 FAIL=0.
- Open: CF-09. The whole `./internal/fixtures/...` suite fails only in `TestLoadExportRoundTripMinimal` (`fixtures_test.go:56`), because `testdata/minimal/tenant.json` has `+09:00` timestamps while the export is UTC (`export.go:27`).
- Unchecked s464 Done items: every verification line in one final run; wave-B contracts unchanged; admin stubs untouched; compile-gate log.

**Tasks**:
1. Write the intent snapshot and the pre-edit hashes, including `fixtures_test.go` and the 14 minimal files.
2. Run the baseline verification (step names `baseline-*`).
3. CF-09 data fix:
   - In `internal/fixtures/testdata/minimal/tenant.json`, change `createdAt` and `updatedAt` from `2026-10-20T21:00:00+09:00` to `2026-10-20T12:00:00Z` (the same instant in UTC). Change nothing else in that file.
   - If any other minimal file still differs from the exporter output, regenerate only that file from the export (load the minimal set into a temp DB, export to a temp dir outside the repo, `cp` then `mv` atomically). The manifest's minimal jq check must still print `same` for the 13 non-tenant files.
4. `fixtures_test.go`: change it only if the UTC timestamp needs an assertion change. Keep the byte-identical comparison and the second-round fixed-point check. Never skip or delete a test.
5. Run the final verification in one uninterrupted run (step names `final-*`). Check the remaining s464 Done items:
   - `server.Config/New/Handler/Serve/Close`, the testutil exports and the 23 admin `Operations()` rows are unchanged from `ea385c7` (`git diff ea385c7 -- internal/server internal/testutil internal/admin/routes.go`; record that the diff is empty or the change is listed).
   - The six PF-24 admin stub files are unchanged (hashes equal to `ea385c7`).

**Pitfalls**:
- Never weaken UTC export or `TestDefaultRoundTrip`.
- Never edit `internal/fixtures/types.go`, `internal/model/*`, `internal/store/ids.go`, PF-06's other tests, or `testdata/invalid/**`.
- PF-14a/b/c and PF-15 load `fixtures/default` in tests during this wave. Do not change `fixtures/default/*.json` in s465 unless a verification line fails on them. Any change keeps ids and codes, and is written atomically.

### Done Criteria (session 465)

- [x] CF-09 closed: whole `go test -count=1 -v -race ./internal/fixtures/...` exit 0, FAIL=0, SKIP=0, with `--- PASS: TestLoadExportRoundTripMinimal` and `--- PASS: TestDefaultRoundTrip`, also under `TZ=Asia/Tokyo`
- [x] Minimal jq check: 13 `same` and DIFF count 0; tenant timestamps are `2026-10-20T12:00:00Z`
- [x] Every manifest verification line passes in one final run, with exit status, log path and counts; the only allowed SKIP is `TestEmbeddedBuild`
- [x] Wave-B contracts and the six admin stubs unchanged versus `ea385c7`; compile gate logged after every write; gofmt clean

## Execution Protocol

Evidence reporting (README rule 14; design architecture.md s465 rule 3):
1. The `verification` array holds only commands that are required to pass. An intentional negative or mutation proof (an expected non-zero exit, such as an `rg` ban grep with no match) never goes there. It goes into the `authorSelfCheck.checks` evidence text, with the command, the expected exit status, the observed exit status and the log path.
2. When assigned work remains, report `implementationIncomplete: true` and list the remaining items in `implPlanUpdates` and in this Progress Log. Never report remaining work as a High/Medium finding, a risk or a residual risk.
3. High/Medium labels are only for defects in delivered code, each with `file:line`. Sibling breakage is a blocker that names the owning plan, not a finding.
4. Every attempt ends with `go build ./...` exit 0 and the completed work recorded here.

Follow "Shared Execution Protocol" in `impl-plans/README.md` (rules 1-15). In session 464 every edited file is in this plan's `writePaths`: the transferred PF-01 CLI files, the PF-06 loader/exporter and the PF-07 fixture data. Record pre/post hashes and keep accepted tests green.

## Progress Log

### Session: 2026-10-21 (session 463, PF-16)
**Intent snapshot**: Implement PF-16 server assembly and tests, SPA handler, admin API route contract/stubs, CLI subcommands/config, testutil, and default fixture round-trip test. Preserve the shared CLI `Run` signature and fixture data. Evidence: `tmp/punchflow-kickflow-dummy-s463/PF-16/attempt-1/intent-001.txt` through `intent-014-plan-webui-evidence.txt` and `pre-node-snapshot.json`.
**Tasks Completed**: Added `server.New`/`Handler`/`Serve`/`Close`, real accepted PF-17 OIDC wiring, all middleware and reserved mounts, health endpoint and SPA handler; added 23 admin Operations rows and PF-24 stubs; added CLI serve/fixtures/token handlers and env/flag defaults; added testutil Env/personas/receiver/assertions; added `TestDefaultRoundTrip` and scoped package tests. Wave-B seam: admin routes are 23 operations; PF-24 stub files are `tokens.go`, `service_accounts.go`, `webhooks.go`, `tenant.go`, `faults.go`, `fixtures.go`; testutil exports `Env`, `Option`, `WithLocalLogin`, `WithAdminReset`, `WithRateLimit`, `New`, `Do`, `DoRaw`, `DeliverWebhooks`, `RegisterReceiverWebhook`, `UserID`, `Persona`, `Admin`, `Manager`, `Finance`, `Employee`, `Integration`, `Response`, `Receiver`, `Received`, `AssertError`, `AssertPagination`, `AssertKeys`; `server.Config`, `Server`, `New`, `Handler`, `Serve`, `Close` are in place. These contracts are frozen for wave B.
**Verification**: `gofmt -l internal/server internal/webui internal/admin internal/cli internal/testutil internal/fixtures/default_roundtrip_test.go` exit 0, no output. Assigned Go files are all below 1000 lines (1759 aggregate). Current-source webui race suite `go test -count=1 -v -race ./internal/webui/...` exited 0, PASS=3 FAIL=0 SKIP=0; log `/tmp/pf16-s463-webui-race.log` and counts `/tmp/pf16-s463-webui-race-counts.log`, copied to the attempt evidence directory. Moving-tree `go build ./...` exited 1, log `/tmp/pf16-s463-build.log` copied to `tmp/punchflow-kickflow-dummy-s463/PF-16/attempt-1/pf16-s463-build-moving-tree.log`; PF-15-owned unresolved types are the cause. The moving-tree `go test ./internal/webui/... ./internal/testutil/...` log `/tmp/pf16-s463-webui-testutil.log` copied to the same evidence directory: webui PASS=3 FAIL=0 SKIP=0; testutil did not compile due to the same PF-15-owned undefined types. Full server/admin/CLI/testutil race suite, build/vet and version command remain unverified; `TestDefaultRoundTrip` has a current-source failure recorded below. Pre-edit shared CLI hashes are in `pre-edit-hashes.txt`; final shared CLI and fixture hashes are in `shared-post-hashes.txt`, and full source hashes in `final-source-hashes.txt`; no fixture data was modified.
**Reconciliation requests**: `internal/webhook/payload.go:25,41,45,51` references `present.WebhookTenant`, `present.Comment`, `present.TicketWithStep`, and `present.TicketDetail`, absent while PF-15 is in progress. Owner: PF-15 (`internal/present/{ticket.go,ticket_detail.go,comment.go,tenant.go}` and `internal/webhook/payload.go` per the s463 ownership declarations). Final `go build ./...` rerun exited 1; full log `/tmp/pf16-s463-build-final-attempt.log` copied to `tmp/punchflow-kickflow-dummy-s463/PF-16/attempt-1/pf16-s463-build-final-attempt.log`. Resume by rerunning all PF-16 s463 commands after PF-15 lands and those symbols compile; do not modify foreign files.
**Follow-up verification**: `bash -o pipefail -c "go test -count=1 -v -run 'TestDefaultRoundTrip' ./internal/fixtures/... 2>&1 | tee /tmp/pf16-s463-roundtrip.log"` exited 1; counts PASS=0, FAIL=1, SKIP=0 from `/tmp/pf16-s463-roundtrip-counts.log`. The failure at `internal/fixtures/default_roundtrip_test.go:34` is `UNIQUE constraint failed: team_memberships.team_id, team_memberships.user_id` during `fixtures.Load`, so byte-identical export was not reached. Full log is copied to `tmp/punchflow-kickflow-dummy-s463/PF-16/attempt-1/pf16-s463-roundtrip.log`.
**Additional reconciliation request**: The duplicate membership failure originates in accepted PF-06 `internal/fixtures/load.go` (owner per `punchflow-kickflow-dummy-s462-dispatch.json`). PF-14 independently recorded the same fixture-load failure. Route a repair to PF-06 and rerun `TestDefaultRoundTrip` after it lands. Do not modify the accepted loader from PF-16. Required full build/vet/scoped race tests and version command remain unverified while PF-15's payload symbols are missing; final acceptance criterion remains unchecked.


### Session: 2026-10-21 (session 464, PF-16)
**Intent snapshot**: Pre-edit source hashes and source copies (`.source` extension), tree HEAD/status, and per-edit intentions are under `tmp/punchflow-kickflow-dummy-s464/PF-16/attempt-1/pre-edit/` and `edit-intents.md`. The first snapshot attempt briefly copied `.go` into the evidence tree; its initial build failure was captured, the copies were renamed `.source`, and a clean-tree baseline build passed. All baseline, intermediate and final complete logs are retained under `attempt-1/baseline/`, `attempt-1/`, and `attempt-1/final/`.
**Tasks Completed**: Fixed `Env.DoRaw` to clear `RequestURI` on its clone while preserving relative URL resolution. Updated tenant Load/Reset insertion to preserve nonzero fixture timestamps independently, with clock fallback for zero values; Export emits UTC timestamps. Extended `TestDefaultRoundTrip` with a second export fixed-point assertion and added Load/Reset clock-fallback assertions. Exported the current default fixtures through the CLI and replaced the 14 JSON files atomically; normalization against `44e40cc` yields 14 `same`, DIFF=0. Frozen `internal/model/*`, `internal/fixtures/types.go`, and `internal/store/ids.go` are unchanged. Admin stub files match `44e40cc`; `Operations()` has 23 rows.
**Verification**: Final `go build ./...` and scoped `go vet` exited 0. `go test -count=1 -v -race ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/...` exited 0, PASS=22 FAIL=0 SKIP=0. Focused `TestDefaultRoundTrip` and `TestTenantTimestampFallback` race tests exited 0, PASS=2 FAIL=0 SKIP=0; `TZ=Asia/Tokyo` default round-trip exited 0, PASS=1 FAIL=0 SKIP=0. `go test -count=1 -v -race ./internal/fixtures/...` is not green: PASS=35 FAIL=1 SKIP=0, sole failure `TestLoadExportRoundTripMinimal` at `internal/fixtures/fixtures_test.go:56`; its expected fixture uses `2026-10-20T21:00:00+09:00` while accepted CF-04 requires UTC output. CF-01 grep, gofmt, frozen paths and version checks pass. Test logs and command outputs are in `attempt-1/final/`.
**Reconciliation request (owner PF-06)**: `internal/fixtures/fixtures_test.go` compares exported bytes to `internal/fixtures/testdata/minimal/tenant.json`, whose `createdAt`/`updatedAt` use `+09:00`; UTC export correctly canonicalizes them to `Z`. PF-16 is explicitly forbidden from editing `internal/fixtures/testdata/**`, and `fixtures_test.go` is outside PF-16 `writePaths`. Route the test expectation/canonical testdata update to PF-06 (accepted owner), scoped to preserving the UTC contract; resume PF-16 when that owner repair lands and rerun the whole `./internal/fixtures/...` race suite. Do not weaken UTC export or edit the foreign test here.
**Shared-tree compile record**: Builds immediately after edits to `env.go` and `load.go` exited 1 because concurrent PF-15 work left `internal/store/read_tickets.go` referencing undefined `pf08StringPtr`; subsequent build after `export.go` and final build both exited 0 after the shared tree recovered. The PF-16 code caused no compile error. Test-file writes were followed by scoped vet (the first vet caught and then the fix cleared an unqualified external test type).


### Session: 2026-10-21 (session 465, PF-16)
**Intent snapshot and edits**: The immutable native pre-node snapshot, exact edit intentions, and pre/post hashes are under `tmp/punchflow-kickflow-dummy-s465/PF-16/attempt-1/`. Changed only `internal/fixtures/testdata/minimal/tenant.json`, replacing both `2026-10-20T21:00:00+09:00` timestamps with the same instant in canonical UTC (`2026-10-20T12:00:00Z`). `fixtures_test.go` and its byte-identical/fixed-point assertions remain unchanged. The tenant fixture pre-edit SHA-256 is `f016c7ad93d353da81fb05b33f21dfdb3369941255e338eb964ea4ad75acf138`; post-edit hash is in `post-edit-hashes.txt`.
**Verification**: `go build ./...` and scoped `go vet ./internal/server/... ./internal/webui/... ./internal/admin/... ./internal/cli/... ./internal/testutil/... ./internal/fixtures/...` exited 0. The server/webui/admin/cli/testutil uncached race suite exited 0 (PASS=22, FAIL=0, SKIP=0); the whole fixtures uncached race suite exited 0 (PASS=36, FAIL=0, SKIP=0), including `TestLoadExportRoundTripMinimal` and `TestDefaultRoundTrip`; under `TZ=Asia/Tokyo`, both round-trip tests passed (PASS=2, FAIL=0, SKIP=0). `DoRaw` relative URL test passed. Default fixture normalization: 14 same, 0 diff; minimal fixture normalization: 13 same, 0 diff. Version output is `punchflow 0.1.0`; gofmt clean. Full logs are in `tmp/punchflow-kickflow-dummy-s465/PF-16/attempt-1/final/`, with command logs at `/tmp/pf16-s465-*.log`.
**Contract checks**: `git diff ea385c7 -- internal/server internal/testutil internal/admin/routes.go` is empty. The six PF-24 stub files' blob hashes match `ea385c7`. Frozen `internal/model`, `internal/fixtures/types.go`, and `internal/store/ids.go` are unchanged versus `44e40cc`. The CF-01 ban search has no matches (expected `rg` exit 1; recorded as a negative proof, not a required verification gate). No Go source was written in session 465, so no post-write Go compile/vet gate was triggered.

### Session: 2026-10-21 (session 467, PF-27 archival)
Archived by PF-27 after the s467 final gate passed (tmp/punchflow-kickflow-dummy-s467/PF-27/attempt-1/).
