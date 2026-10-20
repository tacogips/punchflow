# PF-01 Foundation Primitives Implementation Plan

**Status**: Ready
**Wave**: 1 | **Depends On**: - | **Plan ID**: PF-01
**Design Reference**: design-docs/specs/architecture.md#package-layout, #developer-tooling; design-docs/specs/design-api-coverage.md#cross-cutting-v1-conventions; design-docs/specs/notes.md INF-33
**Created**: 2026-10-20
**Last Updated**: 2026-10-20

---

## Intent and Context

punchflow is a dev-only dummy kickflow server (Go module `punchflow`, go 1.25.4). The repo is an empty scaffold: `cmd/punchflow/.gitkeep`, `internal/.gitkeep`, `internal/build/VERSION` (contains `0.1.0`), `mise.toml`, `.gitignore`. This plan creates the stdlib-only primitives every other Go plan imports (time formatting, kickflow error type, version, CLI entry, embed anchors) and the tooling files (mise tasks, gitignore).

## Non-goals

- No external Go dependencies (PF-02 owns `go.mod`/`go.sum`).
- No `serve`/`fixtures`/`token` logic (PF-16 replaces the CLI stubs).
- No HTTP middleware (PF-04).
- Do not touch `scripts/`, `packaging/`, `.claude/`, `design-docs/`.

## Files

| Path | Action |
|---|---|
| `internal/kftime/kftime.go`, `internal/kftime/fake.go`, `internal/kftime/kftime_test.go` | create |
| `internal/kferr/kferr.go`, `internal/kferr/kferr_test.go` | create |
| `internal/build/version.go`, `internal/build/version_test.go` | create (keep `VERSION` unchanged) |
| `internal/cli/cli.go`, `internal/cli/cli_test.go` | create (skeleton) |
| `cmd/punchflow/main.go` | create; delete `cmd/punchflow/.gitkeep` (`rm` the file; no git command) |
| `embed.go` (module root, `package punchflow`), `embed_test.go` | create |
| `web/dist/.gitkeep`, `fixtures/default/.gitkeep` | create (empty files) |
| `mise.toml`, `.gitignore` | modify |

## Path Declarations

### writePaths
- `impl-plans/active/pf-01-foundation-primitives.md`
- `internal/kftime/kftime.go`
- `internal/kftime/fake.go`
- `internal/kftime/kftime_test.go`
- `internal/kferr/kferr.go`
- `internal/kferr/kferr_test.go`
- `internal/build/version.go`
- `internal/build/version_test.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `cmd/punchflow/main.go`
- `cmd/punchflow/.gitkeep`
- `embed.go`
- `embed_test.go`
- `web/dist/.gitkeep`
- `fixtures/default/.gitkeep`
- `mise.toml`
- `.gitignore`

### sharedPaths
- (none)

### sharedPathNotes
- (none)

### artifactRoots
- (none)

## Contracts (pin exactly; other plans compile against these)

```go
// internal/kftime
type Clock interface{ Now() time.Time }
type SystemClock struct{}
var JST *time.Location // time.FixedZone("JST", 9*60*60); never time.LoadLocation
func Format(t time.Time) string              // "2006-01-02T15:04:05.000+09:00" in JST
func FormatDate(t time.Time) string          // "2006-01-02" in JST
func ParseInput(s string) (time.Time, error) // the 4 accepted formats; date-only = 00:00 JST
func ToMillis(t time.Time) int64
func FromMillis(ms int64) time.Time
type FakeClock struct{ /* mutex-guarded */ }
func NewFakeClock(t time.Time) *FakeClock
func (c *FakeClock) Now() time.Time; func (c *FakeClock) Set(t time.Time); func (c *FakeClock) Advance(d time.Duration)

// internal/kferr
type Error struct {
    Status  int
    Code    string
    Message string
    Errors  map[string][]string // only for 422 validation errors; nil otherwise
}
func (e *Error) Error() string
func New(status int, code, message string) *Error
func As(err error) (*Error, bool)
func Write(w http.ResponseWriter, err error) // non-*Error -> 500 internal_server_error
```

`kferr` also exports one constructor per documented code (names are the contract): `InvalidParameter(msg)`, `InvalidContentType()`, `InvalidJSON()`, `PayloadTooLarge()`, `InvalidHeader(msg)`, `InvalidAccessToken()`, `UserNotRegistered()`, `UserInactive()`, `InvalidCallerID()`, `InvitationExpired()`, `MissingPermission(msg)`, `UpgradeRequired()`, `InvisibleTicket()`, `UserNotAssigned()`, `TicketCreateRestricted()`, `IPAddressDenied()`, `AdminIsWorking()`, `TenantSuspended()`, `TenantMaintenance()`, `ResourceNotEditable()`, `ResourceNotFound()`, `EndpointNotFound()`, `ValidationFailed(fieldErrors map[string][]string)` (message = first message joined, e.g. `"amount must not be empty"`), `InvalidState(msg)`, `DomainNotAllowed()`, `AssignmentRequiredBeforeApprove()`, `InputRequiredBeforeApprove()`, `RateLimited()`, `Internal()`. HTTP statuses and codes exactly as in the error catalog of `design-api-coverage.md`.

```go
// internal/build
func Version() string // //go:embed VERSION, strings.TrimSpace

// internal/cli
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int

// embed.go (package punchflow)
//go:embed all:web/dist
var WebDist embed.FS
//go:embed all:fixtures/default
var DefaultFixtures embed.FS
```

## Key Points and Pitfalls

- `kferr.Write` body: `{"code":..,"message":..}` plus `"errors"` only when non-nil; never `doc_url`. Use `json.Marshal` (HTML escaping on, no trailing newline; INF-33). Set `Content-Type: application/json; charset=utf-8` before `WriteHeader`. Do NOT use `json.NewEncoder` (adds `\n`).
- `Format` must always print exactly 3 fractional digits and `+09:00` (not `Z`), even for UTC inputs. Truncate (not round) to milliseconds.
- `ParseInput` accepts: `2020-05-01T12:34:56.789+09:00`, `2020-05-01T12:34:56+09:00`, `2020-05-01T12:34:56.789Z`, `2020-05-01`. Anything else returns an error (callers map it to `invalid_parameter`).
- `cli.Run`: `version` prints `punchflow <Version()>` + newline to stdout, exit 0. `serve`, `fixtures`, `token` print `not implemented` to stderr and return 1 (PF-16 replaces them). No args or unknown command: usage to stderr, exit 2.
- `main.go`: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, then `os.Exit(cli.Run(...))`. Keep it under 20 lines.
- `embed.go` must use the `all:` prefix so the `.gitkeep` files satisfy the patterns; otherwise `go build` fails on a fresh clone.
- `.gitignore`: append `/web/dist/app/` and `/.punchflow-data/`. Do not remove existing lines.
- `mise.toml`: keep every existing task and the `[tools]` block unchanged except the tasks below. Use `dir = "web"` for web tasks (not `cd`). Add/modify exactly:
  - `web:install` -> `bun install --frozen-lockfile` (dir web)
  - `web:build` (depends `web:install`) -> `bun run build`
  - `web:test` (depends `web:install`) -> `bun test`
  - `web:typecheck` (depends `web:install`) -> `bun run typecheck`
  - `build` (depends `web:build`) -> `go build -o punchflow ./cmd/punchflow`
  - `dev` (depends `web:build`) -> `go run ./cmd/punchflow serve --enable-local-login --enable-admin-reset`
  - `run` -> `mise run build && ./punchflow serve`
  - `test` -> `go test ./...` then `mise run web:test`
  - `lint` -> `golangci-lint run ./...`, `go vet ./...`, `mise run web:typecheck`
  - `keycloak:up` -> `docker compose -f deploy/keycloak/docker-compose.yml up -d`; `keycloak:down` -> `... down`

## Tests (input -> expected)

- `Format(2020-05-01 03:34:56.789123 UTC)` -> `2020-05-01T12:34:56.789+09:00`
- `Format(t with 0 ns)` -> ends with `.000+09:00`
- each of the 4 input formats -> parses; `2020/05/01` -> error
- `FormatDate(2020-05-01T20:00Z)` -> `2020-05-02`
- `kferr.Write(w, kferr.ValidationFailed({"comment":["must not be empty"]}))` -> 422, body `{"code":"validation_failed","message":"comment must not be empty","errors":{"comment":["must not be empty"]}}`
- `kferr.Write(w, errors.New("boom"))` -> 500 `internal_server_error`
- message containing `<a&b>` -> body bytes contain `<a&b>`, no trailing `\n`
- `cli.Run(["version"])` -> 0 and stdout `punchflow 0.1.0\n`; `cli.Run(["nope"])` -> 2
- `embed_test.go`: `fs.Stat(WebDist, "web/dist/.gitkeep")` and `fs.Stat(DefaultFixtures, "fixtures/default/.gitkeep")` succeed

## Verification (each must exit 0; record exit status and log path)

```bash
go build ./... 2>&1 | tee /tmp/pf01-build.log
go vet ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... ./cmd/... . 2>&1 | tee /tmp/pf01-vet.log
go test ./internal/kftime/... ./internal/kferr/... ./internal/build/... ./internal/cli/... . 2>&1 | tee /tmp/pf01-test.log
gofmt -l internal/kftime internal/kferr internal/build internal/cli cmd embed.go embed_test.go   # must print nothing
mise tasks ls 2>&1 | tee /tmp/pf01-mise.log   # must list web:install web:build web:test web:typecheck build dev test lint keycloak:up keycloak:down
```

## Done Criteria

- [ ] All files in the table exist; `cmd/punchflow/.gitkeep` removed
- [ ] Contract symbols exist with the exact names/signatures above
- [ ] All listed tests exist and pass; gofmt clean
- [ ] `mise tasks ls` shows the new tasks; existing tasks still present

## Execution Protocol

Follow "Shared Execution Protocol" in `impl-plans/README.md` (ownership, fresh reads + sha256, intent snapshot, drift detection, no git state changes, own progress log only).

## Progress Log

### Session: (not started)
**Intent snapshot**: -
**Tasks Completed**: None
**Verification**: -
**Reconciliation requests**: -
